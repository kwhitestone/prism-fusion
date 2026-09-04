package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	authModel "github.com/kwhitestone/prism-fusion/addons/auth/model"
	"github.com/kwhitestone/prism-fusion/addons/rbac/model"
	"gorm.io/gorm"
)

func TestPrepareLegacySchemaDeduplicatesRolePermissionsBeforeUniqueIndex(t *testing.T) {
	db := openUnmigratedAccessTestDB(t)
	if err := db.Exec("CREATE TABLE role_permissions (id integer primary key autoincrement, role_id integer, permission_id integer)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO role_permissions (role_id, permission_id) VALUES (7, 11), (7, 11), (7, 12)").Error; err != nil {
		t.Fatal(err)
	}
	if err := PrepareLegacySchema(db); err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.RolePermission{}); err != nil {
		t.Fatalf("unique-index migration after cleanup: %v", err)
	}
	var duplicateCount int64
	if err := db.Model(&model.RolePermission{}).Where("role_id = ? AND permission_id = ?", 7, 11).Count(&duplicateCount).Error; err != nil {
		t.Fatal(err)
	}
	if duplicateCount != 1 {
		t.Fatalf("duplicate role permission count=%d, want 1", duplicateCount)
	}
}

func TestMigrateLegacyDataActivatesAndCodesExistingCustomRolesAndMenus(t *testing.T) {
	db := openUnmigratedAccessTestDB(t)
	if err := db.Exec("CREATE TABLE roles (id integer primary key autoincrement, role_id integer unique, role_name text, parent_id integer, default_router text, data_scope text, deleted_at datetime)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO roles (role_id, role_name) VALUES (12, 'Legacy operator'), (13, 'Legacy auditor')").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE menus (id integer primary key autoincrement, parent_id integer, path text, name text, component text, title text, rank integer, show_link numeric, deleted_at datetime)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO menus (parent_id, path, name, title, rank, show_link) VALUES (0, '/legacy-a', 'LegacyA', 'Legacy A', 4, 1), (0, '/legacy-b', 'LegacyB', 'Legacy B', 5, 0)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Role{}, &model.Menu{}); err != nil {
		t.Fatal(err)
	}
	if err := MigrateLegacyData(db); err != nil {
		t.Fatal(err)
	}
	var roles []model.Role
	if err := db.Order("role_id").Find(&roles).Error; err != nil {
		t.Fatal(err)
	}
	if len(roles) != 2 || !roles[0].IsEnabled || !roles[1].IsEnabled || roles[0].Code != "legacy_role_12" || roles[1].Code != "legacy_role_13" {
		t.Fatalf("legacy roles not migrated: %#v", roles)
	}
	var menus []model.Menu
	if err := db.Order("id").Find(&menus).Error; err != nil {
		t.Fatal(err)
	}
	if len(menus) != 2 || menus[0].Code != "legacy_menu_1" || menus[1].Code != "legacy_menu_2" ||
		menus[0].Sort != 4 || menus[0].Type != "menu" || menus[1].Type != "menu" ||
		menus[1].IsVisible == nil || *menus[1].IsVisible {
		t.Fatalf("legacy menus not migrated: %#v", menus)
	}
}

