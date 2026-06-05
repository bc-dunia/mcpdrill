package types

import "encoding/json"

// JSON-RPC Types

// JSONRPCRequest represents a JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"` // nil for notifications
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
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

// ClientInfo contains information about the MCP client.
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ServerInfo contains information about the MCP server.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// InitializeParams contains parameters for the initialize request.
type InitializeParams struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	Capabilities    map[string]interface{} `json:"capabilities"`
	ClientInfo      ClientInfo             `json:"clientInfo"`
}

// InitializeResult contains the result of an initialize request.
type InitializeResult struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	Capabilities    map[string]interface{} `json:"capabilities"`
	ServerInfo      ServerInfo             `json:"serverInfo"`
	Instructions    string                 `json:"instructions,omitempty"`
}

type CacheableResult struct {
	ResultType string `json:"resultType,omitempty"`
	TTLMs      int64  `json:"ttlMs,omitempty"`
	CacheScope string `json:"cacheScope,omitempty"`
}

type InputRequest struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type InputRequiredResult struct {
	ResultType    string                  `json:"resultType"`
	InputRequests map[string]InputRequest `json:"inputRequests,omitempty"`
	RequestState  string                  `json:"requestState,omitempty"`
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
	// TaskSupport indicates task support: "forbidden", "optional", or "required"
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
	CacheableResult
	Tools      []Tool  `json:"tools"`
	NextCursor *string `json:"nextCursor,omitempty"`
}

// ToolsCallParams contains parameters for a tools/call request.
type ToolsCallParams struct {
	Name           string                     `json:"name"`
	Arguments      map[string]interface{}     `json:"arguments,omitempty"`
	InputResponses map[string]json.RawMessage `json:"inputResponses,omitempty"`
	RequestState   string                     `json:"requestState,omitempty"`
}

// ToolContent represents content returned by a tool.
type ToolContent struct {
	Type        string           `json:"type"`
	Text        string           `json:"text,omitempty"`
	Data        string           `json:"data,omitempty"`
	MimeType    string           `json:"mimeType,omitempty"`
	URI         string           `json:"uri,omitempty"`
	Name        string           `json:"name,omitempty"`
	Description string           `json:"description,omitempty"`
	Resource    *ResourceContent `json:"resource,omitempty"`
	Annotations *Annotations     `json:"annotations,omitempty"`
}

type Annotations struct {
	Audience     []string `json:"audience,omitempty"`
	Priority     *float64 `json:"priority,omitempty"`
	LastModified string   `json:"lastModified,omitempty"`
}

// ToolsCallResult contains the result of a tools/call request.
type ToolsCallResult struct {
	ResultType        string        `json:"resultType,omitempty"`
	Content           []ToolContent `json:"content"`
	StructuredContent interface{}   `json:"structuredContent,omitempty"`
	IsError           bool          `json:"isError,omitempty"`
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
	CacheableResult
	Resources  []Resource `json:"resources"`
	NextCursor *string    `json:"nextCursor,omitempty"`
}

// ResourcesReadParams contains parameters for a resources/read request.
type ResourcesReadParams struct {
	URI            string                     `json:"uri"`
	InputResponses map[string]json.RawMessage `json:"inputResponses,omitempty"`
	RequestState   string                     `json:"requestState,omitempty"`
}

// ResourceContent represents content returned by reading a resource.
type ResourceContent struct {
	URI         string       `json:"uri"`
	MimeType    string       `json:"mimeType,omitempty"`
	Text        string       `json:"text,omitempty"`
	Blob        string       `json:"blob,omitempty"`
	Annotations *Annotations `json:"annotations,omitempty"`
}

// ResourcesReadResult contains the result of a resources/read request.
type ResourcesReadResult struct {
	CacheableResult
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
	CacheableResult
	Prompts    []Prompt `json:"prompts"`
	NextCursor *string  `json:"nextCursor,omitempty"`
}

// PromptsGetParams contains parameters for a prompts/get request.
type PromptsGetParams struct {
	Name           string                     `json:"name"`
	Arguments      map[string]interface{}     `json:"arguments,omitempty"`
	InputResponses map[string]json.RawMessage `json:"inputResponses,omitempty"`
	RequestState   string                     `json:"requestState,omitempty"`
}

// PromptMessage represents a message in a prompt.
type PromptMessage struct {
	Role    string        `json:"role"`
	Content PromptContent `json:"content"`
}

// PromptContent represents the content of a prompt message.
type PromptContent struct {
	Type        string           `json:"type"`
	Text        string           `json:"text,omitempty"`
	Data        string           `json:"data,omitempty"`
	MimeType    string           `json:"mimeType,omitempty"`
	Resource    *ResourceContent `json:"resource,omitempty"`
	Annotations *Annotations     `json:"annotations,omitempty"`
}

// PromptsGetResult contains the result of a prompts/get request.
type PromptsGetResult struct {
	Description string          `json:"description,omitempty"`
	Messages    []PromptMessage `json:"messages"`
}

type SubscriptionNotifications struct {
	ToolsListChanged      bool     `json:"toolsListChanged,omitempty"`
	PromptsListChanged    bool     `json:"promptsListChanged,omitempty"`
	ResourcesListChanged  bool     `json:"resourcesListChanged,omitempty"`
	ResourceSubscriptions []string `json:"resourceSubscriptions,omitempty"`
	TaskIDs               []string `json:"taskIds,omitempty"`
}

type SubscriptionsListenParams struct {
	Notifications SubscriptionNotifications `json:"notifications"`
}

type TaskStatus string

const (
	TaskStatusWorking       TaskStatus = "working"
	TaskStatusInputRequired TaskStatus = "input_required"
	TaskStatusCompleted     TaskStatus = "completed"
	TaskStatusFailed        TaskStatus = "failed"
	TaskStatusCancelled     TaskStatus = "cancelled"
)

type Task struct {
	ResultType     string                  `json:"resultType,omitempty"`
	TaskID         string                  `json:"taskId"`
	Status         TaskStatus              `json:"status"`
	CreatedAt      string                  `json:"createdAt,omitempty"`
	LastUpdatedAt  string                  `json:"lastUpdatedAt,omitempty"`
	StatusMessage  string                  `json:"statusMessage,omitempty"`
	Result         json.RawMessage         `json:"result,omitempty"`
	Error          *JSONRPCError           `json:"error,omitempty"`
	InputRequests  map[string]InputRequest `json:"inputRequests,omitempty"`
	TTLMs          int64                   `json:"ttlMs,omitempty"`
	PollIntervalMs int64                   `json:"pollIntervalMs,omitempty"`
}

type CreateTaskResult struct {
	ResultType string `json:"resultType"`
	Task
}

type TasksGetParams struct {
	TaskID string `json:"taskId"`
}

type TasksUpdateParams struct {
	TaskID         string                     `json:"taskId"`
	InputResponses map[string]json.RawMessage `json:"inputResponses,omitempty"`
}

type TasksCancelParams struct {
	TaskID string `json:"taskId"`
}
