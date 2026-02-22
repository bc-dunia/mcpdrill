package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// JWTAuthenticator validates JWT tokens from request headers.
// This is a minimal implementation for future use - does not support RS256 or key rotation.
type JWTAuthenticator struct {
	secret []byte
	issuer string
}

// NewJWTAuthenticator creates a new JWT authenticator.
func NewJWTAuthenticator(config *Config) *JWTAuthenticator {
	return &JWTAuthenticator{
		secret: config.JWTSecret,
		issuer: config.JWTIssuer,
	}
}

type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

type jwtClaims struct {
	Sub   string   `json:"sub"`
	Iss   string   `json:"iss"`
	Exp   int64    `json:"exp"`
	Iat   int64    `json:"iat"`
	Roles []string `json:"roles,omitempty"`
}

// Authenticate extracts and validates the JWT from the request.
func (a *JWTAuthenticator) Authenticate(r *http.Request) (*User, error) {
	token := a.extractToken(r)
	if token == "" {
		return nil, ErrMissingCredentials
	}

	claims, err := a.validateToken(token)
	if err != nil {
		return nil, err
	}

	roles := make([]Role, 0, len(claims.Roles))
	for _, r := range claims.Roles {
		roles = append(roles, Role(r))
	}
	if len(roles) == 0 {
		roles = []Role{RoleViewer}
	}

	return &User{
		ID:    claims.Sub,
		Roles: roles,
	}, nil
}

func (a *JWTAuthenticator) extractToken(r *http.Request) string {
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

func (a *JWTAuthenticator) validateToken(token string) (*jwtClaims, error) {
	if len(a.secret) == 0 {
		return nil, &AuthError{
			StatusCode: http.StatusInternalServerError,
			ErrorType:  "configuration_error",
			ErrorCode:  "JWT_SECRET_REQUIRED",
			Message:    "JWT secret is not configured",
		}
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, ErrInvalidCredentials
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	var header jwtHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, ErrInvalidCredentials
	}

	if header.Alg != "HS256" {
		return nil, &AuthError{
			StatusCode: http.StatusUnauthorized,
			ErrorType:  "unauthorized",
			ErrorCode:  "UNSUPPORTED_ALGORITHM",
			Message:    "Only HS256 algorithm is supported",
		}
	}

	claimsBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	var claims jwtClaims
	if err := json.Unmarshal(claimsBytes, &claims); err != nil {
		return nil, ErrInvalidCredentials
	}

	signatureBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	expectedSig := a.computeSignature(parts[0] + "." + parts[1])
	if !hmac.Equal(signatureBytes, expectedSig) {
		return nil, ErrInvalidCredentials
	}

	now := time.Now().Unix()
	if claims.Exp > 0 && claims.Exp < now {
		return nil, &AuthError{
			StatusCode: http.StatusUnauthorized,
			ErrorType:  "unauthorized",
			ErrorCode:  "TOKEN_EXPIRED",
			Message:    "Token has expired",
		}
	}

	if a.issuer != "" && claims.Iss != a.issuer {
		return nil, &AuthError{
			StatusCode: http.StatusUnauthorized,
			ErrorType:  "unauthorized",
			ErrorCode:  "INVALID_ISSUER",
			Message:    "Invalid token issuer",
		}
	}

	return &claims, nil
}

func (a *JWTAuthenticator) computeSignature(data string) []byte {
	h := hmac.New(sha256.New, a.secret)
	h.Write([]byte(data))
	return h.Sum(nil)
}