func TestRegisteredSeedsAndLegacyReaders(t *testing.T) {
	db := openAccessTestDB(t)
	seedRegistry.Lock()
	originalPermissions := seedRegistry.permissions
	originalMenus := seedRegistry.menus
	seedRegistry.Unlock()
	t.Cleanup(func() {
		seedRegistry.Lock()
		seedRegistry.permissions = originalPermissions
		seedRegistry.menus = originalMenus
		seedRegistry.Unlock()
	})
	legacyUser := authModel.User{ID: 77, UUID: "legacy-role-zero", Username: "legacy-role-zero", Enable: 1, RoleID: 0}
	if err := db.Create(&legacyUser).Error; err != nil {
		t.Fatal(err)
	}
	legacyPermission := model.Permission{Code: "auth:user:role:write", Name: "Legacy role assignment", Module: "access"}
	if err := db.Create(&legacyPermission).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.RolePermission{RoleID: 55, PermissionID: legacyPermission.ID}).Error; err != nil {
		t.Fatal(err)
	}
	RegisterPermissions(
		PermissionSeed{Code: "test:catalog:read", Name: "Read catalog", Module: "test", DefaultRoleIDs: []uint{DefaultUserRoleID, DefaultUserRoleID}},
		PermissionSeed{Code: "test:catalog:write", Name: "Write catalog", Module: "test", DefaultRoleIDs: []uint{AdminRoleID}},
		PermissionSeed{},
	)
	RegisterMenus(
		MenuSeed{Code: "test_catalog", Title: "Catalog", Path: "/catalog", App: "core", Type: "menu", PermissionCode: "test:catalog:read", Sort: 700, IsVisible: true},
		MenuSeed{ParentCode: "test_catalog", Code: "test_catalog_edit", Title: "Edit catalog", Path: "/catalog/edit", App: "core", Type: "menu", PermissionCode: "test:catalog:write", Sort: 701, IsVisible: true},
		MenuSeed{ParentCode: "late_parent", Code: "early_child", Title: "Early child", Path: "/late/child", App: "example-site", Type: "menu", Sort: 10, IsVisible: true},
		MenuSeed{Code: "late_parent", Title: "Late parent", Path: "/late", App: "example-site", Type: "group", Sort: 20, IsVisible: true},
		MenuSeed{},
	)
	if err := SeedRegisteredData(); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&legacyUser, legacyUser.ID).Error; err != nil {
		t.Fatal(err)
	}
	if legacyUser.RoleID != DefaultUserRoleID {
		t.Fatalf("legacy user's primary role = %d, want %d", legacyUser.RoleID, DefaultUserRoleID)
	}
	var defaultAssignment int64
	if err := db.Model(&model.UserRole{}).Where("user_id = ? AND role_id = ?", legacyUser.ID, DefaultUserRoleID).Count(&defaultAssignment).Error; err != nil || defaultAssignment != 1 {
		t.Fatalf("legacy default assignment count=%d err=%v", defaultAssignment, err)
	}
	var migratedPermission model.Permission
	if err := db.Where("code = ?", "auth:user-role:write").First(&migratedPermission).Error; err != nil {
		t.Fatalf("legacy permission was not migrated: %v", err)
	}
	var migratedGrant int64
	if err := db.Model(&model.RolePermission{}).Where("role_id = ? AND permission_id = ?", 55, migratedPermission.ID).Count(&migratedGrant).Error; err != nil || migratedGrant != 1 {
		t.Fatalf("legacy permission grant count=%d err=%v", migratedGrant, err)
	}
	// Compatibility entrypoint must remain idempotent for downstream projects.
	SeedData()

	roles, err := (&RoleService{}).GetRoleList()
	if err != nil || len(roles) < 3 {
		t.Fatalf("legacy role list: len=%d err=%v", len(roles), err)
	}
	var permission model.Permission
	if err := db.Where("code = ?", "test:catalog:read").First(&permission).Error; err != nil {
		t.Fatal(err)
	}
	userPermissions, err := (&RoleService{}).GetRolePermissions(DefaultUserRoleID)
	if err != nil || !HasPermission(userPermissions, permission.Code) {
		t.Fatalf("legacy role permissions=%v err=%v", userPermissions, err)
	}
	superPermissions, err := (&RoleService{}).GetRolePermissions(SuperAdminRoleID)
	if err != nil || len(superPermissions) != 1 || superPermissions[0] != "*:*:*" {
		t.Fatalf("super permissions=%v err=%v", superPermissions, err)
	}

	var parent, child model.Menu
	if err := db.Where("code = ?", "test_catalog").First(&parent).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Where("code = ?", "test_catalog_edit").First(&child).Error; err != nil {
		t.Fatal(err)
	}
	if child.ParentID != parent.ID {
		t.Fatalf("child parent=%d want=%d", child.ParentID, parent.ID)
	}
	if err := NewAccessService(db).DeleteMenu(context.Background(), 7, child.ID, "seed-delete"); err != nil {
		t.Fatal(err)
	}
	if err := SeedRegisteredData(); err != nil {
		t.Fatalf("registered menu must be recoverable after soft delete: %v", err)
	}
	if err := db.Where("code = ?", "test_catalog_edit").First(&child).Error; err != nil {
		t.Fatalf("registered menu was not restored: %v", err)
	}
	hide := false
	if err := db.Model(&parent).Updates(map[string]any{
		"name": "Catalog", "component": "CatalogView", "redirect": "/catalog/edit",
		"rank": 1, "show_link": &hide, "roles": `["user"]`, "auths": `["catalog:read"]`,
	}).Error; err != nil {
		t.Fatal(err)
	}
	routes, err := (&MenuService{}).GetAsyncRoutes()
	if err != nil || len(routes) == 0 {
		t.Fatalf("legacy routes len=%d err=%v", len(routes), err)
	}
	visible, err := NewAccessService(db).ResolveVisibleMenus(context.Background(), 100)
	if !errors.Is(err, gorm.ErrRecordNotFound) || visible != nil {
		t.Fatalf("missing user visible menus=%v err=%v", visible, err)
	}
}

