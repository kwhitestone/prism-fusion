package service

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/kwhitestone/prism-fusion/addons/auth/model"
	"github.com/kwhitestone/prism-fusion/global"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrInvalidRefreshToken    = errors.New("invalid refresh token")
	ErrExpiredRefreshToken    = errors.New("expired refresh token")
	ErrRefreshTokenReused     = errors.New("refresh token reuse detected")
	ErrRefreshUserUnavailable = errors.New("refresh user unavailable")
)

const (
	refreshTokenBytes           = 32
	defaultRefreshTTL           = 30 * 24 * time.Hour
	defaultRefreshFamilyTTL     = 90 * 24 * time.Hour
	defaultRefreshRotationGrace = 60 * time.Second
)

type RefreshSessionService struct{}

func (s *RefreshSessionService) Issue(userID uint) (string, time.Time, error) {
	token, _, expiresAt, err := s.IssueWithFamily(userID)
	return token, expiresAt, err
}

func (s *RefreshSessionService) IssueWithFamily(
	userID uint,
) (string, string, time.Time, error) {
	// Retain revoked rows until their natural expiry for reuse detection, then
	// remove them opportunistically so rotation history stays bounded.
	now := time.Now()
	if err := cleanupExpiredRefreshSessions(global.PRISM_DB, now); err != nil && global.PRISM_LOG != nil {
		global.PRISM_LOG.Warn("refresh session cleanup deferred", zap.Error(err))
	}
	familyID := uuid.NewString()
	token, expiresAt, err := s.issue(
		global.PRISM_DB,
		userID,
		familyID,
		now.Add(s.familyTTL()),
	)
	return token, familyID, expiresAt, err
}

func cleanupExpiredRefreshSessions(db *gorm.DB, now time.Time) error {
	// Cleanup is bounded and best-effort: stale history must not turn login into
	// an availability dependency. Zero family expiry rows are retained for the
	// explicit compatibility fallback in rotate().
	return db.Where("expires_at < ?", now).
		Or("family_expires_at > ? AND family_expires_at < ?", time.Time{}, now).
		Limit(500).
		Delete(&model.RefreshSession{}).Error
}

func (s *RefreshSessionService) issue(
	db *gorm.DB,
	userID uint,
	familyID string,
	familyExpiresAt time.Time,
) (string, time.Time, error) {
	token, tokenHash, err := newRefreshToken()
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := minTime(time.Now().Add(s.ttl()), familyExpiresAt)
	session := &model.RefreshSession{
		UserID:          userID,
		FamilyID:        familyID,
		TokenHash:       tokenHash,
		ExpiresAt:       expiresAt,
		FamilyExpiresAt: familyExpiresAt,
	}
	if err := db.Create(session).Error; err != nil {
		return "", time.Time{}, err
	}
	return token, expiresAt, nil
}

func (s *RefreshSessionService) Rotate(
	rawToken string,
	requestID string,
) (uint, string, time.Time, error) {
	return s.rotate(rawToken, requestID, nil)
}

func (s *RefreshSessionService) RotateForActiveUser(
	rawToken string,
	requestID string,
) (*model.User, string, string, time.Time, error) {
	var user model.User
	var familyID string
	_, token, expiresAt, err := s.rotate(rawToken, requestID, func(
		tx *gorm.DB,
		userID uint,
		currentFamilyID string,
	) error {
		familyID = currentFamilyID
		if err := tx.First(&user, userID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrRefreshUserUnavailable
			}
			return err
		}
		if user.Enable != 1 {
			return ErrRefreshUserUnavailable
		}
		return nil
	})
	if err != nil {
		return nil, "", "", time.Time{}, err
	}
	return &user, familyID, token, expiresAt, nil
}

