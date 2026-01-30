package analysis

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNewReporter(t *testing.T) {
	r := NewReporter()
	if r == nil {
		t.Fatal("NewReporter returned nil")
	}
}

func TestGenerateJSON_NilReport(t *testing.T) {
	r := NewReporter()
	_, err := r.GenerateJSON(nil)
	if err == nil {
		t.Fatal("expected error for nil report")
	}
	if !strings.Contains(err.Error(), "nil") {
		t.Errorf("error should mention nil: %v", err)
	}
}

func TestGenerateJSON_EmptyMetrics(t *testing.T) {
	r := NewReporter()
	report := &Report{
		RunID:      "run_0000000000000123",
		ScenarioID: "test-scenario",
		StartTime:  1700000000000,
		EndTime:    1700000060000,
		Duration:   60000,
		Metrics:    nil,
		StopReason: "completed",
	}

	data, err := r.GenerateJSON(report)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed Report
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if parsed.RunID != "run_0000000000000123" {
		t.Errorf("RunID mismatch: got %s", parsed.RunID)
	}
	if parsed.Metrics == nil {
		t.Error("Metrics should not be nil in output")
	}
}

func TestGenerateJSON_FullMetrics(t *testing.T) {
	r := NewReporter()
	report := createFullReport()

	data, err := r.GenerateJSON(report)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(string(data), "\"run_id\": \"run_0000000000000456\"") {
		t.Error("JSON should contain run_id")
	}
	if !strings.Contains(string(data), "\"scenario_id\": \"stress-test\"") {
		t.Error("JSON should contain scenario_id")
	}
	if !strings.Contains(string(data), "\"total_ops\": 1000") {
		t.Error("JSON should contain total_ops")
	}

	var parsed Report
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if parsed.Metrics.TotalOps != 1000 {
		t.Errorf("TotalOps mismatch: got %d", parsed.Metrics.TotalOps)
	}
	if parsed.Metrics.ByOperation["tools_list"].TotalOps != 500 {
		t.Error("ByOperation data missing")
	}
	if parsed.Metrics.ByTool["echo"].TotalOps != 200 {
		t.Error("ByTool data missing")
	}
}

func TestGenerateJSON_PrettyPrinted(t *testing.T) {
	r := NewReporter()
	report := &Report{
		RunID:      "run_0000000000000001",
		ScenarioID: "test",
		Metrics:    nil,
	}

	data, err := r.GenerateJSON(report)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(string(data), "\n") {
		t.Error("JSON should be pretty-printed with newlines")
	}
	if !strings.Contains(string(data), "  ") {
		t.Error("JSON should be indented")
	}
}

func TestGenerateHTML_NilReport(t *testing.T) {
	r := NewReporter()
	_, err := r.GenerateHTML(nil)
	if err == nil {
		t.Fatal("expected error for nil report")
	}
}

func TestGenerateHTML_EmptyMetrics(t *testing.T) {
	r := NewReporter()
	report := &Report{
		RunID:      "run_0000000000000000",
		ScenarioID: "empty-test",
		StartTime:  1700000000000,
		EndTime:    1700000060000,
		Duration:   60000,
		Metrics:    nil,
		StopReason: "completed",
	}

	data, err := r.GenerateHTML(report)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	html := string(data)
	assertValidHTML(t, html)
	assertContains(t, html, "run_0000000000000000")
	assertContains(t, html, "empty-test")
	assertContains(t, html, "No operation data available")
	assertContains(t, html, "No tool data available")
}

func TestGenerateHTML_FullMetrics(t *testing.T) {
	r := NewReporter()
	report := createFullReport()

	data, err := r.GenerateHTML(report)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	html := string(data)
	assertValidHTML(t, html)

	assertContains(t, html, "run_0000000000000456")
	assertContains(t, html, "stress-test")
	assertContains(t, html, "1000")
	assertContains(t, html, "950")
	assertContains(t, html, "50")

	assertContains(t, html, "tools_list")
	assertContains(t, html, "tools_call")
	assertContains(t, html, "echo")
	assertContains(t, html, "add")

	assertContains(t, html, "100")
	assertContains(t, html, "200")
	assertContains(t, html, "500")
}

func TestGenerateHTML_SelfContained(t *testing.T) {
	r := NewReporter()
	report := createFullReport()

	data, err := r.GenerateHTML(report)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	html := string(data)

	assertContains(t, html, "<style>")
	assertContains(t, html, "</style>")

	if strings.Contains(html, "<link rel=\"stylesheet\"") {
		t.Error("HTML should not have external stylesheet links")
	}
	if strings.Contains(html, "<script src=") {
		t.Error("HTML should not have external script links")
	}
}

func TestGenerateHTML_ValidHTML5(t *testing.T) {
	r := NewReporter()
	report := createFullReport()

	data, err := r.GenerateHTML(report)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	html := string(data)

	assertContains(t, html, "<!DOCTYPE html>")
	assertContains(t, html, "<html lang=\"en\">")
	assertContains(t, html, "<meta charset=\"UTF-8\">")
	assertContains(t, html, "<meta name=\"viewport\"")
	assertContains(t, html, "</html>")
}

