package core

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"whitestone.top/prism-fusion/global"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

// Viper 初始化配置文件
func Viper(path ...string) *viper.Viper {
	var configFile string
	if len(path) == 0 {
		flag.StringVar(&configFile, "c", "", "choose config file.")
		flag.Parse()
		if configFile == "" { // 判断命令行参数是否为空
			if configEnv := os.Getenv("PRISM_CONFIG"); configEnv == "" { // 判断 PRISM_CONFIG 环境变量是否为空
				switch gin.Mode() {
				case gin.DebugMode:
					configFile = "config.yaml"
				case gin.ReleaseMode:
					configFile = "config.yaml"
				case gin.TestMode:
					configFile = "config.yaml"
				}
			} else { // PRISM_CONFIG 环境变量不为空 将值赋值给configEnv
				configFile = configEnv
			}
		}
	} else {
		configFile = path[0]
	}

	v := viper.New()
	v.SetConfigFile(configFile)
	v.SetConfigType("yaml")

	// 启用环境变量支持
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))

	if err := v.ReadInConfig(); err != nil {
		panic(fmt.Errorf("Fatal error config file: %s \n", err))
	}

	v.WatchConfig()

	if err := v.Unmarshal(&global.PRISM_CONFIG); err != nil {
		panic(err)
	}

	// 展开配置中的环境变量
	expandEnvInConfig()

	// 根据当前文件设置根路径
	global.PRISM_CONFIG.System.Env = v.GetString("system.env")

	return v
}

// expandEnvWithDefault 展开环境变量，支持 ${VAR:-default} 语法
// Go 原生 os.ExpandEnv 不支持 :-default，此函数补充该功能
func expandEnvWithDefault(s string) string {
	return os.Expand(s, func(key string) string {
		// 处理 ${VAR:-default} 语法
		if idx := strings.Index(key, ":-"); idx >= 0 {
			envKey := key[:idx]
			defaultVal := key[idx+2:]
			if val := os.Getenv(envKey); val != "" {
				return val
			}
			return defaultVal
		}
		return os.Getenv(key)
	})
}

// expandEnvInConfig 展开配置中的环境变量
func expandEnvInConfig() {
	// 展开系统配置中的环境变量
	if portStr := os.Getenv("GATEWAY_PORT"); portStr != "" {
		if port, err := strconv.Atoi(portStr); err == nil {
			global.PRISM_CONFIG.System.Addr = port
		}
	}

	// 展开 MySQL 配置中的环境变量
	global.PRISM_CONFIG.Mysql.Host = expandEnvWithDefault(global.PRISM_CONFIG.Mysql.Host)
	global.PRISM_CONFIG.Mysql.Port = expandEnvWithDefault(global.PRISM_CONFIG.Mysql.Port)
	global.PRISM_CONFIG.Mysql.Dbname = expandEnvWithDefault(global.PRISM_CONFIG.Mysql.Dbname)
	global.PRISM_CONFIG.Mysql.Username = expandEnvWithDefault(global.PRISM_CONFIG.Mysql.Username)
	global.PRISM_CONFIG.Mysql.Password = expandEnvWithDefault(global.PRISM_CONFIG.Mysql.Password)
}
