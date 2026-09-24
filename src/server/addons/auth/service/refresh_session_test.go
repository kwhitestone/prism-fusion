package service

import (
	"errors"
	"testing"
	"time"

	"github.com/kwhitestone/prism-fusion/addons/auth/model"
	"github.com/kwhitestone/prism-fusion/config"
	"github.com/kwhitestone/prism-fusion/global"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupRefreshTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.RefreshSession{}); err != nil {
		t.Fatal(err)
	}
	previousDB := global.PRISM_DB
	previousConfig := global.PRISM_CONFIG
	global.PRISM_DB = db
	global.PRISM_CONFIG.JWT = config.JWT{
		SigningKey:               "test-refresh-rotation-signing-key",
		RefreshExpiresTime:       "168h",
		RefreshFamilyExpiresTime: "720h",
		RefreshRotationGrace:     "60s",
	}
	t.Cleanup(func() {
		global.PRISM_DB = previousDB
		global.PRISM_CONFIG = previousConfig
	})
}

func TestRefreshSessionRotationStoresOnlyHashes(t *testing.T) {
	setupRefreshTestDB(t)
	service := &RefreshSessionService{}
	token, expiresAt, err := service.Issue(7)
	if err != nil {
		t.Fatal(err)
	}
	if token == "" {
		t.Fatal("expected opaque refresh token")
	}
	if time.Until(expiresAt) < 167*time.Hour {
		t.Fatalf("unexpected expiry: %v", expiresAt)
	}

	var stored model.RefreshSession
	if err := global.PRISM_DB.First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.TokenHash == token {
		t.Fatal("raw refresh token must never be stored")
	}

	userID, rotated, _, err := service.Rotate(token, "rotation-request-0001")
	if err != nil {
		t.Fatal(err)
	}
	if userID != 7 || rotated == "" || rotated == token {
		t.Fatal("rotation did not return a new token")
	}
	_, replayed, _, err := service.Rotate(token, "rotation-request-0001")
	if err != nil || replayed != rotated {
		t.Fatalf("a lost refresh response must be retry-safe, got token=%q err=%v", replayed, err)
	}
	old := time.Now().Add(-2 * service.rotationGrace())
	if err := global.PRISM_DB.Model(&model.RefreshSession{}).
		Where("token_hash = ?", hashRefreshToken(token)).
		Update("revoked_at", old).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := service.Rotate(token, "rotation-request-0001"); !errors.Is(err, ErrRefreshTokenReused) {
		t.Fatalf("expected reuse detection after the grace period, got %v", err)
	}
	if _, _, _, err := service.Rotate(rotated, "rotation-request-0002"); !errors.Is(err, ErrRefreshTokenReused) {
		t.Fatalf("reuse must revoke the whole session family, got %v", err)
	}
}

func TestRefreshDoesNotRotateForMissingOrDisabledUser(t *testing.T) {
	setupRefreshTestDB(t)
	service := &RefreshSessionService{}
	token, _, err := service.Issue(9)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := service.RotateForActiveUser(token, "rotation-request-0003"); !errors.Is(err, ErrRefreshUserUnavailable) {
		t.Fatalf("expected missing user rejection, got %v", err)
	}

	user := &model.User{ID: 9, Username: "disabled", Enable: 2}
	if err := global.PRISM_DB.Create(user).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := service.RotateForActiveUser(token, "rotation-request-0003"); !errors.Is(err, ErrRefreshUserUnavailable) {
		t.Fatalf("expected disabled user rejection, got %v", err)
	}
	if err := global.PRISM_DB.Model(user).Update("enable", 0).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := service.RotateForActiveUser(token, "rotation-request-0003"); !errors.Is(err, ErrRefreshUserUnavailable) {
		t.Fatalf("expected non-active user rejection, got %v", err)
	}

	if err := global.PRISM_DB.Model(user).Update("enable", 1).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := service.RotateForActiveUser(token, "rotation-request-0003"); err != nil {
		t.Fatalf("a failed user validation must not consume the token: %v", err)
	}
}

