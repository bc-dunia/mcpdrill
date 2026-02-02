package types

// StreamInfo contains streaming-specific telemetry for an operation.
type StreamInfo struct {
	IsStreaming     bool  `json:"is_streaming"`
	EventsCount     int   `json:"events_count"`
	EndedNormally   bool  `json:"ended_normally"`
	Stalled         bool  `json:"stalled"`
	StallDurationMs int64 `json:"stall_duration_ms"`
}

// OperationOutcome represents a single operation result for telemetry.
type OperationOutcome struct {
	OpID        string      `json:"op_id"`
	Operation   string      `json:"operation"`
	ToolName    string      `json:"tool_name,omitempty"`
	LatencyMs   int         `json:"latency_ms"`
	OK          bool        `json:"ok"`
	ErrorType   string      `json:"error_type,omitempty"`
	ErrorCode   string      `json:"error_code,omitempty"`
	HTTPStatus  int         `json:"http_status,omitempty"`
	TimestampMs int64       `json:"ts_ms"`
	Stream      *StreamInfo `json:"stream,omitempty"`
	WorkerID    string      `json:"worker_id,omitempty"`
	ExecutionID string      `json:"execution_id,omitempty"`
	Stage       string      `json:"stage,omitempty"`
	StageID     string      `json:"stage_id,omitempty"`
	VUID        string      `json:"vu_id,omitempty"`
	SessionID   string      `json:"session_id,omitempty"`
	TokenIndex  *int        `json:"token_index,omitempty"`
}

// ErrorResponse represents a standard API error response.
type ErrorResponse struct {
	ErrorType    string `json:"error_type"`
	ErrorCode    string `json:"error_code"`
	ErrorMessage string `json:"error_message"`
}
