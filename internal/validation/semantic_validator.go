package validation

import (
	"net/url"
	"path"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

var stageIDPatternSemantic = regexp.MustCompile(`^stg_[0-9a-f]{3,81}$`)

type SystemPolicy struct {
	AllowedSecretRefs     []string         `json:"allowed_secret_refs"`
	GlobalAllowlist       []AllowlistEntry `json:"global_allowlist"`
	GlobalHardCaps        HardCaps         `json:"global_hard_caps"`
	ForbiddenPatterns     []string         `json:"forbidden_patterns"`
	RequireIdentification bool             `json:"require_identification"`
	AllowPrivateNetworks  []string         `json:"allow_private_networks"`
}

type AllowlistEntry struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type HardCaps struct {
	MaxVUs             int     `json:"max_vus"`
	MaxRPS             float64 `json:"max_rps"`
	MaxConnections     int     `json:"max_connections"`
	MaxDurationMs      int     `json:"max_duration_ms"`
	MaxInFlightPerVU   int     `json:"max_in_flight_per_vu"`
	MaxTelemetryQDepth int     `json:"max_telemetry_q_depth"`
}

type SemanticValidator struct {
	systemPolicy *SystemPolicy
}

func NewSemanticValidator(policy *SystemPolicy) *SemanticValidator {
	return &SemanticValidator{
		systemPolicy: policy,
	}
}

func DefaultSystemPolicy() *SystemPolicy {
	return &SystemPolicy{
		AllowedSecretRefs: []string{"env://MCPDRILL_*", "file:///run/secrets/mcpdrill/*"},
		GlobalAllowlist:   []AllowlistEntry{},
		GlobalHardCaps: HardCaps{
			MaxVUs:             10000,
			MaxRPS:             100000,
			MaxConnections:     100000,
			MaxDurationMs:      86400000,
			MaxInFlightPerVU:   100,
			MaxTelemetryQDepth: 1000000,
		},
		ForbiddenPatterns:     []string{},
		RequireIdentification: true,
		AllowPrivateNetworks:  []string{},
	}
}

func (v *SemanticValidator) Validate(data []byte) *ValidationReport {
	report := NewValidationReport()

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		report.AddError(CodeSchemaViolation, "Invalid JSON", "")
		return report
	}

	v.validateStagesRequired(config, report)
	v.validateRequiredStagesPresent(config, report)
	v.validatePreflightFirst(config, report)
	v.validateDurationPositive(config, report)
	v.validateLoadNonnegative(config, report)
	v.validateOperationMixNonempty(config, report)
	v.validateToolsCallRequiresTools(config, report)
	v.validateCapsRequired(config, report)
	v.validateCapsConsistent(config, report)
	v.validateCapsWithinSystemPolicy(config, report)
	v.validateAllowlistRequired(config, report)
	v.validateTargetWithinSystemAllowlist(config, report)
	v.validateSecretRefsAllowed(config, report)
	v.validateIdentificationRequired(config, report)
	v.validateRampByDefaultGuard(config, report)
	v.validateStopConditionsRequired(config, report)
	v.validateStreamingGuardrails(config, report)
	v.validateRedirectPolicyRequired(config, report)
	v.validateWorkerFailurePolicy(config, report)
	v.validateChurnIntervalOps(config, report)
	v.validateTargetWithinRunAllowlist(config, report)
	v.validateForbiddenPatterns(config, report)
	v.validateStageIDFormats(config, report)

	return report
}


