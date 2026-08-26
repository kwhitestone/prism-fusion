package authMiddleware

import "testing"

func TestAuthPathLimitScopesSensitiveRoutes(t *testing.T) {
	if authPathLimit("/api/v1/addons/auth/login") != 30 {
		t.Fatal("login IP limit must allow account throttling to be authoritative")
	}
	if authPathLimit("/api/v1/addons/auth/register") != 10 {
		t.Fatal("registration must be rate limited")
	}
	if authPathLimit("/api/v1/addons/auth/user-info") != 0 {
		t.Fatal("protected reads must not use the public auth mutation limiter")
	}
	if scope := authRateScope("/api/v1/addons/auth/refresh-token"); len(scope) > 32 {
		t.Fatalf("rate-limit scope must fit existing schemas, got %q", scope)
	}
}
