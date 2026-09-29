package core

import (
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/kwhitestone/prism-fusion/config"
	"github.com/kwhitestone/prism-fusion/global"

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
	return expandEnv(s, nil)
}

// expandEnv 同 expandEnvWithDefault；missing 非 nil 时记录无默认值且为空的 ${VAR} 键名
func expandEnv(s string, missing *[]string) string {
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
		val := os.Getenv(key)
		if val == "" && missing != nil {
			*missing = append(*missing, key)
		}
		return val
	})
}

// resolveCORSWhitelist 展开 allow-origin 并在启动时校验（错误只含键名/模板，不含值）：
//   - '${VAR}'   必填：未设置或为空 → 启动失败，列出缺失键名
//   - '${VAR:-}' 可选：为空时移除该条目（该环境没有这个前端）
//   - 展开结果必须是精确 origin（scheme://host[:port]，小写，无路径/通配符/凭据/逗号列表），
//     否则中间件的精确匹配永远不会命中，属于静默错误
func resolveCORSWhitelist(entries []config.CORSWhitelist) ([]config.CORSWhitelist, error) {
	var missing, invalid []string
	resolved := make([]config.CORSWhitelist, 0, len(entries))
	for _, entry := range entries {
		raw := entry.AllowOrigin
		entry.AllowOrigin = expandEnv(raw, &missing)
		if entry.AllowOrigin == "" {
			continue
		}
		if !isExactOrigin(entry.AllowOrigin) {
			invalid = append(invalid, raw)
			continue
		}
		resolved = append(resolved, entry)
	}
	var errs []error
	if len(missing) > 0 {
		slices.Sort(missing)
		errs = append(errs, fmt.Errorf("cors whitelist: required env not set: %s (stage the value, or write '${KEY:-}' if the slot is optional)", strings.Join(slices.Compact(missing), ", ")))
	}
	if len(invalid) > 0 {
		errs = append(errs, fmt.Errorf("cors whitelist: value is not an exact http(s) origin for entries: %s", strings.Join(invalid, ", ")))
	}
	return resolved, errors.Join(errs...)
}

func isExactOrigin(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" &&
		s == u.Scheme+"://"+u.Host && u.Host == strings.ToLower(u.Host) && !strings.ContainsAny(u.Host, "*,@")
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

	// 展开 JWT signing-key 与 CORS 白名单中的环境变量（nucleagent 各服务在 config.yaml 用 ${VAR} 写法）
	global.PRISM_CONFIG.JWT.SigningKey = expandEnvWithDefault(global.PRISM_CONFIG.JWT.SigningKey)
	whitelist, err := resolveCORSWhitelist(global.PRISM_CONFIG.CORS.Whitelist)
	if err != nil {
		panic(err) // 与配置读取失败一致：RunApplication 将其转为启动错误
	}
	global.PRISM_CONFIG.CORS.Whitelist = whitelist
}
