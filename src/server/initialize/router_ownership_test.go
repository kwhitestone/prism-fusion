package initialize

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/kwhitestone/prism-fusion/global"
	"github.com/kwhitestone/prism-fusion/plugin"
	"go.uber.org/zap"
)

type ownershipPlugin struct {
	plugin.BasePlugin
	prefix string
	path   string
	method string
}

func (p *ownershipPlugin) RoutePrefix() string { return p.prefix }

func (p *ownershipPlugin) RegisterRoutes(api huma.API) {
	if p.path == "" {
		return
	}
	huma.Register(api, huma.Operation{
		OperationID: p.Name(),
		Method:      p.method,
		Path:        p.path,
	}, func(context.Context, *struct{}) (*struct {
		Body struct {
			OK bool `json:"ok"`
		}
	}, error) {
		response := &struct {
			Body struct {
				OK bool `json:"ok"`
			}
		}{}
		response.Body.OK = true
		return response, nil
	})
}

type ownershipV2Plugin struct {
	*ownershipPlugin
	scopes []string
}

func (p *ownershipV2Plugin) Manifest() plugin.Manifest {
	return plugin.Manifest{
		APIVersion:  plugin.APIVersionV2,
		ID:          p.Name(),
		Version:     "1.0.0",
		Kind:        plugin.KindBackendAddon,
		RouteScopes: p.scopes,
	}
}

func routeOwner(id, scope, path string) *ownershipPlugin {
	return &ownershipPlugin{
		BasePlugin: plugin.BasePlugin{PluginName: id},
		prefix:     scope,
		path:       path,
		method:     http.MethodGet,
	}
}

