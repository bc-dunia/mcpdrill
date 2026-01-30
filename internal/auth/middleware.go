package auth

import (
	"encoding/json"
	"net/http"
	"strings"
)

// Authenticator validates credentials and returns a user.
type Authenticator interface {
	Authenticate(r *http.Request) (*User, error)
}

// AuthError represents an authentication/authorization error.
type AuthError struct {
	StatusCode int
	ErrorType  string
	ErrorCode  string
	Message    string
}

func (e *AuthError) Error() string {
	return e.Message
}

var (
	ErrMissingCredentials = &AuthError{
		StatusCode: http.StatusUnauthorized,
		ErrorType:  "unauthorized",
		ErrorCode:  "MISSING_CREDENTIALS",
		Message:    "Missing authentication credentials",
	}
	ErrInvalidCredentials = &AuthError{
		StatusCode: http.StatusUnauthorized,
		ErrorType:  "unauthorized",
		ErrorCode:  "INVALID_CREDENTIALS",
		Message:    "Invalid authentication credentials",
	}
	ErrForbidden = &AuthError{
		StatusCode: http.StatusForbidden,
		ErrorType:  "forbidden",
		ErrorCode:  "INSUFFICIENT_PERMISSIONS",
		Message:    "Insufficient permissions for this operation",
	}
)

// Middleware provides HTTP middleware for authentication and authorization.
type Middleware struct {
	config        *Config
	authenticator Authenticator
	skipPaths     map[string]bool
}

// NewMiddleware creates a new authentication middleware.
func NewMiddleware(config *Config, authenticator Authenticator) *Middleware {
	skipPaths := make(map[string]bool)
	skipPaths["/healthz"] = true
	skipPaths["/readyz"] = true
	for _, path := range config.SkipPaths {
		skipPaths[path] = true
	}

	return &Middleware{
		config:        config,
		authenticator: authenticator,
		skipPaths:     skipPaths,
	}
}

// Handler wraps an http.Handler with authentication.
func (m *Middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m.config.Mode == AuthModeNone {
			next.ServeHTTP(w, r)
			return
		}

		if m.shouldSkip(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		// Guard against nil authenticator (invalid auth mode)
		if m.authenticator == nil {
			m.writeError(w, &AuthError{
				StatusCode: http.StatusInternalServerError,
				ErrorType:  "configuration_error",
				ErrorCode:  "INVALID_AUTH_MODE",
				Message:    "Authentication is misconfigured",
			})
			return
		}

		user, err := m.authenticator.Authenticate(r)
		if err != nil {
			m.writeError(w, err)
			return
		}

		ctx := SetUserInContext(r.Context(), user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
// RequireRoles returns middleware that requires the user to have one of the specified roles.
func (m *Middleware) RequireRoles(roles ...Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if m.config.Mode == AuthModeNone {
				next.ServeHTTP(w, r)
				return
			}

			user := GetUserFromContext(r.Context())
			if user == nil {
				m.writeError(w, ErrMissingCredentials)
				return
			}

			if !user.HasAnyRole(roles...) {
				m.writeError(w, ErrForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func (m *Middleware) shouldSkip(path string) bool {
	if m.skipPaths[path] {
		return true
	}
	for skipPath := range m.skipPaths {
		if strings.HasPrefix(path, skipPath) && (len(path) == len(skipPath) || path[len(skipPath)] == '/') {
			return true
		}
	}
	return false
}

func (m *Middleware) writeError(w http.ResponseWriter, err error) {
	authErr, ok := err.(*AuthError)
	if !ok {
		authErr = &AuthError{
			StatusCode: http.StatusInternalServerError,
			ErrorType:  "internal",
			ErrorCode:  "INTERNAL_ERROR",
			Message:    "Internal authentication error",
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(authErr.StatusCode)

	resp := map[string]interface{}{
		"error_type":    authErr.ErrorType,
		"error_code":    authErr.ErrorCode,
		"error_message": authErr.Message,
		"retryable":     false,
	}
	json.NewEncoder(w).Encode(resp)
}
