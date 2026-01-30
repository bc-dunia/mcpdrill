package transport

import (
	"encoding/json"
	"fmt"
)

const (
	MCPProtocolVersion = "2025-03-26"
	MCPClientName      = "mcpdrill"
	MCPClientVersion   = "1.0.0"
)

func NewInitializeRequest(id string) *JSONRPCRequest {
	return &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  string(OpInitialize),
		Params: InitializeParams{
			ProtocolVersion: MCPProtocolVersion,
			Capabilities:    map[string]interface{}{},
			ClientInfo: ClientInfo{
				Name:    MCPClientName,
				Version: MCPClientVersion,
			},
		},
	}
}

func NewInitializedNotification() *JSONRPCRequest {
	return &JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  string(OpInitialized),
		Params:  map[string]interface{}{},
	}
}

func NewToolsListRequest(id string, cursor *string) *JSONRPCRequest {
	params := map[string]interface{}{}
	if cursor != nil {
		params["cursor"] = *cursor
	}
	return &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  string(OpToolsList),
		Params:  params,
	}
}

func NewToolsCallRequest(id string, toolName string, arguments map[string]interface{}) *JSONRPCRequest {
	return &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  string(OpToolsCall),
		Params: ToolsCallParams{
			Name:      toolName,
			Arguments: arguments,
		},
	}
}

func NewPingRequest(id string) *JSONRPCRequest {
	return &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  string(OpPing),
		Params:  map[string]interface{}{},
	}
}

func NewResourcesListRequest(id string, cursor *string) *JSONRPCRequest {
	params := map[string]interface{}{}
	if cursor != nil {
		params["cursor"] = *cursor
	}
	return &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  string(OpResourcesList),
		Params:  params,
	}
}

func NewResourcesReadRequest(id string, uri string) *JSONRPCRequest {
	return &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  string(OpResourcesRead),
		Params: ResourcesReadParams{
			URI: uri,
		},
	}
}

func NewPromptsListRequest(id string, cursor *string) *JSONRPCRequest {
	params := map[string]interface{}{}
	if cursor != nil {
		params["cursor"] = *cursor
	}
	return &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  string(OpPromptsList),
		Params:  params,
	}
}

func NewPromptsGetRequest(id string, name string, arguments map[string]interface{}) *JSONRPCRequest {
	return &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  string(OpPromptsGet),
		Params: PromptsGetParams{
			Name:      name,
			Arguments: arguments,
		},
	}
}

func ParseInitializeResult(data json.RawMessage) (*InitializeResult, error) {
	var result InitializeResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func ParseToolsListResult(data json.RawMessage) (*ToolsListResult, error) {
	var result ToolsListResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func ParseToolsCallResult(data json.RawMessage) (*ToolsCallResult, error) {
	var result ToolsCallResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func ValidateJSONRPCResponse(resp *JSONRPCResponse, expectedID string) *OperationError {
	if resp.JSONRPC != "2.0" {
		return &OperationError{
			Type:    ErrorTypeProtocol,
			Code:    CodeInvalidJSONRPC,
			Message: "invalid JSON-RPC version",
		}
	}

	if resp.ID == nil {
		return &OperationError{
			Type:    ErrorTypeProtocol,
			Code:    CodeMissingID,
			Message: "missing response ID",
		}
	}

	respID, ok := resp.ID.(string)
	if !ok {
		if numID, ok := resp.ID.(float64); ok {
			respID = fmt.Sprintf("%v", numID)
		} else {
			respID = fmt.Sprintf("%v", resp.ID)
		}
	}

	if respID != expectedID {
		return &OperationError{
			Type:    ErrorTypeProtocol,
			Code:    CodeIDMismatch,
			Message: "response ID does not match request ID",
			Details: map[string]interface{}{
				"expected": expectedID,
				"actual":   resp.ID,
			},
		}
	}

	return nil
}

func ExtractJSONRPCError(resp *JSONRPCResponse) *OperationError {
	if resp.Error == nil {
		return nil
	}

	return MapJSONRPCError(resp.Error.Code, resp.Error.Message, resp.Error.Data)
}

func CheckToolError(result *ToolsCallResult, toolName string) *OperationError {
	if result.IsError {
		return MapToolError(toolName, result.Content)
	}
	return nil
}
