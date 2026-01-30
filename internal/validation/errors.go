// Package validation provides schema and semantic validation for mcpdrill configurations.
package validation

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ValidationLevel indicates the severity of a validation issue.
type ValidationLevel string

const (
	LevelError   ValidationLevel = "error"
	LevelWarning ValidationLevel = "warning"
)

// ValidationIssue represents a single validation problem.
type ValidationIssue struct {
	Level       ValidationLevel `json:"level"`
	Code        string          `json:"code"`
	Message     string          `json:"message"`
	JSONPointer string          `json:"json_pointer,omitempty"`
	Remediation string          `json:"remediation,omitempty"`
}

// ValidationReport contains the results of validating a configuration.
type ValidationReport struct {
	OK       bool              `json:"ok"`
	Errors   []ValidationIssue `json:"errors"`
	Warnings []ValidationIssue `json:"warnings"`
}

// NewValidationReport creates a new empty validation report.
func NewValidationReport() *ValidationReport {
	return &ValidationReport{
		OK:       true,
		Errors:   []ValidationIssue{},
		Warnings: []ValidationIssue{},
	}
}

// AddError adds an error-level issue to the report.
func (r *ValidationReport) AddError(code, message, jsonPointer string) {
	r.OK = false
	r.Errors = append(r.Errors, ValidationIssue{
		Level:       LevelError,
		Code:        code,
		Message:     message,
		JSONPointer: jsonPointer,
	})
}

// AddErrorWithRemediation adds an error-level issue with remediation guidance.
func (r *ValidationReport) AddErrorWithRemediation(code, message, jsonPointer, remediation string) {
	r.OK = false
	r.Errors = append(r.Errors, ValidationIssue{
		Level:       LevelError,
		Code:        code,
		Message:     message,
		JSONPointer: jsonPointer,
		Remediation: remediation,
	})
}

// AddWarning adds a warning-level issue to the report.
func (r *ValidationReport) AddWarning(code, message, jsonPointer string) {
	r.Warnings = append(r.Warnings, ValidationIssue{
		Level:       LevelWarning,
		Code:        code,
		Message:     message,
		JSONPointer: jsonPointer,
	})
}

// Merge combines another report into this one.
func (r *ValidationReport) Merge(other *ValidationReport) {
	if other == nil {
		return
	}
	if !other.OK {
		r.OK = false
	}
	r.Errors = append(r.Errors, other.Errors...)
	r.Warnings = append(r.Warnings, other.Warnings...)
}

// HasErrors returns true if there are any error-level issues.
func (r *ValidationReport) HasErrors() bool {
	return len(r.Errors) > 0
}

// HasWarnings returns true if there are any warning-level issues.
func (r *ValidationReport) HasWarnings() bool {
	return len(r.Warnings) > 0
}

