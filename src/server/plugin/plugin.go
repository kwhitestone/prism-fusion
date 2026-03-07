package plugin

import (
	"sort"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gin-gonic/gin"
)

// Plugin 插件接口，所有插件必须实现此接口
type Plugin interface {
	// Name 返回插件唯一标识
	Name() string
	// Description 返回插件描述
	Description() string
	// Priority 返回插件优先级，数值越小越先执行
	// auth=10, rbac=20, 普通插件默认=100
	Priority() int
	// RoutePrefix 返回插件路由前缀，框架据此限定中间件作用域
	// 默认返回 /api/v1/addons/{name}
	RoutePrefix() string
	// RegisterRoutes 注册插件路由到 Huma API
	RegisterRoutes(api huma.API)
	// Models 返回需要自动迁移的数据模型
	Models() []interface{}
	// Middlewares 返回插件中间件，框架会自动限定为仅对 RoutePrefix() 生效
	Middlewares() []gin.HandlerFunc
	// GlobalMiddlewares 返回全局中间件，对所有路由生效
	GlobalMiddlewares() []gin.HandlerFunc
}

// PluginRegistry 全局插件注册表
var registry = make(map[string]Plugin)

// Register 注册插件（在插件的 init() 中调用）
func Register(p Plugin) {
	registry[p.Name()] = p
}

// All 返回所有已注册的插件
func All() map[string]Plugin {
	return registry
}

// Sorted 返回按 Priority 排序的插件列表（优先级小的在前）
func Sorted() []Plugin {
	plugins := make([]Plugin, 0, len(registry))
	for _, p := range registry {
		plugins = append(plugins, p)
	}
	sort.Slice(plugins, func(i, j int) bool {
		return plugins[i].Priority() < plugins[j].Priority()
	})
	return plugins
}

// Get 根据名称获取插件
func Get(name string) (Plugin, bool) {
	p, ok := registry[name]
	return p, ok
}

// Names 返回所有已注册插件的名称列表
func Names() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	return names
}

// Count 返回已注册插件数量
func Count() int {
	return len(registry)
}
