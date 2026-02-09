package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Mode != AuthModeAPIKey {
		t.Errorf("expected mode %q, got %q", AuthModeAPIKey, cfg.Mode)
	}
	if cfg.InsecureMode {
		t.Error("expected insecure mode to be false by default")
	}
	if len(cfg.SkipPaths) != 2 {
		t.Errorf("expected 2 skip paths, got %d", len(cfg.SkipPaths))
	}
}

func TestUserHasRole(t *testing.T) {
	tests := []struct {
		name     string
		user     *User
		role     Role
		expected bool
	}{
		{"nil user", nil, RoleAdmin, false},
		{"admin has admin", &User{Roles: []Role{RoleAdmin}}, RoleAdmin, true},
		{"admin has operator", &User{Roles: []Role{RoleAdmin}}, RoleOperator, true},
		{"admin has viewer", &User{Roles: []Role{RoleAdmin}}, RoleViewer, true},
		{"operator has operator", &User{Roles: []Role{RoleOperator}}, RoleOperator, true},
		{"operator no admin", &User{Roles: []Role{RoleOperator}}, RoleAdmin, false},
		{"viewer has viewer", &User{Roles: []Role{RoleViewer}}, RoleViewer, true},
		{"viewer no operator", &User{Roles: []Role{RoleViewer}}, RoleOperator, false},
		{"multiple roles", &User{Roles: []Role{RoleOperator, RoleViewer}}, RoleViewer, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.user.HasRole(tt.role)
			if got != tt.expected {
				t.Errorf("HasRole(%v) = %v, want %v", tt.role, got, tt.expected)
			}
		})
	}
}

func TestUserHasAnyRole(t *testing.T) {
	user := &User{Roles: []Role{RoleOperator}}

	if !user.HasAnyRole(RoleOperator, RoleViewer) {
		t.Error("expected HasAnyRole to return true for operator")
	}
	if user.HasAnyRole(RoleAdmin) {
		t.Error("expected HasAnyRole to return false for admin only")
	}
}

func TestContextHelpers(t *testing.T) {
	ctx := context.Background()

	if GetUserFromContext(ctx) != nil {
		t.Error("expected nil user from empty context")
	}
	if GetRolesFromContext(ctx) != nil {
		t.Error("expected nil roles from empty context")
	}

	user := &User{ID: "test-user", Roles: []Role{RoleOperator}}
	ctx = SetUserInContext(ctx, user)

	got := GetUserFromContext(ctx)
	if got == nil || got.ID != "test-user" {
		t.Error("expected user from context")
	}

	roles := GetRolesFromContext(ctx)
	if len(roles) != 1 || roles[0] != RoleOperator {
		t.Error("expected operator role from context")
	}

	if !HasRole(ctx, RoleOperator) {
		t.Error("expected HasRole to return true")
	}
	if HasRole(ctx, RoleAdmin) {
		t.Error("expected HasRole to return false for admin")
	}
}

func TestAPIKeyAuthenticator(t *testing.T) {
	config := &Config{
		Mode:    AuthModeAPIKey,
		APIKeys: []string{"test-key-1", "test-key-2"},
		APIKeyRoles: map[string][]Role{
			"test-key-1": {RoleAdmin},
		},
	}
	auth := NewAPIKeyAuthenticator(config)

	tests := []struct {
		name        string
		headers     map[string]string
		expectError bool
		expectRole  Role
	}{
		{
			name:        "missing credentials",
			headers:     map[string]string{},
			expectError: true,
		},
		{
			name:        "invalid key",
			headers:     map[string]string{"X-API-Key": "invalid"},
			expectError: true,
		},
		{
			name:        "valid key via X-API-Key",
			headers:     map[string]string{"X-API-Key": "test-key-1"},
			expectError: false,
			expectRole:  RoleAdmin,
		},
		{
			name:        "valid key via Bearer",
			headers:     map[string]string{"Authorization": "Bearer test-key-2"},
			expectError: false,
			expectRole:  RoleOperator,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			user, err := auth.Authenticate(req)
			if tt.expectError {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if !user.HasRole(tt.expectRole) {
				t.Errorf("expected role %v, got %v", tt.expectRole, user.Roles)
			}
		})
	}
}

func TestMiddlewareNoAuth(t *testing.T) {
	config := &Config{Mode: AuthModeNone}
	mw := NewMiddleware(config, nil)

	called := false
	handler := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("handler was not called")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

func TestMiddlewareSkipPaths(t *testing.T) {
	config := &Config{
		Mode:      AuthModeAPIKey,
		APIKeys:   []string{"test-key"},
		SkipPaths: []string{"/custom"},
	}
	auth := NewAPIKeyAuthenticator(config)
	mw := NewMiddleware(config, auth)

	handler := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		path       string
		expectCode int
	}{
		{"/healthz", http.StatusOK},
		{"/readyz", http.StatusOK},
		{"/custom", http.StatusOK},
		{"/protected", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.expectCode {
				t.Errorf("path %s: expected status %d, got %d", tt.path, tt.expectCode, rec.Code)
			}
		})
	}
}

