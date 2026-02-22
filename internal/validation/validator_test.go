package validation

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestValidationReport(t *testing.T) {
	t.Run("NewValidationReport starts OK", func(t *testing.T) {
		r := NewValidationReport()
		if !r.OK {
			t.Error("Expected OK to be true")
		}
		if len(r.Errors) != 0 {
			t.Error("Expected no errors")
		}
	})

	t.Run("AddError sets OK to false", func(t *testing.T) {
		r := NewValidationReport()
		r.AddError("TEST_CODE", "test message", "/test/path")
		if r.OK {
			t.Error("Expected OK to be false after adding error")
		}
		if len(r.Errors) != 1 {
			t.Errorf("Expected 1 error, got %d", len(r.Errors))
		}
		if r.Errors[0].Code != "TEST_CODE" {
			t.Errorf("Expected code TEST_CODE, got %s", r.Errors[0].Code)
		}
	})

	t.Run("AddWarning keeps OK true", func(t *testing.T) {
		r := NewValidationReport()
		r.AddWarning("WARN_CODE", "warning message", "/warn/path")
		if !r.OK {
			t.Error("Expected OK to remain true after adding warning")
		}
		if len(r.Warnings) != 1 {
			t.Errorf("Expected 1 warning, got %d", len(r.Warnings))
		}
	})

	t.Run("Merge combines reports", func(t *testing.T) {
		r1 := NewValidationReport()
		r1.AddError("ERR1", "error 1", "/path1")

		r2 := NewValidationReport()
		r2.AddError("ERR2", "error 2", "/path2")
		r2.AddWarning("WARN1", "warning 1", "/path3")

		r1.Merge(r2)
		if len(r1.Errors) != 2 {
			t.Errorf("Expected 2 errors after merge, got %d", len(r1.Errors))
		}
		if len(r1.Warnings) != 1 {
			t.Errorf("Expected 1 warning after merge, got %d", len(r1.Warnings))
		}
	})
}

func TestNewValidationError(t *testing.T) {
	report := NewValidationReport()
	report.AddError("TEST_CODE", "test message", "/test/path")

	envelope := NewValidationError(report)
	if envelope.Error.ErrorType != ErrorTypeInvalidArgument {
		t.Errorf("Expected error_type %s, got %s", ErrorTypeInvalidArgument, envelope.Error.ErrorType)
	}
	if envelope.Error.ErrorCode != "VALIDATION_FAILED" {
		t.Errorf("Expected error_code VALIDATION_FAILED, got %s", envelope.Error.ErrorCode)
	}

	issues, ok := envelope.Error.Details["issues"].([]map[string]interface{})
	if !ok {
		t.Fatal("Expected issues in details")
	}
	if len(issues) != 1 {
		t.Errorf("Expected 1 issue, got %d", len(issues))
	}
}

func TestSemanticValidator_StagesRequired(t *testing.T) {
	v := NewSemanticValidator(DefaultSystemPolicy())

	t.Run("rejects config with no stages", func(t *testing.T) {
		config := map[string]interface{}{
			"stages": []interface{}{},
		}
		data, _ := json.Marshal(config)
		report := v.Validate(data)
		if report.OK {
			t.Error("Expected validation to fail for empty stages")
		}
		hasCode := false
		for _, e := range report.Errors {
			if e.Code == CodeStagesRequired {
				hasCode = true
				break
			}
		}
		if !hasCode {
			t.Error("Expected STAGES_REQUIRED error code")
		}
	})

	t.Run("rejects config with all stages disabled", func(t *testing.T) {
		config := map[string]interface{}{
			"stages": []interface{}{
				map[string]interface{}{"stage": "preflight", "enabled": false},
			},
		}
		data, _ := json.Marshal(config)
		report := v.Validate(data)
		if report.OK {
			t.Error("Expected validation to fail for all disabled stages")
		}
	})
}

func TestSemanticValidator_RequiredStagesPresent(t *testing.T) {
	v := NewSemanticValidator(DefaultSystemPolicy())

	t.Run("rejects config missing preflight", func(t *testing.T) {
		config := map[string]interface{}{
			"stages": []interface{}{
				map[string]interface{}{"stage": "baseline", "enabled": true, "duration_ms": 60000.0, "load": map[string]interface{}{"target_vus": 10.0, "target_rps": 10.0}, "stop_conditions": []interface{}{map[string]interface{}{"id": "sc", "metric": "error_rate", "comparator": ">", "threshold": 0.05, "window_ms": 30000.0, "sustain_windows": 2.0, "scope": map[string]interface{}{}}}},
				map[string]interface{}{"stage": "ramp", "enabled": true, "duration_ms": 60000.0, "load": map[string]interface{}{"target_vus": 10.0, "target_rps": 10.0}, "stop_conditions": []interface{}{map[string]interface{}{"id": "sc", "metric": "error_rate", "comparator": ">", "threshold": 0.05, "window_ms": 30000.0, "sustain_windows": 2.0, "scope": map[string]interface{}{}}}},
			},
		}
		data, _ := json.Marshal(config)
		report := v.Validate(data)
		hasCode := false
		for _, e := range report.Errors {
			if e.Code == CodePreflightRequired {
				hasCode = true
				break
			}
		}
		if !hasCode {
			t.Error("Expected PREFLIGHT_REQUIRED error code")
		}
	})
}

