// Package transport provides MCP transport adapters for mcpdrill.
package transport

import (
	"context"
	"encoding/json"
	"time"
)

// OperationType represents the type of MCP operation.
type OperationType string

const (
	OpInitialize    OperationType = "initialize"
	OpInitialized   OperationType = "notifications/initialized"
	OpToolsList     OperationType = "tools/list"
	OpToolsCall     OperationType = "tools/call"
	OpPing          OperationType = "ping"
	OpResourcesList OperationType = "resources/list"
	OpResourcesRead OperationType = "resources/read"
	OpPromptsList   OperationType = "prompts/list"
	OpPromptsGet    OperationType = "prompts/get"
)

// ErrorType represents the stable error type for operation outcomes.
type ErrorType string

const (
	ErrorTypeDNS         ErrorType = "dns_error"
	ErrorTypeConnect     ErrorType = "connect_error"
	ErrorTypeTLS         ErrorType = "tls_error"
	ErrorTypeTimeout     ErrorType = "timeout"
	ErrorTypeHTTP        ErrorType = "http_error"
	ErrorTypeRateLimited ErrorType = "rate_limited"
	ErrorTypeProtocol    ErrorType = "protocol_error"
	ErrorTypeJSONRPC     ErrorType = "jsonrpc_error"
	ErrorTypeMCP         ErrorType = "mcp_error"
	ErrorTypeTool        ErrorType = "tool_error"
	ErrorTypeUnknown     ErrorType = "unknown"
	ErrorTypeCancelled   ErrorType = "cancelled"
	ErrorTypeStreamStall ErrorType = "stream_stall"
)

// ErrorCode represents specific error codes within an error type.
type ErrorCode string

const (
	CodeDNSLookupFailed ErrorCode = "DNS_LOOKUP_FAILED"
	CodeDNSTimeout      ErrorCode = "DNS_TIMEOUT"

	CodeConnectTimeout     ErrorCode = "CONNECT_TIMEOUT"
	CodeConnectionRefused  ErrorCode = "CONNECTION_REFUSED"
	CodeConnectionReset    ErrorCode = "CONNECTION_RESET"
	CodeNetworkUnreachable ErrorCode = "NETWORK_UNREACHABLE"
	CodeConnectionEOF      ErrorCode = "CONNECTION_EOF"
	CodeSSEDisconnect      ErrorCode = "SSE_DISCONNECT"

	CodeTLSHandshakeFailed  ErrorCode = "TLS_HANDSHAKE_FAILED"
	CodeTLSCertificateError ErrorCode = "TLS_CERTIFICATE_ERROR"

	CodeRequestTimeout     ErrorCode = "REQUEST_TIMEOUT"
	CodeReadTimeout        ErrorCode = "READ_TIMEOUT"
	CodeStreamStallTimeout ErrorCode = "STREAM_STALL_TIMEOUT"

	// HTTP errors
	CodeHTTPBadRequest   ErrorCode = "HTTP_400"
	CodeHTTPUnauthorized ErrorCode = "HTTP_401"
	CodeHTTPForbidden    ErrorCode = "HTTP_403"
	CodeHTTPNotFound     ErrorCode = "HTTP_404"
	CodeHTTPRateLimited  ErrorCode = "HTTP_429"
	CodeHTTPServerError  ErrorCode = "HTTP_5XX"

	// Protocol errors
	CodeJSONParseError ErrorCode = "JSON_PARSE_ERROR"
	CodeInvalidJSONRPC ErrorCode = "INVALID_JSONRPC"
	CodeMissingID      ErrorCode = "MISSING_ID"
	CodeIDMismatch     ErrorCode = "ID_MISMATCH"

	// JSON-RPC errors
	CodeJSONRPCParseError     ErrorCode = "JSONRPC_PARSE_ERROR"
	CodeJSONRPCInvalidRequest ErrorCode = "JSONRPC_INVALID_REQUEST"
	CodeJSONRPCMethodNotFound ErrorCode = "JSONRPC_METHOD_NOT_FOUND"
	CodeJSONRPCInvalidParams  ErrorCode = "JSONRPC_INVALID_PARAMS"
	CodeJSONRPCInternalError  ErrorCode = "JSONRPC_INTERNAL_ERROR"

	// MCP errors
	CodeMCPError ErrorCode = "MCP_ERROR"

	// Tool errors
	CodeToolError ErrorCode = "TOOL_ERROR"

	// Cancelled
	CodeCancelled ErrorCode = "CANCELLED"
)

