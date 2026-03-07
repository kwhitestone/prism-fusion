package config

// Auth 认证配置
type Auth struct {
	Provider string `mapstructure:"provider" json:"provider" yaml:"provider"` // "builtin" (默认) 或自定义 provider
}

// RBAC 权限控制配置
type RBAC struct {
	Provider string `mapstructure:"provider" json:"provider" yaml:"provider"` // "builtin" (默认) 或 "casbin"
}
