package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	authModel "github.com/kwhitestone/prism-fusion/addons/auth/model"
	"github.com/kwhitestone/prism-fusion/addons/rbac/model"
	"github.com/kwhitestone/prism-fusion/global"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrAccessDenied         = errors.New("access denied")
	ErrInvalidInput         = errors.New("invalid input")
	ErrUnknownRole          = errors.New("unknown role")
	ErrUnknownPermission    = errors.New("unknown permission")
	ErrProtectedRole        = errors.New("protected role")
	ErrSelfLockout          = errors.New("operation would remove the actor's management access")
	ErrLastSuperAdmin       = errors.New("at least one super administrator is required")
	ErrRoleInUse            = errors.New("role is assigned to users")
	ErrMenuCycle            = errors.New("menu hierarchy contains a cycle")
	ErrProtectedMenu        = errors.New("access management menu is protected")
	ErrCodeConflict         = errors.New("code already exists")
	ErrReservedRoleConflict = errors.New("reserved system role conflicts with existing data")
)

var (
	roleCodePattern       = regexp.MustCompile(`^[a-z][a-z0-9_]{1,63}$`)
	menuCodePattern       = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,63}$`)
	appIDPattern          = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9._-]{0,62}[a-z0-9])?$`)
	permissionPartPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
)

func validPermissionCode(code string) bool {
	if len(code) == 0 || len(code) > 128 {
		return false
	}
	parts := strings.Split(code, ":")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if !permissionPartPattern.MatchString(part) {
			return false
		}
	}
	return true
}

const (
	DefaultUserRoleID  uint = 1
	AdminRoleID        uint = 900
	SuperAdminRoleID   uint = 999
	permissionBatchMax      = 1000
)

type AccessService struct {
	db *gorm.DB
}

type UserAccess struct {
	Roles       []string     `json:"roles"`
	Permissions []string     `json:"permissions"`
	Menus       []model.Menu `json:"menus"`
}

type UserSummary struct {
	ID        uint     `json:"id"`
	Username  string   `json:"username"`
	NickName  string   `json:"nickName"`
	Email     string   `json:"email"`
	Enable    int      `json:"enable"`
	RoleIDs   []uint   `json:"roleIds"`
	RoleCodes []string `json:"roleCodes"`
}

type UserPage struct {
	Items    []UserSummary `json:"items"`
	Total    int64         `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"pageSize"`
}

type AuditPage struct {
	Items    []model.AuditLog `json:"items"`
	Total    int64            `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"pageSize"`
}

type RolePatch struct {
	Name        *string
	Description *string
	IsEnabled   *bool
}

type MenuInput struct {
	ParentID       uint
	Code           string
	Title          string
	TitleKey       string
	Path           string
	Icon           string
	App            string
	Type           string
	PermissionCode string
	Sort           int
	IsVisible      bool
}

// ResolveConfiguredUserAccess keeps builtin and external-table RBAC consumers
// strongly consistent while allowing Prism applications to disable RBAC
// without requiring the RBAC schema to exist.
func ResolveConfiguredUserAccess(ctx context.Context, db *gorm.DB, userID uint, legacyRoleID ...uint) (*UserAccess, error) {
	provider := strings.TrimSpace(global.PRISM_CONFIG.RBAC.Provider)
	if provider != "" && provider != "builtin" && provider != "external" {
		roles := []string{}
		if len(legacyRoleID) > 0 && legacyRoleID[0] != 0 {
			roles = []string{"user"}
			if legacyRoleID[0] == SuperAdminRoleID {
				roles = []string{"admin"}
			}
		}
		return &UserAccess{Roles: roles, Permissions: []string{}, Menus: []model.Menu{}}, nil
	}
	return NewAccessService(db).ResolveUserAccess(ctx, userID)
}

func NewAccessService(db *gorm.DB) *AccessService {
	return &AccessService{db: db}
}

