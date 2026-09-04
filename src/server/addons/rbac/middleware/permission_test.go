package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequirePermissionDefaultsToDeny(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name        string
		permissions any
		status      int
	}{
		{name: "missing context", status: http.StatusServiceUnavailable},
		{name: "wrong permission", permissions: []string{"auth:role:read"}, status: http.StatusForbidden},
		{name: "exact permission", permissions: []string{"auth:role:write"}, status: http.StatusNoContent},
		{name: "wildcard permission", permissions: []string{"*:*:*"}, status: http.StatusNoContent},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handled := false
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set("user_id", uint(5))
				if tc.permissions != nil {
					c.Set("permissions", tc.permissions)
				}
				c.Next()
			})
			router.GET("/protected", RequirePermission("auth:role:write"), func(c *gin.Context) {
				handled = true
				c.Status(http.StatusNoContent)
			})
			request := httptest.NewRequest(http.MethodGet, "/protected", nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tc.status {
				t.Fatalf("status = %d, want %d", response.Code, tc.status)
			}
			if handled != (tc.status == http.StatusNoContent) {
				t.Fatalf("handler execution = %v", handled)
			}
		})
	}
}

func TestRequirePermissionRejectsUnauthenticatedRequest(t *testing.T) {
	router := gin.New()
	router.GET("/protected", RequirePermission("auth:role:read"), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/protected", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want=%d", response.Code, http.StatusUnauthorized)
	}
}

func TestAuthorizeManagementRoutesAppliesResolvedCapability(t *testing.T) {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", uint(5))
		c.Set("permissions", []string{"auth:menu:read"})
		c.Next()
	})
	router.Use(AuthorizeManagementRoutes())
	router.GET("/api/v1/addons/rbac/menus", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	router.POST("/api/v1/addons/rbac/menus", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	for _, tc := range []struct {
		method string
		want   int
	}{{http.MethodGet, http.StatusNoContent}, {http.MethodPost, http.StatusForbidden}} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(tc.method, "/api/v1/addons/rbac/menus", nil))
		if response.Code != tc.want {
			t.Fatalf("%s status=%d want=%d", tc.method, response.Code, tc.want)
		}
	}
}

func TestAuthorizeUserRoleMutationAcceptsThreeSegmentWildcard(t *testing.T) {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", uint(1))
		c.Set("permissions", []string{"*:*:*"})
		c.Next()
	})
	router.Use(AuthorizeManagementRoutes())
	router.PUT("/api/v1/addons/rbac/users/:id/roles", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPut, "/api/v1/addons/rbac/users/8/roles", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestManagementPermissionNeverTrustsQueryRoleID(t *testing.T) {
	cases := []struct {
		method string
		path   string
		want   string
	}{
		{http.MethodGet, "/api/v1/addons/rbac/me", ""},
		{http.MethodGet, "/api/v1/addons/rbac/async-routes/", ""},
		{http.MethodGet, "/api/v1/addons/rbac/roles", "auth:role:read"},
		{http.MethodPost, "/api/v1/addons/rbac/roles", "auth:role:write"},
		{http.MethodGet, "/api/v1/addons/rbac/permissions?roleId=999", ""},
		{http.MethodGet, "/api/v1/addons/rbac/permission-catalog", "auth:permission:read"},
		{http.MethodPut, "/api/v1/addons/rbac/roles/999/permissions", "auth:role:write"},
		{http.MethodGet, "/api/v1/addons/rbac/menus", "auth:menu:read"},
		{http.MethodDelete, "/api/v1/addons/rbac/menus/3", "auth:menu:write"},
		{http.MethodGet, "/api/v1/addons/rbac/users", "auth:user:read"},
		{http.MethodPut, "/api/v1/addons/rbac/users/7/roles", "auth:user-role:write"},
		{http.MethodGet, "/api/v1/addons/rbac/audit", "auth:audit:read"},
		{http.MethodGet, "/api/v1/addons/rbac/unrecognized", "auth:super-admin:grant"},
	}
	for _, tc := range cases {
		if got := ManagementPermission(tc.method, tc.path); got != tc.want {
			t.Errorf("ManagementPermission(%q, %q) = %q, want %q", tc.method, tc.path, got, tc.want)
		}
	}
}
