package transport

import (
	"encoding/json"
	"strings"
	"testing"
)

func intPtr(i int) *int           { return &i }
func floatPtr(f float64) *float64 { return &f }
func strPtr(s string) *string     { return &s }

func TestArgumentValidation(t *testing.T) {
	tests := []struct {
		name    string
		schema  *ArgumentSchema
		args    map[string]any
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil schema accepts anything",
			schema:  nil,
			args:    map[string]any{"foo": "bar"},
			wantErr: false,
		},
		{
			name: "valid string",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"name": {Type: TypeString},
				},
			},
			args:    map[string]any{"name": "Alice"},
			wantErr: false,
		},
		{
			name: "invalid string type",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"name": {Type: TypeString},
				},
			},
			args:    map[string]any{"name": 123},
			wantErr: true,
			errMsg:  "expected string",
		},
		{
			name: "valid number",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"count": {Type: TypeNumber},
				},
			},
			args:    map[string]any{"count": 42.5},
			wantErr: false,
		},
		{
			name: "valid integer",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"count": {Type: TypeInteger},
				},
			},
			args:    map[string]any{"count": float64(42)},
			wantErr: false,
		},
		{
			name: "invalid integer (has decimal)",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"count": {Type: TypeInteger},
				},
			},
			args:    map[string]any{"count": 42.5},
			wantErr: true,
			errMsg:  "expected integer",
		},
		{
			name: "valid boolean",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"enabled": {Type: TypeBoolean},
				},
			},
			args:    map[string]any{"enabled": true},
			wantErr: false,
		},
		{
			name: "invalid boolean type",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"enabled": {Type: TypeBoolean},
				},
			},
			args:    map[string]any{"enabled": "true"},
			wantErr: true,
			errMsg:  "expected boolean",
		},
		{
			name: "valid array",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"items": {Type: TypeArray},
				},
			},
			args:    map[string]any{"items": []any{1, 2, 3}},
			wantErr: false,
		},
		{
			name: "valid object",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"config": {Type: TypeObject},
				},
			},
			args:    map[string]any{"config": map[string]any{"key": "value"}},
			wantErr: false,
		},
		{
			name: "required field missing",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"name": {Type: TypeString},
				},
				Required: []string{"name"},
			},
			args:    map[string]any{},
			wantErr: true,
			errMsg:  "required field is missing",
		},
		{
			name: "required field present",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"name": {Type: TypeString},
				},
				Required: []string{"name"},
			},
			args:    map[string]any{"name": "test"},
			wantErr: false,
		},
		{
			name: "null value when type is null",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"value": {Type: TypeNull},
				},
			},
			args:    map[string]any{"value": nil},
			wantErr: false,
		},
		{
			name: "non-null value when type is null",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"value": {Type: TypeNull},
				},
			},
			args:    map[string]any{"value": "not null"},
			wantErr: true,
			errMsg:  "expected null",
		},
		{
			name: "null value when type is not null",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"name": {Type: TypeString},
				},
			},
			args:    map[string]any{"name": nil},
			wantErr: true,
			errMsg:  "expected type string, got null",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateArguments(tt.args, tt.schema)
			if tt.wantErr {
				if result.Valid {
					t.Errorf("expected validation error but got valid")
				}
				if tt.errMsg != "" && !strings.Contains(result.Error(), tt.errMsg) {
					t.Errorf("error %q does not contain %q", result.Error(), tt.errMsg)
				}
			} else {
				if !result.Valid {
					t.Errorf("expected valid but got error: %v", result.Error())
				}
			}
		})
	}
}

