package service

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	authModel "github.com/kwhitestone/prism-fusion/addons/auth/model"
	"github.com/kwhitestone/prism-fusion/addons/rbac/model"
	"github.com/kwhitestone/prism-fusion/config"
	"github.com/kwhitestone/prism-fusion/global"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var accessTestDBSequence atomic.Uint64

func TestResolveConfiguredUserAccessHonorsProviderMode(t *testing.T) {
	previousConfig := global.PRISM_CONFIG
	t.Cleanup(func() { global.PRISM_CONFIG = previousConfig })

	global.PRISM_CONFIG.RBAC = config.RBAC{Provider: "disabled"}
	disabled, err := ResolveConfiguredUserAccess(context.Background(), nil, 7)
	if err != nil || len(disabled.Roles) != 0 || len(disabled.Permissions) != 0 || len(disabled.Menus) != 0 {
		t.Fatalf("disabled RBAC must not query storage: access=%#v err=%v", disabled, err)
	}
	legacy, err := ResolveConfiguredUserAccess(context.Background(), nil, 7, SuperAdminRoleID)
	if err != nil || len(legacy.Roles) != 1 || legacy.Roles[0] != "admin" {
		t.Fatalf("disabled RBAC must preserve the legacy role contract: access=%#v err=%v", legacy, err)
	}

	db := openAccessTestDB(t)
	if err := db.Create(&model.Role{RoleID: 12, Code: "shared_reader", RoleName: "Shared reader", IsEnabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&authModel.User{ID: 7, UUID: "user-7", Username: "shared-user", Enable: 1, RoleID: 12}).Error; err != nil {
		t.Fatal(err)
	}
	for _, provider := range []string{"external", "builtin"} {
		global.PRISM_CONFIG.RBAC = config.RBAC{Provider: provider}
		access, err := ResolveConfiguredUserAccess(context.Background(), db, 7)
		if err != nil || len(access.Roles) != 1 || access.Roles[0] != "shared_reader" {
			t.Fatalf("%s RBAC did not resolve its configured tables: access=%#v err=%v", provider, access, err)
		}
	}
}

func openAccessTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := openUnmigratedAccessTestDB(t)
	if err := db.AutoMigrate(
		&authModel.User{},
		&authModel.RefreshSession{},
		&model.Role{},
		&model.Permission{},
		&model.RolePermission{},
		&model.UserRole{},
		&model.Menu{},
		&model.AuditLog{},
	); err != nil {
		t.Fatalf("migrate access schema: %v", err)
	}
	global.PRISM_DB = db
	t.Cleanup(func() { global.PRISM_DB = nil })
	return db
}

func openUnmigratedAccessTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", t.Name(), accessTestDBSequence.Add(1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestHasPermissionSupportsExactAndSegmentWildcards(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		have []string
		want string
		ok   bool
	}{
		{name: "exact", have: []string{"auth:role:read"}, want: "auth:role:read", ok: true},
		{name: "global wildcard", have: []string{"*:*:*"}, want: "core:provider:write", ok: true},
		{name: "module wildcard", have: []string{"core:provider:*"}, want: "core:provider:write", ok: true},
		{name: "different action", have: []string{"core:provider:read"}, want: "core:provider:write", ok: false},
		{name: "malformed wildcard", have: []string{"core*"}, want: "core:provider:write", ok: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := HasPermission(tc.have, tc.want); got != tc.ok {
				t.Fatalf("HasPermission(%v, %q) = %v, want %v", tc.have, tc.want, got, tc.ok)
			}
		})
	}
}

