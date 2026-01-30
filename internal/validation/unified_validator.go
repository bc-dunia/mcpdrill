package validation

import "encoding/json"

type UnifiedValidator struct {
	schemaValidator   *SchemaValidator
	ssrfValidator     *SSRFValidator
	semanticValidator *SemanticValidator
	systemPolicy      *SystemPolicy
	// Note: DNS rebinding protection is enforced at runtime in the transport layer's safeDialer,
	// not at config validation time, since DNS can change between validation and execution.
}

func NewUnifiedValidator(systemPolicy *SystemPolicy) (*UnifiedValidator, error) {
	if systemPolicy == nil {
		systemPolicy = DefaultSystemPolicy()
	}

	schemaValidator, err := NewSchemaValidator()
	if err != nil {
		return nil, err
	}

	return &UnifiedValidator{
		schemaValidator:   schemaValidator,
		ssrfValidator:     NewSSRFValidator(systemPolicy.AllowPrivateNetworks),
		semanticValidator: NewSemanticValidator(systemPolicy),
		systemPolicy:      systemPolicy,
	}, nil
}

func (v *UnifiedValidator) ValidateRunConfig(data []byte) *ValidationReport {
	report := NewValidationReport()

	schemaReport := v.schemaValidator.ValidateRunConfig(data)
	report.Merge(schemaReport)

	if !schemaReport.OK {
		return report
	}

	ssrfReport := v.ssrfValidator.Validate(data)
	report.Merge(ssrfReport)

	if !ssrfReport.OK {
		return report
	}

	var configMap map[string]interface{}
	if err := json.Unmarshal(data, &configMap); err == nil {
		redirectReport := NewValidationReport()
		v.ssrfValidator.ValidateRedirectPolicy(configMap, redirectReport)
		report.Merge(redirectReport)
	}

	semanticReport := v.semanticValidator.Validate(data)
	report.Merge(semanticReport)

	return report
}

func (v *UnifiedValidator) ValidateOpLog(data []byte) *ValidationReport {
	report := NewValidationReport()

	schemaReport := v.schemaValidator.ValidateOpLog(data)
	report.Merge(schemaReport)

	if !schemaReport.OK {
		return report
	}

	correlationValidator := NewCorrelationValidator()
	correlationReport := correlationValidator.ValidateOpLog(data)
	report.Merge(correlationReport)

	return report
}

func (v *UnifiedValidator) ValidateEvent(data []byte) *ValidationReport {
	report := v.schemaValidator.ValidateEvent(data)
	if !report.OK {
		return report
	}

	// Validate event_id format after schema validation
	var event map[string]interface{}
	if err := json.Unmarshal(data, &event); err == nil {
		if eventID, ok := event["event_id"].(string); ok && eventID != "" {
			if !ValidateEventID(eventID) {
				report.AddErrorWithRemediation(CodeInvalidIDFormat,
					"event_id must match pattern ^evt_[0-9a-f]{8,64}$",
					"/event_id",
					"Use format: evt_<8-64 hex chars (0-9a-f)>")
			}
		}
	}

	return report
}

func (v *UnifiedValidator) ValidateReport(data []byte) *ValidationReport {
	return v.schemaValidator.ValidateReport(data)
}

func (v *UnifiedValidator) GetEffectiveLimits(runConfig map[string]interface{}) *EffectiveLimits {
	return ComputeEffectiveLimits(runConfig, v.systemPolicy)
}

func (v *UnifiedValidator) ValidateWithEffectiveLimits(data []byte) (*ValidationReport, *EffectiveLimits) {
	report := v.ValidateRunConfig(data)

	var config map[string]interface{}
	if err := parseJSON(data, &config); err != nil {
		return report, nil
	}

	limits := v.GetEffectiveLimits(config)
	return report, limits
}

func parseJSON(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
