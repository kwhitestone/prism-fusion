package auth

import (
	authMiddleware "whitestone.top/prism-fusion/addons/auth/middleware"
	authModel "whitestone.top/prism-fusion/addons/auth/model"
	authRouter "whitestone.top/prism-fusion/addons/auth/router"
	"whitestone.top/prism-fusion/addons/auth/service"
	"whitestone.top/prism-fusion/global"
	"whitestone.top/prism-fusion/plugin"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gin-gonic/gin"
)

// AuthPlugin 内置认证插件
type AuthPlugin struct {
	plugin.BasePlugin
}

func init() {
	plugin.Register(&AuthPlugin{
		BasePlugin: plugin.BasePlugin{
			PluginName:        "auth",
			PluginDescription: "认证插件 - 提供 JWT 登录、注册、Token 刷新、用户管理",
		},
	})
}

// isEnabled 检查 builtin auth 是否启用（默认启用）
func isEnabled() bool {
	provider := global.PRISM_CONFIG.Auth.Provider
	return provider == "" || provider == "builtin"
}

func (p *AuthPlugin) Priority() int {
	// 认证插件最先执行
	return 10
}

func (p *AuthPlugin) RoutePrefix() string {
	return "/api/v1/addons/auth"
}

func (p *AuthPlugin) RegisterRoutes(api huma.API) {
	if !isEnabled() {
		return
	}

	authRouter.RegisterRoutes(api)

	// 初始化种子数据（管理员用户）
	userService := &service.UserService{}
	userService.SeedAdminUser()

	global.PRISM_LOG.Info("Auth plugin routes registered")
}

func (p *AuthPlugin) Models() []interface{} {
	if !isEnabled() {
		return nil
	}
	return []interface{}{
		&authModel.User{},
	}
}

func (p *AuthPlugin) GlobalMiddlewares() []gin.HandlerFunc {
	if !isEnabled() {
		return nil
	}
	return []gin.HandlerFunc{
		authMiddleware.JwtAuthMiddleware(),
	}
}