func TestStringValidation(t *testing.T) {
	tests := []struct {
		name    string
		schema  *ArgumentSchema
		args    map[string]any
		wantErr bool
		errMsg  string
	}{
		{
			name: "minLength valid",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"name": {Type: TypeString, MinLength: intPtr(3)},
				},
			},
			args:    map[string]any{"name": "Alice"},
			wantErr: false,
		},
		{
			name: "minLength invalid",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"name": {Type: TypeString, MinLength: intPtr(10)},
				},
			},
			args:    map[string]any{"name": "Alice"},
			wantErr: true,
			errMsg:  "less than minimum",
		},
		{
			name: "maxLength valid",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"name": {Type: TypeString, MaxLength: intPtr(10)},
				},
			},
			args:    map[string]any{"name": "Alice"},
			wantErr: false,
		},
		{
			name: "maxLength invalid",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"name": {Type: TypeString, MaxLength: intPtr(3)},
				},
			},
			args:    map[string]any{"name": "Alice"},
			wantErr: true,
			errMsg:  "exceeds maximum",
		},
		{
			name: "pattern valid email",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"email": {Type: TypeString, Pattern: strPtr(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)},
				},
			},
			args:    map[string]any{"email": "test@example.com"},
			wantErr: false,
		},
		{
			name: "pattern invalid email",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"email": {Type: TypeString, Pattern: strPtr(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)},
				},
			},
			args:    map[string]any{"email": "not-an-email"},
			wantErr: true,
			errMsg:  "does not match pattern",
		},
		{
			name: "invalid regex pattern",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"field": {Type: TypeString, Pattern: strPtr(`[invalid`)},
				},
			},
			args:    map[string]any{"field": "test"},
			wantErr: true,
			errMsg:  "invalid regex pattern",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateArguments(tt.args, tt.schema)
			if tt.wantErr {
				if result.Valid {
					t.Errorf("expected validation error but got valid")
				}
				if tt.errMsg != "" && !strings.Contains(result.Error(), tt.errMsg) {
					t.Errorf("error %q does not contain %q", result.Error(), tt.errMsg)
				}
			} else {
				if !result.Valid {
					t.Errorf("expected valid but got error: %v", result.Error())
				}
			}
		})
	}
}

func TestNumberValidation(t *testing.T) {
	tests := []struct {
		name    string
		schema  *ArgumentSchema
		args    map[string]any
		wantErr bool
		errMsg  string
	}{
		{
			name: "minimum valid",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"age": {Type: TypeNumber, Minimum: floatPtr(0)},
				},
			},
			args:    map[string]any{"age": float64(25)},
			wantErr: false,
		},
		{
			name: "minimum invalid",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"age": {Type: TypeNumber, Minimum: floatPtr(18)},
				},
			},
			args:    map[string]any{"age": float64(15)},
			wantErr: true,
			errMsg:  "less than minimum",
		},
		{
			name: "maximum valid",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"score": {Type: TypeNumber, Maximum: floatPtr(100)},
				},
			},
			args:    map[string]any{"score": float64(85)},
			wantErr: false,
		},
		{
			name: "maximum invalid",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"score": {Type: TypeNumber, Maximum: floatPtr(100)},
				},
			},
			args:    map[string]any{"score": float64(150)},
			wantErr: true,
			errMsg:  "exceeds maximum",
		},
		{
			name: "range valid",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"rating": {Type: TypeNumber, Minimum: floatPtr(1), Maximum: floatPtr(5)},
				},
			},
			args:    map[string]any{"rating": float64(3)},
			wantErr: false,
		},
		{
			name: "int type converts to float",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"count": {Type: TypeNumber, Minimum: floatPtr(0)},
				},
			},
			args:    map[string]any{"count": 10},
			wantErr: false,
		},
		{
			name: "int64 type converts to float",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"count": {Type: TypeNumber},
				},
			},
			args:    map[string]any{"count": int64(10)},
			wantErr: false,
		},
		{
			name: "int32 type converts to float",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"count": {Type: TypeNumber},
				},
			},
			args:    map[string]any{"count": int32(10)},
			wantErr: false,
		},
		{
			name: "float32 type converts to float64",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"value": {Type: TypeNumber},
				},
			},
			args:    map[string]any{"value": float32(3.14)},
			wantErr: false,
		},
		{
			name: "json.Number valid",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"value": {Type: TypeNumber, Minimum: floatPtr(0)},
				},
			},
			args:    map[string]any{"value": json.Number("42.5")},
			wantErr: false,
		},
		{
			name: "json.Number invalid string",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"value": {Type: TypeNumber},
				},
			},
			args:    map[string]any{"value": json.Number("not-a-number")},
			wantErr: true,
			errMsg:  "invalid number",
		},
		{
			name: "string is not a number",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"value": {Type: TypeNumber},
				},
			},
			args:    map[string]any{"value": "42"},
			wantErr: true,
			errMsg:  "expected number",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateArguments(tt.args, tt.schema)
			if tt.wantErr {
				if result.Valid {
					t.Errorf("expected validation error but got valid")
				}
				if tt.errMsg != "" && !strings.Contains(result.Error(), tt.errMsg) {
					t.Errorf("error %q does not contain %q", result.Error(), tt.errMsg)
				}
			} else {
				if !result.Valid {
					t.Errorf("expected valid but got error: %v", result.Error())
				}
			}
		})
	}
}

