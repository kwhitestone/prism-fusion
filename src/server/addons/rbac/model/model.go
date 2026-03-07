package model

import (
	"gorm.io/gorm"
)

// Role 角色表
type Role struct {
	ID            uint           `json:"id" gorm:"primarykey;comment:主键ID"`
	RoleID        uint           `json:"roleId" gorm:"column:role_id;not null;uniqueIndex;comment:角色ID"`
	RoleName      string         `json:"roleName" gorm:"column:role_name;comment:角色名称"`
	ParentID      uint           `json:"parentId" gorm:"column:parent_id;comment:父角色ID"`
	DefaultRouter string         `json:"defaultRouter" gorm:"column:default_router;default:dashboard;comment:默认菜单"`
	DataScope     string         `json:"dataScope" gorm:"column:data_scope;comment:数据权限"`
	DeletedAt     gorm.DeletedAt `json:"-" gorm:"index;comment:删除时间"`
}

// TableName 指定表名
func (Role) TableName() string {
	return "roles"
}

// Permission 权限表
type Permission struct {
	ID          uint           `json:"id" gorm:"primarykey;comment:主键ID"`
	Name        string         `json:"name" gorm:"column:name;size:100;comment:权限名称"`
	Code        string         `json:"code" gorm:"column:code;size:100;uniqueIndex;comment:权限标识"`
	Description string         `json:"description" gorm:"column:description;comment:权限描述"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index;comment:删除时间"`
}

// TableName 指定表名
func (Permission) TableName() string {
	return "permissions"
}

// RolePermission 角色权限关联表
type RolePermission struct {
	ID           uint `json:"id" gorm:"primarykey"`
	RoleID       uint `json:"roleId" gorm:"column:role_id;index;comment:角色ID"`
	PermissionID uint `json:"permissionId" gorm:"column:permission_id;index;comment:权限ID"`
}

// TableName 指定表名
func (RolePermission) TableName() string {
	return "role_permissions"
}

// Menu 菜单表（后端动态路由）
type Menu struct {
	ID        uint           `json:"id" gorm:"primarykey;comment:主键ID"`
	ParentID  uint           `json:"parentId" gorm:"column:parent_id;comment:父菜单ID"`
	Path      string         `json:"path" gorm:"column:path;comment:路由路径"`
	Name      string         `json:"name" gorm:"column:name;comment:路由名称"`
	Component string         `json:"component" gorm:"column:component;comment:组件路径"`
	Redirect  string         `json:"redirect" gorm:"column:redirect;comment:重定向路径"`
	Title     string         `json:"title" gorm:"column:title;comment:菜单标题"`
	Icon      string         `json:"icon" gorm:"column:icon;comment:菜单图标"`
	Rank      int            `json:"rank" gorm:"column:rank;comment:排序"`
	ShowLink  *bool          `json:"showLink" gorm:"column:show_link;default:true;comment:是否显示"`
	Roles     string         `json:"roles" gorm:"column:roles;type:text;comment:允许的角色(JSON数组)"`
	Auths     string         `json:"auths" gorm:"column:auths;type:text;comment:按钮权限(JSON数组)"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index;comment:删除时间"`
}

// TableName 指定表名
func (Menu) TableName() string {
	return "menus"
}
