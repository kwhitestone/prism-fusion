package auth

import (
	authMiddleware "github.com/kwhitestone/prism-fusion/addons/auth/middleware"
	authModel "github.com/kwhitestone/prism-fusion/addons/auth/model"
	authRouter "github.com/kwhitestone/prism-fusion/addons/auth/router"
	"github.com/kwhitestone/prism-fusion/addons/auth/service"
	"github.com/kwhitestone/prism-fusion/global"
	"github.com/kwhitestone/prism-fusion/plugin"

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

func shouldServeRoutes() bool {
	configured := global.PRISM_CONFIG.Auth.ServeRoutes
	return configured == nil || *configured
}

func (p *AuthPlugin) Priority() int {
	// 认证插件最先执行
	return 10
}

func (p *AuthPlugin) RoutePrefix() string {
	return "/api/v1/addons/auth"
}

func (p *AuthPlugin) PluginEnabled() bool {
	return isEnabled()
}

func (p *AuthPlugin) RegisterRoutes(api huma.API) {
	if !isEnabled() {
		return
	}
	if err := service.ValidateTokenConfiguration(); err != nil {
		panic(err)
	}
	if !shouldServeRoutes() {
		return
	}

	authRouter.RegisterRoutes(api)

	userService := &service.UserService{}
	if err := userService.BootstrapAdminFromEnvironment(); err != nil {
		panic(err)
	}

	global.PRISM_LOG.Info("Auth plugin routes registered")
}

func (p *AuthPlugin) Models() []interface{} {
	if !isEnabled() {
		return nil
	}
	return []interface{}{
		&authModel.User{},
		&authModel.RefreshSession{},
		&authModel.AuthRateWindow{},
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
