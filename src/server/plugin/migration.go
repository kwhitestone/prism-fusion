package plugin

import "gorm.io/gorm"

// BeforeMigrator 是模型迁移前的可选扩展点。
type BeforeMigrator interface {
	BeforeMigrate(db *gorm.DB) error
}

// AfterMigrator 是模型迁移后的可选扩展点。
type AfterMigrator interface {
	AfterMigrate(db *gorm.DB) error
}
