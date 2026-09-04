package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kwhitestone/prism-fusion/addons/rbac/service"
	"github.com/kwhitestone/prism-fusion/global"
)

func RequirePermission(code string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, authenticated := c.Get("user_id"); !authenticated {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "authentication required"})
			return
		}
		value, resolved := c.Get("permissions")
		permissions, valid := value.([]string)
		if !resolved || !valid {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"code": 503, "message": "authorization state unavailable"})
			return
		}
		if !service.HasPermission(permissions, code) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 403, "message": "permission denied"})
			return
		}
		c.Set("required_permission", code)
		c.Next()
	}
}

func AuthorizeManagementRoutes() gin.HandlerFunc {
	return func(c *gin.Context) {
		required := ManagementPermission(c.Request.Method, c.Request.URL.Path)
		if required == "" {
			c.Next()
			return
		}
		if _, resolved := c.Get("permissions"); !resolved {
			userID, authenticated := c.Get("user_id")
			id, valid := userID.(uint)
			if !authenticated || !valid || id == 0 {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "authentication required"})
				return
			}
			access, err := service.NewAccessService(global.PRISM_DB).ResolveUserAuthorization(c.Request.Context(), id)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"code": 503, "message": "authorization state unavailable"})
				return
			}
			c.Set("roles", access.Roles)
			c.Set("permissions", access.Permissions)
		}
		RequirePermission(required)(c)
	}
}

func ManagementPermission(method, rawPath string) string {
	path := rawPath
	if index := strings.IndexByte(path, '?'); index >= 0 {
		path = path[:index]
	}
	path = strings.TrimSuffix(path, "/")
	relative := strings.TrimPrefix(path, "/api/v1/addons/rbac")
	if relative == "" || relative == "/me" || relative == "/async-routes" {
		return ""
	}
	segments := strings.Split(strings.Trim(relative, "/"), "/")
	if len(segments) == 0 {
		return "auth:super-admin:grant"
	}
	switch segments[0] {
	case "roles":
		if method == http.MethodGet {
			return "auth:role:read"
		}
		return "auth:role:write"
	case "permissions":
		return ""
	case "permission-catalog":
		return "auth:permission:read"
	case "menus":
		if method == http.MethodGet {
			return "auth:menu:read"
		}
		return "auth:menu:write"
	case "users":
		if len(segments) >= 3 && segments[len(segments)-1] == "roles" && method != http.MethodGet {
			return "auth:user-role:write"
		}
		return "auth:user:read"
	case "audit":
		return "auth:audit:read"
	default:
		return "auth:super-admin:grant"
	}
}
