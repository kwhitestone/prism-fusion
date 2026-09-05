package initialize

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humagin"
	"github.com/gin-gonic/gin"
)

func TestScopeMiddlewareUsesPathSegmentBoundaries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name    string
		scope   string
		path    string
		wantHit bool
	}{
		{name: "exact", scope: "/api/v1/addons/auth", path: "/api/v1/addons/auth", wantHit: true},
		{name: "child", scope: "/api/v1/addons/auth", path: "/api/v1/addons/auth/session", wantHit: true},
		{name: "scope trailing slash", scope: "/api/v1/addons/auth/", path: "/api/v1/addons/auth/session", wantHit: true},
		{name: "lookalike suffix", scope: "/api/v1/addons/auth", path: "/api/v1/addons/authz", wantHit: false},
		{name: "lookalike separator", scope: "/api/v1/addons/auth", path: "/api/v1/addons/auth-other", wantHit: false},
		{name: "parent", scope: "/api/v1/addons/auth", path: "/api/v1/addons", wantHit: false},
		{name: "root is explicit global scope", scope: "/", path: "/anything", wantHit: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hits := 0
			router := gin.New()
			router.Use(scopeMiddleware(tt.scope, func(c *gin.Context) {
				hits++
				c.Next()
			}))
			router.GET("/*path", func(c *gin.Context) {
				c.Status(http.StatusNoContent)
			})

			request := httptest.NewRequest(http.MethodGet, tt.path, nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			wantHits := 0
			if tt.wantHit {
				wantHits = 1
			}
			if hits != wantHits {
				t.Fatalf("middleware hits = %d, want %d", hits, wantHits)
			}
		})
	}
}

func TestScopeMiddlewareRejectsInvalidScopes(t *testing.T) {
	for _, scope := range []string{"", "relative", "/api/../admin", "/api/v1/tenants/:tenant"} {
		t.Run(scope, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatalf("scopeMiddleware(%q) did not panic", scope)
				}
			}()
			scopeMiddleware(scope, func(c *gin.Context) { c.Next() })
		})
	}
}

func TestValidateAddedPluginRoutes(t *testing.T) {
	before := []gin.RouteInfo{{Method: http.MethodGet, Path: "/health"}}
	inside := append(append([]gin.RouteInfo(nil), before...), gin.RouteInfo{
		Method: http.MethodGet,
		Path:   "/api/v1/addons/example/items/:id",
	})
	if err := validateAddedPluginRoutes("example", []string{"/api/v1/addons/example"}, before, inside); err != nil {
		t.Fatalf("validate in-scope route: %v", err)
	}

	outside := append(append([]gin.RouteInfo(nil), before...), gin.RouteInfo{
		Method: http.MethodPost,
		Path:   "/api/v1/admin/example",
	})
	err := validateAddedPluginRoutes("example", []string{"/api/v1/addons/example"}, before, outside)
	if err == nil {
		t.Fatal("out-of-scope plugin route unexpectedly passed validation")
	}
}

func TestValidateAddedPluginRoutesWithHumaAdapter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := humagin.New(engine, huma.DefaultConfig("scope-test", "1.0.0"))
	before := engine.Routes()
	type output struct {
		Body struct {
			OK bool `json:"ok"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "scopeTestRoute",
		Method:      http.MethodGet,
		Path:        "/api/v1/addons/example/items/{id}",
	}, func(context.Context, *struct {
		ID string `path:"id"`
	}) (*output, error) {
		response := &output{}
		response.Body.OK = true
		return response, nil
	})

	if err := validateAddedPluginRoutes(
		"example",
		[]string{"/api/v1/addons/example"},
		before,
		engine.Routes(),
	); err != nil {
		t.Fatalf("validate Huma route: %v", err)
	}
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/addons/example/items/42", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("parameterized Huma route status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestScopeMiddlewareForMatchesMultipleScopesOnlyOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	hits := 0
	router := gin.New()
	router.Use(scopeMiddlewareFor(
		[]string{"/api/v1/addons/example", "/api/v1/addons/example/admin"},
		func(c *gin.Context) {
			hits++
			c.Next()
		},
	))
	router.GET("/*path", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/addons/example/admin/items", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if hits != 1 {
		t.Fatalf("middleware hits = %d, want 1", hits)
	}
}

func TestValidateRouteScopeIsolationRejectsCrossPluginOverlap(t *testing.T) {
	tests := []struct {
		name    string
		scopes  map[string][]string
		wantErr bool
	}{
		{
			name: "broad scope reaches child plugin",
			scopes: map[string][]string{
				"gateway": {"/api/v1/addons"},
				"auth":    {"/api/v1/addons/auth"},
			},
			wantErr: true,
		},
		{
			name: "same scope reused by two plugins",
			scopes: map[string][]string{
				"first":  {"/api/v1/shared"},
				"second": {"/api/v1/shared"},
			},
			wantErr: true,
		},
		{
			name: "sibling segment prefixes do not overlap",
			scopes: map[string][]string{
				"auth":  {"/api/v1/addons/auth"},
				"authz": {"/api/v1/addons/authz"},
			},
		},
		{
			name: "nested scopes owned by one plugin are allowed",
			scopes: map[string][]string{
				"auth": {"/api/v1/addons/auth", "/api/v1/addons/auth/admin"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRouteScopeIsolation(tt.scopes)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateRouteScopeIsolation() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
