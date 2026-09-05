package plugin

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

type lifecycleAddon struct {
	BasePlugin
	events          *[]string
	fail            string
	panicPhase      string
	block           chan struct{}
	entered         chan struct{}
	ctx             context.Context
	stopCtx         context.Context
	stopWasCanceled bool
	requires        []Dependency
}

func (p *lifecycleAddon) Manifest() Manifest {
	return Manifest{APIVersion: APIVersionV2, ID: p.Name(), Version: "2.0.0", Kind: KindBackendAddon, Requires: p.requires}
}
func (p *lifecycleAddon) record(phase string, ctx context.Context) error {
	*p.events = append(*p.events, phase+":"+p.Name())
	if p.panicPhase == phase {
		panic("test panic")
	}
	if p.fail == phase {
		return errors.New("test failure")
	}
	return nil
}
func (p *lifecycleAddon) Validate(ctx context.Context) error { return p.record("validate", ctx) }
func (p *lifecycleAddon) Initialize(ctx context.Context) error {
	p.ctx = ctx
	if p.block != nil {
		close(p.entered)
		<-p.block
	}
	return p.record("initialize", ctx)
}
func (p *lifecycleAddon) Start(ctx context.Context) error { return p.record("start", ctx) }
func (p *lifecycleAddon) Ready(ctx context.Context) error { return p.record("ready", ctx) }
func (p *lifecycleAddon) Stop(ctx context.Context) error {
	p.stopCtx = ctx
	p.stopWasCanceled = ctx.Err() != nil
	return p.record("stop", ctx)
}

func lifecycleFixture(t *testing.T) (*Lifecycle, *lifecycleAddon, *lifecycleAddon, *[]string) {
	t.Helper()
	events := []string{}
	a := &lifecycleAddon{BasePlugin: BasePlugin{PluginName: "a"}, events: &events}
	b := &lifecycleAddon{BasePlugin: BasePlugin{PluginName: "b"}, events: &events, requires: []Dependency{{ID: "a"}}}
	r := NewRegistry()
	for _, p := range []Plugin{b, a} {
		if err := r.Register(p); err != nil {
			t.Fatal(err)
		}
	}
	return NewLifecycle(r), a, b, &events
}

func TestLifecycleOrderedPhasesAndReverseIdempotentStop(t *testing.T) {
	l, a, b, events := lifecycleFixture(t)
	ctx := context.Background()
	if err := l.Prepare(ctx); err != nil {
		t.Fatal(err)
	}
	if err := l.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := l.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err := l.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	want := []string{"validate:a", "validate:b", "initialize:a", "initialize:b", "start:a", "start:b", "ready:a", "ready:b", "stop:b", "stop:a"}
	if !reflect.DeepEqual(*events, want) {
		t.Fatalf("events = %v", *events)
	}
	if a.ctx.Err() == nil || b.ctx.Err() == nil {
		t.Fatal("plugin contexts not canceled before stop")
	}
	if a.stopWasCanceled {
		t.Fatal("normal stop received canceled application context")
	}
	if l.State() != LifecycleStopped {
		t.Fatalf("state = %s", l.State())
	}
}

func TestLifecycleFailureRollback(t *testing.T) {
	for _, phase := range []string{"validate", "initialize", "start", "ready"} {
		t.Run(phase, func(t *testing.T) {
			l, a, b, events := lifecycleFixture(t)
			b.fail = phase
			err := l.Prepare(context.Background())
			if err == nil {
				err = l.Start(context.Background())
			}
			if err == nil || !strings.Contains(err.Error(), phase) {
				t.Fatalf("error = %v", err)
			}
			if phase == "validate" {
				if len(*events) != 2 {
					t.Fatalf("validate failure caused side effects: %v", *events)
				}
			} else {
				if got := (*events)[len(*events)-2:]; !reflect.DeepEqual(got, []string{"stop:b", "stop:a"}) {
					t.Fatalf("rollback = %v", *events)
				}
				if a.ctx.Err() == nil {
					t.Fatal("rollback failed to cancel workers")
				}
			}
			if err := l.Prepare(context.Background()); !errors.Is(err, ErrLifecycleState) {
				t.Fatalf("restart error = %v", err)
			}
		})
	}
}

func TestLifecyclePanicRollbackAndCleanupContinue(t *testing.T) {
	l, a, b, events := lifecycleFixture(t)
	b.panicPhase = "initialize"
	a.panicPhase = "stop"
	err := l.Prepare(context.Background())
	if err == nil || !strings.Contains(err.Error(), "initialize") || !strings.Contains(err.Error(), "stop") {
		t.Fatalf("error = %v", err)
	}
	if got := (*events)[len(*events)-2:]; !reflect.DeepEqual(got, []string{"stop:b", "stop:a"}) {
		t.Fatalf("events = %v", *events)
	}
}

func TestLifecycleRejectsConcurrentAndOutOfOrderCalls(t *testing.T) {
	l, a, _, _ := lifecycleFixture(t)
	if err := l.Start(context.Background()); !errors.Is(err, ErrLifecycleState) {
		t.Fatalf("start before prepare = %v", err)
	}
	a.block, a.entered = make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- l.Prepare(context.Background()) }()
	<-a.entered
	for _, operation := range []func(context.Context) error{l.Prepare, l.Start, l.Stop} {
		if err := operation(context.Background()); !errors.Is(err, ErrLifecycleState) {
			t.Fatalf("concurrent operation = %v", err)
		}
	}
	close(a.block)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := l.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestLifecycleCanceledPrepareHasNoSideEffects(t *testing.T) {
	l, _, _, events := lifecycleFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := l.Prepare(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
	if len(*events) != 0 {
		t.Fatalf("events = %v", *events)
	}
}

func TestLifecycleLegacyAndMissingDependencies(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(&BasePlugin{PluginName: "legacy"}); err != nil {
		t.Fatal(err)
	}
	l := NewLifecycle(r)
	if err := l.Prepare(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := l.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := l.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	l, _, b, events := lifecycleFixture(t)
	// A separate registry is required: registration already snapshots manifests.
	b.requires = []Dependency{{ID: "absent"}}
	r = NewRegistry()
	if err := r.Register(b); err != nil {
		t.Fatal(err)
	}
	if err := NewLifecycle(r).Prepare(context.Background()); !errors.Is(err, ErrMissingDependency) {
		t.Fatalf("error = %v", err)
	}
	if len(*events) != 0 {
		t.Fatal("missing dependency ran lifecycle")
	}
}

type waitingStopAddon struct {
	BasePlugin
	stopped bool
}

func (p *waitingStopAddon) Stop(ctx context.Context) error {
	<-ctx.Done()
	p.stopped = true
	return ctx.Err()
}

func TestLifecycleStopTimeout(t *testing.T) {
	r := NewRegistry()
	p := &waitingStopAddon{BasePlugin: BasePlugin{PluginName: "waiting"}}
	if err := r.Register(p); err != nil {
		t.Fatal(err)
	}
	l := NewLifecycle(r, LifecycleOptions{StopTimeout: 10 * time.Millisecond})
	if err := l.Prepare(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := l.Stop(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("stop = %v", err)
	}
	if !p.stopped {
		t.Fatal("stop hook skipped")
	}
}
