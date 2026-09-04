package plugin

import (
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

var defaultRegistry = NewRegistry()

// Register 注册插件（在插件的 init() 中调用）
//
// 为保持 V1 源码兼容，该函数保留无返回值签名。注册失败会直接 panic，
// 从而避免重复插件或无效插件被静默覆盖。
func Register(p Plugin) {
	if err := defaultRegistry.Register(p); err != nil {
		panic(err)
	}
}

// TryRegister 注册插件并向调用方返回校验错误。
func TryRegister(p Plugin) error {
	return defaultRegistry.Register(p)
}

// Freeze 校验依赖图并冻结全局注册表。
func Freeze() error {
	return defaultRegistry.Freeze()
}

// Resolve 返回依赖解析后的插件及其不可变 Manifest 快照。
func Resolve() ([]ResolvedPlugin, error) {
	return defaultRegistry.Resolve()
}

// MustResolve 冻结全局注册表并返回解析结果，失败时 panic。
func MustResolve() []ResolvedPlugin {
	return defaultRegistry.MustResolve()
}

// All 返回所有已注册的插件
func All() map[string]Plugin {
	return defaultRegistry.All()
}

// Sorted 返回依赖解析后的插件列表。
//
// V2 中依赖关系优先于 Priority；无依赖约束时按 Priority、Name 稳定排序。
// 首次调用会冻结注册表，解析错误会 panic，以保持 V1 函数签名兼容。
func Sorted() []Plugin {
	resolved := defaultRegistry.MustResolve()
	plugins := make([]Plugin, 0, len(resolved))
	for _, entry := range resolved {
		plugins = append(plugins, entry.Plugin)
	}
	return plugins
}

// Get 根据名称获取插件
func Get(name string) (Plugin, bool) {
	return defaultRegistry.Get(name)
}

// Names 返回所有已注册插件的名称列表
func Names() []string {
	return defaultRegistry.Names()
}

// Count 返回已注册插件数量
func Count() int {
	return defaultRegistry.Count()
}
