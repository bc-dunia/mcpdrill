// Package auth provides authentication and authorization for the MCP Drill API.
package auth

import (
	"context"
)

// AuthMode defines the authentication mode.
type AuthMode string

const (
	// AuthModeNone disables authentication (default for backward compatibility).
	AuthModeNone AuthMode = "none"
	// AuthModeAPIKey enables API key authentication.
	AuthModeAPIKey AuthMode = "api_key"
	// AuthModeJWT enables JWT token authentication.
	AuthModeJWT AuthMode = "jwt"
)

// Role defines user roles for RBAC.
type Role string

const (
	// RoleAdmin has full access to all operations.
	RoleAdmin Role = "admin"
	// RoleOperator can create and manage runs.
	RoleOperator Role = "operator"
	// RoleViewer can only read data.
	RoleViewer Role = "viewer"
)

// Config holds authentication configuration.
type Config struct {
	// Mode is the authentication mode (none, api_key, jwt).
	Mode AuthMode `json:"mode"`
	// APIKeys is a list of valid API keys (for api_key mode).
	// Each key can optionally have role mappings via APIKeyRoles.
	APIKeys []string `json:"api_keys,omitempty"`
	// APIKeyRoles maps API keys to their roles.
	// If a key is not in this map, it defaults to RoleOperator.
	APIKeyRoles map[string][]Role `json:"api_key_roles,omitempty"`
	// JWTSecret is the secret for JWT validation (for jwt mode).
	JWTSecret []byte `json:"-"`
	// JWTIssuer is the expected issuer for JWT tokens.
	JWTIssuer string `json:"jwt_issuer,omitempty"`
	// SkipPaths are paths that don't require authentication.
	// /healthz and /readyz are always skipped.
	SkipPaths []string `json:"skip_paths,omitempty"`
}

// DefaultConfig returns a default configuration with auth disabled.
func DefaultConfig() *Config {
	return &Config{
		Mode:      AuthModeNone,
		SkipPaths: []string{"/healthz", "/readyz"},
	}
}

// User represents an authenticated user.
type User struct {
	// ID is the user identifier (API key hash or JWT subject).
	ID string
	// Roles are the roles assigned to this user.
	Roles []Role
}

// HasRole checks if the user has a specific role.
func (u *User) HasRole(role Role) bool {
	if u == nil {
		return false
	}
	for _, r := range u.Roles {
		if r == role || r == RoleAdmin {
			return true
		}
	}
	return false
}

// HasAnyRole checks if the user has any of the specified roles.
func (u *User) HasAnyRole(roles ...Role) bool {
	for _, role := range roles {
		if u.HasRole(role) {
			return true
		}
	}
	return false
}

// contextKey is an unexported type for context keys to prevent collisions.
type contextKey struct{ name string }

var (
	userContextKey = &contextKey{"user"}
)

// SetUserInContext stores the user in the context.
func SetUserInContext(ctx context.Context, user *User) context.Context {
	return context.WithValue(ctx, userContextKey, user)
}

// GetUserFromContext retrieves the user from the context.
// Returns nil if no user is set.
func GetUserFromContext(ctx context.Context) *User {
	user, _ := ctx.Value(userContextKey).(*User)
	return user
}

// GetRolesFromContext retrieves the user's roles from the context.
// Returns nil if no user is set.
func GetRolesFromContext(ctx context.Context) []Role {
	user := GetUserFromContext(ctx)
	if user == nil {
		return nil
	}
	return user.Roles
}

// HasRole checks if the user in the context has a specific role.
func HasRole(ctx context.Context, role Role) bool {
	user := GetUserFromContext(ctx)
	return user.HasRole(role)
}

// HasAnyRole checks if the user in the context has any of the specified roles.
func HasAnyRole(ctx context.Context, roles ...Role) bool {
	user := GetUserFromContext(ctx)
	if user == nil {
		return false
	}
	return user.HasAnyRole(roles...)
}
