package service

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	authModel "github.com/kwhitestone/prism-fusion/addons/auth/model"
	"github.com/kwhitestone/prism-fusion/addons/rbac/model"
	"github.com/kwhitestone/prism-fusion/global"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PermissionSeed struct {
	Code           string
	Name           string
	Module         string
	Description    string
	IsHighRisk     bool
	DefaultRoleIDs []uint
}

type MenuSeed struct {
	ParentCode     string
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

var ErrSeedConflict = errors.New("seed code already registered")

var seedRegistry = struct {
	sync.RWMutex
	permissions map[string]PermissionSeed
	menus       map[string]MenuSeed
}{permissions: make(map[string]PermissionSeed), menus: make(map[string]MenuSeed)}

// PrepareLegacySchema runs before AutoMigrate adds composite unique indexes.
// Older Prism schemas allowed duplicate role-permission rows, so those rows
// must be normalized first or a production upgrade can fail at startup.
func PrepareLegacySchema(db *gorm.DB) error {
	if db == nil || !db.Migrator().HasTable(&model.RolePermission{}) {
		return nil
	}
	type legacyGrant struct {
		ID           uint
		RoleID       uint
		PermissionID uint
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var grants []legacyGrant
		if err := tx.Table("role_permissions").
			Select("id", "role_id", "permission_id").Order("id ASC").Scan(&grants).Error; err != nil {
			return err
		}
		seen := make(map[[2]uint]struct{}, len(grants))
		duplicateIDs := make([]uint, 0)
		for _, grant := range grants {
			key := [2]uint{grant.RoleID, grant.PermissionID}
			if _, exists := seen[key]; exists {
				duplicateIDs = append(duplicateIDs, grant.ID)
				continue
			}
			seen[key] = struct{}{}
		}
		if len(duplicateIDs) == 0 {
			return nil
		}
		return tx.Exec("DELETE FROM role_permissions WHERE id IN ?", duplicateIDs).Error
	})
}

