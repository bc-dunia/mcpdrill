package plugin

import (
	"context"
	"encoding/json"

	"github.com/bc-dunia/mcpdrill/internal/transport"
)

func init() {
	MustRegister(&ToolsListOperation{})
	MustRegister(&ToolsCallOperation{})
	MustRegister(&PingOperation{})
	MustRegister(&PromptsListOperation{})
	MustRegister(&PromptsGetOperation{})
	MustRegister(&SubscriptionsListenOperation{})
	MustRegister(&TasksGetOperation{})
	MustRegister(&TasksUpdateOperation{})
	MustRegister(&TasksCancelOperation{})
}

const (
	OpNameToolsList           = "tools/list"
	OpNameToolsCall           = "tools/call"
	OpNamePing                = "ping"
	OpNamePromptsList         = "prompts/list"
	OpNamePromptsGet          = "prompts/get"
	OpNameSubscriptionsListen = "subscriptions/listen"
	OpNameTasksGet            = "tasks/get"
	OpNameTasksUpdate         = "tasks/update"
	OpNameTasksCancel         = "tasks/cancel"
)

type subscriptionListener interface {
	SubscriptionsListen(ctx context.Context, params *transport.SubscriptionsListenParams) (*transport.OperationOutcome, error)
}

type taskGetter interface {
	TasksGet(ctx context.Context, taskID string) (*transport.OperationOutcome, error)
}

type taskUpdater interface {
	TasksUpdate(ctx context.Context, params *transport.TasksUpdateParams) (*transport.OperationOutcome, error)
}

