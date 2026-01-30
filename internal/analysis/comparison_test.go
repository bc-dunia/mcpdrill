package analysis

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCompareRuns_BasicComparison(t *testing.T) {
	metricsA := &AggregatedMetrics{
		TotalOps:   1000,
		SuccessOps: 950,
		FailureOps: 50,
		RPS:        100.0,
		LatencyP50: 50,
		LatencyP99: 200,
		ErrorRate:  0.05,
	}

	metricsB := &AggregatedMetrics{
		TotalOps:   1200,
		SuccessOps: 1180,
		FailureOps: 20,
		RPS:        120.0,
		LatencyP50: 40,
		LatencyP99: 150,
		ErrorRate:  0.0167,
	}

	report := CompareRuns("run_000000000000000a", "run_000000000000000b", metricsA, metricsB)

	if report.RunA != "run_000000000000000a" {
		t.Errorf("expected RunA='run_a', got %s", report.RunA)
	}
	if report.RunB != "run_000000000000000b" {
		t.Errorf("expected RunB='run_b', got %s", report.RunB)
	}

	// Throughput: (120 - 100) / 100 * 100 = 20%
	if report.ThroughputDelta < 19.9 || report.ThroughputDelta > 20.1 {
		t.Errorf("expected ThroughputDelta ~20%%, got %.2f%%", report.ThroughputDelta)
	}

	// Latency P50: (50 - 40) / 50 * 100 = 20% improvement
	if report.LatencyP50Delta < 19.9 || report.LatencyP50Delta > 20.1 {
		t.Errorf("expected LatencyP50Delta ~20%%, got %.2f%%", report.LatencyP50Delta)
	}

	// Latency P99: (200 - 150) / 200 * 100 = 25% improvement
	if report.LatencyP99Delta < 24.9 || report.LatencyP99Delta > 25.1 {
		t.Errorf("expected LatencyP99Delta ~25%%, got %.2f%%", report.LatencyP99Delta)
	}

	// Error rate: 0.0167 - 0.05 = -0.0333 (improvement)
	if report.ErrorRateDelta > -0.033 || report.ErrorRateDelta < -0.034 {
		t.Errorf("expected ErrorRateDelta ~-0.0333, got %.4f", report.ErrorRateDelta)
	}

	if report.MetricsA == nil || report.MetricsB == nil {
		t.Error("expected MetricsA and MetricsB to be populated")
	}
}

func TestCompareRuns_NilMetrics(t *testing.T) {
	report := CompareRuns("run_000000000000000a", "run_000000000000000b", nil, nil)

	if report.RunA != "run_000000000000000a" || report.RunB != "run_000000000000000b" {
		t.Error("expected run IDs to be set")
	}

	if report.ThroughputDelta != 0 {
		t.Errorf("expected ThroughputDelta=0 for nil metrics, got %.2f", report.ThroughputDelta)
	}

	if report.MetricsA == nil || report.MetricsB == nil {
		t.Error("expected MetricsA and MetricsB to be initialized")
	}
}

func TestCompareRuns_Regression(t *testing.T) {
	metricsA := &AggregatedMetrics{
		RPS:        100.0,
		LatencyP50: 50,
		LatencyP99: 200,
		ErrorRate:  0.01,
	}

	metricsB := &AggregatedMetrics{
		RPS:        80.0,
		LatencyP50: 70,
		LatencyP99: 300,
		ErrorRate:  0.05,
	}

	report := CompareRuns("run_000000000000000a", "run_000000000000000b", metricsA, metricsB)

	// Throughput regression: (80 - 100) / 100 * 100 = -20%
	if report.ThroughputDelta > -19.9 || report.ThroughputDelta < -20.1 {
		t.Errorf("expected ThroughputDelta ~-20%%, got %.2f%%", report.ThroughputDelta)
	}

	// Latency P50 regression: (50 - 70) / 50 * 100 = -40%
	if report.LatencyP50Delta > -39.9 || report.LatencyP50Delta < -40.1 {
		t.Errorf("expected LatencyP50Delta ~-40%%, got %.2f%%", report.LatencyP50Delta)
	}

	// Error rate regression: 0.05 - 0.01 = 0.04
	if report.ErrorRateDelta < 0.039 || report.ErrorRateDelta > 0.041 {
		t.Errorf("expected ErrorRateDelta ~0.04, got %.4f", report.ErrorRateDelta)
	}

	if !strings.Contains(report.Summary, "Regressions") {
		t.Errorf("expected summary to mention regressions, got: %s", report.Summary)
	}
}

