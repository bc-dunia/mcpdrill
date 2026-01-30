// Package analysis provides telemetry aggregation and metrics computation.
package analysis

import (
	"fmt"
	"strings"
	"time"
)

// ComparisonReport contains the delta metrics between two load test runs.
type ComparisonReport struct {
	RunA            string    `json:"run_a"`
	RunB            string    `json:"run_b"`
	ThroughputDelta float64   `json:"throughput_delta"`  // percentage change in RPS
	LatencyP50Delta float64   `json:"latency_p50_delta"` // percentage change
	LatencyP99Delta float64   `json:"latency_p99_delta"` // percentage change
	ErrorRateDelta  float64   `json:"error_rate_delta"`  // absolute change (B - A)
	Summary         string    `json:"summary"`
	GeneratedAt     time.Time `json:"generated_at"`

	// Detailed metrics for reference
	MetricsA *ComparisonMetricsSummary `json:"metrics_a"`
	MetricsB *ComparisonMetricsSummary `json:"metrics_b"`

	// Knee point detection results
	KneePointA *KneeDetectionResult `json:"knee_point_a,omitempty"`
	KneePointB *KneeDetectionResult `json:"knee_point_b,omitempty"`
}

// ComparisonMetricsSummary contains the key metrics from a run for comparison.
type ComparisonMetricsSummary struct {
	TotalOps   int     `json:"total_ops"`
	RPS        float64 `json:"rps"`
	LatencyP50 int     `json:"latency_p50"`
	LatencyP99 int     `json:"latency_p99"`
	ErrorRate  float64 `json:"error_rate"`
}

// CompareRuns compares two aggregated metrics and generates a comparison report.
// Positive delta means run B performed better (higher throughput, lower latency).
// Negative delta means run A performed better.
func CompareRuns(runA, runB string, metricsA, metricsB *AggregatedMetrics) *ComparisonReport {
	report := &ComparisonReport{
		RunA:        runA,
		RunB:        runB,
		GeneratedAt: time.Now().UTC(),
	}

	// Handle nil metrics
	if metricsA == nil {
		metricsA = &AggregatedMetrics{}
	}
	if metricsB == nil {
		metricsB = &AggregatedMetrics{}
	}

	// Store summaries
	report.MetricsA = &ComparisonMetricsSummary{
		TotalOps:   metricsA.TotalOps,
		RPS:        metricsA.RPS,
		LatencyP50: metricsA.LatencyP50,
		LatencyP99: metricsA.LatencyP99,
		ErrorRate:  metricsA.ErrorRate,
	}
	report.MetricsB = &ComparisonMetricsSummary{
		TotalOps:   metricsB.TotalOps,
		RPS:        metricsB.RPS,
		LatencyP50: metricsB.LatencyP50,
		LatencyP99: metricsB.LatencyP99,
		ErrorRate:  metricsB.ErrorRate,
	}

	// Calculate throughput delta (positive = B is better)
	// Higher RPS is better, so (B - A) / A * 100
	report.ThroughputDelta = calculatePercentageChange(metricsA.RPS, metricsB.RPS)

	// Calculate latency deltas (positive = B is better)
	// Lower latency is better, so we invert: (A - B) / A * 100
	report.LatencyP50Delta = calculateLatencyChange(metricsA.LatencyP50, metricsB.LatencyP50)
	report.LatencyP99Delta = calculateLatencyChange(metricsA.LatencyP99, metricsB.LatencyP99)

	// Calculate error rate delta (absolute change)
	// Lower error rate is better, so negative delta means B is better
	report.ErrorRateDelta = metricsB.ErrorRate - metricsA.ErrorRate

	// Generate summary
	report.Summary = generateComparisonSummary(report)

	return report
}

// CompareRunsWithKneeDetection compares two runs including knee point analysis.
// It accepts optional time series data and detects knee points for each run.
func CompareRunsWithKneeDetection(runA, runB string, metricsA, metricsB *AggregatedMetrics,
	timeSeriesA, timeSeriesB []TimeSeriesPoint) *ComparisonReport {
	report := CompareRuns(runA, runB, metricsA, metricsB)

	if len(timeSeriesA) > 0 {
		detector := NewKneeDetector()
		report.KneePointA = detector.DetectCombinedKnee(timeSeriesA)
	}
	if len(timeSeriesB) > 0 {
		detector := NewKneeDetector()
		report.KneePointB = detector.DetectCombinedKnee(timeSeriesB)
	}

	return report
}

// calculatePercentageChange calculates (new - old) / old * 100.
// Returns 0 if old is 0 to avoid division by zero.
func calculatePercentageChange(old, new float64) float64 {
	if old == 0 {
		if new == 0 {
			return 0
		}
		return 100 // Infinite improvement, cap at 100%
	}
	return ((new - old) / old) * 100
}

