// Package vu provides the Virtual User engine for mcpdrill load generation.
package vu

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/session"
	"github.com/bc-dunia/mcpdrill/internal/transport"
)

// OperationType represents the type of MCP operation to execute.
type OperationType string

const (
	OpToolsList     OperationType = "tools/list"
	OpToolsCall     OperationType = "tools/call"
	OpPing          OperationType = "ping"
	OpResourcesList OperationType = "resources/list"
	OpResourcesRead OperationType = "resources/read"
	OpPromptsList   OperationType = "prompts/list"
	OpPromptsGet    OperationType = "prompts/get"
)

// OperationWeight represents a weighted operation in the mix.
type OperationWeight struct {
	// Operation is the type of operation to execute.
	Operation OperationType `json:"operation"`

	// Weight is the relative weight for sampling (higher = more frequent).
	Weight int `json:"weight"`

	// ToolName is the tool to call (only for tools/call operations).
	ToolName string `json:"tool_name,omitempty"`

	// Arguments are the arguments to pass to the tool (only for tools/call and prompts/get).
	Arguments map[string]interface{} `json:"arguments,omitempty"`

	// URI is the resource URI (only for resources/read operations).
	URI string `json:"uri,omitempty"`

	// PromptName is the prompt name (only for prompts/get operations).
	PromptName string `json:"prompt_name,omitempty"`
}

// OperationMix represents the weighted distribution of operations.
type OperationMix struct {
	// Operations is the list of weighted operations.
	Operations []OperationWeight `json:"operations"`
}

// TotalWeight returns the sum of all operation weights.
func (m *OperationMix) TotalWeight() int {
	total := 0
	for _, op := range m.Operations {
		total += op.Weight
	}
	return total
}

// ThinkTimeConfig configures think time between operations.
type ThinkTimeConfig struct {
	// BaseMs is the base think time in milliseconds.
	BaseMs int64 `json:"base_ms"`

	// JitterMs is the maximum random jitter to add in milliseconds.
	JitterMs int64 `json:"jitter_ms"`
}

type UserJourneyConfig struct {
	StartupSequence *StartupSequenceConfig `json:"startup_sequence,omitempty"`
	PeriodicOps     *PeriodicOpsConfig     `json:"periodic_ops,omitempty"`
	ReconnectPolicy *ReconnectPolicyConfig `json:"reconnect_policy,omitempty"`
}

type StartupSequenceConfig struct {
	RunToolsListOnStart bool `json:"run_tools_list_on_start"`
}

type PeriodicOpsConfig struct {
	ToolsListIntervalMs  int64 `json:"tools_list_interval_ms,omitempty"`
	ToolsListAfterErrors int   `json:"tools_list_after_errors,omitempty"`
}

type ReconnectPolicyConfig struct {
	Enabled        bool    `json:"enabled"`
	InitialDelayMs int64   `json:"initial_delay_ms"`
	MaxDelayMs     int64   `json:"max_delay_ms"`
	Multiplier     float64 `json:"multiplier"`
	JitterFraction float64 `json:"jitter_fraction"`
	MaxRetries     int     `json:"max_retries"`
}

const (
	DefaultToolsListIntervalMs  = 300000
	DefaultReconnectInitialMs   = 100
	DefaultReconnectMaxMs       = 30000
	DefaultReconnectMultiplier  = 2.0
	DefaultReconnectJitter      = 0.2
	DefaultReconnectMaxRetries  = 10
	DefaultToolsListAfterErrors = 3
)

func DefaultUserJourneyConfig() *UserJourneyConfig {
	return &UserJourneyConfig{
		StartupSequence: &StartupSequenceConfig{
			RunToolsListOnStart: true,
		},
		PeriodicOps: &PeriodicOpsConfig{
			ToolsListIntervalMs:  DefaultToolsListIntervalMs,
			ToolsListAfterErrors: DefaultToolsListAfterErrors,
		},
		ReconnectPolicy: &ReconnectPolicyConfig{
			Enabled:        true,
			InitialDelayMs: DefaultReconnectInitialMs,
			MaxDelayMs:     DefaultReconnectMaxMs,
			Multiplier:     DefaultReconnectMultiplier,
			JitterFraction: DefaultReconnectJitter,
			MaxRetries:     DefaultReconnectMaxRetries,
		},
	}
}

// LoadTarget represents the target load for a VU group.
type LoadTarget struct {
	// TargetVUs is the number of virtual users to run.
	TargetVUs int `json:"target_vus"`

	// TargetRPS is the optional target requests per second (0 = unlimited).
	TargetRPS float64 `json:"target_rps,omitempty"`
}

type VUConfig struct {
	RunID            string
	StageID          string
	AssignmentID     string
	WorkerID         string
	LeaseID          string
	Load             LoadTarget
	OperationMix     *OperationMix
	InFlightPerVU    int
	ThinkTime        ThinkTimeConfig
	SessionManager   session.SessionManager
	TransportAdapter transport.Adapter
	TransportConfig  *transport.TransportConfig
	Mode             VUMode
	SwarmConfig      *SwarmConfig
	UserJourney      *UserJourneyConfig
}