func (v *SemanticValidator) validateStageIDFormats(config map[string]interface{}, report *ValidationReport) {
	stages, ok := config["stages"].([]interface{})
	if !ok {
		return
	}

	for i, s := range stages {
		stage, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		stageID, ok := stage["stage_id"].(string)
		if !ok || stageID == "" {
			// stage_id can be null/missing, only validate non-empty strings
			continue
		}
		if !stageIDPatternSemantic.MatchString(stageID) {
			report.AddErrorWithRemediation(CodeInvalidIDFormat,
				"stage_id must match pattern ^stg_[0-9a-f]{3,81}$",
				"/stages/"+strconv.Itoa(i)+"/stage_id",
				"Use format: stg_<3-81 hex chars (0-9a-f)>")
		}
	}
}
func (v *SemanticValidator) validateStagesRequired(config map[string]interface{}, report *ValidationReport) {
	stages, ok := config["stages"].([]interface{})
	if !ok || len(stages) == 0 {
		report.AddError(CodeStagesRequired, "At least one stage must be defined", "/stages")
		return
	}

	hasEnabled := false
	for _, s := range stages {
		stage, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		if enabled, ok := stage["enabled"].(bool); ok && enabled {
			hasEnabled = true
			break
		}
	}

	if !hasEnabled {
		report.AddError(CodeStagesRequired, "At least one stage must be enabled", "/stages")
	}
}

func (v *SemanticValidator) validateRequiredStagesPresent(config map[string]interface{}, report *ValidationReport) {
	stages, ok := config["stages"].([]interface{})
	if !ok {
		return
	}

	hasPreflight := false
	hasBaseline := false
	hasRamp := false

	for _, s := range stages {
		stage, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		enabled, _ := stage["enabled"].(bool)
		if !enabled {
			continue
		}
		stageType, _ := stage["stage"].(string)
		switch stageType {
		case "preflight":
			hasPreflight = true
		case "baseline":
			hasBaseline = true
		case "ramp":
			hasRamp = true
		}
	}

	if !hasPreflight {
		report.AddError(CodePreflightRequired, "Preflight stage must be enabled for MVP", "/stages")
	}
	if !hasBaseline {
		report.AddError(CodeBaselineRequired, "Baseline stage must be enabled for MVP", "/stages")
	}
	if !hasRamp {
		report.AddError(CodeRampRequired, "Ramp stage must be enabled for MVP", "/stages")
	}
}

func (v *SemanticValidator) validatePreflightFirst(config map[string]interface{}, report *ValidationReport) {
	stages, ok := config["stages"].([]interface{})
	if !ok || len(stages) == 0 {
		return
	}

	for i, s := range stages {
		stage, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		enabled, _ := stage["enabled"].(bool)
		if !enabled {
			continue
		}
		stageType, _ := stage["stage"].(string)
		if stageType != "preflight" {
			report.AddErrorWithRemediation(CodePreflightNotFirst,
				"First enabled stage must be preflight",
				"/stages/"+strconv.Itoa(i),
				"Move preflight stage to be the first enabled stage")
		}
		return
	}
}

func (v *SemanticValidator) validateDurationPositive(config map[string]interface{}, report *ValidationReport) {
	stages, ok := config["stages"].([]interface{})
	if !ok {
		return
	}

	for i, s := range stages {
		stage, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		enabled, _ := stage["enabled"].(bool)
		if !enabled {
			continue
		}
		durationMs, ok := stage["duration_ms"].(float64)
		if !ok {
			continue
		}
		if durationMs < 1000 {
			report.AddError(CodeDurationInvalid,
				"Enabled stages must have duration_ms >= 1000",
				"/stages/"+strconv.Itoa(i)+"/duration_ms")
		}
	}
}

func (v *SemanticValidator) validateLoadNonnegative(config map[string]interface{}, report *ValidationReport) {
	stages, ok := config["stages"].([]interface{})
	if !ok {
		return
	}

	for i, s := range stages {
		stage, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		enabled, _ := stage["enabled"].(bool)
		if !enabled {
			continue
		}
		load, ok := stage["load"].(map[string]interface{})
		if !ok {
			continue
		}
		if targetVUs, ok := load["target_vus"].(float64); ok && targetVUs < 0 {
			report.AddError(CodeLoadInvalid,
				"target_vus must be >= 0",
				"/stages/"+strconv.Itoa(i)+"/load/target_vus")
		}
		if targetRPS, ok := load["target_rps"].(float64); ok && targetRPS < 0 {
			report.AddError(CodeLoadInvalid,
				"target_rps must be >= 0",
				"/stages/"+strconv.Itoa(i)+"/load/target_rps")
		}
	}
}

