package transport

import (
	"encoding/json"
	"fmt"

	"github.com/bc-dunia/mcpdrill/internal/mcp"
)

var (
	MCPProtocolVersion = mcp.DefaultProtocolVersion
	MCPClientName      = mcp.ClientName
	MCPClientVersion   = mcp.ClientVersion
)

func NewInitializeRequest(id string, params *InitializeParams) *JSONRPCRequest {
	if params == nil {
		params = &InitializeParams{
			ProtocolVersion: mcp.DefaultProtocolVersion,
			Capabilities:    map[string]interface{}{},
			ClientInfo: ClientInfo{
				Name:    mcp.ClientName,
				Version: mcp.ClientVersion,
			},
		}
	}
	return &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  string(OpInitialize),
		Params:  *params,
	}
}

func NewInitializedNotification() *JSONRPCRequest {
	return &JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  string(OpInitialized),
		Params:  map[string]interface{}{},
	}
}

func NewServerDiscoverRequest(id string) *JSONRPCRequest {
	return &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  string(OpServerDiscover),
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
	params := ToolsCallParams{
		Name:      toolName,
		Arguments: arguments,
	}
	return NewToolsCallRequestWithParams(id, params)
}

func NewToolsCallRequestWithParams(id string, params ToolsCallParams) *JSONRPCRequest {
	return &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  string(OpToolsCall),
		Params:  params,
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
	params := ResourcesReadParams{URI: uri}
	return NewResourcesReadRequestWithParams(id, params)
}

func NewResourcesReadRequestWithParams(id string, params ResourcesReadParams) *JSONRPCRequest {
	return &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  string(OpResourcesRead),
		Params:  params,
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
	params := PromptsGetParams{Name: name, Arguments: arguments}
	return NewPromptsGetRequestWithParams(id, params)
}

func NewPromptsGetRequestWithParams(id string, params PromptsGetParams) *JSONRPCRequest {
	return &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  string(OpPromptsGet),
		Params:  params,
	}
}

func NewSubscriptionsListenRequest(id string, params *SubscriptionsListenParams) *JSONRPCRequest {
	if params == nil {
		params = &SubscriptionsListenParams{}
	}
	return &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  string(OpSubscriptionsListen),
		Params:  *params,
	}
}

func NewTasksGetRequest(id string, taskID string) *JSONRPCRequest {
	return &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  string(OpTasksGet),
		Params:  TasksGetParams{TaskID: taskID},
	}
}

func NewTasksUpdateRequest(id string, params *TasksUpdateParams) *JSONRPCRequest {
	if params == nil {
		params = &TasksUpdateParams{}
	}
	return &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  string(OpTasksUpdate),
		Params:  *params,
	}
}

func NewTasksCancelRequest(id string, taskID string) *JSONRPCRequest {
	return &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  string(OpTasksCancel),
		Params:  TasksCancelParams{TaskID: taskID},
	}
}

func ParseInitializeResult(data json.RawMessage) (*InitializeResult, error) {
	var result InitializeResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func ParseDiscoverResult(data json.RawMessage) (*DiscoverResult, error) {
	var result DiscoverResult
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
