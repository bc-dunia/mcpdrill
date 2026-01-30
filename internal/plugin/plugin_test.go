package plugin

import (
	"context"
	"errors"
	"testing"

	"github.com/bc-dunia/mcpdrill/internal/transport"
)

type mockConnection struct {
	toolsListCalled bool
	toolsCallCalled bool
	pingCalled      bool
	toolsCallParams *transport.ToolsCallParams
}

func (m *mockConnection) Initialize(ctx context.Context, params *transport.InitializeParams) (*transport.OperationOutcome, error) {
	return &transport.OperationOutcome{OK: true}, nil
}

func (m *mockConnection) SendInitialized(ctx context.Context) (*transport.OperationOutcome, error) {
	return &transport.OperationOutcome{OK: true}, nil
}

func (m *mockConnection) ToolsList(ctx context.Context, cursor *string) (*transport.OperationOutcome, error) {
	m.toolsListCalled = true
	return &transport.OperationOutcome{OK: true}, nil
}

func (m *mockConnection) ToolsCall(ctx context.Context, params *transport.ToolsCallParams) (*transport.OperationOutcome, error) {
	m.toolsCallCalled = true
	m.toolsCallParams = params
	return &transport.OperationOutcome{OK: true}, nil
}

func (m *mockConnection) Ping(ctx context.Context) (*transport.OperationOutcome, error) {
	m.pingCalled = true
	return &transport.OperationOutcome{OK: true}, nil
}

func (m *mockConnection) ResourcesList(ctx context.Context, cursor *string) (*transport.OperationOutcome, error) {
	return &transport.OperationOutcome{OK: true}, nil
}

func (m *mockConnection) ResourcesRead(ctx context.Context, params *transport.ResourcesReadParams) (*transport.OperationOutcome, error) {
	return &transport.OperationOutcome{OK: true}, nil
}

func (m *mockConnection) PromptsList(ctx context.Context, cursor *string) (*transport.OperationOutcome, error) {
	return &transport.OperationOutcome{OK: true}, nil
}

func (m *mockConnection) PromptsGet(ctx context.Context, params *transport.PromptsGetParams) (*transport.OperationOutcome, error) {
	return &transport.OperationOutcome{OK: true}, nil
}

func (m *mockConnection) Close() error                  { return nil }
func (m *mockConnection) SessionID() string             { return "test-session" }
func (m *mockConnection) SetSessionID(sessionID string) {}
func (m *mockConnection) SetLastEventID(eventID string) {}

func TestRegistry_Register(t *testing.T) {
	r := NewRegistry()

	op := &ToolsListOperation{}
	err := r.Register(op)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if r.Count() != 1 {
		t.Errorf("expected count 1, got %d", r.Count())
	}
}

func TestRegistry_RegisterDuplicate(t *testing.T) {
	r := NewRegistry()

	op := &ToolsListOperation{}
	_ = r.Register(op)

	err := r.Register(op)
	if err == nil {
		t.Fatal("expected error for duplicate registration")
	}

	var regErr *RegistrationError
	if !errors.As(err, &regErr) {
		t.Errorf("expected RegistrationError, got %T", err)
	}
}

func TestRegistry_RegisterNil(t *testing.T) {
	r := NewRegistry()

	err := r.Register(nil)
	if err == nil {
		t.Fatal("expected error for nil operation")
	}
}

func TestRegistry_Get(t *testing.T) {
	r := NewRegistry()

	op := &ToolsListOperation{}
	_ = r.Register(op)

	got, found := r.Get(OpNameToolsList)
	if !found {
		t.Fatal("expected to find operation")
	}
	if got.Name() != OpNameToolsList {
		t.Errorf("expected name %s, got %s", OpNameToolsList, got.Name())
	}
}

func TestRegistry_GetNotFound(t *testing.T) {
	r := NewRegistry()

	_, found := r.Get("nonexistent")
	if found {
		t.Error("expected not to find operation")
	}
}