func TestMenuLifecycleValidatesHierarchyPermissionsAndPaths(t *testing.T) {
	db := openAccessTestDB(t)
	permissions := []model.Permission{
		{Code: "auth:menu:read", Name: "Read menus", Module: "access"},
		{Code: "auth:menu:write", Name: "Write menus", Module: "access"},
	}
	if err := db.Create(&permissions).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewAccessService(db)
	valid := MenuInput{Code: "menu_group", Title: "Menu group", App: "auth", Type: "group", Sort: 10, IsVisible: true}
	invalid := []MenuInput{
		{Code: "!", App: "auth", Type: "menu"},
		{Code: "bad_sort", App: "auth", Type: "menu", Sort: -1},
		{Code: "bad_type", App: "auth", Type: "unknown"},
		{Code: "bad_app", App: "Bad App!", Type: "menu"},
		{Code: "bad_path", App: "auth", Type: "menu", Path: "https://example.test"},
		{Code: "bad_permission", App: "auth", Type: "menu", PermissionCode: "auth:missing:read"},
	}
	for _, input := range invalid {
		if _, err := svc.CreateMenu(context.Background(), 7, input, "request-invalid"); err == nil {
			t.Fatalf("invalid menu accepted: %#v", input)
		}
	}
	parent, err := svc.CreateMenu(context.Background(), 7, valid, "request-parent")
	if err != nil {
		t.Fatal(err)
	}
	pluginMenu := MenuInput{Code: "plugin_menu", Title: "Plugin", App: "example-site", Type: "menu", IsVisible: true}
	if _, err := svc.CreateMenu(context.Background(), 7, pluginMenu, "request-plugin"); err != nil {
		t.Fatalf("plugin-defined app identifier was rejected: %v", err)
	}
	childInput := MenuInput{
		ParentID: parent.ID, Code: "menu_child", Title: "Menu child", Path: "/menus/child",
		App: "auth", Type: "menu", PermissionCode: "auth:menu:read|auth:menu:write", Sort: 11, IsVisible: false,
	}
	child, err := svc.CreateMenu(context.Background(), 7, childInput, "request-child")
	if err != nil {
		t.Fatal(err)
	}
	button, err := svc.CreateMenu(context.Background(), 7, MenuInput{
		Code: "menu_button", Title: "Action", App: "shell", Type: "button", IsVisible: true,
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	buttonChild := childInput
	buttonChild.ParentID = button.ID
	buttonChild.Code = "button_child"
	if _, err := svc.CreateMenu(context.Background(), 7, buttonChild, ""); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("button parent error=%v, want ErrInvalidInput", err)
	}
	parentAsButton := valid
	parentAsButton.Type = "button"
	if _, err := svc.UpdateMenu(context.Background(), 7, parent.ID, parentAsButton, ""); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("parent converted to button error=%v, want ErrInvalidInput", err)
	}
	var unchangedParent model.Menu
	if err := db.First(&unchangedParent, parent.ID).Error; err != nil {
		t.Fatal(err)
	}
	if unchangedParent.Type != valid.Type {
		t.Fatalf("rejected parent conversion persisted type=%q", unchangedParent.Type)
	}
	cycle := valid
	cycle.ParentID = child.ID
	if _, err := svc.UpdateMenu(context.Background(), 7, parent.ID, cycle, ""); err != ErrMenuCycle {
		t.Fatalf("cycle update=%v, want ErrMenuCycle", err)
	}
	if err := svc.DeleteMenu(context.Background(), 7, parent.ID, ""); err != ErrInvalidInput {
		t.Fatalf("delete parent=%v, want ErrInvalidInput", err)
	}
	if err := svc.DeleteMenu(context.Background(), 7, child.ID, "request-delete-child"); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteMenu(context.Background(), 7, parent.ID, "request-delete-parent"); err != nil {
		t.Fatal(err)
	}
	accessInput := MenuInput{
		Code: "account_access", Title: "Access", Path: "/account/access", App: "auth", Type: "menu",
		PermissionCode: "auth:menu:read", Sort: 20, IsVisible: true,
	}
	accessMenu, err := svc.CreateMenu(context.Background(), 7, accessInput, "")
	if err != nil {
		t.Fatal(err)
	}
	hiddenAccess := accessInput
	hiddenAccess.IsVisible = false
	if _, err := svc.UpdateMenu(context.Background(), 7, accessMenu.ID, hiddenAccess, ""); err != ErrProtectedMenu {
		t.Fatalf("hide access menu=%v, want ErrProtectedMenu", err)
	}
	buttonAccess := accessInput
	buttonAccess.Type = "button"
	if _, err := svc.UpdateMenu(context.Background(), 7, accessMenu.ID, buttonAccess, ""); err != ErrProtectedMenu {
		t.Fatalf("change access menu type=%v, want ErrProtectedMenu", err)
	}
	if err := svc.DeleteMenu(context.Background(), 7, accessMenu.ID, ""); err != ErrProtectedMenu {
		t.Fatalf("delete access menu=%v, want ErrProtectedMenu", err)
	}
	if err := svc.DeleteMenu(context.Background(), 7, 999999, ""); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("missing menu delete=%v", err)
	}
	audit, err := svc.ListAudit(context.Background(), 0, 1000)
	if err != nil || audit.Page != 1 || audit.PageSize != 100 || audit.Total < 4 {
		t.Fatalf("audit=%#v err=%v", audit, err)
	}
}

