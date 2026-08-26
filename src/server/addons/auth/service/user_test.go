package service

import (
	"testing"
	"time"

	"github.com/kwhitestone/prism-fusion/addons/auth/model"
	"github.com/kwhitestone/prism-fusion/global"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupUserTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.RefreshSession{}, &model.AuthRateWindow{}); err != nil {
		t.Fatal(err)
	}
	previous := global.PRISM_DB
	global.PRISM_DB = db
	t.Cleanup(func() { global.PRISM_DB = previous })
}

func TestBootstrapAdminDoesNothingWithoutExplicitCredentials(t *testing.T) {
	setupUserTestDB(t)
	t.Setenv("AUTH_BOOTSTRAP_ADMIN_USERNAME", "")
	t.Setenv("AUTH_BOOTSTRAP_ADMIN_PASSWORD", "")

	if err := (&UserService{}).BootstrapAdminFromEnvironment(); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := global.PRISM_DB.Model(&model.User{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected no default administrator, got %d users", count)
	}
}

func TestBootstrapAdminRequiresStrongExplicitCredentials(t *testing.T) {
	setupUserTestDB(t)
	t.Setenv("AUTH_BOOTSTRAP_ADMIN_USERNAME", "first-admin")
	t.Setenv("AUTH_BOOTSTRAP_ADMIN_PASSWORD", "strong-one-time-password")

	if err := (&UserService{}).BootstrapAdminFromEnvironment(); err != nil {
		t.Fatal(err)
	}
	var admin model.User
	if err := global.PRISM_DB.Where("username = ?", "first-admin").First(&admin).Error; err != nil {
		t.Fatal(err)
	}
	if admin.RoleID != 999 || !admin.CheckPassword("strong-one-time-password") {
		t.Fatal("bootstrap administrator was not created with the requested credentials")
	}
}

func TestLoginRateSubjectUsesStableUserIDAndCanonicalUnknownName(t *testing.T) {
	setupUserTestDB(t)
	user := &model.User{Username: "existing", Enable: 1}
	if err := global.PRISM_DB.Create(user).Error; err != nil {
		t.Fatal(err)
	}
	service := &UserService{}
	subject, err := service.LoginRateSubject("existing")
	if err != nil {
		t.Fatal(err)
	}
	if subject != "user-id:1" {
		t.Fatalf("expected stable user-ID subject, got %q", subject)
	}
	composed, err := service.LoginRateSubject(" Éxample ")
	if err != nil {
		t.Fatal(err)
	}
	decomposed, err := service.LoginRateSubject("e\u0301XAMPLE")
	if err != nil {
		t.Fatal(err)
	}
	if composed != decomposed {
		t.Fatalf("expected equivalent unknown identifiers to share a bucket: %q != %q", composed, decomposed)
	}
}

func TestBootstrapDisablesAndExplicitlyRotatesLegacyDefaultAdmin(t *testing.T) {
	setupUserTestDB(t)
	legacy := &model.User{
		UUID:     "legacy-admin",
		Username: "admin",
		RoleID:   999,
		Enable:   1,
	}
	if err := legacy.SetPassword("admin123"); err != nil {
		t.Fatal(err)
	}
	if err := global.PRISM_DB.Create(legacy).Error; err != nil {
		t.Fatal(err)
	}
	refresh := &model.RefreshSession{
		UserID:    legacy.ID,
		FamilyID:  "legacy-admin-family",
		TokenHash: "legacy-admin-token-hash",
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := global.PRISM_DB.Create(refresh).Error; err != nil {
		t.Fatal(err)
	}
	t.Setenv("AUTH_BOOTSTRAP_ADMIN_USERNAME", "")
	t.Setenv("AUTH_BOOTSTRAP_ADMIN_PASSWORD", "")

	if err := (&UserService{}).BootstrapAdminFromEnvironment(); err == nil {
		t.Fatal("legacy default administrator must block startup until explicitly migrated")
	}
	if err := global.PRISM_DB.First(legacy, legacy.ID).Error; err != nil {
		t.Fatal(err)
	}
	if legacy.Enable != 2 {
		t.Fatal("legacy default administrator must be frozen before startup fails")
	}
	if err := global.PRISM_DB.First(refresh, refresh.ID).Error; err != nil {
		t.Fatal(err)
	}
	if refresh.RevokedAt == nil {
		t.Fatal("legacy administrator refresh families must be revoked")
	}

	t.Setenv("AUTH_BOOTSTRAP_ADMIN_USERNAME", "admin")
	t.Setenv("AUTH_BOOTSTRAP_ADMIN_PASSWORD", "rotated-strong-password")
	if err := (&UserService{}).BootstrapAdminFromEnvironment(); err != nil {
		t.Fatal(err)
	}
	if err := global.PRISM_DB.First(legacy, legacy.ID).Error; err != nil {
		t.Fatal(err)
	}
	if legacy.Enable != 1 || legacy.CheckPassword("admin123") ||
		!legacy.CheckPassword("rotated-strong-password") {
		t.Fatal("explicit migration did not rotate and re-enable the legacy administrator")
	}
}

func TestBootstrapDoesNotTreatOrdinaryAdminNamedUserAsLegacySeed(t *testing.T) {
	setupUserTestDB(t)
	ordinary := &model.User{
		UUID:     "ordinary-admin-name",
		Username: "admin",
		RoleID:   1,
		Enable:   1,
	}
	if err := ordinary.SetPassword("admin123"); err != nil {
		t.Fatal(err)
	}
	if err := global.PRISM_DB.Create(ordinary).Error; err != nil {
		t.Fatal(err)
	}
	t.Setenv("AUTH_BOOTSTRAP_ADMIN_USERNAME", "")
	t.Setenv("AUTH_BOOTSTRAP_ADMIN_PASSWORD", "")

	if err := (&UserService{}).BootstrapAdminFromEnvironment(); err != nil {
		t.Fatalf("ordinary account must not trigger legacy-seed startup failure: %v", err)
	}
	if err := global.PRISM_DB.First(ordinary, ordinary.ID).Error; err != nil {
		t.Fatal(err)
	}
	if ordinary.Enable != 1 {
		t.Fatal("ordinary account was unexpectedly frozen")
	}
}
