package initialize

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/kwhitestone/prism-fusion/global"
	"github.com/kwhitestone/prism-fusion/middleware"
	"github.com/kwhitestone/prism-fusion/plugin"
	"github.com/kwhitestone/prism-fusion/router"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humagin"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Routers 初始化总路由
func Routers() *gin.Engine {
	Router := gin.Default()
	// Never trust arbitrary X-Forwarded-For values. Deployments behind a proxy
	// must establish trust at the edge instead of accepting client-supplied IPs.
	if err := Router.SetTrustedProxies(global.PRISM_CONFIG.System.TrustedProxies); err != nil {
		panic(err)
	}

	// 设置gin模式
	if global.PRISM_CONFIG.System.Env == "public" {
		gin.SetMode(gin.ReleaseMode) // 线上环境
	} else {
		gin.SetMode(gin.DebugMode) // 开发环境
	}

	// 添加中间件
	Router.Use(middleware.Cors())   // 跨域
	Router.Use(middleware.Logger()) // 日志

	// 冻结注册窗口并按 V2 依赖拓扑解析；无依赖约束时按优先级和 ID 稳定排序。
	resolvedPlugins := plugin.MustResolve()

	// 注册插件全局中间件（对所有路由生效）
	for _, resolved := range resolvedPlugins {
		p := resolved.Plugin
		pluginID := resolved.Manifest.ID
		globalMws := p.GlobalMiddlewares()
		if len(globalMws) > 0 {
			global.PRISM_LOG.Info("Registering plugin global middlewares",
				zap.String("plugin", pluginID),
				zap.Int("priority", resolved.Priority),
				zap.Int("count", len(globalMws)),
			)
			Router.Use(globalMws...)
		}
	}

	// 路由命名空间由所有插件独占，与插件是否提供作用域中间件无关。
	pluginScopes := make(map[string][]string, len(resolvedPlugins))
	scopedMiddlewares := make(map[string][]gin.HandlerFunc, len(resolvedPlugins))
	for _, resolved := range resolvedPlugins {
		p := resolved.Plugin
		pluginID := resolved.Manifest.ID
		middlewares := p.Middlewares()
		pluginScopes[pluginID] = resolved.Manifest.RouteScopes
		if len(middlewares) > 0 {
			scopes := resolved.Manifest.RouteScopes
			if len(scopes) == 0 {
				panic(fmt.Errorf("plugin %q has scoped middlewares but no route scopes", pluginID))
			}
			scopedMiddlewares[pluginID] = middlewares
		}
	}
	if err := validateRouteScopeIsolation(pluginScopes); err != nil {
		panic(err)
	}
	for _, resolved := range resolvedPlugins {
		pluginID := resolved.Manifest.ID
		middlewares := scopedMiddlewares[pluginID]
		if len(middlewares) > 0 {
			scopes := resolved.Manifest.RouteScopes
			global.PRISM_LOG.Info("Registering plugin scoped middlewares",
				zap.String("plugin", pluginID),
				zap.Int("priority", resolved.Priority),
				zap.Strings("scopes", scopes),
				zap.Int("count", len(middlewares)),
			)
			for _, mw := range middlewares {
				Router.Use(scopeMiddlewareFor(scopes, mw))
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

	// 核心路由与 system 命名空间不允许被插件声明或按其他 HTTP 方法复用。
	if err := validateCoreRouteIsolation(pluginScopes, Router.Routes()); err != nil {
		panic(err)
	}

	// 自动注册所有插件路由（按优先级排序）
	for _, resolved := range resolvedPlugins {
		p := resolved.Plugin
		pluginID := resolved.Manifest.ID
		global.PRISM_LOG.Info("Registering plugin routes",
			zap.String("plugin", pluginID),
			zap.Int("priority", resolved.Priority),
		)
		beforeRoutes := Router.Routes()
		p.RegisterRoutes(api)
		if err := validateAddedPluginRoutes(pluginID, resolved.Manifest.RouteScopes, beforeRoutes, Router.Routes()); err != nil {
			panic(err)
		}
	}
	global.PRISM_LOG.Info("Plugin routes registered",
		zap.Int("activeCount", len(resolvedPlugins)),
		zap.Int("installedCount", plugin.Count()),
	)

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
	return scopeMiddlewareFor([]string{prefix}, mw)
}

func scopeMiddlewareFor(scopes []string, mw gin.HandlerFunc) gin.HandlerFunc {
	if len(scopes) == 0 {
		panic("scoped middleware requires at least one route scope")
	}
	normalizedScopes := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		normalized, err := plugin.NormalizeRouteScope(scope)
		if err != nil {
			panic(fmt.Errorf("invalid middleware route scope %q: %w", scope, err))
		}
		normalizedScopes = append(normalizedScopes, normalized)
	}

	return func(c *gin.Context) {
		for _, scope := range normalizedScopes {
			if pathMatchesScope(c.Request.URL.Path, scope) {
				mw(c)
				return
			}
		}
		c.Next()
	}
}

func pathMatchesScope(requestPath, scope string) bool {
	return scope == "/" || requestPath == scope || strings.HasPrefix(requestPath, scope+"/")
}

func validateRouteScopeIsolation(pluginScopes map[string][]string) error {
	pluginIDs := make([]string, 0, len(pluginScopes))
	for pluginID := range pluginScopes {
		pluginIDs = append(pluginIDs, pluginID)
	}
	sort.Strings(pluginIDs)
	for index, ownerID := range pluginIDs {
		for _, otherID := range pluginIDs[index+1:] {
			for _, ownerScope := range pluginScopes[ownerID] {
				for _, otherScope := range pluginScopes[otherID] {
					if pathMatchesScope(ownerScope, otherScope) || pathMatchesScope(otherScope, ownerScope) {
						return fmt.Errorf(
							"plugin %q route scope %q overlaps plugin %q scope %q",
							ownerID, ownerScope, otherID, otherScope,
						)
					}
				}
			}
		}
	}
	return nil
}

// All core route prefixes are reserved across HTTP methods. Dynamic core
// routes reserve their static parent namespace, e.g. /assets/*filepath.
func validateCoreRouteIsolation(pluginScopes map[string][]string, coreRoutes []gin.RouteInfo) error {
	reserved := map[string]struct{}{"/api/v1/system": {}}
	for _, route := range coreRoutes {
		scope := route.Path
		if dynamic := strings.IndexAny(scope, ":*"); dynamic >= 0 {
			scope = scope[:strings.LastIndex(scope[:dynamic], "/")]
			if scope == "" {
				scope = "/"
			}
		}
		reserved[scope] = struct{}{}
	}
	coreScopes := make([]string, 0, len(reserved))
	for scope := range reserved {
		coreScopes = append(coreScopes, scope)
	}
	sort.Strings(coreScopes)
	pluginIDs := make([]string, 0, len(pluginScopes))
	for pluginID := range pluginScopes {
		pluginIDs = append(pluginIDs, pluginID)
	}
	sort.Strings(pluginIDs)
	for _, pluginID := range pluginIDs {
		for _, scope := range pluginScopes[pluginID] {
			for _, coreScope := range coreScopes {
				if pathMatchesScope(scope, coreScope) || pathMatchesScope(coreScope, scope) {
					return fmt.Errorf("plugin %q route scope %q overlaps reserved core scope %q", pluginID, scope, coreScope)
				}
			}
		}
	}
	return nil
}

func validateAddedPluginRoutes(pluginID string, scopes []string, before, after []gin.RouteInfo) error {
	existing := make(map[string]struct{}, len(before))
	for _, route := range before {
		existing[route.Method+"\x00"+route.Path] = struct{}{}
	}
	for _, route := range after {
		if _, alreadyRegistered := existing[route.Method+"\x00"+route.Path]; alreadyRegistered {
			continue
		}
		inScope := false
		for _, scope := range scopes {
			if pathMatchesScope(route.Path, scope) {
				inScope = true
				break
			}
		}
		if !inScope {
			return fmt.Errorf("plugin %q registered %s %s outside route scopes %v", pluginID, route.Method, route.Path, scopes)
		}
	}
	return nil
}