func TestGenerateHTML_NoOperations(t *testing.T) {
	r := NewReporter()
	report := &Report{
		RunID:      "run_0000000000000004",
		ScenarioID: "no-ops-test",
		Metrics: &AggregatedMetrics{
			TotalOps:    0,
			ByOperation: make(map[string]*OperationMetrics),
			ByTool:      make(map[string]*OperationMetrics),
		},
	}

	data, err := r.GenerateHTML(report)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	html := string(data)
	assertValidHTML(t, html)
	assertContains(t, html, "No operation data available")
	assertContains(t, html, "No tool data available")
}

func TestGenerateHTML_NoTools(t *testing.T) {
	r := NewReporter()
	report := &Report{
		RunID:      "run_0000000000000005",
		ScenarioID: "no-tools-test",
		Metrics: &AggregatedMetrics{
			TotalOps:   100,
			SuccessOps: 100,
			ByOperation: map[string]*OperationMetrics{
				"ping": {TotalOps: 100, SuccessOps: 100},
			},
			ByTool: make(map[string]*OperationMetrics),
		},
	}

	data, err := r.GenerateHTML(report)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	html := string(data)
	assertValidHTML(t, html)
	assertContains(t, html, "ping")
	assertContains(t, html, "No tool data available")
}

func TestFormatTimestamp(t *testing.T) {
	tests := []struct {
		name     string
		ts       int64
		expected string
	}{
		{"zero", 0, "N/A"},
		{"valid", 1700000000000, "2023-11-14T22:13:20Z"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatTimestamp(tt.ts)
			if result != tt.expected {
				t.Errorf("formatTimestamp(%d) = %s, want %s", tt.ts, result, tt.expected)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		ms       int64
		expected string
	}{
		{"zero", 0, "0s"},
		{"milliseconds", 500, "500ms"},
		{"seconds", 5000, "5.0s"},
		{"minutes", 120000, "2.0m"},
		{"hours", 7200000, "2.0h"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDuration(tt.ms)
			if result != tt.expected {
				t.Errorf("formatDuration(%d) = %s, want %s", tt.ms, result, tt.expected)
			}
		})
	}
}

func TestBuildOperationRows_Empty(t *testing.T) {
	rows := buildOperationRows(nil)
	if rows != nil {
		t.Error("expected nil for nil input")
	}

	rows = buildOperationRows(make(map[string]*OperationMetrics))
	if rows != nil {
		t.Error("expected nil for empty map")
	}
}

func TestBuildOperationRows_Sorted(t *testing.T) {
	metrics := map[string]*OperationMetrics{
		"zebra": {TotalOps: 1},
		"alpha": {TotalOps: 2},
		"beta":  {TotalOps: 3},
	}

	rows := buildOperationRows(metrics)
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	if rows[0].Name != "alpha" {
		t.Errorf("first row should be alpha, got %s", rows[0].Name)
	}
	if rows[1].Name != "beta" {
		t.Errorf("second row should be beta, got %s", rows[1].Name)
	}
	if rows[2].Name != "zebra" {
		t.Errorf("third row should be zebra, got %s", rows[2].Name)
	}
}

func createFullReport() *Report {
	return &Report{
		RunID:      "run_0000000000000456",
		ScenarioID: "stress-test",
		StartTime:  1700000000000,
		EndTime:    1700000060000,
		Duration:   60000,
		StopReason: "duration_reached",
		Metrics: &AggregatedMetrics{
			TotalOps:   1000,
			SuccessOps: 950,
			FailureOps: 50,
			RPS:        16.67,
			LatencyP50: 100,
			LatencyP95: 200,
			LatencyP99: 500,
			ErrorRate:  5.0,
			ByOperation: map[string]*OperationMetrics{
				"tools_list": {
					TotalOps:   500,
					SuccessOps: 490,
					FailureOps: 10,
					LatencyP50: 80,
					LatencyP95: 150,
					LatencyP99: 300,
					ErrorRate:  2.0,
				},
				"tools_call": {
					TotalOps:   500,
					SuccessOps: 460,
					FailureOps: 40,
					LatencyP50: 120,
					LatencyP95: 250,
					LatencyP99: 600,
					ErrorRate:  8.0,
				},
			},
			ByTool: map[string]*OperationMetrics{
				"echo": {
					TotalOps:   200,
					SuccessOps: 195,
					FailureOps: 5,
					LatencyP50: 50,
					LatencyP95: 100,
					LatencyP99: 200,
					ErrorRate:  2.5,
				},
				"add": {
					TotalOps:   300,
					SuccessOps: 265,
					FailureOps: 35,
					LatencyP50: 150,
					LatencyP95: 300,
					LatencyP99: 800,
					ErrorRate:  11.67,
				},
			},
		},
	}
}

func assertValidHTML(t *testing.T, html string) {
	t.Helper()
	if !strings.HasPrefix(html, "<!DOCTYPE html>") {
		t.Error("HTML should start with DOCTYPE")
	}
	if !strings.Contains(html, "<html") {
		t.Error("HTML should contain <html> tag")
	}
	if !strings.Contains(html, "</html>") {
		t.Error("HTML should contain closing </html> tag")
	}
	if !strings.Contains(html, "<head>") {
		t.Error("HTML should contain <head> tag")
	}
	if !strings.Contains(html, "</head>") {
		t.Error("HTML should contain closing </head> tag")
	}
	if !strings.Contains(html, "<body>") {
		t.Error("HTML should contain <body> tag")
	}
	if !strings.Contains(html, "</body>") {
		t.Error("HTML should contain closing </body> tag")
	}
}

func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("expected string to contain %q", substr)
	}
}
