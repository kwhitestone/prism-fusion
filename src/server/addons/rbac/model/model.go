package model

import (
	"time"

	"gorm.io/gorm"
)

// Role 角色表
type Role struct {
	ID            uint           `json:"id" gorm:"primarykey;comment:主键ID"`
	RoleID        uint           `json:"roleId" gorm:"column:role_id;not null;uniqueIndex;comment:稳定角色ID"`
	Code          string         `json:"code" gorm:"column:code;size:64;uniqueIndex;comment:稳定角色编码"`
	RoleName      string         `json:"name" gorm:"column:role_name;size:64;comment:角色名称"`
	Description   string         `json:"description" gorm:"column:description;size:255;comment:角色说明"`
	IsSystem      bool           `json:"isSystem" gorm:"column:is_system;not null;default:false;comment:系统角色"`
	IsEnabled     bool           `json:"isEnabled" gorm:"column:is_enabled;not null;default:true;comment:是否启用"`
	ParentID      uint           `json:"parentId" gorm:"column:parent_id;comment:父角色ID"`
	DefaultRouter string         `json:"defaultRouter" gorm:"column:default_router;default:dashboard;comment:默认菜单"`
	DataScope     string         `json:"dataScope" gorm:"column:data_scope;comment:数据权限"`
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
	DeletedAt     gorm.DeletedAt `json:"-" gorm:"index;comment:删除时间"`
}

// TableName 指定表名
func (Role) TableName() string {
	return "roles"
}

// Permission 权限表
type Permission struct {
	ID          uint           `json:"id" gorm:"primarykey;comment:主键ID"`
	Name        string         `json:"name" gorm:"column:name;size:128;comment:权限名称"`
	Code        string         `json:"code" gorm:"column:code;size:128;uniqueIndex;comment:权限标识"`
	Module      string         `json:"module" gorm:"column:module;size:64;index;comment:权限分组"`
	Description string         `json:"description" gorm:"column:description;size:255;comment:权限描述"`
	IsSystem    bool           `json:"isSystem" gorm:"column:is_system;not null;default:false;comment:系统权限"`
	IsHighRisk  bool           `json:"isHighRisk" gorm:"column:is_high_risk;not null;default:false;comment:高风险权限"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index;comment:删除时间"`
}

// TableName 指定表名
func (Permission) TableName() string {
	return "permissions"
}

// RolePermission 角色权限关联表
type RolePermission struct {
	ID           uint      `json:"id" gorm:"primarykey"`
	RoleID       uint      `json:"roleId" gorm:"column:role_id;not null;uniqueIndex:idx_role_permissions_role_perm;comment:稳定角色ID"`
	PermissionID uint      `json:"permissionId" gorm:"column:permission_id;not null;uniqueIndex:idx_role_permissions_role_perm;comment:权限ID"`
	CreatedAt    time.Time `json:"createdAt"`
}

// TableName 指定表名
func (RolePermission) TableName() string {
	return "role_permissions"
}

// Menu 菜单表（后端动态路由）
type Menu struct {
	ID             uint           `json:"id" gorm:"primarykey;comment:主键ID"`
	ParentID       uint           `json:"parentId" gorm:"column:parent_id;not null;default:0;index;comment:父菜单ID"`
	Code           string         `json:"code" gorm:"column:code;size:64;uniqueIndex;comment:稳定菜单编码"`
	Path           string         `json:"path" gorm:"column:path;size:255;comment:路由路径"`
	Name           string         `json:"name" gorm:"column:name;size:128;comment:路由名称"`
	Component      string         `json:"component" gorm:"column:component;size:255;comment:组件路径"`
	Redirect       string         `json:"redirect" gorm:"column:redirect;size:255;comment:重定向路径"`
	Title          string         `json:"title" gorm:"column:title;size:128;comment:菜单标题"`
	TitleKey       string         `json:"titleKey" gorm:"column:title_key;size:128;comment:国际化标题键"`
	Icon           string         `json:"icon" gorm:"column:icon;size:512;comment:菜单图标"`
	App            string         `json:"app" gorm:"column:app;size:64;comment:所属子应用"`
	Type           string         `json:"type" gorm:"column:type;size:16;default:menu;comment:group/menu/button"`
	PermissionCode string         `json:"permissionCode" gorm:"column:permission_code;size:255;comment:任一权限表达式"`
	Sort           int            `json:"sort" gorm:"column:sort;not null;default:0;comment:排序"`
	IsVisible      *bool          `json:"isVisible" gorm:"column:is_visible;default:true;comment:是否显示"`
	Rank           int            `json:"rank" gorm:"column:rank;comment:旧版排序"`
	ShowLink       *bool          `json:"showLink" gorm:"column:show_link;default:true;comment:旧版显示标记"`
	Roles          string         `json:"roles,omitempty" gorm:"column:roles;type:text;comment:旧版角色JSON"`
	Auths          string         `json:"auths,omitempty" gorm:"column:auths;type:text;comment:旧版按钮权限JSON"`
	CreatedAt      time.Time      `json:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt"`
	DeletedAt      gorm.DeletedAt `json:"-" gorm:"index;comment:删除时间"`
}

// TableName 指定表名
func (Menu) TableName() string {
	return "menus"
}

// UserRole 用户与角色的多对多关系。users.role_id 在迁移期继续作为主角色兼容列。
type UserRole struct {
	ID        uint      `json:"id" gorm:"primarykey"`
	UserID    uint      `json:"userId" gorm:"column:user_id;not null;uniqueIndex:idx_user_roles_user_role"`
	RoleID    uint      `json:"roleId" gorm:"column:role_id;not null;uniqueIndex:idx_user_roles_user_role;index:idx_user_roles_role"`
	CreatedAt time.Time `json:"createdAt"`
}

func (UserRole) TableName() string { return "user_roles" }

// AuditLog 是追加写的授权管理审计记录。Detail 只保存脱敏后的变更摘要。
type AuditLog struct {
	ID        uint64    `json:"id" gorm:"primarykey"`
	ActorID   uint      `json:"actorId" gorm:"column:actor_id;not null;index"`
	Action    string    `json:"action" gorm:"column:action;size:64;not null;index"`
	Target    string    `json:"target" gorm:"column:target;size:255;not null"`
	Detail    string    `json:"detail" gorm:"column:detail;type:text"`
	RequestID string    `json:"requestId" gorm:"column:request_id;size:128;index"`
	CreatedAt time.Time `json:"createdAt" gorm:"index"`
}

func (AuditLog) TableName() string { return "rbac_audit_logs" }