func (v *SemanticValidator) validateOperationMixNonempty(config map[string]interface{}, report *ValidationReport) {
	workload, ok := config["workload"].(map[string]interface{})
	if !ok {
		return
	}

	opMix, opMixOk := workload["operation_mix"].([]interface{})
	altOpMix, altOpMixOk := workload["op_mix"].([]interface{})

	if !opMixOk && altOpMixOk {
		opMix = altOpMix
		opMixOk = true
	}

	if !opMixOk || len(opMix) == 0 {
		report.AddError(CodeOperationMixEmpty,
			"At least one operation must be defined in operation_mix or op_mix",
			"/workload/operation_mix")
		return
	}

	totalWeight := 0.0
	for _, op := range opMix {
		opMap, ok := op.(map[string]interface{})
		if !ok {
			continue
		}
		if weight, ok := opMap["weight"].(float64); ok {
			totalWeight += weight
		}
	}

	if totalWeight <= 0 {
		report.AddError(CodeOperationMixEmpty,
			"Total weight of operations must be > 0",
			"/workload/operation_mix")
	}
}

func (v *SemanticValidator) validateToolsCallRequiresTools(config map[string]interface{}, report *ValidationReport) {
	workload, ok := config["workload"].(map[string]interface{})
	if !ok {
		return
	}

	hasToolsCall := false
	opMix, ok := workload["operation_mix"].([]interface{})
	if !ok {
		opMix, ok = workload["op_mix"].([]interface{})
	}
	if ok {
		for _, op := range opMix {
			opMap, ok := op.(map[string]interface{})
			if !ok {
				continue
			}
			if operation, ok := opMap["operation"].(string); ok {
				if operation == "tools_call" || operation == "tools/call" {
					hasToolsCall = true
					break
				}
			}
		}
	}

	if !hasToolsCall {
		return
	}

	tools, ok := workload["tools"].(map[string]interface{})
	if !ok {
		report.AddError(CodeToolsCallRequiresTemplates,
			"tools_call operation requires tools.templates to be defined",
			"/workload/tools")
		return
	}

	templates, ok := tools["templates"].([]interface{})
	if !ok || len(templates) == 0 {
		report.AddError(CodeToolsCallRequiresTemplates,
			"tools_call operation requires at least one tool template",
			"/workload/tools/templates")
	}
}

func (v *SemanticValidator) validateCapsRequired(config map[string]interface{}, report *ValidationReport) {
	safety, ok := config["safety"].(map[string]interface{})
	if !ok {
		report.AddError(CodeCapsRequired, "safety section is required", "/safety")
		return
	}

	hardCaps, ok := safety["hard_caps"].(map[string]interface{})
	if !ok {
		report.AddError(CodeCapsRequired, "safety.hard_caps is required", "/safety/hard_caps")
		return
	}

	if maxVUs, ok := hardCaps["max_vus"].(float64); !ok || maxVUs <= 0 {
		report.AddError(CodeCapsRequired,
			"safety.hard_caps.max_vus must be set and > 0",
			"/safety/hard_caps/max_vus")
	}

	if maxDuration, ok := hardCaps["max_duration_ms"].(float64); !ok || maxDuration <= 0 {
		report.AddError(CodeCapsRequired,
			"safety.hard_caps.max_duration_ms must be set and > 0",
			"/safety/hard_caps/max_duration_ms")
	}
}