// String returns a human-readable summary of the report.
func (r *ValidationReport) String() string {
	if r.OK && !r.HasWarnings() {
		return "Validation passed"
	}

	var sb strings.Builder
	if !r.OK {
		sb.WriteString(fmt.Sprintf("Validation failed with %d error(s)", len(r.Errors)))
		if r.HasWarnings() {
			sb.WriteString(fmt.Sprintf(" and %d warning(s)", len(r.Warnings)))
		}
		sb.WriteString(":\n")
	} else {
		sb.WriteString(fmt.Sprintf("Validation passed with %d warning(s):\n", len(r.Warnings)))
	}

	for _, e := range r.Errors {
		sb.WriteString(fmt.Sprintf("  [ERROR] %s: %s", e.Code, e.Message))
		if e.JSONPointer != "" {
			sb.WriteString(fmt.Sprintf(" (at %s)", e.JSONPointer))
		}
		sb.WriteString("\n")
	}

	for _, w := range r.Warnings {
		sb.WriteString(fmt.Sprintf("  [WARN] %s: %s", w.Code, w.Message))
		if w.JSONPointer != "" {
			sb.WriteString(fmt.Sprintf(" (at %s)", w.JSONPointer))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// Validation Issue Codes - Schema Validation
const (
	CodeSchemaViolation      = "SCHEMA_VIOLATION"
	CodeRequiredFieldMissing = "REQUIRED_FIELD_MISSING"
	CodeInvalidFormat        = "INVALID_FORMAT"
	CodeInvalidSchemaVersion = "INVALID_SCHEMA_VERSION"
)

// Validation Issue Codes - Security/SSRF
const (
	CodeAllowlistViolation      = "ALLOWLIST_VIOLATION"
	CodeIPLiteralBlocked        = "IP_LITERAL_BLOCKED"
	CodePrivateAddressBlocked   = "PRIVATE_ADDRESS_BLOCKED"
	CodeForbiddenPatternMatched = "FORBIDDEN_PATTERN_MATCHED"
	CodeSecretRefNotAllowed     = "SECRET_REF_NOT_ALLOWED"
	CodeInvalidURLScheme        = "INVALID_URL_SCHEME"
	CodeUserInfoBlocked         = "USERINFO_BLOCKED"
	CodeLocalhostBlocked        = "LOCALHOST_BLOCKED"
	CodeLinkLocalBlocked        = "LINK_LOCAL_BLOCKED"
	CodeMetadataIPBlocked       = "METADATA_IP_BLOCKED"
	CodeLoopbackBlocked         = "LOOPBACK_BLOCKED"
	CodeMulticastBlocked        = "MULTICAST_BLOCKED"
	CodeIPv4MappedBlocked       = "IPV4_MAPPED_BLOCKED"
	CodeNAT64Blocked            = "NAT64_BLOCKED"
	CodeDocumentationIPBlocked  = "DOCUMENTATION_IP_BLOCKED"
	CodeUniqueLocalBlocked      = "UNIQUE_LOCAL_BLOCKED"
	CodeMaxRedirectsExceeded    = "MAX_REDIRECTS_EXCEEDED"
	CodeDNSRebindingBlocked     = "DNS_REBINDING_BLOCKED"
)

// Validation Issue Codes - Policy
const (
	CodeSystemPolicyViolation = "SYSTEM_POLICY_VIOLATION"
	CodeCapsExceeded          = "CAPS_EXCEEDED"
)

// Validation Issue Codes - Semantic
const (
	CodeStagesRequired             = "STAGES_REQUIRED"
	CodePreflightRequired          = "PREFLIGHT_REQUIRED"
	CodeBaselineRequired           = "BASELINE_REQUIRED"
	CodeRampRequired               = "RAMP_REQUIRED"
	CodePreflightNotFirst          = "PREFLIGHT_NOT_FIRST"
	CodeDurationInvalid            = "DURATION_INVALID"
	CodeLoadInvalid                = "LOAD_INVALID"
	CodeOperationMixEmpty          = "OPERATION_MIX_EMPTY"
	CodeToolsCallRequiresTemplates = "TOOLS_CALL_REQUIRES_TEMPLATES"
	CodeCapsRequired               = "CAPS_REQUIRED"
	CodeCapsInconsistent           = "CAPS_INCONSISTENT"
	CodeAllowlistRequired          = "ALLOWLIST_REQUIRED"
	CodeIdentificationRequired     = "IDENTIFICATION_REQUIRED"
	CodeRampByDefaultGuard         = "RAMP_BY_DEFAULT_GUARD"
	CodeStopConditionsRequired     = "STOP_CONDITIONS_REQUIRED"
	CodeStreamingGuardrails        = "STREAMING_GUARDRAILS"
	CodeRedirectPolicyRequired     = "REDIRECT_POLICY_REQUIRED"
	CodeInvalidStageOrder          = "INVALID_STAGE_ORDER"
	CodeInvalidWorkerFailurePolicy = "INVALID_WORKER_FAILURE_POLICY"
	CodeChurnIntervalOpsInvalid    = "CHURN_INTERVAL_OPS_INVALID"
)

// ErrorEnvelope represents the canonical API error response format.
type ErrorEnvelope struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains the details of an error.
type ErrorDetail struct {
	ErrorType    string                 `json:"error_type"`
	ErrorCode    string                 `json:"error_code"`
	ErrorMessage string                 `json:"error_message"`
	Retryable    bool                   `json:"retryable"`
	Details      map[string]interface{} `json:"details,omitempty"`
}

// API Error Types
const (
	ErrorTypeInvalidArgument    = "invalid_argument"
	ErrorTypeFailedPrecondition = "failed_precondition"
	ErrorTypeNotFound           = "not_found"
	ErrorTypeUnauthorized       = "unauthorized"
	ErrorTypeForbidden          = "forbidden"
	ErrorTypeRateLimited        = "rate_limited"
	ErrorTypeResourceExhausted  = "resource_exhausted"
	ErrorTypeTimeout            = "timeout"
	ErrorTypeUnavailable        = "unavailable"
	ErrorTypeConflict           = "conflict"
	ErrorTypeInternal           = "internal"
	ErrorTypeNotImplemented     = "not_implemented"
)

// NewValidationError creates an error envelope for validation failures.
func NewValidationError(report *ValidationReport) *ErrorEnvelope {
	issues := make([]map[string]interface{}, 0, len(report.Errors))
	for _, e := range report.Errors {
		issue := map[string]interface{}{
			"code":    e.Code,
			"message": e.Message,
		}
		if e.JSONPointer != "" {
			issue["json_pointer"] = e.JSONPointer
		}
		if e.Remediation != "" {
			issue["remediation"] = e.Remediation
		}
		issues = append(issues, issue)
	}

	return &ErrorEnvelope{
		Error: ErrorDetail{
			ErrorType:    ErrorTypeInvalidArgument,
			ErrorCode:    "VALIDATION_FAILED",
			ErrorMessage: "Configuration validation failed",
			Retryable:    false,
			Details: map[string]interface{}{
				"issues": issues,
			},
		},
	}
}

// ToJSON serializes the error envelope to JSON.
func (e *ErrorEnvelope) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

// ValidationError is an error type that wraps a validation report.
type ValidationError struct {
	Report *ValidationReport
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	return e.Report.String()
}

// NewValidationErrorFromReport creates a ValidationError from a report.
func NewValidationErrorFromReport(report *ValidationReport) error {
	if report.OK {
		return nil
	}
	return &ValidationError{Report: report}
}
