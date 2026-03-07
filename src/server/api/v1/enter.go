package v1

// ApiGroup 框架核心 API 组
// 插件 API 已迁移至 addons/ 目录，由插件自行管理
type ApiGroup struct{}

var ApiGroupApp = new(ApiGroup)
