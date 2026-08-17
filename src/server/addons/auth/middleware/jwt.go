package authMiddleware

import (
	"net/http"
	"strings"
	"sync"

	"github.com/kwhitestone/prism-fusion/addons/auth/service"
	"github.com/kwhitestone/prism-fusion/global"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// 不需要认证的路径白名单
var publicPaths = []string{
	"/api/v1/addons/auth/login",
	"/api/v1/addons/auth/register",
	"/api/v1/addons/auth/refresh-token",
	"/api/llm-proxy",     // LLM 代理：用 x-llm-proxy-key 自鉴权（executor S2S）
	"/api/v1/addons/s2s", // Executor S2S：用 X-Executor-Token 自鉴权
	"/health",
	"/openapi",
	"/docs",
	"/scalar",
}

// publicPathsMu 保护 publicPaths 的并发追加。
var publicPathsMu sync.RWMutex

// AddPublicPath 运行时追加一个免 JWT 认证的路径前缀（供自鉴权的 S2S 插件用）。
func AddPublicPath(prefix string) {
	publicPathsMu.Lock()
	defer publicPathsMu.Unlock()
	publicPaths = append(publicPaths, prefix)
}

// JwtAuthMiddleware JWT 认证全局中间件
// 对 /api/ 路径强制认证（白名单除外），非 API 路径直接放行
func JwtAuthMiddleware() gin.HandlerFunc {
	jwtService := &service.JwtService{}

	return func(c *gin.Context) {
		path := c.Request.URL.Path

		// 非 API 路径直接放行（静态文件、前端页面等）
		if !strings.HasPrefix(path, "/api/") {
			c.Next()
			return
		}

		// 白名单路径放行
		publicPathsMu.RLock()
		paths := publicPaths
		publicPathsMu.RUnlock()
		for _, p := range paths {
			if path == p || strings.HasPrefix(path, p) {
				c.Next()
				return
			}
		}

		// 提取 Token
		tokenStr := c.GetHeader("Authorization")
		if tokenStr == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    401,
				"message": "未提供认证令牌",
			})
			return
		}

		// 去除 Bearer 前缀
		if strings.HasPrefix(tokenStr, "Bearer ") {
			tokenStr = tokenStr[7:]
		}

		// 解析验证 Token
		claims, err := jwtService.ParseToken(tokenStr)
		if err != nil {
			global.PRISM_LOG.Debug("JWT 验证失败",
				zap.String("path", path),
				zap.Error(err),
			)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    401,
				"message": "认证令牌无效或已过期",
			})
			return
		}

		// 将用户信息写入 Context，供后续处理使用
		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("role_id", claims.RoleID)

		// 检查是否需要刷新 Token
		if jwtService.NeedRefresh(claims) {
			newToken, err := jwtService.GenerateToken(claims.UserID, claims.Username, claims.RoleID)
			if err == nil {
				c.Header("X-New-Token", newToken)
			}
		}

		c.Next()
	}
}
