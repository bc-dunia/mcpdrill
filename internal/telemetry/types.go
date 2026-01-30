// Package telemetry provides structured operation logging and metrics collection for mcpdrill.
package telemetry

import (
	"encoding/json"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/transport"
)

// LogTier represents the priority tier of a telemetry record.
// Tier 0 records are never dropped, Tier 2 records can be shed under backpressure.
type LogTier int

const (
	// Tier0Lifecycle represents critical lifecycle events (never dropped).
	Tier0Lifecycle LogTier = 0

	// Tier1Operation represents standard operation logs (dropped under heavy pressure).
	Tier1Operation LogTier = 1

	// Tier2Verbose represents verbose/debug logs (first to be shed).
	Tier2Verbose LogTier = 2
)

// OpLogVersion is the current operation log format version.
const OpLogVersion = "op-log/v1"

// CorrelationKeys contains all required correlation identifiers for an operation log.
// These keys enable distributed tracing and log correlation across the system.
type CorrelationKeys struct {
	// RunID is the unique identifier for the entire load test run.
	RunID string `json:"run_id"`

	// ExecutionID is the unique identifier for this execution instance.
	ExecutionID string `json:"execution_id"`

	// Stage is the current stage name (e.g., "ramp-up", "steady", "ramp-down").
	Stage string `json:"stage"`

	// StageID is the unique identifier for the current stage.
	StageID string `json:"stage_id"`

	// WorkerID is the unique identifier for the worker process.
	WorkerID string `json:"worker_id"`

	// VUID is the unique identifier for the virtual user.
	VUID string `json:"vu_id"`

	// SessionID is the MCP session identifier.
	SessionID string `json:"session_id"`

	// OpID is the unique identifier for this specific operation (optional).
	OpID string `json:"op_id,omitempty"`

	// Attempt is the retry attempt number (1-based, optional).
	Attempt int `json:"attempt,omitempty"`
}

// OpLog represents a single operation log record in op-log/v1 format.
type OpLog struct {
	// Version is the log format version (always "op-log/v1").
	Version string `json:"version"`

	// Timestamp is when the operation completed (RFC3339Nano).
	Timestamp time.Time `json:"timestamp"`

	// Tier is the log priority tier.
	Tier LogTier `json:"tier"`

	// Correlation keys for distributed tracing.
	CorrelationKeys

	// Operation is the type of MCP operation.
	Operation string `json:"operation"`

	// ToolName is the tool name (for tools/call operations).
	ToolName string `json:"tool_name,omitempty"`

	// LatencyMs is the operation latency in milliseconds.
	LatencyMs int64 `json:"latency_ms"`

	// FirstByteMs is time to first byte in milliseconds (optional).
	FirstByteMs *int64 `json:"first_byte_ms,omitempty"`

	// PhaseTiming contains detailed latency decomposition (PRD P0).
	PhaseTiming *PhaseTimingInfo `json:"phase_timing,omitempty"`

	// BytesIn is the number of bytes received.
	BytesIn int64 `json:"bytes_in,omitempty"`

	// BytesOut is the number of bytes sent.
	BytesOut int64 `json:"bytes_out,omitempty"`

	// OK indicates whether the operation succeeded.
	OK bool `json:"ok"`

	// ErrorType is the stable error type (if failed).
	ErrorType string `json:"error_type,omitempty"`

	// ErrorCode is the specific error code (if failed).
	ErrorCode string `json:"error_code,omitempty"`

	// ErrorMessage is the human-readable error message (if failed).
	ErrorMessage string `json:"error_message,omitempty"`

	// Transport is the transport type used (e.g., "streamable-http").
	Transport string `json:"transport,omitempty"`

	// HTTPStatus is the HTTP status code (if applicable).
	HTTPStatus *int `json:"http_status,omitempty"`

	// JSONRPCErrorCode is the JSON-RPC error code (if applicable).
	JSONRPCErrorCode *int `json:"jsonrpc_error_code,omitempty"`

	// ArgumentSize is the JSON byte length of tool call arguments.
	ArgumentSize int `json:"argument_size,omitempty"`

	// ResultSize is the byte length of the result payload.
	ResultSize int `json:"result_size,omitempty"`

	// ArgumentDepth is the maximum JSON nesting level of arguments.
	ArgumentDepth int `json:"argument_depth,omitempty"`

	// ParseError indicates if there was an error parsing arguments.
	ParseError bool `json:"parse_error,omitempty"`

	// ExecutionError indicates if the tool execution failed.
	ExecutionError bool `json:"execution_error,omitempty"`

	// Stream contains streaming-related signals (if applicable).
	Stream *StreamInfo `json:"stream,omitempty"`

	// TraceID is the OpenTelemetry trace ID (W3C format, optional).
	TraceID string `json:"trace_id,omitempty"`

	// SpanID is the OpenTelemetry span ID (W3C format, optional).
	SpanID string `json:"span_id,omitempty"`
}

