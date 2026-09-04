package plugin

import (
	"errors"
	"reflect"
	"testing"
)

type activationTestPlugin struct {
	*registryTestV2Plugin
	enabled bool
}

func (p *activationTestPlugin) PluginEnabled() bool { return p.enabled }

type legacyEnabledMethodPlugin struct {
	*registryTestPlugin
}

// Enabled is a business method that predates the V2 activation contract. A V1
// plugin must not be activated or deactivated merely because this name happens
// to match a framework extension method.
func (p *legacyEnabledMethodPlugin) Enabled() bool { return false }

func TestRegistryDoesNotTreatLegacyEnabledMethodAsActivationContract(t *testing.T) {
	r := NewRegistry()
	candidate := &legacyEnabledMethodPlugin{
		registryTestPlugin: &registryTestPlugin{name: "legacy-enabled", priority: 100},
	}
	if err := r.Register(candidate); err != nil {
		t.Fatalf("register: %v", err)
	}

	resolved, err := r.Resolve()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got := resolvedNames(resolved); !reflect.DeepEqual(got, []string{"legacy-enabled"}) {
		t.Fatalf("legacy plugin was implicitly deactivated: %v", got)
	}
}

func TestRegistryResolvesOnlyActivePlugins(t *testing.T) {
	r := NewRegistry()
	disabledProvider := &activationTestPlugin{
		registryTestV2Plugin: v2TestPlugin("provider", 10, nil),
		enabled:              false,
	}
	consumer := v2TestPlugin("consumer", 20, func(manifest *Manifest) {
		manifest.Optional = []Dependency{{ID: "provider"}}
	})
	for _, candidate := range []Plugin{disabledProvider, consumer} {
		if err := r.Register(candidate); err != nil {
			t.Fatalf("register %q: %v", candidate.Name(), err)
		}
	}

	resolved, err := r.Resolve()
	if err != nil {
		t.Fatalf("resolve with inactive optional dependency: %v", err)
	}
	if got, want := resolvedNames(resolved), []string{"consumer"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("resolved names = %v, want %v", got, want)
	}
}

func TestRegistryTreatsInactiveRequiredPluginAsMissing(t *testing.T) {
	r := NewRegistry()
	disabledProvider := &activationTestPlugin{
		registryTestV2Plugin: v2TestPlugin("provider", 10, nil),
		enabled:              false,
	}
	consumer := v2TestPlugin("consumer", 20, func(manifest *Manifest) {
		manifest.Requires = []Dependency{{ID: "provider"}}
	})
	for _, candidate := range []Plugin{disabledProvider, consumer} {
		if err := r.Register(candidate); err != nil {
			t.Fatalf("register %q: %v", candidate.Name(), err)
		}
	}

	if _, err := r.Resolve(); !errors.Is(err, ErrMissingDependency) {
		t.Fatalf("Resolve() error = %v, want %v", err, ErrMissingDependency)
	}
}

func TestRegistryFreezesActivationDecision(t *testing.T) {
	r := NewRegistry()
	candidate := &activationTestPlugin{
		registryTestV2Plugin: v2TestPlugin("switchable", 100, nil),
		enabled:              true,
	}
	if err := r.Register(candidate); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := r.Freeze(); err != nil {
		t.Fatalf("freeze: %v", err)
	}
	candidate.enabled = false

	resolved, err := r.Resolve()
	if err != nil {
		t.Fatalf("resolve frozen registry: %v", err)
	}
	if got := resolvedNames(resolved); !reflect.DeepEqual(got, []string{"switchable"}) {
		t.Fatalf("frozen activation changed: %v", got)
	}
}

type registryInspectingActivation struct {
	*registryTestV2Plugin
	registry     *Registry
	reentrantErr error
	inspected    bool
}

func (p *registryInspectingActivation) PluginEnabled() bool {
	_, found := p.registry.Get(p.Name())
	resolveResult, resolveErr := p.registry.Resolve()
	freezeErr := p.registry.Freeze()
	p.inspected = found && p.registry.Count() == 1 && len(p.registry.Names()) == 1 &&
		resolveResult == nil && errors.Is(resolveErr, p.reentrantErr) && errors.Is(freezeErr, p.reentrantErr)
	return true
}

func TestActivationCanInspectRegistryWithoutDeadlock(t *testing.T) {
	r := NewRegistry()
	candidate := &registryInspectingActivation{
		registryTestV2Plugin: v2TestPlugin("inspector", 100, nil),
		registry:             r,
		reentrantErr:         ErrRegistryFreezing,
	}
	if err := r.Register(candidate); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := r.Freeze(); err != nil {
		t.Fatalf("freeze: %v", err)
	}
	if !candidate.inspected {
		t.Fatal("activation could not inspect registry safely")
	}
}

func TestPreviewActivationCannotReenterResolution(t *testing.T) {
	r := NewRegistry()
	candidate := &registryInspectingActivation{
		registryTestV2Plugin: v2TestPlugin("preview-inspector", 100, nil),
		registry:             r,
		reentrantErr:         ErrRegistryResolving,
	}
	if err := r.Register(candidate); err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := r.Resolve(); err != nil {
		t.Fatalf("resolve preview: %v", err)
	}
	if !candidate.inspected {
		t.Fatal("preview activation could not inspect registry safely")
	}
}

type blockingActivation struct {
	*registryTestV2Plugin
	started chan struct{}
	release chan struct{}
}

func (p *blockingActivation) PluginEnabled() bool {
	close(p.started)
	<-p.release
	return true
}

func TestRegistryRejectsRegistrationWhileFreezeIsInProgress(t *testing.T) {
	r := NewRegistry()
	candidate := &blockingActivation{
		registryTestV2Plugin: v2TestPlugin("blocking", 100, nil),
		started:              make(chan struct{}),
		release:              make(chan struct{}),
	}
	if err := r.Register(candidate); err != nil {
		t.Fatalf("register: %v", err)
	}
	freezeResult := make(chan error, 1)
	go func() {
		freezeResult <- r.Freeze()
	}()
	<-candidate.started

	if err := r.Register(v2TestPlugin("late", 100, nil)); !errors.Is(err, ErrRegistryFrozen) {
		t.Fatalf("register during freeze error = %v, want %v", err, ErrRegistryFrozen)
	}
	close(candidate.release)
	if err := <-freezeResult; err != nil {
		t.Fatalf("freeze: %v", err)
	}
}

func TestRegistryDoesNotFreezeDuringPreviewResolution(t *testing.T) {
	r := NewRegistry()
	candidate := &blockingActivation{
		registryTestV2Plugin: v2TestPlugin("preview-blocking", 100, nil),
		started:              make(chan struct{}),
		release:              make(chan struct{}),
	}
	if err := r.Register(candidate); err != nil {
		t.Fatalf("register: %v", err)
	}
	resolveResult := make(chan error, 1)
	go func() {
		_, err := r.Resolve()
		resolveResult <- err
	}()
	<-candidate.started

	if err := r.Freeze(); !errors.Is(err, ErrRegistryResolving) {
		t.Fatalf("freeze during preview error = %v, want %v", err, ErrRegistryResolving)
	}
	close(candidate.release)
	if err := <-resolveResult; err != nil {
		t.Fatalf("resolve preview: %v", err)
	}
}