// MigrateLegacyData runs after AutoMigrate has added the new columns. Values
// are only backfilled when the stable code is absent, which distinguishes old
// rows from intentionally disabled records created by the new control plane.
func MigrateLegacyData(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if tx.Migrator().HasTable(&model.Role{}) {
			var roles []model.Role
			if err := tx.Unscoped().Where("(code IS NULL OR code = '') AND role_id NOT IN ?", []uint{DefaultUserRoleID, AdminRoleID, SuperAdminRoleID}).Find(&roles).Error; err != nil {
				return err
			}
			for _, role := range roles {
				if err := tx.Unscoped().Model(&model.Role{}).Where("id = ?", role.ID).Updates(map[string]any{
					"code":       fmt.Sprintf("legacy_role_%d", role.RoleID),
					"is_enabled": true,
				}).Error; err != nil {
					return err
				}
			}
		}
		if tx.Migrator().HasTable(&model.Menu{}) {
			var menus []model.Menu
			if err := tx.Unscoped().Where("code IS NULL OR code = ''").Order("id ASC").Find(&menus).Error; err != nil {
				return err
			}
			for _, menu := range menus {
				visible := true
				if menu.ShowLink != nil {
					visible = *menu.ShowLink
				}
				if err := tx.Unscoped().Model(&model.Menu{}).Where("id = ?", menu.ID).Updates(map[string]any{
					"code":       fmt.Sprintf("legacy_menu_%d", menu.ID),
					"app":        "shell",
					"type":       "menu",
					"sort":       menu.Rank,
					"is_visible": visible,
				}).Error; err != nil {
					return err
				}
			}
			if err := tx.Model(&model.Menu{}).Where("type IS NULL OR type = ''").Update("type", "menu").Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func RegisterPermissions(seeds ...PermissionSeed) {
	seedRegistry.Lock()
	defer seedRegistry.Unlock()
	next := make(map[string]PermissionSeed, len(seedRegistry.permissions)+len(seeds))
	for code, seed := range seedRegistry.permissions {
		next[code] = seed
	}
	for _, seed := range seeds {
		if err := addPermissionSeed(next, seed); err != nil {
			panic(err)
		}
	}
	seedRegistry.permissions = next
}

func RegisterMenus(seeds ...MenuSeed) {
	seedRegistry.Lock()
	defer seedRegistry.Unlock()
	next := make(map[string]MenuSeed, len(seedRegistry.menus)+len(seeds))
	for code, seed := range seedRegistry.menus {
		next[code] = seed
	}
	for _, seed := range seeds {
		if err := addMenuSeed(next, seed); err != nil {
			panic(err)
		}
	}
	seedRegistry.menus = next
}

func addPermissionSeed(target map[string]PermissionSeed, seed PermissionSeed) error {
	seed, err := normalizePermissionSeed(seed)
	if err != nil {
		return err
	}
	if seed.Code == "" {
		return nil
	}
	if _, exists := target[seed.Code]; exists {
		return fmt.Errorf("%w: permission %q", ErrSeedConflict, seed.Code)
	}
	target[seed.Code] = seed
	return nil
}

func addMenuSeed(target map[string]MenuSeed, seed MenuSeed) error {
	seed, err := normalizeMenuSeed(seed)
	if err != nil {
		return err
	}
	if seed.Code == "" {
		return nil
	}
	if _, exists := target[seed.Code]; exists {
		return fmt.Errorf("%w: menu %q", ErrSeedConflict, seed.Code)
	}
	target[seed.Code] = seed
	return nil
}

func normalizePermissionSeed(seed PermissionSeed) (PermissionSeed, error) {
	seed.Code = strings.TrimSpace(seed.Code)
	seed.Name = strings.TrimSpace(seed.Name)
	seed.Module = strings.TrimSpace(seed.Module)
	seed.Description = strings.TrimSpace(seed.Description)
	if seed.Code == "" {
		if seed.Name == "" && seed.Module == "" && seed.Description == "" &&
			!seed.IsHighRisk && len(seed.DefaultRoleIDs) == 0 {
			return PermissionSeed{}, nil
		}
		return PermissionSeed{}, ErrInvalidInput
	}
	if !validPermissionCode(seed.Code) {
		return PermissionSeed{}, fmt.Errorf("%w: permission code %q must use domain:resource:action", ErrInvalidInput, seed.Code)
	}
	if !validText(seed.Name, 1, 128) || !validText(seed.Module, 1, 64) ||
		!validText(seed.Description, 0, 255) {
		return PermissionSeed{}, ErrInvalidInput
	}
	for _, roleID := range seed.DefaultRoleIDs {
		if roleID == 0 {
			return PermissionSeed{}, ErrInvalidInput
		}
	}
	seed.DefaultRoleIDs = uniqueUint(seed.DefaultRoleIDs)
	return seed, nil
}

func normalizeMenuSeed(seed MenuSeed) (MenuSeed, error) {
	seed.ParentCode = strings.TrimSpace(seed.ParentCode)
	seed.Code = strings.TrimSpace(seed.Code)
	seed.Title = strings.TrimSpace(seed.Title)
	seed.TitleKey = strings.TrimSpace(seed.TitleKey)
	seed.Path = strings.TrimSpace(seed.Path)
	seed.Icon = strings.TrimSpace(seed.Icon)
	seed.App = strings.TrimSpace(seed.App)
	seed.Type = strings.TrimSpace(seed.Type)
	seed.PermissionCode = strings.TrimSpace(seed.PermissionCode)
	if seed.Code == "" {
		if seed.ParentCode == "" && seed.Title == "" && seed.TitleKey == "" &&
			seed.Path == "" && seed.Icon == "" && seed.App == "" && seed.Type == "" &&
			seed.PermissionCode == "" && seed.Sort == 0 && !seed.IsVisible {
			return MenuSeed{}, nil
		}
		return MenuSeed{}, ErrInvalidInput
	}
	if seed.ParentCode != "" && !menuCodePattern.MatchString(seed.ParentCode) {
		return MenuSeed{}, ErrInvalidInput
	}
	if err := validateMenuFields(MenuInput{
		Code: seed.Code, Title: seed.Title, TitleKey: seed.TitleKey,
		Path: seed.Path, Icon: seed.Icon, App: seed.App, Type: seed.Type,
		PermissionCode: seed.PermissionCode, Sort: seed.Sort,
	}); err != nil {
		return MenuSeed{}, err
	}
	return seed, nil
}

func registeredSeeds() ([]PermissionSeed, []MenuSeed) {
	seedRegistry.RLock()
	defer seedRegistry.RUnlock()
	permissions := make([]PermissionSeed, 0, len(seedRegistry.permissions))
	for _, seed := range seedRegistry.permissions {
		seed.DefaultRoleIDs = append([]uint(nil), seed.DefaultRoleIDs...)
		permissions = append(permissions, seed)
	}
	menus := make([]MenuSeed, 0, len(seedRegistry.menus))
	for _, seed := range seedRegistry.menus {
		menus = append(menus, seed)
	}
	sort.Slice(permissions, func(i, j int) bool { return permissions[i].Code < permissions[j].Code })
	sort.Slice(menus, func(i, j int) bool {
		if menus[i].Sort != menus[j].Sort {
			return menus[i].Sort < menus[j].Sort
		}
		return menus[i].Code < menus[j].Code
	})
	return permissions, menus
}

func SeedRegisteredData() error {
	db := global.PRISM_DB
	if db == nil {
		return gorm.ErrInvalidDB
	}
	registeredPermissions, registeredMenus := registeredSeeds()
	permissions, err := mergePermissionSeeds(defaultManagementPermissions(), registeredPermissions)
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := validateBuiltinRoleConflicts(tx); err != nil {
			return err
		}
		if err := migrateLegacyPermissionCodes(tx); err != nil {
			return err
		}
		roles := []model.Role{
			{RoleID: DefaultUserRoleID, Code: "user", RoleName: "User", Description: "Default authenticated user", IsSystem: true, IsEnabled: true, DefaultRouter: "chat"},
			{RoleID: AdminRoleID, Code: "admin", RoleName: "Administrator", Description: "Access administrator", IsSystem: true, IsEnabled: true, DefaultRouter: "access"},
			{RoleID: SuperAdminRoleID, Code: "super_admin", RoleName: "Super administrator", Description: "Protected full-access administrator", IsSystem: true, IsEnabled: true, DefaultRouter: "access"},
		}
		for _, role := range roles {
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "role_id"}},
				DoUpdates: clause.AssignmentColumns([]string{"code", "role_name", "description", "is_system", "is_enabled", "default_router"}),
			}).Create(&role).Error; err != nil {
				return err
			}
		}

		permissionByCode := make(map[string]model.Permission, len(permissions))
		for _, seed := range permissions {
			permission := model.Permission{
				Code: seed.Code, Name: seed.Name, Module: seed.Module,
				Description: seed.Description, IsSystem: true, IsHighRisk: seed.IsHighRisk,
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "code"}},
				DoUpdates: clause.AssignmentColumns([]string{"name", "module", "description", "is_system", "is_high_risk"}),
			}).Create(&permission).Error; err != nil {
				return err
			}
			if err := tx.Where("code = ?", seed.Code).First(&permission).Error; err != nil {
				return err
			}
			permissionByCode[seed.Code] = permission
			for _, roleID := range uniqueUint(seed.DefaultRoleIDs) {
				var roleCount int64
				if err := tx.Model(&model.Role{}).Where("role_id = ?", roleID).Count(&roleCount).Error; err != nil {
					return err
				}
				if roleCount == 0 {
					return fmt.Errorf("%w: default role %d for permission %q", ErrUnknownRole, roleID, seed.Code)
				}
				grant := model.RolePermission{RoleID: roleID, PermissionID: permission.ID}
				if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&grant).Error; err != nil {
					return err
				}
			}
		}

		var users []authModel.User
		if err := tx.Select("id", "role_id").Where("enable = ?", 1).Find(&users).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		for _, user := range users {
			roleID := user.RoleID
			if roleID == 0 {
				roleID = DefaultUserRoleID
				if err := tx.Model(&authModel.User{}).Where("id = ?", user.ID).Update("role_id", roleID).Error; err != nil {
					return err
				}
			}
			assignment := model.UserRole{UserID: user.ID, RoleID: roleID}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&assignment).Error; err != nil {
				return err
			}
		}

		return seedMenus(tx, registeredMenus, permissionByCode)
	})
}

