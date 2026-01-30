package analysis

import "sort"

type ToolMetrics struct {
	ToolName       string  `json:"tool_name"`
	TotalCalls     int     `json:"total_calls"`
	SuccessCount   int     `json:"success_count"`
	ErrorCount     int     `json:"error_count"`
	AvgLatencyMs   float64 `json:"avg_latency_ms"`
	MinLatencyMs   float64 `json:"min_latency_ms"`
	MaxLatencyMs   float64 `json:"max_latency_ms"`
	P25LatencyMs   float64 `json:"p25_latency_ms"`
	P50LatencyMs   float64 `json:"p50_latency_ms"`
	P75LatencyMs   float64 `json:"p75_latency_ms"`
	P90LatencyMs   float64 `json:"p90_latency_ms"`
	P95LatencyMs   float64 `json:"p95_latency_ms"`
	P99LatencyMs   float64 `json:"p99_latency_ms"`
	AvgPayloadSize int     `json:"avg_payload_size"`
}

type OperationLog struct {
	Operation      string
	ToolName       string
	LatencyMs      int64
	OK             bool
	ArgumentSize   int
	ResultSize     int
	ArgumentDepth  int
	ParseError     bool
	ExecutionError bool
}

func AggregateToolMetrics(logs []OperationLog) map[string]*ToolMetrics {
	result := make(map[string]*ToolMetrics)

	toolData := make(map[string]*toolAggData)

	for _, log := range logs {
		if log.ToolName == "" {
			continue
		}

		data, exists := toolData[log.ToolName]
		if !exists {
			data = &toolAggData{
				latencies:    make([]int, 0),
				payloadSizes: make([]int, 0),
			}
			toolData[log.ToolName] = data
		}

		data.totalCalls++
		if log.OK {
			data.successCount++
		} else {
			data.errorCount++
		}

		data.latencies = append(data.latencies, int(log.LatencyMs))
		data.totalPayload += log.ArgumentSize + log.ResultSize
		data.payloadSizes = append(data.payloadSizes, log.ArgumentSize+log.ResultSize)
	}

	for toolName, data := range toolData {
		metrics := &ToolMetrics{
			ToolName:     toolName,
			TotalCalls:   data.totalCalls,
			SuccessCount: data.successCount,
			ErrorCount:   data.errorCount,
		}

		if len(data.latencies) > 0 {
			var sum int64
			minLat := data.latencies[0]
			maxLat := data.latencies[0]
			for _, lat := range data.latencies {
				sum += int64(lat)
				if lat < minLat {
					minLat = lat
				}
				if lat > maxLat {
					maxLat = lat
				}
			}
			metrics.AvgLatencyMs = float64(sum) / float64(len(data.latencies))
			metrics.MinLatencyMs = float64(minLat)
			metrics.MaxLatencyMs = float64(maxLat)
			metrics.P25LatencyMs = float64(computePercentile(data.latencies, 25))
			metrics.P50LatencyMs = float64(computePercentile(data.latencies, 50))
			metrics.P75LatencyMs = float64(computePercentile(data.latencies, 75))
			metrics.P90LatencyMs = float64(computePercentile(data.latencies, 90))
			metrics.P95LatencyMs = float64(computePercentile(data.latencies, 95))
			metrics.P99LatencyMs = float64(computePercentile(data.latencies, 99))
		}

		if data.totalCalls > 0 {
			metrics.AvgPayloadSize = data.totalPayload / data.totalCalls
		}

		result[toolName] = metrics
	}

	return result
}

type toolAggData struct {
	totalCalls   int
	successCount int
	errorCount   int
	latencies    []int
	totalPayload int
	payloadSizes []int
}

func CalculateArgumentDepth(v any) int {
	switch val := v.(type) {
	case map[string]any:
		if len(val) == 0 {
			return 1
		}
		maxChild := 0
		for _, child := range val {
			depth := CalculateArgumentDepth(child)
			if depth > maxChild {
				maxChild = depth
			}
		}
		return 1 + maxChild
	case []any:
		if len(val) == 0 {
			return 1
		}
		maxChild := 0
		for _, child := range val {
			depth := CalculateArgumentDepth(child)
			if depth > maxChild {
				maxChild = depth
			}
		}
		return 1 + maxChild
	default:
		return 0
	}
}