type taskCanceller interface {
	TasksCancel(ctx context.Context, taskID string) (*transport.OperationOutcome, error)
}

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
		Name:           name,
		Arguments:      arguments,
		InputResponses: rawMessageMap(params["input_responses"]),
	}
	if requestState, ok := params["request_state"].(string); ok {
		callParams.RequestState = requestState
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
	nameStr, isString := name.(string)
	if !isString {
		return NewValidationError(OpNameToolsCall, "name", "must be a string")
	}
	if nameStr == "" {
		return NewValidationError(OpNameToolsCall, "name", "cannot be empty")
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
		Name:           name,
		Arguments:      arguments,
		InputResponses: rawMessageMap(params["input_responses"]),
	}
	if requestState, ok := params["request_state"].(string); ok {
		getParams.RequestState = requestState
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
	nameStr, isString := name.(string)
	if !isString {
		return NewValidationError(OpNamePromptsGet, "name", "must be a string")
	}
	if nameStr == "" {
		return NewValidationError(OpNamePromptsGet, "name", "cannot be empty")
	}

	if args, ok := params["arguments"]; ok {
		if _, isMap := args.(map[string]interface{}); !isMap {
			return NewValidationError(OpNamePromptsGet, "arguments", "must be an object")
		}
	}

	return nil
}

type SubscriptionsListenOperation struct{}

func (o *SubscriptionsListenOperation) Name() string { return OpNameSubscriptionsListen }

func (o *SubscriptionsListenOperation) Execute(ctx context.Context, conn transport.Connection, params map[string]interface{}) (*transport.OperationOutcome, error) {
	listener, ok := conn.(subscriptionListener)
	if !ok {
		return nil, NewOperationError(OpNameSubscriptionsListen, "connection does not support subscriptions/listen", nil)
	}
	listenParams := &transport.SubscriptionsListenParams{}
	if params != nil {
		if notifications, ok := params["notifications"].(map[string]interface{}); ok {
			listenParams.Notifications = parseSubscriptionNotifications(notifications)
		}
	}
	return listener.SubscriptionsListen(ctx, listenParams)
}

func (o *SubscriptionsListenOperation) Validate(params map[string]interface{}) error { return nil }

type TasksGetOperation struct{}

func (o *TasksGetOperation) Name() string { return OpNameTasksGet }

func (o *TasksGetOperation) Execute(ctx context.Context, conn transport.Connection, params map[string]interface{}) (*transport.OperationOutcome, error) {
	getter, ok := conn.(taskGetter)
	if !ok {
		return nil, NewOperationError(OpNameTasksGet, "connection does not support tasks/get", nil)
	}
	taskID, ok := params["task_id"].(string)
	if !ok {
		return nil, NewOperationError(OpNameTasksGet, "task_id parameter required", nil)
	}
	return getter.TasksGet(ctx, taskID)
}

func (o *TasksGetOperation) Validate(params map[string]interface{}) error {
	return validateTaskID(OpNameTasksGet, params)
}

type TasksUpdateOperation struct{}

func (o *TasksUpdateOperation) Name() string { return OpNameTasksUpdate }

func (o *TasksUpdateOperation) Execute(ctx context.Context, conn transport.Connection, params map[string]interface{}) (*transport.OperationOutcome, error) {
	updater, ok := conn.(taskUpdater)
	if !ok {
		return nil, NewOperationError(OpNameTasksUpdate, "connection does not support tasks/update", nil)
	}
	taskID, ok := params["task_id"].(string)
	if !ok {
		return nil, NewOperationError(OpNameTasksUpdate, "task_id parameter required", nil)
	}
	inputResponses := rawMessageMap(params["input_responses"])
	if len(inputResponses) == 0 {
		return nil, NewOperationError(OpNameTasksUpdate, "input_responses parameter required", nil)
	}
	return updater.TasksUpdate(ctx, &transport.TasksUpdateParams{TaskID: taskID, InputResponses: inputResponses})
}

func (o *TasksUpdateOperation) Validate(params map[string]interface{}) error {
	if err := validateTaskID(OpNameTasksUpdate, params); err != nil {
		return err
	}
	if len(rawMessageMap(params["input_responses"])) == 0 {
		return NewOperationError(OpNameTasksUpdate, "input_responses parameter required", nil)
	}
	return nil
}

type TasksCancelOperation struct{}

func (o *TasksCancelOperation) Name() string { return OpNameTasksCancel }

func (o *TasksCancelOperation) Execute(ctx context.Context, conn transport.Connection, params map[string]interface{}) (*transport.OperationOutcome, error) {
	canceller, ok := conn.(taskCanceller)
	if !ok {
		return nil, NewOperationError(OpNameTasksCancel, "connection does not support tasks/cancel", nil)
	}
	taskID, ok := params["task_id"].(string)
	if !ok {
		return nil, NewOperationError(OpNameTasksCancel, "task_id parameter required", nil)
	}
	return canceller.TasksCancel(ctx, taskID)
}

func (o *TasksCancelOperation) Validate(params map[string]interface{}) error {
	return validateTaskID(OpNameTasksCancel, params)
}

func validateTaskID(opName string, params map[string]interface{}) error {
	if params == nil {
		return NewValidationError(opName, "", "params required")
	}
	taskID, ok := params["task_id"].(string)
	if !ok || taskID == "" {
		return NewValidationError(opName, "task_id", "required")
	}
	return nil
}

func rawMessageMap(value interface{}) map[string]json.RawMessage {
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

func parseSubscriptionNotifications(m map[string]interface{}) transport.SubscriptionNotifications {
	return transport.SubscriptionNotifications{
		ToolsListChanged:      boolValue(m["toolsListChanged"]),
		PromptsListChanged:    boolValue(m["promptsListChanged"]),
		ResourcesListChanged:  boolValue(m["resourcesListChanged"]),
		ResourceSubscriptions: stringSlice(m["resourceSubscriptions"]),
		TaskIDs:               stringSlice(m["taskIds"]),
	}
}

func boolValue(v interface{}) bool {
	b, _ := v.(bool)
	return b
}

func stringSlice(v interface{}) []string {
	items, ok := v.([]interface{})
	if !ok {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}
