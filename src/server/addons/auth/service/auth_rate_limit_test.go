package service

import (
	"testing"
	"time"

	"github.com/kwhitestone/prism-fusion/addons/auth/model"
	"github.com/kwhitestone/prism-fusion/global"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAuthRateLimitIsSharedByNormalizedAccount(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.AuthRateWindow{}); err != nil {
		t.Fatal(err)
	}
	limiter := &AuthRateLimitService{DB: db}
	now := time.Date(2026, 8, 26, 12, 34, 30, 0, time.UTC)

	for _, account := range []string{"Alice", " alice ", "ALICE"} {
		allowed, err := limiter.AllowAt("login-account", account, 3, now)
		if err != nil || !allowed {
			t.Fatalf("expected request for %q to be allowed, allowed=%v err=%v", account, allowed, err)
		}
	}
	allowed, err := limiter.AllowAt("login-account", "alice", 3, now)
	if err != nil {
		t.Fatal(err)
	}
	if allowed {
		t.Fatal("expected the shared account bucket to reject request 4")
	}
}

func TestAuthRateLimitValidatesStorageAndInput(t *testing.T) {
	previousDB := global.PRISM_DB
	global.PRISM_DB = nil
	t.Cleanup(func() { global.PRISM_DB = previousDB })
	limiter := &AuthRateLimitService{}
	if _, err := limiter.Allow("login-account", "alice", 1); err == nil {
		t.Fatal("missing rate-limit database must fail closed")
	}

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.AuthRateWindow{}); err != nil {
		t.Fatal(err)
	}
	limiter.DB = db
	if _, err := limiter.AllowAt("", "alice", 1, time.Now()); err == nil {
		t.Fatal("empty scope must be rejected")
	}
	if _, err := limiter.AllowAt("login-account", "", 1, time.Now()); err == nil {
		t.Fatal("empty subject must be rejected")
	}
	if _, err := limiter.AllowAt("login-account", "alice", 0, time.Now()); err == nil {
		t.Fatal("zero limit must be rejected")
	}
}

func TestAuthRateLimitUsesAFreshFixedWindow(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.AuthRateWindow{}); err != nil {
		t.Fatal(err)
	}
	limiter := &AuthRateLimitService{DB: db}
	now := time.Date(2026, 8, 26, 12, 34, 59, 0, time.UTC)
	allowed, err := limiter.AllowAt("login-ip", "127.0.0.1", 1, now)
	if err != nil || !allowed {
		t.Fatalf("first request should pass, allowed=%v err=%v", allowed, err)
	}
	allowed, err = limiter.AllowAt("login-ip", "127.0.0.1", 1, now.Add(time.Second))
	if err != nil || !allowed {
		t.Fatalf("new minute should reset the bucket, allowed=%v err=%v", allowed, err)
	}
}