func TestSeedMenusRejectsUnknownPermissionCodes(t *testing.T) {
	db := openAccessTestDB(t)
	permissions := map[string]model.Permission{
		"catalog:read:item": {Code: "catalog:read:item"},
	}
	seeds := []MenuSeed{{
		Code: "catalog", Title: "Catalog", App: "example-site", Type: "menu",
		PermissionCode: "catalog:read:item|catalog:missing:item", IsVisible: true,
	}}

	if err := seedMenus(db, seeds, permissions); !errors.Is(err, ErrUnknownPermission) {
		t.Fatalf("seedMenus error=%v, want ErrUnknownPermission", err)
	}
	var count int64
	if err := db.Model(&model.Menu{}).Where("code = ?", "catalog").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("invalid menu seed was persisted: count=%d", count)
	}
}

func TestConcurrentInverseMenuUpdatesCannotCreateCycle(t *testing.T) {
	db := openAccessTestDB(t)
	menuA := model.Menu{Code: "menu_a", Title: "A", App: "example-site", Type: "menu"}
	menuB := model.Menu{Code: "menu_b", Title: "B", App: "example-site", Type: "menu"}
	if err := db.Create(&[]*model.Menu{&menuA, &menuB}).Error; err != nil {
		t.Fatal(err)
	}

	arrived := make(chan struct{}, 2)
	release := make(chan struct{})
	callbackName := "test:inverse-menu-update-barrier"
	if err := db.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table != "menus" || len(tx.Statement.Selects) != 1 ||
			tx.Statement.Selects[0] != "id" {
			return
		}
		arrived <- struct{}{}
		select {
		case <-release:
		case <-time.After(200 * time.Millisecond):
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Query().Remove(callbackName) })

	start := make(chan struct{})
	errorsByUpdate := make(chan error, 2)
	svc := NewAccessService(db)
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		<-start
		_, err := svc.UpdateMenu(context.Background(), 7, menuA.ID, MenuInput{
			ParentID: menuB.ID, Code: menuA.Code, Title: menuA.Title,
			App: menuA.App, Type: menuA.Type, IsVisible: true,
		}, "update-a")
		errorsByUpdate <- err
	}()
	go func() {
		defer workers.Done()
		<-start
		_, err := svc.UpdateMenu(context.Background(), 7, menuB.ID, MenuInput{
			ParentID: menuA.ID, Code: menuB.Code, Title: menuB.Title,
			App: menuB.App, Type: menuB.Type, IsVisible: true,
		}, "update-b")
		errorsByUpdate <- err
	}()
	close(start)
	<-arrived
	select {
	case <-arrived:
	case <-time.After(250 * time.Millisecond):
	}
	close(release)
	workers.Wait()
	close(errorsByUpdate)

	successes, cycleErrors := 0, 0
	for err := range errorsByUpdate {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrMenuCycle):
			cycleErrors++
		default:
			t.Fatalf("unexpected concurrent update error: %v", err)
		}
	}
	if successes != 1 || cycleErrors != 1 {
		t.Fatalf("concurrent updates successes=%d cycleErrors=%d", successes, cycleErrors)
	}
}