// OperationError represents an error that occurred during an operation.
type OperationError struct {
	Type    ErrorType              `json:"error_type"`
	Code    ErrorCode              `json:"error_code"`
	Message string                 `json:"error_message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// Error implements the error interface.
func (e *OperationError) Error() string {
	return e.Message
}

// StreamSignals contains information about streaming responses.
// Enhanced for PRD P0 requirement: SSE stream quality metrics.
type StreamSignals struct {
	IsStreaming     bool `json:"is_streaming"`
	EventsCount     int  `json:"events_count,omitempty"`
	EndedNormally   bool `json:"ended_normally,omitempty"`
	Stalled         bool `json:"stalled,omitempty"`
	StallDurationMs int  `json:"stall_duration_ms,omitempty"`

	// PRD P0: Enhanced SSE stream quality metrics
	StreamConnectMs    int64   `json:"stream_connect_ms,omitempty"`
	TimeToFirstEventMs int64   `json:"time_to_first_event_ms,omitempty"`
	StallCount         int     `json:"stall_count,omitempty"`
	TotalStallSeconds  float64 `json:"total_stall_seconds,omitempty"`

	// Event gap histogram buckets (inter-event delays)
	// Bucket boundaries: 0-10ms, 10-50ms, 50-100ms, 100-500ms, 500-1000ms, 1000ms+
	EventGapHistogram *EventGapHistogram `json:"event_gap_histogram,omitempty"`
}

// EventGapHistogram tracks the distribution of inter-event delays in SSE streams.
// This helps identify stream quality issues like bursty delivery or long gaps.
type EventGapHistogram struct {
	// Bucket counts for inter-event delays
	Under10ms     int `json:"under_10ms"`
	From10to50    int `json:"10ms_to_50ms"`
	From50to100   int `json:"50ms_to_100ms"`
	From100to500  int `json:"100ms_to_500ms"`
	From500to1000 int `json:"500ms_to_1000ms"`
	Over1000ms    int `json:"over_1000ms"`

	// Summary statistics
	MinGapMs int64   `json:"min_gap_ms"`
	MaxGapMs int64   `json:"max_gap_ms"`
	AvgGapMs float64 `json:"avg_gap_ms"`
	P50GapMs int64   `json:"p50_gap_ms,omitempty"`
	P95GapMs int64   `json:"p95_gap_ms,omitempty"`
	P99GapMs int64   `json:"p99_gap_ms,omitempty"`
}

// OperationOutcome represents the result of a single MCP operation.
type OperationOutcome struct {
	// Operation identity
	Operation     OperationType `json:"operation"`
	OperationName string        `json:"operation_name,omitempty"`
	ToolName      string        `json:"tool_name,omitempty"`
	JSONRPCID     string        `json:"jsonrpc_id,omitempty"`

	// Timing - End-to-end latency
	StartTime   time.Time `json:"-"`
	LatencyMs   int64     `json:"latency_ms"`
	FirstByteMs *int64    `json:"first_byte_ms,omitempty"`

	// Phase timing decomposition (PRD P0 requirement)
	// All timings are in milliseconds from request start
	PhaseTiming *PhaseTiming `json:"phase_timing,omitempty"`

	// Size
	BytesIn  int64 `json:"bytes_in,omitempty"`
	BytesOut int64 `json:"bytes_out,omitempty"`

	// Protocol signals
	Transport        string `json:"transport"`
	HTTPStatus       *int   `json:"http_status,omitempty"`
	ContentType      string `json:"content_type,omitempty"`
	JSONRPCErrorCode *int   `json:"jsonrpc_error_code,omitempty"`
	MCPErrorCode     string `json:"mcp_error_code,omitempty"`

	// Streaming
	Stream *StreamSignals `json:"stream,omitempty"`

	// Session
	SessionID string `json:"session_id,omitempty"`

	// Outcome
	OK     bool            `json:"ok"`
	Error  *OperationError `json:"error,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
}