func (v *SemanticValidator) validateCapsConsistent(config map[string]interface{}, report *ValidationReport) {
	workload, ok := config["workload"].(map[string]interface{})
	if !ok {
		return
	}
	safety, ok := config["safety"].(map[string]interface{})
	if !ok {
		return
	}
	hardCaps, ok := safety["hard_caps"].(map[string]interface{})
	if !ok {
		return
	}

	inFlightPerVU, ok := workload["in_flight_per_vu"].(float64)
	if !ok {
		return
	}
	maxInFlightPerVU, ok := hardCaps["max_in_flight_per_vu"].(float64)
	if !ok {
		return
	}

	if inFlightPerVU > maxInFlightPerVU {
		report.AddError(CodeCapsInconsistent,
			"workload.in_flight_per_vu exceeds safety.hard_caps.max_in_flight_per_vu",
			"/workload/in_flight_per_vu")
	}
}

func (v *SemanticValidator) validateCapsWithinSystemPolicy(config map[string]interface{}, report *ValidationReport) {
	if v.systemPolicy == nil {
		return
	}

	safety, ok := config["safety"].(map[string]interface{})
	if !ok {
		return
	}
	hardCaps, ok := safety["hard_caps"].(map[string]interface{})
	if !ok {
		return
	}

	if maxVUs, ok := hardCaps["max_vus"].(float64); ok {
		if int(maxVUs) > v.systemPolicy.GlobalHardCaps.MaxVUs {
			report.AddError(CodeSystemPolicyViolation,
				"safety.hard_caps.max_vus exceeds system policy limit",
				"/safety/hard_caps/max_vus")
		}
	}

	if maxRPS, ok := hardCaps["max_rps"].(float64); ok {
		if maxRPS > v.systemPolicy.GlobalHardCaps.MaxRPS {
			report.AddError(CodeSystemPolicyViolation,
				"safety.hard_caps.max_rps exceeds system policy limit",
				"/safety/hard_caps/max_rps")
		}
	}

	if maxDuration, ok := hardCaps["max_duration_ms"].(float64); ok {
		if int(maxDuration) > v.systemPolicy.GlobalHardCaps.MaxDurationMs {
			report.AddError(CodeSystemPolicyViolation,
				"safety.hard_caps.max_duration_ms exceeds system policy limit",
				"/safety/hard_caps/max_duration_ms")
		}
	}
}

func (v *SemanticValidator) validateAllowlistRequired(config map[string]interface{}, report *ValidationReport) {
	env, ok := config["environment"].(map[string]interface{})
	if !ok {
		report.AddError(CodeAllowlistRequired, "environment section is required", "/environment")
		return
	}

	allowlist, ok := env["allowlist"].(map[string]interface{})
	if !ok {
		report.AddError(CodeAllowlistRequired, "environment.allowlist is required", "/environment/allowlist")
		return
	}

	mode, ok := allowlist["mode"].(string)
	if !ok || mode != "deny_by_default" {
		report.AddError(CodeAllowlistRequired,
			"environment.allowlist.mode must be 'deny_by_default'",
			"/environment/allowlist/mode")
	}
}

func (v *SemanticValidator) validateTargetWithinSystemAllowlist(config map[string]interface{}, report *ValidationReport) {
	if v.systemPolicy == nil || len(v.systemPolicy.GlobalAllowlist) == 0 {
		return
	}

	target, ok := config["target"].(map[string]interface{})
	if !ok {
		return
	}

	url, ok := target["url"].(string)
	if !ok {
		return
	}

	if !v.matchesAllowlist(url, v.systemPolicy.GlobalAllowlist) {
		report.AddError(CodeAllowlistViolation,
			"Target URL does not match system policy global allowlist",
			"/target/url")
	}
}

func (v *SemanticValidator) matchesAllowlist(targetURL string, allowlist []AllowlistEntry) bool {
	targetHost := extractHost(targetURL)
	for _, entry := range allowlist {
		switch entry.Kind {
		case "exact":
			entryHost := extractHost(entry.Value)
			if targetHost == entryHost {
				return true
			}
		case "suffix":
			suffix := strings.ToLower(entry.Value)
			// Normalize suffix: ensure it starts with a dot for boundary-safe matching
			if !strings.HasPrefix(suffix, ".") {
				suffix = "." + suffix
			}
			// Boundary-safe suffix matching: host must equal suffix (without dot) or end with suffix
			if targetHost == strings.TrimPrefix(suffix, ".") || strings.HasSuffix(targetHost, suffix) {
				return true
			}
		}
	}
	return false
}

