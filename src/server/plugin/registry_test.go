package plugin

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gin-gonic/gin"
)

type registryTestPlugin struct {
	name        string
	description string
	priority    int
	routePrefix string
}

func (p *registryTestPlugin) Name() string                   { return p.name }
func (p *registryTestPlugin) Description() string            { return p.description }
func (p *registryTestPlugin) Priority() int                  { return p.priority }
func (p *registryTestPlugin) RoutePrefix() string            { return p.routePrefix }
func (p *registryTestPlugin) RegisterRoutes(huma.API)        {}
func (p *registryTestPlugin) Models() []interface{}          { return nil }
func (p *registryTestPlugin) Middlewares() []gin.HandlerFunc { return nil }
func (p *registryTestPlugin) GlobalMiddlewares() []gin.HandlerFunc {
	return nil
}

type registryTestV2Plugin struct {
	*registryTestPlugin
	manifest Manifest
}

func (p *registryTestV2Plugin) Manifest() Manifest { return p.manifest }

func legacyTestPlugin(name string, priority int) *registryTestPlugin {
	return &registryTestPlugin{
		name:        name,
		description: name + " description",
		priority:    priority,
		routePrefix: "/api/v1/addons/" + name,
	}
}

func v2TestPlugin(name string, priority int, configure func(*Manifest)) *registryTestV2Plugin {
	manifest := Manifest{
		APIVersion: APIVersionV2,
		ID:         name,
		Version:    "1.0.0",
		Kind:       KindBackendAddon,
		RouteScopes: []string{
			"/api/v1/addons/" + name,
		},
	}
	if configure != nil {
		configure(&manifest)
	}
	return &registryTestV2Plugin{
		registryTestPlugin: legacyTestPlugin(name, priority),
		manifest:           manifest,
	}
}

func resolvedNames(entries []ResolvedPlugin) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Manifest.ID)
	}
	return names
}

func TestLegacyPluginStillSatisfiesPlugin(t *testing.T) {
	var _ Plugin = (*registryTestPlugin)(nil)

	r := NewRegistry()
	if err := r.Register(legacyTestPlugin("legacy", 100)); err != nil {
		t.Fatalf("register legacy plugin: %v", err)
	}

	resolved, err := r.Resolve()
	if err != nil {
		t.Fatalf("resolve legacy plugin: %v", err)
	}
	if got, want := resolvedNames(resolved), []string{"legacy"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("resolved names = %v, want %v", got, want)
	}
	if got := resolved[0].Manifest.APIVersion; got != APIVersionV1 {
		t.Fatalf("legacy apiVersion = %q, want %q", got, APIVersionV1)
	}
}

