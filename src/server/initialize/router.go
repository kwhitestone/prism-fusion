package initialize

import (
	"net/http"
	"strings"

	"whitestone.top/prism-fusion/global"
	"whitestone.top/prism-fusion/middleware"
	"whitestone.top/prism-fusion/plugin"
	"whitestone.top/prism-fusion/router"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humagin"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Routers 初始化总路由
func Routers() *gin.Engine {
	Router := gin.Default()

	// 设置gin模式
	if global.PRISM_CONFIG.System.Env == "public" {
		gin.SetMode(gin.ReleaseMode) // 线上环境
	} else {
		gin.SetMode(gin.DebugMode) // 开发环境
	}

	// 添加中间件
	Router.Use(middleware.Cors())   // 跨域
	Router.Use(middleware.Logger()) // 日志

	// 按优先级排序注册插件中间件（优先级小的先执行，保证 auth 在 rbac 之前）
	sortedPlugins := plugin.Sorted()

	// 注册插件全局中间件（对所有路由生效）
	for _, p := range sortedPlugins {
		globalMws := p.GlobalMiddlewares()
		if len(globalMws) > 0 {
			global.PRISM_LOG.Info("Registering plugin global middlewares",
				zap.String("plugin", p.Name()),
				zap.Int("priority", p.Priority()),
				zap.Int("count", len(globalMws)),
			)
			Router.Use(globalMws...)
		}
	}

	// 注册插件作用域中间件（自动限定到插件的 RoutePrefix）
	for _, p := range sortedPlugins {
		middlewares := p.Middlewares()
		if len(middlewares) > 0 {
			prefix := p.RoutePrefix()
			global.PRISM_LOG.Info("Registering plugin scoped middlewares",
				zap.String("plugin", p.Name()),
				zap.Int("priority", p.Priority()),
				zap.String("prefix", prefix),
				zap.Int("count", len(middlewares)),
			)
			for _, mw := range middlewares {
				Router.Use(scopeMiddleware(prefix, mw))
			}
		}
	}

	// 静态文件服务 - 服务前端构建文件
	Router.Static("/static", "./web/static")
	Router.Static("/assets", "./web/assets")
	Router.StaticFile("/favicon.ico", "./web/favicon.ico")
	Router.StaticFile("/platform-config.json", "./web/platform-config.json")
	Router.StaticFile("/logo.svg", "./web/logo.svg")

	// 健康检查
	Router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"code":    0,
			"message": "server is running",
			"data": gin.H{
				"status": "healthy",
			},
		})
	})

	// 集成 Huma API 以生成 OpenAPI 3.1 文档
	config := huma.DefaultConfig("Prism Fusion API", "1.0.0")
	config.Info.Title = "Prism Fusion API"
	config.Info.Description = "Prism Fusion系统API接口文档 - 基于 OpenAPI 3.1 规范"
	config.Info.Contact = &huma.Contact{
		Name: "API Support",
		URL:  "https://github.com/kwhitestone/prism-fusion",
	}
	config.Info.License = &huma.License{
		Name: "MIT",
		URL:  "https://opensource.org/licenses/MIT",
	}

	// 添加安全定义
	config.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"AuthTokenAuth": {
			Type: "apiKey",
			In:   "header",
			Name: "Authorization",
		},
	}

	// 创建 Huma API 实例，自动生成 OpenAPI 3.1 文档
	api := humagin.New(Router, config)

	// ReDoc文档（美化版本的API文档）
	router.InitRedocRouter(Router.Group(""))

	// Scalar API Reference（现代化的交互式API测试工具）
	router.InitScalarRouter(Router.Group(""))

	// 注册 Huma 路由（所有API都通过Huma注册）
	router.InitHumaRoutes(api)

	// 自动注册所有插件路由（按优先级排序）
	for _, p := range sortedPlugins {
		global.PRISM_LOG.Info("Registering plugin routes",
			zap.String("plugin", p.Name()),
			zap.Int("priority", p.Priority()),
		)
		p.RegisterRoutes(api)
	}
	global.PRISM_LOG.Info("Plugin routes registered", zap.Int("count", plugin.Count()))

	// SPA 路由支持 - 除了 /api 路径外，其他路径都返回 index.html
	Router.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		// 如果是API路径，返回404
		if len(path) > 4 && path[:4] == "/api" {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    404,
				"message": "API endpoint not found",
				"path":    path,
			})
			return
		}
		// 其他路径返回前端页面
		c.File("./web/index.html")
	})

	global.PRISM_LOG.Info("router register success")
	return Router
}

// scopeMiddleware 将中间件限定为仅对指定路径前缀生效
// 框架自动调用，插件中间件无需自行判断路径
func scopeMiddleware(prefix string, mw gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, prefix) {
			mw(c)
		} else {
			c.Next()
		}
	}
}
