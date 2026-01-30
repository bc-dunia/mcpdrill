package transport

import (
	"strings"
	"testing"
)

func TestValidateResult(t *testing.T) {
	tests := []struct {
		name    string
		result  *ToolsCallResult
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil result",
			result:  nil,
			wantErr: true,
			errMsg:  "result is nil",
		},
		{
			name: "valid text result",
			result: &ToolsCallResult{
				Content: []ToolContent{
					{Type: ContentTypeText, Text: "Hello, world!"},
				},
				IsError: false,
			},
			wantErr: false,
		},
		{
			name: "valid multi-content result",
			result: &ToolsCallResult{
				Content: []ToolContent{
					{Type: ContentTypeText, Text: "First"},
					{Type: ContentTypeText, Text: "Second"},
				},
				IsError: false,
			},
			wantErr: false,
		},
		{
			name: "empty content array",
			result: &ToolsCallResult{
				Content: []ToolContent{},
				IsError: false,
			},
			wantErr: true,
			errMsg:  "content array is empty",
		},
		{
			name: "content missing type",
			result: &ToolsCallResult{
				Content: []ToolContent{
					{Type: "", Text: "no type"},
				},
				IsError: false,
			},
			wantErr: true,
			errMsg:  "type is required",
		},
		{
			name: "text type missing text",
			result: &ToolsCallResult{
				Content: []ToolContent{
					{Type: ContentTypeText, Text: ""},
				},
				IsError: false,
			},
			wantErr: true,
			errMsg:  "text is required",
		},
		{
			name: "image type valid without text",
			result: &ToolsCallResult{
				Content: []ToolContent{
					{Type: ContentTypeImage},
				},
				IsError: false,
			},
			wantErr: false,
		},
		{
			name: "resource type valid without text",
			result: &ToolsCallResult{
				Content: []ToolContent{
					{Type: ContentTypeResource},
				},
				IsError: false,
			},
			wantErr: false,
		},
		{
			name: "unknown type accepted (forward compatibility)",
			result: &ToolsCallResult{
				Content: []ToolContent{
					{Type: "future-type"},
				},
				IsError: false,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateResult(tt.result)
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

func TestErrorConsistency(t *testing.T) {
	tests := []struct {
		name    string
		result  *ToolsCallResult
		wantErr bool
		errMsg  string
	}{
		{
			name: "isError true with error message",
			result: &ToolsCallResult{
				Content: []ToolContent{
					{Type: ContentTypeText, Text: "Error: something went wrong"},
				},
				IsError: true,
			},
			wantErr: false,
		},
		{
			name: "isError true without text content",
			result: &ToolsCallResult{
				Content: []ToolContent{
					{Type: ContentTypeImage},
				},
				IsError: true,
			},
			wantErr: true,
			errMsg:  "isError is true but no text content",
		},
		{
			name: "isError true with empty text",
			result: &ToolsCallResult{
				Content: []ToolContent{
					{Type: ContentTypeText, Text: ""},
				},
				IsError: true,
			},
			wantErr: true, // both "text is required" and "isError true but no text content"
		},
		{
			name: "isError false without text is valid",
			result: &ToolsCallResult{
				Content: []ToolContent{
					{Type: ContentTypeImage},
				},
				IsError: false,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateResult(tt.result)
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

func TestResultPayloadSize(t *testing.T) {
	t.Run("small payload within limit", func(t *testing.T) {
		result := &ToolsCallResult{
			Content: []ToolContent{
				{Type: ContentTypeText, Text: "small"},
			},
		}
		vr := ValidateResultWithMaxSize(result, 1024)
		if !vr.Valid {
			t.Errorf("expected valid but got: %v", vr.Error())
		}
		if vr.PayloadSize == 0 {
			t.Error("expected non-zero payload size")
		}
	})

	t.Run("large payload exceeds limit", func(t *testing.T) {
		largeText := strings.Repeat("x", 1000)
		result := &ToolsCallResult{
			Content: []ToolContent{
				{Type: ContentTypeText, Text: largeText},
			},
		}
		vr := ValidateResultWithMaxSize(result, 100)
		if vr.Valid {
			t.Error("expected validation error for large payload")
		}
		if !strings.Contains(vr.Error(), "exceeds maximum") {
			t.Errorf("unexpected error: %v", vr.Error())
		}
	})

	t.Run("default max size is large", func(t *testing.T) {
		result := &ToolsCallResult{
			Content: []ToolContent{
				{Type: ContentTypeText, Text: strings.Repeat("x", 10000)},
			},
		}
		vr := ValidateResult(result)
		if !vr.Valid {
			t.Errorf("expected valid with default max size: %v", vr.Error())
		}
	})
}

func TestCalculateResultSize(t *testing.T) {
	t.Run("nil result", func(t *testing.T) {
		size := CalculateResultSize(nil)
		if size != 0 {
			t.Errorf("expected 0 for nil result, got %d", size)
		}
	})

	t.Run("empty result", func(t *testing.T) {
		result := &ToolsCallResult{
			Content: []ToolContent{},
		}
		size := CalculateResultSize(result)
		if size == 0 {
			t.Error("expected non-zero size for empty result struct")
		}
	})

	t.Run("result with content", func(t *testing.T) {
		result := &ToolsCallResult{
			Content: []ToolContent{
				{Type: ContentTypeText, Text: "hello"},
			},
		}
		size := CalculateResultSize(result)
		if size == 0 {
			t.Error("expected non-zero size")
		}
	})
}

func TestExtractResultMetrics(t *testing.T) {
	t.Run("nil result", func(t *testing.T) {
		metrics := ExtractResultMetrics(nil)
		if metrics.PayloadSize != 0 {
			t.Errorf("expected 0 payload size, got %d", metrics.PayloadSize)
		}
		if metrics.ContentCount != 0 {
			t.Errorf("expected 0 content count, got %d", metrics.ContentCount)
		}
	})

	t.Run("result with text content", func(t *testing.T) {
		result := &ToolsCallResult{
			Content: []ToolContent{
				{Type: ContentTypeText, Text: "hello"},
				{Type: ContentTypeText, Text: "world"},
			},
			IsError: false,
		}
		metrics := ExtractResultMetrics(result)
		if metrics.ContentCount != 2 {
			t.Errorf("expected 2 content items, got %d", metrics.ContentCount)
		}
		if metrics.HasError {
			t.Error("expected HasError to be false")
		}
		if metrics.TextContentSize != 10 { // "hello" + "world" = 10
			t.Errorf("expected text size 10, got %d", metrics.TextContentSize)
		}
	})

	t.Run("error result", func(t *testing.T) {
		result := &ToolsCallResult{
			Content: []ToolContent{
				{Type: ContentTypeText, Text: "error occurred"},
			},
			IsError: true,
		}
		metrics := ExtractResultMetrics(result)
		if !metrics.HasError {
			t.Error("expected HasError to be true")
		}
	})

	t.Run("mixed content types", func(t *testing.T) {
		result := &ToolsCallResult{
			Content: []ToolContent{
				{Type: ContentTypeText, Text: "text1"},
				{Type: ContentTypeImage},
				{Type: ContentTypeText, Text: "text2"},
			},
		}
		metrics := ExtractResultMetrics(result)
		if metrics.ContentCount != 3 {
			t.Errorf("expected 3 content items, got %d", metrics.ContentCount)
		}
		if metrics.TextContentSize != 10 { // "text1" + "text2" = 10
			t.Errorf("expected text size 10, got %d", metrics.TextContentSize)
		}
	})
}

func TestResultValidatorOptions(t *testing.T) {
	t.Run("custom max payload size", func(t *testing.T) {
		validator := NewResultValidator(WithMaxPayloadSize(50))
		result := &ToolsCallResult{
			Content: []ToolContent{
				{Type: ContentTypeText, Text: strings.Repeat("x", 100)},
			},
		}
		vr := validator.Validate(result)
		if vr.Valid {
			t.Error("expected validation error with small max size")
		}
	})

	t.Run("default validator", func(t *testing.T) {
		validator := NewResultValidator()
		result := &ToolsCallResult{
			Content: []ToolContent{
				{Type: ContentTypeText, Text: "test"},
			},
		}
		vr := validator.Validate(result)
		if !vr.Valid {
			t.Errorf("expected valid: %v", vr.Error())
		}
	})
}

func TestResultValidationErrorInterface(t *testing.T) {
	t.Run("error with field", func(t *testing.T) {
		err := &ResultValidationError{Field: "content", Message: "is empty"}
		if err.Error() != "content: is empty" {
			t.Errorf("unexpected error format: %s", err.Error())
		}
	})

	t.Run("error without field", func(t *testing.T) {
		err := &ResultValidationError{Message: "general error"}
		if err.Error() != "general error" {
			t.Errorf("unexpected error format: %s", err.Error())
		}
	})
}

func TestResultValidationResultError(t *testing.T) {
	t.Run("valid result has empty error", func(t *testing.T) {
		result := &ResultValidationResult{Valid: true}
		if result.Error() != "" {
			t.Errorf("expected empty error for valid result, got: %s", result.Error())
		}
	})

	t.Run("multiple errors joined", func(t *testing.T) {
		result := &ResultValidationResult{
			Valid: false,
			Errors: []*ResultValidationError{
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

func TestMultipleContentValidation(t *testing.T) {
	t.Run("multiple content items with errors", func(t *testing.T) {
		result := &ToolsCallResult{
			Content: []ToolContent{
				{Type: ContentTypeText, Text: "valid"},
				{Type: "", Text: "missing type"},
				{Type: ContentTypeText, Text: ""},
			},
		}
		vr := ValidateResult(result)
		if vr.Valid {
			t.Error("expected validation errors")
		}
		if len(vr.Errors) < 2 {
			t.Errorf("expected at least 2 errors, got %d", len(vr.Errors))
		}
	})

	t.Run("all content items valid", func(t *testing.T) {
		result := &ToolsCallResult{
			Content: []ToolContent{
				{Type: ContentTypeText, Text: "first"},
				{Type: ContentTypeText, Text: "second"},
				{Type: ContentTypeImage},
			},
		}
		vr := ValidateResult(result)
		if !vr.Valid {
			t.Errorf("expected valid: %v", vr.Error())
		}
	})
}

func TestEdgeCases(t *testing.T) {
	t.Run("whitespace-only text is valid", func(t *testing.T) {
		result := &ToolsCallResult{
			Content: []ToolContent{
				{Type: ContentTypeText, Text: "   "},
			},
		}
		vr := ValidateResult(result)
		if !vr.Valid {
			t.Errorf("expected valid for whitespace text: %v", vr.Error())
		}
	})

	t.Run("very long text content", func(t *testing.T) {
		result := &ToolsCallResult{
			Content: []ToolContent{
				{Type: ContentTypeText, Text: strings.Repeat("x", 1000000)},
			},
		}
		vr := ValidateResult(result)
		if !vr.Valid {
			t.Errorf("expected valid for long text: %v", vr.Error())
		}
		if vr.PayloadSize < 1000000 {
			t.Errorf("expected payload size > 1MB, got %d", vr.PayloadSize)
		}
	})

	t.Run("unicode text content", func(t *testing.T) {
		result := &ToolsCallResult{
			Content: []ToolContent{
				{Type: ContentTypeText, Text: "Hello, 世界! 🌍"},
			},
		}
		vr := ValidateResult(result)
		if !vr.Valid {
			t.Errorf("expected valid for unicode text: %v", vr.Error())
		}
	})
}