func HasPermission(granted []string, required string) bool {
	requiredParts := strings.Split(strings.TrimSpace(required), ":")
	if len(requiredParts) != 3 {
		return false
	}
	for _, candidate := range granted {
		parts := strings.Split(strings.TrimSpace(candidate), ":")
		if len(parts) != len(requiredParts) {
			continue
		}
		matched := true
		for i := range parts {
			if parts[i] != "*" && parts[i] != requiredParts[i] {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func (s *AccessService) ResolveUserAccess(ctx context.Context, userID uint) (*UserAccess, error) {
	access, err := s.ResolveUserAuthorization(ctx, userID)
	if err != nil {
		return nil, err
	}
	menus, err := s.filterMenus(ctx, access.Permissions, false)
	if err != nil {
		return nil, err
	}
	access.Menus = menus
	return access, nil
}

// ResolveUserAuthorization resolves only roles and permissions. Request
// middleware uses this lightweight path; menus are loaded only by endpoints
// that actually return navigation data.
func (s *AccessService) ResolveUserAuthorization(ctx context.Context, userID uint) (*UserAccess, error) {
	if s == nil || s.db == nil || userID == 0 {
		return nil, ErrAccessDenied
	}
	roleIDs, roles, err := s.resolveRoles(ctx, userID)
	if err != nil {
		return nil, err
	}
	permissions, err := s.permissionsForRoleIDs(ctx, roleIDs, roles)
	if err != nil {
		return nil, err
	}
	roleCodes := make([]string, 0, len(roles))
	for _, role := range roles {
		roleCodes = append(roleCodes, role.Code)
	}
	sort.Strings(roleCodes)
	return &UserAccess{Roles: roleCodes, Permissions: permissions, Menus: []model.Menu{}}, nil
}

func (s *AccessService) ResolveVisibleMenus(ctx context.Context, userID uint) ([]model.Menu, error) {
	access, err := s.ResolveUserAccess(ctx, userID)
	if err != nil {
		return nil, err
	}
	visible := make([]model.Menu, 0, len(access.Menus))
	for _, menu := range access.Menus {
		if menu.IsVisible == nil || *menu.IsVisible {
			visible = append(visible, menu)
		}
	}
	return visible, nil
}

func (s *AccessService) ResolveAsyncRoutes(ctx context.Context, userID uint) ([]MenuNode, error) {
	menus, err := s.ResolveVisibleMenus(ctx, userID)
	if err != nil {
		return nil, err
	}
	routes := buildMenuTree(routableMenus(menus), 0)
	if routes == nil {
		routes = []MenuNode{}
	}
	return routes, nil
}

func routableMenus(menus []model.Menu) []model.Menu {
	routable := make([]model.Menu, 0, len(menus))
	for _, menu := range menus {
		// Empty type is retained only for pre-migration compatibility. New and
		// migrated data uses group/menu; buttons are permission metadata, not routes.
		if menu.Type == "" || menu.Type == "group" || menu.Type == "menu" {
			routable = append(routable, menu)
		}
	}
	return routable
}

func (s *AccessService) resolveRoles(ctx context.Context, userID uint) ([]uint, []model.Role, error) {
	var roles []model.Role
	err := s.db.WithContext(ctx).
		Model(&model.Role{}).
		Joins("JOIN user_roles ON user_roles.role_id = roles.role_id").
		Where("user_roles.user_id = ? AND roles.is_enabled = ?", userID, true).
		Order("roles.role_id ASC").
		Find(&roles).Error
	if err != nil {
		return nil, nil, err
	}
	if len(roles) == 0 {
		var user authModel.User
		if err := s.db.WithContext(ctx).Select("id", "role_id").First(&user, userID).Error; err != nil {
			return nil, nil, err
		}
		if user.RoleID != 0 {
			if err := s.db.WithContext(ctx).Where("role_id = ? AND is_enabled = ?", user.RoleID, true).Find(&roles).Error; err != nil {
				return nil, nil, err
			}
		}
	}
	roleIDs := make([]uint, 0, len(roles))
	for _, role := range roles {
		roleIDs = append(roleIDs, role.RoleID)
	}
	return roleIDs, roles, nil
}

func (s *AccessService) permissionsForRoleIDs(ctx context.Context, roleIDs []uint, roles []model.Role) ([]string, error) {
	if len(roleIDs) == 0 {
		return []string{}, nil
	}
	for _, role := range roles {
		if role.Code == "super_admin" && role.IsSystem && role.IsEnabled {
			return []string{"*:*:*"}, nil
		}
	}
	var permissions []string
	err := s.db.WithContext(ctx).
		Model(&model.Permission{}).
		Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id").
		Where("role_permissions.role_id IN ?", roleIDs).
		Distinct("permissions.code").
		Order("permissions.code ASC").
		Pluck("permissions.code", &permissions).Error
	if err != nil {
		return nil, err
	}
	if permissions == nil {
		permissions = []string{}
	}
	return permissions, nil
}

func (s *AccessService) filterMenus(ctx context.Context, permissions []string, visibleOnly bool) ([]model.Menu, error) {
	query := s.db.WithContext(ctx).Order("sort ASC, id ASC")
	if visibleOnly {
		query = query.Where("is_visible = ?", true)
	}
	var all []model.Menu
	if err := query.Find(&all).Error; err != nil {
		return nil, err
	}
	filtered := make([]model.Menu, 0, len(all))
	for _, menu := range all {
		if matchesAnyPermission(permissions, menu.PermissionCode) {
			filtered = append(filtered, menu)
		}
	}
	return filtered, nil
}

func matchesAnyPermission(granted []string, expression string) bool {
	if strings.TrimSpace(expression) == "" {
		return true
	}
	for _, required := range strings.Split(expression, "|") {
		if HasPermission(granted, strings.TrimSpace(required)) {
			return true
		}
	}
	return false
}

func (s *AccessService) ListRoles(ctx context.Context) ([]model.Role, error) {
	var roles []model.Role
	err := s.db.WithContext(ctx).Order("role_id ASC").Find(&roles).Error
	return roles, err
}

func (s *AccessService) ListPermissions(ctx context.Context) ([]model.Permission, error) {
	var permissions []model.Permission
	err := s.db.WithContext(ctx).Order("module ASC, code ASC").Find(&permissions).Error
	return permissions, err
}

func (s *AccessService) GetRolePermissions(ctx context.Context, roleID uint) ([]model.Permission, error) {
	if err := s.requireRole(ctx, roleID); err != nil {
		return nil, err
	}
	var permissions []model.Permission
	err := s.db.WithContext(ctx).
		Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id").
		Where("role_permissions.role_id = ?", roleID).
		Order("permissions.module ASC, permissions.code ASC").
		Find(&permissions).Error
	return permissions, err
}

func (s *AccessService) CreateRole(ctx context.Context, actorID uint, code, name, description, requestID string) (*model.Role, error) {
	code = strings.TrimSpace(code)
	name = strings.TrimSpace(name)
	if !roleCodePattern.MatchString(code) || !validText(name, 1, 64) || !validText(description, 0, 255) {
		return nil, ErrInvalidInput
	}
	role := &model.Role{Code: code, RoleName: name, Description: strings.TrimSpace(description), IsEnabled: true}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// The immutable admin row is a database-backed allocation mutex. This
		// keeps concurrent creators from choosing the same stable role_id.
		var allocationLock model.Role
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("role_id = ? AND is_system = ?", AdminRoleID, true).First(&allocationLock).Error; err != nil {
			return translateNotFound(err, ErrReservedRoleConflict)
		}
		var codeCount int64
		if err := tx.Unscoped().Model(&model.Role{}).Where("code = ?", role.Code).Count(&codeCount).Error; err != nil {
			return err
		}
		if codeCount > 0 {
			return ErrCodeConflict
		}
		var used []uint
		if err := tx.Unscoped().Model(&model.Role{}).Pluck("role_id", &used).Error; err != nil {
			return err
		}
		occupied := make(map[uint]struct{}, len(used))
		for _, roleID := range used {
			occupied[roleID] = struct{}{}
		}
		for candidate := uint(2); candidate < SuperAdminRoleID; candidate++ {
			if _, exists := occupied[candidate]; !exists {
				role.RoleID = candidate
				break
			}
		}
		if role.RoleID == 0 {
			return ErrInvalidInput
		}
		if err := tx.Create(role).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return ErrCodeConflict
			}
			return err
		}
		return appendAudit(tx, actorID, "role.create", fmt.Sprintf("role:%d", role.RoleID), map[string]any{"code": role.Code}, requestID)
	})
	return role, err
}

