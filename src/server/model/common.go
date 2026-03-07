package model

import (
	"gorm.io/gorm"
)

// MODEL 基础模型
type MODEL struct {
	ID        uint           `json:"id" gorm:"primarykey;comment:主键ID"`
	CreatedAt *LocalTime     `json:"createdAt" gorm:"column:created_at;comment:创建时间"`
	UpdatedAt *LocalTime     `json:"updatedAt" gorm:"column:updated_at;comment:更新时间"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index;comment:删除时间"`
}

// 注意：User 和 Authority 模型已迁移至插件
// User -> addons/auth/model/user.go
// Authority -> addons/rbac/model/model.go (Role)
