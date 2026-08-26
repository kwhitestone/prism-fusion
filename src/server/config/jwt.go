package config

// JWT JWT配置
type JWT struct {
	SigningKey               string `mapstructure:"signing-key" json:"signing-key" yaml:"signing-key"`                                                 // jwt签名
	ExpiresTime              string `mapstructure:"expires-time" json:"expires-time" yaml:"expires-time"`                                              // access token 过期时间
	RefreshExpiresTime       string `mapstructure:"refresh-expires-time" json:"refresh-expires-time" yaml:"refresh-expires-time"`                      // 单个 refresh token 空闲过期时间
	RefreshFamilyExpiresTime string `mapstructure:"refresh-family-expires-time" json:"refresh-family-expires-time" yaml:"refresh-family-expires-time"` // refresh 会话族绝对过期时间
	RefreshRotationGrace     string `mapstructure:"refresh-rotation-grace" json:"refresh-rotation-grace" yaml:"refresh-rotation-grace"`                // 响应丢失时的幂等重放窗口
	BufferTime               string `mapstructure:"buffer-time" json:"buffer-time" yaml:"buffer-time"`                                                 // 缓冲时间
	Issuer                   string `mapstructure:"issuer" json:"issuer" yaml:"issuer"`                                                                // 签发者
}
