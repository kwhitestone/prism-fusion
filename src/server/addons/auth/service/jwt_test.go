package service

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kwhitestone/prism-fusion/config"
	"github.com/kwhitestone/prism-fusion/global"
)

func TestGeneratedJWTIsAccessOnly(t *testing.T) {
	previous := global.PRISM_CONFIG
	global.PRISM_CONFIG.JWT = config.JWT{
		SigningKey:  "test-signing-key-with-sufficient-entropy",
		ExpiresTime: "24h",
		Issuer:      "test-auth",
	}
	t.Cleanup(func() { global.PRISM_CONFIG = previous })

	token, err := (&JwtService{}).GenerateToken(1, "alice", 1)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := (&JwtService{}).ParseAccessToken(token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.TokenType != AccessTokenType {
		t.Fatalf("unexpected token type %q", claims.TokenType)
	}
	if _, err := (&JwtService{}).ParseRefreshToken(token); err == nil {
		t.Fatal("an access token must not be accepted as a refresh token")
	}
}

func TestSessionJWTCarriesRevocableFamilyID(t *testing.T) {
	previous := global.PRISM_CONFIG
	global.PRISM_CONFIG.JWT = config.JWT{
		SigningKey:  "test-signing-key-with-sufficient-entropy",
		ExpiresTime: "15m",
	}
	t.Cleanup(func() { global.PRISM_CONFIG = previous })
	token, err := (&JwtService{}).GenerateSessionToken(1, "alice", 1, "family-123")
	if err != nil {
		t.Fatal(err)
	}
	claims, err := (&JwtService{}).ParseAccessToken(token)
	if err != nil || claims.SessionID != "family-123" {
		t.Fatalf("unexpected session claims: %#v err=%v", claims, err)
	}
}

func TestJWTPrincipalTypeDistinguishesServicesFromLegacyRoleZeroUsers(t *testing.T) {
	previous := global.PRISM_CONFIG
	global.PRISM_CONFIG.JWT = config.JWT{
		SigningKey:  "test-signing-key-with-sufficient-entropy",
		ExpiresTime: "15m",
	}
	t.Cleanup(func() { global.PRISM_CONFIG = previous })

	serviceToken, err := (&JwtService{}).GenerateToken(7, "core", 0)
	if err != nil {
		t.Fatal(err)
	}
	serviceClaims, err := (&JwtService{}).ParseAccessToken(serviceToken)
	if err != nil || !serviceClaims.IsServicePrincipal() {
		t.Fatalf("service claims=%#v err=%v", serviceClaims, err)
	}

	userToken, err := (&JwtService{}).GenerateSessionToken(8, "legacy-user", 0, "family-legacy")
	if err != nil {
		t.Fatal(err)
	}
	userClaims, err := (&JwtService{}).ParseAccessToken(userToken)
	if err != nil || userClaims.IsServicePrincipal() {
		t.Fatalf("role-zero session claims=%#v err=%v", userClaims, err)
	}
}

func TestAccessParserRejectsUnexpectedSigningAlgorithm(t *testing.T) {
	previous := global.PRISM_CONFIG
	global.PRISM_CONFIG.JWT = config.JWT{SigningKey: "test-signing-key-with-sufficient-entropy"}
	t.Cleanup(func() { global.PRISM_CONFIG = previous })

	claims := Claims{RegisteredClaims: jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS512, claims).
		SignedString([]byte(global.PRISM_CONFIG.JWT.SigningKey))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (&JwtService{}).ParseAccessToken(token); err == nil {
		t.Fatal("expected HS512 token to be rejected")
	}
}

// UNI PR-1: the parse side must reject tokens minted by a different issuer.
// An empty configured issuer stays unchecked so the framework's own fixtures
// and any deployment that does not set `issuer` keep working.
func TestAccessParserEnforcesConfiguredIssuer(t *testing.T) {
	previous := global.PRISM_CONFIG
	t.Cleanup(func() { global.PRISM_CONFIG = previous })

	const signingKey = "test-signing-key-with-sufficient-entropy"

	// mint signs a token with the given issuer, independent of the parse-side config.
	mint := func(t *testing.T, issuer string) string {
		t.Helper()
		claims := Claims{
			TokenType: AccessTokenType,
			Principal: PrincipalTypeUser,
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
				Issuer:    issuer,
			},
		}
		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(signingKey))
		if err != nil {
			t.Fatal(err)
		}
		return token
	}

	for _, tc := range []struct {
		name       string
		configured string
		tokenIss   string
		wantErr    bool
	}{
		{name: "matching issuer passes", configured: "nucleagent-auth", tokenIss: "nucleagent-auth"},
		{name: "wrong issuer rejected", configured: "nucleagent-auth", tokenIss: "nucleagent-core", wantErr: true},
		{name: "missing issuer rejected", configured: "nucleagent-auth", tokenIss: "", wantErr: true},
		{name: "unset config accepts any issuer", configured: "", tokenIss: "somebody-else"},
		{name: "unset config accepts missing issuer", configured: "", tokenIss: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			global.PRISM_CONFIG.JWT = config.JWT{SigningKey: signingKey, Issuer: tc.configured}
			_, err := (&JwtService{}).ParseAccessToken(mint(t, tc.tokenIss))
			if tc.wantErr && err == nil {
				t.Fatalf("issuer %q must be rejected when %q is configured", tc.tokenIss, tc.configured)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("issuer %q must be accepted when %q is configured: %v", tc.tokenIss, tc.configured, err)
			}
		})
	}
}

