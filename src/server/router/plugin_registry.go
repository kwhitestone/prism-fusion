package router

import (
	"context"
	"encoding/json"

	"github.com/danielgtaylor/huma/v2"
	"go.uber.org/zap"
	"github.com/kwhitestone/prism-fusion/global"
)

// ==================== 前端插件注册上报 ====================

// ReportMenuItem 前端上报的菜单项
type ReportMenuItem struct {
	Path     string           `json:"path" doc:"路由路径"`
	Name     string           `json:"name,omitempty" doc:"路由名称"`
	Title    string           `json:"title,omitempty" doc:"菜单标题"`
	Icon     string           `json:"icon,omitempty" doc:"菜单图标"`
	Rank     int              `json:"rank,omitempty" doc:"菜单排序"`
	ShowLink *bool            `json:"showLink,omitempty" doc:"是否在菜单中显示"`
	Children []ReportMenuItem `json:"children,omitempty" doc:"子菜单"`
}

// ReportPermission 前端上报的权限声明
type ReportPermission struct {
	Key         string `json:"key" doc:"权限唯一标识"`
	Name        string `json:"name" doc:"权限名称"`
	Description string `json:"description,omitempty" doc:"权限描述"`
}

// PluginRegistryEntry 单个插件的注册信息
type PluginRegistryEntry struct {
	Name        string             `json:"name" doc:"插件名称"`
	Description string             `json:"description,omitempty" doc:"插件描述"`
	Version     string             `json:"version,omitempty" doc:"插件版本"`
	Menus       []ReportMenuItem   `json:"menus" doc:"菜单列表"`
	Permissions []ReportPermission `json:"permissions" doc:"权限列表"`
}

// PluginRegistryPayload 前端上报的完整插件注册表
type PluginRegistryPayload struct {
	Plugins []PluginRegistryEntry `json:"plugins" doc:"插件列表"`
}

// InitPluginRegistryRoutes 注册插件注册上报 API
func InitPluginRegistryRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "reportPluginRegistry",
		Method:      "POST",
		Path:        "/api/v1/system/plugin-registry",
		Summary:     "上报前端插件注册表",
		Description: "前端页面初始化时上报所有插件的菜单和权限声明，后端收集并打印日志",
		Tags:        []string{"System"},
	}, func(ctx context.Context, input *struct {
		Body PluginRegistryPayload
	}) (*struct {
		Body struct {
			Code    int    `json:"code" example:"0" doc:"状态码"`
			Message string `json:"message" example:"success" doc:"响应消息"`
		}
	}, error) {
		payload := input.Body

		// 打印完整的插件注册表
		global.PRISM_LOG.Info("========== Frontend Plugin Registry ==========")

		totalMenus := 0
		totalPermissions := 0

		for _, p := range payload.Plugins {
			menuCount := countMenus(p.Menus)
			totalMenus += menuCount
			totalPermissions += len(p.Permissions)

			global.PRISM_LOG.Info("Plugin registered",
				zap.String("name", p.Name),
				zap.String("description", p.Description),
				zap.String("version", p.Version),
				zap.Int("menus", menuCount),
				zap.Int("permissions", len(p.Permissions)),
			)

			// 打印菜单树
			for _, menu := range p.Menus {
				printMenu(menu, 1)
			}

			// 打印权限列表
			for _, perm := range p.Permissions {
				global.PRISM_LOG.Info("  Permission",
					zap.String("key", perm.Key),
					zap.String("name", perm.Name),
					zap.String("description", perm.Description),
				)
			}
		}

		global.PRISM_LOG.Info("========== Registry Summary ==========",
			zap.Int("total_plugins", len(payload.Plugins)),
			zap.Int("total_menus", totalMenus),
			zap.Int("total_permissions", totalPermissions),
		)

		// 同时以 JSON 格式打印完整注册表，方便调试
		if jsonBytes, err := json.MarshalIndent(payload, "", "  "); err == nil {
			global.PRISM_LOG.Debug("Full plugin registry JSON",
				zap.String("registry", string(jsonBytes)),
			)
		}

		resp := &struct {
			Body struct {
				Code    int    `json:"code" example:"0" doc:"状态码"`
				Message string `json:"message" example:"success" doc:"响应消息"`
			}
		}{}
		resp.Body.Code = 0
		resp.Body.Message = "success"
		return resp, nil
	})
}

// printMenu 递归打印菜单树
func printMenu(menu ReportMenuItem, depth int) {
	indent := ""
	for i := 0; i < depth; i++ {
		indent += "  "
	}
	global.PRISM_LOG.Info(indent+"Menu",
		zap.String("path", menu.Path),
		zap.String("title", menu.Title),
		zap.String("icon", menu.Icon),
		zap.Int("rank", menu.Rank),
	)
	for _, child := range menu.Children {
		printMenu(child, depth+1)
	}
}

// countMenus 统计菜单总数（含子菜单）
func countMenus(menus []ReportMenuItem) int {
	count := len(menus)
	for _, m := range menus {
		count += countMenus(m.Children)
	}
	return count
}