func TestSemanticValidator_CapsRequired(t *testing.T) {
	v := NewSemanticValidator(DefaultSystemPolicy())

	t.Run("rejects config without hard_caps", func(t *testing.T) {
		config := map[string]interface{}{
			"safety": map[string]interface{}{},
		}
		data, _ := json.Marshal(config)
		report := v.Validate(data)
		hasCode := false
		for _, e := range report.Errors {
			if e.Code == CodeCapsRequired {
				hasCode = true
				break
			}
		}
		if !hasCode {
			t.Error("Expected CAPS_REQUIRED error code")
		}
	})
}

func TestSemanticValidator_AllowlistRequired(t *testing.T) {
	v := NewSemanticValidator(DefaultSystemPolicy())

	t.Run("rejects config with allow_all mode", func(t *testing.T) {
		config := map[string]interface{}{
			"environment": map[string]interface{}{
				"allowlist": map[string]interface{}{
					"mode": "allow_all",
				},
			},
		}
		data, _ := json.Marshal(config)
		report := v.Validate(data)
		hasCode := false
		for _, e := range report.Errors {
			if e.Code == CodeAllowlistRequired {
				hasCode = true
				break
			}
		}
		if !hasCode {
			t.Error("Expected ALLOWLIST_REQUIRED error code")
		}
	})
}

func TestSemanticValidator_CapsConsistent(t *testing.T) {
	v := NewSemanticValidator(DefaultSystemPolicy())

	t.Run("rejects in_flight_per_vu exceeding cap", func(t *testing.T) {
		config := map[string]interface{}{
			"workload": map[string]interface{}{
				"in_flight_per_vu": 10.0,
			},
			"safety": map[string]interface{}{
				"hard_caps": map[string]interface{}{
					"max_in_flight_per_vu": 5.0,
				},
			},
		}
		data, _ := json.Marshal(config)
		report := v.Validate(data)
		hasCode := false
		for _, e := range report.Errors {
			if e.Code == CodeCapsInconsistent {
				hasCode = true
				break
			}
		}
		if !hasCode {
			t.Error("Expected CAPS_INCONSISTENT error code")
		}
	})
}

func TestSSRFValidator(t *testing.T) {
	v := NewSSRFValidator(nil)

	t.Run("rejects IP literal", func(t *testing.T) {
		config := map[string]interface{}{
			"target": map[string]interface{}{
				"url": "http://10.0.0.1/mcp",
			},
		}
		data, _ := json.Marshal(config)
		report := v.Validate(data)
		hasCode := false
		for _, e := range report.Errors {
			if e.Code == CodeIPLiteralBlocked {
				hasCode = true
				break
			}
		}
		if !hasCode {
			t.Error("Expected IP_LITERAL_BLOCKED error code")
		}
	})

	t.Run("rejects localhost", func(t *testing.T) {
		config := map[string]interface{}{
			"target": map[string]interface{}{
				"url": "http://localhost:8080/mcp",
			},
		}
		data, _ := json.Marshal(config)
		report := v.Validate(data)
		hasCode := false
		for _, e := range report.Errors {
			if e.Code == CodeLocalhostBlocked {
				hasCode = true
				break
			}
		}
		if !hasCode {
			t.Error("Expected LOCALHOST_BLOCKED error code")
		}
	})

	t.Run("rejects loopback IP", func(t *testing.T) {
		config := map[string]interface{}{
			"target": map[string]interface{}{
				"url": "http://127.0.0.1/mcp",
			},
		}
		data, _ := json.Marshal(config)
		report := v.Validate(data)
		hasCode := false
		for _, e := range report.Errors {
			if e.Code == CodeLoopbackBlocked {
				hasCode = true
				break
			}
		}
		if !hasCode {
			t.Error("Expected LOOPBACK_BLOCKED error code")
		}
	})

	t.Run("rejects metadata IP", func(t *testing.T) {
		config := map[string]interface{}{
			"target": map[string]interface{}{
				"url": "http://169.254.169.254/latest/meta-data",
			},
		}
		data, _ := json.Marshal(config)
		report := v.Validate(data)
		hasCode := false
		for _, e := range report.Errors {
			if e.Code == CodeMetadataIPBlocked {
				hasCode = true
				break
			}
		}
		if !hasCode {
			t.Error("Expected METADATA_IP_BLOCKED error code")
		}
	})

	t.Run("rejects file:// scheme", func(t *testing.T) {
		config := map[string]interface{}{
			"target": map[string]interface{}{
				"url": "file:///etc/passwd",
			},
		}
		data, _ := json.Marshal(config)
		report := v.Validate(data)
		hasCode := false
		for _, e := range report.Errors {
			if e.Code == CodeInvalidURLScheme {
				hasCode = true
				break
			}
		}
		if !hasCode {
			t.Error("Expected INVALID_URL_SCHEME error code")
		}
	})

	t.Run("rejects URL with userinfo", func(t *testing.T) {
		config := map[string]interface{}{
			"target": map[string]interface{}{
				"url": "http://user:pass@example.com/mcp",
			},
		}
		data, _ := json.Marshal(config)
		report := v.Validate(data)
		hasCode := false
		for _, e := range report.Errors {
			if e.Code == CodeUserInfoBlocked {
				hasCode = true
				break
			}
		}
		if !hasCode {
			t.Error("Expected USERINFO_BLOCKED error code")
		}
	})

	t.Run("allows private network when configured", func(t *testing.T) {
		vWithPrivate := NewSSRFValidator([]string{"10.100.0.0/16"})
		config := map[string]interface{}{
			"target": map[string]interface{}{
				"url": "http://10.100.1.1/mcp",
			},
		}
		data, _ := json.Marshal(config)
		report := vWithPrivate.Validate(data)
		hasPrivateBlocked := false
		for _, e := range report.Errors {
			if e.Code == CodePrivateAddressBlocked {
				hasPrivateBlocked = true
				break
			}
		}
		if hasPrivateBlocked {
			t.Error("Should not block private address when explicitly allowed")
		}
	})

	t.Run("allows localhost when loopback range explicitly configured", func(t *testing.T) {
		vWithLoopback := NewSSRFValidator([]string{"127.0.0.0/8"})
		config := map[string]interface{}{
			"target": map[string]interface{}{
				"url": "http://localhost:8080/mcp",
			},
		}
		data, _ := json.Marshal(config)
		report := vWithLoopback.Validate(data)
		hasLocalhostBlocked := false
		for _, e := range report.Errors {
			if e.Code == CodeLocalhostBlocked {
				hasLocalhostBlocked = true
				break
			}
		}
		if hasLocalhostBlocked {
			t.Error("localhost should be allowed when loopback range is explicitly allowed")
		}
	})

	t.Run("allows localhost when ipv6 loopback is explicitly configured", func(t *testing.T) {
		vWithLoopback := NewSSRFValidator([]string{"::1/128"})
		config := map[string]interface{}{
			"target": map[string]interface{}{
				"url": "http://localhost:8080/mcp",
			},
		}
		data, _ := json.Marshal(config)
		report := vWithLoopback.Validate(data)
		hasLocalhostBlocked := false
		for _, e := range report.Errors {
			if e.Code == CodeLocalhostBlocked {
				hasLocalhostBlocked = true
				break
			}
		}
		if hasLocalhostBlocked {
			t.Error("localhost should be allowed when ipv6 loopback range is explicitly allowed")
		}
	})
}

