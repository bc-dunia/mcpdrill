// Package transport provides MCP transport adapters for mcpdrill.
package transport

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// SchemaType represents JSON schema types.
type SchemaType string

const (
	TypeString  SchemaType = "string"
	TypeNumber  SchemaType = "number"
	TypeInteger SchemaType = "integer"
	TypeBoolean SchemaType = "boolean"
	TypeObject  SchemaType = "object"
	TypeArray   SchemaType = "array"
	TypeNull    SchemaType = "null"
)

// PropertySchema defines validation rules for a single property.
type PropertySchema struct {
	Type SchemaType `json:"type"`

	// String validation
	MinLength *int    `json:"minLength,omitempty"`
	MaxLength *int    `json:"maxLength,omitempty"`
	Pattern   *string `json:"pattern,omitempty"`

	// Number validation
	Minimum *float64 `json:"minimum,omitempty"`
	Maximum *float64 `json:"maximum,omitempty"`

	// Array validation
	MinItems *int            `json:"minItems,omitempty"`
	MaxItems *int            `json:"maxItems,omitempty"`
	Items    *PropertySchema `json:"items,omitempty"`

	// Object validation (nested)
	Properties map[string]PropertySchema `json:"properties,omitempty"`
	Required   []string                  `json:"required,omitempty"`
}

// ArgumentSchema defines the schema for validating tool arguments.
type ArgumentSchema struct {
	Type       SchemaType                `json:"type"`
	Properties map[string]PropertySchema `json:"properties,omitempty"`
	Required   []string                  `json:"required,omitempty"`
}

// ValidationError contains details about a validation failure.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Value   any    `json:"value,omitempty"`
}

func (e *ValidationError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("%s: %s", e.Field, e.Message)
	}
	return e.Message
}

// ValidationResult contains all validation errors.
type ValidationResult struct {
	Valid  bool               `json:"valid"`
	Errors []*ValidationError `json:"errors,omitempty"`
}

var patternCache sync.Map

func getCompiledPattern(pattern string) (*regexp.Regexp, error) {
	if cached, ok := patternCache.Load(pattern); ok {
		return cached.(*regexp.Regexp), nil
	}
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	patternCache.Store(pattern, compiled)
	return compiled, nil
}

// Error returns a combined error message.
func (r *ValidationResult) Error() string {
	if r.Valid {
		return ""
	}
	var msgs []string
	for _, e := range r.Errors {
		msgs = append(msgs, e.Error())
	}
	return strings.Join(msgs, "; ")
}

// ArgumentValidator validates tool arguments against a schema.
type ArgumentValidator struct {
	schema *ArgumentSchema
}

// NewArgumentValidator creates a new validator with the given schema.
func NewArgumentValidator(schema *ArgumentSchema) *ArgumentValidator {
	return &ArgumentValidator{schema: schema}
}

// Validate checks if the arguments match the schema.
func (v *ArgumentValidator) Validate(args map[string]any) *ValidationResult {
	result := &ValidationResult{Valid: true}

	if v.schema == nil {
		return result
	}

	// Validate required fields
	for _, required := range v.schema.Required {
		if _, exists := args[required]; !exists {
			result.Valid = false
			result.Errors = append(result.Errors, &ValidationError{
				Field:   required,
				Message: "required field is missing",
			})
		}
	}

	// Validate each property
	for name, propSchema := range v.schema.Properties {
		value, exists := args[name]
		if !exists {
			continue // Already checked required fields
		}
		v.validateProperty(name, value, &propSchema, result)
	}

	return result
}

func (v *ArgumentValidator) validateProperty(field string, value any, schema *PropertySchema, result *ValidationResult) {
	if value == nil {
		if schema.Type != TypeNull {
			result.Valid = false
			result.Errors = append(result.Errors, &ValidationError{
				Field:   field,
				Message: fmt.Sprintf("expected type %s, got null", schema.Type),
				Value:   value,
			})
		}
		return
	}

	switch schema.Type {
	case TypeString:
		v.validateString(field, value, schema, result)
	case TypeNumber, TypeInteger:
		v.validateNumber(field, value, schema, result)
	case TypeBoolean:
		v.validateBoolean(field, value, schema, result)
	case TypeArray:
		v.validateArray(field, value, schema, result)
	case TypeObject:
		v.validateObject(field, value, schema, result)
	case TypeNull:
		if value != nil {
			result.Valid = false
			result.Errors = append(result.Errors, &ValidationError{
				Field:   field,
				Message: "expected null value",
				Value:   value,
			})
		}
	}
}