func TestConcurrentActorRoleDisablesCannotSelfLock(t *testing.T) {
	db := openAccessTestDB(t)
	writePermission := model.Permission{
		Code: "auth:role:write", Name: "Manage roles", Module: "auth",
	}
	roles := []model.Role{
		{RoleID: 20, Code: "manager_a", RoleName: "Manager A", IsEnabled: true},
		{RoleID: 21, Code: "manager_b", RoleName: "Manager B", IsEnabled: true},
	}
	if err := db.Create(&writePermission).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&roles).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&authModel.User{
		ID: 7, UUID: "actor-7", Username: "manager", RoleID: 20, Enable: 1,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]model.UserRole{{UserID: 7, RoleID: 20}, {UserID: 7, RoleID: 21}}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]model.RolePermission{
		{RoleID: 20, PermissionID: writePermission.ID},
		{RoleID: 21, PermissionID: writePermission.ID},
	}).Error; err != nil {
		t.Fatal(err)
	}

	arrived := make(chan struct{}, 2)
	release := make(chan struct{})
	callbackName := "test:role-disable-barrier"
	if err := db.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table != "roles" {
			return
		}
		arrived <- struct{}{}
		select {
		case <-release:
		case <-time.After(time.Second):
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Update().Remove(callbackName) })

	disabled := false
	svc := NewAccessService(db)
	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for _, roleID := range []uint{20, 21} {
		workers.Add(1)
		go func(id uint) {
			defer workers.Done()
			<-start
			_, err := svc.UpdateRole(context.Background(), 7, id, RolePatch{IsEnabled: &disabled}, "")
			results <- err
		}(roleID)
	}
	close(start)
	select {
	case <-arrived:
	case <-time.After(time.Second):
		t.Fatal("neither role update reached the persistence barrier")
	}
	select {
	case <-arrived:
	case <-time.After(250 * time.Millisecond):
	}
	close(release)
	workers.Wait()
	close(results)

	successes, selfLockouts := 0, 0
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrSelfLockout):
			selfLockouts++
		default:
			t.Fatalf("unexpected concurrent role update error: %v", err)
		}
	}
	if successes != 1 || selfLockouts != 1 {
		t.Fatalf("concurrent role updates successes=%d selfLockouts=%d", successes, selfLockouts)
	}
	var enabledCount int64
	if err := db.Model(&model.Role{}).Where("role_id IN ? AND is_enabled = ?", []uint{20, 21}, true).Count(&enabledCount).Error; err != nil {
		t.Fatal(err)
	}
	if enabledCount != 1 {
		t.Fatalf("enabled management roles=%d, want 1", enabledCount)
	}
}

func TestSeedRejectsReservedRoleIDAndCodeConflicts(t *testing.T) {
	for _, tc := range []struct {
		name string
		role model.Role
	}{
		{name: "reserved admin id", role: model.Role{RoleID: AdminRoleID, Code: "custom_legacy", RoleName: "Legacy custom", IsEnabled: true}},
		{name: "reserved admin code", role: model.Role{RoleID: 77, Code: "admin", RoleName: "Conflicting admin", IsEnabled: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := openAccessTestDB(t)
			if err := db.Create(&tc.role).Error; err != nil {
				t.Fatal(err)
			}
			if err := SeedRegisteredData(); err != ErrReservedRoleConflict {
				t.Fatalf("seed conflict=%v, want ErrReservedRoleConflict", err)
			}
		})
	}
}