// ToolRegressionSummary represents a single tool's regression or improvement between two runs.
type ToolRegressionSummary struct {
	ToolName           string  `json:"tool_name"`
	LatencyP95DeltaPct float64 `json:"latency_p95_delta_pct"`
	LatencyP99DeltaPct float64 `json:"latency_p99_delta_pct"`
	ErrorRateDelta     float64 `json:"error_rate_delta"`
	RegressionSeverity string  `json:"regression_severity"`
}

// ToolComparisonReport contains the comparison results between two runs' tool metrics.
type ToolComparisonReport struct {
	RunA            string                  `json:"run_a"`
	RunB            string                  `json:"run_b"`
	TopRegressions  []ToolRegressionSummary `json:"top_regressions"`
	TopImprovements []ToolRegressionSummary `json:"top_improvements"`
}

// CompareToolMetrics compares tool metrics between two runs and identifies regressions and improvements.
// Negative delta = regression (worse), positive delta = improvement (better).
func CompareToolMetrics(runA, runB string, toolsA, toolsB map[string]*ToolMetrics) *ToolComparisonReport {
	report := &ToolComparisonReport{
		RunA:            runA,
		RunB:            runB,
		TopRegressions:  []ToolRegressionSummary{},
		TopImprovements: []ToolRegressionSummary{},
	}

	// Collect all regressions and improvements
	var allChanges []ToolRegressionSummary

	for toolName, metricsA := range toolsA {
		metricsB, exists := toolsB[toolName]
		if !exists {
			continue
		}

		// Calculate error rates
		errorRateA := 0.0
		if metricsA.TotalCalls > 0 {
			errorRateA = float64(metricsA.ErrorCount) / float64(metricsA.TotalCalls)
		}
		errorRateB := 0.0
		if metricsB.TotalCalls > 0 {
			errorRateB = float64(metricsB.ErrorCount) / float64(metricsB.TotalCalls)
		}

		// Calculate latency deltas (positive = improvement, negative = regression)
		// For latency: lower is better, so delta = (A-B)/A*100
		// If A=100ms, B=50ms: delta = 50% (improvement)
		// If A=100ms, B=150ms: delta = -50% (regression)
		p95DeltaPct := 0.0
		if metricsA.P95LatencyMs > 0 {
			p95DeltaPct = ((metricsA.P95LatencyMs - metricsB.P95LatencyMs) / metricsA.P95LatencyMs) * 100
		}

		p99DeltaPct := 0.0
		if metricsA.P99LatencyMs > 0 {
			p99DeltaPct = ((metricsA.P99LatencyMs - metricsB.P99LatencyMs) / metricsA.P99LatencyMs) * 100
		}

		errorRateDelta := errorRateB - errorRateA

		// Determine severity based on p95 regression (negative delta = worse)
		severity := "none"
		if p95DeltaPct < -50 {
			severity = "critical"
		} else if p95DeltaPct < -20 {
			severity = "warning"
		} else if p95DeltaPct < -5 {
			severity = "minor"
		}

		summary := ToolRegressionSummary{
			ToolName:           toolName,
			LatencyP95DeltaPct: p95DeltaPct,
			LatencyP99DeltaPct: p99DeltaPct,
			ErrorRateDelta:     errorRateDelta,
			RegressionSeverity: severity,
		}

		allChanges = append(allChanges, summary)
	}

	// Sort by p95 delta to identify top regressions and improvements
	// Regressions: most negative (worst) first
	// Improvements: most positive (best) first
	regressions := []ToolRegressionSummary{}
	improvements := []ToolRegressionSummary{}

	for _, change := range allChanges {
		if change.LatencyP95DeltaPct < 0 {
			regressions = append(regressions, change)
		} else if change.LatencyP95DeltaPct > 0 {
			improvements = append(improvements, change)
		}
	}

	// Sort regressions by p95 delta (ascending = worst first)
	sortByP95Asc := func(i, j int) bool {
		return regressions[i].LatencyP95DeltaPct < regressions[j].LatencyP95DeltaPct
	}
	sort.Slice(regressions, sortByP95Asc)

	// Sort improvements by p95 delta (descending = best first)
	sortByP95Desc := func(i, j int) bool {
		return improvements[i].LatencyP95DeltaPct > improvements[j].LatencyP95DeltaPct
	}
	sort.Slice(improvements, sortByP95Desc)

	// Take top 5 of each
	if len(regressions) > 5 {
		report.TopRegressions = regressions[:5]
	} else {
		report.TopRegressions = regressions
	}

	if len(improvements) > 5 {
		report.TopImprovements = improvements[:5]
	} else {
		report.TopImprovements = improvements
	}

	return report
}
