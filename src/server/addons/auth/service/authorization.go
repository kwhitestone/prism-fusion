package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/kwhitestone/prism-fusion/global"
)

var (
	ErrAuthorizationProviderUnavailable = errors.New("authorization provider unavailable")
	ErrAuthorizationProviderDuplicate   = errors.New("authorization provider already registered")
)

type AuthorizationState struct {
	Roles       []string            `json:"roles"`
	Permissions []string            `json:"permissions"`
	Menus       []AuthorizationMenu `json:"menus"`
}

// AuthorizationMenu is the auth-facing navigation contract. Keeping this DTO
// in the auth addon lets authorization providers contribute menus without
// coupling auth to a concrete RBAC model package.
type AuthorizationMenu struct {
	ID             uint   `json:"id"`
	ParentID       uint   `json:"parentId"`
	Code           string `json:"code"`
	Name           string `json:"name,omitempty"`
	Component      string `json:"component,omitempty"`
	Redirect       string `json:"redirect,omitempty"`
	Title          string `json:"title"`
	TitleKey       string `json:"titleKey,omitempty"`
	Path           string `json:"path"`
	Icon           string `json:"icon,omitempty"`
	App            string `json:"app,omitempty"`
	Type           string `json:"type"`
	PermissionCode string `json:"permissionCode,omitempty"`
	Sort           int    `json:"sort"`
	IsVisible      *bool  `json:"isVisible"`
	Rank           int    `json:"rank,omitempty"`
	ShowLink       *bool  `json:"showLink,omitempty"`
}

type AuthorizationResolver func(context.Context, uint, uint) (*AuthorizationState, error)

var authorizationResolvers = struct {
	sync.RWMutex
	byProvider map[string]AuthorizationResolver
}{byProvider: make(map[string]AuthorizationResolver)}

// RegisterAuthorizationResolver contributes an authorization implementation
// without making the auth addon import a concrete RBAC package.
func RegisterAuthorizationResolver(provider string, resolver AuthorizationResolver) error {
	provider = strings.TrimSpace(provider)
	if provider == "" || resolver == nil {
		return ErrAuthorizationProviderUnavailable
	}
	authorizationResolvers.Lock()
	defer authorizationResolvers.Unlock()
	if _, exists := authorizationResolvers.byProvider[provider]; exists {
		return fmt.Errorf("%w: %s", ErrAuthorizationProviderDuplicate, provider)
	}
	authorizationResolvers.byProvider[provider] = resolver
	return nil
}

// ResolveAuthorization selects a registered provider. The zero-value/default
// configuration safely falls back to legacy roles when the optional RBAC addon
// is not linked; an explicitly configured missing provider fails closed.
func ResolveAuthorization(ctx context.Context, userID, legacyRoleID uint) (*AuthorizationState, error) {
	if userID == 0 {
		return nil, ErrAuthorizationProviderUnavailable
	}
	configured := strings.TrimSpace(global.PRISM_CONFIG.RBAC.Provider)
	if configured == "disabled" {
		return legacyAuthorization(legacyRoleID), nil
	}
	provider := configured
	if provider == "" {
		provider = "builtin"
	}
	authorizationResolvers.RLock()
	resolver := authorizationResolvers.byProvider[provider]
	authorizationResolvers.RUnlock()
	if resolver != nil {
		state, err := resolver(ctx, userID, legacyRoleID)
		if err != nil {
			return nil, err
		}
		if state == nil {
			return nil, fmt.Errorf("%w: %s returned no state", ErrAuthorizationProviderUnavailable, provider)
		}
		return state, nil
	}
	if configured == "" {
		return legacyAuthorization(legacyRoleID), nil
	}
	return nil, fmt.Errorf("%w: %s", ErrAuthorizationProviderUnavailable, provider)
}

func legacyAuthorization(roleID uint) *AuthorizationState {
	roles := []string{}
	if roleID != 0 {
		roles = []string{"user"}
		if roleID == 900 || roleID == 999 {
			roles = []string{"admin"}
		}
	}
	return &AuthorizationState{Roles: roles, Permissions: []string{}, Menus: []AuthorizationMenu{}}
}
