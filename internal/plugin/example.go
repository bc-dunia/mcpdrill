package plugin

import (
	"context"
	"encoding/json"

	"github.com/bc-dunia/mcpdrill/internal/transport"
)

func init() {
	MustRegister(&ResourcesListOperation{})
	MustRegister(&ResourcesReadOperation{})
}

const (
	OpNameResourcesList = "resources/list"
	OpNameResourcesRead = "resources/read"
)

type ResourcesListOperation struct{}

func (o *ResourcesListOperation) Name() string {
	return OpNameResourcesList
}

func (o *ResourcesListOperation) Execute(ctx context.Context, conn transport.Connection, params map[string]interface{}) (*transport.OperationOutcome, error) {
	var cursor *string
	if params != nil {
		if c, ok := params["cursor"].(string); ok {
			cursor = &c
		}
	}
	return conn.ResourcesList(ctx, cursor)
}

func (o *ResourcesListOperation) Validate(params map[string]interface{}) error {
	if params == nil {
		return nil
	}
	if cursor, ok := params["cursor"]; ok {
		if _, isString := cursor.(string); !isString {
			return NewValidationError(OpNameResourcesList, "cursor", "must be a string")
		}
	}
	return nil
}

type ResourcesReadOperation struct{}

func (o *ResourcesReadOperation) Name() string {
	return OpNameResourcesRead
}

func (o *ResourcesReadOperation) Execute(ctx context.Context, conn transport.Connection, params map[string]interface{}) (*transport.OperationOutcome, error) {
	if params == nil {
		return nil, NewOperationError(OpNameResourcesRead, "params required", nil)
	}

	uri, ok := params["uri"].(string)
	if !ok {
		return nil, NewOperationError(OpNameResourcesRead, "uri parameter required", nil)
	}

	readParams := &transport.ResourcesReadParams{
		URI:            uri,
		InputResponses: rawResourceMessageMap(params["input_responses"]),
	}
	if requestState, ok := params["request_state"].(string); ok {
		readParams.RequestState = requestState
	}
	return conn.ResourcesRead(ctx, readParams)
}

func (o *ResourcesReadOperation) Validate(params map[string]interface{}) error {
	if params == nil {
		return NewValidationError(OpNameResourcesRead, "", "params required")
	}

	uri, ok := params["uri"]
	if !ok {
		return NewValidationError(OpNameResourcesRead, "uri", "required")
	}
	uriStr, isString := uri.(string)
	if !isString {
		return NewValidationError(OpNameResourcesRead, "uri", "must be a string")
	}
	if uriStr == "" {
		return NewValidationError(OpNameResourcesRead, "uri", "cannot be empty")
	}

	return nil
}

func rawResourceMessageMap(value interface{}) map[string]json.RawMessage {
	m, ok := value.(map[string]interface{})
	if !ok || len(m) == 0 {
		return nil
	}
	result := make(map[string]json.RawMessage, len(m))
	for k, v := range m {
		data, err := json.Marshal(v)
		if err == nil {
			result[k] = data
		}
	}
	return result
}
