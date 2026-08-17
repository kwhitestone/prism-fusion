package router

import (
	"context"
	"net/http"

	"github.com/kwhitestone/prism-fusion/addons/rbac/service"

	"github.com/danielgtaylor/huma/v2"
)

var (
	roleService = &service.RoleService{}
	menuService = &service.MenuService{}
)

// ---- Named response types (avoid Huma "duplicate name: DataStruct") ----

// AsyncRoutesOutput 动态路由响应
type AsyncRoutesOutput struct {
	Body struct {
		Success bool               `json:"success" doc:"是否成功"`
		Data    []service.MenuNode `json:"data" doc:"动态路由列表"`
	}
}

// RoleListOutput 角色列表响应
type RoleListOutput struct {
	Body struct {
		Code    int                `json:"code" example:"0" doc:"状态码"`
		Message string             `json:"message" example:"success" doc:"响应消息"`
		Data    []service.RoleInfo `json:"data" doc:"角色列表"`
	}
}

// PermissionsOutput 权限列表响应
type PermissionsOutput struct {
	Body struct {
		Code    int      `json:"code" example:"0" doc:"状态码"`
		Message string   `json:"message" example:"success" doc:"响应消息"`
		Data    []string `json:"data" doc:"权限标识列表"`
	}
}

// RegisterRoutes 注册 RBAC 路由到 Huma
func RegisterRoutes(api huma.API) {
	// 获取动态路由（前端调用 /get-async-routes 的真实后端实现）
	huma.Register(api, huma.Operation{
		OperationID: "getAsyncRoutes",
		Method:      http.MethodGet,
		Path:        "/api/v1/addons/rbac/async-routes",
		Summary:     "获取动态路由",
		Description: "获取后端配置的动态路由菜单",
		Tags:        []string{"RBAC"},
		Security: []map[string][]string{
			{"AuthTokenAuth": {}},
		},
	}, func(ctx context.Context, input *struct{}) (*AsyncRoutesOutput, error) {
		menus, err := menuService.GetAsyncRoutes()
		resp := &AsyncRoutesOutput{}
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "获取路由失败")
		}
		resp.Body.Success = true
		resp.Body.Data = menus
		if resp.Body.Data == nil {
			resp.Body.Data = []service.MenuNode{}
		}
		return resp, nil
	})

	// 获取角色列表
	huma.Register(api, huma.Operation{
		OperationID: "getRoleList",
		Method:      http.MethodGet,
		Path:        "/api/v1/addons/rbac/roles",
		Summary:     "获取角色列表",
		Description: "获取系统中所有角色",
		Tags:        []string{"RBAC"},
		Security: []map[string][]string{
			{"AuthTokenAuth": {}},
		},
	}, func(ctx context.Context, input *struct{}) (*RoleListOutput, error) {
		roles, err := roleService.GetRoleList()
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, err.Error())
		}

		resp := &RoleListOutput{}
		roleInfos := make([]service.RoleInfo, len(roles))
		for i, r := range roles {
			roleInfos[i] = service.RoleInfo{
				RoleID:        r.RoleID,
				RoleName:      r.RoleName,
				DefaultRouter: r.DefaultRouter,
			}
		}

		resp.Body.Code = 0
		resp.Body.Message = "success"
		resp.Body.Data = roleInfos
		return resp, nil
	})

	// 获取当前用户权限
	huma.Register(api, huma.Operation{
		OperationID: "getUserPermissions",
		Method:      http.MethodGet,
		Path:        "/api/v1/addons/rbac/permissions",
		Summary:     "获取当前用户权限",
		Description: "获取当前登录用户的按钮级别权限列表",
		Tags:        []string{"RBAC"},
		Security: []map[string][]string{
			{"AuthTokenAuth": {}},
		},
	}, func(ctx context.Context, input *struct {
		RoleID uint `query:"roleId" doc:"角色ID"`
	}) (*PermissionsOutput, error) {
		permissions, err := roleService.GetRolePermissions(input.RoleID)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, err.Error())
		}
		resp := &PermissionsOutput{}
		resp.Body.Code = 0
		resp.Body.Message = "success"
		resp.Body.Data = permissions
		if resp.Body.Data == nil {
			resp.Body.Data = []string{}
		}
		return resp, nil
	})
}