// PhaseTiming contains detailed phase timing decomposition for HTTP requests.
// This enables identifying which phase of a request is contributing to latency.
// All values are in milliseconds.
type PhaseTiming struct {
	// DNSMs is the time spent on DNS lookup (0 if connection reused or IP literal)
	DNSMs int64 `json:"dns_ms"`

	// TCPConnectMs is the time spent establishing TCP connection (0 if reused)
	TCPConnectMs int64 `json:"tcp_connect_ms"`

	// TLSHandshakeMs is the time spent on TLS handshake (0 for HTTP or reused)
	TLSHandshakeMs int64 `json:"tls_handshake_ms,omitempty"`

	// TTFBMs is Time To First Byte - from connection ready to first response byte
	TTFBMs int64 `json:"ttfb_ms"`

	// DownloadMs is the time spent downloading the response body
	DownloadMs int64 `json:"download_ms"`

	// E2EMs is the total end-to-end latency (should match LatencyMs)
	E2EMs int64 `json:"e2e_ms"`

	// ConnectionReused indicates if an existing connection was reused
	ConnectionReused bool `json:"connection_reused"`
}

// TimeoutConfig holds timeout settings for transport operations.
type TimeoutConfig struct {
	ConnectTimeout     time.Duration
	RequestTimeout     time.Duration
	StreamStallTimeout time.Duration
}

// DefaultTimeoutConfig returns sensible default timeout values.
func DefaultTimeoutConfig() TimeoutConfig {
	return TimeoutConfig{
		ConnectTimeout:     5 * time.Second,
		RequestTimeout:     30 * time.Second,
		StreamStallTimeout: 15 * time.Second,
	}
}

// RedirectPolicyConfig holds redirect policy configuration.
type RedirectPolicyConfig struct {
	// Mode is the redirect policy mode: "deny", "same_origin", or "allowlist_only"
	Mode string
	// MaxRedirects is the maximum number of redirects to follow (max 3)
	MaxRedirects int
	// Allowlist is a list of allowed redirect target hosts (for allowlist_only mode)
	Allowlist []string
}

// TransportConfig holds configuration for a transport adapter.
type TransportConfig struct {
	// Endpoint is the target URL
	Endpoint string

	// Headers are additional headers to include in requests
	Headers map[string]string

	// Timeouts configuration
	Timeouts TimeoutConfig

	// TLS configuration
	TLSSkipVerify bool
	CABundle      []byte

	// Session ID to include in requests (set after initialize)
	SessionID string

	// AllowPrivateNetworks is a list of CIDR ranges that are allowed
	AllowPrivateNetworks []string

	// RedirectPolicy configuration
	RedirectPolicy *RedirectPolicyConfig

	// LastEventID for SSE resumption
	LastEventID string
}

// JSONRPCRequest represents a JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"` // nil for notifications
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// JSONRPCResponse represents a JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError represents a JSON-RPC 2.0 error.
type JSONRPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// MCP Protocol Types

// InitializeParams contains parameters for the initialize request.
type InitializeParams struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	Capabilities    map[string]interface{} `json:"capabilities"`
	ClientInfo      ClientInfo             `json:"clientInfo"`
}

// ClientInfo contains information about the MCP client.
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// InitializeResult contains the result of an initialize request.
type InitializeResult struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	Capabilities    map[string]interface{} `json:"capabilities"`
	ServerInfo      ServerInfo             `json:"serverInfo"`
	Instructions    string                 `json:"instructions,omitempty"`
}

