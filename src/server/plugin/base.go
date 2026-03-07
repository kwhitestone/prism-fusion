package plugin

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/gin-gonic/gin"
)

// BasePlugin 提供默认实现，插件可嵌入此结构体简化开发
type BasePlugin struct {
	PluginName        string
	PluginDescription string
}

func (b *BasePlugin) Name() string {
	return b.PluginName
}

func (b *BasePlugin) Description() string {
	return b.PluginDescription
}

func (b *BasePlugin) Priority() int {
	// 普通插件默认优先级为 100
	return 100
}

func (b *BasePlugin) RoutePrefix() string {
	// 默认根据插件名生成路由前缀
	return "/api/v1/addons/" + b.PluginName
}

func (b *BasePlugin) RegisterRoutes(api huma.API) {
	// 默认空实现，子类可覆盖
}

func (b *BasePlugin) Models() []interface{} {
	// 默认返回空切片
	return nil
}

func (b *BasePlugin) Middlewares() []gin.HandlerFunc {
	// 默认返回空切片
	return nil
}

func (b *BasePlugin) GlobalMiddlewares() []gin.HandlerFunc {
	// 默认返回空切片
	return nil
}