func TestRegistry_List(t *testing.T) {
	r := NewRegistry()

	_ = r.Register(&ToolsListOperation{})
	_ = r.Register(&PingOperation{})

	names := r.List()
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(names))
	}

	if names[0] != OpNamePing || names[1] != OpNameToolsList {
		t.Errorf("expected sorted names [ping, tools/list], got %v", names)
	}
}

func TestRegistry_Unregister(t *testing.T) {
	r := NewRegistry()

	_ = r.Register(&ToolsListOperation{})

	removed := r.Unregister(OpNameToolsList)
	if !removed {
		t.Error("expected operation to be removed")
	}

	if r.Count() != 0 {
		t.Errorf("expected count 0, got %d", r.Count())
	}
}

func TestRegistry_UnregisterNotFound(t *testing.T) {
	r := NewRegistry()

	removed := r.Unregister("nonexistent")
	if removed {
		t.Error("expected false for nonexistent operation")
	}
}

func TestRegistry_MustRegister(t *testing.T) {
	r := NewRegistry()

	defer func() {
		if recover() != nil {
			t.Error("unexpected panic")
		}
	}()

	r.MustRegister(&ToolsListOperation{})
}

func TestRegistry_MustRegisterPanics(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&ToolsListOperation{})

	defer func() {
		if recover() == nil {
			t.Error("expected panic for duplicate registration")
		}
	}()

	r.MustRegister(&ToolsListOperation{})
}

