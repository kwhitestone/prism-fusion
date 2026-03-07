package service

import (
	"errors"
	"time"

	"whitestone.top/prism-fusion/global"

	"github.com/golang-jwt/jwt/v5"
)

// Claims JWT 自定义声明
type Claims struct {
	UserID   uint   `json:"userId"`
	Username string `json:"username"`
	RoleID   uint   `json:"roleId"`
	jwt.RegisteredClaims
}

// JwtService JWT 服务
type JwtService struct{}

// GenerateToken 签发 JWT Token
func (s *JwtService) GenerateToken(userID uint, username string, roleID uint) (string, error) {
	cfg := global.PRISM_CONFIG.JWT
	signingKey := []byte(cfg.SigningKey)

	expiresTime := parseDuration(cfg.ExpiresTime, 7*24*time.Hour)

	claims := Claims{
		UserID:   userID,
		Username: username,
		RoleID:   roleID,
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

// ParseToken 解析 JWT Token
func (s *JwtService) ParseToken(tokenString string) (*Claims, error) {
	cfg := global.PRISM_CONFIG.JWT
	signingKey := []byte(cfg.SigningKey)

	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		return signingKey, nil
	})
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
	bufferTime := parseDuration(cfg.BufferTime, 24*time.Hour)
	return time.Until(claims.ExpiresAt.Time) < bufferTime
}

// parseDuration 解析时间字符串，如 "7d", "24h", "1d"
func parseDuration(s string, fallback time.Duration) time.Duration {
	if len(s) == 0 {
		return fallback
	}
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
		if err != nil {
			return fallback
		}
		return d
	}
	var num int
	for _, c := range numStr {
		if c >= '0' && c <= '9' {
			num = num*10 + int(c-'0')
		}
	}
	if num == 0 {
		return fallback
	}
	return time.Duration(num) * multiplier
}
