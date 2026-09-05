package plugin

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Lifecycle contracts are optional. Neither Plugin nor BasePlugin gains methods.
// Validate must be side-effect free; Initialize runs before model migration.
// Start must return after launching workers, and Ready must verify usability.
// Stop must tolerate partial initialization and cooperate with its deadline.
type Validator interface{ Validate(context.Context) error }
type Initializer interface{ Initialize(context.Context) error }
type Starter interface{ Start(context.Context) error }
type Readiness interface{ Ready(context.Context) error }
type Stopper interface{ Stop(context.Context) error }

type LifecycleState string

const (
	LifecycleNew       LifecycleState = "new"
	LifecyclePreparing LifecycleState = "preparing"
	LifecyclePrepared  LifecycleState = "prepared"
	LifecycleStarting  LifecycleState = "starting"
	LifecycleReady     LifecycleState = "ready"
	LifecycleStopping  LifecycleState = "stopping"
	LifecycleStopped   LifecycleState = "stopped"
)

var ErrLifecycleState = errors.New("invalid plugin lifecycle state")

type LifecycleOptions struct {
	// StopTimeout bounds cooperative rollback and shutdown; default 30 seconds.
	StopTimeout time.Duration
}

// Lifecycle owns one startup/shutdown attempt. Concurrent or reentrant stages
// fail explicitly instead of executing hooks twice or deadlocking callbacks.
type Lifecycle struct {
	mu          sync.Mutex
	registry    *Registry
	state       LifecycleState
	resolved    []ResolvedPlugin
	entered     []ResolvedPlugin
	ctx         context.Context
	cancel      context.CancelFunc
	stopTimeout time.Duration
	stopErr     error
}

// NewLifecycle uses the process registry when registry is nil. A failed or
// stopped instance cannot restart; create a new application process instead.
func NewLifecycle(registry *Registry, options ...LifecycleOptions) *Lifecycle {
	if registry == nil {
		registry = defaultRegistry
	}
	timeout := 30 * time.Second
	if len(options) > 0 && options[0].StopTimeout > 0 {
		timeout = options[0].StopTimeout
	}
	return &Lifecycle{registry: registry, state: LifecycleNew, stopTimeout: timeout}
}

func (l *Lifecycle) State() LifecycleState {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.state
}

func (l *Lifecycle) transition(next LifecycleState, allowed ...LifecycleState) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, state := range allowed {
		if l.state == state {
			l.state = next
			return nil
		}
	}
	return fmt.Errorf("%w: cannot enter %s from %s", ErrLifecycleState, next, l.state)
}

// Prepare freezes configuration-dependent activation and validates every addon
// before initializing any addon. Attempted initializers participate in rollback.
func (l *Lifecycle) Prepare(ctx context.Context) (err error) {
	if err = l.transition(LifecyclePreparing, LifecycleNew); err != nil {
		return err
	}
	l.ctx, l.cancel = context.WithCancel(ctx)
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("prepare plugins panicked: %v", recovered)
		}
		if err != nil {
			err = errors.Join(err, l.rollback())
		}
	}()
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = l.registry.Freeze(); err != nil {
		return err
	}
	if l.resolved, err = l.registry.Resolve(); err != nil {
		return err
	}
	for _, entry := range l.resolved {
		if validator, ok := entry.Plugin.(Validator); ok {
			if err = lifecycleHook(l.ctx, entry.Manifest.ID, "validate", validator.Validate); err != nil {
				return err
			}
		}
	}
	for _, entry := range l.resolved {
		if err = l.ctx.Err(); err != nil {
			return err
		}
		l.entered = append(l.entered, entry)
		if initializer, ok := entry.Plugin.(Initializer); ok {
			if err = lifecycleHook(l.ctx, entry.Manifest.ID, "initialize", initializer.Initialize); err != nil {
				return err
			}
		}
	}
	return l.transition(LifecyclePrepared, LifecyclePreparing)
}

// Start launches all addons in dependency order, then verifies all readiness
// checks. The host must not expose HTTP until this method succeeds.
func (l *Lifecycle) Start(ctx context.Context) (err error) {
	if err = l.transition(LifecycleStarting, LifecyclePrepared); err != nil {
		return err
	}
	stopCancellation := context.AfterFunc(ctx, l.cancel)
	defer stopCancellation()
	defer func() {
		if err != nil {
			err = errors.Join(err, l.rollback())
		}
	}()
	if err = ctx.Err(); err != nil {
		return err
	}
	for _, entry := range l.resolved {
		if starter, ok := entry.Plugin.(Starter); ok {
			if err = lifecycleHook(l.ctx, entry.Manifest.ID, "start", starter.Start); err != nil {
				return err
			}
		}
	}
	for _, entry := range l.resolved {
		if ready, ok := entry.Plugin.(Readiness); ok {
			if err = lifecycleHook(l.ctx, entry.Manifest.ID, "ready", ready.Ready); err != nil {
				return err
			}
		}
	}
	if err = l.ctx.Err(); err != nil {
		return err
	}
	return l.transition(LifecycleReady, LifecycleStarting)
}

func (l *Lifecycle) rollback() error {
	if err := l.transition(LifecycleStopping, LifecyclePreparing, LifecycleStarting); err != nil {
		return err
	}
	return l.stop(context.WithoutCancel(l.ctx))
}

// Stop cancels worker contexts before invoking every entered addon's Stop in
// reverse dependency order. The supplied context is for cleanup, not workers.
// Repeated completed calls return the original cleanup result.
func (l *Lifecycle) Stop(ctx context.Context) error {
	l.mu.Lock()
	if l.state == LifecycleStopped {
		err := l.stopErr
		l.mu.Unlock()
		return err
	}
	l.mu.Unlock()
	if err := l.transition(LifecycleStopping, LifecycleNew, LifecyclePrepared, LifecycleReady); err != nil {
		return err
	}
	return l.stop(ctx)
}

func (l *Lifecycle) stop(ctx context.Context) error {
	if l.cancel != nil {
		l.cancel()
	}
	cleanupCtx, cancel := context.WithTimeout(ctx, l.stopTimeout)
	defer cancel()
	var result error
	for i := len(l.entered) - 1; i >= 0; i-- {
		entry := l.entered[i]
		if stopper, ok := entry.Plugin.(Stopper); ok {
			// Cleanup still runs after deadline so later owners can release resources.
			result = errors.Join(result, invokeLifecycleHook(cleanupCtx, entry.Manifest.ID, "stop", stopper.Stop))
		}
	}
	l.mu.Lock()
	l.stopErr, l.state = result, LifecycleStopped
	l.mu.Unlock()
	return result
}

func lifecycleHook(ctx context.Context, id, phase string, hook func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("plugin %s %s: %w", id, phase, err)
	}
	if err := invokeLifecycleHook(ctx, id, phase, hook); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("plugin %s %s: %w", id, phase, err)
	}
	return nil
}

func invokeLifecycleHook(ctx context.Context, id, phase string, hook func(context.Context) error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("plugin %s %s panicked: %v", id, phase, recovered)
		}
	}()
	if err = hook(ctx); err != nil {
		return fmt.Errorf("plugin %s %s: %w", id, phase, err)
	}
	return nil
}
