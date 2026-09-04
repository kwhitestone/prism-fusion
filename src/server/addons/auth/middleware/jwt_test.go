package authMiddleware

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	authModel "github.com/kwhitestone/prism-fusion/addons/auth/model"
	"github.com/kwhitestone/prism-fusion/addons/auth/service"
	"github.com/kwhitestone/prism-fusion/config"
	"github.com/kwhitestone/prism-fusion/global"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupJWTMiddlewareTest(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "auth.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&authModel.User{}, &authModel.RefreshSession{}, &authModel.AuthRateWindow{}); err != nil {
		t.Fatal(err)
	}
	previousConfig, previousDB, previousLog := global.PRISM_CONFIG, global.PRISM_DB, global.PRISM_LOG
	previousRateLimits := authRateLimits
	publicPathsMu.Lock()
	previousPublicPaths := append([]string(nil), publicPaths...)
	publicPathsMu.Unlock()
	global.PRISM_CONFIG.JWT = config.JWT{
		SigningKey:               "test-signing-key-with-sufficient-entropy",
		ExpiresTime:              "15m",
		RefreshExpiresTime:       "24h",
		RefreshFamilyExpiresTime: "48h",
		Issuer:                   "test-auth",
	}
	global.PRISM_CONFIG.RBAC = config.RBAC{Provider: "disabled"}
	global.PRISM_DB = db
	global.PRISM_LOG = zap.NewNop()
	authRateLimits = &service.AuthRateLimitService{DB: db}
	t.Cleanup(func() {
		global.PRISM_CONFIG = previousConfig
		global.PRISM_DB = previousDB
		global.PRISM_LOG = previousLog
		authRateLimits = previousRateLimits
		publicPathsMu.Lock()
		publicPaths = previousPublicPaths
		publicPathsMu.Unlock()
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func serveWithJWTMiddleware(method, path, token string, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	engine := gin.New()
	engine.Use(JwtAuthMiddleware())
	engine.Handle(method, path, handler)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	engine.ServeHTTP(response, request)
	return response
}

func TestServiceTokensCannotEnterHumanIdentityOrAuthorizationAPIs(t *testing.T) {
	for _, path := range []string{
		"/api/v1/addons/auth/user-info",
		"/api/v1/addons/auth/api-keys",
		"/api/v1/addons/rbac/me",
		"/api/v1/addons/rbac/permissions",
	} {
		if serviceTokenPathAllowed(path) {
			t.Fatalf("service token unexpectedly allowed on %s", path)
		}
	}
	for _, path := range []string{
		"/api/v1/addons/conversation/7",
		"/api/v1/files/7",
	} {
		if !serviceTokenPathAllowed(path) {
			t.Fatalf("delegated service route unexpectedly denied on %s", path)
		}
	}
}

func TestPublicPathMatchingUsesSegmentBoundaries(t *testing.T) {
	if publicPathMatches("/api/v1/addons/auth/login-extra", "/api/v1/addons/auth/login") {
		t.Fatal("login-extra must not inherit the login endpoint's public access")
	}
	if !publicPathMatches("/api/v1/addons/s2s/executor/register", "/api/v1/addons/s2s") {
		t.Fatal("nested S2S route must remain public to its own authentication middleware")
	}
}

func TestLegacyUntypedRoleZeroTokenIsNotAcceptedAsServicePrincipal(t *testing.T) {
	previousConfig, previousLog := global.PRISM_CONFIG, global.PRISM_LOG
	global.PRISM_CONFIG.JWT = config.JWT{
		SigningKey: "test-signing-key-with-sufficient-entropy",
		Issuer:     "test-auth",
	}
	global.PRISM_LOG = zap.NewNop()
	t.Cleanup(func() {
		global.PRISM_CONFIG = previousConfig
		global.PRISM_LOG = previousLog
	})

	claims := service.Claims{
		UserID:   42,
		Username: "legacy-role-zero-user",
		RoleID:   0,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "test-auth",
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).
		SignedString([]byte(global.PRISM_CONFIG.JWT.SigningKey))
	if err != nil {
		t.Fatal(err)
	}

	engine := gin.New()
	engine.Use(JwtAuthMiddleware())
	engine.GET("/api/v1/addons/conversation/7", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/addons/conversation/7", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	engine.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("legacy untyped role-zero token status=%d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestJWTMiddlewareEnforcesSessionAndPrincipalBoundaries(t *testing.T) {
	db := setupJWTMiddlewareTest(t)
	user := authModel.User{ID: 42, UUID: "user-42", Username: "human", RoleID: 10, Enable: 1}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	refreshService := &service.RefreshSessionService{}
	_, familyID, _, err := refreshService.IssueWithFamily(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	jwtService := &service.JwtService{}
	humanToken, err := jwtService.GenerateSessionToken(user.ID, user.Username, user.RoleID, familyID)
	if err != nil {
		t.Fatal(err)
	}
	inactiveToken, err := jwtService.GenerateSessionToken(user.ID, user.Username, user.RoleID, "missing-family")
	if err != nil {
		t.Fatal(err)
	}
	legacyHumanToken, err := jwtService.GenerateToken(user.ID, user.Username, user.RoleID)
	if err != nil {
		t.Fatal(err)
	}
	serviceToken, err := jwtService.GenerateToken(7, "executor", 0)
	if err != nil {
		t.Fatal(err)
	}
	noContent := func(c *gin.Context) { c.Status(http.StatusNoContent) }

	if got := serveWithJWTMiddleware(http.MethodGet, "/index.html", "", noContent).Code; got != http.StatusNoContent {
		t.Fatalf("static route status=%d", got)
	}
	if got := serveWithJWTMiddleware(http.MethodGet, "/api/v1/private", "", noContent).Code; got != http.StatusUnauthorized {
		t.Fatalf("missing token status=%d", got)
	}
	if got := serveWithJWTMiddleware(http.MethodGet, "/api/v1/private", "invalid", noContent).Code; got != http.StatusUnauthorized {
		t.Fatalf("invalid token status=%d", got)
	}
	if got := serveWithJWTMiddleware(http.MethodGet, "/api/v1/private", legacyHumanToken, noContent).Code; got != http.StatusUnauthorized {
		t.Fatalf("legacy human token status=%d", got)
	}
	if got := serveWithJWTMiddleware(http.MethodGet, "/api/v1/private", inactiveToken, noContent).Code; got != http.StatusUnauthorized {
		t.Fatalf("inactive session status=%d", got)
	}
	if got := serveWithJWTMiddleware(http.MethodGet, "/api/v1/addons/rbac/me", serviceToken, noContent).Code; got != http.StatusUnauthorized {
		t.Fatalf("service token on RBAC status=%d", got)
	}
	if got := serveWithJWTMiddleware(http.MethodGet, "/api/v1/private", serviceToken, noContent).Code; got != http.StatusNoContent {
		t.Fatalf("service token status=%d", got)
	}
	humanResponse := serveWithJWTMiddleware(http.MethodGet, "/api/v1/private", humanToken, func(c *gin.Context) {
		if c.GetUint("user_id") != user.ID || c.GetString("username") != user.Username ||
			c.GetUint("role_id") != user.RoleID || c.GetString("session_id") != familyID {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		roles, _ := c.Get("roles")
		if got, ok := roles.([]string); !ok || len(got) != 1 || got[0] != "user" {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusNoContent)
	})
	if humanResponse.Code != http.StatusNoContent {
		t.Fatalf("active human session status=%d body=%s", humanResponse.Code, humanResponse.Body.String())
	}

	AddPublicPath("/api/v1/public-addon")
	if got := serveWithJWTMiddleware(http.MethodGet, "/api/v1/public-addon/status", "", noContent).Code; got != http.StatusNoContent {
		t.Fatalf("contributed public path status=%d", got)
	}
	if got := serveWithJWTMiddleware(http.MethodOptions, "/api/v1/addons/auth/login", "", noContent).Code; got != http.StatusNoContent {
		t.Fatalf("auth preflight status=%d", got)
	}
}

func TestJWTMiddlewareRateLimitsPublicAuthMutations(t *testing.T) {
	setupJWTMiddlewareTest(t)
	noContent := func(c *gin.Context) { c.Status(http.StatusNoContent) }
	for attempt := 1; attempt <= 31; attempt++ {
		response := serveWithJWTMiddleware(http.MethodPost, "/api/v1/addons/auth/login", "", noContent)
		if attempt <= 30 && response.Code != http.StatusNoContent {
			t.Fatalf("allowed attempt %d status=%d", attempt, response.Code)
		}
		if attempt == 31 {
			if response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") != "60" {
				t.Fatalf("limited attempt status=%d retry-after=%q", response.Code, response.Header().Get("Retry-After"))
			}
		}
	}

	global.PRISM_DB = nil
	authRateLimits = &service.AuthRateLimitService{}
	if got := serveWithJWTMiddleware(http.MethodPost, "/api/v1/addons/auth/register", "", noContent).Code; got != http.StatusServiceUnavailable {
		t.Fatalf("rate-limit storage failure status=%d", got)
	}
}
