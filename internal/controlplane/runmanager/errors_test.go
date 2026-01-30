package runmanager

import (
	"errors"
	"testing"
)

func TestNewNotFoundError(t *testing.T) {
	err := NewNotFoundError("run_123")

	if err.Kind != ErrKindNotFound {
		t.Errorf("Expected ErrKindNotFound, got %v", err.Kind)
	}
	if err.RunID != "run_123" {
		t.Errorf("Expected run_123, got %s", err.RunID)
	}
	if err.Error() != "run not found: run_123" {
		t.Errorf("Unexpected error message: %s", err.Error())
	}
}

func TestNewTerminalStateError(t *testing.T) {
	err := NewTerminalStateError("run_123", RunStateCompleted, "stop")

	if err.Kind != ErrKindTerminalState {
		t.Errorf("Expected ErrKindTerminalState, got %v", err.Kind)
	}
	if err.State != RunStateCompleted {
		t.Errorf("Expected RunStateCompleted, got %v", err.State)
	}
}

func TestNewInvalidStateError(t *testing.T) {
	err := NewInvalidStateError("run_123", RunStateAnalyzing, RunStateCreated, "start")

	if err.Kind != ErrKindInvalidState {
		t.Errorf("Expected ErrKindInvalidState, got %v", err.Kind)
	}
	if err.State != RunStateAnalyzing {
		t.Errorf("Expected RunStateAnalyzing, got %v", err.State)
	}
}

func TestNewInvalidTransitionError(t *testing.T) {
	err := NewInvalidTransitionError("run_123", RunStateCreated, RunStateAnalyzing)

	if err.Kind != ErrKindInvalidTransition {
		t.Errorf("Expected ErrKindInvalidTransition, got %v", err.Kind)
	}
}

func TestAsRunManagerError(t *testing.T) {
	// Test with RunManagerError
	rmErr := NewNotFoundError("run_123")
	result := AsRunManagerError(rmErr)
	if result == nil {
		t.Error("Expected non-nil result for RunManagerError")
	}
	if result.Kind != ErrKindNotFound {
		t.Errorf("Expected ErrKindNotFound, got %v", result.Kind)
	}

	// Test with wrapped error
	wrapped := errors.New("wrapper: " + rmErr.Error())
	result = AsRunManagerError(wrapped)
	if result != nil {
		t.Error("Expected nil result for non-RunManagerError")
	}

	// Test with nil
	result = AsRunManagerError(nil)
	if result != nil {
		t.Error("Expected nil result for nil error")
	}
}

func TestIsNotFound(t *testing.T) {
	notFoundErr := NewNotFoundError("run_123")
	if !IsNotFound(notFoundErr) {
		t.Error("Expected IsNotFound to return true for not found error")
	}

	terminalErr := NewTerminalStateError("run_123", RunStateCompleted, "stop")
	if IsNotFound(terminalErr) {
		t.Error("Expected IsNotFound to return false for terminal state error")
	}

	regularErr := errors.New("some error")
	if IsNotFound(regularErr) {
		t.Error("Expected IsNotFound to return false for regular error")
	}
}

func TestIsTerminalState(t *testing.T) {
	terminalErr := NewTerminalStateError("run_123", RunStateCompleted, "stop")
	if !IsTerminalState(terminalErr) {
		t.Error("Expected IsTerminalState to return true for terminal state error")
	}

	notFoundErr := NewNotFoundError("run_123")
	if IsTerminalState(notFoundErr) {
		t.Error("Expected IsTerminalState to return false for not found error")
	}
}

func TestIsInvalidState(t *testing.T) {
	invalidStateErr := NewInvalidStateError("run_123", RunStateAnalyzing, RunStateCreated, "start")
	if !IsInvalidState(invalidStateErr) {
		t.Error("Expected IsInvalidState to return true for invalid state error")
	}

	transitionErr := NewInvalidTransitionError("run_123", RunStateCreated, RunStateAnalyzing)
	if !IsInvalidState(transitionErr) {
		t.Error("Expected IsInvalidState to return true for invalid transition error")
	}

	notFoundErr := NewNotFoundError("run_123")
	if IsInvalidState(notFoundErr) {
		t.Error("Expected IsInvalidState to return false for not found error")
	}
}

func TestRunManagerError_Unwrap(t *testing.T) {
	cause := errors.New("underlying error")
	err := NewInternalError("run_123", cause)

	unwrapped := errors.Unwrap(err)
	if unwrapped != cause {
		t.Error("Expected Unwrap to return the cause")
	}

	// Error without cause
	err2 := NewNotFoundError("run_123")
	unwrapped2 := errors.Unwrap(err2)
	if unwrapped2 != nil {
		t.Error("Expected Unwrap to return nil when no cause")
	}
}
