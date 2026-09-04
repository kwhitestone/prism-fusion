package rbac

import (
	"context"

	authService "github.com/kwhitestone/prism-fusion/addons/auth/service"
	rbacMiddleware "github.com/kwhitestone/prism-fusion/addons/rbac/middleware"
	rbacModel "github.com/kwhitestone/prism-fusion/addons/rbac/model"
	rbacRouter "github.com/kwhitestone/prism-fusion/addons/rbac/router"
	"github.com/kwhitestone/prism-fusion/addons/rbac/service"
	"github.com/kwhitestone/prism-fusion/global"
	"github.com/kwhitestone/prism-fusion/plugin"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RbacPlugin 内置权限管理插件
type RbacPlugin struct {
	plugin.BasePlugin
}

func init() {
	for _, provider := range []string{"builtin", "external"} {
		if err := authService.RegisterAuthorizationResolver(provider, resolveAuthorization); err != nil {
			panic(err)
		}
	}
	plugin.Register(&RbacPlugin{
		BasePlugin: plugin.BasePlugin{
			PluginName:        "rbac",
			PluginDescription: "权限管理插件 - 提供角色、权限、动态路由管理",
		},
	})
}

func resolveAuthorization(ctx context.Context, userID, _ uint) (*authService.AuthorizationState, error) {
	access, err := service.NewAccessService(global.PRISM_DB).ResolveUserAuthorization(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &authService.AuthorizationState{Roles: access.Roles, Permissions: access.Permissions}, nil
}

// isEnabled 检查 builtin rbac 是否启用（默认启用）
func isEnabled() bool {
	provider := global.PRISM_CONFIG.RBAC.Provider
	return provider == "" || provider == "builtin"
}

func (p *RbacPlugin) Priority() int {
	// RBAC 在 auth 之后执行
	return 20
}

func (p *RbacPlugin) RoutePrefix() string {
	return "/api/v1/addons/rbac"
}

func (p *RbacPlugin) Manifest() plugin.Manifest {
	return plugin.Manifest{
		APIVersion: plugin.APIVersionV2,
		ID:         "rbac",
		Version:    "2.0.0",
		Kind:       plugin.KindBackendAddon,
		Requires:   []plugin.Dependency{{ID: "auth"}},
		RouteScopes: []string{
			p.RoutePrefix(),
		},
	}
}

func (p *RbacPlugin) PluginEnabled() bool {
	return isEnabled()
}

func (p *RbacPlugin) BeforeMigrate(db *gorm.DB) error {
	if !isEnabled() {
		return nil
	}
	return service.PrepareLegacySchema(db)
}

func (p *RbacPlugin) AfterMigrate(db *gorm.DB) error {
	if !isEnabled() {
		return nil
	}
	return service.MigrateLegacyData(db)
}

func (p *RbacPlugin) RegisterRoutes(api huma.API) {
	if !isEnabled() {
		return
	}

	rbacRouter.RegisterRoutes(api)

	if err := service.SeedRegisteredData(); err != nil {
		panic(err)
	}

	global.PRISM_LOG.Info("RBAC plugin routes registered")
}

func (p *RbacPlugin) Models() []interface{} {
	if !isEnabled() {
		return nil
	}
	return []interface{}{
		&rbacModel.Role{},
		&rbacModel.Permission{},
		&rbacModel.RolePermission{},
		&rbacModel.UserRole{},
		&rbacModel.Menu{},
		&rbacModel.AuditLog{},
	}
}

func (p *RbacPlugin) Middlewares() []gin.HandlerFunc {
	if !isEnabled() {
		return nil
	}
	return []gin.HandlerFunc{rbacMiddleware.AuthorizeManagementRoutes()}
}