// A token signed by a granule configured for one issuer must not parse at a
// granule expecting another — the doc's "issuer A signs / B parses" acceptance.
func TestGeneratedTokenRejectedByDifferentlyConfiguredIssuer(t *testing.T) {
	previous := global.PRISM_CONFIG
	t.Cleanup(func() { global.PRISM_CONFIG = previous })

	global.PRISM_CONFIG.JWT = config.JWT{
		SigningKey:  "test-signing-key-with-sufficient-entropy",
		ExpiresTime: "15m",
		Issuer:      "issuer-a",
	}
	token, err := (&JwtService{}).GenerateSessionToken(1, "alice", 1, "family-123")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (&JwtService{}).ParseAccessToken(token); err != nil {
		t.Fatalf("issuer-a token must parse under issuer-a: %v", err)
	}

	global.PRISM_CONFIG.JWT.Issuer = "issuer-b"
	if _, err := (&JwtService{}).ParseAccessToken(token); err == nil {
		t.Fatal("token signed by issuer-a must be rejected when issuer-b is expected")
	}
}

func TestTokenConfigurationRejectsUnsafeOrInvalidDurations(t *testing.T) {
	previous := global.PRISM_CONFIG
	t.Cleanup(func() { global.PRISM_CONFIG = previous })

	global.PRISM_CONFIG.JWT = config.JWT{
		SigningKey:               "test-signing-key-with-sufficient-entropy",
		ExpiresTime:              "24h",
		RefreshExpiresTime:       "24h",
		RefreshFamilyExpiresTime: "720h",
	}
	if err := ValidateTokenConfiguration(); err == nil || !strings.Contains(err.Error(), "must be longer") {
		t.Fatalf("expected access/refresh ordering error, got %v", err)
	}

	global.PRISM_CONFIG.JWT.RefreshExpiresTime = "168h"
	global.PRISM_CONFIG.JWT.RefreshRotationGrace = "not-a-duration"
	if err := ValidateTokenConfiguration(); err == nil || !strings.Contains(err.Error(), "refresh-rotation-grace") {
		t.Fatalf("expected invalid duration error, got %v", err)
	}
}

func TestTokenConfigurationRejectsMissingSigningKey(t *testing.T) {
	previous := global.PRISM_CONFIG
	global.PRISM_CONFIG.JWT = config.JWT{
		ExpiresTime:              "24h",
		RefreshExpiresTime:       "168h",
		RefreshFamilyExpiresTime: "720h",
	}
	t.Cleanup(func() { global.PRISM_CONFIG = previous })
	if err := ValidateTokenConfiguration(); err == nil || !strings.Contains(err.Error(), "signing-key") {
		t.Fatalf("expected signing key validation error, got %v", err)
	}
}

func TestConfiguredDurationRejectsOverflow(t *testing.T) {
	if _, err := parseConfiguredDuration("999999999999999999999d"); err == nil {
		t.Fatal("expected overflowing duration to be rejected")
	}
}

func TestJWTAccessMetadataAndRefreshWindow(t *testing.T) {
	previous := global.PRISM_CONFIG
	global.PRISM_CONFIG.JWT = config.JWT{
		SigningKey:  "test-signing-key-with-sufficient-entropy",
		ExpiresTime: "30m",
		BufferTime:  "5m",
	}
	t.Cleanup(func() { global.PRISM_CONFIG = previous })

	service := &JwtService{}
	if service.AccessExpiresIn() != "30m" {
		t.Fatalf("access expiry=%q", service.AccessExpiresIn())
	}
	if _, err := service.GenerateSessionToken(1, "alice", 1, ""); err == nil {
		t.Fatal("empty session ID must be rejected")
	}
	if (*Claims)(nil).IsServicePrincipal() {
		t.Fatal("nil claims cannot be a service principal")
	}
	if !service.NeedRefresh(&Claims{RegisteredClaims: jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
	}}) {
		t.Fatal("token inside buffer should refresh")
	}
	if service.NeedRefresh(&Claims{RegisteredClaims: jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}}) {
		t.Fatal("token outside buffer should not refresh")
	}
}
