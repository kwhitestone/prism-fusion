package router

import (
	"context"
	"errors"
	"net/http"
	"time"

	authService "github.com/kwhitestone/prism-fusion/addons/auth/service"
	"github.com/kwhitestone/prism-fusion/addons/rbac/model"
	rbacService "github.com/kwhitestone/prism-fusion/addons/rbac/service"
	"github.com/kwhitestone/prism-fusion/global"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"
)

type EmptyData struct{}

type AccessOutput struct {
	Body struct {
		Code    int                     `json:"code"`
		Message string                  `json:"message"`
		Data    *rbacService.UserAccess `json:"data"`
	}
}

type AsyncRoutesOutput struct {
	Body struct {
		Success bool                   `json:"success"`
		Data    []rbacService.MenuNode `json:"data"`
	}
}

type PermissionCodesOutput struct {
	Body struct {
		Code    int      `json:"code"`
		Message string   `json:"message"`
		Data    []string `json:"data"`
	}
}

type RoleView struct {
	ID            uint      `json:"id"`
	RoleID        uint      `json:"roleId"`
	Code          string    `json:"code"`
	Name          string    `json:"name"`
	RoleName      string    `json:"roleName"`
	Description   string    `json:"description"`
	IsSystem      bool      `json:"isSystem"`
	IsEnabled     bool      `json:"isEnabled"`
	ParentID      uint      `json:"parentId"`
	DefaultRouter string    `json:"defaultRouter"`
	DataScope     string    `json:"dataScope"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type RolesOutput struct {
	Body struct {
		Code    int        `json:"code"`
		Message string     `json:"message"`
		Data    []RoleView `json:"data"`
	}
}

type RoleOutput struct {
	Body struct {
		Code    int       `json:"code"`
		Message string    `json:"message"`
		Data    *RoleView `json:"data"`
	}
}

type PermissionsOutput struct {
	Body struct {
		Code    int                `json:"code"`
		Message string             `json:"message"`
		Data    []model.Permission `json:"data"`
	}
}

type MenusOutput struct {
	Body struct {
		Code    int          `json:"code"`
		Message string       `json:"message"`
		Data    []model.Menu `json:"data"`
	}
}

type MenuOutput struct {
	Body struct {
		Code    int         `json:"code"`
		Message string      `json:"message"`
		Data    *model.Menu `json:"data"`
	}
}

type UsersOutput struct {
	Body struct {
		Code    int                   `json:"code"`
		Message string                `json:"message"`
		Data    *rbacService.UserPage `json:"data"`
	}
}

type AuditOutput struct {
	Body struct {
		Code    int                    `json:"code"`
		Message string                 `json:"message"`
		Data    *rbacService.AuditPage `json:"data"`
	}
}

type EmptyOutput struct {
	Body struct {
		Code    int       `json:"code"`
		Message string    `json:"message"`
		Data    EmptyData `json:"data"`
	}
}

type AuthHeader struct {
	Authorization string `header:"Authorization" required:"true" doc:"Bearer access token"`
	RequestID     string `header:"X-Request-ID" maxLength:"128"`
}

type RolePath struct {
	RoleID uint `path:"roleId" minimum:"1"`
}

type RoleCreateInput struct {
	AuthHeader
	Body struct {
		Code        string `json:"code" minLength:"2" maxLength:"64"`
		Name        string `json:"name" minLength:"1" maxLength:"64"`
		Description string `json:"description,omitempty" maxLength:"255"`
	}
}

type RoleUpdateInput struct {
	AuthHeader
	RolePath
	Body struct {
		Name        *string `json:"name,omitempty" minLength:"1" maxLength:"64"`
		Description *string `json:"description,omitempty" maxLength:"255"`
		IsEnabled   *bool   `json:"isEnabled,omitempty"`
	}
}

type RoleMutationInput struct {
	AuthHeader
	RolePath
}

type RolePermissionsInput struct {
	AuthHeader
	RolePath
	Body struct {
		PermissionIDs []uint `json:"permissionIds" maxItems:"1000"`
	}
}

type MenuBody struct {
	ParentID       uint   `json:"parentId"`
	Code           string `json:"code" minLength:"2" maxLength:"64"`
	Title          string `json:"title" maxLength:"128"`
	TitleKey       string `json:"titleKey,omitempty" maxLength:"128"`
	Path           string `json:"path,omitempty" maxLength:"255"`
	Icon           string `json:"icon,omitempty" maxLength:"512"`
	App            string `json:"app" minLength:"1" maxLength:"64"`
	Type           string `json:"type" enum:"group,menu,button"`
	PermissionCode string `json:"permissionCode,omitempty" maxLength:"255"`
	Sort           int    `json:"sort" minimum:"0" maximum:"100000"`
	IsVisible      bool   `json:"isVisible"`
}

type MenuCreateInput struct {
	AuthHeader
	Body MenuBody
}

type MenuUpdateInput struct {
	AuthHeader
	MenuID uint `path:"menuId" minimum:"1"`
	Body   MenuBody
}

type MenuDeleteInput struct {
	AuthHeader
	MenuID uint `path:"menuId" minimum:"1"`
}

type UserListInput struct {
	AuthHeader
	Page     int    `query:"page" default:"1" minimum:"1"`
	PageSize int    `query:"pageSize" default:"20" minimum:"1" maximum:"100"`
	Search   string `query:"search" maxLength:"100"`
}

type UserRolesInput struct {
	AuthHeader
	UserID uint `path:"userId" minimum:"1"`
	Body   struct {
		RoleIDs []uint `json:"roleIds" minItems:"1" maxItems:"100"`
	}
}

type AuditListInput struct {
	AuthHeader
	Page     int `query:"page" default:"1" minimum:"1"`
	PageSize int `query:"pageSize" default:"20" minimum:"1" maximum:"100"`
}

func RegisterRoutes(api huma.API) {
	access := rbacService.NewAccessService(global.PRISM_DB)
	registerSelfRoutes(api, access)
	registerRoleRoutes(api, access)
	registerMenuRoutes(api, access)
	registerUserRoutes(api, access)
	registerAuditRoutes(api, access)
}

func registerSelfRoutes(api huma.API, access *rbacService.AccessService) {
	huma.Register(api, operation("rbacGetCurrentAccess", http.MethodGet, "/api/v1/addons/rbac/me", "Get current user access"),
		func(ctx context.Context, input *AuthHeader) (*AccessOutput, error) {
			actorID, err := actorFromHeader(input.Authorization)
			if err != nil {
				return nil, err
			}
			resolved, err := access.ResolveUserAccess(ctx, actorID)
			if err != nil {
				return nil, mapError(err)
			}
			out := &AccessOutput{}
			out.Body.Message = "success"
			out.Body.Data = resolved
			return out, nil
		})

	huma.Register(api, huma.Operation{
		OperationID: "rbacGetAsyncRoutes", Method: http.MethodGet, Path: "/api/v1/addons/rbac/async-routes",
		Summary: "Get current user routes", Tags: []string{"RBAC"},
		Security: []map[string][]string{{"AuthTokenAuth": {}}},
	}, func(ctx context.Context, input *AuthHeader) (*AsyncRoutesOutput, error) {
		actorID, err := actorFromHeader(input.Authorization)
		if err != nil {
			return nil, err
		}
		routes, err := access.ResolveAsyncRoutes(ctx, actorID)
		if err != nil {
			return nil, mapError(err)
		}
		out := &AsyncRoutesOutput{}
		out.Body.Success = true
		out.Body.Data = routes
		return out, nil
	})

	huma.Register(api, operation("rbacGetCurrentPermissions", http.MethodGet, "/api/v1/addons/rbac/permissions", "Get current user permissions"),
		func(ctx context.Context, input *AuthHeader) (*PermissionCodesOutput, error) {
			actorID, err := actorFromHeader(input.Authorization)
			if err != nil {
				return nil, err
			}
			resolved, err := access.ResolveUserAccess(ctx, actorID)
			if err != nil {
				return nil, mapError(err)
			}
			out := &PermissionCodesOutput{}
			out.Body.Message = "success"
			out.Body.Data = resolved.Permissions
			return out, nil
		})
}

func registerRoleRoutes(api huma.API, access *rbacService.AccessService) {
	huma.Register(api, operation("rbacListRoles", http.MethodGet, "/api/v1/addons/rbac/roles", "List roles"),
		func(ctx context.Context, _ *AuthHeader) (*RolesOutput, error) {
			roles, err := access.ListRoles(ctx)
			if err != nil {
				return nil, mapError(err)
			}
			out := &RolesOutput{}
			out.Body.Message = "success"
			out.Body.Data = roleViews(roles)
			return out, nil
		})

	huma.Register(api, operation("rbacCreateRole", http.MethodPost, "/api/v1/addons/rbac/roles", "Create role"),
		func(ctx context.Context, input *RoleCreateInput) (*RoleOutput, error) {
			actorID, err := actorFromHeader(input.Authorization)
			if err != nil {
				return nil, err
			}
			role, err := access.CreateRole(ctx, actorID, input.Body.Code, input.Body.Name, input.Body.Description, input.RequestID)
			if err != nil {
				return nil, mapError(err)
			}
			out := &RoleOutput{}
			out.Body.Message = "created"
			view := roleView(*role)
			out.Body.Data = &view
			return out, nil
		})

	huma.Register(api, operation("rbacUpdateRole", http.MethodPatch, "/api/v1/addons/rbac/roles/{roleId}", "Update role"),
		func(ctx context.Context, input *RoleUpdateInput) (*RoleOutput, error) {
			actorID, err := actorFromHeader(input.Authorization)
			if err != nil {
				return nil, err
			}
			role, err := access.UpdateRole(ctx, actorID, input.RoleID, rbacService.RolePatch{
				Name: input.Body.Name, Description: input.Body.Description, IsEnabled: input.Body.IsEnabled,
			}, input.RequestID)
			if err != nil {
				return nil, mapError(err)
			}
			out := &RoleOutput{}
			out.Body.Message = "updated"
			view := roleView(*role)
			out.Body.Data = &view
			return out, nil
		})

	huma.Register(api, operation("rbacDeleteRole", http.MethodDelete, "/api/v1/addons/rbac/roles/{roleId}", "Delete role"),
		func(ctx context.Context, input *RoleMutationInput) (*EmptyOutput, error) {
			actorID, err := actorFromHeader(input.Authorization)
			if err != nil {
				return nil, err
			}
			if err := access.DeleteRole(ctx, actorID, input.RoleID, input.RequestID); err != nil {
				return nil, mapError(err)
			}
			return empty("deleted"), nil
		})

	huma.Register(api, operation("rbacListPermissions", http.MethodGet, "/api/v1/addons/rbac/permission-catalog", "List permissions"),
		func(ctx context.Context, _ *AuthHeader) (*PermissionsOutput, error) {
			permissions, err := access.ListPermissions(ctx)
			if err != nil {
				return nil, mapError(err)
			}
			out := &PermissionsOutput{}
			out.Body.Message = "success"
			out.Body.Data = permissions
			return out, nil
		})

	huma.Register(api, operation("rbacGetRolePermissions", http.MethodGet, "/api/v1/addons/rbac/roles/{roleId}/permissions", "Get role permissions"),
		func(ctx context.Context, input *RoleMutationInput) (*PermissionsOutput, error) {
			permissions, err := access.GetRolePermissions(ctx, input.RoleID)
			if err != nil {
				return nil, mapError(err)
			}
			out := &PermissionsOutput{}
			out.Body.Message = "success"
			out.Body.Data = permissions
			return out, nil
		})

	huma.Register(api, operation("rbacSetRolePermissions", http.MethodPut, "/api/v1/addons/rbac/roles/{roleId}/permissions", "Set role permissions"),
		func(ctx context.Context, input *RolePermissionsInput) (*EmptyOutput, error) {
			actorID, err := actorFromHeader(input.Authorization)
			if err != nil {
				return nil, err
			}
			if err := access.SetRolePermissions(ctx, actorID, input.RoleID, input.Body.PermissionIDs, input.RequestID); err != nil {
				return nil, mapError(err)
			}
			return empty("updated"), nil
		})
}

func registerMenuRoutes(api huma.API, access *rbacService.AccessService) {
	huma.Register(api, operation("rbacListMenus", http.MethodGet, "/api/v1/addons/rbac/menus", "List menus"),
		func(ctx context.Context, _ *AuthHeader) (*MenusOutput, error) {
			menus, err := access.ListMenus(ctx)
			if err != nil {
				return nil, mapError(err)
			}
			out := &MenusOutput{}
			out.Body.Message = "success"
			out.Body.Data = menus
			return out, nil
		})
	huma.Register(api, operation("rbacCreateMenu", http.MethodPost, "/api/v1/addons/rbac/menus", "Create menu"),
		func(ctx context.Context, input *MenuCreateInput) (*MenuOutput, error) {
			actorID, err := actorFromHeader(input.Authorization)
			if err != nil {
				return nil, err
			}
			menu, err := access.CreateMenu(ctx, actorID, toMenuInput(input.Body), input.RequestID)
			if err != nil {
				return nil, mapError(err)
			}
			out := &MenuOutput{}
			out.Body.Message = "created"
			out.Body.Data = menu
			return out, nil
		})
	huma.Register(api, operation("rbacUpdateMenu", http.MethodPatch, "/api/v1/addons/rbac/menus/{menuId}", "Update menu"),
		func(ctx context.Context, input *MenuUpdateInput) (*MenuOutput, error) {
			actorID, err := actorFromHeader(input.Authorization)
			if err != nil {
				return nil, err
			}
			menu, err := access.UpdateMenu(ctx, actorID, input.MenuID, toMenuInput(input.Body), input.RequestID)
			if err != nil {
				return nil, mapError(err)
			}
			out := &MenuOutput{}
			out.Body.Message = "updated"
			out.Body.Data = menu
			return out, nil
		})
	huma.Register(api, operation("rbacDeleteMenu", http.MethodDelete, "/api/v1/addons/rbac/menus/{menuId}", "Delete menu"),
		func(ctx context.Context, input *MenuDeleteInput) (*EmptyOutput, error) {
			actorID, err := actorFromHeader(input.Authorization)
			if err != nil {
				return nil, err
			}
			if err := access.DeleteMenu(ctx, actorID, input.MenuID, input.RequestID); err != nil {
				return nil, mapError(err)
			}
			return empty("deleted"), nil
		})
}

func registerUserRoutes(api huma.API, access *rbacService.AccessService) {
	huma.Register(api, operation("rbacListUsers", http.MethodGet, "/api/v1/addons/rbac/users", "List users"),
		func(ctx context.Context, input *UserListInput) (*UsersOutput, error) {
			page, err := access.ListUsers(ctx, input.Page, input.PageSize, input.Search)
			if err != nil {
				return nil, mapError(err)
			}
			out := &UsersOutput{}
			out.Body.Message = "success"
			out.Body.Data = page
			return out, nil
		})
	huma.Register(api, operation("rbacSetUserRoles", http.MethodPut, "/api/v1/addons/rbac/users/{userId}/roles", "Set user roles"),
		func(ctx context.Context, input *UserRolesInput) (*EmptyOutput, error) {
			actorID, err := actorFromHeader(input.Authorization)
			if err != nil {
				return nil, err
			}
			if err := access.SetUserRoles(ctx, actorID, input.UserID, input.Body.RoleIDs, input.RequestID); err != nil {
				return nil, mapError(err)
			}
			return empty("updated"), nil
		})
}

func registerAuditRoutes(api huma.API, access *rbacService.AccessService) {
	huma.Register(api, operation("rbacListAudit", http.MethodGet, "/api/v1/addons/rbac/audit", "List access audit"),
		func(ctx context.Context, input *AuditListInput) (*AuditOutput, error) {
			page, err := access.ListAudit(ctx, input.Page, input.PageSize)
			if err != nil {
				return nil, mapError(err)
			}
			out := &AuditOutput{}
			out.Body.Message = "success"
			out.Body.Data = page
			return out, nil
		})
}

func operation(id, method, path, summary string) huma.Operation {
	return huma.Operation{
		OperationID: id, Method: method, Path: path, Summary: summary, Tags: []string{"RBAC"},
		Security: []map[string][]string{{"AuthTokenAuth": {}}},
	}
}

func actorFromHeader(header string) (uint, error) {
	token := header
	if len(token) > 7 && token[:7] == "Bearer " {
		token = token[7:]
	}
	claims, err := (&authService.JwtService{}).ParseAccessToken(token)
	if err != nil || claims.UserID == 0 || claims.IsServicePrincipal() || claims.SessionID == "" {
		return 0, huma.NewError(http.StatusUnauthorized, "invalid access token")
	}
	return claims.UserID, nil
}

func toMenuInput(body MenuBody) rbacService.MenuInput {
	return rbacService.MenuInput{
		ParentID: body.ParentID, Code: body.Code, Title: body.Title, TitleKey: body.TitleKey,
		Path: body.Path, Icon: body.Icon, App: body.App, Type: body.Type,
		PermissionCode: body.PermissionCode, Sort: body.Sort, IsVisible: body.IsVisible,
	}
}

func empty(message string) *EmptyOutput {
	out := &EmptyOutput{}
	out.Body.Message = message
	return out
}

func roleViews(roles []model.Role) []RoleView {
	views := make([]RoleView, 0, len(roles))
	for _, role := range roles {
		views = append(views, roleView(role))
	}
	return views
}

func roleView(role model.Role) RoleView {
	return RoleView{
		ID: role.ID, RoleID: role.RoleID, Code: role.Code,
		Name: role.RoleName, RoleName: role.RoleName, Description: role.Description,
		IsSystem: role.IsSystem, IsEnabled: role.IsEnabled, ParentID: role.ParentID,
		DefaultRouter: role.DefaultRouter, DataScope: role.DataScope,
		CreatedAt: role.CreatedAt, UpdatedAt: role.UpdatedAt,
	}
}

func mapError(err error) error {
	switch {
	case errors.Is(err, rbacService.ErrAccessDenied):
		return huma.NewError(http.StatusForbidden, err.Error())
	case errors.Is(err, rbacService.ErrInvalidInput), errors.Is(err, rbacService.ErrMenuCycle):
		return huma.NewError(http.StatusBadRequest, err.Error())
	case errors.Is(err, rbacService.ErrUnknownRole), errors.Is(err, rbacService.ErrUnknownPermission), errors.Is(err, gorm.ErrRecordNotFound):
		return huma.NewError(http.StatusNotFound, err.Error())
	case errors.Is(err, rbacService.ErrProtectedRole), errors.Is(err, rbacService.ErrProtectedMenu), errors.Is(err, rbacService.ErrSelfLockout), errors.Is(err, rbacService.ErrLastSuperAdmin):
		return huma.NewError(http.StatusForbidden, err.Error())
	case errors.Is(err, rbacService.ErrRoleInUse), errors.Is(err, rbacService.ErrCodeConflict):
		return huma.NewError(http.StatusConflict, err.Error())
	default:
		return huma.NewError(http.StatusInternalServerError, "access management request failed")
	}
}