func TestRoundTrip(t *testing.T) {
	validFixtures, err := filepath.Glob("../../testdata/fixtures/valid/*.json")
	if err != nil {
		t.Fatalf("Failed to glob valid fixtures: %v", err)
	}

	for _, fixture := range validFixtures {
		t.Run(filepath.Base(fixture), func(t *testing.T) {
			data, err := os.ReadFile(fixture)
			if err != nil {
				t.Fatalf("Failed to read fixture: %v", err)
			}

			var original map[string]interface{}
			if err := json.Unmarshal(data, &original); err != nil {
				t.Fatalf("Failed to parse original: %v", err)
			}

			serialized, err := json.Marshal(original)
			if err != nil {
				t.Fatalf("Failed to serialize: %v", err)
			}

			var reparsed map[string]interface{}
			if err := json.Unmarshal(serialized, &reparsed); err != nil {
				t.Fatalf("Failed to reparse: %v", err)
			}

			originalJSON, _ := json.Marshal(original)
			reparsedJSON, _ := json.Marshal(reparsed)
			if string(originalJSON) != string(reparsedJSON) {
				t.Error("Round-trip serialization produced different result")
			}
		})
	}
}

func TestValidFixtures(t *testing.T) {
	validFixtures, err := filepath.Glob("../../testdata/fixtures/valid/*.json")
	if err != nil {
		t.Fatalf("Failed to glob valid fixtures: %v", err)
	}

	if len(validFixtures) == 0 {
		t.Skip("No valid fixtures found")
	}

	semanticValidator := NewSemanticValidator(DefaultSystemPolicy())
	ssrfValidator := NewSSRFValidator(nil)

	for _, fixture := range validFixtures {
		t.Run(filepath.Base(fixture), func(t *testing.T) {
			data, err := os.ReadFile(fixture)
			if err != nil {
				t.Fatalf("Failed to read fixture: %v", err)
			}

			semanticReport := semanticValidator.Validate(data)
			if !semanticReport.OK {
				t.Errorf("Valid fixture failed semantic validation: %s", semanticReport.String())
			}

			ssrfReport := ssrfValidator.Validate(data)
			if !ssrfReport.OK {
				t.Errorf("Valid fixture failed SSRF validation: %s", ssrfReport.String())
			}
		})
	}
}

func TestInvalidFixtures(t *testing.T) {
	invalidFixtures, err := filepath.Glob("../../testdata/fixtures/invalid/*.json")
	if err != nil {
		t.Fatalf("Failed to glob invalid fixtures: %v", err)
	}

	if len(invalidFixtures) == 0 {
		t.Skip("No invalid fixtures found")
	}

	semanticValidator := NewSemanticValidator(DefaultSystemPolicy())
	ssrfValidator := NewSSRFValidator(nil)

	expectedCodes := map[string][]string{
		"missing_caps.json":                      {CodeCapsRequired},
		"missing_allowlist.json":                 {CodeAllowlistRequired},
		"missing_stop_conditions.json":           {CodeStopConditionsRequired},
		"missing_stop_conditions_baseline.json":  {CodeStopConditionsRequired},
		"missing_stop_conditions_ramp.json":      {CodeStopConditionsRequired},
		"in_flight_exceeds_cap.json":             {CodeCapsInconsistent},
		"streaming_without_stall_condition.json": {CodeStreamingGuardrails},
		"ssrf_ip_literal.json":                   {CodeIPLiteralBlocked},
		"ssrf_localhost.json":                    {CodeLocalhostBlocked},
		"preflight_not_first.json":               {CodePreflightNotFirst},
		"duration_too_short.json":                {CodeDurationInvalid},
		"negative_load.json":                     {CodeLoadInvalid},
		"tools_call_no_templates.json":           {CodeToolsCallRequiresTemplates},
	}

	for _, fixture := range invalidFixtures {
		name := filepath.Base(fixture)
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(fixture)
			if err != nil {
				t.Fatalf("Failed to read fixture: %v", err)
			}

			semanticReport := semanticValidator.Validate(data)
			ssrfReport := ssrfValidator.Validate(data)

			var config map[string]interface{}
			json.Unmarshal(data, &config)
			redirectReport := NewValidationReport()
			ssrfValidator.ValidateRedirectPolicy(config, redirectReport)

			allErrors := append(semanticReport.Errors, ssrfReport.Errors...)
			allErrors = append(allErrors, redirectReport.Errors...)

			if len(allErrors) == 0 {
				t.Error("Expected validation errors for invalid fixture")
				return
			}

			expected, hasExpected := expectedCodes[name]
			if !hasExpected {
				return
			}

			for _, expectedCode := range expected {
				found := false
				for _, e := range allErrors {
					if e.Code == expectedCode {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected error code %s not found in validation errors", expectedCode)
				}
			}
		})
	}
}

