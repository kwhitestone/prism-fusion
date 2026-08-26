package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"sync/atomic"
	"time"

	"github.com/kwhitestone/prism-fusion/addons/auth/model"
	"github.com/kwhitestone/prism-fusion/global"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrAuthRateLimited = errors.New("认证请求过于频繁，请稍后重试")

const authRateWindowDuration = time.Minute

type AuthRateLimitService struct {
	DB *gorm.DB
}

var rateLimitCleanupCounter atomic.Uint64

func (s *AuthRateLimitService) database() *gorm.DB {
	if s.DB != nil {
		return s.DB
	}
	return global.PRISM_DB
}

func normalizeRateSubject(scope, subject string) string {
	subject = strings.TrimSpace(subject)
	if strings.HasSuffix(scope, "-account") {
		return strings.ToLower(subject)
	}
	return subject
}

func rateWindowKey(scope, subject string, bucket time.Time) string {
	digest := sha256.Sum256([]byte(
		scope + "\x00" + normalizeRateSubject(scope, subject) + "\x00" +
			bucket.UTC().Format(time.RFC3339),
	))
	return hex.EncodeToString(digest[:])
}

func (s *AuthRateLimitService) Allow(scope, subject string, limit uint) (bool, error) {
	return s.AllowAt(scope, subject, limit, time.Now())
}

func (s *AuthRateLimitService) AllowAt(
	scope string,
	subject string,
	limit uint,
	now time.Time,
) (bool, error) {
	db := s.database()
	if db == nil {
		return false, errors.New("auth rate-limit database is unavailable")
	}
	if scope == "" || normalizeRateSubject(scope, subject) == "" || limit == 0 {
		return false, errors.New("auth rate-limit input is invalid")
	}
	bucket := now.UTC().Truncate(authRateWindowDuration)
	row := model.AuthRateWindow{
		KeyHash:   rateWindowKey(scope, subject, bucket),
		Scope:     scope,
		Count:     1,
		ExpiresAt: bucket.Add(2 * authRateWindowDuration),
	}
	err := db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "key_hash"}},
		DoUpdates: clause.Assignments(map[string]any{
			"count":      gorm.Expr("count + ?", 1),
			"updated_at": now.UTC(),
		}),
	}).Create(&row).Error
	if err != nil {
		return false, err
	}
	var count uint
	if err := db.Model(&model.AuthRateWindow{}).
		Where("key_hash = ?", row.KeyHash).
		Pluck("count", &count).Error; err != nil {
		return false, err
	}
	if rateLimitCleanupCounter.Add(1)%1024 == 0 {
		_ = db.Where("expires_at < ?", now.UTC()).Delete(&model.AuthRateWindow{}).Error
	}
	return count <= limit, nil
}