func (s *AccessService) UpdateRole(ctx context.Context, actorID, roleID uint, patch RolePatch, requestID string) (*model.Role, error) {
	updates := map[string]any{}
	if patch.Name != nil {
		name := strings.TrimSpace(*patch.Name)
		if !validText(name, 1, 64) {
			return nil, ErrInvalidInput
		}
		updates["role_name"] = name
	}
	if patch.Description != nil {
		description := strings.TrimSpace(*patch.Description)
		if !validText(description, 0, 255) {
			return nil, ErrInvalidInput
		}
		updates["description"] = description
	}
	if patch.IsEnabled != nil {
		updates["is_enabled"] = *patch.IsEnabled
	}
	authorizationMutation.Lock()
	defer authorizationMutation.Unlock()
	var role model.Role
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockAuthorizationActor(tx, actorID); err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("role_id = ?", roleID).First(&role).Error; err != nil {
			return translateNotFound(err, ErrUnknownRole)
		}
		if role.IsSystem {
			return ErrProtectedRole
		}
		txAccess := NewAccessService(tx)
		if err := txAccess.requireRoleWithinCeiling(ctx, actorID, roleID); err != nil {
			return err
		}
		if patch.IsEnabled != nil && !*patch.IsEnabled {
			currentRoleIDs, currentRoles, err := txAccess.resolveRoles(ctx, actorID)
			if err != nil {
				return err
			}
			if containsUint(currentRoleIDs, roleID) {
				remainingIDs := make([]uint, 0, len(currentRoleIDs)-1)
				remainingRoles := make([]model.Role, 0, len(currentRoles)-1)
				for _, currentRole := range currentRoles {
					if currentRole.RoleID != roleID {
						remainingIDs = append(remainingIDs, currentRole.RoleID)
						remainingRoles = append(remainingRoles, currentRole)
					}
				}
				remainingPermissions, permissionErr := txAccess.permissionsForRoleIDs(ctx, remainingIDs, remainingRoles)
				if permissionErr != nil {
					return permissionErr
				}
				if !HasPermission(remainingPermissions, "auth:role:write") {
					return ErrSelfLockout
				}
			}
		}
		if len(updates) == 0 {
			return nil
		}
		if err := tx.Model(&model.Role{}).Where("role_id = ?", roleID).Updates(updates).Error; err != nil {
			return err
		}
		if err := revokeRoleSessions(tx, roleID); err != nil {
			return err
		}
		return appendAudit(tx, actorID, "role.update", fmt.Sprintf("role:%d", roleID), updates, requestID)
	})
	if err == nil {
		if name, ok := updates["role_name"].(string); ok {
			role.RoleName = name
		}
		if description, ok := updates["description"].(string); ok {
			role.Description = description
		}
		if enabled, ok := updates["is_enabled"].(bool); ok {
			role.IsEnabled = enabled
		}
	}
	return &role, err
}