func TestCorrelationValidator(t *testing.T) {
	v := NewCorrelationValidator()

	t.Run("rejects missing run_id", func(t *testing.T) {
		record := map[string]interface{}{
			"execution_id": "exe_abc12345",
			"stage":        "baseline",
			"stage_id":     "stg_0000000000000001",
			"worker_id":    "wkr_0123456789abcdef",
			"vu_id":        "vu_1",
			"session_id":   "session123",
		}
		data, _ := json.Marshal(record)
		report := v.ValidateOpLog(data)
		hasCode := false
		for _, e := range report.Errors {
			if e.Code == CodeMissingCorrelationKey && e.JSONPointer == "/run_id" {
				hasCode = true
				break
			}
		}
		if !hasCode {
			t.Error("Expected MISSING_CORRELATION_KEY for run_id")
		}
	})

	t.Run("rejects invalid run_id format", func(t *testing.T) {
		record := map[string]interface{}{
			"run_id":       "invalid_run_id",
			"execution_id": "exe_abc12345",
			"stage":        "baseline",
			"stage_id":     "stg_0000000000000001",
			"worker_id":    "wkr_0123456789abcdef",
			"vu_id":        "vu_1",
			"session_id":   "session123",
		}
		data, _ := json.Marshal(record)
		report := v.ValidateOpLog(data)
		hasCode := false
		for _, e := range report.Errors {
			if e.Code == CodeInvalidIDFormat && e.JSONPointer == "/run_id" {
				hasCode = true
				break
			}
		}
		if !hasCode {
			t.Error("Expected INVALID_ID_FORMAT for run_id")
		}
	})

	t.Run("accepts valid correlation keys", func(t *testing.T) {
		record := map[string]interface{}{
			"run_id":       "run_0000000000000001",
			"execution_id": "exe_abc12345",
			"stage":        "baseline",
			"stage_id":     "stg_0000000000000001",
			"worker_id":    "wkr_0123456789abcdef",
			"vu_id":        "vu_1",
			"session_id":   "session123",
		}
		data, _ := json.Marshal(record)
		report := v.ValidateOpLog(data)
		if !report.OK {
			t.Errorf("Expected valid record to pass: %s", report.String())
		}
	})
}