// StreamInfo contains information about streaming responses.
type StreamInfo struct {
	IsStreaming     bool `json:"is_streaming"`
	EventsCount     int  `json:"events_count,omitempty"`
	EndedNormally   bool `json:"ended_normally,omitempty"`
	Stalled         bool `json:"stalled,omitempty"`
	StallDurationMs int  `json:"stall_duration_ms,omitempty"`

	StreamConnectMs    int64   `json:"stream_connect_ms,omitempty"`
	TimeToFirstEventMs int64   `json:"time_to_first_event_ms,omitempty"`
	StallCount         int     `json:"stall_count,omitempty"`
	TotalStallSeconds  float64 `json:"total_stall_seconds,omitempty"`

	EventGapMin int64   `json:"event_gap_min_ms,omitempty"`
	EventGapMax int64   `json:"event_gap_max_ms,omitempty"`
	EventGapAvg float64 `json:"event_gap_avg_ms,omitempty"`
	EventGapP95 int64   `json:"event_gap_p95_ms,omitempty"`
}

type PhaseTimingInfo struct {
	DNSMs            int64 `json:"dns_ms"`
	TCPConnectMs     int64 `json:"tcp_connect_ms"`
	TLSHandshakeMs   int64 `json:"tls_handshake_ms,omitempty"`
	TTFBMs           int64 `json:"ttfb_ms"`
	DownloadMs       int64 `json:"download_ms"`
	E2EMs            int64 `json:"e2e_ms"`
	ConnectionReused bool  `json:"connection_reused"`
}

// WorkerHealth represents a point-in-time health snapshot of a worker.
type WorkerHealth struct {
	// Timestamp is when the snapshot was taken.
	Timestamp time.Time `json:"timestamp"`

	// WorkerID is the worker identifier.
	WorkerID string `json:"worker_id"`

	// CPUPercent is the CPU usage percentage (0-100).
	CPUPercent float64 `json:"cpu_percent"`

	// MemBytes is the memory usage in bytes.
	MemBytes int64 `json:"mem_bytes"`

	// ActiveVUs is the number of currently active virtual users.
	ActiveVUs int64 `json:"active_vus"`

	// ActiveSessions is the number of active MCP sessions.
	ActiveSessions int64 `json:"active_sessions"`

	// InFlightOps is the number of in-flight operations.
	InFlightOps int64 `json:"in_flight_ops"`

	// QueueDepth is the current telemetry queue depth.
	QueueDepth int `json:"queue_depth"`

	// QueueCapacity is the maximum queue capacity.
	QueueCapacity int `json:"queue_capacity"`

	// DroppedTier2 is the count of dropped Tier 2 records since last snapshot.
	DroppedTier2 int64 `json:"dropped_tier2"`
}