// calculateLatencyChange calculates latency improvement percentage.
// Lower latency is better, so positive result means B is better.
// Returns (old - new) / old * 100.
func calculateLatencyChange(old, new int) float64 {
	if old == 0 {
		if new == 0 {
			return 0
		}
		return -100 // Regression from 0, cap at -100%
	}
	return (float64(old-new) / float64(old)) * 100
}

// generateComparisonSummary creates a human-readable summary of the comparison.
func generateComparisonSummary(report *ComparisonReport) string {
	var improvements, regressions []string

	// Throughput
	if report.ThroughputDelta > 5 {
		improvements = append(improvements, fmt.Sprintf("throughput +%.1f%%", report.ThroughputDelta))
	} else if report.ThroughputDelta < -5 {
		regressions = append(regressions, fmt.Sprintf("throughput %.1f%%", report.ThroughputDelta))
	}

	// Latency P50
	if report.LatencyP50Delta > 5 {
		improvements = append(improvements, fmt.Sprintf("p50 latency +%.1f%%", report.LatencyP50Delta))
	} else if report.LatencyP50Delta < -5 {
		regressions = append(regressions, fmt.Sprintf("p50 latency %.1f%%", report.LatencyP50Delta))
	}

	// Latency P99
	if report.LatencyP99Delta > 5 {
		improvements = append(improvements, fmt.Sprintf("p99 latency +%.1f%%", report.LatencyP99Delta))
	} else if report.LatencyP99Delta < -5 {
		regressions = append(regressions, fmt.Sprintf("p99 latency %.1f%%", report.LatencyP99Delta))
	}

	// Error rate (absolute change) - threshold should be 0.01 (1% as fraction)
	if report.ErrorRateDelta < -0.01 {
		improvements = append(improvements, fmt.Sprintf("error rate %.2f%%", report.ErrorRateDelta*100))
	} else if report.ErrorRateDelta > 0.01 {
		regressions = append(regressions, fmt.Sprintf("error rate +%.2f%%", report.ErrorRateDelta*100))
	}

	if len(improvements) == 0 && len(regressions) == 0 {
		return "No significant differences between runs"
	}

	summary := ""
	if len(improvements) > 0 {
		summary = fmt.Sprintf("Improvements in run B: %s", joinStrings(improvements))
	}
	if len(regressions) > 0 {
		if summary != "" {
			summary += ". "
		}
		summary += fmt.Sprintf("Regressions in run B: %s", joinStrings(regressions))
	}

	return summary
}

func joinStrings(strs []string) string {
	if len(strs) == 0 {
		return ""
	}
	if len(strs) == 1 {
		return strs[0]
	}
	var b strings.Builder
	b.WriteString(strs[0])
	for i := 1; i < len(strs); i++ {
		b.WriteString(", ")
		b.WriteString(strs[i])
	}
	return b.String()
}

// FormatComparisonText formats the comparison report as human-readable text.
func FormatComparisonText(report *ComparisonReport) string {
	text := fmt.Sprintf(`A/B Comparison Report
=====================
Run A: %s
Run B: %s
Generated: %s

Throughput (RPS)
  Run A: %.2f
  Run B: %.2f
  Delta: %+.1f%%

Latency P50 (ms)
  Run A: %d
  Run B: %d
  Delta: %+.1f%% (positive = improvement)

Latency P99 (ms)
  Run A: %d
  Run B: %d
  Delta: %+.1f%% (positive = improvement)

Error Rate
  Run A: %.2f%%
  Run B: %.2f%%
  Delta: %+.2f%% (negative = improvement)

Summary
-------
%s
`,
		report.RunA,
		report.RunB,
		report.GeneratedAt.Format(time.RFC3339),
		report.MetricsA.RPS,
		report.MetricsB.RPS,
		report.ThroughputDelta,
		report.MetricsA.LatencyP50,
		report.MetricsB.LatencyP50,
		report.LatencyP50Delta,
		report.MetricsA.LatencyP99,
		report.MetricsB.LatencyP99,
		report.LatencyP99Delta,
		report.MetricsA.ErrorRate*100,
		report.MetricsB.ErrorRate*100,
		report.ErrorRateDelta*100,
		report.Summary,
	)

	if report.KneePointA != nil || report.KneePointB != nil {
		text += "\nKnee Point Analysis\n-------------------\n"
		if report.KneePointA != nil {
			text += formatKneePointInfo("Run A", report.KneePointA)
		}
		if report.KneePointB != nil {
			text += formatKneePointInfo("Run B", report.KneePointB)
		}
	}

	return text
}

func formatKneePointInfo(runName string, result *KneeDetectionResult) string {
	if !result.Detected {
		return fmt.Sprintf("%s: No knee point detected (%s)\n", runName, result.AnalysisDetails)
	}

	return fmt.Sprintf("%s: Knee detected at load level %d\n  Metric: %s\n  Value: %.2f\n  Change Ratio: %.2f\n  Significance: %.2f\n",
		runName,
		result.KneePoint.LoadLevel,
		result.Metric,
		result.KneePoint.MetricValue,
		result.KneePoint.ChangeRatio,
		result.KneePoint.Significance,
	)
}
