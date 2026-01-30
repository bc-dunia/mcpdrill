package validation

import (
	"fmt"
	"encoding/json"
	"regexp"
)

var (
	runIDPattern       = regexp.MustCompile(`^run_[0-9a-f]{16,64}$`)
	executionIDPattern = regexp.MustCompile(`^exe_[0-9a-f]{8,64}$`)
	workerIDPattern    = regexp.MustCompile(`^wkr_[0-9a-f]{8,64}$`)
	scenarioIDPattern  = regexp.MustCompile(`^scn_[a-z0-9][a-z0-9._-]{2,80}$`)
	stageIDPattern     = regexp.MustCompile(`^stg_[0-9a-f]{3,81}$`)
	vuIDPattern        = regexp.MustCompile(`^vu_[0-9]{1,10}$`)
	sessionIDPattern   = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,256}$`)
	leaseIDPattern     = regexp.MustCompile(`^lse_[0-9a-f]{8,64}$`)
	eventIDPattern     = regexp.MustCompile(`^evt_[0-9a-f]{8,64}$`)
)

const maxSessionIDLength = 256

const (
	CodeMissingCorrelationKey = "MISSING_CORRELATION_KEY"
	CodeInvalidIDFormat       = "INVALID_ID_FORMAT"
)

var requiredCorrelationKeys = []string{
	"run_id",
	"execution_id",
	"stage",
	"stage_id",
	"worker_id",
	"vu_id",
	"session_id",
}

// allowedStages defines the valid stage enum values
var allowedStages = map[string]bool{
	"preflight": true,
	"baseline":  true,
	"ramp":      true,
	"soak":      true,
	"spike":     true,
	"custom":    true,
}

type CorrelationValidator struct{}

func NewCorrelationValidator() *CorrelationValidator {
	return &CorrelationValidator{}
}

func (v *CorrelationValidator) ValidateOpLog(data []byte) *ValidationReport {
	report := NewValidationReport()

	var record map[string]interface{}
	if err := json.Unmarshal(data, &record); err != nil {
		report.AddError(CodeSchemaViolation, "Invalid JSON", "")
		return report
	}

	for _, key := range requiredCorrelationKeys {
		if _, exists := record[key]; !exists {
			report.AddError(CodeMissingCorrelationKey,
				key+" required",
				"/"+key)
		}
	}

	v.validateIDFormats(record, report)
	return report
}

func (v *CorrelationValidator) validateIDFormats(record map[string]interface{}, report *ValidationReport) {
	if runID, ok := record["run_id"].(string); ok {
		if !runIDPattern.MatchString(runID) {
			report.AddErrorWithRemediation(CodeInvalidIDFormat,
				"run_id must match pattern ^run_[0-9a-f]{16,64}$",
				"/run_id",
				"Use format: run_<16-64 hex chars (0-9a-f)>")
		}
	}

	if execID, ok := record["execution_id"].(string); ok {
		if !executionIDPattern.MatchString(execID) {
			report.AddErrorWithRemediation(CodeInvalidIDFormat,
				"execution_id must match pattern ^exe_[0-9a-f]{8,64}$",
				"/execution_id",
				"Use format: exe_<8-64 hex chars (0-9a-f)>")
		}
	}

	if workerID, ok := record["worker_id"].(string); ok {
		if !workerIDPattern.MatchString(workerID) {
			report.AddErrorWithRemediation(CodeInvalidIDFormat,
				"worker_id must match pattern ^wkr_[0-9a-f]{8,64}$",
				"/worker_id",
				"Use format: wkr_<8-64 hex chars (0-9a-f)>")
		}
	}

	if scenarioID, ok := record["scenario_id"].(string); ok && scenarioID != "" {
		if !scenarioIDPattern.MatchString(scenarioID) {
			report.AddErrorWithRemediation(CodeInvalidIDFormat,
				"scenario_id must match pattern ^scn_[a-z0-9][a-z0-9._-]{2,80}$",
				"/scenario_id",
				"Use format: scn_<alphanumeric start, then 2-80 chars of a-z0-9._->")
		}
	}

	if stageID, ok := record["stage_id"].(string); ok {
		if !stageIDPattern.MatchString(stageID) {
			report.AddErrorWithRemediation(CodeInvalidIDFormat,
				"stage_id must match pattern ^stg_[0-9a-f]{3,81}$",
				"/stage_id",
				"Use format: stg_<3-81 hex chars (0-9a-f)>")
		}
	}

	// Validate stage enum
	if stage, ok := record["stage"].(string); ok {
		if !allowedStages[stage] {
			report.AddErrorWithRemediation(CodeInvalidIDFormat,
				"stage must be one of: preflight, baseline, ramp, soak, spike, custom",
				"/stage",
				"Use one of the allowed stage values: preflight, baseline, ramp, soak, spike, custom")
		}
	}

	// Validate vu_id format
	if vuID, ok := record["vu_id"].(string); ok {
		if !vuIDPattern.MatchString(vuID) {
			report.AddErrorWithRemediation(CodeInvalidIDFormat,
				"vu_id must match pattern ^vu_[0-9]{1,10}$",
				"/vu_id",
				"Use format: vu_<1-10 digit number>")
		}
	}

	// Validate session_id format
	if sessionID, ok := record["session_id"].(string); ok {
		if len(sessionID) > maxSessionIDLength {
			report.AddError(CodeInvalidIDFormat,
				"session_id exceeds maximum length of 256 characters",
				"/session_id")
		} else if !sessionIDPattern.MatchString(sessionID) {
			report.AddErrorWithRemediation(CodeInvalidIDFormat,
				"session_id must contain only alphanumeric characters, underscores, dots, and hyphens",
				"/session_id",
				"Use format: 1-256 characters from [a-zA-Z0-9_.-]")
		}
	}

	// Validate lease_id format
	if leaseID, ok := record["lease_id"].(string); ok && leaseID != "" {
		if !leaseIDPattern.MatchString(leaseID) {
			report.AddErrorWithRemediation(CodeInvalidIDFormat,
				"lease_id must match pattern ^lse_[0-9a-f]{8,64}$",
				"/lease_id",
				"Use format: lse_<8-64 hex chars (0-9a-f)>")
		}
	}
}

func (v *CorrelationValidator) ValidateTelemetryBatch(records []map[string]interface{}) *ValidationReport {
	report := NewValidationReport()

	for i, record := range records {
		recordJSON, err := json.Marshal(record)
		if err != nil {
			continue
		}
		recordReport := v.ValidateOpLog(recordJSON)
		for _, e := range recordReport.Errors {
			e.JSONPointer = fmt.Sprintf("/%d%s", i, e.JSONPointer)
			report.Errors = append(report.Errors, e)
			report.OK = false
		}
	}

	return report
}

func ValidateRunID(runID string) bool {
	return runIDPattern.MatchString(runID)
}

func ValidateExecutionID(execID string) bool {
	return executionIDPattern.MatchString(execID)
}

func ValidateWorkerID(workerID string) bool {
	return workerIDPattern.MatchString(workerID)
}

func ValidateScenarioID(scenarioID string) bool {
	return scenarioIDPattern.MatchString(scenarioID)
}

func ValidateStageID(stageID string) bool {
	return stageIDPattern.MatchString(stageID)
}

func ValidateSessionID(sessionID string) bool {
	return len(sessionID) <= maxSessionIDLength && sessionIDPattern.MatchString(sessionID)
}

func ValidateVUID(vuID string) bool {
	return vuIDPattern.MatchString(vuID)
}

func ValidateStage(stage string) bool {
	return allowedStages[stage]
}

func ValidateLeaseID(leaseID string) bool {
	return leaseIDPattern.MatchString(leaseID)
}

func ValidateEventID(eventID string) bool {
	return eventIDPattern.MatchString(eventID)
}