// Each child process exercises the public bootstrap with a fresh global
// registry, including real Huma/Gin route registration and HTTP dispatch.
func TestRoutersPluginRouteOwnership(t *testing.T) {
	const childKey = "PRISM_TEST_ROUTE_OWNERSHIP_CASE"
	tests := []struct {
		name      string
		plugins   []plugin.Plugin
		wantPanic string
		paths     []string
	}{
		{
			name:    "legacy routes without middleware",
			plugins: []plugin.Plugin{routeOwner("legacy", "/api/v1/addons/legacy", "/api/v1/addons/legacy/items")},
			paths:   []string{"/api/v1/addons/legacy/items", "/health", "/openapi.json"},
		},
		{
			name: "V2 explicit public callback scope",
			plugins: []plugin.Plugin{&ownershipV2Plugin{
				ownershipPlugin: routeOwner("callback", "/api/v1/addons/callback", "/callbacks/provider"),
				scopes:          []string{"/callbacks/provider"},
			}},
			paths: []string{"/callbacks/provider"},
		},
		{
			name:      "legacy route escapes without middleware",
			plugins:   []plugin.Plugin{routeOwner("legacy", "/api/v1/addons/legacy", "/api/v1/admin/items")},
			wantPanic: "outside route scopes",
		},
		{
			name: "V2 route escapes without middleware",
			plugins: []plugin.Plugin{&ownershipV2Plugin{
				ownershipPlugin: routeOwner("v2", "/api/v1/addons/v2", "/api/v1/addons/other/items"),
				scopes:          []string{"/api/v1/addons/v2"},
			}},
			wantPanic: "outside route scopes",
		},
		{
			name:      "similar prefix is outside scope",
			plugins:   []plugin.Plugin{routeOwner("legacy", "/api/v1/addons/legacy", "/api/v1/addons/legacy-other/items")},
			wantPanic: "outside route scopes",
		},
		{
			name:      "no scope cannot register routes",
			plugins:   []plugin.Plugin{routeOwner("unscoped", "", "/callbacks/provider")},
			wantPanic: "outside route scopes",
		},
		{
			name:    "no route plugin may have no scope",
			plugins: []plugin.Plugin{routeOwner("provider", "", "")},
			paths:   []string{"/health"},
		},
		{
			name: "overlapping declarations without middleware",
			plugins: []plugin.Plugin{
				routeOwner("broad", "/api/v1/addons", "/api/v1/addons/shared/broad"),
				routeOwner("child", "/api/v1/addons/shared", "/api/v1/addons/shared/child"),
			},
			wantPanic: "overlaps plugin",
		},
		{
			name: "same declaration without middleware",
			plugins: []plugin.Plugin{
				routeOwner("first", "/callbacks/shared", "/callbacks/shared/first"),
				routeOwner("second", "/callbacks/shared", "/callbacks/shared/second"),
			},
			wantPanic: "overlaps plugin",
		},
		{
			name: "disjoint auth and API key endpoints",
			plugins: []plugin.Plugin{
				&ownershipV2Plugin{
					ownershipPlugin: routeOwner("auth", "/api/v1/addons/auth", "/api/v1/addons/auth/login"),
					scopes:          []string{"/api/v1/addons/auth/login", "/api/v1/addons/auth/logout"},
				},
				routeOwner("apikey", "/api/v1/addons/auth/api-keys", "/api/v1/addons/auth/api-keys/list"),
			},
			paths: []string{"/api/v1/addons/auth/login", "/api/v1/addons/auth/api-keys/list"},
		},
		{
			name: "core endpoint cannot be claimed using another method",
			plugins: []plugin.Plugin{&ownershipPlugin{
				BasePlugin: plugin.BasePlugin{PluginName: "health-owner"},
				prefix:     "/health", path: "/health", method: http.MethodPost,
			}},
			wantPanic: "reserved core",
		},
		{
			name:      "core static namespace cannot be claimed",
			plugins:   []plugin.Plugin{routeOwner("assets-owner", "/assets/business", "")},
			wantPanic: "reserved core",
		},
		{
			name:      "core schema namespace cannot be claimed",
			plugins:   []plugin.Plugin{routeOwner("schema-owner", "/schemas/business", "")},
			wantPanic: "reserved core",
		},
		{
			name:      "core docs descendants cannot be claimed",
			plugins:   []plugin.Plugin{routeOwner("docs-owner", "/docs/business", "/docs/business/items")},
			wantPanic: "reserved core",
		},
		{
			name:      "core parent namespace cannot be claimed",
			plugins:   []plugin.Plugin{routeOwner("api-owner", "/api/v1", "/api/v1/business/items")},
			wantPanic: "reserved core",
		},
		{
			name:      "core system namespace cannot be claimed",
			plugins:   []plugin.Plugin{routeOwner("system-owner", "/api/v1/system/business", "/api/v1/system/business/items")},
			wantPanic: "reserved core",
		},
		{
			name:      "root ownership cannot absorb core routes",
			plugins:   []plugin.Plugin{routeOwner("root-owner", "/", "/business/items")},
			wantPanic: "reserved core",
		},
	}

	child := os.Getenv(childKey)
	for _, tt := range tests {
		if child != "" {
			if child != tt.name {
				continue
			}
			global.PRISM_LOG = zap.NewNop()
			global.PRISM_CONFIG.System.Env = "public"
			for _, candidate := range tt.plugins {
				plugin.Register(candidate)
			}
			defer func() {
				recovered := recover()
				if tt.wantPanic == "" && recovered != nil {
					t.Fatalf("unexpected startup panic: %v", recovered)
				}
				if tt.wantPanic != "" && (recovered == nil || !strings.Contains(fmt.Sprint(recovered), tt.wantPanic)) {
					t.Fatalf("startup panic = %v, want error containing %q", recovered, tt.wantPanic)
				}
			}()
			engine := Routers()
			for _, path := range tt.paths {
				response := httptest.NewRecorder()
				engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
				if response.Code != http.StatusOK {
					t.Fatalf("GET %s status = %d, body = %s", path, response.Code, response.Body.String())
				}
			}
			return
		}
		t.Run(tt.name, func(t *testing.T) {
			command := exec.Command(os.Args[0], "-test.run=^TestRoutersPluginRouteOwnership$")
			command.Env = append(os.Environ(), childKey+"="+tt.name)
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("bootstrap regression: %v\n%s", err, output)
			}
		})
	}
	if child != "" {
		t.Fatalf("unknown bootstrap test case %q", child)
	}
}
