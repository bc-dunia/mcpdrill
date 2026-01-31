package validation

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bc-dunia/mcpdrill/schemas"
)

type SchemaValidator struct {
	schemas map[string]map[string]interface{}
	defs    map[string]map[string]interface{} // $defs for each schema
}

func NewSchemaValidator() (*SchemaValidator, error) {
	v := &SchemaValidator{
		schemas: make(map[string]map[string]interface{}),
		defs:    make(map[string]map[string]interface{}),
	}

	schemaFiles := []string{
		"run-config/v1.json",
		"op-log/v1.json",
		"event/v1.json",
		"worker-protocol/v1.json",
		"report/v1.json",
		"metrics-snapshot/v1.json",
		"metrics-window/v1.json",
	}

	for _, path := range schemaFiles {
		data, err := schemas.FS.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read schema %s: %w", path, err)
		}

		var schema map[string]interface{}
		if err := json.Unmarshal(data, &schema); err != nil {
			return nil, fmt.Errorf("failed to parse schema %s: %w", path, err)
		}

		schemaVersion := extractSchemaVersion(path)
		v.schemas[schemaVersion] = schema

		if defs, ok := schema["$defs"].(map[string]interface{}); ok {
			v.defs[schemaVersion] = defs
		}
	}

	return v, nil
}

func extractSchemaVersion(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) >= 2 {
		name := parts[0]
		version := strings.TrimSuffix(parts[1], ".json")
		return name + "/" + version
	}
	return path
}

func (v *SchemaValidator) ValidateRunConfig(data []byte) *ValidationReport {
	report := NewValidationReport()

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		report.AddError(CodeSchemaViolation, fmt.Sprintf("Invalid JSON: %v", err), "")
		return report
	}

	schemaVersion, ok := config["schema_version"].(string)
	if !ok {
		report.AddError(CodeRequiredFieldMissing, "schema_version is required", "/schema_version")
		return report
	}

	if schemaVersion != "run-config/v1" {
		report.AddError(CodeInvalidSchemaVersion,
			fmt.Sprintf("Expected schema_version 'run-config/v1', got '%s'", schemaVersion),
			"/schema_version")
		return report
	}

	schema, ok := v.schemas["run-config/v1"]
	if !ok {
		report.AddError(CodeSchemaViolation, "Schema run-config/v1 not loaded", "")
		return report
	}

	v.validateObjectWithContext(config, schema, "", "run-config/v1", report)
	return report
}

func (v *SchemaValidator) ValidateOpLog(data []byte) *ValidationReport {
	report := NewValidationReport()

	var record map[string]interface{}
	if err := json.Unmarshal(data, &record); err != nil {
		report.AddError(CodeSchemaViolation, fmt.Sprintf("Invalid JSON: %v", err), "")
		return report
	}

	schemaVersion, ok := record["schema_version"].(string)
	if !ok {
		report.AddError(CodeRequiredFieldMissing, "schema_version is required", "/schema_version")
		return report
	}

	if schemaVersion != "op-log/v1" {
		report.AddError(CodeInvalidSchemaVersion,
			fmt.Sprintf("Expected schema_version 'op-log/v1', got '%s'", schemaVersion),
			"/schema_version")
		return report
	}

	schema, ok := v.schemas["op-log/v1"]
	if !ok {
		report.AddError(CodeSchemaViolation, "Schema op-log/v1 not loaded", "")
		return report
	}

	v.validateObjectWithContext(record, schema, "", "op-log/v1", report)
	return report
}