func mergePermissionSeeds(base, contributed []PermissionSeed) ([]PermissionSeed, error) {
	merged := make([]PermissionSeed, 0, len(base)+len(contributed))
	seen := make(map[string]struct{}, len(base)+len(contributed))
	for _, group := range [][]PermissionSeed{base, contributed} {
		for _, seed := range group {
			var err error
			seed, err = normalizePermissionSeed(seed)
			if err != nil {
				return nil, err
			}
			if seed.Code == "" {
				continue
			}
			if _, exists := seen[seed.Code]; exists {
				return nil, fmt.Errorf("%w: permission %q", ErrSeedConflict, seed.Code)
			}
			seen[seed.Code] = struct{}{}
			merged = append(merged, seed)
		}
	}
	return merged, nil
}

func validateBuiltinRoleConflicts(tx *gorm.DB) error {
	type builtin struct {
		roleID         uint
		code           string
		allowLegacyNil bool
	}
	for _, expected := range []builtin{
		{roleID: DefaultUserRoleID, code: "user", allowLegacyNil: true},
		{roleID: AdminRoleID, code: "admin"},
		{roleID: SuperAdminRoleID, code: "super_admin", allowLegacyNil: true},
	} {
		var byID model.Role
		idErr := tx.Unscoped().Where("role_id = ?", expected.roleID).First(&byID).Error
		if idErr == nil {
			legacyAllowed := expected.allowLegacyNil && strings.TrimSpace(byID.Code) == ""
			recognized := byID.Code == expected.code && (expected.roleID != AdminRoleID || byID.IsSystem)
			if !legacyAllowed && !recognized {
				return ErrReservedRoleConflict
			}
		} else if !errors.Is(idErr, gorm.ErrRecordNotFound) {
			return idErr
		}
		var byCode model.Role
		codeErr := tx.Unscoped().Where("code = ?", expected.code).First(&byCode).Error
		if codeErr == nil && byCode.RoleID != expected.roleID {
			return ErrReservedRoleConflict
		}
		if codeErr != nil && !errors.Is(codeErr, gorm.ErrRecordNotFound) {
			return codeErr
		}
	}
	return nil
}

