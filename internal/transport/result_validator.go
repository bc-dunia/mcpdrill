package transport

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	ContentTypeText     = "text"
	ContentTypeImage    = "image"
	ContentTypeResource = "resource"
)

type ResultValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e *ResultValidationError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("%s: %s", e.Field, e.Message)
	}
	return e.Message
}

type ResultValidationResult struct {
	Valid       bool                     `json:"valid"`
	Errors      []*ResultValidationError `json:"errors,omitempty"`
	PayloadSize int                      `json:"payload_size"`
}

func (r *ResultValidationResult) Error() string {
	if r.Valid {
		return ""
	}
	var msgs []string
	for _, e := range r.Errors {
		msgs = append(msgs, e.Error())
	}
	return strings.Join(msgs, "; ")
}

type ResultValidator struct {
	maxPayloadSize int
}

type ResultValidatorOption func(*ResultValidator)

func WithMaxPayloadSize(size int) ResultValidatorOption {
	return func(v *ResultValidator) {
		v.maxPayloadSize = size
	}
}

const DefaultMaxResultPayloadSize = 100 * 1024 * 1024

func NewResultValidator(opts ...ResultValidatorOption) *ResultValidator {
	v := &ResultValidator{
		maxPayloadSize: DefaultMaxResultPayloadSize,
	}
	for _, opt := range opts {
		opt(v)
	}
	return v
}

func (v *ResultValidator) Validate(result *ToolsCallResult) *ResultValidationResult {
	vr := &ResultValidationResult{Valid: true}

	if result == nil {
		vr.Valid = false
		vr.Errors = append(vr.Errors, &ResultValidationError{
			Message: "result is nil",
		})
		return vr
	}

	vr.PayloadSize = v.calculatePayloadSize(result)

	if vr.PayloadSize > v.maxPayloadSize {
		vr.Valid = false
		vr.Errors = append(vr.Errors, &ResultValidationError{
			Field:   "payload",
			Message: fmt.Sprintf("payload size %d bytes exceeds maximum %d bytes", vr.PayloadSize, v.maxPayloadSize),
		})
	}

	v.validateContent(result, vr)
	v.validateErrorConsistency(result, vr)

	return vr
}

func (v *ResultValidator) validateContent(result *ToolsCallResult, vr *ResultValidationResult) {
	if len(result.Content) == 0 {
		vr.Valid = false
		vr.Errors = append(vr.Errors, &ResultValidationError{
			Field:   "content",
			Message: "content array is empty",
		})
		return
	}

	for i, content := range result.Content {
		v.validateContentItem(i, &content, vr)
	}
}

func (v *ResultValidator) validateContentItem(index int, content *ToolContent, vr *ResultValidationResult) {
	fieldPrefix := fmt.Sprintf("content[%d]", index)

	if content.Type == "" {
		vr.Valid = false
		vr.Errors = append(vr.Errors, &ResultValidationError{
			Field:   fieldPrefix + ".type",
			Message: "type is required",
		})
	}

	switch content.Type {
	case ContentTypeText:
		if content.Text == "" {
			vr.Valid = false
			vr.Errors = append(vr.Errors, &ResultValidationError{
				Field:   fieldPrefix + ".text",
				Message: "text is required for text content type",
			})
		}
	case ContentTypeImage, ContentTypeResource:
		// Valid types, additional fields may be present
	case "":
		// Already handled above
	default:
		// Unknown type - warn but don't fail (forward compatibility)
	}
}

func (v *ResultValidator) validateErrorConsistency(result *ToolsCallResult, vr *ResultValidationResult) {
	if !result.IsError {
		return
	}

	hasErrorContent := false
	for _, content := range result.Content {
		if content.Type == ContentTypeText && content.Text != "" {
			hasErrorContent = true
			break
		}
	}

	if !hasErrorContent {
		vr.Valid = false
		vr.Errors = append(vr.Errors, &ResultValidationError{
			Field:   "isError",
			Message: "isError is true but no text content describing the error",
		})
	}
}

func (v *ResultValidator) calculatePayloadSize(result *ToolsCallResult) int {
	data, err := json.Marshal(result)
	if err != nil {
		return 0
	}
	return len(data)
}

func ValidateResult(result *ToolsCallResult) *ResultValidationResult {
	return NewResultValidator().Validate(result)
}

func ValidateResultWithMaxSize(result *ToolsCallResult, maxSize int) *ResultValidationResult {
	return NewResultValidator(WithMaxPayloadSize(maxSize)).Validate(result)
}

func CalculateResultSize(result *ToolsCallResult) int {
	if result == nil {
		return 0
	}
	data, err := json.Marshal(result)
	if err != nil {
		return 0
	}
	return len(data)
}

type ResultMetrics struct {
	PayloadSize     int  `json:"payload_size"`
	ContentCount    int  `json:"content_count"`
	HasError        bool `json:"has_error"`
	TextContentSize int  `json:"text_content_size"`
}

func ExtractResultMetrics(result *ToolsCallResult) *ResultMetrics {
	if result == nil {
		return &ResultMetrics{}
	}

	metrics := &ResultMetrics{
		PayloadSize:  CalculateResultSize(result),
		ContentCount: len(result.Content),
		HasError:     result.IsError,
	}

	for _, content := range result.Content {
		if content.Type == ContentTypeText {
			metrics.TextContentSize += len(content.Text)
		}
	}

	return metrics
}