func TestArrayValidation(t *testing.T) {
	tests := []struct {
		name    string
		schema  *ArgumentSchema
		args    map[string]any
		wantErr bool
		errMsg  string
	}{
		{
			name: "minItems valid",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"items": {Type: TypeArray, MinItems: intPtr(1)},
				},
			},
			args:    map[string]any{"items": []any{1, 2, 3}},
			wantErr: false,
		},
		{
			name: "minItems invalid",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"items": {Type: TypeArray, MinItems: intPtr(5)},
				},
			},
			args:    map[string]any{"items": []any{1, 2}},
			wantErr: true,
			errMsg:  "less than minimum",
		},
		{
			name: "maxItems valid",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"items": {Type: TypeArray, MaxItems: intPtr(5)},
				},
			},
			args:    map[string]any{"items": []any{1, 2, 3}},
			wantErr: false,
		},
		{
			name: "maxItems invalid",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"items": {Type: TypeArray, MaxItems: intPtr(2)},
				},
			},
			args:    map[string]any{"items": []any{1, 2, 3, 4, 5}},
			wantErr: true,
			errMsg:  "exceeds maximum",
		},
		{
			name: "items type validation valid",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"names": {
						Type:  TypeArray,
						Items: &PropertySchema{Type: TypeString},
					},
				},
			},
			args:    map[string]any{"names": []any{"Alice", "Bob"}},
			wantErr: false,
		},
		{
			name: "items type validation invalid",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"names": {
						Type:  TypeArray,
						Items: &PropertySchema{Type: TypeString},
					},
				},
			},
			args:    map[string]any{"names": []any{"Alice", 123}},
			wantErr: true,
			errMsg:  "expected string",
		},
		{
			name: "not an array",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"items": {Type: TypeArray},
				},
			},
			args:    map[string]any{"items": "not-an-array"},
			wantErr: true,
			errMsg:  "expected array",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateArguments(tt.args, tt.schema)
			if tt.wantErr {
				if result.Valid {
					t.Errorf("expected validation error but got valid")
				}
				if tt.errMsg != "" && !strings.Contains(result.Error(), tt.errMsg) {
					t.Errorf("error %q does not contain %q", result.Error(), tt.errMsg)
				}
			} else {
				if !result.Valid {
					t.Errorf("expected valid but got error: %v", result.Error())
				}
			}
		})
	}
}

