package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humagin"
	"github.com/gin-gonic/gin"
	authModel "github.com/kwhitestone/prism-fusion/addons/auth/model"
	authService "github.com/kwhitestone/prism-fusion/addons/auth/service"
	rbacMiddleware "github.com/kwhitestone/prism-fusion/addons/rbac/middleware"
	"github.com/kwhitestone/prism-fusion/addons/rbac/model"
	rbacService "github.com/kwhitestone/prism-fusion/addons/rbac/service"
	"github.com/kwhitestone/prism-fusion/config"
	"github.com/kwhitestone/prism-fusion/global"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestUserRoleFilterHTTPContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&authModel.User{}, &authModel.RefreshSession{},
		&model.Role{}, &model.Permission{}, &model.RolePermission{},
		&model.UserRole{}, &model.Menu{}, &model.AuditLog{},
	); err != nil {
		t.Fatal(err)
	}
	previousDB, previousConfig := global.PRISM_DB, global.PRISM_CONFIG
	global.PRISM_DB = db
	global.PRISM_CONFIG.JWT = config.JWT{SigningKey: "test-rbac-router-signing-key-with-entropy", ExpiresTime: "1h"}
	t.Cleanup(func() {
		global.PRISM_DB = previousDB
		global.PRISM_CONFIG = previousConfig
	})
	if err := rbacService.SeedRegisteredData(); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]authModel.User{
		{ID: 1, UUID: "u1", Username: "root", Enable: 1, RoleID: rbacService.SuperAdminRoleID},
		{ID: 2, UUID: "u2", Username: "member", Enable: 1, RoleID: rbacService.DefaultUserRoleID},
		{ID: 3, UUID: "u3", Username: "legacy-admin", Enable: 1, RoleID: rbacService.AdminRoleID},
	}).Error; err != nil {
		t.Fatal(err)
	}
	// User 3 has no user_roles row: it must be counted and filtered through the
	// users.role_id fallback. Seeding backfills rows, so remove it afterwards.
	if err := db.Where("user_id = ?", 3).Delete(&model.UserRole{}).Error; err != nil {
		t.Fatal(err)
	}

	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		claims, err := (&authService.JwtService{}).ParseAccessToken(c.GetHeader("Authorization")[len("Bearer "):])
		if err == nil {
			c.Set("user_id", claims.UserID)
		}
		c.Next()
	})
	engine.Use(rbacMiddleware.AuthorizeManagementRoutes())
	api := humagin.New(engine, huma.DefaultConfig("RBAC test", "1.0.0"))
	RegisterRoutes(api)

	call := func(userID uint, path string) *httptest.ResponseRecorder {
		t.Helper()
		token, err := (&authService.JwtService{}).GenerateSessionToken(userID, "u", 0, "role-filter-family")
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, req)
		return recorder
	}

	ok := call(1, "/api/v1/addons/rbac/users?roles=admin,super_admin&pageSize=100")
	if ok.Code != http.StatusOK {
		t.Fatalf("filter status=%d body=%s", ok.Code, ok.Body.String())
	}
	var users struct {
		Data rbacService.UserPage `json:"data"`
	}
	if err := json.Unmarshal(ok.Body.Bytes(), &users); err != nil {
		t.Fatal(err)
	}
	if users.Data.Total != 2 || len(users.Data.Items) != 2 || users.Data.Items[0].ID != 1 || users.Data.Items[1].ID != 3 {
		t.Fatalf("roles=admin,super_admin returned %#v", users.Data)
	}

	if bad := call(1, "/api/v1/addons/rbac/users?roles=ghost"); bad.Code != http.StatusBadRequest {
		t.Fatalf("unknown role code status=%d body=%s", bad.Code, bad.Body.String())
	}

	rolesResponse := call(1, "/api/v1/addons/rbac/roles")
	var roles struct {
		Data []RoleView `json:"data"`
	}
	if err := json.Unmarshal(rolesResponse.Body.Bytes(), &roles); err != nil {
		t.Fatal(err)
	}
	counts := map[string]int64{}
	for _, role := range roles.Data {
		counts[role.Code] = role.UserCount
	}
	if counts["user"] != 1 || counts["admin"] != 1 || counts["super_admin"] != 1 {
		t.Fatalf("userCount mismatch: %v", counts)
	}
	if !json.Valid(rolesResponse.Body.Bytes()) || !containsKey(rolesResponse.Body.Bytes(), "userCount") {
		t.Fatalf("roles payload must expose userCount: %s", rolesResponse.Body.String())
	}

	// A7: a plain user lacks auth:user:read, so the filter endpoint is denied.
	if denied := call(2, "/api/v1/addons/rbac/users?roles=admin"); denied.Code != http.StatusForbidden {
		t.Fatalf("user without auth:user:read status=%d body=%s", denied.Code, denied.Body.String())
	}
}

func containsKey(body []byte, key string) bool {
	var envelope struct {
		Data []map[string]any `json:"data"`
	}
	if json.Unmarshal(body, &envelope) != nil || len(envelope.Data) == 0 {
		return false
	}
	_, ok := envelope.Data[0][key]
	return ok
}