func TestIDFormatValidation(t *testing.T) {
	t.Run("ValidateRunID", func(t *testing.T) {
		validIDs := []string{
			"run_abc123def456789a",
			"run_0123456789abcdef",
			"run_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		}
		for _, id := range validIDs {
			if !ValidateRunID(id) {
				t.Errorf("Expected %s to be valid run_id", id)
			}
		}

		invalidIDs := []string{
			"run_short",
			"invalid_0123456789abcdef",
			"exe_abc123def456789a",
			"abc123def456ghi7",
		}
		for _, id := range invalidIDs {
			if ValidateRunID(id) {
				t.Errorf("Expected %s to be invalid run_id", id)
			}
		}
	})

	t.Run("ValidateExecutionID", func(t *testing.T) {
		if !ValidateExecutionID("exe_abc12345") {
			t.Error("Expected exe_abc12345 to be valid")
		}
		if ValidateExecutionID("exe_short") {
			t.Error("Expected exe_short to be invalid")
		}
	})

	t.Run("ValidateWorkerID", func(t *testing.T) {
		if !ValidateWorkerID("wkr_0123456789abcdef") {
			t.Error("Expected wkr_0000000000000001 to be valid")
		}
		if ValidateWorkerID("worker01") {
			t.Error("Expected worker01 to be invalid")
		}
	})

	t.Run("ValidateStageID", func(t *testing.T) {
		if !ValidateStageID("stg_0000000000000001") {
			t.Error("Expected stg_0000000000000001 to be valid")
		}
		if ValidateStageID("stg_ab") {
			t.Error("Expected stg_a to be invalid (too short)")
		}
	})

	t.Run("ValidateSessionID", func(t *testing.T) {
		if !ValidateSessionID("any-opaque-string") {
			t.Error("Expected any-opaque-string to be valid")
		}
		longID := make([]byte, 300)
		for i := range longID {
			longID[i] = 'a'
		}
		if ValidateSessionID(string(longID)) {
			t.Error("Expected 300-char session_id to be invalid")
		}
	})
}

func TestPolicyMerge(t *testing.T) {
	t.Run("numeric caps use minimum", func(t *testing.T) {
		systemPolicy := &SystemPolicy{
			GlobalHardCaps: HardCaps{
				MaxVUs:        1000,
				MaxRPS:        2000,
				MaxDurationMs: 3600000,
			},
		}

		runConfig := map[string]interface{}{
			"safety": map[string]interface{}{
				"hard_caps": map[string]interface{}{
					"max_vus":         500.0,
					"max_rps":         3000.0,
					"max_duration_ms": 1800000.0,
				},
			},
		}

		limits := ComputeEffectiveLimits(runConfig, systemPolicy)

		if limits.MaxVUs != 500 {
			t.Errorf("Expected MaxVUs=500, got %d", limits.MaxVUs)
		}
		if limits.MaxRPS != 2000 {
			t.Errorf("Expected MaxRPS=2000 (capped by system), got %f", limits.MaxRPS)
		}
		if limits.MaxDurationMs != 1800000 {
			t.Errorf("Expected MaxDurationMs=1800000, got %d", limits.MaxDurationMs)
		}
	})

	t.Run("identification required is OR", func(t *testing.T) {
		systemPolicy := &SystemPolicy{
			RequireIdentification: true,
			GlobalHardCaps:        HardCaps{MaxVUs: 1000},
		}

		runConfig := map[string]interface{}{
			"safety": map[string]interface{}{
				"identification_required": false,
			},
		}

		limits := ComputeEffectiveLimits(runConfig, systemPolicy)
		if !limits.IdentificationRequired {
			t.Error("Expected IdentificationRequired=true when system policy requires it")
		}
	})

	t.Run("forbidden patterns are union", func(t *testing.T) {
		systemPolicy := &SystemPolicy{
			ForbiddenPatterns: []string{"*.prod.*"},
			GlobalHardCaps:    HardCaps{MaxVUs: 1000},
		}

		runConfig := map[string]interface{}{
			"environment": map[string]interface{}{
				"forbidden_patterns": []interface{}{"*.internal.*"},
			},
		}

		limits := ComputeEffectiveLimits(runConfig, systemPolicy)
		if len(limits.EffectiveForbidden) != 2 {
			t.Errorf("Expected 2 forbidden patterns, got %d", len(limits.EffectiveForbidden))
		}
	})
}

func TestUnifiedValidator(t *testing.T) {
	v, err := NewUnifiedValidator(DefaultSystemPolicy())
	if err != nil {
		t.Fatalf("Failed to create unified validator: %v", err)
	}

	t.Run("validates in correct order", func(t *testing.T) {
		invalidJSON := []byte(`{invalid json}`)
		report := v.ValidateRunConfig(invalidJSON)
		if report.OK {
			t.Error("Expected schema validation to fail for invalid JSON")
		}
		if len(report.Errors) == 0 || report.Errors[0].Code != CodeSchemaViolation {
			t.Error("Expected SCHEMA_VIOLATION as first error")
		}
	})

	t.Run("stops at schema errors", func(t *testing.T) {
		invalidSchema := []byte(`{"schema_version": "wrong/v1"}`)
		report := v.ValidateRunConfig(invalidSchema)
		if report.OK {
			t.Error("Expected validation to fail")
		}
		for _, e := range report.Errors {
			if e.Code == CodeCapsRequired || e.Code == CodeAllowlistRequired {
				t.Error("Should not reach semantic validation when schema fails")
			}
		}
	})
}

func TestSSRFValidator_MaxRedirects(t *testing.T) {
	v := NewSSRFValidator(nil)

	t.Run("rejects max_redirects > 3", func(t *testing.T) {
		config := map[string]interface{}{
			"target": map[string]interface{}{
				"redirect_policy": map[string]interface{}{
					"mode":          "same_origin",
					"max_redirects": 5.0,
				},
			},
		}
		report := NewValidationReport()
		v.ValidateRedirectPolicy(config, report)
		hasCode := false
		for _, e := range report.Errors {
			if e.Code == CodeMaxRedirectsExceeded {
				hasCode = true
				break
			}
		}
		if !hasCode {
			t.Error("Expected MAX_REDIRECTS_EXCEEDED error code")
		}
	})

	t.Run("accepts max_redirects <= 3", func(t *testing.T) {
		config := map[string]interface{}{
			"target": map[string]interface{}{
				"redirect_policy": map[string]interface{}{
					"mode":          "same_origin",
					"max_redirects": 3.0,
				},
			},
		}
		report := NewValidationReport()
		v.ValidateRedirectPolicy(config, report)
		for _, e := range report.Errors {
			if e.Code == CodeMaxRedirectsExceeded {
				t.Error("Should not reject max_redirects=3")
			}
		}
	})
}

func TestSSRFValidator_IPv6(t *testing.T) {
	v := NewSSRFValidator(nil)

	ipv6Tests := []struct {
		name     string
		url      string
		expected string
	}{
		{"loopback", "http://[::1]/mcp", CodeLoopbackBlocked},
		{"unique local", "http://[fc00::1]/mcp", CodeUniqueLocalBlocked},
		{"link local", "http://[fe80::1]/mcp", CodeLinkLocalBlocked},
		{"multicast", "http://[ff00::1]/mcp", CodeMulticastBlocked},
		{"ipv4 mapped", "http://[::ffff:7f00:1]/mcp", CodeLoopbackBlocked},
		{"nat64", "http://[64:ff9b::1]/mcp", CodeNAT64Blocked},
		{"documentation", "http://[2001:db8::1]/mcp", CodeDocumentationIPBlocked},
	}

	for _, tc := range ipv6Tests {
		t.Run(tc.name, func(t *testing.T) {
			config := map[string]interface{}{
				"target": map[string]interface{}{
					"url": tc.url,
				},
			}
			data, _ := json.Marshal(config)
			report := v.Validate(data)
			hasCode := false
			for _, e := range report.Errors {
				if e.Code == tc.expected {
					hasCode = true
					break
				}
			}
			if !hasCode {
				t.Errorf("Expected %s error code for %s", tc.expected, tc.url)
			}
		})
	}
}

func TestDNSRebindingValidator(t *testing.T) {
	v := NewDNSRebindingValidator(nil)

	t.Run("blocks loopback IP", func(t *testing.T) {
		ips := []net.IP{net.ParseIP("127.0.0.1")}
		report := v.ValidateResolvedIPs("example.com", ips)
		if report.OK {
			t.Error("Expected loopback IP to be blocked")
		}
	})

	t.Run("allows public IP", func(t *testing.T) {
		ips := []net.IP{net.ParseIP("8.8.8.8")}
		report := v.ValidateResolvedIPs("example.com", ips)
		if !report.OK {
			t.Errorf("Expected public IP to be allowed: %s", report.String())
		}
	})

	t.Run("caches DNS results", func(t *testing.T) {
		ips := []net.IP{net.ParseIP("8.8.8.8")}
		v.ValidateResolvedIPs("test.com", ips)
		cached, ok := v.cache.Lookup("test.com")
		if !ok {
			t.Error("Expected DNS result to be cached")
		}
		if len(cached) != 1 || !cached[0].Equal(ips[0]) {
			t.Error("Cached IP doesn't match")
		}
	})

	t.Run("cache is not mutated by caller slice changes", func(t *testing.T) {
		ips := []net.IP{net.ParseIP("8.8.8.8")}
		v.ValidateResolvedIPs("immutability-test.com", ips)
		ips[0] = net.ParseIP("1.1.1.1")

		cached, ok := v.cache.Lookup("immutability-test.com")
		if !ok {
			t.Fatal("Expected DNS result to be cached")
		}
		if !cached[0].Equal(net.ParseIP("8.8.8.8")) {
			t.Fatalf("expected cached IP to remain 8.8.8.8, got %s", cached[0].String())
		}
	})

	t.Run("lookup returns immutable copy", func(t *testing.T) {
		ips := []net.IP{net.ParseIP("9.9.9.9")}
		v.ValidateResolvedIPs("lookup-copy-test.com", ips)

		cached, ok := v.cache.Lookup("lookup-copy-test.com")
		if !ok {
			t.Fatal("Expected DNS result to be cached")
		}
		cached[0] = net.ParseIP("4.4.4.4")

		cachedAgain, ok := v.cache.Lookup("lookup-copy-test.com")
		if !ok {
			t.Fatal("Expected DNS result to still be cached")
		}
		if !cachedAgain[0].Equal(net.ParseIP("9.9.9.9")) {
			t.Fatalf("expected cached IP to remain 9.9.9.9, got %s", cachedAgain[0].String())
		}
	})

	t.Run("clears cache", func(t *testing.T) {
		ips := []net.IP{net.ParseIP("8.8.8.8")}
		v.ValidateResolvedIPs("clear-test.com", ips)
		v.ClearCache()
		_, ok := v.cache.Lookup("clear-test.com")
		if ok {
			t.Error("Expected cache to be cleared")
		}
	})
}

func TestValidationReportString(t *testing.T) {
	t.Run("OK report", func(t *testing.T) {
		r := NewValidationReport()
		s := r.String()
		if s != "Validation passed" {
			t.Errorf("Expected 'Validation passed', got %s", s)
		}
	})

	t.Run("report with errors", func(t *testing.T) {
		r := NewValidationReport()
		r.AddError("ERR1", "error message", "/path")
		s := r.String()
		if !r.HasErrors() {
			t.Error("Expected HasErrors to be true")
		}
		if len(s) == 0 {
			t.Error("Expected non-empty string")
		}
	})

	t.Run("report with warnings only", func(t *testing.T) {
		r := NewValidationReport()
		r.AddWarning("WARN1", "warning message", "/path")
		s := r.String()
		if !r.HasWarnings() {
			t.Error("Expected HasWarnings to be true")
		}
		if len(s) == 0 {
			t.Error("Expected non-empty string")
		}
	})
}

func TestValidationErrorFromReport(t *testing.T) {
	t.Run("returns nil for OK report", func(t *testing.T) {
		r := NewValidationReport()
		err := NewValidationErrorFromReport(r)
		if err != nil {
			t.Error("Expected nil error for OK report")
		}
	})

	t.Run("returns error for failed report", func(t *testing.T) {
		r := NewValidationReport()
		r.AddError("ERR1", "error", "/path")
		err := NewValidationErrorFromReport(r)
		if err == nil {
			t.Error("Expected non-nil error")
		}
		ve, ok := err.(*ValidationError)
		if !ok {
			t.Error("Expected ValidationError type")
		}
		if ve.Error() == "" {
			t.Error("Expected non-empty error string")
		}
	})
}

func TestErrorEnvelopeToJSON(t *testing.T) {
	r := NewValidationReport()
	r.AddError("ERR1", "error", "/path")
	envelope := NewValidationError(r)
	data, err := envelope.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}
	if len(data) == 0 {
		t.Error("Expected non-empty JSON")
	}
}

