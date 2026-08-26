package service

import (
	"testing"
	"time"

	"github.com/kwhitestone/prism-fusion/addons/auth/model"
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
