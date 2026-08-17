package rbac

import (
	rbacModel "github.com/kwhitestone/prism-fusion/addons/rbac/model"
	rbacRouter "github.com/kwhitestone/prism-fusion/addons/rbac/router"
	"github.com/kwhitestone/prism-fusion/addons/rbac/service"
	"github.com/kwhitestone/prism-fusion/global"
	"github.com/kwhitestone/prism-fusion/plugin"

	"github.com/danielgtaylor/huma/v2"
)

// RbacPlugin 内置权限管理插件
type RbacPlugin struct {
	plugin.BasePlugin
}

func init() {
	plugin.Register(&RbacPlugin{
		BasePlugin: plugin.BasePlugin{
			PluginName:        "rbac",
			PluginDescription: "权限管理插件 - 提供角色、权限、动态路由管理",
		},
	})
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

func (p *RbacPlugin) RegisterRoutes(api huma.API) {
	if !isEnabled() {
		return
	}

	rbacRouter.RegisterRoutes(api)

	// 初始化种子数据
	service.SeedData()

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
		&rbacModel.Menu{},
	}
}