func TestUnifiedValidatorOpLog(t *testing.T) {
	v, err := NewUnifiedValidator(DefaultSystemPolicy())
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	t.Run("rejects op-log missing correlation keys", func(t *testing.T) {
		opLog := map[string]interface{}{
			"schema_version": "op-log/v1",
		}
		data, _ := json.Marshal(opLog)
		report := v.ValidateOpLog(data)
		if report.OK {
			t.Error("Expected validation to fail for missing correlation keys")
		}
	})
}

func TestCorrelationValidatorBatch(t *testing.T) {
	v := NewCorrelationValidator()

	records := []map[string]interface{}{
		{
			"run_id":       "run_0000000000000001",
			"execution_id": "exe_abc12345",
			"stage":        "baseline",
			"stage_id":     "stg_0000000000000001",
			"worker_id":    "wkr_0123456789abcdef",
			"vu_id":        "vu_1",
			"session_id":   "session123",
		},
		{
			"execution_id": "exe_abc12345",
			"stage":        "baseline",
		},
	}

	report := v.ValidateTelemetryBatch(records)
	if report.OK {
		t.Error("Expected batch validation to fail for missing keys in second record")
	}
}

func TestSchemaValidatorMethods(t *testing.T) {
	v, err := NewSchemaValidator()
	if err != nil {
		t.Fatalf("Failed to create schema validator: %v", err)
	}

	t.Run("ValidateEvent", func(t *testing.T) {
		event := map[string]interface{}{
			"schema_version": "event/v1",
			"event_id":       "evt_abc12345",
			"event_type":     "STATE_TRANSITION",
			"ts_ms":          1234567890000,
			"run_id":         "run_abc123def456789a",
			"execution_id":   "exe_abc12345",
		}
		data, _ := json.Marshal(event)
		report := v.ValidateEvent(data)
		if report.OK {
			t.Log("Event validation passed")
		}
	})

	t.Run("ValidateReport", func(t *testing.T) {
		report := map[string]interface{}{
			"schema_version": "report/v1",
			"run_id":         "run_abc123def456789a",
		}
		data, _ := json.Marshal(report)
		result := v.ValidateReport(data)
		if result.OK {
			t.Log("Report validation passed")
		}
	})
}

