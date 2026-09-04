package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
		RefreshExpiresTime:       "24h",
		RefreshFamilyExpiresTime: "48h",
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

	user := model.User{UUID: "auth-user-1", Username: "auth-user", NickName: "Auth User", RoleID: 1, Enable: 1}
	if err := user.SetPassword("correct-password"); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	engine := gin.New()
	engine.Use(authMiddleware.JwtAuthMiddleware())
	api := humagin.New(engine, huma.DefaultConfig("Auth test", "1.0.0"))
	RegisterRoutes(api)
	request := func(method, path string, body any, accessToken string) *httptest.ResponseRecorder {
		t.Helper()
		var payload []byte
		if body != nil {
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
}