func TestToolsListOperation_Execute(t *testing.T) {
	op := &ToolsListOperation{}
	conn := &mockConnection{}

	outcome, err := op.Execute(context.Background(), conn, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !outcome.OK {
		t.Error("expected OK outcome")
	}
	if !conn.toolsListCalled {
		t.Error("expected ToolsList to be called")
	}
}

func TestToolsListOperation_Validate(t *testing.T) {
	op := &ToolsListOperation{}

	if err := op.Validate(nil); err != nil {
		t.Errorf("expected no error for nil params, got %v", err)
	}

	if err := op.Validate(map[string]interface{}{"cursor": "abc"}); err != nil {
		t.Errorf("expected no error for valid cursor, got %v", err)
	}

	if err := op.Validate(map[string]interface{}{"cursor": 123}); err == nil {
		t.Error("expected error for invalid cursor type")
	}
}

func TestToolsCallOperation_Execute(t *testing.T) {
	op := &ToolsCallOperation{}
	conn := &mockConnection{}

	params := map[string]interface{}{
		"name":      "test-tool",
		"arguments": map[string]interface{}{"arg1": "value1"},
	}

	outcome, err := op.Execute(context.Background(), conn, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !outcome.OK {
		t.Error("expected OK outcome")
	}
	if !conn.toolsCallCalled {
		t.Error("expected ToolsCall to be called")
	}
	if conn.toolsCallParams.Name != "test-tool" {
		t.Errorf("expected tool name 'test-tool', got %s", conn.toolsCallParams.Name)
	}
}

func TestToolsCallOperation_Validate(t *testing.T) {
	op := &ToolsCallOperation{}

	if err := op.Validate(nil); err == nil {
		t.Error("expected error for nil params")
	}

	if err := op.Validate(map[string]interface{}{}); err == nil {
		t.Error("expected error for missing name")
	}

	if err := op.Validate(map[string]interface{}{"name": 123}); err == nil {
		t.Error("expected error for invalid name type")
	}

	if err := op.Validate(map[string]interface{}{"name": "tool", "arguments": "invalid"}); err == nil {
		t.Error("expected error for invalid arguments type")
	}

	if err := op.Validate(map[string]interface{}{"name": "tool"}); err != nil {
		t.Errorf("expected no error for valid params, got %v", err)
	}
}

func TestPingOperation_Execute(t *testing.T) {
	op := &PingOperation{}
	conn := &mockConnection{}

	outcome, err := op.Execute(context.Background(), conn, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !outcome.OK {
		t.Error("expected OK outcome")
	}
	if !conn.pingCalled {
		t.Error("expected Ping to be called")
	}
}

func TestPingOperation_Validate(t *testing.T) {
	op := &PingOperation{}

	if err := op.Validate(nil); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestOperationFunc(t *testing.T) {
	executeCalled := false
	validateCalled := false

	op := NewOperationFunc(
		"custom/op",
		func(params map[string]interface{}) error {
			validateCalled = true
			return nil
		},
		func(ctx context.Context, conn transport.Connection, params map[string]interface{}) (*transport.OperationOutcome, error) {
			executeCalled = true
			return &transport.OperationOutcome{OK: true}, nil
		},
	)

	if op.Name() != "custom/op" {
		t.Errorf("expected name 'custom/op', got %s", op.Name())
	}

	if err := op.Validate(nil); err != nil {
		t.Errorf("unexpected validation error: %v", err)
	}
	if !validateCalled {
		t.Error("expected validate to be called")
	}

	conn := &mockConnection{}
	outcome, err := op.Execute(context.Background(), conn, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !outcome.OK {
		t.Error("expected OK outcome")
	}
	if !executeCalled {
		t.Error("expected execute to be called")
	}
}

func TestOperationFunc_NilValidate(t *testing.T) {
	op := NewOperationFunc(
		"custom/op",
		nil,
		func(ctx context.Context, conn transport.Connection, params map[string]interface{}) (*transport.OperationOutcome, error) {
			return &transport.OperationOutcome{OK: true}, nil
		},
	)

	if err := op.Validate(nil); err != nil {
		t.Errorf("expected no error for nil validate func, got %v", err)
	}
}

func TestOperationFunc_NilExecute(t *testing.T) {
	op := NewOperationFunc("custom/op", nil, nil)

	conn := &mockConnection{}
	_, err := op.Execute(context.Background(), conn, nil)
	if err == nil {
		t.Error("expected error for nil execute func")
	}
}

func TestDefaultRegistry_BuiltinOperations(t *testing.T) {
	names := List()

	expectedOps := []string{OpNamePing, OpNameToolsCall, OpNameToolsList}
	if len(names) < len(expectedOps) {
		t.Fatalf("expected at least %d operations, got %d", len(expectedOps), len(names))
	}

	for _, expected := range expectedOps {
		op, found := Get(expected)
		if !found {
			t.Errorf("expected to find builtin operation %s", expected)
			continue
		}
		if op.Name() != expected {
			t.Errorf("expected name %s, got %s", expected, op.Name())
		}
	}
}

func TestOperationError(t *testing.T) {
	err := NewOperationError("test/op", "something failed", errors.New("underlying"))
	if err.Error() != "operation test/op: something failed: underlying" {
		t.Errorf("unexpected error message: %s", err.Error())
	}

	if err.Unwrap() == nil {
		t.Error("expected unwrap to return underlying error")
	}

	errNoUnderlying := NewOperationError("test/op", "something failed", nil)
	if errNoUnderlying.Error() != "operation test/op: something failed" {
		t.Errorf("unexpected error message: %s", errNoUnderlying.Error())
	}
}

func TestValidationError(t *testing.T) {
	err := NewValidationError("test/op", "param1", "must be string")
	if err.Error() != `operation test/op: invalid parameter "param1": must be string` {
		t.Errorf("unexpected error message: %s", err.Error())
	}

	errNoParam := NewValidationError("test/op", "", "general failure")
	if errNoParam.Error() != "operation test/op: validation failed: general failure" {
		t.Errorf("unexpected error message: %s", errNoParam.Error())
	}
}

func TestRegistrationError(t *testing.T) {
	err := NewRegistrationError("test/op", "already exists")
	if err.Error() != `registration failed for operation "test/op": already exists` {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}