func (v *SchemaValidator) ValidateEvent(data []byte) *ValidationReport {
	report := NewValidationReport()

	var event map[string]interface{}
	if err := json.Unmarshal(data, &event); err != nil {
		report.AddError(CodeSchemaViolation, fmt.Sprintf("Invalid JSON: %v", err), "")
		return report
	}

	schemaVersion, ok := event["schema_version"].(string)
	if !ok {
		report.AddError(CodeRequiredFieldMissing, "schema_version is required", "/schema_version")
		return report
	}

	if schemaVersion != "event/v1" {
		report.AddError(CodeInvalidSchemaVersion,
			fmt.Sprintf("Expected schema_version 'event/v1', got '%s'", schemaVersion),
			"/schema_version")
		return report
	}

	schema, ok := v.schemas["event/v1"]
	if !ok {
		report.AddError(CodeSchemaViolation, "Schema event/v1 not loaded", "")
		return report
	}

	v.validateObjectWithContext(event, schema, "", "event/v1", report)
	return report
}

func (v *SchemaValidator) ValidateReport(data []byte) *ValidationReport {
	report := NewValidationReport()

	var reportData map[string]interface{}
	if err := json.Unmarshal(data, &reportData); err != nil {
		report.AddError(CodeSchemaViolation, fmt.Sprintf("Invalid JSON: %v", err), "")
		return report
	}

	schemaVersion, ok := reportData["schema_version"].(string)
	if !ok {
		report.AddError(CodeRequiredFieldMissing, "schema_version is required", "/schema_version")
		return report
	}

	if schemaVersion != "report/v1" {
		report.AddError(CodeInvalidSchemaVersion,
			fmt.Sprintf("Expected schema_version 'report/v1', got '%s'", schemaVersion),
			"/schema_version")
		return report
	}

	schema, ok := v.schemas["report/v1"]
	if !ok {
		report.AddError(CodeSchemaViolation, "Schema report/v1 not loaded", "")
		return report
	}

	v.validateObjectWithContext(reportData, schema, "", "report/v1", report)
	return report
}

func (v *SchemaValidator) validateObject(data map[string]interface{}, schema map[string]interface{}, path string, report *ValidationReport) {
	v.validateObjectWithContext(data, schema, path, "", report)
}

func (v *SchemaValidator) validateValue(value interface{}, schema map[string]interface{}, path string, report *ValidationReport) {
	v.validateValueWithContext(value, schema, path, "", report)
}

func (v *SchemaValidator) validateValueWithContext(value interface{}, schema map[string]interface{}, path string, schemaVersion string, report *ValidationReport) {
	if ref, ok := schema["$ref"].(string); ok {
		resolved := v.resolveRef(ref, schemaVersion)
		if resolved != nil {
			v.validateValueWithContext(value, resolved, path, schemaVersion, report)
		}
		return
	}

	if oneOf, ok := schema["oneOf"].([]interface{}); ok {
		v.validateOneOf(value, oneOf, path, schemaVersion, report)
		return
	}

	if allOf, ok := schema["allOf"].([]interface{}); ok {
		v.validateAllOf(value, allOf, path, schemaVersion, report)
	}

	v.validateIfThenElse(value, schema, path, schemaVersion, report)

	if value == nil {
		typeSpec := schema["type"]
		if typeSpec != nil {
			if !isNullAllowed(typeSpec) {
				report.AddError(CodeSchemaViolation, "Value cannot be null", path)
			}
		}
		return
	}

	typeSpec := schema["type"]
	if typeSpec != nil {
		if !v.validateType(value, typeSpec, path, report) {
			return
		}
	}

	switch val := value.(type) {
	case string:
		v.validateString(val, schema, path, report)
	case float64:
		v.validateNumber(val, schema, path, report)
	case bool:
	case map[string]interface{}:
		v.validateObjectWithContext(val, schema, path, schemaVersion, report)
	case []interface{}:
		v.validateArrayWithContext(val, schema, path, schemaVersion, report)
	}

	if constVal, hasConst := schema["const"]; hasConst {
		if value != constVal {
			report.AddError(CodeSchemaViolation,
				fmt.Sprintf("Value must be '%v', got '%v'", constVal, value),
				path)
		}
	}

	if enumVals, hasEnum := schema["enum"].([]interface{}); hasEnum {
		found := false
		for _, e := range enumVals {
			if value == e {
				found = true
				break
			}
		}
		if !found {
			report.AddError(CodeSchemaViolation,
				fmt.Sprintf("Value '%v' is not one of the allowed values", value),
				path)
		}
	}
}

