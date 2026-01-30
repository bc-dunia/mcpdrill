// Package session provides session management for mcpdrill VUs.
package session

import (
	"context"
	"sync"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/transport"
)

// SessionMode represents the session management mode.
type SessionMode string

const (
	// ModeReuse keeps one session per VU for entire stage duration.
	// Re-initializes on 404 (session expired on server).
	ModeReuse SessionMode = "reuse"

	// ModePerRequest creates a new session (initialize) before every operation.
	// High overhead, useful for testing session creation paths.
	ModePerRequest SessionMode = "per_request"

	// ModePool shares a connection pool among VUs with session affinity.
	// Sessions are borrowed from pool and returned after use.
	ModePool SessionMode = "pool"

	// ModeChurn deliberately rotates sessions to stress creation/teardown.
	// Sessions are created and destroyed at configurable intervals.
	ModeChurn SessionMode = "churn"
)

// SessionState represents the current state of a session.
type SessionState string

const (
	// StateCreating indicates the session is being initialized.
	StateCreating SessionState = "creating"

	// StateActive indicates the session is ready for operations.
	StateActive SessionState = "active"

	// StateIdle indicates the session is not currently in use.
	StateIdle SessionState = "idle"

	// StateExpired indicates the session has been evicted (TTL or idle timeout).
	StateExpired SessionState = "expired"

	// StateClosed indicates the session has been explicitly closed.
	StateClosed SessionState = "closed"
)

// SessionConfig holds configuration for session management.
type SessionConfig struct {
	// Mode determines how sessions are managed.
	Mode SessionMode

	// PoolSize is the maximum number of sessions in the pool (for pool mode).
	// Must be > 0 for pool mode.
	PoolSize int

	// TTLMs is the maximum lifetime of a session in milliseconds.
	// 0 means no TTL limit.
	TTLMs int64

	// MaxIdleMs is the maximum idle time before a session is evicted.
	// 0 means no idle limit.
	MaxIdleMs int64

	// ChurnIntervalMs is the interval between session rotations (for churn mode).
	// Only used when Mode is ModeChurn. If 0, uses ChurnIntervalOps instead.
	ChurnIntervalMs int64

	// ChurnIntervalOps is the number of operations after which to churn the session.
	// Only used when Mode is ModeChurn and ChurnIntervalMs is 0.
	// Default is 1 (churn after every operation).
	ChurnIntervalOps int64

	// TransportConfig is the configuration for creating transport connections.
	TransportConfig *transport.TransportConfig

	// Adapter is the transport adapter used to create connections.
	Adapter transport.Adapter
}

// DefaultSessionConfig returns a default session configuration.
func DefaultSessionConfig() *SessionConfig {
	return &SessionConfig{
		Mode:      ModeReuse,
		PoolSize:  100,
		TTLMs:     900000,  // 15 minutes
		MaxIdleMs: 60000,   // 1 minute
	}
}

// SessionInfo contains information about a session.
type SessionInfo struct {
	// ID is the session identifier (from MCP server).
	ID string

	// VUID is the VU that owns this session (for reuse/per_request modes).
	// Empty for pool mode sessions.
	VUID string

	// State is the current session state.
	State SessionState

	// CreatedAt is when the session was created.
	CreatedAt time.Time

	// LastUsedAt is when the session was last used.
	LastUsedAt time.Time

	// ExpiresAt is when the session will expire (based on TTL).
	// Zero value means no expiration.
	ExpiresAt time.Time

	// IdleExpiresAt is when the session will expire due to idle timeout.
	// Updated on each use.
	IdleExpiresAt time.Time

	// OperationCount is the number of operations performed on this session.
	OperationCount int64

	// Connection is the underlying transport connection.
	Connection transport.Connection

	// mu protects mutable fields.
	mu sync.RWMutex
}

// NewSessionInfo creates a new SessionInfo.
func NewSessionInfo(id string, conn transport.Connection, ttlMs, maxIdleMs int64) *SessionInfo {
	now := time.Now()
	info := &SessionInfo{
		ID:             id,
		State:          StateActive,
		CreatedAt:      now,
		LastUsedAt:     now,
		Connection:     conn,
		OperationCount: 0,
	}

	if ttlMs > 0 {
		info.ExpiresAt = now.Add(time.Duration(ttlMs) * time.Millisecond)
	}

	if maxIdleMs > 0 {
		info.IdleExpiresAt = now.Add(time.Duration(maxIdleMs) * time.Millisecond)
	}

	return info
}