func extractHost(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		// Fallback to manual parsing for malformed URLs
		rawURL = strings.TrimPrefix(rawURL, "https://")
		rawURL = strings.TrimPrefix(rawURL, "http://")
		if idx := strings.Index(rawURL, "/"); idx != -1 {
			rawURL = rawURL[:idx]
		}
		if idx := strings.Index(rawURL, ":"); idx != -1 {
			rawURL = rawURL[:idx]
		}
		return strings.ToLower(rawURL)
	}
	host := parsed.Hostname()
	return strings.ToLower(host)
}

func (v *SemanticValidator) validateSecretRefsAllowed(config map[string]interface{}, report *ValidationReport) {
	if v.systemPolicy == nil || len(v.systemPolicy.AllowedSecretRefs) == 0 {
		return
	}

	target, ok := config["target"].(map[string]interface{})
	if !ok {
		return
	}

	auth, ok := target["auth"].(map[string]interface{})
	if !ok {
		return
	}

	secretRefs := []struct {
		field string
		path  string
	}{
		{"bearer_token_ref", "/target/auth/bearer_token_ref"},
		{"api_key_ref", "/target/auth/api_key_ref"},
	}

	for _, ref := range secretRefs {
		if val, ok := auth[ref.field].(string); ok && val != "" {
			if !v.matchesSecretRefPattern(val) {
				report.AddError(CodeSecretRefNotAllowed,
					"Secret reference does not match allowed patterns",
					ref.path)
			}
		}
	}

	tls, ok := target["tls"].(map[string]interface{})
	if ok {
		if caRef, ok := tls["ca_bundle_ref"].(string); ok && caRef != "" {
			if !v.matchesSecretRefPattern(caRef) {
				report.AddError(CodeSecretRefNotAllowed,
					"CA bundle reference does not match allowed patterns",
					"/target/tls/ca_bundle_ref")
			}
		}
	}
}

func (v *SemanticValidator) matchesSecretRefPattern(ref string) bool {
	for _, pattern := range v.systemPolicy.AllowedSecretRefs {
		if strings.HasSuffix(pattern, "*") {
			prefix := strings.TrimSuffix(pattern, "*")
			if strings.HasPrefix(ref, prefix) {
				return true
			}
		} else if ref == pattern {
			return true
		}
	}
	return false
}

func (v *SemanticValidator) validateIdentificationRequired(config map[string]interface{}, report *ValidationReport) {
	safety, ok := config["safety"].(map[string]interface{})
	if !ok {
		return
	}

	identRequired, _ := safety["identification_required"].(bool)
	systemRequires := v.systemPolicy != nil && v.systemPolicy.RequireIdentification

	if !identRequired && !systemRequires {
		return
	}

	target, ok := config["target"].(map[string]interface{})
	if !ok {
		return
	}

	identification, ok := target["identification"].(map[string]interface{})
	if !ok {
		report.AddError(CodeIdentificationRequired,
			"Test identification is required but target.identification is missing",
			"/target/identification")
		return
	}

	runIDHeader, ok := identification["run_id_header"].(map[string]interface{})
	if !ok {
		report.AddError(CodeIdentificationRequired,
			"target.identification.run_id_header is required",
			"/target/identification/run_id_header")
	} else {
		if _, ok := runIDHeader["name"].(string); !ok {
			report.AddError(CodeIdentificationRequired,
				"target.identification.run_id_header.name is required",
				"/target/identification/run_id_header/name")
		}
		if _, ok := runIDHeader["value_template"].(string); !ok {
			report.AddError(CodeIdentificationRequired,
				"target.identification.run_id_header.value_template is required",
				"/target/identification/run_id_header/value_template")
		}
	}

	userAgent, ok := identification["user_agent"].(map[string]interface{})
	if !ok {
		report.AddError(CodeIdentificationRequired,
			"target.identification.user_agent is required",
			"/target/identification/user_agent")
	} else {
		if val, ok := userAgent["value"].(string); !ok || !strings.Contains(val, "${run_id}") {
			report.AddError(CodeIdentificationRequired,
				"target.identification.user_agent.value must include ${run_id}",
				"/target/identification/user_agent/value")
		}
	}
}

