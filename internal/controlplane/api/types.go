package api

import (
	"encoding/json"

	"github.com/bc-dunia/mcpdrill/internal/analysis"
	"github.com/bc-dunia/mcpdrill/internal/controlplane/runmanager"
	"github.com/bc-dunia/mcpdrill/internal/controlplane/scheduler"
	"github.com/bc-dunia/mcpdrill/internal/metrics"
	"github.com/bc-dunia/mcpdrill/internal/types"
	"github.com/bc-dunia/mcpdrill/internal/validation"
)

// CreateRunRequest is the request body for POST /runs.
type CreateRunRequest struct {
	Config json.RawMessage `json:"config"`
	Actor  string          `json:"actor"`
}

// CreateRunResponse is the response body for POST /runs.
type CreateRunResponse struct {
	RunID string `json:"run_id"`
}

// ValidateConfigRequest is the request body for POST /runs/{id}/validate.
type ValidateConfigRequest struct {
	Config json.RawMessage `json:"config"`
}

// ValidateConfigResponse is the response body for POST /runs/{id}/validate.
type ValidateConfigResponse struct {
	OK       bool                         `json:"ok"`
	Errors   []validation.ValidationIssue `json:"errors,omitempty"`
	Warnings []validation.ValidationIssue `json:"warnings,omitempty"`
}

// StartRunRequest is the request body for POST /runs/{id}/start.
type StartRunRequest struct {
	Actor string `json:"actor"`
}

// StartRunResponse is the response body for POST /runs/{id}/start.
type StartRunResponse struct {
	RunID string `json:"run_id"`
	State string `json:"state"`
}

// StopRunRequest is the request body for POST /runs/{id}/stop.
type StopRunRequest struct {
	Mode  string `json:"mode"` // "drain" or "immediate"
	Actor string `json:"actor"`
}

// StopRunResponse is the response body for POST /runs/{id}/stop.
type StopRunResponse struct {
	RunID string `json:"run_id"`
	State string `json:"state"`
}

// EmergencyStopRequest is the request body for POST /runs/{id}/emergency-stop.
type EmergencyStopRequest struct {
	Actor string `json:"actor"`
}

// EmergencyStopResponse is the response body for POST /runs/{id}/emergency-stop.
type EmergencyStopResponse struct {
	RunID string `json:"run_id"`
	State string `json:"state"`
}

// GetRunResponse is the response body for GET /runs/{id}.
// It wraps RunView from runmanager.
type GetRunResponse struct {
	*runmanager.RunView
}

// CloneRunRequest is the request body for POST /runs/{id}/clone.
type CloneRunRequest struct {
	Actor string `json:"actor"`
}

// CloneRunResponse is the response body for POST /runs/{id}/clone.
type CloneRunResponse struct {
	RunID string `json:"run_id"`
}

// ErrorResponse is the standard error response format.
// Matches ref/04-data-models.md Section 5.1 error envelope.
type ErrorResponse struct {
	ErrorType    string                 `json:"error_type"`
	ErrorCode    string                 `json:"error_code"`
	ErrorMessage string                 `json:"error_message"`
	Retryable    bool                   `json:"retryable"`
	Details      map[string]interface{} `json:"details,omitempty"`
}

// HealthResponse is the response body for GET /healthz.
type HealthResponse struct {
	Status string `json:"status"`
}

// ReadyResponse is the response body for GET /readyz.
type ReadyResponse struct {
	Status string `json:"status"`
	Ready  bool   `json:"ready"`
}

// ErrorType constants for API errors.
// Matches ref/04-data-models.md Section 5.1 APIErrorType enum.
const (
	ErrorTypeInvalidArgument    = "invalid_argument"
	ErrorTypeFailedPrecondition = "failed_precondition"
	ErrorTypeNotFound           = "not_found"
	ErrorTypeUnauthorized       = "unauthorized"
	ErrorTypeForbidden          = "forbidden"
	ErrorTypeRateLimited        = "rate_limited"
	ErrorTypeResourceExhausted  = "resource_exhausted"
	ErrorTypeTimeout            = "timeout"
	ErrorTypeUnavailable        = "unavailable"
	ErrorTypeConflict           = "conflict"
	ErrorTypeInternal           = "internal"
	ErrorTypeNotImplemented     = "not_implemented"
)

// ErrorCode constants for specific error conditions.
const (
	ErrorCodeValidationFailed = "VALIDATION_FAILED"
	ErrorCodeRunNotFound      = "RUN_NOT_FOUND"
	ErrorCodeInvalidState     = "INVALID_STATE"
	ErrorCodeInvalidRequest   = "INVALID_REQUEST"
	ErrorCodeInvalidStopMode  = "INVALID_STOP_MODE"
	ErrorCodeTerminalState    = "TERMINAL_STATE"
	ErrorCodeInternalError    = "INTERNAL_ERROR"
	ErrorCodeMethodNotAllowed = "METHOD_NOT_ALLOWED"
)