// TelemetryBatch represents a batch of telemetry records for emission.
type TelemetryBatch struct {
	// Records is the list of operation logs in the batch.
	Records []*OpLog `json:"records"`

	// WorkerHealth is the optional worker health snapshot.
	WorkerHealth *WorkerHealth `json:"worker_health,omitempty"`

	// BatchID is a unique identifier for this batch.
	BatchID string `json:"batch_id"`

	// CreatedAt is when the batch was created.
	CreatedAt time.Time `json:"created_at"`
}

// TelemetryRecord is a wrapper that holds either an OpLog or WorkerHealth.
type TelemetryRecord struct {
	// Type indicates the record type ("op_log" or "worker_health").
	Type string `json:"type"`

	// OpLog is the operation log (if Type == "op_log").
	OpLog *OpLog `json:"op_log,omitempty"`

	// WorkerHealth is the worker health snapshot (if Type == "worker_health").
	WorkerHealth *WorkerHealth `json:"worker_health,omitempty"`

	// Tier is the priority tier for queue management.
	Tier LogTier `json:"-"`
}

// NewOpLogFromOutcome creates an OpLog from a transport.OperationOutcome.
func NewOpLogFromOutcome(
	outcome *transport.OperationOutcome,
	keys CorrelationKeys,
	tier LogTier,
) *OpLog {
	log := &OpLog{
		Version:          OpLogVersion,
		Timestamp:        time.Now(),
		Tier:             tier,
		CorrelationKeys:  keys,
		Operation:        string(outcome.Operation),
		ToolName:         outcome.ToolName,
		LatencyMs:        outcome.LatencyMs,
		FirstByteMs:      outcome.FirstByteMs,
		BytesIn:          outcome.BytesIn,
		BytesOut:         outcome.BytesOut,
		OK:               outcome.OK,
		Transport:        outcome.Transport,
		HTTPStatus:       outcome.HTTPStatus,
		JSONRPCErrorCode: outcome.JSONRPCErrorCode,
	}

	if outcome.Error != nil {
		log.ErrorType = string(outcome.Error.Type)
		log.ErrorCode = string(outcome.Error.Code)
		log.ErrorMessage = outcome.Error.Message
	}

	if outcome.PhaseTiming != nil {
		log.PhaseTiming = &PhaseTimingInfo{
			DNSMs:            outcome.PhaseTiming.DNSMs,
			TCPConnectMs:     outcome.PhaseTiming.TCPConnectMs,
			TLSHandshakeMs:   outcome.PhaseTiming.TLSHandshakeMs,
			TTFBMs:           outcome.PhaseTiming.TTFBMs,
			DownloadMs:       outcome.PhaseTiming.DownloadMs,
			E2EMs:            outcome.PhaseTiming.E2EMs,
			ConnectionReused: outcome.PhaseTiming.ConnectionReused,
		}
	}

	if outcome.Stream != nil {
		log.Stream = &StreamInfo{
			IsStreaming:        outcome.Stream.IsStreaming,
			EventsCount:        outcome.Stream.EventsCount,
			EndedNormally:      outcome.Stream.EndedNormally,
			Stalled:            outcome.Stream.Stalled,
			StallDurationMs:    outcome.Stream.StallDurationMs,
			StreamConnectMs:    outcome.Stream.StreamConnectMs,
			TimeToFirstEventMs: outcome.Stream.TimeToFirstEventMs,
			StallCount:         outcome.Stream.StallCount,
			TotalStallSeconds:  outcome.Stream.TotalStallSeconds,
		}
		if outcome.Stream.EventGapHistogram != nil {
			log.Stream.EventGapMin = outcome.Stream.EventGapHistogram.MinGapMs
			log.Stream.EventGapMax = outcome.Stream.EventGapHistogram.MaxGapMs
			log.Stream.EventGapAvg = outcome.Stream.EventGapHistogram.AvgGapMs
			log.Stream.EventGapP95 = outcome.Stream.EventGapHistogram.P95GapMs
		}
	}

	return log
}

