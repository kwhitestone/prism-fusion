package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humagin"
	"github.com/gin-gonic/gin"
	authModel "github.com/kwhitestone/prism-fusion/addons/auth/model"
	authService "github.com/kwhitestone/prism-fusion/addons/auth/service"
	"github.com/kwhitestone/prism-fusion/addons/rbac/model"
	rbacService "github.com/kwhitestone/prism-fusion/addons/rbac/service"
	"github.com/kwhitestone/prism-fusion/config"
	"github.com/kwhitestone/prism-fusion/global"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type testEnvelope struct {
	Code int             `json:"code"`
	Data json.RawMessage `json:"data"`
}

func TestManagementAPIEndToEnd(t *testing.T) {
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
	users := []authModel.User{
		{ID: 1, UUID: "actor-1", Username: "root", Enable: 1, RoleID: rbacService.SuperAdminRoleID},
		{ID: 2, UUID: "target-2", Username: "member", Enable: 1, RoleID: rbacService.DefaultUserRoleID},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]model.UserRole{
		{UserID: 1, RoleID: rbacService.SuperAdminRoleID},
		{UserID: 2, RoleID: rbacService.DefaultUserRoleID},
	}).Error; err != nil {
		t.Fatal(err)
	}
	token, err := (&authService.JwtService{}).GenerateSessionToken(1, "root", rbacService.SuperAdminRoleID, "router-test-family")
	if err != nil {
		t.Fatal(err)
	}

	engine := gin.New()
	api := humagin.New(engine, huma.DefaultConfig("RBAC test", "1.0.0"))
	RegisterRoutes(api)

	request := func(method, path string, body any) testEnvelope {
		t.Helper()
		var payload []byte
		if body != nil {
			payload, err = json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(payload))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Request-ID", "rbac-router-integration-request")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s %s: status=%d body=%s", method, path, recorder.Code, recorder.Body.String())
		}
		var envelope testEnvelope
		if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("%s %s decode: %v body=%s", method, path, err, recorder.Body.String())
		}
		if envelope.Code != 0 {
			t.Fatalf("%s %s code=%d", method, path, envelope.Code)
		}
		return envelope
	}

	request(http.MethodGet, "/api/v1/addons/rbac/me", nil)
	asyncRoutes := request(http.MethodGet, "/api/v1/addons/rbac/async-routes", nil)
	var routeList []rbacService.MenuNode
	if err := json.Unmarshal(asyncRoutes.Data, &routeList); err != nil {
		t.Fatalf("async routes must preserve the legacy array contract: %v data=%s", err, asyncRoutes.Data)
	}
	rolesResponse := request(http.MethodGet, "/api/v1/addons/rbac/roles", nil)
	var roleContract []map[string]any
	if err := json.Unmarshal(rolesResponse.Data, &roleContract); err != nil || len(roleContract) == 0 {
		t.Fatalf("decode roles contract: %#v err=%v", roleContract, err)
	}
	if roleContract[0]["name"] == nil || roleContract[0]["roleName"] == nil || roleContract[0]["name"] != roleContract[0]["roleName"] {
		t.Fatalf("roles response must expose compatible name fields: %#v", roleContract[0])
	}
	currentPermissions := request(http.MethodGet, "/api/v1/addons/rbac/permissions", nil)
	var permissionCodes []string
	if err := json.Unmarshal(currentPermissions.Data, &permissionCodes); err != nil {
		t.Fatalf("permissions must preserve the current-user string array contract: %v data=%s", err, currentPermissions.Data)
	}
	request(http.MethodGet, "/api/v1/addons/rbac/permission-catalog", nil)
	request(http.MethodGet, "/api/v1/addons/rbac/users?page=1&pageSize=10&search=mem", nil)

	created := request(http.MethodPost, "/api/v1/addons/rbac/roles", map[string]any{
		"code": "support_operator", "name": "Support operator", "description": "Support access",
	})
	var role model.Role
	if err := json.Unmarshal(created.Data, &role); err != nil || role.RoleID == 0 {
		t.Fatalf("decode created role: %#v err=%v", role, err)
	}
	rolePath := fmt.Sprintf("/api/v1/addons/rbac/roles/%d", role.RoleID)
	request(http.MethodPatch, rolePath, map[string]any{"name": "Support specialist", "isEnabled": true})
	request(http.MethodGet, rolePath+"/permissions", nil)

	var permission model.Permission
	if err := db.Where("code = ?", "auth:role:read").First(&permission).Error; err != nil {
		t.Fatal(err)
	}
	request(http.MethodPut, rolePath+"/permissions", map[string]any{"permissionIds": []uint{permission.ID}})
	request(http.MethodPut, "/api/v1/addons/rbac/users/2/roles", map[string]any{"roleIds": []uint{role.RoleID}})

	menuCreated := request(http.MethodPost, "/api/v1/addons/rbac/menus", map[string]any{
		"parentId": 0, "code": "support_queue", "title": "Support queue", "path": "/support",
		"icon": "", "app": "auth", "type": "menu", "permissionCode": "auth:role:read",
		"sort": 100, "isVisible": true,
	})
	var menu model.Menu
	if err := json.Unmarshal(menuCreated.Data, &menu); err != nil || menu.ID == 0 {
		t.Fatalf("decode created menu: %#v err=%v", menu, err)
	}
	request(http.MethodGet, "/api/v1/addons/rbac/menus", nil)
	menuPath := fmt.Sprintf("/api/v1/addons/rbac/menus/%d", menu.ID)
	request(http.MethodPatch, menuPath, map[string]any{
		"parentId": 0, "code": "support_queue", "title": "Support inbox", "path": "/support",
		"icon": "", "app": "auth", "type": "menu", "permissionCode": "auth:role:read",
		"sort": 101, "isVisible": true,
	})
	request(http.MethodDelete, menuPath, nil)
	request(http.MethodGet, "/api/v1/addons/rbac/audit?page=1&pageSize=100", nil)

	request(http.MethodPut, "/api/v1/addons/rbac/users/2/roles", map[string]any{"roleIds": []uint{rbacService.DefaultUserRoleID}})
	request(http.MethodDelete, rolePath, nil)
	recreated := request(http.MethodPost, "/api/v1/addons/rbac/roles", map[string]any{
		"code": "support_auditor", "name": "Support auditor", "description": "Read-only support access",
	})
	var recreatedRole model.Role
	if err := json.Unmarshal(recreated.Data, &recreatedRole); err != nil || recreatedRole.RoleID == role.RoleID {
		t.Fatalf("soft-deleted role ID must not be reused: role=%#v err=%v", recreatedRole, err)
	}
	var audited model.AuditLog
	if err := db.Where("action = ?", "role.permissions.update").First(&audited).Error; err != nil {
		t.Fatal(err)
	}
	if audited.RequestID != "rbac-router-integration-request" {
		t.Fatalf("mutation request id was not audited: %#v", audited)
	}
}

