package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humagin"
	"github.com/gin-gonic/gin"
	"github.com/kwhitestone/prism-fusion/addons/auth/model"
	"github.com/kwhitestone/prism-fusion/addons/auth/router"
	"github.com/kwhitestone/prism-fusion/addons/auth/service"
	"github.com/kwhitestone/prism-fusion/config"
	"github.com/kwhitestone/prism-fusion/global"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAuthPluginDeclaresOnlyItsOwnedEndpointScopes(t *testing.T) {
	manifest := (&AuthPlugin{}).Manifest()
	want := []string{
		"/api/v1/addons/auth/login",
		"/api/v1/addons/auth/logout",
		"/api/v1/addons/auth/refresh-token",
		"/api/v1/addons/auth/register",
		"/api/v1/addons/auth/user-info",
	}
	if !reflect.DeepEqual(manifest.RouteScopes, want) {
		t.Fatalf("RouteScopes = %v, want %v", manifest.RouteScopes, want)
	}
}

func TestServeRoutesPreservesRefreshRotation(t *testing.T) {
	enabled, disabled := true, false
	for _, tc := range []struct {
		name string
		flag *bool
	}{
		{"disabled", &disabled},
		{"enabled", &enabled},
		{"default", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "auth.db")), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			if err := db.AutoMigrate(&model.User{}, &model.RefreshSession{}, &model.AuthRateWindow{}); err != nil {
				t.Fatal(err)
			}
			previousDB, previousConfig, previousLog := global.PRISM_DB, global.PRISM_CONFIG, global.PRISM_LOG
			t.Cleanup(func() {
				global.PRISM_DB, global.PRISM_CONFIG, global.PRISM_LOG = previousDB, previousConfig, previousLog
				sqlDB, _ := db.DB()
				_ = sqlDB.Close()
			})
			global.PRISM_DB, global.PRISM_LOG = db, zap.NewNop()
			global.PRISM_CONFIG.Auth = config.Auth{Provider: "builtin", ServeRoutes: tc.flag}
			global.PRISM_CONFIG.RBAC = config.RBAC{Provider: "disabled"}
			global.PRISM_CONFIG.JWT = config.JWT{
				SigningKey: "test-serve-routes-signing-key-with-entropy", Issuer: "serve-routes-test",
				ExpiresTime: "15m", RefreshExpiresTime: "24h",
				RefreshFamilyExpiresTime: "48h", RefreshRotationGrace: "60s",
			}
			t.Setenv("AUTH_BOOTSTRAP_ADMIN_USERNAME", "")
			t.Setenv("AUTH_BOOTSTRAP_ADMIN_PASSWORD", "")
			user := model.User{UUID: "refresh-user", Username: "refresh-user", RoleID: 1, Enable: 1}
			if err := db.Create(&user).Error; err != nil {
				t.Fatal(err)
			}
			sessions := &service.RefreshSessionService{}
			raw, family, _, err := sessions.IssueWithFamily(user.ID)
			if err != nil {
				t.Fatal(err)
			}
			p := &AuthPlugin{}
			engine := gin.New()
			engine.Use(p.GlobalMiddlewares()...)
			api := humagin.New(engine, huma.DefaultConfig("Serve routes test", "1.0.0"))
			p.RegisterRoutes(api)
			wantRoutes := 5
			if tc.flag != nil && !*tc.flag {
				wantRoutes = 1
			}
			if got := len(api.OpenAPI().Paths); got != wantRoutes {
				t.Errorf("auth routes=%d, want %d", got, wantRoutes)
			}
			// Portal endpoints can be registered by an independent addon.
			for _, path := range []string{"portal/start", "portal/login"} {
				engine.POST("/api/v1/addons/auth/"+path, func(c *gin.Context) { c.Status(http.StatusOK) })
			}
			request := func(path, cookie, requestID string) *httptest.ResponseRecorder {
				t.Helper()
				req := httptest.NewRequest(http.MethodPost, "/api/v1/addons/auth/"+path, strings.NewReader("{}"))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Refresh-Cookie-Only", "1")
				req.Header.Set("X-Refresh-Request-ID", requestID)
				if cookie != "" {
					req.AddCookie(&http.Cookie{Name: "nucleagent_refresh", Value: cookie})
				}
				rec := httptest.NewRecorder()
				engine.ServeHTTP(rec, req)
				return rec
			}
			refreshed := request("refresh-token", raw, "serve-routes-rotation-1")
			if refreshed.Code != http.StatusOK {
				t.Fatalf("refresh status=%d body=%s", refreshed.Code, refreshed.Body.String())
			}
			var envelope struct {
				Data router.LoginData `json:"data"`
			}
			if err := json.Unmarshal(refreshed.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			claims, err := (&service.JwtService{}).ParseAccessToken(envelope.Data.AccessToken)
			if err != nil || claims.UserID != user.ID || claims.SessionID != family {
				t.Fatalf("refresh access token did not preserve user/family: %v", err)
			}
			cookies := refreshed.Result().Cookies()
			if len(cookies) != 1 || cookies[0].Name != "nucleagent_refresh" ||
				!cookies[0].HttpOnly || cookies[0].Value == "" || cookies[0].Value == raw ||
				envelope.Data.RefreshToken != "" {
				t.Fatal("refresh must rotate the HttpOnly cookie without exposing it in JSON")
			}
			var live, revoked int64
			if err := db.Model(&model.RefreshSession{}).Where("family_id = ? AND revoked_at IS NULL", family).Count(&live).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Model(&model.RefreshSession{}).Where("family_id = ? AND revoked_at IS NOT NULL", family).Count(&revoked).Error; err != nil {
				t.Fatal(err)
			}
			if live != 1 || revoked != 1 {
				t.Fatalf("rotation rows: live=%d revoked=%d", live, revoked)
			}
			if got := request("refresh-token", "", "").Code; got != http.StatusBadRequest {
				t.Errorf("no-cookie route probe=%d, want 400", got)
			}
			if got := request("refresh-token", "", "serve-routes-missing-cookie").Code; got != http.StatusUnauthorized {
				t.Errorf("missing cookie=%d, want 401", got)
			}
			for _, path := range []string{"login", "register", "logout", "portal/start", "portal/login"} {
				got := request(path, "", "").Code
				if tc.flag != nil && !*tc.flag {
					if got != http.StatusNotFound {
						t.Errorf("%s status=%d, want 404", path, got)
					}
				} else if got == http.StatusNotFound || got == http.StatusMethodNotAllowed {
					t.Errorf("%s unexpectedly unavailable: %d", path, got)
				}
			}
		})
	}
}

func TestShouldServeRoutesCanDisableResourceServiceAuthEndpoints(t *testing.T) {
	previous := global.PRISM_CONFIG
	t.Cleanup(func() { global.PRISM_CONFIG = previous })

	disabled := false
	global.PRISM_CONFIG.Auth = config.Auth{Provider: "builtin", ServeRoutes: &disabled}
	if shouldServeRoutes() {
		t.Fatal("resource services must be able to keep JWT middleware without serving login routes")
	}

	enabled := true
	global.PRISM_CONFIG.Auth.ServeRoutes = &enabled
	if !shouldServeRoutes() {
		t.Fatal("auth service must be able to expose its routes explicitly")
	}
}