func (v *SemanticValidator) validateRampByDefaultGuard(config map[string]interface{}, report *ValidationReport) {
	safety, ok := config["safety"].(map[string]interface{})
	if !ok {
		return
	}

	rampByDefault, _ := safety["ramp_by_default"].(bool)
	if !rampByDefault {
		return
	}

	hardCaps, ok := safety["hard_caps"].(map[string]interface{})
	if !ok {
		return
	}

	maxVUs, ok := hardCaps["max_vus"].(float64)
	if !ok {
		return
	}

	stages, ok := config["stages"].([]interface{})
	if !ok {
		return
	}

	preflightPassed := false
	for _, s := range stages {
		stage, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		enabled, _ := stage["enabled"].(bool)
		if !enabled {
			continue
		}
		stageType, _ := stage["stage"].(string)

		if stageType == "preflight" {
			preflightPassed = true
			continue
		}

		if !preflightPassed {
			continue
		}

		load, ok := stage["load"].(map[string]interface{})
		if !ok {
			continue
		}

		targetVUs, ok := load["target_vus"].(float64)
		if !ok {
			continue
		}

		if targetVUs > maxVUs*0.5 {
			report.AddWarning(CodeRampByDefaultGuard,
				"Stage jumps to > 50% of max_vus without gradual ramp",
				"/stages")
		}
	}
}

func (v *SemanticValidator) validateStopConditionsRequired(config map[string]interface{}, report *ValidationReport) {
	stages, ok := config["stages"].([]interface{})
	if !ok {
		return
	}

	for i, s := range stages {
		stage, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		enabled, _ := stage["enabled"].(bool)
		if !enabled {
			continue
		}
		stageType, _ := stage["stage"].(string)

		if stageType != "baseline" && stageType != "ramp" {
			continue
		}

		stopConditions, ok := stage["stop_conditions"].([]interface{})
		if !ok || len(stopConditions) == 0 {
			report.AddError(CodeStopConditionsRequired,
				stageType+" stage must define at least one stop condition",
				"/stages/"+strconv.Itoa(i)+"/stop_conditions")
		}
	}
}

func (v *SemanticValidator) validateStreamingGuardrails(config map[string]interface{}, report *ValidationReport) {
	workload, ok := config["workload"].(map[string]interface{})
	if !ok {
		return
	}

	hasStreaming := false
	tools, ok := workload["tools"].(map[string]interface{})
	if ok {
		templates, ok := tools["templates"].([]interface{})
		if ok {
			for _, t := range templates {
				tmpl, ok := t.(map[string]interface{})
				if !ok {
					continue
				}
				if expectsStreaming, ok := tmpl["expects_streaming"].(bool); ok && expectsStreaming {
					hasStreaming = true
					break
				}
			}
		}
	}

	if !hasStreaming {
		return
	}

	stages, ok := config["stages"].([]interface{})
	if !ok {
		return
	}

	hasStreamStallCondition := false
	for _, s := range stages {
		stage, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		enabled, _ := stage["enabled"].(bool)
		if !enabled {
			continue
		}

		stopConditions, ok := stage["stop_conditions"].([]interface{})
		if !ok {
			continue
		}

		for _, sc := range stopConditions {
			cond, ok := sc.(map[string]interface{})
			if !ok {
				continue
			}
			metric, _ := cond["metric"].(string)
			if strings.Contains(metric, "stream_stall") {
				hasStreamStallCondition = true
				break
			}
		}
	}

	if !hasStreamStallCondition {
		report.AddError(CodeStreamingGuardrails,
			"Streaming is enabled but no stream stall stop condition is defined",
			"/stages")
	}
}