func (s *RefreshSessionService) rotate(
	rawToken string,
	requestID string,
	validateUser func(*gorm.DB, uint, string) error,
) (uint, string, time.Time, error) {
	if len(rawToken) < 32 || len(rawToken) > 512 {
		return 0, "", time.Time{}, ErrInvalidRefreshToken
	}
	tokenHash := hashRefreshToken(rawToken)
	requestHash := hashRotationRequest(requestID)
	var userID uint
	var nextToken string
	var nextExpiry time.Time
	var outcome error

	err := global.PRISM_DB.Transaction(func(tx *gorm.DB) error {
		var current model.RefreshSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("token_hash = ?", tokenHash).First(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvalidRefreshToken
			}
			return err
		}
		userID = current.UserID
		now := time.Now()
		if current.RevokedAt != nil {
			// A refresh response can be lost after the database commit. During a
			// short grace period, derive and return the exact same replacement so
			// a network retry is idempotent rather than revoking a healthy family.
			if current.ReplacedByHash != "" &&
				current.RotationRequestHash != "" &&
				hmac.Equal([]byte(current.RotationRequestHash), []byte(requestHash)) &&
				now.Sub(*current.RevokedAt) <= s.rotationGrace() {
				replacement := deriveRefreshToken(rawToken)
				if hashRefreshToken(replacement) == current.ReplacedByHash {
					var next model.RefreshSession
					if err := tx.Where("token_hash = ? AND revoked_at IS NULL", current.ReplacedByHash).
						First(&next).Error; err == nil && next.ExpiresAt.After(now) {
						if validateUser != nil {
							if err := validateUser(tx, current.UserID, current.FamilyID); err != nil {
								return err
							}
						}
						nextToken = replacement
						nextExpiry = next.ExpiresAt
						return nil
					}
				}
			}
			outcome = ErrRefreshTokenReused
			return revokeFamily(tx, current.FamilyID, now)
		}
		familyExpiresAt := current.FamilyExpiresAt
		if familyExpiresAt.IsZero() {
			// Compatibility for a database migrated from the first opaque-token
			// implementation, before absolute family expiry was recorded.
			familyExpiresAt = current.ExpiresAt
		}
		if !current.ExpiresAt.After(now) || !familyExpiresAt.After(now) {
			outcome = ErrExpiredRefreshToken
			return tx.Model(&current).Update("revoked_at", now).Error
		}
		if validateUser != nil {
			if err := validateUser(tx, current.UserID, current.FamilyID); err != nil {
				return err
			}
		}

		var nextHash string
		nextToken = deriveRefreshToken(rawToken)
		nextHash = hashRefreshToken(nextToken)
		nextExpiry = minTime(now.Add(s.ttl()), familyExpiresAt)
		next := &model.RefreshSession{
			UserID:          current.UserID,
			FamilyID:        current.FamilyID,
			TokenHash:       nextHash,
			ExpiresAt:       nextExpiry,
			FamilyExpiresAt: familyExpiresAt,
		}
		if err := tx.Create(next).Error; err != nil {
			// A database without effective row locks may race on the deterministic
			// replacement hash. If the winner committed the same live replacement,
			// return it idempotently instead of surfacing a duplicate-key failure.
			var existing model.RefreshSession
			if lookupErr := tx.Where("token_hash = ? AND revoked_at IS NULL", nextHash).
				First(&existing).Error; lookupErr == nil && existing.ExpiresAt.After(now) {
				nextExpiry = existing.ExpiresAt
				return nil
			}
			return err
		}
		result := tx.Model(&model.RefreshSession{}).
			Where("id = ? AND revoked_at IS NULL", current.ID).
			Updates(map[string]any{
				"revoked_at":            now,
				"replaced_by_hash":      nextHash,
				"rotation_request_hash": requestHash,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			outcome = ErrRefreshTokenReused
			return revokeFamily(tx, current.FamilyID, now)
		}
		return nil
	})
	if err != nil {
		return 0, "", time.Time{}, err
	}
	if outcome != nil {
		return 0, "", time.Time{}, outcome
	}
	return userID, nextToken, nextExpiry, nil
}

func (s *RefreshSessionService) Revoke(rawToken string) error {
	if len(rawToken) < 32 || len(rawToken) > 512 {
		return nil
	}
	var session model.RefreshSession
	if err := global.PRISM_DB.Where("token_hash = ?", hashRefreshToken(rawToken)).First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	return revokeFamily(global.PRISM_DB, session.FamilyID, time.Now())
}

func (s *RefreshSessionService) IsFamilyActive(
	userID uint,
	familyID string,
) (bool, error) {
	if userID == 0 || familyID == "" {
		return false, nil
	}
	now := time.Now()
	var count int64
	err := global.PRISM_DB.Model(&model.RefreshSession{}).
		Joins("JOIN users ON users.id = refresh_sessions.user_id").
		Where(
			"refresh_sessions.user_id = ? AND refresh_sessions.family_id = ? AND refresh_sessions.revoked_at IS NULL AND refresh_sessions.expires_at > ?",
			userID,
			familyID,
			now,
		).
		Where("refresh_sessions.family_expires_at = ? OR refresh_sessions.family_expires_at > ?", time.Time{}, now).
		Where("users.enable = ? AND users.deleted_at IS NULL", 1).
		Count(&count).Error
	return count > 0, err
}

func (s *RefreshSessionService) RefreshExpiresIn() string {
	if value := global.PRISM_CONFIG.JWT.RefreshExpiresTime; value != "" {
		return value
	}
	return "720h"
}

func (s *RefreshSessionService) ttl() time.Duration {
	return parseDuration(global.PRISM_CONFIG.JWT.RefreshExpiresTime, defaultRefreshTTL)
}

func (s *RefreshSessionService) familyTTL() time.Duration {
	return parseDuration(global.PRISM_CONFIG.JWT.RefreshFamilyExpiresTime, defaultRefreshFamilyTTL)
}

func (s *RefreshSessionService) rotationGrace() time.Duration {
	return parseDuration(global.PRISM_CONFIG.JWT.RefreshRotationGrace, defaultRefreshRotationGrace)
}

func revokeFamily(db *gorm.DB, familyID string, now time.Time) error {
	return db.Model(&model.RefreshSession{}).
		Where("family_id = ? AND revoked_at IS NULL", familyID).
		Update("revoked_at", now).Error
}

func newRefreshToken() (string, string, error) {
	bytes := make([]byte, refreshTokenBytes)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", err
	}
	raw := base64.RawURLEncoding.EncodeToString(bytes)
	return raw, hashRefreshToken(raw), nil
}

func hashRefreshToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func hashRotationRequest(requestID string) string {
	if requestID == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(requestID))
	return hex.EncodeToString(sum[:])
}

func deriveRefreshToken(raw string) string {
	mac := hmac.New(sha256.New, []byte(global.PRISM_CONFIG.JWT.SigningKey))
	_, _ = mac.Write([]byte("prism-refresh-rotation:" + raw))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func minTime(left, right time.Time) time.Time {
	if left.Before(right) {
		return left
	}
	return right
}
