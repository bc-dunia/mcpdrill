package validation

type EffectiveLimits struct {
	MaxVUs             int
	MaxRPS             float64
	MaxConnections     int
	MaxDurationMs      int
	MaxInFlightPerVU   int
	MaxTelemetryQDepth int

	EffectiveAllowlist    []AllowlistEntry
	EffectiveForbidden    []string
	IdentificationRequired bool
}

func ComputeEffectiveLimits(runConfig map[string]interface{}, systemPolicy *SystemPolicy) *EffectiveLimits {
	if systemPolicy == nil {
		systemPolicy = DefaultSystemPolicy()
	}

	limits := &EffectiveLimits{
		MaxVUs:             systemPolicy.GlobalHardCaps.MaxVUs,
		MaxRPS:             systemPolicy.GlobalHardCaps.MaxRPS,
		MaxConnections:     systemPolicy.GlobalHardCaps.MaxConnections,
		MaxDurationMs:      systemPolicy.GlobalHardCaps.MaxDurationMs,
		MaxInFlightPerVU:   systemPolicy.GlobalHardCaps.MaxInFlightPerVU,
		MaxTelemetryQDepth: systemPolicy.GlobalHardCaps.MaxTelemetryQDepth,
	}

	if safety, ok := runConfig["safety"].(map[string]interface{}); ok {
		if hardCaps, ok := safety["hard_caps"].(map[string]interface{}); ok {
			limits.MaxVUs = minInt(getIntOrDefault(hardCaps, "max_vus", limits.MaxVUs), systemPolicy.GlobalHardCaps.MaxVUs)
			limits.MaxRPS = minFloat(getFloatOrDefault(hardCaps, "max_rps", limits.MaxRPS), systemPolicy.GlobalHardCaps.MaxRPS)
			limits.MaxConnections = minInt(getIntOrDefault(hardCaps, "max_connections", limits.MaxConnections), systemPolicy.GlobalHardCaps.MaxConnections)
			limits.MaxDurationMs = minInt(getIntOrDefault(hardCaps, "max_duration_ms", limits.MaxDurationMs), systemPolicy.GlobalHardCaps.MaxDurationMs)
			limits.MaxInFlightPerVU = minInt(getIntOrDefault(hardCaps, "max_in_flight_per_vu", limits.MaxInFlightPerVU), systemPolicy.GlobalHardCaps.MaxInFlightPerVU)
		}

		if identRequired, ok := safety["identification_required"].(bool); ok && identRequired {
			limits.IdentificationRequired = true
		}
	}

	if systemPolicy.RequireIdentification {
		limits.IdentificationRequired = true
	}

	limits.EffectiveAllowlist = computeAllowlistIntersection(runConfig, systemPolicy)
	limits.EffectiveForbidden = computeForbiddenUnion(runConfig, systemPolicy)

	return limits
}

func computeAllowlistIntersection(runConfig map[string]interface{}, systemPolicy *SystemPolicy) []AllowlistEntry {
	if len(systemPolicy.GlobalAllowlist) == 0 {
		return extractRunConfigAllowlist(runConfig)
	}

	runAllowlist := extractRunConfigAllowlist(runConfig)
	if len(runAllowlist) == 0 {
		return systemPolicy.GlobalAllowlist
	}

	var intersection []AllowlistEntry
	for _, runEntry := range runAllowlist {
		for _, sysEntry := range systemPolicy.GlobalAllowlist {
			if entriesOverlap(runEntry, sysEntry) {
				intersection = append(intersection, runEntry)
				break
			}
		}
	}

	return intersection
}

func extractRunConfigAllowlist(runConfig map[string]interface{}) []AllowlistEntry {
	env, ok := runConfig["environment"].(map[string]interface{})
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

func entriesOverlap(a, b AllowlistEntry) bool {
	if a.Kind == "exact" && b.Kind == "exact" {
		return a.Value == b.Value
	}

	if a.Kind == "suffix" && b.Kind == "suffix" {
		return a.Value == b.Value ||
			len(a.Value) > len(b.Value) && a.Value[len(a.Value)-len(b.Value):] == b.Value ||
			len(b.Value) > len(a.Value) && b.Value[len(b.Value)-len(a.Value):] == a.Value
	}

	if a.Kind == "exact" && b.Kind == "suffix" {
		return len(a.Value) >= len(b.Value) && a.Value[len(a.Value)-len(b.Value):] == b.Value
	}

	if a.Kind == "suffix" && b.Kind == "exact" {
		return len(b.Value) >= len(a.Value) && b.Value[len(b.Value)-len(a.Value):] == a.Value
	}

	return false
}

func computeForbiddenUnion(runConfig map[string]interface{}, systemPolicy *SystemPolicy) []string {
	forbidden := make(map[string]bool)

	for _, p := range systemPolicy.ForbiddenPatterns {
		forbidden[p] = true
	}

	if env, ok := runConfig["environment"].(map[string]interface{}); ok {
		if patterns, ok := env["forbidden_patterns"].([]interface{}); ok {
			for _, p := range patterns {
				if s, ok := p.(string); ok {
					forbidden[s] = true
				}
			}
		}
	}

	result := make([]string, 0, len(forbidden))
	for p := range forbidden {
		result = append(result, p)
	}
	return result
}

func ValidateSecretRef(ref string, systemPolicy *SystemPolicy) bool {
	if systemPolicy == nil || len(systemPolicy.AllowedSecretRefs) == 0 {
		return true
	}

	for _, pattern := range systemPolicy.AllowedSecretRefs {
		if matchesPattern(ref, pattern) {
			return true
		}
	}
	return false
}

func matchesPattern(value, pattern string) bool {
	if len(pattern) == 0 {
		return false
	}

	if pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1]
		return len(value) >= len(prefix) && value[:len(prefix)] == prefix
	}

	return value == pattern
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func getIntOrDefault(m map[string]interface{}, key string, defaultVal int) int {
	if v, ok := m[key].(float64); ok {
		return int(v)
	}
	return defaultVal
}

func getFloatOrDefault(m map[string]interface{}, key string, defaultVal float64) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return defaultVal
}