func (v *SemanticValidator) validateRedirectPolicyRequired(config map[string]interface{}, report *ValidationReport) {
	target, ok := config["target"].(map[string]interface{})
	if !ok {
		return
	}

	redirectPolicy, ok := target["redirect_policy"].(map[string]interface{})
	if !ok {
		report.AddError(CodeRedirectPolicyRequired,
			"target.redirect_policy is required",
			"/target/redirect_policy")
		return
	}

	if _, ok := redirectPolicy["mode"].(string); !ok {
		report.AddError(CodeRedirectPolicyRequired,
			"target.redirect_policy.mode must be explicitly set",
			"/target/redirect_policy/mode")
	}
}

var validWorkerFailurePolicies = map[string]bool{
	"fail_fast":           true,
	"replace_if_possible": true,
	"best_effort":         true,
}

func (v *SemanticValidator) validateWorkerFailurePolicy(config map[string]interface{}, report *ValidationReport) {
	safety, ok := config["safety"].(map[string]interface{})
	if !ok {
		return
	}

	policy, ok := safety["worker_failure_policy"].(string)
	if !ok {
		return
	}

	if !validWorkerFailurePolicies[policy] {
		report.AddErrorWithRemediation(CodeInvalidWorkerFailurePolicy,
			"Invalid worker_failure_policy value: "+policy,
			"/safety/worker_failure_policy",
			"Valid values are: fail_fast, replace_if_possible, best_effort")
	}
}

func (v *SemanticValidator) validateChurnIntervalOps(config map[string]interface{}, report *ValidationReport) {
	sessionPolicy, ok := config["session_policy"].(map[string]interface{})
	if !ok {
		return
	}

	mode, _ := sessionPolicy["mode"].(string)
	churnIntervalOps, hasChurnOps := sessionPolicy["churn_interval_ops"].(float64)

	if hasChurnOps && churnIntervalOps > 0 && mode != "churn" {
		report.AddErrorWithRemediation(CodeChurnIntervalOpsInvalid,
			"churn_interval_ops is only valid when session_policy.mode is 'churn'",
			"/session_policy/churn_interval_ops",
			"Either set mode to 'churn' or remove churn_interval_ops")
	}
}
func (v *SemanticValidator) validateTargetWithinRunAllowlist(config map[string]interface{}, report *ValidationReport) {
	target, ok := config["target"].(map[string]interface{})
	if !ok {
		return
	}

	targetURL, ok := target["url"].(string)
	if !ok || targetURL == "" {
		return
	}

	// Check if environment.allowlist section exists
	env, envOk := config["environment"].(map[string]interface{})
	if !envOk {
		// No environment section - skip run allowlist validation
		// (validateAllowlistRequired will handle this)
		return
	}

	allowlistSection, allowlistOk := env["allowlist"].(map[string]interface{})
	if !allowlistOk {
		// No allowlist section - skip run allowlist validation
		return
	}

	runAllowlist := extractRunConfigAllowlistFromConfig(config)
	
	// Default-deny: if allowlist section exists but allowed_targets is empty/missing, reject
	if len(runAllowlist) == 0 {
		// Check if mode is deny_by_default (which requires explicit allowed_targets)
		mode, _ := allowlistSection["mode"].(string)
		if mode == "deny_by_default" {
			report.AddError(CodeAllowlistViolation,
				"Target URL provided but no allowed_targets defined in environment.allowlist (default-deny)",
				"/target/url")
			return
		}
		// If mode is not deny_by_default, skip validation
		return
	}

	targetHost := extractHost(targetURL)

	matchesRun := false
	for _, entry := range runAllowlist {
		if matchesAllowlistEntry(targetHost, entry) {
			matchesRun = true
			break
		}
	}

	if !matchesRun {
		report.AddError(CodeAllowlistViolation,
			"Target URL does not match run config allowlist (environment.allowlist.allowed_targets)",
			"/target/url")
		return
	}

	if v.systemPolicy != nil && len(v.systemPolicy.GlobalAllowlist) > 0 {
		matchesSys := false
		for _, entry := range v.systemPolicy.GlobalAllowlist {
			if matchesAllowlistEntry(targetHost, entry) {
				matchesSys = true
				break
			}
		}
		if !matchesSys {
			report.AddError(CodeAllowlistViolation,
				"Target URL matches run config allowlist but not system policy global allowlist",
				"/target/url")
		}
	}
}

