package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
)

// APIKeyAuthenticator validates API keys from request headers.
type APIKeyAuthenticator struct {
	keyHashes  map[string]bool
	keyToRoles map[string][]Role
}

// NewAPIKeyAuthenticator creates a new API key authenticator.
func NewAPIKeyAuthenticator(config *Config) *APIKeyAuthenticator {
	a := &APIKeyAuthenticator{
		keyHashes:  make(map[string]bool),
		keyToRoles: make(map[string][]Role),
	}

	for _, key := range config.APIKeys {
		hash := hashKey(key)
		a.keyHashes[hash] = true

		if roles, ok := config.APIKeyRoles[key]; ok {
			a.keyToRoles[key] = roles
		} else {
			a.keyToRoles[key] = []Role{RoleOperator}
		}
	}

	return a
}

// Authenticate extracts and validates the API key from the request.
func (a *APIKeyAuthenticator) Authenticate(r *http.Request) (*User, error) {
	key := a.extractAPIKey(r)
	if key == "" {
		return nil, ErrMissingCredentials
	}

	if !a.validateKey(key) {
		return nil, ErrInvalidCredentials
	}

	roles := a.keyToRoles[key]
	if roles == nil {
		roles = []Role{RoleOperator}
	}

	return &User{
		ID:    hashKey(key)[:16],
		Roles: roles,
	}, nil
}

func (a *APIKeyAuthenticator) extractAPIKey(r *http.Request) string {
	if key := r.Header.Get("X-API-Key"); key != "" {
		return key
	}

	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}

	const bearerPrefix = "Bearer "
	if strings.HasPrefix(auth, bearerPrefix) {
		return strings.TrimPrefix(auth, bearerPrefix)
	}

	return ""
}

func (a *APIKeyAuthenticator) validateKey(key string) bool {
	return a.keyHashes[hashKey(key)]
}

func hashKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}