func (v *SchemaValidator) resolveRef(ref string, schemaVersion string) map[string]interface{} {
	if !strings.HasPrefix(ref, "#/$defs/") {
		return nil
	}
	defName := strings.TrimPrefix(ref, "#/$defs/")
	if defs, ok := v.defs[schemaVersion]; ok {
		if def, ok := defs[defName].(map[string]interface{}); ok {
			return def
		}
	}
	return nil
}

func (v *SchemaValidator) validateOneOf(value interface{}, schemas []interface{}, path string, schemaVersion string, report *ValidationReport) {
	matchCount := 0
	for _, s := range schemas {
		subSchema, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		testReport := NewValidationReport()
		v.validateValueWithContext(value, subSchema, path, schemaVersion, testReport)
		if testReport.OK {
			matchCount++
		}
	}
	if matchCount != 1 {
		report.AddError(CodeSchemaViolation,
			fmt.Sprintf("Value must match exactly one schema in oneOf, matched %d", matchCount),
			path)
	}
}

func (v *SchemaValidator) validateAllOf(value interface{}, schemas []interface{}, path string, schemaVersion string, report *ValidationReport) {
	for _, s := range schemas {
		subSchema, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		v.validateValueWithContext(value, subSchema, path, schemaVersion, report)
	}
}

func (v *SchemaValidator) validateIfThenElse(value interface{}, schema map[string]interface{}, path string, schemaVersion string, report *ValidationReport) {
	ifSchema, hasIf := schema["if"].(map[string]interface{})
	if !hasIf {
		return
	}

	testReport := NewValidationReport()
	v.validateValueWithContext(value, ifSchema, path, schemaVersion, testReport)

	if testReport.OK {
		if thenSchema, ok := schema["then"].(map[string]interface{}); ok {
			v.validateValueWithContext(value, thenSchema, path, schemaVersion, report)
		}
	} else {
		if elseSchema, ok := schema["else"].(map[string]interface{}); ok {
			v.validateValueWithContext(value, elseSchema, path, schemaVersion, report)
		}
	}
}

func (v *SchemaValidator) validateObjectWithContext(data map[string]interface{}, schema map[string]interface{}, path string, schemaVersion string, report *ValidationReport) {
	required, _ := schema["required"].([]interface{})
	for _, r := range required {
		fieldName, _ := r.(string)
		if _, exists := data[fieldName]; !exists {
			report.AddError(CodeRequiredFieldMissing,
				fmt.Sprintf("Required field '%s' is missing", fieldName),
				joinPath(path, fieldName))
		}
	}

	properties, _ := schema["properties"].(map[string]interface{})
	additionalPropsSchema, hasAdditionalSchema := schema["additionalProperties"].(map[string]interface{})

	for fieldName, value := range data {
		fieldPath := joinPath(path, fieldName)

		propSchema, hasProp := properties[fieldName].(map[string]interface{})
		if !hasProp {
			additionalProps, hasAdditional := schema["additionalProperties"]
			if hasAdditional {
				if additionalProps == false {
					report.AddError(CodeSchemaViolation,
						fmt.Sprintf("Additional property '%s' is not allowed", fieldName),
						fieldPath)
				} else if hasAdditionalSchema {
					v.validateValueWithContext(value, additionalPropsSchema, fieldPath, schemaVersion, report)
				}
			}
			continue
		}

		v.validateValueWithContext(value, propSchema, fieldPath, schemaVersion, report)
	}
}

