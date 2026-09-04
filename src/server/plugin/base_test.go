package plugin

import "testing"

func TestBasePluginDefaults(t *testing.T) {
	candidate := &BasePlugin{
		PluginName:        "sample",
		PluginDescription: "sample plugin",
	}
	if candidate.Name() != "sample" || candidate.Description() != "sample plugin" {
		t.Fatalf("unexpected base metadata: name=%q description=%q", candidate.Name(), candidate.Description())
	}
	if candidate.Priority() != 100 || candidate.RoutePrefix() != "/api/v1/addons/sample" {
		t.Fatalf("unexpected base ordering or route defaults")
	}
	candidate.RegisterRoutes(nil)
	if candidate.Models() != nil || candidate.Middlewares() != nil || candidate.GlobalMiddlewares() != nil {
		t.Fatal("base plugin extension defaults must be nil")
	}
}

func TestNormalizeRouteScopeRejectsAmbiguousScopes(t *testing.T) {
	for _, scope := range []string{"", "relative", " /api", "/api?tenant=1", "/api#fragment", "/api//v1", "/api/../admin", "/api/:id", "/api/*path", "/api/{id}"} {
		if _, err := NormalizeRouteScope(scope); err == nil {
			t.Fatalf("NormalizeRouteScope(%q) unexpectedly succeeded", scope)
		}
	}
	if got, err := NormalizeRouteScope("/api/v1/"); err != nil || got != "/api/v1" {
		t.Fatalf("NormalizeRouteScope() = %q, %v; want /api/v1", got, err)
	}
}
