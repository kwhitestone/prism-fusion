package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	authModel "github.com/kwhitestone/prism-fusion/addons/auth/model"
	"github.com/kwhitestone/prism-fusion/addons/rbac/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	menuMutationMu        sync.Mutex
	authorizationMutation sync.Mutex
)

func (s *AccessService) ListUsers(ctx context.Context, page, pageSize int, search string) (*UserPage, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	query := s.db.WithContext(ctx).Model(&authModel.User{})
	if search = strings.TrimSpace(search); search != "" {
		like := "%" + search + "%"
		query = query.Where("username LIKE ? OR nick_name LIKE ? OR email LIKE ?", like, like, like)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var users []authModel.User
	if err := query.Order("id ASC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&users).Error; err != nil {
		return nil, err
	}
	userIDs := make([]uint, 0, len(users))
	for _, user := range users {
		userIDs = append(userIDs, user.ID)
	}
	roleIDsByUser, roleCodesByUser, err := s.rolesForUsers(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	primaryRoleCodes, err := s.primaryRoleCodes(ctx, users, roleIDsByUser)
	if err != nil {
		return nil, err
	}
	items := make([]UserSummary, 0, len(users))
	for _, user := range users {
		roleIDs := roleIDsByUser[user.ID]
		roleCodes := roleCodesByUser[user.ID]
		if len(roleIDs) == 0 && user.RoleID != 0 {
			roleIDs = []uint{user.RoleID}
			if code, exists := primaryRoleCodes[user.RoleID]; exists {
				roleCodes = []string{code}
			}
		}
		items = append(items, UserSummary{
			ID: user.ID, Username: user.Username, NickName: user.NickName,
			Email: user.Email, Enable: user.Enable, RoleIDs: roleIDs, RoleCodes: roleCodes,
		})
	}
	return &UserPage{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

func (s *AccessService) primaryRoleCodes(ctx context.Context, users []authModel.User, assigned map[uint][]uint) (map[uint]string, error) {
	roleIDs := make([]uint, 0, len(users))
	for _, user := range users {
		if len(assigned[user.ID]) == 0 && user.RoleID != 0 {
			roleIDs = append(roleIDs, user.RoleID)
		}
	}
	roleIDs = uniqueUint(roleIDs)
	codes := make(map[uint]string, len(roleIDs))
	if len(roleIDs) == 0 {
		return codes, nil
	}
	var roles []model.Role
	if err := s.db.WithContext(ctx).Select("role_id", "code").Where("role_id IN ?", roleIDs).Find(&roles).Error; err != nil {
		return nil, err
	}
	for _, role := range roles {
		codes[role.RoleID] = role.Code
	}
	return codes, nil
}

func (s *AccessService) SetUserRoles(ctx context.Context, actorID, targetUserID uint, roleIDs []uint, requestIDs ...string) error {
	if len(roleIDs) == 0 || len(roleIDs) > 100 {
		return ErrInvalidInput
	}
	ids := uniqueUint(roleIDs)
	if len(ids) != len(roleIDs) {
		return ErrInvalidInput
	}
	containsSuper := containsUint(ids, SuperAdminRoleID)
	primaryRoleID := ids[0]
	if containsSuper {
		primaryRoleID = SuperAdminRoleID
	}
	authorizationMutation.Lock()
	defer authorizationMutation.Unlock()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		userIDs := uniqueUint([]uint{actorID, targetUserID})
		sort.Slice(userIDs, func(i, j int) bool { return userIDs[i] < userIDs[j] })
		var lockedUsers []authModel.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id IN ?", userIDs).Order("id ASC").Find(&lockedUsers).Error; err != nil {
			return err
		}
		var target authModel.User
		actorFound, targetFound := false, false
		for _, user := range lockedUsers {
			if user.ID == actorID {
				actorFound = true
			}
			if user.ID == targetUserID {
				target, targetFound = user, true
			}
		}
		if !targetFound {
			return gorm.ErrRecordNotFound
		}
		if !actorFound {
			return ErrAccessDenied
		}
		var roles []model.Role
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("role_id IN ? AND is_enabled = ?", ids, true).Find(&roles).Error; err != nil {
			return err
		}
		if len(roles) != len(ids) {
			return ErrUnknownRole
		}
		txAccess := NewAccessService(tx)
		projected, err := txAccess.permissionsForRoleIDs(ctx, ids, roles)
		if err != nil {
			return err
		}
		if err := txAccess.requireGrantCeiling(ctx, actorID, projected); err != nil {
			return err
		}
		if actorID == targetUserID && !HasPermission(projected, "auth:user-role:write") {
			return ErrSelfLockout
		}
		var currentAssignments []model.UserRole
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ?", targetUserID).Find(&currentAssignments).Error; err != nil {
			return err
		}
		currentIDs := make([]uint, 0, len(currentAssignments)+1)
		for _, assignment := range currentAssignments {
			currentIDs = append(currentIDs, assignment.RoleID)
		}
		if len(currentIDs) == 0 && target.RoleID != 0 {
			currentIDs = append(currentIDs, target.RoleID)
		}
		currentlySuper := containsUint(currentIDs, SuperAdminRoleID)
		if containsSuper || currentlySuper {
			actorAccess, accessErr := txAccess.ResolveUserAuthorization(ctx, actorID)
			if accessErr != nil || !HasPermission(actorAccess.Permissions, "auth:super-admin:grant") {
				return ErrAccessDenied
			}
		}
		if target.Enable == 1 && currentlySuper && !containsSuper {
			activeSuperAdmins, err := activeSuperAdminCount(tx, true)
			if err != nil {
				return err
			}
			if activeSuperAdmins <= 1 {
				return ErrLastSuperAdmin
			}
		}
		if err := tx.Where("user_id = ?", targetUserID).Delete(&model.UserRole{}).Error; err != nil {
			return err
		}
		for _, roleID := range ids {
			if err := tx.Create(&model.UserRole{UserID: targetUserID, RoleID: roleID}).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&authModel.User{}).Where("id = ?", targetUserID).Update("role_id", primaryRoleID).Error; err != nil {
			return err
		}
		if err := revokeUserSessions(tx, targetUserID); err != nil {
			return err
		}
		return appendAudit(tx, actorID, "user.roles.update", fmt.Sprintf("user:%d", targetUserID), map[string]any{"roleIds": ids}, firstString(requestIDs))
	})
}

