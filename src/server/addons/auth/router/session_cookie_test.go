package router

import (
	"net/http"
	"testing"
	"time"

	"github.com/kwhitestone/prism-fusion/config"
	"github.com/kwhitestone/prism-fusion/global"
)

func TestCookieOnlyRefreshNeverExposesOrFallsBackToBodyToken(t *testing.T) {
	cookie := http.Cookie{Name: refreshCookieName, Value: "cookie-secret"}
	if got := refreshCredential("1", "body-secret", cookie); got != "cookie-secret" {
		t.Fatalf("expected cookie credential, got %q", got)
	}
	if got := exposedRefreshToken("1", "cookie-secret"); got != "" {
		t.Fatalf("cookie-only response exposed refresh token %q", got)
	}
	if got := refreshCredential("", "legacy-body-secret", cookie); got != "legacy-body-secret" {
		t.Fatalf("legacy body compatibility failed, got %q", got)
	}
	if validRefreshRequestID("1", "short") {
		t.Fatal("cookie-only refresh accepted a weak request identifier")
	}
	if !validRefreshRequestID("1", "request-id-with-enough-entropy") {
		t.Fatal("cookie-only refresh rejected a valid request identifier")
	}
}

func TestRefreshCookieIsHttpOnlyAndSecureInPublicMode(t *testing.T) {
	previous := global.PRISM_CONFIG
	global.PRISM_CONFIG.System = config.System{Env: "public"}
	t.Cleanup(func() { global.PRISM_CONFIG = previous })

	cookie := newRefreshCookie("secret", time.Now().Add(time.Hour))
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteNoneMode {
		t.Fatalf("unsafe refresh cookie attributes: %#v", cookie)
	}
	if cookie.Path != refreshCookiePath || cookie.MaxAge <= 0 {
		t.Fatalf("unexpected refresh cookie scope or lifetime: %#v", cookie)
	}
}
