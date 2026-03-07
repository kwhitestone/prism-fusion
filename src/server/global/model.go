package global

import (
	"time"

	"gorm.io/gorm"
)

// MODEL 基础模型，包含公共字段
type MODEL struct {
	ID        uint           `json:"id" gorm:"primarykey"` // 主键ID
	CreatedAt time.Time      `json:"createdAt"`            // 创建时间
	UpdatedAt time.Time      `json:"updatedAt"`            // 更新时间
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`       // 删除时间
}

// PAGINATE 分页请求
type PAGINATE struct {
	Page     int `json:"page" form:"page"`         // 页码
	PageSize int `json:"pageSize" form:"pageSize"` // 每页大小
}

// PageInfo 分页结果
type PageInfo struct {
	Page     int `json:"page"`     // 页码
	PageSize int `json:"pageSize"` // 每页大小
	Total    int64 `json:"total"`    // 总数
}

// PageResult 分页返回结构
type PageResult struct {
	List     interface{} `json:"list"`     // 数据列表
	Total    int64      `json:"total"`    // 总数
	Page     int        `json:"page"`     // 页码
	PageSize int        `json:"pageSize"` // 每页大小
}