func (v *ArgumentValidator) validateString(field string, value any, schema *PropertySchema, result *ValidationResult) {
	str, ok := value.(string)
	if !ok {
		result.Valid = false
		result.Errors = append(result.Errors, &ValidationError{
			Field:   field,
			Message: fmt.Sprintf("expected string, got %T", value),
			Value:   value,
		})
		return
	}

	// Check minLength
	if schema.MinLength != nil && len(str) < *schema.MinLength {
		result.Valid = false
		result.Errors = append(result.Errors, &ValidationError{
			Field:   field,
			Message: fmt.Sprintf("string length %d is less than minimum %d", len(str), *schema.MinLength),
			Value:   value,
		})
	}

	// Check maxLength
	if schema.MaxLength != nil && len(str) > *schema.MaxLength {
		result.Valid = false
		result.Errors = append(result.Errors, &ValidationError{
			Field:   field,
			Message: fmt.Sprintf("string length %d exceeds maximum %d", len(str), *schema.MaxLength),
			Value:   value,
		})
	}

	// Check pattern
	if schema.Pattern != nil {
		re, err := getCompiledPattern(*schema.Pattern)
		if err != nil {
			result.Valid = false
			result.Errors = append(result.Errors, &ValidationError{
				Field:   field,
				Message: fmt.Sprintf("invalid regex pattern: %v", err),
			})
		} else if !re.MatchString(str) {
			result.Valid = false
			result.Errors = append(result.Errors, &ValidationError{
				Field:   field,
				Message: fmt.Sprintf("string does not match pattern %s", *schema.Pattern),
				Value:   value,
			})
		}
	}
}

func (v *ArgumentValidator) validateNumber(field string, value any, schema *PropertySchema, result *ValidationResult) {
	var num float64
	switch n := value.(type) {
	case float64:
		num = n
	case float32:
		num = float64(n)
	case int:
		num = float64(n)
	case int64:
		num = float64(n)
	case int32:
		num = float64(n)
	case json.Number:
		f, err := n.Float64()
		if err != nil {
			result.Valid = false
			result.Errors = append(result.Errors, &ValidationError{
				Field:   field,
				Message: fmt.Sprintf("invalid number: %v", err),
				Value:   value,
			})
			return
		}
		num = f
	default:
		result.Valid = false
		result.Errors = append(result.Errors, &ValidationError{
			Field:   field,
			Message: fmt.Sprintf("expected number, got %T", value),
			Value:   value,
		})
		return
	}

	// For integer type, check if the number is a whole number
	if schema.Type == TypeInteger {
		if num != float64(int64(num)) {
			result.Valid = false
			result.Errors = append(result.Errors, &ValidationError{
				Field:   field,
				Message: fmt.Sprintf("expected integer, got %v", num),
				Value:   value,
			})
			return
		}
	}

	// Check minimum
	if schema.Minimum != nil && num < *schema.Minimum {
		result.Valid = false
		result.Errors = append(result.Errors, &ValidationError{
			Field:   field,
			Message: fmt.Sprintf("value %v is less than minimum %v", num, *schema.Minimum),
			Value:   value,
		})
	}

	// Check maximum
	if schema.Maximum != nil && num > *schema.Maximum {
		result.Valid = false
		result.Errors = append(result.Errors, &ValidationError{
			Field:   field,
			Message: fmt.Sprintf("value %v exceeds maximum %v", num, *schema.Maximum),
			Value:   value,
		})
	}
}

