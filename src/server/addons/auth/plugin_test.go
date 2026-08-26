package auth

import (
	"testing"

	"github.com/kwhitestone/prism-fusion/config"
	"github.com/kwhitestone/prism-fusion/global"
)

func TestShouldServeRoutesCanDisableResourceServiceAuthEndpoints(t *testing.T) {
	previous := global.PRISM_CONFIG
	t.Cleanup(func() { global.PRISM_CONFIG = previous })

	disabled := false
	global.PRISM_CONFIG.Auth = config.Auth{Provider: "builtin", ServeRoutes: &disabled}
	if shouldServeRoutes() {
		t.Fatal("resource services must be able to keep JWT middleware without serving login routes")
	}

	enabled := true
	global.PRISM_CONFIG.Auth.ServeRoutes = &enabled
	if !shouldServeRoutes() {
		t.Fatal("auth service must be able to expose its routes explicitly")
	}
}