func extractRunConfigAllowlistFromConfig(config map[string]interface{}) []AllowlistEntry {
	env, ok := config["environment"].(map[string]interface{})
	if !ok {
		return nil
	}

	allowlist, ok := env["allowlist"].(map[string]interface{})
	if !ok {
		return nil
	}

	entries, ok := allowlist["allowed_targets"].([]interface{})
	if !ok {
		return nil
	}

	var result []AllowlistEntry
	for _, e := range entries {
		entry, ok := e.(map[string]interface{})
		if !ok {
			continue
		}
		kind, _ := entry["kind"].(string)
		value, _ := entry["value"].(string)
		if kind != "" && value != "" {
			result = append(result, AllowlistEntry{Kind: kind, Value: value})
		}
	}

	return result
}

func matchesAllowlistEntry(targetHost string, entry AllowlistEntry) bool {
	targetHost = strings.ToLower(targetHost)
	switch entry.Kind {
	case "exact":
		entryHost := extractHost(entry.Value)
		return targetHost == entryHost
	case "suffix":
		suffix := strings.ToLower(entry.Value)
		// Normalize suffix: ensure it starts with a dot for boundary-safe matching
		if !strings.HasPrefix(suffix, ".") {
			suffix = "." + suffix
		}
		// Boundary-safe suffix matching: host must equal suffix (without dot) or end with suffix
		return targetHost == strings.TrimPrefix(suffix, ".") || strings.HasSuffix(targetHost, suffix)
	}
	return false
}

func (v *SemanticValidator) validateForbiddenPatterns(config map[string]interface{}, report *ValidationReport) {
	target, ok := config["target"].(map[string]interface{})
	if !ok {
		return
	}

	targetURL, ok := target["url"].(string)
	if !ok || targetURL == "" {
		return
	}

	targetHost := extractHost(targetURL)

	var forbiddenPatterns []string

	if v.systemPolicy != nil {
		forbiddenPatterns = append(forbiddenPatterns, v.systemPolicy.ForbiddenPatterns...)
	}

	if env, ok := config["environment"].(map[string]interface{}); ok {
		if patterns, ok := env["forbidden_patterns"].([]interface{}); ok {
			for _, p := range patterns {
				if s, ok := p.(string); ok {
					forbiddenPatterns = append(forbiddenPatterns, s)
				}
			}
		}
	}

	for _, pattern := range forbiddenPatterns {
		if matchesForbiddenPattern(targetHost, pattern) || matchesForbiddenPattern(targetURL, pattern) {
			report.AddError(CodeForbiddenPatternMatched,
				"Target URL matches forbidden pattern: "+pattern,
				"/target/url")
			return
		}
	}
}


// matchesForbiddenPattern checks if value matches pattern.
// For glob patterns (containing *?[]), uses path.Match for pure glob matching.
// For non-glob patterns, uses exact case-insensitive match.
func matchesForbiddenPattern(value, pattern string) bool {
	valueLower := strings.ToLower(value)
	patternLower := strings.ToLower(pattern)

	// Check if pattern contains glob characters
	if strings.ContainsAny(pattern, "*?[]") {
		// Use path.Match for pure glob matching only
		matched, err := path.Match(patternLower, valueLower)
		return err == nil && matched
	}

	// Non-glob pattern: exact case-insensitive match
	return valueLower == patternLower
}
