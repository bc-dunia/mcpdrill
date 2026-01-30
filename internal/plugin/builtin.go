package plugin

import (
	"context"

	"github.com/bc-dunia/mcpdrill/internal/transport"
)

func init() {
	MustRegister(&ToolsListOperation{})
	MustRegister(&ToolsCallOperation{})
	MustRegister(&PingOperation{})
	MustRegister(&PromptsListOperation{})
	MustRegister(&PromptsGetOperation{})
}

const (
	OpNameToolsList   = "tools/list"
	OpNameToolsCall   = "tools/call"
	OpNamePing        = "ping"
	OpNamePromptsList = "prompts/list"
	OpNamePromptsGet  = "prompts/get"
)

type ToolsListOperation struct{}

func (o *ToolsListOperation) Name() string {
	return OpNameToolsList
}

func (o *ToolsListOperation) Execute(ctx context.Context, conn transport.Connection, params map[string]interface{}) (*transport.OperationOutcome, error) {
	var cursor *string
	if params != nil {
		if c, ok := params["cursor"].(string); ok {
			cursor = &c
		}
	}
	return conn.ToolsList(ctx, cursor)
}

func (o *ToolsListOperation) Validate(params map[string]interface{}) error {
	if params == nil {
		return nil
	}
	if cursor, ok := params["cursor"]; ok {
		if _, isString := cursor.(string); !isString {
			return NewValidationError(OpNameToolsList, "cursor", "must be a string")
		}
	}
	return nil
}

type ToolsCallOperation struct{}

func (o *ToolsCallOperation) Name() string {
	return OpNameToolsCall
}

func (o *ToolsCallOperation) Execute(ctx context.Context, conn transport.Connection, params map[string]interface{}) (*transport.OperationOutcome, error) {
	if params == nil {
		return nil, NewOperationError(OpNameToolsCall, "params required", nil)
	}

	name, ok := params["name"].(string)
	if !ok {
		return nil, NewOperationError(OpNameToolsCall, "name parameter required", nil)
	}

	var arguments map[string]interface{}
	if args, ok := params["arguments"].(map[string]interface{}); ok {
		arguments = args
	}

	callParams := &transport.ToolsCallParams{
		Name:      name,
		Arguments: arguments,
	}
	return conn.ToolsCall(ctx, callParams)
}

func (o *ToolsCallOperation) Validate(params map[string]interface{}) error {
	if params == nil {
		return NewValidationError(OpNameToolsCall, "", "params required")
	}

	name, ok := params["name"]
	if !ok {
		return NewValidationError(OpNameToolsCall, "name", "required")
	}
	if _, isString := name.(string); !isString {
		return NewValidationError(OpNameToolsCall, "name", "must be a string")
	}

	if args, ok := params["arguments"]; ok {
		if _, isMap := args.(map[string]interface{}); !isMap {
			return NewValidationError(OpNameToolsCall, "arguments", "must be an object")
		}
	}

	return nil
}

type PingOperation struct{}

func (o *PingOperation) Name() string {
	return OpNamePing
}

func (o *PingOperation) Execute(ctx context.Context, conn transport.Connection, params map[string]interface{}) (*transport.OperationOutcome, error) {
	return conn.Ping(ctx)
}

func (o *PingOperation) Validate(params map[string]interface{}) error {
	return nil
}

// PromptsListOperation handles prompts/list requests.
type PromptsListOperation struct{}

func (o *PromptsListOperation) Name() string {
	return OpNamePromptsList
}

func (o *PromptsListOperation) Execute(ctx context.Context, conn transport.Connection, params map[string]interface{}) (*transport.OperationOutcome, error) {
	var cursor *string
	if params != nil {
		if c, ok := params["cursor"].(string); ok {
			cursor = &c
		}
	}
	return conn.PromptsList(ctx, cursor)
}

func (o *PromptsListOperation) Validate(params map[string]interface{}) error {
	if params == nil {
		return nil
	}
	if cursor, ok := params["cursor"]; ok {
		if _, isString := cursor.(string); !isString {
			return NewValidationError(OpNamePromptsList, "cursor", "must be a string")
		}
	}
	return nil
}

// PromptsGetOperation handles prompts/get requests.
type PromptsGetOperation struct{}

func (o *PromptsGetOperation) Name() string {
	return OpNamePromptsGet
}

func (o *PromptsGetOperation) Execute(ctx context.Context, conn transport.Connection, params map[string]interface{}) (*transport.OperationOutcome, error) {
	if params == nil {
		return nil, NewOperationError(OpNamePromptsGet, "params required", nil)
	}

	name, ok := params["name"].(string)
	if !ok {
		return nil, NewOperationError(OpNamePromptsGet, "name parameter required", nil)
	}

	var arguments map[string]interface{}
	if args, ok := params["arguments"].(map[string]interface{}); ok {
		arguments = args
	}

	getParams := &transport.PromptsGetParams{
		Name:      name,
		Arguments: arguments,
	}
	return conn.PromptsGet(ctx, getParams)
}

func (o *PromptsGetOperation) Validate(params map[string]interface{}) error {
	if params == nil {
		return NewValidationError(OpNamePromptsGet, "", "params required")
	}

	name, ok := params["name"]
	if !ok {
		return NewValidationError(OpNamePromptsGet, "name", "required")
	}
	if _, isString := name.(string); !isString {
		return NewValidationError(OpNamePromptsGet, "name", "must be a string")
	}

	if args, ok := params["arguments"]; ok {
		if _, isMap := args.(map[string]interface{}); !isMap {
			return NewValidationError(OpNamePromptsGet, "arguments", "must be an object")
		}
	}

	return nil
}