func TestMiddlewareRequireRoles(t *testing.T) {
	config := &Config{
		Mode:    AuthModeAPIKey,
		APIKeys: []string{"admin-key", "viewer-key"},
		APIKeyRoles: map[string][]Role{
			"admin-key":  {RoleAdmin},
			"viewer-key": {RoleViewer},
		},
	}
	auth := NewAPIKeyAuthenticator(config)
	mw := NewMiddleware(config, auth)

	baseHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	roleHandler := mw.RequireRoles(RoleAdmin, RoleOperator)(baseHandler)
	authHandler := mw.Handler(roleHandler)

	tests := []struct {
		name       string
		key        string
		expectCode int
	}{
		{"admin allowed", "admin-key", http.StatusOK},
		{"viewer forbidden", "viewer-key", http.StatusForbidden},
		{"no key", "", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			if tt.key != "" {
				req.Header.Set("X-API-Key", tt.key)
			}
			rec := httptest.NewRecorder()
			authHandler.ServeHTTP(rec, req)

			if rec.Code != tt.expectCode {
				t.Errorf("expected status %d, got %d", tt.expectCode, rec.Code)
			}
		})
	}
}

func TestJWTAuthenticator(t *testing.T) {
	secret := []byte("test-secret-key-for-jwt")
	config := &Config{
		Mode:      AuthModeJWT,
		JWTSecret: secret,
		JWTIssuer: "test-issuer",
	}
	auth := NewJWTAuthenticator(config)

	t.Run("missing token", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		_, err := auth.Authenticate(req)
		if err != ErrMissingCredentials {
			t.Errorf("expected ErrMissingCredentials, got %v", err)
		}
	})

	t.Run("invalid token format", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer invalid")
		_, err := auth.Authenticate(req)
		if err != ErrInvalidCredentials {
			t.Errorf("expected ErrInvalidCredentials, got %v", err)
		}
	})

	t.Run("valid token", func(t *testing.T) {
		token := createTestJWT(t, secret, "test-user", "test-issuer", time.Now().Add(time.Hour).Unix(), []string{"admin"})
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		user, err := auth.Authenticate(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if user.ID != "test-user" {
			t.Errorf("expected user ID 'test-user', got %q", user.ID)
		}
		if !user.HasRole(RoleAdmin) {
			t.Error("expected admin role")
		}
	})

	t.Run("expired token", func(t *testing.T) {
		token := createTestJWT(t, secret, "test-user", "test-issuer", time.Now().Add(-time.Hour).Unix(), nil)
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		_, err := auth.Authenticate(req)
		if err == nil {
			t.Error("expected error for expired token")
		}
		authErr, ok := err.(*AuthError)
		if !ok || authErr.ErrorCode != "TOKEN_EXPIRED" {
			t.Errorf("expected TOKEN_EXPIRED error, got %v", err)
		}
	})

	t.Run("wrong issuer", func(t *testing.T) {
		token := createTestJWT(t, secret, "test-user", "wrong-issuer", time.Now().Add(time.Hour).Unix(), nil)
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		_, err := auth.Authenticate(req)
		if err == nil {
			t.Error("expected error for wrong issuer")
		}
		authErr, ok := err.(*AuthError)
		if !ok || authErr.ErrorCode != "INVALID_ISSUER" {
			t.Errorf("expected INVALID_ISSUER error, got %v", err)
		}
	})

	t.Run("wrong signature", func(t *testing.T) {
		token := createTestJWT(t, []byte("wrong-secret"), "test-user", "test-issuer", time.Now().Add(time.Hour).Unix(), nil)
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		_, err := auth.Authenticate(req)
		if err != ErrInvalidCredentials {
			t.Errorf("expected ErrInvalidCredentials, got %v", err)
		}
	})
}

func createTestJWT(t *testing.T, secret []byte, sub, iss string, exp int64, roles []string) string {
	t.Helper()

	header := map[string]string{"alg": "HS256", "typ": "JWT"}
	headerBytes, _ := json.Marshal(header)
	headerB64 := base64.RawURLEncoding.EncodeToString(headerBytes)

	claims := map[string]interface{}{
		"sub": sub,
		"iss": iss,
		"exp": exp,
		"iat": time.Now().Unix(),
	}
	if roles != nil {
		claims["roles"] = roles
	}
	claimsBytes, _ := json.Marshal(claims)
	claimsB64 := base64.RawURLEncoding.EncodeToString(claimsBytes)

	signData := headerB64 + "." + claimsB64
	h := hmac.New(sha256.New, secret)
	h.Write([]byte(signData))
	sig := base64.RawURLEncoding.EncodeToString(h.Sum(nil))

	return signData + "." + sig
}