func TestCompareRuns_NoSignificantDifference(t *testing.T) {
	metricsA := &AggregatedMetrics{
		RPS:        100.0,
		LatencyP50: 50,
		LatencyP99: 200,
		ErrorRate:  0.02,
	}

	metricsB := &AggregatedMetrics{
		RPS:        101.0,
		LatencyP50: 49,
		LatencyP99: 198,
		ErrorRate:  0.021,
	}

	report := CompareRuns("run_000000000000000a", "run_000000000000000b", metricsA, metricsB)

	if !strings.Contains(report.Summary, "No significant differences") {
		t.Errorf("expected 'No significant differences' in summary, got: %s", report.Summary)
	}
}

func TestCompareRuns_ZeroBaseline(t *testing.T) {
	metricsA := &AggregatedMetrics{
		RPS:        0,
		LatencyP50: 0,
		LatencyP99: 0,
		ErrorRate:  0,
	}

	metricsB := &AggregatedMetrics{
		RPS:        100.0,
		LatencyP50: 50,
		LatencyP99: 200,
		ErrorRate:  0.05,
	}

	report := CompareRuns("run_000000000000000a", "run_000000000000000b", metricsA, metricsB)

	// Should handle division by zero gracefully
	if report.ThroughputDelta != 100 {
		t.Errorf("expected ThroughputDelta=100 for zero baseline, got %.2f", report.ThroughputDelta)
	}

	// Latency from 0 to positive is a regression
	if report.LatencyP50Delta != -100 {
		t.Errorf("expected LatencyP50Delta=-100 for zero baseline, got %.2f", report.LatencyP50Delta)
	}
}

func TestFormatComparisonText(t *testing.T) {
	metricsA := &AggregatedMetrics{
		RPS:        100.0,
		LatencyP50: 50,
		LatencyP99: 200,
		ErrorRate:  0.05,
	}

	metricsB := &AggregatedMetrics{
		RPS:        120.0,
		LatencyP50: 40,
		LatencyP99: 150,
		ErrorRate:  0.02,
	}

	report := CompareRuns("run_000000000000000a", "run_000000000000000b", metricsA, metricsB)
	text := FormatComparisonText(report)

	expectedStrings := []string{
		"A/B Comparison Report",
		"Run A: run_000000000000000a",
		"Run B: run_000000000000000b",
		"Throughput (RPS)",
		"Latency P50",
		"Latency P99",
		"Error Rate",
		"Summary",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(text, expected) {
			t.Errorf("expected text to contain %q", expected)
		}
	}
}

func TestComparisonReport_JSONSerialization(t *testing.T) {
	metricsA := &AggregatedMetrics{
		RPS:        100.0,
		LatencyP50: 50,
		LatencyP99: 200,
		ErrorRate:  0.05,
	}

	metricsB := &AggregatedMetrics{
		RPS:        120.0,
		LatencyP50: 40,
		LatencyP99: 150,
		ErrorRate:  0.02,
	}

	report := CompareRuns("run_000000000000000a", "run_000000000000000b", metricsA, metricsB)

	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("failed to marshal report: %v", err)
	}

	var decoded ComparisonReport
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal report: %v", err)
	}

	if decoded.RunA != report.RunA || decoded.RunB != report.RunB {
		t.Error("run IDs not preserved after JSON round-trip")
	}

	if decoded.ThroughputDelta != report.ThroughputDelta {
		t.Errorf("ThroughputDelta not preserved: got %.2f, want %.2f", decoded.ThroughputDelta, report.ThroughputDelta)
	}
}

func TestCalculatePercentageChange(t *testing.T) {
	tests := []struct {
		name     string
		old      float64
		new      float64
		expected float64
	}{
		{"increase", 100, 120, 20},
		{"decrease", 100, 80, -20},
		{"no change", 100, 100, 0},
		{"zero old positive new", 0, 100, 100},
		{"zero both", 0, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculatePercentageChange(tt.old, tt.new)
			if result != tt.expected {
				t.Errorf("calculatePercentageChange(%v, %v) = %v, want %v", tt.old, tt.new, result, tt.expected)
			}
		})
	}
}

func TestCalculateLatencyChange(t *testing.T) {
	tests := []struct {
		name     string
		old      int
		new      int
		expected float64
	}{
		{"improvement", 100, 80, 20},
		{"regression", 100, 120, -20},
		{"no change", 100, 100, 0},
		{"zero old positive new", 0, 100, -100},
		{"zero both", 0, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateLatencyChange(tt.old, tt.new)
			if result != tt.expected {
				t.Errorf("calculateLatencyChange(%v, %v) = %v, want %v", tt.old, tt.new, result, tt.expected)
			}
		})
	}
}
