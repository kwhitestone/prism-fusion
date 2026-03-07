package router

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// InitScalarRouter 初始化Scalar API Reference路由
func InitScalarRouter(Router *gin.RouterGroup) {
	Router.GET("/scalar", func(c *gin.Context) {
		html := fmt.Sprintf(`<!doctype html>
<html>
<head>
    <title>Prism Fusion API Reference</title>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
</head>
<body>
    <script id="api-reference" data-url="/openapi.json"></script>
    <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
</body>
</html>`)
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(http.StatusOK, html)
	})
}