// Touch updates the last used time and resets idle expiration.
func (s *SessionInfo) Touch(maxIdleMs int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	s.LastUsedAt = now
	s.OperationCount++

	if maxIdleMs > 0 {
		s.IdleExpiresAt = now.Add(time.Duration(maxIdleMs) * time.Millisecond)
	}
}

// IsExpired checks if the session has expired (TTL or idle).
func (s *SessionInfo) IsExpired() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()

	// Check TTL expiration
	if !s.ExpiresAt.IsZero() && now.After(s.ExpiresAt) {
		return true
	}

	// Check idle expiration
	if !s.IdleExpiresAt.IsZero() && now.After(s.IdleExpiresAt) {
		return true
	}

	return false
}

// SetState updates the session state.
func (s *SessionInfo) SetState(state SessionState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.State = state
}

// GetState returns the current session state.
func (s *SessionInfo) GetState() SessionState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.State
}

// SessionMetrics contains metrics about session management.
type SessionMetrics struct {
	// ActiveSessions is the current number of active sessions.
	ActiveSessions int64

	// IdleSessions is the current number of idle sessions.
	IdleSessions int64

	// TotalCreated is the total number of sessions created.
	TotalCreated int64

	// TotalEvicted is the total number of sessions evicted.
	TotalEvicted int64

	// TTLEvictions is the number of sessions evicted due to TTL.
	TTLEvictions int64

	// IdleEvictions is the number of sessions evicted due to idle timeout.
	IdleEvictions int64

	// Reconnects is the number of session reconnects (after 404).
	Reconnects int64

	// PoolWaits is the number of times a VU had to wait for a pool slot.
	PoolWaits int64

	// PoolTimeouts is the number of times a VU timed out waiting for a pool slot.
	PoolTimeouts int64
}

// SessionManager defines the interface for session management.
type SessionManager interface {
	// Acquire gets a session for the given VU.
	// For reuse mode, returns the VU's existing session or creates one.
	// For per_request mode, always creates a new session.
	// For pool mode, borrows a session from the pool.
	// For churn mode, may create a new session based on churn interval.
	Acquire(ctx context.Context, vuID string) (*SessionInfo, error)

	// Release returns a session after use.
	// For reuse mode, marks the session as idle.
	// For per_request mode, closes the session.
	// For pool mode, returns the session to the pool.
	// For churn mode, may close the session based on churn policy.
	Release(ctx context.Context, session *SessionInfo) error

	// Invalidate marks a session as invalid (e.g., after 404).
	// The session will be closed and a new one created on next Acquire.
	Invalidate(ctx context.Context, session *SessionInfo) error

	// Close shuts down the session manager and closes all sessions.
	Close(ctx context.Context) error

	// Metrics returns current session metrics.
	Metrics() *SessionMetrics

	// Mode returns the session mode.
	Mode() SessionMode
}

// SessionError represents an error from session management.
type SessionError struct {
	Op      string // Operation that failed
	Session string // Session ID (if available)
	Err     error  // Underlying error
}

func (e *SessionError) Error() string {
	if e.Session != "" {
		return "session " + e.Session + ": " + e.Op + ": " + e.Err.Error()
	}
	return "session: " + e.Op + ": " + e.Err.Error()
}

func (e *SessionError) Unwrap() error {
	return e.Err
}

// Common session errors.
var (
	ErrSessionExpired    = &SessionError{Op: "use", Err: errSessionExpired}
	ErrSessionClosed     = &SessionError{Op: "use", Err: errSessionClosed}
	ErrPoolExhausted     = &SessionError{Op: "acquire", Err: errPoolExhausted}
	ErrPoolTimeout       = &SessionError{Op: "acquire", Err: errPoolTimeout}
	ErrManagerClosed     = &SessionError{Op: "acquire", Err: errManagerClosed}
	ErrInvalidConfig     = &SessionError{Op: "create", Err: errInvalidConfig}
)

// Internal error values for comparison.
var (
	errSessionExpired = errorString("session expired")
	errSessionClosed  = errorString("session closed")
	errPoolExhausted  = errorString("pool exhausted")
	errPoolTimeout    = errorString("pool timeout")
	errManagerClosed  = errorString("manager closed")
	errInvalidConfig  = errorString("invalid configuration")
)

// errorString is a simple error type for internal errors.
type errorString string

func (e errorString) Error() string { return string(e) }
