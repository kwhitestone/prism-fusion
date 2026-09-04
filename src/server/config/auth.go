package config

// Auth 认证配置
type Auth struct {
	Provider    string `mapstructure:"provider" json:"provider" yaml:"provider"` // "builtin" (默认) 或自定义 provider
	ServeRoutes *bool  `mapstructure:"serve-routes" json:"serveRoutes" yaml:"serve-routes"`
}

// RBAC 权限控制配置
type RBAC struct {
	Provider string `mapstructure:"provider" json:"provider" yaml:"provider"` // "builtin"（控制面）、"external"（共享表消费）、"disabled" 或已注册的自定义 provider
}
