package router

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// InitRedocRouter 初始化ReDoc文档路由
func InitRedocRouter(Router *gin.RouterGroup) {
	Router.GET("/redoc", func(c *gin.Context) {
		html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>Prism Fusion API Documentation</title>
    <meta charset="utf-8"/>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <link href="https://fonts.googleapis.com/css?family=Montserrat:300,400,700|Roboto:300,400,700" rel="stylesheet">
    <style>body { margin: 0; padding: 0; }</style>
</head>
<body>
    <redoc spec-url='/openapi.json'></redoc>
    <script src="https://cdn.jsdelivr.net/npm/redoc@latest/bundles/redoc.standalone.js"></script>
</body>
</html>`)
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(http.StatusOK, html)
	})
}
