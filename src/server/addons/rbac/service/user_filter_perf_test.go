package service

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	authModel "github.com/kwhitestone/prism-fusion/addons/auth/model"
	"github.com/kwhitestone/prism-fusion/addons/rbac/model"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestListUsersRoleFilterPerformanceMySQL is the A10 check: 10k synthetic
// users on a throwaway MySQL, first page p95 (100 samples after warm-up) < 300ms and EXPLAIN must use
// idx_user_roles_role. Opt-in only (needs a disposable database):
//
//	RBAC_PERF_MYSQL_DSN='root:pw@tcp(127.0.0.1:26999)/ac1?parseTime=true' go test -run PerformanceMySQL -v ./addons/rbac/service/
func TestListUsersRoleFilterPerformanceMySQL(t *testing.T) {
	dsn := os.Getenv("RBAC_PERF_MYSQL_DSN")
	if dsn == "" {
		t.Skip("RBAC_PERF_MYSQL_DSN not set")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	tables := []any{&authModel.User{}, &model.Role{}, &model.UserRole{}}
	if err := db.Migrator().DropTable(tables...); err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(tables...); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]model.Role{
		{RoleID: DefaultUserRoleID, Code: "user", RoleName: "User", IsSystem: true, IsEnabled: true},
		{RoleID: AdminRoleID, Code: "admin", RoleName: "Admin", IsSystem: true, IsEnabled: true},
		{RoleID: SuperAdminRoleID, Code: "super_admin", RoleName: "Super", IsSystem: true, IsEnabled: true},
	}).Error; err != nil {
		t.Fatal(err)
	}
	const total = 10000
	users := make([]authModel.User, 0, total)
	assignments := make([]model.UserRole, 0, total)
	for i := 1; i <= total; i++ {
		user := authModel.User{ID: uint(i), UUID: fmt.Sprintf("perf-%d", i), Username: fmt.Sprintf("perf_user_%05d", i), NickName: fmt.Sprintf("Perf %d", i), Email: fmt.Sprintf("p%d@example.test", i), Enable: 1}
		switch {
		case i%100 == 0: // 1% super admins with explicit rows
			user.RoleID = SuperAdminRoleID
			assignments = append(assignments, model.UserRole{UserID: user.ID, RoleID: SuperAdminRoleID})
		case i%20 == 0: // ~4% admins with explicit rows
			user.RoleID = AdminRoleID
			assignments = append(assignments, model.UserRole{UserID: user.ID, RoleID: AdminRoleID})
		case i%7 == 0: // ~14% legacy-only users (role_id without user_roles)
			user.RoleID = AdminRoleID
			if i%2 == 0 {
				user.RoleID = DefaultUserRoleID
			}
		case i%11 == 0: // unassigned
		default:
			user.RoleID = DefaultUserRoleID
			assignments = append(assignments, model.UserRole{UserID: user.ID, RoleID: DefaultUserRoleID})
		}
		users = append(users, user)
	}
	if err := db.CreateInBatches(users, 1000).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.CreateInBatches(assignments, 1000).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("ANALYZE TABLE users, user_roles, roles").Error; err != nil {
		t.Fatal(err)
	}

	// Capture the SQL ListUsers issues so EXPLAIN runs on exactly that text.
	var captured []string
	if err := db.Callback().Query().After("gorm:query").Register("ac1:capture", func(tx *gorm.DB) {
		captured = append(captured, tx.Dialector.Explain(tx.Statement.SQL.String(), tx.Statement.Vars...))
	}); err != nil {
		t.Fatal(err)
	}
	access := NewAccessService(db)
	ctx := context.Background()
	for _, filter := range [][]string{{}, {"admin"}, {"admin", "super_admin"}, {"none"}, {"user", "none"}} {
		captured = captured[:0]
		page, err := access.ListUsers(ctx, 1, 20, "", filter...)
		if err != nil {
			t.Fatal(err)
		}
		listSQL := ""
		for _, statement := range captured {
			if strings.Contains(statement, "FROM `users`") && strings.Contains(statement, "LIMIT") {
				listSQL = statement
			}
		}
		if listSQL == "" {
			t.Fatalf("list SQL not captured: %v", captured)
		}
		type explainRow struct {
			ID           *int
			SelectType   *string `gorm:"column:select_type"`
			Table        *string
			Type         *string
			PossibleKeys *string `gorm:"column:possible_keys"`
			Key          *string
			Rows         *int64
			Extra        *string
		}
		var plan []explainRow
		if err := db.Raw("EXPLAIN " + listSQL).Scan(&plan).Error; err != nil {
			t.Fatal(err)
		}
		usedRoleIndex := false
		for _, row := range plan {
			key := ""
			if row.Key != nil {
				key = *row.Key
			}
			if row.Table != nil && *row.Table == "user_roles" && key == "idx_user_roles_role" {
				usedRoleIndex = true
			}
			t.Logf("EXPLAIN roles=%v table=%v type=%v key=%q rows=%v", filter, deref(row.Table), deref(row.Type), key, derefInt(row.Rows))
		}
		if !usedRoleIndex && len(filter) > 0 && filter[0] != "none" {
			t.Errorf("roles=%v: EXPLAIN did not use idx_user_roles_role", filter)
		}

		for i := 0; i < 5; i++ { // warm-up: buffer pool and connection pool
			if _, err := access.ListUsers(ctx, 1, 20, "", filter...); err != nil {
				t.Fatal(err)
			}
		}
		durations := make([]time.Duration, 0, 100)
		for i := 0; i < 100; i++ {
			started := time.Now()
			if _, err := access.ListUsers(ctx, 1, 20, "", filter...); err != nil {
				t.Fatal(err)
			}
			durations = append(durations, time.Since(started))
		}
		sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
		p95 := durations[len(durations)*95/100-1]
		t.Logf("roles=%v total=%d first-page p50=%s p95=%s", filter, page.Total, durations[len(durations)/2], p95)
		if p95 >= 300*time.Millisecond {
			t.Errorf("roles=%v: p95 %s >= 300ms", filter, p95)
		}
	}
	counts, err := access.RoleUserCounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if _, err := access.RoleUserCounts(ctx); err != nil {
		t.Fatal(err)
	}
	t.Logf("role counts=%v computed in %s", counts, time.Since(started))
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func derefInt(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
