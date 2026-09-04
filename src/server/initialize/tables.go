package initialize

import (
	"fmt"

	"github.com/kwhitestone/prism-fusion/global"
	"github.com/kwhitestone/prism-fusion/plugin"

	"go.uber.org/zap"
	"gorm.io/gorm"
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
		}
		if err := migratePluginModels(db, p, models); err != nil {
			global.PRISM_LOG.Error("插件迁移失败", zap.String("plugin", p.Name()), zap.Error(err))
			panic(err)
		}
	}
	global.PRISM_LOG.Info("Plugin models migrated", zap.Int("pluginCount", plugin.Count()))
}

func migratePlugin(db *gorm.DB, p plugin.Plugin) error {
	return migratePluginModels(db, p, p.Models())
}

func migratePluginModels(db *gorm.DB, p plugin.Plugin, models []interface{}) error {
	if migrator, ok := p.(plugin.BeforeMigrator); ok {
		if err := migrator.BeforeMigrate(db); err != nil {
			return fmt.Errorf("prepare plugin %s migration: %w", p.Name(), err)
		}
	}
	if len(models) > 0 {
		if err := db.AutoMigrate(models...); err != nil {
			return fmt.Errorf("migrate plugin %s models: %w", p.Name(), err)
		}
	}
	if migrator, ok := p.(plugin.AfterMigrator); ok {
		if err := migrator.AfterMigrate(db); err != nil {
			return fmt.Errorf("finalize plugin %s migration: %w", p.Name(), err)
		}
	}
	return nil
}