// VUMode represents the VU execution mode.
type VUMode string

const (
	// ModeNormal runs a fixed set of VUs for the entire duration.
	ModeNormal VUMode = "normal"

	// ModeSwarm continuously creates and terminates VUs.
	ModeSwarm VUMode = "swarm"
)

// SwarmConfig configures swarm mode behavior.
type SwarmConfig struct {
	// SpawnIntervalMs is the interval between spawning new VUs.
	SpawnIntervalMs int64 `json:"spawn_interval_ms"`

	// VULifetimeMs is how long each VU lives before termination.
	VULifetimeMs int64 `json:"vu_lifetime_ms"`

	// MaxConcurrentVUs is the maximum concurrent VUs at any time.
	MaxConcurrentVUs int `json:"max_concurrent_vus"`
}

// DefaultSwarmConfig returns sensible defaults for swarm mode.
func DefaultSwarmConfig() *SwarmConfig {
	return &SwarmConfig{
		SpawnIntervalMs:  1000,  // 1 second
		VULifetimeMs:     30000, // 30 seconds
		MaxConcurrentVUs: 100,
	}
}

// VUState represents the state of a virtual user.
type VUState string

const (
	// StateIdle indicates the VU is not running.
	StateIdle VUState = "idle"

	// StateInitializing indicates the VU is acquiring a session.
	StateInitializing VUState = "initializing"

	// StateRunning indicates the VU is executing operations.
	StateRunning VUState = "running"

	// StateDraining indicates the VU is finishing current operations.
	StateDraining VUState = "draining"

	// StateStopped indicates the VU has stopped.
	StateStopped VUState = "stopped"
)

// VUInstance represents a single virtual user.
type VUInstance struct {
	// ID is the unique VU identifier.
	ID string

	// State is the current VU state.
	state atomic.Value // VUState

	// Session is the current session (if any).
	session *session.SessionInfo

	// RNGSeed is the random seed for this VU.
	RNGSeed int64

	// StartedAt is when the VU started.
	StartedAt time.Time

	// StoppedAt is when the VU stopped (zero if still running).
	StoppedAt time.Time

	// OperationsCompleted is the count of completed operations.
	OperationsCompleted atomic.Int64

	// OperationsFailed is the count of failed operations.
	OperationsFailed atomic.Int64

	// cancel is the context cancel function for this VU.
	cancel context.CancelFunc

	// mu protects mutable fields.
	mu sync.RWMutex
}

// NewVUInstance creates a new VU instance.
func NewVUInstance(id string, seed int64) *VUInstance {
	vu := &VUInstance{
		ID:      id,
		RNGSeed: seed,
	}
	vu.state.Store(StateIdle)
	return vu
}

// State returns the current VU state.
func (v *VUInstance) State() VUState {
	return v.state.Load().(VUState)
}

// SetState sets the VU state.
func (v *VUInstance) SetState(state VUState) {
	v.state.Store(state)
}

// SetSession sets the VU's session.
func (v *VUInstance) SetSession(s *session.SessionInfo) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.session = s
}

// GetSession returns the VU's session.
func (v *VUInstance) GetSession() *session.SessionInfo {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.session
}

// VUMetrics contains metrics about VU execution.
type VUMetrics struct {
	// ActiveVUs is the current number of active VUs.
	ActiveVUs atomic.Int64

	// TotalVUsCreated is the total number of VUs created.
	TotalVUsCreated atomic.Int64

	// TotalVUsTerminated is the total number of VUs terminated.
	TotalVUsTerminated atomic.Int64

	// TotalOperations is the total number of operations executed.
	TotalOperations atomic.Int64

	// SuccessfulOperations is the number of successful operations.
	SuccessfulOperations atomic.Int64

	// FailedOperations is the number of failed operations.
	FailedOperations atomic.Int64

	// RateLimitedOperations is the number of operations that were rate limited.
	RateLimitedOperations atomic.Int64

	// InFlightOperations is the current number of in-flight operations.
	InFlightOperations atomic.Int64

	// MaxInFlightReached is the maximum in-flight operations reached.
	MaxInFlightReached atomic.Int64

	// ThinkTimeTotal is the total think time in milliseconds.
	ThinkTimeTotal atomic.Int64

	// SessionAcquires is the number of session acquires.
	SessionAcquires atomic.Int64

	// SessionReleases is the number of session releases.
	SessionReleases atomic.Int64

	// SessionErrors is the number of session errors.
	SessionErrors atomic.Int64

	// SessionsCreated tracks total sessions created (for churn metrics).
	SessionsCreated atomic.Int64

	// SessionsDestroyed tracks total sessions destroyed (for churn metrics).
	SessionsDestroyed atomic.Int64

	// ActiveSessions tracks current active session count (for churn metrics).
	ActiveSessions atomic.Int64

	// ReconnectAttempts tracks reconnection attempts (for churn metrics).
	ReconnectAttempts atomic.Int64

	// DroppedResults tracks results dropped due to full result channel.
	DroppedResults atomic.Int64
}