func (s *AccessService) DeleteRole(ctx context.Context, actorID, roleID uint, requestID string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var role model.Role
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("role_id = ?", roleID).First(&role).Error; err != nil {
			return translateNotFound(err, ErrUnknownRole)
		}
		if role.IsSystem {
			return ErrProtectedRole
		}
		var assignments int64
		if err := tx.Model(&model.UserRole{}).Where("role_id = ?", roleID).Count(&assignments).Error; err != nil {
			return err
		}
		var legacyAssignments int64
		if err := tx.Model(&authModel.User{}).Where("role_id = ?", roleID).Count(&legacyAssignments).Error; err != nil {
			return err
		}
		if assignments > 0 || legacyAssignments > 0 {
			return ErrRoleInUse
		}
		if err := NewAccessService(tx).requireRoleWithinCeiling(ctx, actorID, roleID); err != nil {
			return err
		}
		if err := tx.Where("role_id = ?", roleID).Delete(&model.RolePermission{}).Error; err != nil {
			return err
		}
		if err := tx.Where("role_id = ?", roleID).Delete(&model.Role{}).Error; err != nil {
			return err
		}
		return appendAudit(tx, actorID, "role.delete", fmt.Sprintf("role:%d", roleID), map[string]any{"code": role.Code}, requestID)
	})
}

