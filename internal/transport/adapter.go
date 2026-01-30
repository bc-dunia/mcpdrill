package transport

import (
	"context"
)

type Adapter interface {
	ID() string
	Connect(ctx context.Context, config *TransportConfig) (Connection, error)
}

type Connection interface {
	Initialize(ctx context.Context, params *InitializeParams) (*OperationOutcome, error)
	SendInitialized(ctx context.Context) (*OperationOutcome, error)
	ToolsList(ctx context.Context, cursor *string) (*OperationOutcome, error)
	ToolsCall(ctx context.Context, params *ToolsCallParams) (*OperationOutcome, error)
	Ping(ctx context.Context) (*OperationOutcome, error)
	ResourcesList(ctx context.Context, cursor *string) (*OperationOutcome, error)
	ResourcesRead(ctx context.Context, params *ResourcesReadParams) (*OperationOutcome, error)
	PromptsList(ctx context.Context, cursor *string) (*OperationOutcome, error)
	PromptsGet(ctx context.Context, params *PromptsGetParams) (*OperationOutcome, error)
	Close() error
	SessionID() string
	SetSessionID(sessionID string)
	SetLastEventID(eventID string)
}

type ResponseHandler interface {
	HandleJSON(data []byte) (*JSONRPCResponse, error)
	HandleSSE(ctx context.Context, reader SSEReader, requestID string) (*JSONRPCResponse, *StreamSignals, error)
}

type SSEReader interface {
	ReadEvent() (*SSEEvent, error)
	Close() error
}