func TestSemanticValidatorSystemPolicy(t *testing.T) {
	policy := &SystemPolicy{
		GlobalAllowlist: []AllowlistEntry{
			{Kind: "suffix", Value: ".staging.example.com"},
		},
		GlobalHardCaps: HardCaps{
			MaxVUs:        100,
			MaxRPS:        1000,
			MaxDurationMs: 3600000,
		},
		RequireIdentification: true,
	}
	v := NewSemanticValidator(policy)

	t.Run("rejects target not in system allowlist", func(t *testing.T) {
		config := map[string]interface{}{
			"target": map[string]interface{}{
				"url": "https://production.example.com/mcp",
			},
		}
		data, _ := json.Marshal(config)
		report := v.Validate(data)
		hasCode := false
		for _, e := range report.Errors {
			if e.Code == CodeAllowlistViolation {
				hasCode = true
				break
			}
		}
		if !hasCode {
			t.Error("Expected ALLOWLIST_VIOLATION error")
		}
	})

	t.Run("accepts target in system allowlist", func(t *testing.T) {
		config := map[string]interface{}{
			"target": map[string]interface{}{
				"url": "https://api.staging.example.com/mcp",
			},
		}
		data, _ := json.Marshal(config)
		report := v.Validate(data)
		for _, e := range report.Errors {
			if e.Code == CodeAllowlistViolation {
				t.Error("Should not reject target in allowlist")
			}
		}
	})
}