func TestRouterHelpersMapSecurityAndValidationErrors(t *testing.T) {
	if _, err := actorFromHeader("not-a-token"); err == nil {
		t.Fatal("invalid token should be rejected")
	}
	body := MenuBody{ParentID: 3, Code: "menu", Title: "Menu", Path: "/menu", App: "auth", Type: "menu", Sort: 2, IsVisible: true}
	if converted := toMenuInput(body); converted.ParentID != body.ParentID || converted.Code != body.Code {
		t.Fatalf("menu conversion mismatch: %#v", converted)
	}
	if empty("done").Body.Message != "done" {
		t.Fatal("empty response message mismatch")
	}
	for _, err := range []error{
		rbacService.ErrAccessDenied, rbacService.ErrInvalidInput, rbacService.ErrUnknownRole,
		rbacService.ErrProtectedRole, rbacService.ErrRoleInUse, gorm.ErrRecordNotFound,
	} {
		if mapError(err) == nil {
			t.Fatalf("error was not mapped: %v", err)
		}
	}
}

func TestActorFromHeaderRejectsServicePrincipal(t *testing.T) {
	previous := global.PRISM_CONFIG
	global.PRISM_CONFIG.JWT = config.JWT{SigningKey: "test-rbac-router-signing-key-with-entropy", ExpiresTime: "1h"}
	t.Cleanup(func() { global.PRISM_CONFIG = previous })
	token, err := (&authService.JwtService{}).GenerateToken(1, "core", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := actorFromHeader("Bearer " + token); err == nil {
		t.Fatal("service principal was accepted as an RBAC management actor")
	}
}
