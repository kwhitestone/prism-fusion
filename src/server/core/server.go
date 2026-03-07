package core

import (
	"fmt"
	"time"

	"whitestone.top/prism-fusion/global"
	"whitestone.top/prism-fusion/initialize"
)

// RunServer 启动服务器
func RunServer() {
	// 暂时跳过Redis和MongoDB初始化
	// if global.PRISM_CONFIG.System.UseRedis {
	//	// 初始化redis服务
	//	initialize.Redis()
	// }

	// if global.PRISM_CONFIG.System.UseMongo {
	//	err := initialize.Mongo()
	//	if err != nil {
	//		zap.L().Error(fmt.Sprintf("%+v", err))
	//	}
	// }

	Router := initialize.Routers()

	address := fmt.Sprintf(":%d", global.PRISM_CONFIG.System.Addr)

	fmt.Printf(`
🔷 Prism Fusion
项目地址：https://github.com/kwhitestone/prism-fusion

🏠 前端应用: http://127.0.0.1%s

📚 API 文档:
  ReDoc 文档:   http://127.0.0.1%s/redoc
  Scalar 文档:  http://127.0.0.1%s/scalar
  OpenAPI JSON: http://127.0.0.1%s/openapi.json

🔗 服务端点:
  健康检查: http://127.0.0.1%s/health
  API 健康: http://127.0.0.1%s/api/health

🚀 服务器启动于端口: %s
`, address, address, address, address, address, address, address)

	initServer(address, Router, 10*time.Minute, 10*time.Minute)
}
