package service

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"

	authModel "github.com/kwhitestone/prism-fusion/addons/auth/model"
	"github.com/kwhitestone/prism-fusion/addons/rbac/model"
	"gorm.io/gorm"
)

// seedRoleFilterFixture covers every assignment shape the role filter must
// honour: explicit user_roles rows, the legacy users.role_id fallback, a
// user_roles row that overrides a stale role_id, unassigned and dangling
// role_id values, and a soft-deleted user that must never be listed.
func seedRoleFilterFixture(t *testing.T) (*gorm.DB, *AccessService) {
	t.Helper()
	db := openAccessTestDB(t)
	roles := []model.Role{
		{RoleID: DefaultUserRoleID, Code: "user", RoleName: "User", IsSystem: true, IsEnabled: true},
		{RoleID: AdminRoleID, Code: "admin", RoleName: "Admin", IsSystem: true, IsEnabled: true},
		{RoleID: SuperAdminRoleID, Code: "super_admin", RoleName: "Super", IsSystem: true, IsEnabled: true},
		{RoleID: 10, Code: "auditor", RoleName: "Auditor", IsEnabled: true},
	}
	if err := db.Create(&roles).Error; err != nil {
		t.Fatal(err)
	}
	users := []authModel.User{
		{ID: 1, UUID: "u1", Username: "alice", Enable: 1},
		{ID: 2, UUID: "u2", Username: "bob", Enable: 1},
		{ID: 3, UUID: "u3", Username: "carol", Enable: 1},
		{ID: 4, UUID: "u4", Username: "dave", Enable: 2, RoleID: AdminRoleID},
		{ID: 5, UUID: "u5", Username: "erin", Enable: 1},
		{ID: 6, UUID: "u6", Username: "frank", Enable: 1, RoleID: AdminRoleID},
		{ID: 7, UUID: "u7", Username: "gina", Enable: 1, RoleID: 555},
		{ID: 8, UUID: "u8", Username: "hank", Enable: 1},
		{ID: 9, UUID: "u9", Username: "ivan", Enable: 1, RoleID: SuperAdminRoleID},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]model.UserRole{
		{UserID: 1, RoleID: DefaultUserRoleID},
		{UserID: 2, RoleID: AdminRoleID},
		{UserID: 3, RoleID: AdminRoleID},
		{UserID: 3, RoleID: SuperAdminRoleID},
		{UserID: 6, RoleID: DefaultUserRoleID},
		{UserID: 8, RoleID: AdminRoleID},
		{UserID: 9, RoleID: SuperAdminRoleID},
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&authModel.User{}, 8).Error; err != nil {
		t.Fatal(err)
	}
	return db, NewAccessService(db)
}