func TestFamilyActivityTracksLogout(t *testing.T) {
	setupRefreshTestDB(t)
	service := &RefreshSessionService{}
	user := &model.User{ID: 14, Username: "session-owner", Enable: 1}
	if err := global.PRISM_DB.Create(user).Error; err != nil {
		t.Fatal(err)
	}
	token, familyID, _, err := service.IssueWithFamily(14)
	if err != nil {
		t.Fatal(err)
	}
	active, err := service.IsFamilyActive(14, familyID)
	if err != nil || !active {
		t.Fatalf("expected issued family to be active, active=%v err=%v", active, err)
	}
	if err := global.PRISM_DB.Model(user).Update("enable", 2).Error; err != nil {
		t.Fatal(err)
	}
	active, err = service.IsFamilyActive(14, familyID)
	if err != nil || active {
		t.Fatalf("frozen account family must be inactive, active=%v err=%v", active, err)
	}
	if err := service.Revoke(token); err != nil {
		t.Fatal(err)
	}
	active, err = service.IsFamilyActive(14, familyID)
	if err != nil || active {
		t.Fatalf("expected revoked family to be inactive, active=%v err=%v", active, err)
	}
}

func TestRefreshReplayRequiresTheOriginalRequestID(t *testing.T) {
	setupRefreshTestDB(t)
	service := &RefreshSessionService{}
	token, _, err := service.Issue(10)
	if err != nil {
		t.Fatal(err)
	}
	_, rotated, _, err := service.Rotate(token, "rotation-request-original")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := service.Rotate(token, "rotation-request-attacker"); !errors.Is(err, ErrRefreshTokenReused) {
		t.Fatalf("expected mismatched request ID to trigger reuse detection, got %v", err)
	}
	if _, _, _, err := service.Rotate(rotated, "rotation-request-next"); !errors.Is(err, ErrRefreshTokenReused) {
		t.Fatalf("mismatched replay must revoke the family, got %v", err)
	}
}

