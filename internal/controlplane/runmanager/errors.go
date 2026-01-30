package runmanager

import (
	"errors"
	"fmt"
)

// RunManagerError is a typed error that can be inspected for proper HTTP mapping.
type RunManagerError struct {
	Kind    ErrorKind
	RunID   string
	State   RunState
	Message string
	Cause   error
}

// ErrorKind categorizes the error for HTTP status mapping.
type ErrorKind int

const (
	ErrKindNotFound ErrorKind = iota
	ErrKindInvalidState
	ErrKindTerminalState
	ErrKindInvalidTransition
	ErrKindInternal
)

func (e *RunManagerError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

func (e *RunManagerError) Unwrap() error {
	return e.Cause
}

// NewNotFoundError creates a not-found error.
func NewNotFoundError(runID string) *RunManagerError {
	return &RunManagerError{
		Kind:    ErrKindNotFound,
		RunID:   runID,
		Message: fmt.Sprintf("run not found: %s", runID),
	}
}

// NewInvalidStateError creates an invalid-state error.
func NewInvalidStateError(runID string, currentState, expectedState RunState, operation string) *RunManagerError {
	return &RunManagerError{
		Kind:    ErrKindInvalidState,
		RunID:   runID,
		State:   currentState,
		Message: fmt.Sprintf("cannot %s run in state %s: expected %s", operation, currentState, expectedState),
	}
}

// NewTerminalStateError creates a terminal-state error.
func NewTerminalStateError(runID string, state RunState, operation string) *RunManagerError {
	return &RunManagerError{
		Kind:    ErrKindTerminalState,
		RunID:   runID,
		State:   state,
		Message: fmt.Sprintf("cannot %s run in terminal state %s", operation, state),
	}
}

// NewInvalidTransitionError creates an invalid-transition error.
func NewInvalidTransitionError(runID string, from, to RunState) *RunManagerError {
	return &RunManagerError{
		Kind:    ErrKindInvalidTransition,
		RunID:   runID,
		State:   from,
		Message: fmt.Sprintf("invalid state transition from %s to %s", from, to),
	}
}

// NewInternalError wraps an internal error.
func NewInternalError(runID string, cause error) *RunManagerError {
	return &RunManagerError{
		Kind:    ErrKindInternal,
		RunID:   runID,
		Message: cause.Error(),
		Cause:   cause,
	}
}

// AsRunManagerError attempts to convert an error to a RunManagerError.
// Returns nil if not possible.
func AsRunManagerError(err error) *RunManagerError {
	var rmErr *RunManagerError
	if errors.As(err, &rmErr) {
		return rmErr
	}
	return nil
}

// IsNotFound checks if the error is a not-found error.
func IsNotFound(err error) bool {
	rmErr := AsRunManagerError(err)
	return rmErr != nil && rmErr.Kind == ErrKindNotFound
}

// IsTerminalState checks if the error is a terminal-state error.
func IsTerminalState(err error) bool {
	rmErr := AsRunManagerError(err)
	return rmErr != nil && rmErr.Kind == ErrKindTerminalState
}

// IsInvalidState checks if the error is an invalid-state error.
func IsInvalidState(err error) bool {
	rmErr := AsRunManagerError(err)
	return rmErr != nil && (rmErr.Kind == ErrKindInvalidState || rmErr.Kind == ErrKindInvalidTransition)
}