func (s *AccessService) SetRolePermissions(ctx context.Context, actorID, roleID uint, permissionIDs []uint, requestIDs ...string) error {
	if len(permissionIDs) > permissionBatchMax {
		return ErrInvalidInput
	}
	ids := uniqueUint(permissionIDs)
	if len(ids) != len(permissionIDs) {
		return ErrInvalidInput
	}
	authorizationMutation.Lock()
	defer authorizationMutation.Unlock()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockAuthorizationActor(tx, actorID); err != nil {
			return err
		}
		var role model.Role
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("role_id = ?", roleID).First(&role).Error; err != nil {
			return translateNotFound(err, ErrUnknownRole)
		}
		if role.IsSystem {
			return ErrProtectedRole
		}
		var count int64
		if len(ids) > 0 {
			if err := tx.Model(&model.Permission{}).Where("id IN ?", ids).Count(&count).Error; err != nil {
				return err
			}
		}
		if count != int64(len(ids)) {
			return ErrUnknownPermission
		}
		txAccess := NewAccessService(tx)
		if err := txAccess.requireRoleWithinCeiling(ctx, actorID, roleID); err != nil {
			return err
		}
		projected, err := txAccess.permissionCodesForIDs(ctx, ids)
		if err != nil {
			return err
		}
		if err := txAccess.requireGrantCeiling(ctx, actorID, projected); err != nil {
			return err
		}
		actorRoleIDs, _, err := txAccess.resolveRoles(ctx, actorID)
		if err != nil {
			return err
		}
		if containsUint(actorRoleIDs, roleID) && !HasPermission(projected, "auth:role:write") {
			return ErrSelfLockout
		}
		if err := tx.Where("role_id = ?", roleID).Delete(&model.RolePermission{}).Error; err != nil {
			return err
		}
		for _, permissionID := range ids {
			if err := tx.Create(&model.RolePermission{RoleID: roleID, PermissionID: permissionID}).Error; err != nil {
				return err
			}
		}
		if err := revokeRoleSessions(tx, roleID); err != nil {
			return err
		}
		return appendAudit(tx, actorID, "role.permissions.update", fmt.Sprintf("role:%d", roleID), map[string]any{"permissionIds": ids}, firstString(requestIDs))
	})
}

func lockAuthorizationActor(tx *gorm.DB, actorID uint) error {
	if actorID == 0 {
		return ErrAccessDenied
	}
	var actor authModel.User
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").First(&actor, actorID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrAccessDenied
		}
		return err
	}
	return nil
}

func (s *AccessService) requireRole(ctx context.Context, roleID uint) error {
	var count int64
	if err := s.db.WithContext(ctx).Model(&model.Role{}).Where("role_id = ?", roleID).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return ErrUnknownRole
	}
	return nil
}

func (s *AccessService) permissionCodesForIDs(ctx context.Context, ids []uint) ([]string, error) {
	if len(ids) == 0 {
		return []string{}, nil
	}
	var codes []string
	err := s.db.WithContext(ctx).Model(&model.Permission{}).Where("id IN ?", ids).Order("code ASC").Pluck("code", &codes).Error
	return codes, err
}

