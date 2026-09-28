package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humagin"
	"github.com/gin-gonic/gin"
	authMiddleware "github.com/kwhitestone/prism-fusion/addons/auth/middleware"
	"github.com/kwhitestone/prism-fusion/addons/auth/model"
	"github.com/kwhitestone/prism-fusion/config"
	"github.com/kwhitestone/prism-fusion/global"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type authTestEnvelope struct {
	Code    int       `json:"code"`
	Message string    `json:"message"`
	Data    LoginData `json:"data"`
}

func TestAuthAPIEndToEnd(t *testing.T) {
	for _, window := range []struct {
		name, refresh, family string
	}{
		{"finite", "24h", "48h"},
		{"aligned", "87600h", "87600h"},
	} {
		t.Run(window.name, func(t *testing.T) {
			testAuthAPIEndToEnd(t, window.refresh, window.family)
		})
	}
}

type authRequest func(method, path string, body any, accessToken string) *httptest.ResponseRecorder

func testAuthAPIEndToEnd(t *testing.T, refreshTTL, familyTTL string) {
	t.Setenv("AUTH_ALLOW_LOCAL_REGISTRATION", "true")
	request, db := newAuthTestServer(t, refreshTTL, familyTTL)
	user := model.User{UUID: "auth-user-1", Username: "auth-user", NickName: "Auth User", RoleID: 1, Enable: 1}
	if err := user.SetPassword("correct-password"); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	registered := request(http.MethodPost, "/api/v1/addons/auth/register", map[string]string{
		"username": "registered-user", "password": "registered-password", "nickName": "Registered User",
	}, "")
	if registered.Code != http.StatusOK {
		t.Fatalf("register status=%d body=%s", registered.Code, registered.Body.String())
	}
	duplicate := request(http.MethodPost, "/api/v1/addons/auth/register", map[string]string{
		"username": "registered-user", "password": "registered-password", "nickName": "Registered User",
	}, "")
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate register status=%d body=%s", duplicate.Code, duplicate.Body.String())
	}
	wrongPassword := request(http.MethodPost, "/api/v1/addons/auth/login", map[string]string{
		"username": user.Username, "password": "wrong-password",
	}, "")
	if wrongPassword.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password status=%d body=%s", wrongPassword.Code, wrongPassword.Body.String())
	}

	login := request(http.MethodPost, "/api/v1/addons/auth/login", map[string]string{
		"username": user.Username, "password": "correct-password",
	}, "")
	if login.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", login.Code, login.Body.String())
	}
	var loginEnvelope authTestEnvelope
	if err := json.Unmarshal(login.Body.Bytes(), &loginEnvelope); err != nil {
		t.Fatal(err)
	}
	if loginEnvelope.Code != 0 || loginEnvelope.Data.AccessToken == "" || loginEnvelope.Data.RefreshToken == "" ||
		loginEnvelope.Data.User == nil || loginEnvelope.Data.User.Username != user.Username {
		t.Fatalf("login contract=%#v", loginEnvelope)
	}

	userInfo := request(http.MethodGet, "/api/v1/addons/auth/user-info", nil, loginEnvelope.Data.AccessToken)
	if userInfo.Code != http.StatusOK {
		t.Fatalf("user info status=%d body=%s", userInfo.Code, userInfo.Body.String())
	}
	invalidRefresh := request(http.MethodPost, "/api/v1/addons/auth/refresh-token", map[string]string{
		"refreshToken": "invalid-refresh-token",
	}, "")
	if invalidRefresh.Code != http.StatusUnauthorized {
		t.Fatalf("invalid refresh status=%d body=%s", invalidRefresh.Code, invalidRefresh.Body.String())
	}
	refreshed := request(http.MethodPost, "/api/v1/addons/auth/refresh-token", map[string]string{
		"refreshToken": loginEnvelope.Data.RefreshToken,
	}, "")
	if refreshed.Code != http.StatusOK {
		t.Fatalf("refresh status=%d body=%s", refreshed.Code, refreshed.Body.String())
	}
	var refreshEnvelope authTestEnvelope
	if err := json.Unmarshal(refreshed.Body.Bytes(), &refreshEnvelope); err != nil {
		t.Fatal(err)
	}
	if refreshEnvelope.Data.AccessToken == "" || refreshEnvelope.Data.RefreshToken == "" {
		t.Fatalf("refresh contract=%#v", refreshEnvelope)
	}
	logout := request(http.MethodPost, "/api/v1/addons/auth/logout", map[string]string{
		"refreshToken": refreshEnvelope.Data.RefreshToken,
	}, "")
	if logout.Code != http.StatusOK {
		t.Fatalf("logout status=%d body=%s", logout.Code, logout.Body.String())
	}
	afterLogout := request(http.MethodGet, "/api/v1/addons/auth/user-info", nil, refreshEnvelope.Data.AccessToken)
	if afterLogout.Code != http.StatusUnauthorized {
		t.Fatalf("revoked session status=%d body=%s", afterLogout.Code, afterLogout.Body.String())
	}
	afterLogoutRefresh := request(http.MethodPost, "/api/v1/addons/auth/refresh-token", map[string]string{
		"refreshToken": refreshEnvelope.Data.RefreshToken,
	}, "")
	if afterLogoutRefresh.Code != http.StatusUnauthorized {
		t.Fatalf("revoked refresh status=%d", afterLogoutRefresh.Code)
	}
}

