package initialize

import (
	"github.com/kwhitestone/prism-fusion/global"
	"github.com/kwhitestone/prism-fusion/plugin"

	"go.uber.org/zap"
)

// InitTables 初始化数据库表
func InitTables() {
	db := global.PRISM_DB

	// 框架核心不再有自己的模型，所有模型由插件提供
	// 自动迁移所有插件模型（按优先级排序）
	for _, p := range plugin.Sorted() {
		models := p.Models()
		if len(models) > 0 {
			global.PRISM_LOG.Info("Migrating plugin models",
				zap.String("plugin", p.Name()),
				zap.Int("priority", p.Priority()),
				zap.Int("count", len(models)),
			)
			if err := db.AutoMigrate(models...); err != nil {
				global.PRISM_LOG.Error("插件模型迁移失败", zap.String("plugin", p.Name()), zap.Error(err))
			}
		}
	}
	global.PRISM_LOG.Info("Plugin models migrated", zap.Int("pluginCount", plugin.Count()))
}
