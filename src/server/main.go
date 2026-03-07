package main

import (
	"whitestone.top/prism-fusion/core"
	"whitestone.top/prism-fusion/global"
	"whitestone.top/prism-fusion/initialize"

	"go.uber.org/zap"

	// 导入插件包，触发所有插件的 init() 自动注册
	_ "whitestone.top/prism-fusion/addons"
)

//go:generate go env -w GO111MODULE=on
//go:generate go env -w GOPROXY=https://goproxy.cn,direct
//go:generate go mod tidy
//go:generate go mod download

func main() {
	// 初始化系统
	initializeSystem()
	// 运行服务器
	core.RunServer()
}

// initializeSystem 初始化系统所有组件
func initializeSystem() {
	global.PRISM_VP = core.Viper() // 初始化Viper
	global.PRISM_LOG = core.Zap()  // 初始化zap日志库
	zap.ReplaceGlobals(global.PRISM_LOG)
	global.PRISM_DB = initialize.Gorm() // gorm连接数据库
	if global.PRISM_DB != nil {
		// 数据库连接成功
		global.PRISM_LOG.Info("Database connected successfully")
		initialize.InitTables() // 初始化数据库表
	}

	global.PRISM_LOG.Info("System initialized successfully")
}
