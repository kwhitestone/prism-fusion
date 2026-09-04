package service

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kwhitestone/prism-fusion/addons/auth/model"
	"github.com/kwhitestone/prism-fusion/global"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupUserTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
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

func TestLoginRequiresExplicitlyActiveStatus(t *testing.T) {
	setupUserTestDB(t)
	service := &UserService{}
	for _, status := range []int{0, 2, 3} {
		user := &model.User{Username: fmt.Sprintf("status-%d", status), Enable: status}
		if err := user.SetPassword("correct-password"); err != nil {
			t.Fatal(err)
		}
		if err := global.PRISM_DB.Create(user).Error; err != nil {
			t.Fatal(err)
		}
		if err := global.PRISM_DB.Model(user).UpdateColumn("enable", status).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := service.Login(user.Username, "correct-password"); err == nil {
			t.Fatalf("status %d must not authenticate", status)
		}
	}
	active := &model.User{Username: "active-user", Enable: 1}
	if err := active.SetPassword("correct-password"); err != nil {
		t.Fatal(err)
	}
	if err := global.PRISM_DB.Create(active).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.Login(active.Username, "correct-password"); err != nil {
		t.Fatalf("active user login failed: %v", err)
	}
}

func TestLoginDoesNotRevealAccountState(t *testing.T) {
	setupUserTestDB(t)
	service := &UserService{}
	disabled := &model.User{Username: "disabled-user", Enable: 2}
	if err := disabled.SetPassword("correct-password"); err != nil {
		t.Fatal(err)
	}
	if err := global.PRISM_DB.Create(disabled).Error; err != nil {
		t.Fatal(err)
	}

	for _, attempt := range []struct {
		username string
		password string
	}{
		{username: "missing-user", password: "correct-password"},
		{username: "disabled-user", password: "correct-password"},
		{username: "disabled-user", password: "wrong-password"},
	} {
		if _, err := service.Login(attempt.username, attempt.password); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("Login(%q) error = %v, want ErrInvalidCredentials", attempt.username, err)
		}
	}
}

func TestRegisterRejectsOversizedIdentityFields(t *testing.T) {
	setupUserTestDB(t)
	service := &UserService{}
	for _, attempt := range []struct {
		name     string
		username string
		password string
		nickName string
	}{
		{name: "username", username: strings.Repeat("u", 65), password: "password", nickName: "User"},
		{name: "password", username: "user", password: strings.Repeat("p", 129), nickName: "User"},
		{name: "nickname", username: "user", password: "password", nickName: strings.Repeat("n", 65)},
	} {
		t.Run(attempt.name, func(t *testing.T) {
			if _, err := service.Register(attempt.username, attempt.password, attempt.nickName, 1); !errors.Is(err, ErrInvalidUserInput) {
				t.Fatalf("Register() error = %v, want ErrInvalidUserInput", err)
			}
		})
	}
}

func TestRegisterPersistsUserAndMapsDuplicateUsername(t *testing.T) {
	setupUserTestDB(t)
	service := &UserService{}
	created, err := service.Register("new-user", "secure-password", "New User", 1)
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == 0 || created.Username != "new-user" || !created.CheckPassword("secure-password") {
		t.Fatalf("unexpected registered user: %#v", created)
	}
	if _, err := service.Register("new-user", "another-password", "Duplicate", 1); !errors.Is(err, ErrUsernameExists) {
		t.Fatalf("duplicate registration error=%v, want ErrUsernameExists", err)
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

func TestUserReadAndPasswordChangeContracts(t *testing.T) {
	setupUserTestDB(t)
	user := &model.User{UUID: "reader-1", Username: "reader", NickName: "Reader", Enable: 1}
	if err := user.SetPassword("old-password"); err != nil {
		t.Fatal(err)
	}
	if err := global.PRISM_DB.Create(user).Error; err != nil {
		t.Fatal(err)
	}

	service := &UserService{}
	loaded, err := service.GetUserByID(user.ID)
	if err != nil || loaded.Username != user.Username {
		t.Fatalf("loaded=%#v err=%v", loaded, err)
	}
	users, total, err := service.GetUserList(1, 10)
	if err != nil || total != 1 || len(users) != 1 || users[0].ID != user.ID {
		t.Fatalf("users=%#v total=%d err=%v", users, total, err)
	}
	if err := service.ChangePassword(user.ID, "old-password", "new-password"); err != nil {
		t.Fatal(err)
	}
	changed, err := service.GetUserByID(user.ID)
	if err != nil || !changed.CheckPassword("new-password") || changed.CheckPassword("old-password") {
		t.Fatalf("password change was not persisted: err=%v", err)
	}
}