// MarshalJSONL marshals the OpLog to a JSONL line (no trailing newline).
func (o *OpLog) MarshalJSONL() ([]byte, error) {
	return json.Marshal(o)
}

// MarshalJSONL marshals the WorkerHealth to a JSONL line (no trailing newline).
func (w *WorkerHealth) MarshalJSONL() ([]byte, error) {
	return json.Marshal(w)
}

// QueueStats contains statistics about the telemetry queue.
type QueueStats struct {
	// Depth is the current number of items in the queue.
	Depth int

	// Capacity is the maximum queue capacity.
	Capacity int

	// TotalEnqueued is the total number of items enqueued.
	TotalEnqueued int64

	// TotalDequeued is the total number of items dequeued.
	TotalDequeued int64

	// DroppedTier2 is the number of Tier 2 items dropped due to backpressure.
	DroppedTier2 int64

	// DroppedTier1 is the number of Tier 1 items dropped due to backpressure.
	DroppedTier1 int64
}

// CollectorConfig holds configuration for the telemetry collector.
type CollectorConfig struct {
	// QueueSize is the maximum number of records in the queue.
	QueueSize int

	// BatchSize is the number of records per batch.
	BatchSize int

	// FlushInterval is how often to flush batches.
	FlushInterval time.Duration

	// WorkerID is the worker identifier for correlation.
	WorkerID string

	// HealthSnapshotInterval is how often to capture worker health.
	HealthSnapshotInterval time.Duration
}

// DefaultCollectorConfig returns sensible defaults for the collector.
func DefaultCollectorConfig() *CollectorConfig {
	return &CollectorConfig{
		QueueSize:              10000,
		BatchSize:              100,
		FlushInterval:          time.Second,
		HealthSnapshotInterval: 5 * time.Second,
	}
}

// EmitterConfig holds configuration for the telemetry emitter.
type EmitterConfig struct {
	// OutputPath is the path to write JSONL output.
	OutputPath string

	// BufferSize is the write buffer size in bytes.
	BufferSize int

	// SyncOnWrite forces sync after each write.
	SyncOnWrite bool
}

// DefaultEmitterConfig returns sensible defaults for the emitter.
func DefaultEmitterConfig() *EmitterConfig {
	return &EmitterConfig{
		BufferSize:  64 * 1024, // 64KB buffer
		SyncOnWrite: false,
	}
}

// ChurnMetrics tracks session churn-specific metrics for stress testing.
// These metrics help identify session lifecycle patterns and connection stability.
type ChurnMetrics struct {
	// SessionsCreated is the total number of sessions created during the test.
	SessionsCreated int64 `json:"sessions_created"`

	// SessionsDestroyed is the total number of sessions destroyed/closed during the test.
	SessionsDestroyed int64 `json:"sessions_destroyed"`

	// ActiveSessions is the current number of active sessions at snapshot time.
	ActiveSessions int `json:"active_sessions"`

	// ReconnectAttempts is the number of reconnection attempts made.
	ReconnectAttempts int64 `json:"reconnect_attempts"`

	// ChurnRate is the calculated churn rate (sessions created + destroyed) per second.
	ChurnRate float64 `json:"churn_rate"`
}

// StreamingMetrics tracks streaming-specific metrics for stop condition evaluation.
// These metrics help detect stream stalls and low event rates.
type StreamingMetrics struct {
	// EventsReceived is the total number of SSE events received during the test.
	EventsReceived int64 `json:"events_received"`

	// LastEventTimeMs is the Unix timestamp (milliseconds) of the last received event.
	LastEventTimeMs int64 `json:"last_event_time_ms"`

	// StreamStallCount is the number of times a stream stall was detected.
	StreamStallCount int `json:"stream_stall_count"`

	// StreamStartTimeMs is the Unix timestamp (milliseconds) when streaming started.
	StreamStartTimeMs int64 `json:"stream_start_time_ms"`
}