func TestResolveUserAccessUnionsEnabledRolesAndFiltersMenus(t *testing.T) {
	db := openAccessTestDB(t)
	roles := []model.Role{
		{RoleID: 10, Code: "reader", RoleName: "Reader", IsEnabled: true},
		{RoleID: 11, Code: "disabled-writer", RoleName: "Disabled writer", IsEnabled: false},
	}
	if err := db.Create(&roles).Error; err != nil {
		t.Fatalf("create roles: %v", err)
	}
	if err := db.Model(&model.Role{}).Where("role_id = ?", 11).Update("is_enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	perms := []model.Permission{
		{Code: "core:conversation:read", Name: "Read conversations", Module: "conversation", IsSystem: true},
		{Code: "core:conversation:write", Name: "Write conversations", Module: "conversation", IsSystem: true},
	}
	if err := db.Create(&perms).Error; err != nil {
		t.Fatalf("create permissions: %v", err)
	}
	if err := db.Create(&[]model.RolePermission{
		{RoleID: 10, PermissionID: perms[0].ID},
		{RoleID: 11, PermissionID: perms[1].ID},
	}).Error; err != nil {
		t.Fatalf("grant permissions: %v", err)
	}
	if err := db.Create(&[]model.UserRole{{UserID: 7, RoleID: 10}, {UserID: 7, RoleID: 11}}).Error; err != nil {
		t.Fatalf("assign roles: %v", err)
	}
	visible := true
	hidden := false
	menus := []model.Menu{
		{Code: "chat", Title: "Chat", Path: "/chat", PermissionCode: "core:conversation:read", IsVisible: &visible},
		{Code: "admin", Title: "Admin", Path: "/admin", PermissionCode: "core:conversation:write", IsVisible: &visible},
		{Code: "detail", Title: "Detail", Path: "/chat/:id", PermissionCode: "core:conversation:read", IsVisible: &hidden},
	}
	if err := db.Create(&menus).Error; err != nil {
		t.Fatalf("create menus: %v", err)
	}

	access, err := NewAccessService(db).ResolveUserAccess(context.Background(), 7)
	if err != nil {
		t.Fatalf("resolve access: %v", err)
	}
	if len(access.Roles) != 1 || access.Roles[0] != "reader" {
		t.Fatalf("roles = %v, want [reader]", access.Roles)
	}
	if len(access.Permissions) != 1 || access.Permissions[0] != "core:conversation:read" {
		t.Fatalf("permissions = %v", access.Permissions)
	}
	if len(access.Menus) != 2 || access.Menus[0].Code != "chat" || access.Menus[1].Code != "detail" {
		t.Fatalf("menus = %#v, want chat and routable detail", access.Menus)
	}

	button := model.Menu{
		ParentID: menus[0].ID, Code: "chat_send", Title: "Send",
		Type: "button", PermissionCode: "core:conversation:read", IsVisible: &visible,
	}
	if err := db.Create(&button).Error; err != nil {
		t.Fatal(err)
	}
	routes, err := NewAccessService(db).ResolveAsyncRoutes(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != 1 || routes[0].Name != "chat" || len(routes[0].Children) != 0 {
		t.Fatalf("async routes included a button or hidden route: %#v", routes)
	}
}

func TestSetUserRolesRejectsSelfLockoutAndRevokesTargetSessions(t *testing.T) {
	db := openAccessTestDB(t)
	if err := db.Create(&authModel.User{ID: 42, UUID: "user-42", Username: "operator", Enable: 1, RoleID: 999}).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Create(&authModel.User{ID: 43, UUID: "user-43", Username: "backup-super", Enable: 1, RoleID: 999}).Error; err != nil {
		t.Fatalf("create backup user: %v", err)
	}
	roles := []model.Role{
		{RoleID: 1, Code: "user", RoleName: "User", IsSystem: true, IsEnabled: true},
		{RoleID: 999, Code: "super_admin", RoleName: "Super administrator", IsSystem: true, IsEnabled: true},
	}
	if err := db.Create(&roles).Error; err != nil {
		t.Fatalf("create roles: %v", err)
	}
	if err := db.Create(&[]model.UserRole{{UserID: 42, RoleID: 1}, {UserID: 42, RoleID: 999}, {UserID: 43, RoleID: 999}}).Error; err != nil {
		t.Fatalf("assign roles: %v", err)
	}
	session := authModel.RefreshSession{
		UserID:          42,
		FamilyID:        "family-00000000000000000000000000000001",
		TokenHash:       "hash",
		ExpiresAt:       time.Now().Add(time.Hour),
		FamilyExpiresAt: time.Now().Add(2 * time.Hour),
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}

	svc := NewAccessService(db)
	if err := svc.SetUserRoles(context.Background(), 42, 42, []uint{1}); err != ErrSelfLockout {
		t.Fatalf("self role downgrade error = %v, want ErrSelfLockout", err)
	}
	if err := svc.SetUserRoles(context.Background(), 43, 42, []uint{1}); err != nil {
		t.Fatalf("administrator role update: %v", err)
	}

	var active int64
	if err := db.Model(&authModel.RefreshSession{}).
		Where("user_id = ? AND revoked_at IS NULL", 42).Count(&active).Error; err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if active != 0 {
		t.Fatalf("active sessions = %d, want 0", active)
	}
	var assignments []model.UserRole
	if err := db.Where("user_id = ?", 42).Find(&assignments).Error; err != nil {
		t.Fatalf("load assignments: %v", err)
	}
	if len(assignments) != 1 || assignments[0].RoleID != 1 {
		t.Fatalf("assignments = %#v, want only user role", assignments)
	}
}

func TestRoleAdministrationCannotGrantCapabilitiesActorDoesNotHave(t *testing.T) {
	db := openAccessTestDB(t)
	roles := []model.Role{
		{RoleID: 10, Code: "role_manager", RoleName: "Role manager", IsEnabled: true},
		{RoleID: 20, Code: "provider_manager", RoleName: "Provider manager", IsEnabled: true},
	}
	if err := db.Create(&roles).Error; err != nil {
		t.Fatal(err)
	}
	permissions := []model.Permission{
		{Code: "auth:role:write", Name: "Manage roles", Module: "access"},
		{Code: "auth:user-role:write", Name: "Assign roles", Module: "access"},
		{Code: "core:provider:write", Name: "Manage providers", Module: "providers"},
	}
	if err := db.Create(&permissions).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&authModel.User{ID: 7, UUID: "user-7", Username: "role-manager", Enable: 1, RoleID: 10}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]model.RolePermission{
		{RoleID: 10, PermissionID: permissions[0].ID},
		{RoleID: 10, PermissionID: permissions[1].ID},
		{RoleID: 20, PermissionID: permissions[2].ID},
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]model.UserRole{{UserID: 7, RoleID: 10}}).Error; err != nil {
		t.Fatal(err)
	}

	svc := NewAccessService(db)
	if err := svc.SetRolePermissions(context.Background(), 7, 20, []uint{permissions[2].ID}); err != ErrAccessDenied {
		t.Fatalf("granting a capability outside actor ceiling = %v, want ErrAccessDenied", err)
	}
	if err := svc.SetRolePermissions(context.Background(), 7, 20, nil); err != ErrAccessDenied {
		t.Fatalf("removing a capability outside actor ceiling = %v, want ErrAccessDenied", err)
	}
	name := "Renamed superior role"
	if _, err := svc.UpdateRole(context.Background(), 7, 20, RolePatch{Name: &name}, ""); err != ErrAccessDenied {
		t.Fatalf("editing a role outside actor ceiling = %v, want ErrAccessDenied", err)
	}
	if err := svc.SetUserRoles(context.Background(), 7, 8, []uint{20}); err != gorm.ErrRecordNotFound {
		t.Fatalf("missing target must be rejected before assignment checks: %v", err)
	}
	if err := db.Create(&authModel.User{ID: 8, UUID: "user-8", Username: "target", Enable: 1, RoleID: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.SetUserRoles(context.Background(), 7, 8, []uint{20}); err != ErrAccessDenied {
		t.Fatalf("assigning a role outside actor ceiling = %v, want ErrAccessDenied", err)
	}
}

func TestNonSuperAdministratorCannotDemoteSuperAdministrator(t *testing.T) {
	db := openAccessTestDB(t)
	roles := []model.Role{
		{RoleID: DefaultUserRoleID, Code: "user", RoleName: "User", IsSystem: true, IsEnabled: true},
		{RoleID: AdminRoleID, Code: "admin", RoleName: "Administrator", IsSystem: true, IsEnabled: true},
		{RoleID: SuperAdminRoleID, Code: "super_admin", RoleName: "Super administrator", IsSystem: true, IsEnabled: true},
	}
	if err := db.Create(&roles).Error; err != nil {
		t.Fatal(err)
	}
	permission := model.Permission{Code: "auth:user-role:write", Name: "Assign roles", Module: "access"}
	if err := db.Create(&permission).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.RolePermission{RoleID: AdminRoleID, PermissionID: permission.ID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]authModel.User{
		{ID: 7, UUID: "admin-7", Username: "admin", Enable: 1, RoleID: AdminRoleID},
		{ID: 8, UUID: "super-8", Username: "super-a", Enable: 1, RoleID: SuperAdminRoleID},
		{ID: 9, UUID: "super-9", Username: "super-b", Enable: 1, RoleID: SuperAdminRoleID},
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]model.UserRole{
		{UserID: 7, RoleID: AdminRoleID},
		{UserID: 8, RoleID: SuperAdminRoleID},
		{UserID: 9, RoleID: SuperAdminRoleID},
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := NewAccessService(db).SetUserRoles(context.Background(), 7, 8, []uint{DefaultUserRoleID}); err != ErrAccessDenied {
		t.Fatalf("non-super demotion=%v, want ErrAccessDenied", err)
	}
}

func TestFrozenSuperAssignmentDoesNotSatisfyLastSuperProtection(t *testing.T) {
	db := openAccessTestDB(t)
	roles := []model.Role{
		{RoleID: AdminRoleID, Code: "admin", RoleName: "Administrator", IsSystem: true, IsEnabled: true},
		{RoleID: SuperAdminRoleID, Code: "super_admin", RoleName: "Super administrator", IsSystem: true, IsEnabled: true},
	}
	if err := db.Create(&roles).Error; err != nil {
		t.Fatal(err)
	}
	permission := model.Permission{Code: "auth:user-role:write", Name: "Assign roles", Module: "access"}
	if err := db.Create(&permission).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.RolePermission{RoleID: AdminRoleID, PermissionID: permission.ID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]authModel.User{
		{ID: 7, UUID: "super-7", Username: "active-super", Enable: 1, RoleID: SuperAdminRoleID},
		{ID: 8, UUID: "super-8", Username: "frozen-super", Enable: 2, RoleID: SuperAdminRoleID},
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]model.UserRole{
		{UserID: 7, RoleID: SuperAdminRoleID},
		{UserID: 8, RoleID: SuperAdminRoleID},
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := NewAccessService(db).SetUserRoles(context.Background(), 7, 7, []uint{AdminRoleID}); err != ErrLastSuperAdmin {
		t.Fatalf("self demotion with only frozen backup=%v, want ErrLastSuperAdmin", err)
	}
	if err := NewAccessService(db).SetUserRoles(context.Background(), 7, 8, []uint{AdminRoleID}); err != nil {
		t.Fatalf("cleaning a frozen user's stale super role = %v, want nil", err)
	}
}

func TestRoleLifecycleProtectsLegacyAssignmentsAndSelfDisable(t *testing.T) {
	db := openAccessTestDB(t)
	role := model.Role{RoleID: 20, Code: "role_manager", RoleName: "Role manager", IsEnabled: true}
	permission := model.Permission{Code: "auth:role:write", Name: "Manage roles", Module: "access"}
	if err := db.Create(&role).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&permission).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.RolePermission{RoleID: role.RoleID, PermissionID: permission.ID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&authModel.User{ID: 7, UUID: "user-7", Username: "operator", Enable: 1, RoleID: role.RoleID}).Error; err != nil {
		t.Fatal(err)
	}
	session := authModel.RefreshSession{
		UserID: 7, FamilyID: "family-legacy-role-000000000000000000001",
		TokenHash: "legacy-hash", ExpiresAt: time.Now().Add(time.Hour), FamilyExpiresAt: time.Now().Add(2 * time.Hour),
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatal(err)
	}

	svc := NewAccessService(db)
	if err := svc.DeleteRole(context.Background(), 99, role.RoleID, ""); err != ErrRoleInUse {
		t.Fatalf("legacy assignment delete = %v, want ErrRoleInUse", err)
	}
	if err := svc.SetRolePermissions(context.Background(), 7, role.RoleID, nil); err != ErrSelfLockout {
		t.Fatalf("legacy self permission removal = %v, want ErrSelfLockout", err)
	}
	disabled := false
	if _, err := svc.UpdateRole(context.Background(), 7, role.RoleID, RolePatch{IsEnabled: &disabled}, ""); err != ErrSelfLockout {
		t.Fatalf("self disable = %v, want ErrSelfLockout", err)
	}
	name := "Updated role manager"
	if _, err := svc.UpdateRole(context.Background(), 7, role.RoleID, RolePatch{Name: &name}, ""); err != nil {
		t.Fatalf("update legacy role: %v", err)
	}
	var active int64
	if err := db.Model(&authModel.RefreshSession{}).Where("user_id = ? AND revoked_at IS NULL", 7).Count(&active).Error; err != nil {
		t.Fatal(err)
	}
	if active != 0 {
		t.Fatalf("active legacy sessions = %d, want 0", active)
	}
}

func TestSetRolePermissionsRejectsSuperAdminAndUnknownPermissions(t *testing.T) {
	db := openAccessTestDB(t)
	if err := db.Create(&authModel.User{ID: 7, UUID: "user-7", Username: "operator", Enable: 1}).Error; err != nil {
		t.Fatalf("create actor: %v", err)
	}
	if err := db.Create(&model.Role{RoleID: 999, Code: "super_admin", RoleName: "Super administrator", IsSystem: true, IsEnabled: true}).Error; err != nil {
		t.Fatalf("create role: %v", err)
	}
	perm := model.Permission{Code: "auth:role:read", Name: "Read roles", Module: "auth", IsSystem: true}
	if err := db.Create(&perm).Error; err != nil {
		t.Fatalf("create permission: %v", err)
	}
	svc := NewAccessService(db)
	if err := svc.SetRolePermissions(context.Background(), 7, 999, []uint{perm.ID}); err != ErrProtectedRole {
		t.Fatalf("super admin update error = %v, want ErrProtectedRole", err)
	}
	if err := db.Create(&model.Role{RoleID: 20, Code: "operator", RoleName: "Operator", IsEnabled: true}).Error; err != nil {
		t.Fatalf("create custom role: %v", err)
	}
	if err := svc.SetRolePermissions(context.Background(), 7, 20, []uint{perm.ID, 999999}); err != ErrUnknownPermission {
		t.Fatalf("unknown permission error = %v, want ErrUnknownPermission", err)
	}
}

func TestRoleAndMenuCodesAreConflictSafe(t *testing.T) {
	db := openAccessTestDB(t)
	svc := NewAccessService(db)
	if err := db.Create(&model.Role{RoleID: AdminRoleID, Code: "admin", RoleName: "Admin", IsSystem: true, IsEnabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateRole(context.Background(), 1, "operator", "Operator", "", "first"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateRole(context.Background(), 1, "operator", "Duplicate", "", "second"); !errors.Is(err, ErrCodeConflict) {
		t.Fatalf("duplicate role error = %v, want ErrCodeConflict", err)
	}

	menu := MenuInput{Code: "operator_menu", Title: "Operator", App: "example-site", Type: "menu", IsVisible: true}
	if _, err := svc.CreateMenu(context.Background(), 1, menu, "first-menu"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateMenu(context.Background(), 1, menu, "second-menu"); !errors.Is(err, ErrCodeConflict) {
		t.Fatalf("duplicate menu error = %v, want ErrCodeConflict", err)
	}
}
