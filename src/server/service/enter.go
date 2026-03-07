package service

// ServiceGroup 框架核心服务组
// 插件服务已迁移至 addons/ 目录，由插件自行管理
type ServiceGroup struct{}

var ServiceGroupApp = new(ServiceGroup)