// NewErrorResponse creates a new ErrorResponse.
func NewErrorResponse(errorType, errorCode, message string, retryable bool, details map[string]interface{}) *ErrorResponse {
	return &ErrorResponse{
		ErrorType:    errorType,
		ErrorCode:    errorCode,
		ErrorMessage: message,
		Retryable:    retryable,
		Details:      details,
	}
}

// NewValidationErrorResponse creates an error response for validation failures.
func NewValidationErrorResponse(report *validation.ValidationReport) *ErrorResponse {
	details := make(map[string]interface{})
	if len(report.Errors) > 0 {
		errors := make([]map[string]interface{}, len(report.Errors))
		for i, e := range report.Errors {
			errors[i] = map[string]interface{}{
				"code":    e.Code,
				"message": e.Message,
			}
			if e.JSONPointer != "" {
				errors[i]["json_pointer"] = e.JSONPointer
			}
		}
		details["errors"] = errors
	}
	if len(report.Warnings) > 0 {
		warnings := make([]map[string]interface{}, len(report.Warnings))
		for i, w := range report.Warnings {
			warnings[i] = map[string]interface{}{
				"code":    w.Code,
				"message": w.Message,
			}
			if w.JSONPointer != "" {
				warnings[i]["json_pointer"] = w.JSONPointer
			}
		}
		details["warnings"] = warnings
	}

	return &ErrorResponse{
		ErrorType:    ErrorTypeInvalidArgument,
		ErrorCode:    ErrorCodeValidationFailed,
		ErrorMessage: "Run configuration validation failed",
		Retryable:    false,
		Details:      details,
	}
}

// NewNotFoundErrorResponse creates an error response for run not found.
func NewNotFoundErrorResponse(runID string) *ErrorResponse {
	return &ErrorResponse{
		ErrorType:    ErrorTypeNotFound,
		ErrorCode:    ErrorCodeRunNotFound,
		ErrorMessage: "Run not found",
		Retryable:    false,
		Details: map[string]interface{}{
			"run_id": runID,
		},
	}
}

// NewInvalidStateErrorResponse creates an error response for invalid state transitions.
func NewInvalidStateErrorResponse(runID, currentState, operation string) *ErrorResponse {
	return &ErrorResponse{
		ErrorType:    ErrorTypeConflict,
		ErrorCode:    ErrorCodeInvalidState,
		ErrorMessage: "Invalid state for operation",
		Retryable:    false,
		Details: map[string]interface{}{
			"run_id":        runID,
			"current_state": currentState,
			"operation":     operation,
		},
	}
}

// NewTerminalStateErrorResponse creates an error response for operations on terminal state runs.
func NewTerminalStateErrorResponse(runID, currentState, operation string) *ErrorResponse {
	return &ErrorResponse{
		ErrorType:    ErrorTypeFailedPrecondition,
		ErrorCode:    ErrorCodeTerminalState,
		ErrorMessage: "Run is in terminal state",
		Retryable:    false,
		Details: map[string]interface{}{
			"run_id":        runID,
			"current_state": currentState,
			"operation":     operation,
		},
	}
}

// NewInvalidRequestErrorResponse creates an error response for invalid requests.
func NewInvalidRequestErrorResponse(message string, details map[string]interface{}) *ErrorResponse {
	return &ErrorResponse{
		ErrorType:    ErrorTypeInvalidArgument,
		ErrorCode:    ErrorCodeInvalidRequest,
		ErrorMessage: message,
		Retryable:    false,
		Details:      details,
	}
}

// NewInternalErrorResponse creates an error response for internal errors.
func NewInternalErrorResponse(message string) *ErrorResponse {
	return &ErrorResponse{
		ErrorType:    ErrorTypeInternal,
		ErrorCode:    ErrorCodeInternalError,
		ErrorMessage: message,
		Retryable:    true,
		Details:      nil,
	}
}

// RegisterWorkerRequest is the request body for POST /workers/register.
type RegisterWorkerRequest struct {
	HostInfo types.HostInfo       `json:"host_info"`
	Capacity types.WorkerCapacity `json:"capacity"`
}

// RegisterWorkerResponse is the response body for POST /workers/register.
type RegisterWorkerResponse struct {
	WorkerID    string `json:"worker_id"`
	WorkerToken string `json:"worker_token,omitempty"`
}

// ListWorkersResponse is the response body for GET /workers.
type ListWorkersResponse struct {
	Workers []*scheduler.WorkerInfo `json:"workers"`
}

// HeartbeatRequest is the request body for POST /workers/{id}/heartbeat.
type HeartbeatRequest struct {
	Health *types.WorkerHealth `json:"health,omitempty"`
}

// HeartbeatResponse is the response body for POST /workers/{id}/heartbeat.
type HeartbeatResponse struct {
	OK                  bool     `json:"ok"`
	StopRunIDs          []string `json:"stop_run_ids,omitempty"`
	ImmediateStopRunIDs []string `json:"immediate_stop_run_ids,omitempty"`
}

// TelemetryBatchRequest is the request body for POST /workers/{id}/telemetry.
type TelemetryBatchRequest struct {
	RunID      string                   `json:"run_id"`
	Operations []types.OperationOutcome `json:"operations"`
	Health     *types.WorkerHealth      `json:"health,omitempty"`
}

