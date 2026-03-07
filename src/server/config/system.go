package config

// System 系统配置
type System struct {
	Env           string `mapstructure:"env" json:"env" yaml:"env"`                                 // 环境值
	Addr          int    `mapstructure:"addr" json:"addr" yaml:"addr"`                             // 端口值
	RouterPrefix  string `mapstructure:"router-prefix" json:"router-prefix" yaml:"router-prefix"`
}