func TestIssueCleanupPreservesLegacyRowsWithoutFamilyExpiry(t *testing.T) {
	setupRefreshTestDB(t)
	legacy := &model.RefreshSession{
		UserID:    12,
		FamilyID:  "legacy-family",
		TokenHash: hashRefreshToken("legacy-refresh-token-with-at-least-32-bytes"),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := global.PRISM_DB.Create(legacy).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := (&RefreshSessionService{}).Issue(13); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := global.PRISM_DB.Model(&model.RefreshSession{}).
		Where("id = ?", legacy.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("cleanup deleted a legacy refresh row with no family expiry")
	}
}

func TestRefreshRotationCannotExtendPastFamilyExpiry(t *testing.T) {
	setupRefreshTestDB(t)
	service := &RefreshSessionService{}
	token, _, err := service.Issue(11)
	if err != nil {
		t.Fatal(err)
	}
	familyExpiry := time.Now().Add(30 * time.Minute)
	if err := global.PRISM_DB.Model(&model.RefreshSession{}).
		Where("token_hash = ?", hashRefreshToken(token)).
		Updates(map[string]any{
			"expires_at":        familyExpiry,
			"family_expires_at": familyExpiry,
		}).Error; err != nil {
		t.Fatal(err)
	}
	_, _, rotatedExpiry, err := service.Rotate(token, "rotation-request-0004")
	if err != nil {
		t.Fatal(err)
	}
	if rotatedExpiry.After(familyExpiry) {
		t.Fatalf("rotation extended beyond family expiry: got %v, cap %v", rotatedExpiry, familyExpiry)
	}
}

func TestRefreshMetadataAndIdempotentUnknownRevocation(t *testing.T) {
	setupRefreshTestDB(t)
	service := &RefreshSessionService{}
	if got := service.RefreshExpiresIn(); got != "168h" {
		t.Fatalf("refresh expiry=%q", got)
	}
	if err := service.Revoke("short"); err != nil {
		t.Fatalf("short unknown token revoke=%v", err)
	}
	if err := service.Revoke("unknown-refresh-token-with-at-least-thirty-two-bytes"); err != nil {
		t.Fatalf("unknown token revoke=%v", err)
	}
}

func TestAlignedRefreshWindows(t *testing.T) {
	setupRefreshTestDB(t)
	global.PRISM_CONFIG.JWT.RefreshExpiresTime = "87600h"
	global.PRISM_CONFIG.JWT.RefreshFamilyExpiresTime = "87600h"
	if err := ValidateTokenConfiguration(); err != nil {
		t.Fatal(err)
	}
	service := &RefreshSessionService{}
	if err := global.PRISM_DB.Create(&model.User{ID: 15, Username: "aligned", Enable: 1}).Error; err != nil {
		t.Fatal(err)
	}
	token, family, expiry, err := service.IssueWithFamily(15)
	if err != nil {
		t.Fatal(err)
	}
	var original model.RefreshSession
	if err := global.PRISM_DB.Where("family_id = ?", family).First(&original).Error; err != nil {
		t.Fatal(err)
	}
	if !expiry.Equal(original.FamilyExpiresAt) ||
		original.FamilyExpiresAt.Sub(original.CreatedAt) < 87599*time.Hour {
		t.Fatalf("expected aligned ten-year expiry: %+v", original)
	}
	// Shift the issued row back 721 hours to exercise rotation after the old cap.
	age := 721 * time.Hour
	familyExpiry := original.FamilyExpiresAt.Add(-age)
	if err := global.PRISM_DB.Model(&original).Updates(map[string]any{
		"created_at": original.CreatedAt.Add(-age), "expires_at": expiry.Add(-age),
		"family_expires_at": familyExpiry,
	}).Error; err != nil {
		t.Fatal(err)
	}
	_, rotatedFamily, rotated, rotatedExpiry, err := service.RotateForActiveUser(token, "aligned-original")
	if err != nil || rotatedFamily != family || !rotatedExpiry.Equal(familyExpiry) {
		t.Fatalf("rotation after old cap: family=%q expiry=%v err=%v", rotatedFamily, rotatedExpiry, err)
	}
	active, err := service.IsFamilyActive(15, family)
	if err != nil || !active {
		t.Fatalf("aligned family inactive: active=%v err=%v", active, err)
	}
	if _, _, _, err := service.Rotate(token, "aligned-reuse"); !errors.Is(err, ErrRefreshTokenReused) {
		t.Fatalf("expected reuse detection, got %v", err)
	}
	active, err = service.IsFamilyActive(15, family)
	if err != nil || active {
		t.Fatalf("reused family still active: active=%v err=%v", active, err)
	}
	if _, _, _, err := service.Rotate(rotated, "aligned-next"); !errors.Is(err, ErrRefreshTokenReused) {
		t.Fatalf("replacement survived family revocation: %v", err)
	}
}

func TestAlignedConfigPreservesExistingFamilyExpiry(t *testing.T) {
	setupRefreshTestDB(t)
	service := &RefreshSessionService{}
	token, family, _, err := service.IssueWithFamily(11)
	if err != nil {
		t.Fatal(err)
	}
	var original model.RefreshSession
	if err := global.PRISM_DB.Where("family_id = ?", family).First(&original).Error; err != nil {
		t.Fatal(err)
	}
	global.PRISM_CONFIG.JWT.RefreshExpiresTime = "87600h"
	global.PRISM_CONFIG.JWT.RefreshFamilyExpiresTime = "87600h"
	_, rotated, expiry, err := service.Rotate(token, "existing-family")
	if err != nil || !expiry.Equal(original.FamilyExpiresAt) {
		t.Fatalf("existing family cap changed: expiry=%v err=%v", expiry, err)
	}
	// Stored family expiry remains authoritative even with a live token expiry.
	if err := global.PRISM_DB.Model(&model.RefreshSession{}).
		Where("token_hash = ?", hashRefreshToken(rotated)).
		Update("family_expires_at", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := service.Rotate(rotated, "existing-expired"); !errors.Is(err, ErrExpiredRefreshToken) {
		t.Fatalf("old family cap no longer enforced: %v", err)
	}
}