// TelemetryBatchResponse is the response body for POST /workers/{id}/telemetry.
type TelemetryBatchResponse struct {
	Accepted int `json:"accepted"`
}

// ErrorCode constants for worker-related errors.
const (
	ErrorCodeWorkerNotFound = "WORKER_NOT_FOUND"
)

// GetAssignmentsResponse is the response body for GET /workers/{id}/assignments.
type GetAssignmentsResponse struct {
	Assignments []types.WorkerAssignment `json:"assignments"`
}

// AckAssignmentsRequest is the request body for POST /workers/{id}/assignments/ack.
type AckAssignmentsRequest struct {
	LeaseIDs []string `json:"lease_ids"`
}

// AckAssignmentsResponse is the response body for POST /workers/{id}/assignments/ack.
type AckAssignmentsResponse struct {
	Acknowledged int `json:"acknowledged"`
}

// OperationLog represents a single operation log entry with full context.
// Used for log query API responses.
type OperationLog struct {
	TimestampMs int64             `json:"timestamp_ms"`
	RunID       string            `json:"run_id"`
	ExecutionID string            `json:"execution_id,omitempty"`
	Stage       string            `json:"stage,omitempty"`
	StageID     string            `json:"stage_id,omitempty"`
	WorkerID    string            `json:"worker_id,omitempty"`
	VUID        string            `json:"vu_id,omitempty"`
	SessionID   string            `json:"session_id,omitempty"`
	Operation   string            `json:"operation"`
	ToolName    string            `json:"tool_name,omitempty"`
	LatencyMs   int               `json:"latency_ms"`
	OK          bool              `json:"ok"`
	ErrorType   string            `json:"error_type,omitempty"`
	ErrorCode   string            `json:"error_code,omitempty"`
	Stream      *types.StreamInfo `json:"stream,omitempty"`
	TokenIndex  *int              `json:"token_index,omitempty"`
}

// LogFilters contains filter parameters for log queries.
type LogFilters struct {
	Stage      string
	StageID    string
	WorkerID   string
	VUID       string
	SessionID  string
	Operation  string
	ToolName   string
	ErrorType  string
	ErrorCode  string
	TokenIndex *int
	Limit      int
	Offset     int
	Order      string // "asc" or "desc"
}

// LogQueryResponse is the response body for GET /runs/{id}/logs.
type LogQueryResponse struct {
	RunID         string         `json:"run_id"`
	Total         int            `json:"total"`
	Offset        int            `json:"offset"`
	Limit         int            `json:"limit"`
	Logs          []OperationLog `json:"logs"`
	LogsTruncated bool           `json:"logs_truncated,omitempty"`
}

// ListRunsResponse is the response body for GET /runs.
type ListRunsResponse struct {
	Runs []*runmanager.RunView `json:"runs"`
}

// RunMetricsResponse is the response body for GET /runs/{id}/metrics.
type RunMetricsResponse struct {
	RunID               string                                `json:"run_id"`
	Throughput          float64                               `json:"throughput"`
	LatencyP50          float64                               `json:"latency_p50_ms"`
	LatencyP95          float64                               `json:"latency_p95_ms"`
	LatencyP99          float64                               `json:"latency_p99_ms"`
	ErrorRate           float64                               `json:"error_rate"`
	TotalOps            int64                                 `json:"total_ops"`
	FailedOps           int64                                 `json:"failed_ops"`
	DurationMs          int64                                 `json:"duration_ms"`
	ByTool              map[string]*analysis.OperationMetrics `json:"by_tool,omitempty"`
	TimeSeriesData      []metrics.MetricsTimePoint            `json:"time_series,omitempty"`
	OperationsTruncated bool                                  `json:"operations_truncated,omitempty"`
}

// CompareRunsResponse represents an A/B comparison of two runs
type CompareRunsResponse struct {
	RunA RunMetricsResponse `json:"run_a"`
	RunB RunMetricsResponse `json:"run_b"`
}

// StabilityResponse is the response body for GET /runs/{id}/stability.
type StabilityResponse struct {
	RunID                string                       `json:"run_id"`
	TotalSessions        int64                        `json:"total_sessions"`
	ActiveSessions       int64                        `json:"active_sessions"`
	DroppedSessions      int64                        `json:"dropped_sessions"`
	TerminatedSessions   int64                        `json:"terminated_sessions"`
	AvgSessionLifetimeMs float64                      `json:"avg_session_lifetime_ms"`
	ReconnectRate        float64                      `json:"reconnect_rate"`
	ProtocolErrorRate    float64                      `json:"protocol_error_rate"`
	ConnectionChurnRate  float64                      `json:"connection_churn_rate"`
	StabilityScore       float64                      `json:"stability_score"`
	DropRate             float64                      `json:"drop_rate"`
	Events               []metrics.ConnectionEvent    `json:"events,omitempty"`
	SessionMetrics       []metrics.ConnectionMetrics  `json:"session_metrics,omitempty"`
	TimeSeriesData       []metrics.StabilityTimePoint `json:"time_series,omitempty"`
	DataTruncated        bool                         `json:"data_truncated,omitempty"`
}