func TestSemanticValidator_AllRules(t *testing.T) {
	v := NewSemanticValidator(DefaultSystemPolicy())

	t.Run("preflight_first", func(t *testing.T) {
		config := map[string]interface{}{
			"stages": []interface{}{
				map[string]interface{}{"stage": "baseline", "enabled": true, "duration_ms": 60000.0},
				map[string]interface{}{"stage": "preflight", "enabled": true, "duration_ms": 60000.0},
			},
		}
		data, _ := json.Marshal(config)
		report := v.Validate(data)
		hasCode := false
		for _, e := range report.Errors {
			if e.Code == CodePreflightNotFirst {
				hasCode = true
				break
			}
		}
		if !hasCode {
			t.Error("Expected PREFLIGHT_NOT_FIRST error")
		}
	})

	t.Run("duration_positive", func(t *testing.T) {
		config := map[string]interface{}{
			"stages": []interface{}{
				map[string]interface{}{"stage": "preflight", "enabled": true, "duration_ms": 500.0},
			},
		}
		data, _ := json.Marshal(config)
		report := v.Validate(data)
		hasCode := false
		for _, e := range report.Errors {
			if e.Code == CodeDurationInvalid {
				hasCode = true
				break
			}
		}
		if !hasCode {
			t.Error("Expected DURATION_INVALID error")
		}
	})

	t.Run("load_nonnegative", func(t *testing.T) {
		config := map[string]interface{}{
			"stages": []interface{}{
				map[string]interface{}{
					"stage":       "preflight",
					"enabled":     true,
					"duration_ms": 60000.0,
					"load": map[string]interface{}{
						"target_vus": -1.0,
					},
				},
			},
		}
		data, _ := json.Marshal(config)
		report := v.Validate(data)
		hasCode := false
		for _, e := range report.Errors {
			if e.Code == CodeLoadInvalid {
				hasCode = true
				break
			}
		}
		if !hasCode {
			t.Error("Expected LOAD_INVALID error")
		}
	})

	t.Run("operation_mix_nonempty", func(t *testing.T) {
		config := map[string]interface{}{
			"workload": map[string]interface{}{
				"operation_mix": []interface{}{},
			},
		}
		data, _ := json.Marshal(config)
		report := v.Validate(data)
		hasCode := false
		for _, e := range report.Errors {
			if e.Code == CodeOperationMixEmpty {
				hasCode = true
				break
			}
		}
		if !hasCode {
			t.Error("Expected OPERATION_MIX_EMPTY error")
		}
	})

	t.Run("tools_call_requires_tools", func(t *testing.T) {
		config := map[string]interface{}{
			"workload": map[string]interface{}{
				"operation_mix": []interface{}{
					map[string]interface{}{"operation": "tools_call", "weight": 1.0},
				},
			},
		}
		data, _ := json.Marshal(config)
		report := v.Validate(data)
		hasCode := false
		for _, e := range report.Errors {
			if e.Code == CodeToolsCallRequiresTemplates {
				hasCode = true
				break
			}
		}
		if !hasCode {
			t.Error("Expected TOOLS_CALL_REQUIRES_TEMPLATES error")
		}
	})

	t.Run("redirect_policy_required", func(t *testing.T) {
		config := map[string]interface{}{
			"target": map[string]interface{}{
				"url": "https://example.com/mcp",
			},
		}
		data, _ := json.Marshal(config)
		report := v.Validate(data)
		hasCode := false
		for _, e := range report.Errors {
			if e.Code == CodeRedirectPolicyRequired {
				hasCode = true
				break
			}
		}
		if !hasCode {
			t.Error("Expected REDIRECT_POLICY_REQUIRED error")
		}
	})

	t.Run("stop_conditions_required_baseline", func(t *testing.T) {
		config := map[string]interface{}{
			"stages": []interface{}{
				map[string]interface{}{"stage": "preflight", "enabled": true, "duration_ms": 60000.0},
				map[string]interface{}{"stage": "baseline", "enabled": true, "duration_ms": 60000.0, "stop_conditions": []interface{}{}},
				map[string]interface{}{"stage": "ramp", "enabled": true, "duration_ms": 60000.0, "stop_conditions": []interface{}{map[string]interface{}{"id": "sc", "metric": "error_rate"}}},
			},
		}
		data, _ := json.Marshal(config)
		report := v.Validate(data)
		hasCode := false
		for _, e := range report.Errors {
			if e.Code == CodeStopConditionsRequired && e.JSONPointer == "/stages/1/stop_conditions" {
				hasCode = true
				break
			}
		}
		if !hasCode {
			t.Error("Expected STOP_CONDITIONS_REQUIRED error for baseline stage")
		}
	})

	t.Run("stop_conditions_required_ramp", func(t *testing.T) {
		config := map[string]interface{}{
			"stages": []interface{}{
				map[string]interface{}{"stage": "preflight", "enabled": true, "duration_ms": 60000.0},
				map[string]interface{}{"stage": "baseline", "enabled": true, "duration_ms": 60000.0, "stop_conditions": []interface{}{map[string]interface{}{"id": "sc", "metric": "error_rate"}}},
				map[string]interface{}{"stage": "ramp", "enabled": true, "duration_ms": 60000.0, "stop_conditions": []interface{}{}},
			},
		}
		data, _ := json.Marshal(config)
		report := v.Validate(data)
		hasCode := false
		for _, e := range report.Errors {
			if e.Code == CodeStopConditionsRequired && e.JSONPointer == "/stages/2/stop_conditions" {
				hasCode = true
				break
			}
		}
		if !hasCode {
			t.Error("Expected STOP_CONDITIONS_REQUIRED error for ramp stage")
		}
	})

	t.Run("stop_conditions_not_required_for_preflight", func(t *testing.T) {
		config := map[string]interface{}{
			"stages": []interface{}{
				map[string]interface{}{"stage": "preflight", "enabled": true, "duration_ms": 60000.0, "stop_conditions": []interface{}{}},
			},
		}
		data, _ := json.Marshal(config)
		report := v.Validate(data)
		for _, e := range report.Errors {
			if e.Code == CodeStopConditionsRequired && e.JSONPointer == "/stages/0/stop_conditions" {
				t.Error("Should not require stop conditions for preflight stage")
			}
		}
	})

	t.Run("worker_failure_policy_valid", func(t *testing.T) {
		validPolicies := []string{"fail_fast", "replace_if_possible", "best_effort"}
		for _, policy := range validPolicies {
			config := map[string]interface{}{
				"safety": map[string]interface{}{
					"worker_failure_policy": policy,
				},
			}
			data, _ := json.Marshal(config)
			report := v.Validate(data)
			for _, e := range report.Errors {
				if e.Code == CodeInvalidWorkerFailurePolicy {
					t.Errorf("Should accept valid worker_failure_policy: %s", policy)
				}
			}
		}
	})

	t.Run("worker_failure_policy_invalid", func(t *testing.T) {
		config := map[string]interface{}{
			"safety": map[string]interface{}{
				"worker_failure_policy": "invalid_policy",
			},
		}
		data, _ := json.Marshal(config)
		report := v.Validate(data)
		hasCode := false
		for _, e := range report.Errors {
			if e.Code == CodeInvalidWorkerFailurePolicy {
				hasCode = true
				break
			}
		}
		if !hasCode {
			t.Error("Expected INVALID_WORKER_FAILURE_POLICY error")
		}
	})

	t.Run("worker_failure_policy_not_required", func(t *testing.T) {
		config := map[string]interface{}{
			"safety": map[string]interface{}{
				"hard_caps": map[string]interface{}{
					"max_vus":         100.0,
					"max_duration_ms": 3600000.0,
				},
			},
		}
		data, _ := json.Marshal(config)
		report := v.Validate(data)
		for _, e := range report.Errors {
			if e.Code == CodeInvalidWorkerFailurePolicy {
				t.Error("Should not require worker_failure_policy")
			}
		}
	})
}