func TestComplexNestedArguments(t *testing.T) {
	tests := []struct {
		name    string
		schema  *ArgumentSchema
		args    map[string]any
		wantErr bool
		errMsg  string
	}{
		{
			name: "nested object valid",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"user": {
						Type: TypeObject,
						Properties: map[string]PropertySchema{
							"name":  {Type: TypeString},
							"email": {Type: TypeString, Pattern: strPtr(`^.+@.+\..+$`)},
						},
						Required: []string{"name"},
					},
				},
			},
			args: map[string]any{
				"user": map[string]any{
					"name":  "Alice",
					"email": "alice@example.com",
				},
			},
			wantErr: false,
		},
		{
			name: "nested object missing required",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"user": {
						Type: TypeObject,
						Properties: map[string]PropertySchema{
							"name": {Type: TypeString},
						},
						Required: []string{"name"},
					},
				},
			},
			args: map[string]any{
				"user": map[string]any{},
			},
			wantErr: true,
			errMsg:  "user.name: required field",
		},
		{
			name: "deeply nested structure",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"level1": {
						Type: TypeObject,
						Properties: map[string]PropertySchema{
							"level2": {
								Type: TypeObject,
								Properties: map[string]PropertySchema{
									"level3": {Type: TypeString, MinLength: intPtr(1)},
								},
							},
						},
					},
				},
			},
			args: map[string]any{
				"level1": map[string]any{
					"level2": map[string]any{
						"level3": "deep value",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "array of objects valid",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"users": {
						Type: TypeArray,
						Items: &PropertySchema{
							Type: TypeObject,
							Properties: map[string]PropertySchema{
								"id":   {Type: TypeInteger},
								"name": {Type: TypeString},
							},
							Required: []string{"id"},
						},
					},
				},
			},
			args: map[string]any{
				"users": []any{
					map[string]any{"id": float64(1), "name": "Alice"},
					map[string]any{"id": float64(2), "name": "Bob"},
				},
			},
			wantErr: false,
		},
		{
			name: "array of objects with invalid item",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"users": {
						Type: TypeArray,
						Items: &PropertySchema{
							Type: TypeObject,
							Properties: map[string]PropertySchema{
								"id": {Type: TypeInteger},
							},
							Required: []string{"id"},
						},
					},
				},
			},
			args: map[string]any{
				"users": []any{
					map[string]any{"id": float64(1)},
					map[string]any{}, // missing required "id"
				},
			},
			wantErr: true,
			errMsg:  "required field is missing",
		},
		{
			name: "object is not an object",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"config": {Type: TypeObject},
				},
			},
			args:    map[string]any{"config": "not-an-object"},
			wantErr: true,
			errMsg:  "expected object",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateArguments(tt.args, tt.schema)
			if tt.wantErr {
				if result.Valid {
					t.Errorf("expected validation error but got valid")
				}
				if tt.errMsg != "" && !strings.Contains(result.Error(), tt.errMsg) {
					t.Errorf("error %q does not contain %q", result.Error(), tt.errMsg)
				}
			} else {
				if !result.Valid {
					t.Errorf("expected valid but got error: %v", result.Error())
				}
			}
		})
	}
}

func TestArgumentSizeLimit(t *testing.T) {
	tests := []struct {
		name    string
		args    map[string]any
		maxSize int
		wantErr bool
	}{
		{
			name:    "small args within limit",
			args:    map[string]any{"name": "test"},
			maxSize: 1024,
			wantErr: false,
		},
		{
			name:    "nil args",
			args:    nil,
			maxSize: 1024,
			wantErr: false,
		},
		{
			name:    "args exceed limit",
			args:    map[string]any{"data": strings.Repeat("x", 1000)},
			maxSize: 100,
			wantErr: true,
		},
		{
			name:    "args exactly at limit",
			args:    map[string]any{"a": "b"},
			maxSize: 9, // {"a":"b"} = 9 bytes
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateArgumentSize(tt.args, tt.maxSize)
			if tt.wantErr && err == nil {
				t.Errorf("expected size validation error but got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
		})
	}
}

func TestCalculateArgumentSize(t *testing.T) {
	tests := []struct {
		name     string
		args     map[string]any
		wantSize int
	}{
		{
			name:     "nil args",
			args:     nil,
			wantSize: 0,
		},
		{
			name:     "empty object",
			args:     map[string]any{},
			wantSize: 2, // {}
		},
		{
			name:     "simple object",
			args:     map[string]any{"a": "b"},
			wantSize: 9, // {"a":"b"}
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			size := CalculateArgumentSize(tt.args)
			if size != tt.wantSize {
				t.Errorf("CalculateArgumentSize() = %d, want %d", size, tt.wantSize)
			}
		})
	}
}

