package authMiddleware

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kwhitestone/prism-fusion/addons/auth/service"
	"github.com/kwhitestone/prism-fusion/global"
	"go.uber.org/zap"
)

const authRateWindowDuration = time.Minute

var authRateLimits = &service.AuthRateLimitService{}

func authRateScope(path string) string {
	return "auth-ip:" + strings.TrimPrefix(path, "/api/v1/addons/auth/")
}

func authPathLimit(path string) uint {
	switch path {
	case "/api/v1/addons/auth/login":
		return 30
	case "/api/v1/addons/auth/register":
		return 10
	case "/api/v1/addons/auth/refresh-token", "/api/v1/addons/auth/logout":
		return 120
	default:
		return 0
	}
}

func enforceAuthRateLimit(c *gin.Context, path string) bool {
	limit := authPathLimit(path)
	if limit == 0 {
		return true
	}
	allowed, err := authRateLimits.Allow(authRateScope(path), c.ClientIP(), limit)
	if err != nil {
		global.PRISM_LOG.Error("auth IP rate limit unavailable", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
			"code":    http.StatusServiceUnavailable,
			"message": "认证服务暂时不可用",
		})
		return false
	}
	if allowed {
		return true
	}
	c.Header("Retry-After", strconv.Itoa(int(authRateWindowDuration.Seconds())))
	c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
		"code":    http.StatusTooManyRequests,
		"message": "请求过于频繁，请稍后重试",
	})
	return false
}