// NewVUMetrics creates a new VUMetrics instance.
func NewVUMetrics() *VUMetrics {
	return &VUMetrics{}
}

// Snapshot returns a copy of the current metrics.
func (m *VUMetrics) Snapshot() VUMetricsSnapshot {
	return VUMetricsSnapshot{
		ActiveVUs:             m.ActiveVUs.Load(),
		TotalVUsCreated:       m.TotalVUsCreated.Load(),
		TotalVUsTerminated:    m.TotalVUsTerminated.Load(),
		TotalOperations:       m.TotalOperations.Load(),
		SuccessfulOperations:  m.SuccessfulOperations.Load(),
		FailedOperations:      m.FailedOperations.Load(),
		RateLimitedOperations: m.RateLimitedOperations.Load(),
		InFlightOperations:    m.InFlightOperations.Load(),
		MaxInFlightReached:    m.MaxInFlightReached.Load(),
		ThinkTimeTotal:        m.ThinkTimeTotal.Load(),
		SessionAcquires:       m.SessionAcquires.Load(),
		SessionReleases:       m.SessionReleases.Load(),
		SessionErrors:         m.SessionErrors.Load(),
		SessionsCreated:       m.SessionsCreated.Load(),
		SessionsDestroyed:     m.SessionsDestroyed.Load(),
		ActiveSessions:        m.ActiveSessions.Load(),
		ReconnectAttempts:     m.ReconnectAttempts.Load(),
		DroppedResults:        m.DroppedResults.Load(),
	}
}

// VUMetricsSnapshot is a point-in-time snapshot of VU metrics.
type VUMetricsSnapshot struct {
	ActiveVUs             int64
	TotalVUsCreated       int64
	TotalVUsTerminated    int64
	TotalOperations       int64
	SuccessfulOperations  int64
	FailedOperations      int64
	RateLimitedOperations int64
	InFlightOperations    int64
	MaxInFlightReached    int64
	ThinkTimeTotal        int64
	SessionAcquires       int64
	SessionReleases       int64
	SessionErrors         int64
	SessionsCreated       int64
	SessionsDestroyed     int64
	ActiveSessions        int64
	ReconnectAttempts     int64
	DroppedResults        int64
}

// OperationResult represents the result of executing an operation.
type OperationResult struct {
	// Operation is the operation that was executed.
	Operation OperationType

	// ToolName is the tool name (for tools/call).
	ToolName string

	// Outcome is the transport-level outcome.
	Outcome *transport.OperationOutcome

	// VUID is the VU that executed the operation.
	VUID string

	// SessionID is the session used.
	SessionID string

	// StartTime is when the operation started.
	StartTime time.Time

	// EndTime is when the operation completed.
	EndTime time.Time

	// TraceID is the OpenTelemetry trace ID (optional).
	TraceID string

	// SpanID is the OpenTelemetry span ID (optional).
	SpanID string

	// ToolMetrics contains tool-specific telemetry (for tools/call).
	ToolMetrics *ToolCallMetrics
}

// ToolCallMetrics captures telemetry data for tool executions.
// These metrics enable per-tool performance analysis and error tracking.
type ToolCallMetrics struct {
	// ToolName is the name of the tool being called.
	ToolName string

	// ArgumentSize is the JSON byte length of the arguments.
	ArgumentSize int

	// ResultSize is the byte length of the result payload.
	ResultSize int

	// ArgumentDepth is the maximum nesting level of the arguments.
	// Primitives = 0, objects/arrays add 1 per level.
	ArgumentDepth int

	// ParseError indicates if there was an error parsing arguments.
	ParseError bool

	// ExecutionError indicates if the tool execution itself failed.
	ExecutionError bool
}

// VUEngineError represents an error from the VU engine.
type VUEngineError struct {
	Op   string // Operation that failed
	VUID string // VU ID (if applicable)
	Err  error  // Underlying error
}

func (e *VUEngineError) Error() string {
	if e.VUID != "" {
		return "vu " + e.VUID + ": " + e.Op + ": " + e.Err.Error()
	}
	return "vu engine: " + e.Op + ": " + e.Err.Error()
}

func (e *VUEngineError) Unwrap() error {
	return e.Err
}

// Common VU engine errors.
var (
	ErrEngineClosed     = &VUEngineError{Op: "execute", Err: errEngineClosed}
	ErrInvalidConfig    = &VUEngineError{Op: "create", Err: errInvalidConfig}
	ErrNoOperations     = &VUEngineError{Op: "sample", Err: errNoOperations}
	ErrRateLimited      = &VUEngineError{Op: "execute", Err: errRateLimited}
	ErrInFlightExceeded = &VUEngineError{Op: "execute", Err: errInFlightExceeded}
)

// Internal error values.
var (
	errEngineClosed     = errorString("engine closed")
	errInvalidConfig    = errorString("invalid configuration")
	errNoOperations     = errorString("no operations in mix")
	errRateLimited      = errorString("rate limited")
	errInFlightExceeded = errorString("in-flight limit exceeded")
)

type errorString string

func (e errorString) Error() string { return string(e) }
