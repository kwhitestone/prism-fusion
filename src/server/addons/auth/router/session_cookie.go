package router

import (
	"net/http"
	"strings"
	"time"

	"github.com/kwhitestone/prism-fusion/global"
)

const (
	refreshCookieName     = "nucleagent_refresh"
	refreshCookiePath     = "/api/v1/addons/auth"
	cookieOnlyHeaderValue = "1"
)

func cookieOnly(mode string) bool {
	return mode == cookieOnlyHeaderValue
}

func validRefreshRequestID(mode, requestID string) bool {
	if !cookieOnly(mode) {
		return true
	}
	return len(requestID) >= 16 && len(requestID) <= 128
}

func refreshCredential(mode, bodyToken string, cookie http.Cookie) string {
	if cookieOnly(mode) {
		return cookie.Value
	}
	return bodyToken
}

func exposedRefreshToken(mode, refreshToken string) string {
	if cookieOnly(mode) {
		return ""
	}
	return refreshToken
}

func newRefreshCookie(refreshToken string, expiresAt time.Time) http.Cookie {
	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge < 1 {
		maxAge = 1
	}
	secure, sameSite := refreshCookieTransport()
	return http.Cookie{
		Name:     refreshCookieName,
		Value:    refreshToken,
		Path:     refreshCookiePath,
		Expires:  expiresAt.UTC(),
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
	}
}

func expiredRefreshCookie() http.Cookie {
	secure, sameSite := refreshCookieTransport()
	return http.Cookie{
		Name:     refreshCookieName,
		Path:     refreshCookiePath,
		Expires:  time.Unix(1, 0).UTC(),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
	}
}

func refreshCookieTransport() (bool, http.SameSite) {
	if strings.EqualFold(global.PRISM_CONFIG.System.Env, "public") {
		// Production may place shell and auth on different sites. Browsers only
		// send a cross-site cookie when SameSite=None is paired with Secure.
		return true, http.SameSiteNoneMode
	}
	return false, http.SameSiteLaxMode
}
