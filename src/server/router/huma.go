package router

import (
	"github.com/danielgtaylor/huma/v2"
)

// InitHumaRoutes 初始化框架核心 Huma API 路由
// 插件路由通过 plugin.RegisterRoutes 自动注册，无需在此添加
func InitHumaRoutes(api huma.API) {
	// 框架核心路由
	InitPluginRegistryRoutes(api)
}
