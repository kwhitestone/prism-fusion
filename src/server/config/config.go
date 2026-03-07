package config

// Server 服务器配置结构
type Server struct {
	System  System  `mapstructure:"system" json:"system" yaml:"system"`
	JWT     JWT     `mapstructure:"jwt" json:"jwt" yaml:"jwt"`
	Auth    Auth    `mapstructure:"auth" json:"auth" yaml:"auth"`
	RBAC    RBAC    `mapstructure:"rbac" json:"rbac" yaml:"rbac"`
	Zap     Zap     `mapstructure:"zap" json:"zap" yaml:"zap"`
	Mysql   Mysql   `mapstructure:"mysql" json:"mysql" yaml:"mysql"`
	Sqlite  Sqlite  `mapstructure:"sqlite" json:"sqlite" yaml:"sqlite"`
	Captcha Captcha `mapstructure:"captcha" json:"captcha" yaml:"captcha"`
	CORS    CORS    `mapstructure:"cors" json:"cors" yaml:"cors"`
}
