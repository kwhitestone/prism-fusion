package middleware

import (
	"net/http"
	"net/url"
	"strings"

	"whitestone.top/prism-fusion/config"
	"whitestone.top/prism-fusion/global"

	"github.com/gin-gonic/gin"
)

// isSameOrigin 判断请求是否同源（前后端同源部署时自动放行）
func isSameOrigin(c *gin.Context, origin string) bool {
	if origin == "" {
		return true
	}

	parsedOrigin, err := url.Parse(origin)
	if err != nil {
		return false
	}

	// 获取请求的 Host（包含端口）
	requestHost := c.Request.Host

	// Origin 的 host（包含端口）
	originHost := parsedOrigin.Host

	// 同一个 host 就是同源
	return requestHost == originHost
}

// Cors 基于配置文件的跨域请求处理
func Cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method
		origin := c.Request.Header.Get("Origin")
		path := c.Request.URL.Path

		// 静态资源路径不需要 CORS 检查，直接放行（无论是否有 Origin 头）
		if strings.HasPrefix(path, "/static/") ||
			strings.HasPrefix(path, "/assets/") ||
			path == "/favicon.ico" ||
			path == "/platform-config.json" ||
			path == "/logo.svg" ||
			path == "/" {
			c.Next()
			return
		}

		// 如果没有 Origin 头，说明不是 CORS 请求，直接放行
		if origin == "" {
			c.Next()
			return
		}

		// 同源请求直接放行（前后端同源部署的场景）
		if isSameOrigin(c, origin) {
			c.Next()
			return
		}

		// === 以下是跨域请求的处理 ===

		// 检查origin是否在白名单中
		allowed := false
		var corsWhiteList *config.CORSWhitelist

		for _, whitelist := range global.PRISM_CONFIG.CORS.Whitelist {
			if whitelist.AllowOrigin == origin {
				allowed = true
				corsWhiteList = &whitelist
				break
			}
		}

		// 如果不在白名单中，拒绝请求
		if !allowed && global.PRISM_CONFIG.CORS.Mode == "strict-whitelist" {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}

		// 如果在白名单中，使用配置的CORS设置
		if corsWhiteList != nil {
			c.Header("Access-Control-Allow-Origin", corsWhiteList.AllowOrigin)
			c.Header("Access-Control-Allow-Headers", corsWhiteList.AllowHeaders)
			c.Header("Access-Control-Allow-Methods", corsWhiteList.AllowMethods)
			c.Header("Access-Control-Expose-Headers", corsWhiteList.ExposeHeaders)
			if corsWhiteList.AllowCredentials {
				c.Header("Access-Control-Allow-Credentials", "true")
			}
		} else {
			// 如果模式不是严格白名单，使用默认设置
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Headers", "Content-Type,AccessToken,X-CSRF-Token,Authorization,Token,X-Token,X-User-Id,auth_token,x-request-id,X-Request-ID,x-session-id,X-Session-ID")
			c.Header("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE, UPDATE")
			c.Header("Access-Control-Expose-Headers", "Content-Length, Access-Control-Allow-Origin, Access-Control-Allow-Headers, Content-Type, New-Token, New-Expires-At")
			c.Header("Access-Control-Allow-Credentials", "true")
		}

		// 放行所有OPTIONS方法
		if method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		// 处理请求
		c.Next()
	}
}