func TestRegistryRejectsInvalidPlugins(t *testing.T) {
	tests := []struct {
		name   string
		plugin Plugin
		want   error
	}{
		{name: "nil", plugin: nil, want: ErrNilPlugin},
		{name: "typed nil", plugin: (*registryTestPlugin)(nil), want: ErrNilPlugin},
		{name: "blank name", plugin: legacyTestPlugin("", 100), want: ErrInvalidPlugin},
		{name: "surrounding name whitespace", plugin: legacyTestPlugin(" legacy", 100), want: ErrInvalidPlugin},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewRegistry().Register(tt.plugin)
			if !errors.Is(err, tt.want) {
				t.Fatalf("Register() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestRegistryAllowsLegacyNonCanonicalName(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(legacyTestPlugin("LegacyPlugin", 100)); err != nil {
		t.Fatalf("register V1 plugin with legacy name: %v", err)
	}
}

func TestRegistryRejectsDuplicateWithoutReplacingOriginal(t *testing.T) {
	r := NewRegistry()
	original := legacyTestPlugin("duplicate", 100)
	replacement := legacyTestPlugin("duplicate", 1)
	if err := r.Register(original); err != nil {
		t.Fatalf("register original: %v", err)
	}
	if err := r.Register(replacement); !errors.Is(err, ErrDuplicatePlugin) {
		t.Fatalf("register duplicate error = %v, want %v", err, ErrDuplicatePlugin)
	}

	got, ok := r.Get("duplicate")
	if !ok || got != original {
		t.Fatalf("duplicate registration replaced the original plugin")
	}
}

func TestRegistryResolveIsDeterministic(t *testing.T) {
	registrations := [][]*registryTestPlugin{
		{legacyTestPlugin("zeta", 100), legacyTestPlugin("alpha", 100), legacyTestPlugin("beta", 100), legacyTestPlugin("first", 10)},
		{legacyTestPlugin("beta", 100), legacyTestPlugin("first", 10), legacyTestPlugin("zeta", 100), legacyTestPlugin("alpha", 100)},
	}
	want := []string{"first", "alpha", "beta", "zeta"}

	for i, plugins := range registrations {
		r := NewRegistry()
		for _, candidate := range plugins {
			if err := r.Register(candidate); err != nil {
				t.Fatalf("case %d register %q: %v", i, candidate.Name(), err)
			}
		}
		for attempt := 0; attempt < 5; attempt++ {
			resolved, err := r.Resolve()
			if err != nil {
				t.Fatalf("case %d resolve: %v", i, err)
			}
			if got := resolvedNames(resolved); !reflect.DeepEqual(got, want) {
				t.Fatalf("case %d attempt %d resolved = %v, want %v", i, attempt, got, want)
			}
		}
	}
}

func TestRegistryOrdersDependenciesBeforeDependents(t *testing.T) {
	r := NewRegistry()
	consumer := v2TestPlugin("consumer", 1, func(manifest *Manifest) {
		manifest.Requires = []Dependency{{ID: "provider"}}
		manifest.Optional = []Dependency{{ID: "optional"}}
	})
	for _, candidate := range []Plugin{
		consumer,
		v2TestPlugin("optional", 200, nil),
		v2TestPlugin("provider", 100, nil),
	} {
		if err := r.Register(candidate); err != nil {
			t.Fatalf("register %q: %v", candidate.Name(), err)
		}
	}

	resolved, err := r.Resolve()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	got := resolvedNames(resolved)
	if indexOf(got, "provider") > indexOf(got, "consumer") {
		t.Fatalf("required dependency order = %v", got)
	}
	if indexOf(got, "optional") > indexOf(got, "consumer") {
		t.Fatalf("present optional dependency order = %v", got)
	}
}

func TestRegistryAllowsMissingOptionalDependency(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(v2TestPlugin("consumer", 100, func(manifest *Manifest) {
		manifest.Optional = []Dependency{{ID: "not-installed"}}
	})); err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := r.Resolve(); err != nil {
		t.Fatalf("missing optional dependency must be allowed: %v", err)
	}
}

func TestRegistryRejectsMissingRequiredDependency(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(v2TestPlugin("consumer", 100, func(manifest *Manifest) {
		manifest.Requires = []Dependency{{ID: "missing"}}
	})); err != nil {
		t.Fatalf("register: %v", err)
	}

	_, err := r.Resolve()
	if !errors.Is(err, ErrMissingDependency) || !strings.Contains(err.Error(), "consumer") || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("Resolve() error = %v, want missing dependency details", err)
	}
}

func TestRegistryRejectsDeclaredConflict(t *testing.T) {
	r := NewRegistry()
	for _, candidate := range []Plugin{
		v2TestPlugin("alpha", 100, func(manifest *Manifest) {
			manifest.Conflicts = []string{"beta"}
		}),
		v2TestPlugin("beta", 100, nil),
	} {
		if err := r.Register(candidate); err != nil {
			t.Fatalf("register %q: %v", candidate.Name(), err)
		}
	}

	_, err := r.Resolve()
	if !errors.Is(err, ErrPluginConflict) || !strings.Contains(err.Error(), "alpha") || !strings.Contains(err.Error(), "beta") {
		t.Fatalf("Resolve() error = %v, want conflict details", err)
	}
}

func TestRegistryRejectsDependencyCycle(t *testing.T) {
	r := NewRegistry()
	for _, dependency := range []struct {
		id       string
		requires string
	}{{"alpha", "beta"}, {"beta", "gamma"}, {"gamma", "alpha"}} {
		candidate := v2TestPlugin(dependency.id, 100, func(manifest *Manifest) {
			manifest.Requires = []Dependency{{ID: dependency.requires}}
		})
		if err := r.Register(candidate); err != nil {
			t.Fatalf("register %q: %v", dependency.id, err)
		}
	}

	_, err := r.Resolve()
	if !errors.Is(err, ErrDependencyCycle) || !strings.Contains(err.Error(), "alpha, beta, gamma") {
		t.Fatalf("Resolve() error = %v, want stable cycle details", err)
	}
}

func TestRegistryReturnsIndependentSnapshots(t *testing.T) {
	r := NewRegistry()
	alpha := legacyTestPlugin("alpha", 100)
	beta := legacyTestPlugin("beta", 100)
	for _, candidate := range []Plugin{beta, alpha} {
		if err := r.Register(candidate); err != nil {
			t.Fatalf("register %q: %v", candidate.Name(), err)
		}
	}

	all := r.All()
	delete(all, "alpha")
	all["beta"] = alpha
	all["injected"] = alpha
	if got := r.Count(); got != 2 {
		t.Fatalf("Count() = %d after mutating All snapshot, want 2", got)
	}
	if got, _ := r.Get("beta"); got != beta {
		t.Fatalf("mutating All snapshot replaced registry value")
	}

	names := r.Names()
	if want := []string{"alpha", "beta"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("Names() = %v, want %v", names, want)
	}
	names[0] = "changed"
	if got := r.Names(); !reflect.DeepEqual(got, []string{"alpha", "beta"}) {
		t.Fatalf("Names() leaked mutable state: %v", got)
	}
}

func TestRegistryRejectsRegistrationAfterFreeze(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(legacyTestPlugin("alpha", 100)); err != nil {
		t.Fatalf("register alpha: %v", err)
	}
	if err := r.Freeze(); err != nil {
		t.Fatalf("freeze: %v", err)
	}
	if err := r.Register(legacyTestPlugin("beta", 100)); !errors.Is(err, ErrRegistryFrozen) {
		t.Fatalf("register after freeze error = %v, want %v", err, ErrRegistryFrozen)
	}
}

func TestRegistryRejectsManifestIdentityMismatch(t *testing.T) {
	r := NewRegistry()
	err := r.Register(v2TestPlugin("implementation-name", 100, func(manifest *Manifest) {
		manifest.ID = "different-manifest-id"
	}))
	if !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("Register() error = %v, want %v", err, ErrInvalidManifest)
	}
}