// newAuthTestServer builds the router under the current environment, so
// callers set AUTH_ALLOW_LOCAL_REGISTRATION before calling it.
func newAuthTestServer(t *testing.T, refreshTTL, familyTTL string) (authRequest, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "auth-router.db")), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.RefreshSession{}, &model.AuthRateWindow{}); err != nil {
		t.Fatal(err)
	}
	previousDB, previousConfig, previousLog := global.PRISM_DB, global.PRISM_CONFIG, global.PRISM_LOG
	global.PRISM_DB = db
	global.PRISM_LOG = zap.NewNop()
	global.PRISM_CONFIG.JWT = config.JWT{
		SigningKey:               "test-auth-router-signing-key-with-entropy",
		ExpiresTime:              "15m",
		RefreshExpiresTime:       refreshTTL,
		RefreshFamilyExpiresTime: familyTTL,
		RefreshRotationGrace:     "60s",
		Issuer:                   "auth-router-test",
	}
	global.PRISM_CONFIG.RBAC = config.RBAC{Provider: "disabled"}
	t.Cleanup(func() {
		global.PRISM_DB = previousDB
		global.PRISM_CONFIG = previousConfig
		global.PRISM_LOG = previousLog
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
	})

	engine := gin.New()
	engine.Use(authMiddleware.JwtAuthMiddleware())
	api := humagin.New(engine, huma.DefaultConfig("Auth test", "1.0.0"))
	RegisterRoutes(api)
	request := func(method, path string, body any, accessToken string) *httptest.ResponseRecorder {
		t.Helper()
		var payload []byte
		if body != nil {
			var err error
			payload, err = json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(payload))
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if accessToken != "" {
			req.Header.Set("Authorization", "Bearer "+accessToken)
		}
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, req)
		return recorder
	}
	return request, db
}

func TestLocalRegistrationFlag(t *testing.T) {
	for _, tc := range []struct {
		name    string
		value   *string
		enabled bool
	}{
		{"absent", nil, false},
		{"empty", ptr(""), false},
		{"TRUE", ptr("TRUE"), false},
		{"1", ptr("1"), false},
		{"true", ptr("true"), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.value == nil {
				t.Setenv("AUTH_ALLOW_LOCAL_REGISTRATION", "")
				os.Unsetenv("AUTH_ALLOW_LOCAL_REGISTRATION")
			} else {
				t.Setenv("AUTH_ALLOW_LOCAL_REGISTRATION", *tc.value)
			}
			request, db := newAuthTestServer(t, "24h", "48h")

			// Logged out, no token: the login page's probe.
			status := request(http.MethodGet, "/api/v1/addons/auth/registration-status", nil, "")
			if status.Code != http.StatusOK {
				t.Fatalf("status code=%d body=%s", status.Code, status.Body.String())
			}
			var envelope struct {
				Code int `json:"code"`
				Data struct {
					Enabled *bool `json:"enabled"`
				} `json:"data"`
			}
			if err := json.Unmarshal(status.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Code != 0 || envelope.Data.Enabled == nil || *envelope.Data.Enabled != tc.enabled {
				t.Fatalf("status body=%s, want enabled=%v", status.Body.String(), tc.enabled)
			}

			valid := map[string]string{"username": "flag-user", "password": "flag-password", "nickName": ""}
			registered := request(http.MethodPost, "/api/v1/addons/auth/register", valid, "")
			var users int64
			db.Model(&model.User{}).Where("username = ?", "flag-user").Count(&users)
			if !tc.enabled {
				// 404 regardless of body validity, and nothing is written.
				invalid := request(http.MethodPost, "/api/v1/addons/auth/register", map[string]string{}, "")
				if registered.Code != http.StatusNotFound || invalid.Code != http.StatusNotFound || users != 0 {
					t.Fatalf("flag off: valid=%d invalid=%d users=%d", registered.Code, invalid.Code, users)
				}
				return
			}
			if registered.Code != http.StatusOK || users != 1 {
				t.Fatalf("flag on: register=%d body=%s users=%d", registered.Code, registered.Body.String(), users)
			}
		})
	}
}

func ptr(s string) *string { return &s }