func activeSuperAdminCount(db *gorm.DB, lock bool) (int, error) {
	query := db.Table("user_roles").
		Select("user_roles.id, user_roles.user_id, user_roles.role_id").
		Joins("JOIN users ON users.id = user_roles.user_id").
		Where("user_roles.role_id = ? AND users.enable = ? AND users.deleted_at IS NULL", SuperAdminRoleID, 1)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var assignments []model.UserRole
	if err := query.Find(&assignments).Error; err != nil {
		return 0, err
	}
	return len(assignments), nil
}

func (s *AccessService) ListMenus(ctx context.Context) ([]model.Menu, error) {
	var menus []model.Menu
	err := s.db.WithContext(ctx).Order("sort ASC, id ASC").Find(&menus).Error
	return menus, err
}

func (s *AccessService) CreateMenu(ctx context.Context, actorID uint, input MenuInput, requestID string) (*model.Menu, error) {
	menuMutationMu.Lock()
	defer menuMutationMu.Unlock()
	visible := input.IsVisible
	menu := &model.Menu{
		ParentID: input.ParentID, Code: strings.TrimSpace(input.Code), Title: strings.TrimSpace(input.Title),
		TitleKey: strings.TrimSpace(input.TitleKey), Path: strings.TrimSpace(input.Path), Icon: strings.TrimSpace(input.Icon),
		App: strings.TrimSpace(input.App), Type: strings.TrimSpace(input.Type), PermissionCode: strings.TrimSpace(input.PermissionCode),
		Sort: input.Sort, IsVisible: &visible,
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockMenuRows(tx); err != nil {
			return err
		}
		if err := NewAccessService(tx).validateMenuInput(ctx, 0, input); err != nil {
			return err
		}
		if err := tx.Create(menu).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return ErrCodeConflict
			}
			return err
		}
		return appendAudit(tx, actorID, "menu.create", fmt.Sprintf("menu:%d", menu.ID), map[string]any{"code": menu.Code}, requestID)
	})
	return menu, err
}

func (s *AccessService) UpdateMenu(ctx context.Context, actorID, menuID uint, input MenuInput, requestID string) (*model.Menu, error) {
	menuMutationMu.Lock()
	defer menuMutationMu.Unlock()
	var updated model.Menu
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockMenuRows(tx); err != nil {
			return err
		}
		var existing model.Menu
		if err := tx.First(&existing, menuID).Error; err != nil {
			return err
		}
		if existing.Code == "account_access" &&
			(strings.TrimSpace(input.Code) != existing.Code ||
				input.ParentID != 0 ||
				strings.TrimSpace(input.Path) != "/account/access" ||
				strings.TrimSpace(input.App) != "auth" ||
				strings.TrimSpace(input.Type) != "menu" ||
				strings.TrimSpace(input.PermissionCode) != "auth:access:read" ||
				!input.IsVisible) {
			return ErrProtectedMenu
		}
		if err := NewAccessService(tx).validateMenuInput(ctx, menuID, input); err != nil {
			return err
		}
		if strings.TrimSpace(input.Type) == "button" {
			var childCount int64
			if err := tx.Model(&model.Menu{}).Where("parent_id = ?", menuID).Count(&childCount).Error; err != nil {
				return err
			}
			if childCount > 0 {
				return ErrInvalidInput
			}
		}
		visible := input.IsVisible
		updated = existing
		updated.ParentID = input.ParentID
		updated.Code = strings.TrimSpace(input.Code)
		updated.Title = strings.TrimSpace(input.Title)
		updated.TitleKey = strings.TrimSpace(input.TitleKey)
		updated.Path = strings.TrimSpace(input.Path)
		updated.Icon = strings.TrimSpace(input.Icon)
		updated.App = strings.TrimSpace(input.App)
		updated.Type = strings.TrimSpace(input.Type)
		updated.PermissionCode = strings.TrimSpace(input.PermissionCode)
		updated.Sort = input.Sort
		updated.IsVisible = &visible
		var codeCount int64
		if err := tx.Unscoped().Model(&model.Menu{}).Where("code = ? AND id <> ?", updated.Code, menuID).Count(&codeCount).Error; err != nil {
			return err
		}
		if codeCount > 0 {
			return ErrCodeConflict
		}
		if err := tx.Save(&updated).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return ErrCodeConflict
			}
			return err
		}
		return appendAudit(tx, actorID, "menu.update", fmt.Sprintf("menu:%d", menuID), map[string]any{"code": updated.Code}, requestID)
	})
	return &updated, err
}

