package rbac

import (
	"context"
	"errors"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humagin"
	"github.com/gin-gonic/gin"
	authAddon "github.com/kwhitestone/prism-fusion/addons/auth"
	authModel "github.com/kwhitestone/prism-fusion/addons/auth/model"
	rbacModel "github.com/kwhitestone/prism-fusion/addons/rbac/model"
	rbacService "github.com/kwhitestone/prism-fusion/addons/rbac/service"
	"github.com/kwhitestone/prism-fusion/config"
	"github.com/kwhitestone/prism-fusion/global"
	"github.com/kwhitestone/prism-fusion/plugin"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPluginCanBeSoleControlPlaneOrDisabledConsumer(t *testing.T) {
	previousConfig, previousDB, previousLog := global.PRISM_CONFIG, global.PRISM_DB, global.PRISM_LOG
	t.Cleanup(func() {
		global.PRISM_CONFIG = previousConfig
		global.PRISM_DB = previousDB
		global.PRISM_LOG = previousLog
	})
	plugin := &RbacPlugin{}
	if plugin.Priority() != 20 || plugin.RoutePrefix() != "/api/v1/addons/rbac" {
		t.Fatalf("unexpected plugin metadata: priority=%d prefix=%q", plugin.Priority(), plugin.RoutePrefix())
	}
	manifest := plugin.Manifest()
	if manifest.APIVersion != "prism-fusion/v2" || manifest.ID != "rbac" || len(manifest.Requires) != 1 || manifest.Requires[0].ID != "auth" {
		t.Fatalf("unexpected V2 manifest: %#v", manifest)
	}

	global.PRISM_CONFIG.RBAC = config.RBAC{Provider: "disabled"}
	if isEnabled() || plugin.Models() != nil || plugin.Middlewares() != nil {
		t.Fatal("disabled RBAC must not migrate, expose routes, or attach middleware")
	}
	engine := gin.New()
	disabledAPI := humagin.New(engine, huma.DefaultConfig("disabled", "1"))
	plugin.RegisterRoutes(disabledAPI)
	global.PRISM_CONFIG.RBAC = config.RBAC{Provider: "external"}
	if isEnabled() || plugin.Models() != nil || plugin.Middlewares() != nil {
		t.Fatal("external RBAC consumer must not own migrations or management routes")
	}

	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	global.PRISM_CONFIG.RBAC = config.RBAC{Provider: "builtin"}
	global.PRISM_DB = db
	global.PRISM_LOG = zap.NewNop()
	models := plugin.Models()
	if len(models) != 6 || len(plugin.Middlewares()) != 1 {
		t.Fatalf("builtin models=%d middlewares=%d", len(models), len(plugin.Middlewares()))
	}
	if err := db.AutoMigrate(append([]any{&authModel.User{}}, models...)...); err != nil {
		t.Fatal(err)
	}
	builtinAPI := humagin.New(gin.New(), huma.DefaultConfig("builtin", "1"))
	plugin.RegisterRoutes(builtinAPI)
	visible := true
	hidden := false
	if err := db.Create(&authModel.User{ID: 7, UUID: "menu-user", Username: "menu-user", RoleID: rbacService.SuperAdminRoleID, Enable: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&rbacModel.Menu{Code: "workspace", Title: "Workspace", Path: "/workspace", App: "core", Type: "menu", IsVisible: &visible}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&rbacModel.Menu{Code: "hidden_workspace", Title: "Hidden workspace", Path: "/hidden", App: "core", Type: "menu", IsVisible: &hidden}).Error; err != nil {
		t.Fatal(err)
	}
	access, err := resolveAuthorization(context.Background(), 7, rbacService.SuperAdminRoleID)
	if err != nil {
		t.Fatal(err)
	}
	if len(access.Menus) != 1 || access.Menus[0].Code != "workspace" || access.Menus[0].Path != "/workspace" {
		t.Fatalf("authorization visible menus = %#v", access.Menus)
	}
}

func TestBuiltinRBACFailsFastWhenBuiltinAuthIsInactive(t *testing.T) {
	previous := global.PRISM_CONFIG
	global.PRISM_CONFIG.Auth = config.Auth{Provider: "custom"}
	global.PRISM_CONFIG.RBAC = config.RBAC{Provider: "builtin"}
	t.Cleanup(func() { global.PRISM_CONFIG = previous })

	registry := plugin.NewRegistry()
	if err := registry.Register(&authAddon.AuthPlugin{BasePlugin: plugin.BasePlugin{PluginName: "auth"}}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(&RbacPlugin{BasePlugin: plugin.BasePlugin{PluginName: "rbac"}}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Freeze(); !errors.Is(err, plugin.ErrMissingDependency) {
		t.Fatalf("Freeze() error=%v, want ErrMissingDependency", err)
	}
}