func (v *ArgumentValidator) validateBoolean(field string, value any, schema *PropertySchema, result *ValidationResult) {
	_, ok := value.(bool)
	if !ok {
		result.Valid = false
		result.Errors = append(result.Errors, &ValidationError{
			Field:   field,
			Message: fmt.Sprintf("expected boolean, got %T", value),
			Value:   value,
		})
	}
}

func (v *ArgumentValidator) validateArray(field string, value any, schema *PropertySchema, result *ValidationResult) {
	arr, ok := value.([]any)
	if !ok {
		result.Valid = false
		result.Errors = append(result.Errors, &ValidationError{
			Field:   field,
			Message: fmt.Sprintf("expected array, got %T", value),
			Value:   value,
		})
		return
	}

	// Check minItems
	if schema.MinItems != nil && len(arr) < *schema.MinItems {
		result.Valid = false
		result.Errors = append(result.Errors, &ValidationError{
			Field:   field,
			Message: fmt.Sprintf("array length %d is less than minimum %d", len(arr), *schema.MinItems),
			Value:   value,
		})
	}

	// Check maxItems
	if schema.MaxItems != nil && len(arr) > *schema.MaxItems {
		result.Valid = false
		result.Errors = append(result.Errors, &ValidationError{
			Field:   field,
			Message: fmt.Sprintf("array length %d exceeds maximum %d", len(arr), *schema.MaxItems),
			Value:   value,
		})
	}

	// Validate items
	if schema.Items != nil {
		for i, item := range arr {
			itemField := fmt.Sprintf("%s[%d]", field, i)
			v.validateProperty(itemField, item, schema.Items, result)
		}
	}
}

func (v *ArgumentValidator) validateObject(field string, value any, schema *PropertySchema, result *ValidationResult) {
	obj, ok := value.(map[string]any)
	if !ok {
		result.Valid = false
		result.Errors = append(result.Errors, &ValidationError{
			Field:   field,
			Message: fmt.Sprintf("expected object, got %T", value),
			Value:   value,
		})
		return
	}

	// Check required fields in nested object
	for _, required := range schema.Required {
		if _, exists := obj[required]; !exists {
			result.Valid = false
			result.Errors = append(result.Errors, &ValidationError{
				Field:   fmt.Sprintf("%s.%s", field, required),
				Message: "required field is missing",
			})
		}
	}

	// Validate nested properties
	for name, propSchema := range schema.Properties {
		nestedValue, exists := obj[name]
		if !exists {
			continue
		}
		nestedField := fmt.Sprintf("%s.%s", field, name)
		ps := propSchema // Create local copy for pointer
		v.validateProperty(nestedField, nestedValue, &ps, result)
	}
}

// ValidateArguments is a convenience function to validate arguments against a schema.
func ValidateArguments(args map[string]any, schema *ArgumentSchema) *ValidationResult {
	validator := NewArgumentValidator(schema)
	return validator.Validate(args)
}

// ValidateArgumentsFromJSON validates arguments from JSON schema definition.
func ValidateArgumentsFromJSON(args map[string]any, schemaJSON []byte) (*ValidationResult, error) {
	var schema ArgumentSchema
	if err := json.Unmarshal(schemaJSON, &schema); err != nil {
		return nil, fmt.Errorf("invalid schema JSON: %w", err)
	}
	return ValidateArguments(args, &schema), nil
}

// CalculateArgumentSize returns the JSON-encoded size of arguments in bytes.
func CalculateArgumentSize(args map[string]any) int {
	if args == nil {
		return 0
	}
	data, err := json.Marshal(args)
	if err != nil {
		return 0
	}
	return len(data)
}

// MaxArgumentSize is the default maximum argument payload size (10MB).
const MaxArgumentSize = 10 * 1024 * 1024

// ValidateArgumentSize checks if arguments exceed the size limit.
func ValidateArgumentSize(args map[string]any, maxSize int) *ValidationError {
	size := CalculateArgumentSize(args)
	if size > maxSize {
		return &ValidationError{
			Field:   "",
			Message: fmt.Sprintf("argument payload size %d bytes exceeds maximum %d bytes", size, maxSize),
			Value:   size,
		}
	}
	return nil
}