func migrateLegacyPermissionCodes(tx *gorm.DB) error {
	const legacyCode = "auth:user:role:write"
	const currentCode = "auth:user-role:write"
	var legacy model.Permission
	if err := tx.Where("code = ?", legacyCode).First(&legacy).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	var current model.Permission
	if err := tx.Where("code = ?", currentCode).First(&current).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return tx.Model(&legacy).Update("code", currentCode).Error
	} else if err != nil {
		return err
	}
	var roleIDs []uint
	if err := tx.Model(&model.RolePermission{}).Where("permission_id = ?", legacy.ID).Pluck("role_id", &roleIDs).Error; err != nil {
		return err
	}
	for _, roleID := range uniqueUint(roleIDs) {
		grant := model.RolePermission{RoleID: roleID, PermissionID: current.ID}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&grant).Error; err != nil {
			return err
		}
	}
	if err := tx.Where("permission_id = ?", legacy.ID).Delete(&model.RolePermission{}).Error; err != nil {
		return err
	}
	return tx.Unscoped().Delete(&legacy).Error
}

func defaultManagementPermissions() []PermissionSeed {
	admin := []uint{AdminRoleID}
	return []PermissionSeed{
		{Code: "auth:access:read", Name: "Open access management", Module: "access", DefaultRoleIDs: admin},
		{Code: "auth:user:read", Name: "View users", Module: "access", DefaultRoleIDs: admin},
		{Code: "auth:user-role:write", Name: "Assign user roles", Module: "access", IsHighRisk: true, DefaultRoleIDs: admin},
		{Code: "auth:role:read", Name: "View roles", Module: "access", DefaultRoleIDs: admin},
		{Code: "auth:role:write", Name: "Manage roles", Module: "access", IsHighRisk: true, DefaultRoleIDs: admin},
		{Code: "auth:permission:read", Name: "View permissions", Module: "access", DefaultRoleIDs: admin},
		{Code: "auth:menu:read", Name: "View menus", Module: "access", DefaultRoleIDs: admin},
		{Code: "auth:menu:write", Name: "Manage menus", Module: "access", IsHighRisk: true, DefaultRoleIDs: admin},
		{Code: "auth:audit:read", Name: "View access audit", Module: "access", DefaultRoleIDs: admin},
		{Code: "auth:super-admin:grant", Name: "Grant super administrator", Module: "access", IsHighRisk: true},
	}
}

