package service

import (
	"encoding/json"

	"whitestone.top/prism-fusion/addons/rbac/model"
	"whitestone.top/prism-fusion/global"
)

// RoleService 角色服务
type RoleService struct{}

// RoleInfo 角色信息（API 返回用）
type RoleInfo struct {
	RoleID        uint   `json:"roleId"`
	RoleName      string `json:"roleName"`
	DefaultRouter string `json:"defaultRouter"`
}

// GetRoleList 获取角色列表
func (s *RoleService) GetRoleList() ([]model.Role, error) {
	var roles []model.Role
	if err := global.PRISM_DB.Find(&roles).Error; err != nil {
		return nil, err
	}
	return roles, nil
}

// GetRolePermissions 获取角色的权限标识列表
func (s *RoleService) GetRolePermissions(roleID uint) ([]string, error) {
	var codes []string
	err := global.PRISM_DB.Model(&model.RolePermission{}).
		Joins("JOIN permissions ON permissions.id = role_permissions.permission_id").
		Where("role_permissions.role_id = ?", roleID).
		Pluck("permissions.code", &codes).Error
	if err != nil {
		return nil, err
	}

	// 超级管理员拥有所有权限
	if roleID == 999 {
		return []string{"*:*:*"}, nil
	}

	return codes, nil
}

// MenuService 菜单服务
type MenuService struct{}

// MenuNode 菜单树节点（返回给前端的格式）
type MenuNode struct {
	Path      string                 `json:"path"`
	Name      string                 `json:"name,omitempty"`
	Component string                 `json:"component,omitempty"`
	Redirect  string                 `json:"redirect,omitempty"`
	Meta      map[string]interface{} `json:"meta"`
	Children  []MenuNode             `json:"children,omitempty"`
}

// GetAsyncRoutes 获取动态路由（按角色过滤）
func (s *MenuService) GetAsyncRoutes() ([]MenuNode, error) {
	var menus []model.Menu
	if err := global.PRISM_DB.Order("rank asc, id asc").Find(&menus).Error; err != nil {
		return nil, err
	}

	// 构建菜单树
	return buildMenuTree(menus, 0), nil
}

// buildMenuTree 递归构建菜单树
func buildMenuTree(menus []model.Menu, parentID uint) []MenuNode {
	var nodes []MenuNode
	for _, m := range menus {
		if m.ParentID != parentID {
			continue
		}

		node := MenuNode{
			Path:      m.Path,
			Name:      m.Name,
			Component: m.Component,
			Redirect:  m.Redirect,
			Meta:      make(map[string]interface{}),
		}

		node.Meta["title"] = m.Title
		if m.Icon != "" {
			node.Meta["icon"] = m.Icon
		}
		if m.Rank > 0 {
			node.Meta["rank"] = m.Rank
		}
		if m.ShowLink != nil && !*m.ShowLink {
			node.Meta["showLink"] = false
		}

		// 解析 roles JSON
		if m.Roles != "" {
			var roles []string
			if err := json.Unmarshal([]byte(m.Roles), &roles); err == nil && len(roles) > 0 {
				node.Meta["roles"] = roles
			}
		}

		// 解析 auths JSON
		if m.Auths != "" {
			var auths []string
			if err := json.Unmarshal([]byte(m.Auths), &auths); err == nil && len(auths) > 0 {
				node.Meta["auths"] = auths
			}
		}

		// 递归子菜单
		children := buildMenuTree(menus, m.ID)
		if len(children) > 0 {
			node.Children = children
		}

		nodes = append(nodes, node)
	}
	return nodes
}

// SeedData 初始化 RBAC 种子数据
func SeedData() {
	db := global.PRISM_DB

	// 初始化角色
	var roleCount int64
	db.Model(&model.Role{}).Count(&roleCount)
	if roleCount == 0 {
		roles := []model.Role{
			{RoleID: 1, RoleName: "普通用户", ParentID: 0, DefaultRouter: "dashboard"},
			{RoleID: 999, RoleName: "超级管理员", ParentID: 0, DefaultRouter: "dashboard"},
		}
		for _, r := range roles {
			db.Create(&r)
		}
		global.PRISM_LOG.Info("初始化角色数据完成")
	}
}