func TestSeedRegistrationRejectsDuplicateCodes(t *testing.T) {
	permissions := map[string]PermissionSeed{}
	validPermission := PermissionSeed{Code: "example:item:read", Name: "Read items", Module: "example"}
	if err := addPermissionSeed(permissions, validPermission); err != nil {
		t.Fatal(err)
	}
	if err := addPermissionSeed(permissions, validPermission); !errors.Is(err, ErrSeedConflict) {
		t.Fatalf("duplicate permission error = %v, want ErrSeedConflict", err)
	}
	if err := addPermissionSeed(permissions, PermissionSeed{Code: "example:read", Name: "Read", Module: "example"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("two-segment permission error = %v, want ErrInvalidInput", err)
	}
	menus := map[string]MenuSeed{}
	validMenu := MenuSeed{Code: "example_menu", Title: "Example", App: "example", Type: "menu"}
	if err := addMenuSeed(menus, validMenu); err != nil {
		t.Fatal(err)
	}
	if err := addMenuSeed(menus, validMenu); !errors.Is(err, ErrSeedConflict) {
		t.Fatalf("duplicate menu error = %v, want ErrSeedConflict", err)
	}
	if _, err := mergePermissionSeeds(
		[]PermissionSeed{{Code: "reserved:item:read", Name: "Read", Module: "reserved"}},
		[]PermissionSeed{{Code: "reserved:item:read", Name: "Read", Module: "reserved"}},
	); !errors.Is(err, ErrSeedConflict) {
		t.Fatalf("reserved permission error = %v, want ErrSeedConflict", err)
	}
}

func TestSeedRegistrationRejectsInvalidFields(t *testing.T) {
	permissionCases := []PermissionSeed{
		{Code: "example:item:read", Module: "example"},
		{Code: "example:item:read", Name: "Read", Module: ""},
		{Code: "example:item:read", Name: strings.Repeat("n", 129), Module: "example"},
		{Code: "example:item:read", Name: "Read", Module: strings.Repeat("m", 65)},
		{Code: "example:item:read", Name: "Read", Module: "example", Description: strings.Repeat("d", 256)},
		{Code: "example:item:read", Name: "Read", Module: "example", DefaultRoleIDs: []uint{0}},
	}
	for i, seed := range permissionCases {
		if err := addPermissionSeed(map[string]PermissionSeed{}, seed); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("permission case %d error=%v, want ErrInvalidInput", i, err)
		}
	}

	menuCases := []MenuSeed{
		{Code: "Bad Code", Title: "Example", App: "example", Type: "menu"},
		{Code: "example", ParentCode: "Bad Parent", Title: "Example", App: "example", Type: "menu"},
		{Code: "example", App: "example", Type: "menu"},
		{Code: "example", Title: "Example", Path: "https://evil.example", App: "example", Type: "menu"},
		{Code: "example", Title: "Example", App: "Bad App", Type: "menu"},
		{Code: "example", Title: "Example", App: "example", Type: "invalid"},
		{Code: "example", Title: "Example", App: "example", Type: "menu", Sort: -1},
		{Code: "example", Title: "Example", App: "example", Type: "menu", PermissionCode: "invalid"},
		{Code: "example", Title: strings.Repeat("t", 129), App: "example", Type: "menu"},
	}
	for i, seed := range menuCases {
		if err := addMenuSeed(map[string]MenuSeed{}, seed); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("menu case %d error=%v, want ErrInvalidInput", i, err)
		}
	}
}

func TestSeedMenusRejectsCycleWithExistingRows(t *testing.T) {
	db := openAccessTestDB(t)
	parent := model.Menu{Code: "existing_parent", Title: "Parent", App: "example", Type: "menu"}
	child := model.Menu{Code: "existing_child", Title: "Child", App: "example", Type: "menu"}
	if err := db.Create(&parent).Error; err != nil {
		t.Fatal(err)
	}
	child.ParentID = parent.ID
	if err := db.Create(&child).Error; err != nil {
		t.Fatal(err)
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		return seedMenus(tx, []MenuSeed{{
			ParentCode: child.Code, Code: parent.Code, Title: parent.Title,
			App: parent.App, Type: parent.Type, IsVisible: true,
		}}, map[string]model.Permission{})
	})
	if !errors.Is(err, ErrMenuCycle) {
		t.Fatalf("seedMenus error=%v, want ErrMenuCycle", err)
	}
	var reloaded model.Menu
	if err := db.First(&reloaded, parent.ID).Error; err != nil {
		t.Fatal(err)
	}
	if reloaded.ParentID != 0 {
		t.Fatalf("cyclic seed was not rolled back: parent_id=%d", reloaded.ParentID)
	}
}

