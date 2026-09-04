package service

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/kwhitestone/prism-fusion/global"

	"github.com/golang-jwt/jwt/v5"
)

const (
	AccessTokenType      = "access"
	PrincipalTypeUser    = "user"
	PrincipalTypeService = "service"
)

// Claims JWT 自定义声明
type Claims struct {
	UserID    uint   `json:"userId"`
	Username  string `json:"username"`
	RoleID    uint   `json:"roleId"`
	TokenType string `json:"tokenType,omitempty"`
	Principal string `json:"principal,omitempty"`
	SessionID string `json:"sid,omitempty"`
	jwt.RegisteredClaims
}

// JwtService JWT 服务
type JwtService struct{}

// GenerateToken 签发 JWT Token
func (s *JwtService) GenerateToken(userID uint, username string, roleID uint) (string, error) {
	principal := PrincipalTypeUser
	if roleID == 0 {
		principal = PrincipalTypeService
	}
	return s.generateToken(userID, username, roleID, principal, "")
}

func (s *JwtService) GenerateSessionToken(
	userID uint,
	username string,
	roleID uint,
	sessionID string,
) (string, error) {
	if sessionID == "" {
		return "", errors.New("session ID is required")
	}
	return s.generateToken(userID, username, roleID, PrincipalTypeUser, sessionID)
}

func (s *JwtService) generateToken(
	userID uint,
	username string,
	roleID uint,
	principal string,
	sessionID string,
) (string, error) {
	cfg := global.PRISM_CONFIG.JWT
	signingKey := []byte(cfg.SigningKey)

	expiresTime := parseDuration(cfg.ExpiresTime, 15*time.Minute)

	claims := Claims{
		UserID:    userID,
		Username:  username,
		RoleID:    roleID,
		TokenType: AccessTokenType,
		Principal: principal,
		SessionID: sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiresTime)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    cfg.Issuer,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(signingKey)
}

// ParseAccessToken accepts typed access tokens and leaves legacy-token policy to
// middleware, where human tokens without a server-side session are rejected.
func (s *JwtService) ParseAccessToken(tokenString string) (*Claims, error) {
	claims, err := s.ParseToken(tokenString)
	if err != nil {
		return nil, err
	}
	if claims.TokenType != "" && claims.TokenType != AccessTokenType {
		return nil, fmt.Errorf("unexpected token type %q", claims.TokenType)
	}
	if claims.Principal != "" && claims.Principal != PrincipalTypeUser && claims.Principal != PrincipalTypeService {
		return nil, fmt.Errorf("unexpected principal type %q", claims.Principal)
	}
	return claims, nil
}

// IsServicePrincipal identifies only explicitly typed service credentials.
// Untyped legacy JWTs are treated as human sessions and rejected when they do
// not carry a revocable session ID; role zero alone is never trusted.
func (c *Claims) IsServicePrincipal() bool {
	if c == nil {
		return false
	}
	return c.Principal == PrincipalTypeService
}

// ParseRefreshToken rejects access JWTs at the refresh boundary. New refresh
// credentials are opaque; this method is retained as an explicit type guard.
func (s *JwtService) ParseRefreshToken(tokenString string) (*Claims, error) {
	claims, err := s.ParseToken(tokenString)
	if err != nil {
		return nil, err
	}
	if claims.TokenType != "refresh" {
		return nil, errors.New("not a refresh token")
	}
	return claims, nil
}

func (s *JwtService) AccessExpiresIn() string {
	if value := global.PRISM_CONFIG.JWT.ExpiresTime; value != "" {
		return value
	}
	return "15m"
}

// ParseToken 解析 JWT Token
func (s *JwtService) ParseToken(tokenString string) (*Claims, error) {
	cfg := global.PRISM_CONFIG.JWT
	signingKey := []byte(cfg.SigningKey)

	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		return signingKey, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}
	return nil, errors.New("invalid token")
}

// NeedRefresh 判断 Token 是否在缓冲期内（需要刷新）
func (s *JwtService) NeedRefresh(claims *Claims) bool {
	cfg := global.PRISM_CONFIG.JWT
	bufferTime := parseDuration(cfg.BufferTime, 5*time.Minute)
	return time.Until(claims.ExpiresAt.Time) < bufferTime
}

// parseDuration 解析时间字符串，如 "7d", "24h", "1d"
func parseDuration(s string, fallback time.Duration) time.Duration {
	duration, err := parseConfiguredDuration(s)
	if err != nil {
		return fallback
	}
	return duration
}

func parseConfiguredDuration(value string) (time.Duration, error) {
	if value == "" {
		return 0, errors.New("duration is empty")
	}
	s := value
	lastChar := s[len(s)-1]
	numStr := s[:len(s)-1]
	var multiplier time.Duration
	switch lastChar {
	case 'd', 'D':
		multiplier = 24 * time.Hour
	case 'h', 'H':
		multiplier = time.Hour
	case 'm', 'M':
		multiplier = time.Minute
	default:
		d, err := time.ParseDuration(s)
		if err != nil || d <= 0 {
			return 0, fmt.Errorf("invalid positive duration %q", value)
		}
		return d, nil
	}
	num, err := strconv.ParseUint(numStr, 10, 64)
	if err != nil || num == 0 {
		return 0, fmt.Errorf("invalid positive duration %q", value)
	}
	const maxDuration = time.Duration(1<<63 - 1)
	if num > uint64(maxDuration/multiplier) {
		return 0, fmt.Errorf("duration %q exceeds supported range", value)
	}
	return time.Duration(num) * multiplier, nil
}

func ValidateTokenConfiguration() error {
	cfg := global.PRISM_CONFIG.JWT
	if len(cfg.SigningKey) < 32 ||
		strings.Contains(cfg.SigningKey, "${") ||
		cfg.SigningKey == "Prism-Fusion-Secret-Key" ||
		cfg.SigningKey == "change-me-in-production" {
		return errors.New("jwt.signing-key must be a non-placeholder secret of at least 32 bytes")
	}
	accessTTL := parseDuration(cfg.ExpiresTime, 15*time.Minute)
	refreshTTL := parseDuration(cfg.RefreshExpiresTime, defaultRefreshTTL)
	familyTTL := parseDuration(cfg.RefreshFamilyExpiresTime, defaultRefreshFamilyTTL)
	configured := []struct {
		name  string
		value string
	}{
		{"expires-time", cfg.ExpiresTime},
		{"refresh-expires-time", cfg.RefreshExpiresTime},
		{"refresh-family-expires-time", cfg.RefreshFamilyExpiresTime},
		{"refresh-rotation-grace", cfg.RefreshRotationGrace},
		{"buffer-time", cfg.BufferTime},
	}
	for _, item := range configured {
		if item.value == "" {
			continue
		}
		if _, err := parseConfiguredDuration(item.value); err != nil {
			return fmt.Errorf("jwt.%s: %w", item.name, err)
		}
	}
	if refreshTTL <= accessTTL {
		return errors.New("jwt.refresh-expires-time must be longer than jwt.expires-time")
	}
	if familyTTL < refreshTTL {
		return errors.New("jwt.refresh-family-expires-time must not be shorter than jwt.refresh-expires-time")
	}
	return nil
}
