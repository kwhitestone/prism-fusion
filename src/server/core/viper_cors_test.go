package core

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kwhitestone/prism-fusion/config"
	"github.com/kwhitestone/prism-fusion/global"
	"github.com/kwhitestone/prism-fusion/middleware"
)

const corsTestYAML = `cors:
  mode: 'strict-whitelist'
  whitelist:
    - allow-origin: '${PFT_WEB_FRONTEND_URL}'
      allow-methods: 'GET,POST,OPTIONS'
      allow-headers: 'Content-Type'
      allow-credentials: true
    - allow-origin: '${PFT_AUTH_FRONTEND_URL}'
      allow-credentials: true
    - allow-origin: '${PFT_WEB_OUTER_FRONTEND_URL:-}'
      allow-credentials: true
    - allow-origin: '${PFT_LOCAL_URL:-http://localhost:3188}'
`

func loadCORSConfig(t *testing.T, env map[string]string) (err any) {
	t.Helper()
	previous := global.PRISM_CONFIG
	t.Cleanup(func() { global.PRISM_CONFIG = previous })
	for _, key := range []string{"PFT_WEB_FRONTEND_URL", "PFT_AUTH_FRONTEND_URL", "PFT_WEB_OUTER_FRONTEND_URL", "PFT_LOCAL_URL"} {
		t.Setenv(key, env[key])
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(corsTestYAML), 0600); err != nil {
		t.Fatal(err)
	}
	defer func() { err = recover() }()
	Viper(path)
	return nil
}

func origins(entries []config.CORSWhitelist) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.AllowOrigin
	}
	return out
}

func TestCORSStagedValuesBuildExactWhitelist(t *testing.T) {
	if err := loadCORSConfig(t, map[string]string{
		"PFT_WEB_FRONTEND_URL":  "https://shell.example.test",
		"PFT_AUTH_FRONTEND_URL": "https://auth.example.test:8443",
	}); err != nil {
		t.Fatalf("boot failed with staged values: %v", err)
	}
	got := strings.Join(origins(global.PRISM_CONFIG.CORS.Whitelist), " ")
	// optional outer slot unset → dropped; defaulted slot → default
	if want := "https://shell.example.test https://auth.example.test:8443 http://localhost:3188"; got != want {
		t.Fatalf("whitelist = %q, want %q", got, want)
	}
	if e := global.PRISM_CONFIG.CORS.Whitelist[0]; e.AllowMethods != "GET,POST,OPTIONS" || !e.AllowCredentials {
		t.Fatalf("entry policy lost: %+v", e)
	}

	router := gin.New()
	router.Use(middleware.Cors())
	for origin, want := range map[string]int{
		"https://shell.example.test":     http.StatusNoContent,
		"https://auth.example.test:8443": http.StatusNoContent,
		"https://untrusted.example.test": http.StatusForbidden,
	} {
		req := httptest.NewRequest(http.MethodOptions, "/api/x", nil)
		req.Header.Set("Origin", origin)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("%s: status=%d want %d", origin, rec.Code, want)
		}
		if want == http.StatusNoContent && (rec.Header().Get("Access-Control-Allow-Origin") != origin || rec.Header().Get("Access-Control-Allow-Credentials") != "true") {
			t.Fatalf("%s: headers=%v", origin, rec.Header())
		}
	}
}

func TestCORSMissingRequiredValuesFailBootNamingKeys(t *testing.T) {
	err := loadCORSConfig(t, map[string]string{"PFT_WEB_OUTER_FRONTEND_URL": "https://secret-outer.example.test"})
	if err == nil {
		t.Fatal("boot succeeded with required CORS env missing")
	}
	msg := err.(error).Error()
	if !strings.Contains(msg, "PFT_AUTH_FRONTEND_URL, PFT_WEB_FRONTEND_URL") {
		t.Fatalf("error does not name missing keys: %s", msg)
	}
	if strings.Contains(msg, "secret-outer") || strings.Contains(msg, "PFT_WEB_OUTER") || strings.Contains(msg, "PFT_LOCAL_URL") {
		t.Fatalf("error leaks values or names optional keys: %s", msg)
	}
}

func TestCORSRejectsNonExactOrigins(t *testing.T) {
	for _, bad := range []string{
		"https://a.example.test,https://b.example.test",
		"https://*.example.test",
		"https://a.example.test/",
		"https://a.example.test/path",
		"https://user:pw@a.example.test",
		"https://A.example.test",
		"ftp://a.example.test",
		"a.example.test",
	} {
		t.Setenv("PFT_BAD", bad)
		_, err := resolveCORSWhitelist([]config.CORSWhitelist{{AllowOrigin: "${PFT_BAD}"}})
		if err == nil || strings.Contains(err.Error(), bad) || !strings.Contains(err.Error(), "${PFT_BAD}") {
			t.Fatalf("%q: err=%v", bad, err)
		}
	}
}