func (s *AccessService) DeleteMenu(ctx context.Context, actorID, menuID uint, requestID string) error {
	if menuID == 0 {
		return ErrInvalidInput
	}
	menuMutationMu.Lock()
	defer menuMutationMu.Unlock()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockMenuRows(tx); err != nil {
			return err
		}
		var menu model.Menu
		if err := tx.Select("id", "code").First(&menu, menuID).Error; err != nil {
			return err
		}
		if menu.Code == "account_access" {
			return ErrProtectedMenu
		}
		var count int64
		if err := tx.Model(&model.Menu{}).Where("parent_id = ?", menuID).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrInvalidInput
		}
		if result := tx.Delete(&model.Menu{}, menuID); result.Error != nil {
			return result.Error
		} else if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return appendAudit(tx, actorID, "menu.delete", fmt.Sprintf("menu:%d", menuID), nil, requestID)
	})
}

func lockMenuRows(tx *gorm.DB) error {
	var menus []model.Menu
	return tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Order("id ASC").Find(&menus).Error
}

func (s *AccessService) ListAudit(ctx context.Context, page, pageSize int) (*AuditPage, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	var total int64
	if err := s.db.WithContext(ctx).Model(&model.AuditLog{}).Count(&total).Error; err != nil {
		return nil, err
	}
	var items []model.AuditLog
	if err := s.db.WithContext(ctx).Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error; err != nil {
		return nil, err
	}
	return &AuditPage{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

func (s *AccessService) validateMenuInput(ctx context.Context, menuID uint, input MenuInput) error {
	if err := validateMenuFields(input); err != nil {
		return err
	}
	for _, code := range strings.Split(input.PermissionCode, "|") {
		code = strings.TrimSpace(code)
		if code == "" {
			continue
		}
		var count int64
		if err := s.db.WithContext(ctx).Model(&model.Permission{}).Where("code = ?", code).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return ErrUnknownPermission
		}
	}
	if input.ParentID == 0 {
		return nil
	}
	if input.ParentID == menuID {
		return ErrMenuCycle
	}
	seen := map[uint]bool{menuID: true}
	parentID := input.ParentID
	for parentID != 0 {
		if seen[parentID] {
			return ErrMenuCycle
		}
		seen[parentID] = true
		var parent model.Menu
		if err := s.db.WithContext(ctx).Select("id", "parent_id", "type").First(&parent, parentID).Error; err != nil {
			return ErrInvalidInput
		}
		if parent.Type == "button" {
			return ErrInvalidInput
		}
		parentID = parent.ParentID
	}
	return nil
}

func validateMenuFields(input MenuInput) error {
	code := strings.TrimSpace(input.Code)
	title := strings.TrimSpace(input.Title)
	titleKey := strings.TrimSpace(input.TitleKey)
	path := strings.TrimSpace(input.Path)
	icon := strings.TrimSpace(input.Icon)
	app := strings.TrimSpace(input.App)
	menuType := strings.TrimSpace(input.Type)
	permissionCode := strings.TrimSpace(input.PermissionCode)
	if !menuCodePattern.MatchString(code) || input.Sort < 0 || input.Sort > 100000 ||
		!validText(title, 1, 128) || !validText(titleKey, 0, 128) ||
		!validText(path, 0, 255) || !validText(icon, 0, 512) ||
		!validText(permissionCode, 0, 255) {
		return ErrInvalidInput
	}
	if menuType != "group" && menuType != "menu" && menuType != "button" {
		return ErrInvalidInput
	}
	if !appIDPattern.MatchString(app) {
		return ErrInvalidInput
	}
	if path != "" && (!strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || strings.Contains(path, "://")) {
		return ErrInvalidInput
	}
	for _, code := range strings.Split(permissionCode, "|") {
		code = strings.TrimSpace(code)
		if code == "" {
			continue
		}
		if !validPermissionCode(code) {
			return ErrInvalidInput
		}
	}
	return nil
}