func TestSeedMenusRejectsButtonParentsInExistingRows(t *testing.T) {
	db := openAccessTestDB(t)
	button := model.Menu{Code: "existing_button", Title: "Action", App: "example", Type: "button"}
	if err := db.Create(&button).Error; err != nil {
		t.Fatal(err)
	}
	child := model.Menu{
		ParentID: button.ID, Code: "invalid_child", Title: "Child", App: "example", Type: "menu",
	}
	if err := db.Create(&child).Error; err != nil {
		t.Fatal(err)
	}
	if err := seedMenus(db, nil, map[string]model.Permission{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("seedMenus error=%v, want ErrInvalidInput", err)
	}
}

func TestSeedRegisteredDataRejectsUnknownDefaultRole(t *testing.T) {
	db := openAccessTestDB(t)
	seedRegistry.Lock()
	originalPermissions := seedRegistry.permissions
	originalMenus := seedRegistry.menus
	seedRegistry.permissions = map[string]PermissionSeed{
		"example:item:read": {
			Code: "example:item:read", Name: "Read items", Module: "example",
			DefaultRoleIDs: []uint{12345},
		},
	}
	seedRegistry.menus = map[string]MenuSeed{}
	seedRegistry.Unlock()
	t.Cleanup(func() {
		seedRegistry.Lock()
		seedRegistry.permissions = originalPermissions
		seedRegistry.menus = originalMenus
		seedRegistry.Unlock()
	})

	if err := SeedRegisteredData(); !errors.Is(err, ErrUnknownRole) {
		t.Fatalf("SeedRegisteredData error=%v, want ErrUnknownRole", err)
	}
	var count int64
	if err := db.Model(&model.Permission{}).Where("code = ?", "example:item:read").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("permission with unknown default role was not rolled back: count=%d", count)
	}
}

func TestListUsersIncludesLegacyPrimaryRoleCode(t *testing.T) {
	db := openAccessTestDB(t)
	if err := db.Create(&model.Role{
		RoleID: 1, Code: "user", RoleName: "User", IsEnabled: true,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&authModel.User{
		ID: 77, UUID: "new-user-77", Username: "new-user", RoleID: 1, Enable: 1,
	}).Error; err != nil {
		t.Fatal(err)
	}

	page, err := NewAccessService(db).ListUsers(context.Background(), 1, 20, "new-user")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || len(page.Items[0].RoleIDs) != 1 ||
		page.Items[0].RoleIDs[0] != 1 || len(page.Items[0].RoleCodes) != 1 ||
		page.Items[0].RoleCodes[0] != "user" {
		t.Fatalf("legacy primary role contract mismatch: %#v", page.Items)
	}
}

func TestAccessCatalogListsUseStableContracts(t *testing.T) {
	db := openAccessTestDB(t)
	role := model.Role{RoleID: 12, Code: "auditor", RoleName: "Auditor", IsEnabled: true}
	permission := model.Permission{Code: "audit:event:read", Name: "Read audit events", Module: "audit"}
	menu := model.Menu{Code: "audit_events", Title: "Audit events", App: "auth", Type: "menu"}
	if err := db.Create(&role).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&permission).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.RolePermission{RoleID: role.RoleID, PermissionID: permission.ID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&menu).Error; err != nil {
		t.Fatal(err)
	}

	svc := NewAccessService(db)
	roles, err := svc.ListRoles(context.Background())
	if err != nil || len(roles) != 1 || roles[0].Code != role.Code {
		t.Fatalf("roles=%#v err=%v", roles, err)
	}
	permissions, err := svc.ListPermissions(context.Background())
	if err != nil || len(permissions) != 1 || permissions[0].Code != permission.Code {
		t.Fatalf("permissions=%#v err=%v", permissions, err)
	}
	granted, err := svc.GetRolePermissions(context.Background(), role.RoleID)
	if err != nil || len(granted) != 1 || granted[0].Code != permission.Code {
		t.Fatalf("role permissions=%#v err=%v", granted, err)
	}
	if _, err := svc.GetRolePermissions(context.Background(), 9999); !errors.Is(err, ErrUnknownRole) {
		t.Fatalf("unknown role permissions error=%v", err)
	}
	menus, err := svc.ListMenus(context.Background())
	if err != nil || len(menus) != 1 || menus[0].Code != menu.Code {
		t.Fatalf("menus=%#v err=%v", menus, err)
	}
	audit, err := svc.ListAudit(context.Background(), -1, 1000)
	if err != nil || audit.Page != 1 || audit.PageSize != 100 || audit.Total != 0 {
		t.Fatalf("audit=%#v err=%v", audit, err)
	}
}