func seedMenus(tx *gorm.DB, seeds []MenuSeed, permissions map[string]model.Permission) error {
	normalized := make([]MenuSeed, 0, len(seeds))
	for _, seed := range seeds {
		validated, err := normalizeMenuSeed(seed)
		if err != nil {
			return err
		}
		if validated.Code != "" {
			normalized = append(normalized, validated)
		}
	}
	seeds = normalized
	for _, seed := range seeds {
		for _, code := range strings.Split(seed.PermissionCode, "|") {
			code = strings.TrimSpace(code)
			if code == "" {
				continue
			}
			if _, exists := permissions[code]; !exists {
				return fmt.Errorf("%w: menu %q references %q", ErrUnknownPermission, seed.Code, code)
			}
		}
	}
	ordered, err := orderMenuSeeds(seeds)
	if err != nil {
		return err
	}
	byCode := make(map[string]model.Menu, len(seeds))
	for _, seed := range ordered {
		parentID := uint(0)
		if seed.ParentCode != "" {
			parent, exists := byCode[seed.ParentCode]
			if !exists {
				if err := tx.Where("code = ?", seed.ParentCode).First(&parent).Error; err != nil {
					return err
				}
			}
			parentID = parent.ID
		}
		visible := seed.IsVisible
		menu := model.Menu{
			ParentID: parentID, Code: seed.Code, Title: seed.Title, TitleKey: seed.TitleKey,
			Path: seed.Path, Icon: seed.Icon, App: seed.App, Type: seed.Type,
			PermissionCode: seed.PermissionCode, Sort: seed.Sort, IsVisible: &visible,
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "code"}},
			DoUpdates: clause.AssignmentColumns([]string{"parent_id", "title", "title_key", "path", "icon", "app", "type", "permission_code", "sort", "is_visible", "deleted_at"}),
		}).Create(&menu).Error; err != nil {
			return err
		}
		menu.ID = 0
		if err := tx.Unscoped().Where("code = ?", seed.Code).First(&menu).Error; err != nil {
			return err
		}
		byCode[seed.Code] = menu
	}
	return validatePersistedMenuHierarchy(tx)
}

func validatePersistedMenuHierarchy(tx *gorm.DB) error {
	var menus []model.Menu
	if err := tx.Select("id", "parent_id", "type").Find(&menus).Error; err != nil {
		return err
	}
	type menuLink struct {
		parentID uint
		menuType string
	}
	links := make(map[uint]menuLink, len(menus))
	for _, menu := range menus {
		links[menu.ID] = menuLink{parentID: menu.ParentID, menuType: menu.Type}
	}
	for _, menu := range menus {
		seen := make(map[uint]struct{})
		current := menu.ID
		for current != 0 {
			if _, exists := seen[current]; exists {
				return ErrMenuCycle
			}
			seen[current] = struct{}{}
			link, exists := links[current]
			if !exists {
				return ErrInvalidInput
			}
			if link.parentID != 0 {
				parent, exists := links[link.parentID]
				if !exists || parent.menuType == "button" {
					return ErrInvalidInput
				}
			}
			current = link.parentID
		}
	}
	return nil
}

func orderMenuSeeds(seeds []MenuSeed) ([]MenuSeed, error) {
	byCode := make(map[string]MenuSeed, len(seeds))
	for _, seed := range seeds {
		byCode[seed.Code] = seed
	}
	orderedInput := append([]MenuSeed(nil), seeds...)
	sort.Slice(orderedInput, func(i, j int) bool {
		if orderedInput[i].Sort != orderedInput[j].Sort {
			return orderedInput[i].Sort < orderedInput[j].Sort
		}
		return orderedInput[i].Code < orderedInput[j].Code
	})
	state := make(map[string]uint8, len(seeds))
	ordered := make([]MenuSeed, 0, len(seeds))
	var visit func(MenuSeed) error
	visit = func(seed MenuSeed) error {
		switch state[seed.Code] {
		case 1:
			return fmt.Errorf("menu seed hierarchy contains a cycle at %q", seed.Code)
		case 2:
			return nil
		}
		state[seed.Code] = 1
		if parent, exists := byCode[seed.ParentCode]; seed.ParentCode != "" && exists {
			if err := visit(parent); err != nil {
				return err
			}
		}
		state[seed.Code] = 2
		ordered = append(ordered, seed)
		return nil
	}
	for _, seed := range orderedInput {
		if err := visit(seed); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}