func TestRegistryRejectsUnsupportedManifestKindAndInvalidVersion(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*Manifest)
	}{
		{name: "reserved strategy provider kind", configure: func(manifest *Manifest) {
			manifest.Kind = KindStrategyProvider
		}},
		{name: "invalid semantic version", configure: func(manifest *Manifest) {
			manifest.Version = "latest"
		}},
		{name: "invalid numeric prerelease", configure: func(manifest *Manifest) {
			manifest.Version = "1.0.0-01"
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewRegistry().Register(v2TestPlugin("candidate", 100, tt.configure))
			if !errors.Is(err, ErrInvalidManifest) {
				t.Fatalf("Register() error = %v, want %v", err, ErrInvalidManifest)
			}
		})
	}
}

func TestRegistryRejectsUnsupportedDependencyVersionConstraint(t *testing.T) {
	r := NewRegistry()
	err := r.Register(v2TestPlugin("consumer", 100, func(manifest *Manifest) {
		manifest.Requires = []Dependency{{ID: "provider", Version: ">=2.0.0"}}
	}))
	if !errors.Is(err, ErrInvalidManifest) || !strings.Contains(err.Error(), "version constraints") {
		t.Fatalf("Register() error = %v, want unsupported version constraint", err)
	}
}

func TestRegistrySnapshotsManifestData(t *testing.T) {
	requires := []Dependency{{ID: "provider"}}
	scopes := []string{"/api/v1/addons/consumer"}
	consumer := v2TestPlugin("consumer", 100, func(manifest *Manifest) {
		manifest.Requires = requires
		manifest.RouteScopes = scopes
	})
	r := NewRegistry()
	if err := r.Register(v2TestPlugin("provider", 100, nil)); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if err := r.Register(consumer); err != nil {
		t.Fatalf("register consumer: %v", err)
	}

	requires[0].ID = "mutated"
	scopes[0] = "/mutated"
	consumer.manifest.Requires[0].ID = "also-mutated"
	consumer.manifest.RouteScopes[0] = "/also-mutated"

	first, err := r.Resolve()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got := first[1].Manifest.Requires[0].ID; got != "provider" {
		t.Fatalf("registered manifest dependency = %q, want provider", got)
	}
	first[1].Manifest.Requires[0].ID = "caller-mutated"
	first[1].Manifest.RouteScopes[0] = "/caller-mutated"

	second, err := r.Resolve()
	if err != nil {
		t.Fatalf("resolve again: %v", err)
	}
	if got := second[1].Manifest.Requires[0].ID; got != "provider" {
		t.Fatalf("resolved manifest leaked dependency mutation: %q", got)
	}
	if got := second[1].Manifest.RouteScopes[0]; got != "/api/v1/addons/consumer" {
		t.Fatalf("resolved manifest leaked scope mutation: %q", got)
	}
}

func indexOf(values []string, target string) int {
	for index, value := range values {
		if value == target {
			return index
		}
	}
	return -1
}