// requireGrantCeiling prevents a delegated role administrator from creating a
// privilege path above their own current capabilities.
func (s *AccessService) requireGrantCeiling(ctx context.Context, actorID uint, projected []string) error {
	access, err := s.ResolveUserAccess(ctx, actorID)
	if err != nil {
		return ErrAccessDenied
	}
	for _, permission := range projected {
		if !HasPermission(access.Permissions, permission) {
			return ErrAccessDenied
		}
	}
	return nil
}

func (s *AccessService) requireRoleWithinCeiling(ctx context.Context, actorID, roleID uint) error {
	var current []string
	if err := s.db.WithContext(ctx).Model(&model.Permission{}).
		Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id").
		Where("role_permissions.role_id = ?", roleID).
		Distinct("permissions.code").Pluck("permissions.code", &current).Error; err != nil {
		return err
	}
	return s.requireGrantCeiling(ctx, actorID, current)
}

func (s *AccessService) rolesForUsers(ctx context.Context, userIDs []uint) (map[uint][]uint, map[uint][]string, error) {
	ids := make(map[uint][]uint, len(userIDs))
	codes := make(map[uint][]string, len(userIDs))
	if len(userIDs) == 0 {
		return ids, codes, nil
	}
	type row struct {
		UserID uint
		RoleID uint
		Code   string
	}
	var rows []row
	err := s.db.WithContext(ctx).Table("user_roles").
		Select("user_roles.user_id, user_roles.role_id, roles.code").
		Joins("JOIN roles ON roles.role_id = user_roles.role_id").
		Where("user_roles.user_id IN ?", userIDs).Order("user_roles.role_id ASC").Scan(&rows).Error
	if err != nil {
		return nil, nil, err
	}
	for _, row := range rows {
		ids[row.UserID] = append(ids[row.UserID], row.RoleID)
		codes[row.UserID] = append(codes[row.UserID], row.Code)
	}
	return ids, codes, nil
}

func revokeRoleSessions(tx *gorm.DB, roleID uint) error {
	var userIDs []uint
	if err := tx.Model(&model.UserRole{}).Where("role_id = ?", roleID).Distinct("user_id").Pluck("user_id", &userIDs).Error; err != nil {
		return err
	}
	var legacyUserIDs []uint
	if err := tx.Model(&authModel.User{}).Where("role_id = ?", roleID).Distinct("id").Pluck("id", &legacyUserIDs).Error; err != nil {
		return err
	}
	userIDs = uniqueUint(append(userIDs, legacyUserIDs...))
	if len(userIDs) == 0 {
		return nil
	}
	return tx.Model(&authModel.RefreshSession{}).
		Where("user_id IN ? AND revoked_at IS NULL", userIDs).
		Update("revoked_at", time.Now()).Error
}

func revokeUserSessions(tx *gorm.DB, userID uint) error {
	return tx.Model(&authModel.RefreshSession{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Update("revoked_at", time.Now()).Error
}

func appendAudit(tx *gorm.DB, actorID uint, action, target string, detail any, requestID string) error {
	payload := "{}"
	if detail != nil {
		encoded, err := json.Marshal(detail)
		if err != nil {
			return err
		}
		payload = string(encoded)
	}
	return tx.Create(&model.AuditLog{
		ActorID: actorID, Action: action, Target: target,
		Detail: payload, RequestID: strings.TrimSpace(requestID),
	}).Error
}

func validText(value string, min, max int) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) >= min && utf8.RuneCountInString(value) <= max
}

func firstString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func uniqueUint(values []uint) []uint {
	seen := make(map[uint]struct{}, len(values))
	out := make([]uint, 0, len(values))
	for _, value := range values {
		if value == 0 {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func containsUint(values []uint, target uint) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func translateNotFound(err, replacement error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return replacement
	}
	return err
}