// ServerInfo contains information about the MCP server.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Tool represents an MCP tool definition.
type Tool struct {
	Name         string           `json:"name"`
	Title        string           `json:"title,omitempty"`
	Description  string           `json:"description,omitempty"`
	InputSchema  json.RawMessage  `json:"inputSchema,omitempty"`
	OutputSchema json.RawMessage  `json:"outputSchema,omitempty"`
	Execution    *ToolExecution   `json:"execution,omitempty"`
	Annotations  *ToolAnnotations `json:"annotations,omitempty"`
	Icons        []Icon           `json:"icons,omitempty"`
}

// ToolExecution describes how a tool supports async tasks.
type ToolExecution struct {
	TaskSupport string `json:"taskSupport,omitempty"`
}

// ToolAnnotations provides hints about tool behavior.
type ToolAnnotations struct {
	Title           string `json:"title,omitempty"`
	ReadOnlyHint    bool   `json:"readOnlyHint,omitempty"`
	DestructiveHint bool   `json:"destructiveHint,omitempty"`
	IdempotentHint  bool   `json:"idempotentHint,omitempty"`
	OpenWorldHint   bool   `json:"openWorldHint,omitempty"`
}

// Icon represents an icon for a tool, resource, or prompt.
type Icon struct {
	Src      string   `json:"src"`
	MimeType string   `json:"mimeType,omitempty"`
	Sizes    []string `json:"sizes,omitempty"`
}

// ToolsListResult contains the result of a tools/list request.
type ToolsListResult struct {
	Tools      []Tool  `json:"tools"`
	NextCursor *string `json:"nextCursor,omitempty"`
}

// ToolsCallParams contains parameters for a tools/call request.
type ToolsCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

// ToolContent represents content returned by a tool.
type ToolContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// ToolsCallResult contains the result of a tools/call request.
type ToolsCallResult struct {
	Content           []ToolContent          `json:"content"`
	StructuredContent map[string]interface{} `json:"structuredContent,omitempty"`
	IsError           bool                   `json:"isError,omitempty"`
}

// Resource represents an MCP resource definition.
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// ResourcesListResult contains the result of a resources/list request.
type ResourcesListResult struct {
	Resources  []Resource `json:"resources"`
	NextCursor *string    `json:"nextCursor,omitempty"`
}

// ResourcesReadParams contains parameters for a resources/read request.
type ResourcesReadParams struct {
	URI string `json:"uri"`
}

// ResourceContent represents content returned by reading a resource.
type ResourceContent struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
	Blob     string `json:"blob,omitempty"`
}

// ResourcesReadResult contains the result of a resources/read request.
type ResourcesReadResult struct {
	Contents []ResourceContent `json:"contents"`
}

// Prompt represents an MCP prompt definition.
type Prompt struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
}

// PromptArgument represents an argument for a prompt.
type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// PromptsListResult contains the result of a prompts/list request.
type PromptsListResult struct {
	Prompts    []Prompt `json:"prompts"`
	NextCursor *string  `json:"nextCursor,omitempty"`
}

// PromptsGetParams contains parameters for a prompts/get request.
type PromptsGetParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

// PromptMessage represents a message in a prompt.
type PromptMessage struct {
	Role    string        `json:"role"`
	Content PromptContent `json:"content"`
}

// PromptContent represents the content of a prompt message.
type PromptContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// PromptsGetResult contains the result of a prompts/get request.
type PromptsGetResult struct {
	Description string          `json:"description,omitempty"`
	Messages    []PromptMessage `json:"messages"`
}

// SSE Event Types

// SSEEvent represents a parsed Server-Sent Event.
type SSEEvent struct {
	ID    string
	Event string
	Data  string
	Retry int
}

// ProgressNotification represents an MCP progress notification.
type ProgressNotification struct {
	ProgressToken interface{} `json:"progressToken"`
	Progress      int         `json:"progress"`
	Total         int         `json:"total,omitempty"`
}

// RequestContext holds context for a single request.
type RequestContext struct {
	Ctx       context.Context
	RequestID string
	SessionID string
}

// NewRequestContext creates a new request context.
func NewRequestContext(ctx context.Context, requestID, sessionID string) *RequestContext {
	return &RequestContext{
		Ctx:       ctx,
		RequestID: requestID,
		SessionID: sessionID,
	}
}
