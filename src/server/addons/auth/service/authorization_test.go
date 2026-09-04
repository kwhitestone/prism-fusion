package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/kwhitestone/prism-fusion/config"
	"github.com/kwhitestone/prism-fusion/global"
)

func TestAuthorizationFallsBackSafelyWhenBuiltinRBACIsNotLinked(t *testing.T) {
	previous := global.PRISM_CONFIG
	global.PRISM_CONFIG.RBAC = config.RBAC{}
	t.Cleanup(func() { global.PRISM_CONFIG = previous })

	state, err := ResolveAuthorization(context.Background(), 7, 999)
	if err != nil {
		t.Fatalf("resolve fallback authorization: %v", err)
	}
	if !reflect.DeepEqual(state.Roles, []string{"admin"}) || len(state.Permissions) != 0 || state.Menus == nil {
		t.Fatalf("fallback authorization = %#v", state)
	}
}

func TestExplicitMissingAuthorizationProviderFailsClosed(t *testing.T) {
	previous := global.PRISM_CONFIG
	global.PRISM_CONFIG.RBAC = config.RBAC{Provider: "missing-provider"}
	t.Cleanup(func() { global.PRISM_CONFIG = previous })

	if _, err := ResolveAuthorization(context.Background(), 7, 1); err == nil {
		t.Fatal("explicitly configured missing provider unexpectedly succeeded")
	}
}

func TestAuthorizationResolverCannotReturnNilState(t *testing.T) {
	const provider = "nil-state-test"
	previous := global.PRISM_CONFIG
	global.PRISM_CONFIG.RBAC = config.RBAC{Provider: provider}
	t.Cleanup(func() {
		global.PRISM_CONFIG = previous
		authorizationResolvers.Lock()
		delete(authorizationResolvers.byProvider, provider)
		authorizationResolvers.Unlock()
	})
	if err := RegisterAuthorizationResolver(provider, func(context.Context, uint, uint) (*AuthorizationState, error) {
		return nil, nil
	}); err != nil {
		t.Fatal(err)
	}

	state, err := ResolveAuthorization(context.Background(), 7, 1)
	if state != nil || !errors.Is(err, ErrAuthorizationProviderUnavailable) {
		t.Fatalf("nil resolver state=%#v err=%v", state, err)
	}
}