func listedIDs(t *testing.T, page *UserPage) []uint {
	t.Helper()
	ids := make([]uint, 0, len(page.Items))
	for _, item := range page.Items {
		ids = append(ids, item.ID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func sameIDs(a, b []uint) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestListUsersFiltersByRoleUnionWithLegacyFallback(t *testing.T) {
	_, access := seedRoleFilterFixture(t)
	ctx := context.Background()
	cases := []struct {
		name  string
		roles []string
		want  []uint
	}{
		{name: "no filter lists every live user", roles: nil, want: []uint{1, 2, 3, 4, 5, 6, 7, 9}},
		{name: "blank entries are ignored", roles: []string{"", " "}, want: []uint{1, 2, 3, 4, 5, 6, 7, 9}},
		{name: "A2 single role uses user_roles then role_id fallback", roles: []string{"admin"}, want: []uint{2, 3, 4}},
		{name: "A3 union is deduplicated", roles: []string{"admin", "super_admin"}, want: []uint{2, 3, 4, 9}},
		{name: "repeated codes are harmless", roles: []string{"admin", " admin "}, want: []uint{2, 3, 4}},
		{name: "none means no visible role", roles: []string{"none"}, want: []uint{5, 7}},
		{name: "none unions with a role", roles: []string{"user", "none"}, want: []uint{1, 5, 6, 7}},
		{name: "A6 role without members", roles: []string{"auditor"}, want: []uint{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			page, err := access.ListUsers(ctx, 1, 100, "", tc.roles...)
			if err != nil {
				t.Fatal(err)
			}
			if got := listedIDs(t, page); !sameIDs(got, tc.want) || page.Total != int64(len(tc.want)) {
				t.Fatalf("ids=%v total=%d, want %v", got, page.Total, tc.want)
			}
			if page.Items == nil {
				t.Fatal("items must encode as [] rather than null")
			}
		})
	}
}

func TestListUsersRoleFilterCombinesWithSearchAndPaging(t *testing.T) {
	_, access := seedRoleFilterFixture(t)
	ctx := context.Background()
	page, err := access.ListUsers(ctx, 1, 100, "ca", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if got := listedIDs(t, page); !sameIDs(got, []uint{3}) || page.Total != 1 {
		t.Fatalf("search+role ids=%v total=%d", got, page.Total)
	}
	second, err := access.ListUsers(ctx, 2, 2, "", "admin", "super_admin")
	if err != nil {
		t.Fatal(err)
	}
	if got := listedIDs(t, second); !sameIDs(got, []uint{4, 9}) || second.Total != 4 {
		t.Fatalf("page 2 ids=%v total=%d", got, second.Total)
	}
	for _, item := range second.Items {
		if item.ID == 4 && (len(item.RoleCodes) != 1 || item.RoleCodes[0] != "admin") {
			t.Fatalf("legacy fallback user must still display its role: %#v", item)
		}
	}
}

func TestListUsersRejectsUnknownOrTooManyRoleCodes(t *testing.T) {
	_, access := seedRoleFilterFixture(t)
	ctx := context.Background()
	tooMany := strings.Split(strings.Repeat("admin,", 11), ",")[:11]
	for name, roles := range map[string][]string{
		"unknown":  {"admin", "ghost"},
		"too many": tooMany,
	} {
		if _, err := access.ListUsers(ctx, 1, 20, "", roles...); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("%s: err=%v, want ErrInvalidInput", name, err)
		}
	}
	ten := []string{"admin", "user", "super_admin", "auditor", "none", "admin", "user", "none", "auditor", "admin"}
	if _, err := access.ListUsers(ctx, 1, 20, "", ten...); err != nil {
		t.Fatalf("ten codes must be accepted: %v", err)
	}
}

func TestRoleUserCountsMatchFilteredTotals(t *testing.T) {
	db, access := seedRoleFilterFixture(t)
	ctx := context.Background()
	counts, err := access.RoleUserCounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := map[uint]int64{DefaultUserRoleID: 2, AdminRoleID: 3, SuperAdminRoleID: 2}
	for roleID, expected := range want {
		if counts[roleID] != expected {
			t.Fatalf("role %d count=%d want %d (all=%v)", roleID, counts[roleID], expected, counts)
		}
	}
	if counts[10] != 0 {
		t.Fatalf("auditor count=%d", counts[10])
	}
	var roles []model.Role
	if err := db.Find(&roles).Error; err != nil {
		t.Fatal(err)
	}
	for _, role := range roles {
		page, err := access.ListUsers(ctx, 1, 1, "", role.Code)
		if err != nil {
			t.Fatal(err)
		}
		if page.Total != counts[role.RoleID] {
			t.Fatalf("A1 %s: count %d != filtered total %d", role.Code, counts[role.RoleID], page.Total)
		}
	}
}

func TestUsersRoleIDIsIndexed(t *testing.T) {
	db := openAccessTestDB(t)
	if !db.Migrator().HasIndex(&authModel.User{}, "idx_users_role_id") {
		t.Fatal("users.role_id must be indexed for the legacy role fallback filter")
	}
}

func TestCreateRoleReservesNoneFilterKeyword(t *testing.T) {
	_, access := seedRoleFilterFixture(t)
	if _, err := access.CreateRole(context.Background(), 1, "none", "None", "", ""); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("role code none must stay reserved for the unassigned filter: %v", err)
	}
}