func TestArgumentTypeCoercion(t *testing.T) {
	tests := []struct {
		name    string
		schema  *ArgumentSchema
		args    map[string]any
		wantErr bool
		errMsg  string
	}{
		{
			name: "string to number should fail",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"count": {Type: TypeNumber},
				},
			},
			args:    map[string]any{"count": "42"},
			wantErr: true,
			errMsg:  "expected number",
		},
		{
			name: "number to string should fail",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"name": {Type: TypeString},
				},
			},
			args:    map[string]any{"name": 42},
			wantErr: true,
			errMsg:  "expected string",
		},
		{
			name: "string to boolean should fail",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"enabled": {Type: TypeBoolean},
				},
			},
			args:    map[string]any{"enabled": "true"},
			wantErr: true,
			errMsg:  "expected boolean",
		},
		{
			name: "number 1 to boolean should fail",
			schema: &ArgumentSchema{
				Type: TypeObject,
				Properties: map[string]PropertySchema{
					"enabled": {Type: TypeBoolean},
				},
			},
			args:    map[string]any{"enabled": 1},
			wantErr: true,
			errMsg:  "expected boolean",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateArguments(tt.args, tt.schema)
			if tt.wantErr {
				if result.Valid {
					t.Errorf("expected validation error but got valid")
				}
				if tt.errMsg != "" && !strings.Contains(result.Error(), tt.errMsg) {
					t.Errorf("error %q does not contain %q", result.Error(), tt.errMsg)
				}
			} else {
				if !result.Valid {
					t.Errorf("expected valid but got error: %v", result.Error())
				}
			}
		})
	}
}

func TestValidateArgumentsFromJSON(t *testing.T) {
	schemaJSON := []byte(`{
		"type": "object",
		"properties": {
			"name": {"type": "string", "minLength": 1},
			"age": {"type": "integer", "minimum": 0}
		},
		"required": ["name"]
	}`)

	t.Run("valid args", func(t *testing.T) {
		args := map[string]any{"name": "Alice", "age": float64(30)}
		result, err := ValidateArgumentsFromJSON(args, schemaJSON)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Valid {
			t.Errorf("expected valid but got: %v", result.Error())
		}
	})

	t.Run("missing required field", func(t *testing.T) {
		args := map[string]any{"age": float64(30)}
		result, err := ValidateArgumentsFromJSON(args, schemaJSON)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Valid {
			t.Error("expected invalid but got valid")
		}
	})

	t.Run("invalid schema JSON", func(t *testing.T) {
		args := map[string]any{"name": "Alice"}
		_, err := ValidateArgumentsFromJSON(args, []byte("invalid json"))
		if err == nil {
			t.Error("expected error for invalid schema JSON")
		}
	})
}

func TestValidationErrorInterface(t *testing.T) {
	t.Run("error with field", func(t *testing.T) {
		err := &ValidationError{Field: "name", Message: "is required"}
		if err.Error() != "name: is required" {
			t.Errorf("unexpected error format: %s", err.Error())
		}
	})

	t.Run("error without field", func(t *testing.T) {
		err := &ValidationError{Message: "general error"}
		if err.Error() != "general error" {
			t.Errorf("unexpected error format: %s", err.Error())
		}
	})
}

func TestValidationResultError(t *testing.T) {
	t.Run("valid result has empty error", func(t *testing.T) {
		result := &ValidationResult{Valid: true}
		if result.Error() != "" {
			t.Errorf("expected empty error for valid result, got: %s", result.Error())
		}
	})

	t.Run("multiple errors joined", func(t *testing.T) {
		result := &ValidationResult{
			Valid: false,
			Errors: []*ValidationError{
				{Field: "a", Message: "error 1"},
				{Field: "b", Message: "error 2"},
			},
		}
		errStr := result.Error()
		if !strings.Contains(errStr, "error 1") || !strings.Contains(errStr, "error 2") {
			t.Errorf("expected both errors in message, got: %s", errStr)
		}
	})
}

func TestNewArgumentValidator(t *testing.T) {
	schema := &ArgumentSchema{
		Type: TypeObject,
		Properties: map[string]PropertySchema{
			"name": {Type: TypeString},
		},
	}
	validator := NewArgumentValidator(schema)
	if validator == nil {
		t.Fatal("NewArgumentValidator returned nil")
	}
	result := validator.Validate(map[string]any{"name": "test"})
	if !result.Valid {
		t.Errorf("expected valid result: %v", result.Error())
	}
}
