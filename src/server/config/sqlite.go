package config

// Sqlite SQLite配置
type Sqlite struct {
	Path         string `mapstructure:"path" json:"path" yaml:"path"`                               // 数据库文件路径
	MaxIdleConns int    `mapstructure:"max-idle-conns" json:"max-idle-conns" yaml:"max-idle-conns"` // 空闲中的最大连接数
	MaxOpenConns int    `mapstructure:"max-open-conns" json:"max-open-conns" yaml:"max-open-conns"` // 打开到数据库的最大连接数
	LogMode      string `mapstructure:"log-mode" json:"log-mode" yaml:"log-mode"`                  // 是否开启Gorm全局日志
	LogZap       bool   `mapstructure:"log-zap" json:"log-zap" yaml:"log-zap"`                     // 是否通过zap写入日志文件
}

func (s *Sqlite) Dsn() string {
	return s.Path
}

// GetLogMode 获取日志模式
func (s *Sqlite) GetLogMode() string {
	return s.LogMode
}