func (v *SchemaValidator) validateArrayWithContext(arr []interface{}, schema map[string]interface{}, path string, schemaVersion string, report *ValidationReport) {
	if minItems, ok := schema["minItems"].(float64); ok {
		if len(arr) < int(minItems) {
			report.AddError(CodeSchemaViolation,
				fmt.Sprintf("Array has %d items, minimum is %d", len(arr), int(minItems)),
				path)
		}
	}

	if maxItems, ok := schema["maxItems"].(float64); ok {
		if len(arr) > int(maxItems) {
			report.AddError(CodeSchemaViolation,
				fmt.Sprintf("Array has %d items, maximum is %d", len(arr), int(maxItems)),
				path)
		}
	}

	if itemSchema, ok := schema["items"].(map[string]interface{}); ok {
		for i, item := range arr {
			itemPath := fmt.Sprintf("%s/%d", path, i)
			v.validateValueWithContext(item, itemSchema, itemPath, schemaVersion, report)
		}
	}
}

func (v *SchemaValidator) validateType(value interface{}, typeSpec interface{}, path string, report *ValidationReport) bool {
	types := normalizeTypeSpec(typeSpec)

	actualType := getJSONType(value)
	for _, t := range types {
		if t == actualType {
			return true
		}
		if t == "integer" && actualType == "number" {
			if num, ok := value.(float64); ok && num == float64(int64(num)) {
				return true
			}
		}
	}

	report.AddError(CodeSchemaViolation,
		fmt.Sprintf("Expected type %v, got %s", types, actualType),
		path)
	return false
}

func (v *SchemaValidator) validateString(val string, schema map[string]interface{}, path string, report *ValidationReport) {
	if minLen, ok := schema["minLength"].(float64); ok {
		if len(val) < int(minLen) {
			report.AddError(CodeSchemaViolation,
				fmt.Sprintf("String length %d is less than minimum %d", len(val), int(minLen)),
				path)
		}
	}

	if maxLen, ok := schema["maxLength"].(float64); ok {
		if len(val) > int(maxLen) {
			report.AddError(CodeSchemaViolation,
				fmt.Sprintf("String length %d exceeds maximum %d", len(val), int(maxLen)),
				path)
		}
	}
}

func (v *SchemaValidator) validateNumber(val float64, schema map[string]interface{}, path string, report *ValidationReport) {
	if min, ok := schema["minimum"].(float64); ok {
		if val < min {
			report.AddError(CodeSchemaViolation,
				fmt.Sprintf("Value %v is less than minimum %v", val, min),
				path)
		}
	}

	if max, ok := schema["maximum"].(float64); ok {
		if val > max {
			report.AddError(CodeSchemaViolation,
				fmt.Sprintf("Value %v exceeds maximum %v", val, max),
				path)
		}
	}

	if exclMin, ok := schema["exclusiveMinimum"].(float64); ok {
		if val <= exclMin {
			report.AddError(CodeSchemaViolation,
				fmt.Sprintf("Value %v must be greater than %v", val, exclMin),
				path)
		}
	}

	if exclMax, ok := schema["exclusiveMaximum"].(float64); ok {
		if val >= exclMax {
			report.AddError(CodeSchemaViolation,
				fmt.Sprintf("Value %v must be less than %v", val, exclMax),
				path)
		}
	}
}

func (v *SchemaValidator) validateArray(arr []interface{}, schema map[string]interface{}, path string, report *ValidationReport) {
	v.validateArrayWithContext(arr, schema, path, "", report)
}

func isNullAllowed(typeSpec interface{}) bool {
	types := normalizeTypeSpec(typeSpec)
	for _, t := range types {
		if t == "null" {
			return true
		}
	}
	return false
}

func normalizeTypeSpec(typeSpec interface{}) []string {
	switch t := typeSpec.(type) {
	case string:
		return []string{t}
	case []interface{}:
		result := make([]string, 0, len(t))
		for _, v := range t {
			if s, ok := v.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

func getJSONType(value interface{}) string {
	switch value.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case float64:
		return "number"
	case string:
		return "string"
	case []interface{}:
		return "array"
	case map[string]interface{}:
		return "object"
	default:
		return "unknown"
	}
}

func joinPath(base, field string) string {
	if base == "" {
		return "/" + field
	}
	return base + "/" + field
}
