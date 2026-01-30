// Package plugin provides a plugin architecture for extensible MCP operations.
package plugin

import (
	"context"
	"fmt"

	"github.com/bc-dunia/mcpdrill/internal/transport"
)

// Operation defines the interface for MCP operations that can be executed by the VU engine.
type Operation interface {
	// Name returns the operation name (e.g., "tools/list", "tools/call", "ping").
	Name() string

	// Execute performs the operation using the provided connection and parameters.
	Execute(ctx context.Context, conn transport.Connection, params map[string]interface{}) (*transport.OperationOutcome, error)

	// Validate validates the parameters before execution.
	// Returns nil if parameters are valid, or an error describing the validation failure.
	Validate(params map[string]interface{}) error
}

// OperationFunc is a helper type that allows creating operations from functions.
// This is useful for simple operations that don't need a full struct.
type OperationFunc struct {
	name     string
	validate func(params map[string]interface{}) error
	execute  func(ctx context.Context, conn transport.Connection, params map[string]interface{}) (*transport.OperationOutcome, error)
}

// NewOperationFunc creates a new function-based operation.
func NewOperationFunc(
	name string,
	validate func(params map[string]interface{}) error,
	execute func(ctx context.Context, conn transport.Connection, params map[string]interface{}) (*transport.OperationOutcome, error),
) *OperationFunc {
	return &OperationFunc{
		name:     name,
		validate: validate,
		execute:  execute,
	}
}

// Name returns the operation name.
func (f *OperationFunc) Name() string {
	return f.name
}

// Execute performs the operation.
func (f *OperationFunc) Execute(ctx context.Context, conn transport.Connection, params map[string]interface{}) (*transport.OperationOutcome, error) {
	if f.execute == nil {
		return nil, fmt.Errorf("operation %s: execute function not defined", f.name)
	}
	return f.execute(ctx, conn, params)
}

// Validate validates the parameters.
func (f *OperationFunc) Validate(params map[string]interface{}) error {
	if f.validate == nil {
		return nil // No validation required
	}
	return f.validate(params)
}

// OperationError represents an error from an operation.
type OperationError struct {
	Operation string
	Message   string
	Err       error
}

func (e *OperationError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("operation %s: %s: %v", e.Operation, e.Message, e.Err)
	}
	return fmt.Sprintf("operation %s: %s", e.Operation, e.Message)
}

func (e *OperationError) Unwrap() error {
	return e.Err
}

// NewOperationError creates a new operation error.
func NewOperationError(operation, message string, err error) *OperationError {
	return &OperationError{
		Operation: operation,
		Message:   message,
		Err:       err,
	}
}

// ValidationError represents a parameter validation error.
type ValidationError struct {
	Operation string
	Param     string
	Message   string
}

func (e *ValidationError) Error() string {
	if e.Param != "" {
		return fmt.Sprintf("operation %s: invalid parameter %q: %s", e.Operation, e.Param, e.Message)
	}
	return fmt.Sprintf("operation %s: validation failed: %s", e.Operation, e.Message)
}

// NewValidationError creates a new validation error.
func NewValidationError(operation, param, message string) *ValidationError {
	return &ValidationError{
		Operation: operation,
		Param:     param,
		Message:   message,
	}
}

// RegistrationError represents an error during operation registration.
type RegistrationError struct {
	Operation string
	Message   string
}

func (e *RegistrationError) Error() string {
	return fmt.Sprintf("registration failed for operation %q: %s", e.Operation, e.Message)
}

// NewRegistrationError creates a new registration error.
func NewRegistrationError(operation, message string) *RegistrationError {
	return &RegistrationError{
		Operation: operation,
		Message:   message,
	}
}
