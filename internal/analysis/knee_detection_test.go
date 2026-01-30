package analysis

import (
	"math"
	"testing"
)

func TestNewKneeDetector(t *testing.T) {
	detector := NewKneeDetector()

	if detector.minDataPoints != 5 {
		t.Errorf("NewKneeDetector() minDataPoints = %d, want 5", detector.minDataPoints)
	}
	if detector.changeRatioThreshold != 1.5 {
		t.Errorf("NewKneeDetector() changeRatioThreshold = %f, want 1.5", detector.changeRatioThreshold)
	}
	if detector.errorRateThreshold != 0.05 {
		t.Errorf("NewKneeDetector() errorRateThreshold = %f, want 0.05", detector.errorRateThreshold)
	}
}

func TestKneeDetector_SetMinDataPoints(t *testing.T) {
	tests := []struct {
		name     string
		input    int
		expected int
	}{
		{
			name:     "valid positive value",
			input:    10,
			expected: 10,
		},
		{
			name:     "minimum valid value",
			input:    1,
			expected: 1,
		},
		{
			name:     "large value",
			input:    1000,
			expected: 1000,
		},
		{
			name:     "zero ignored",
			input:    0,
			expected: 5, // default unchanged
		},
		{
			name:     "negative ignored",
			input:    -5,
			expected: 5, // default unchanged
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detector := NewKneeDetector()
			detector.SetMinDataPoints(tt.input)
			if detector.minDataPoints != tt.expected {
				t.Errorf("SetMinDataPoints(%d) resulted in %d, want %d", tt.input, detector.minDataPoints, tt.expected)
			}
		})
	}
}

func TestKneeDetector_SetChangeRatioThreshold(t *testing.T) {
	tests := []struct {
		name     string
		input    float64
		expected float64
	}{
		{
			name:     "valid ratio above 1.0",
			input:    2.0,
			expected: 2.0,
		},
		{
			name:     "ratio just above 1.0",
			input:    1.01,
			expected: 1.01,
		},
		{
			name:     "large ratio",
			input:    10.5,
			expected: 10.5,
		},
		{
			name:     "ratio at 1.0 ignored",
			input:    1.0,
			expected: 1.5, // default unchanged
		},
		{
			name:     "ratio below 1.0 ignored",
			input:    0.5,
			expected: 1.5, // default unchanged
		},
		{
			name:     "negative ratio ignored",
			input:    -2.0,
			expected: 1.5, // default unchanged
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detector := NewKneeDetector()
			detector.SetChangeRatioThreshold(tt.input)
			if detector.changeRatioThreshold != tt.expected {
				t.Errorf("SetChangeRatioThreshold(%f) resulted in %f, want %f", tt.input, detector.changeRatioThreshold, tt.expected)
			}
		})
	}
}

func TestKneeDetector_SetErrorRateThreshold(t *testing.T) {
	tests := []struct {
		name     string
		input    float64
		expected float64
	}{
		{
			name:     "valid threshold 0.05",
			input:    0.05,
			expected: 0.05,
		},
		{
			name:     "threshold near 0",
			input:    0.001,
			expected: 0.001,
		},
		{
			name:     "threshold near 1.0",
			input:    0.99,
			expected: 0.99,
		},
		{
			name:     "threshold at 0 ignored",
			input:    0.0,
			expected: 0.05, // default unchanged
		},
		{
			name:     "threshold at 1.0 ignored",
			input:    1.0,
			expected: 0.05, // default unchanged
		},
		{
			name:     "threshold above 1.0 ignored",
			input:    1.5,
			expected: 0.05, // default unchanged
		},
		{
			name:     "negative threshold ignored",
			input:    -0.1,
			expected: 0.05, // default unchanged
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detector := NewKneeDetector()
			detector.SetErrorRateThreshold(tt.input)
			if detector.errorRateThreshold != tt.expected {
				t.Errorf("SetErrorRateThreshold(%f) resulted in %f, want %f", tt.input, detector.errorRateThreshold, tt.expected)
			}
		})
	}
}

func TestDetectLatencyKnee_InsufficientData(t *testing.T) {
	detector := NewKneeDetector()
	detector.SetMinDataPoints(5)

	tests := []struct {
		name      string
		dataCount int
	}{
		{
			name:      "no data",
			dataCount: 0,
		},
		{
			name:      "one point",
			dataCount: 1,
		},
		{
			name:      "below minimum",
			dataCount: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			points := make([]TimeSeriesPoint, tt.dataCount)
			for i := 0; i < tt.dataCount; i++ {
				points[i] = TimeSeriesPoint{
					LoadLevel:  10 + i*10,
					LatencyP95: 100 + i*10,
					LatencyP99: 120 + i*10,
				}
			}

			result := detector.DetectLatencyKnee(points, false)

			if result.Detected {
				t.Errorf("DetectLatencyKnee with %d points should not detect knee", tt.dataCount)
			}
			if result.AnalysisDetails != "insufficient data points" {
				t.Errorf("AnalysisDetails = %q, want 'insufficient data points'", result.AnalysisDetails)
			}
			if result.DataPoints != tt.dataCount {
				t.Errorf("DataPoints = %d, want %d", result.DataPoints, tt.dataCount)
			}
		})
	}
}

func TestDetectLatencyKnee_NoSignificantInflection(t *testing.T) {
	detector := NewKneeDetector()

	// Flat data - no significant inflection
	points := []TimeSeriesPoint{
		{LoadLevel: 10, LatencyP95: 100, LatencyP99: 120},
		{LoadLevel: 20, LatencyP95: 101, LatencyP99: 121},
		{LoadLevel: 30, LatencyP95: 102, LatencyP99: 122},
		{LoadLevel: 40, LatencyP95: 103, LatencyP99: 123},
		{LoadLevel: 50, LatencyP95: 104, LatencyP99: 124},
	}

	result := detector.DetectLatencyKnee(points, false)

	if result.Detected {
		t.Errorf("DetectLatencyKnee with flat data should not detect knee")
	}
	if result.AnalysisDetails != "no significant inflection point found" {
		t.Errorf("AnalysisDetails = %q, want 'no significant inflection point found'", result.AnalysisDetails)
	}
}

func TestDetectLatencyKnee_ClearKnee(t *testing.T) {
	detector := NewKneeDetector()

	// Hockey-stick curve: flat then sharp increase
	points := []TimeSeriesPoint{
		{LoadLevel: 10, LatencyP95: 100, LatencyP99: 120},
		{LoadLevel: 20, LatencyP95: 105, LatencyP99: 125},
		{LoadLevel: 30, LatencyP95: 110, LatencyP99: 130},
		{LoadLevel: 40, LatencyP95: 250, LatencyP99: 300},
		{LoadLevel: 50, LatencyP95: 400, LatencyP99: 500},
	}

	result := detector.DetectLatencyKnee(points, false)

	if !result.Detected {
		t.Errorf("DetectLatencyKnee with hockey-stick curve should detect knee")
	}
	if result.KneePoint == nil {
		t.Fatalf("KneePoint is nil")
	}
	if result.AnalysisDetails != "latency inflection detected" {
		t.Errorf("AnalysisDetails = %q, want 'latency inflection detected'", result.AnalysisDetails)
	}
	if result.KneePoint.ChangeRatio < detector.changeRatioThreshold {
		t.Errorf("ChangeRatio = %f, want >= %f", result.KneePoint.ChangeRatio, detector.changeRatioThreshold)
	}
}

func TestDetectLatencyKnee_P99(t *testing.T) {
	detector := NewKneeDetector()

	// Hockey-stick curve with P99 values
	points := []TimeSeriesPoint{
		{LoadLevel: 10, LatencyP95: 100, LatencyP99: 120},
		{LoadLevel: 20, LatencyP95: 105, LatencyP99: 125},
		{LoadLevel: 30, LatencyP95: 110, LatencyP99: 130},
		{LoadLevel: 40, LatencyP95: 250, LatencyP99: 350},
		{LoadLevel: 50, LatencyP95: 400, LatencyP99: 600},
	}

	resultP95 := detector.DetectLatencyKnee(points, false)
	resultP99 := detector.DetectLatencyKnee(points, true)

	if !resultP95.Detected {
		t.Errorf("DetectLatencyKnee with useP99=false should detect knee")
	}
	if resultP95.Metric != "latency_p95" {
		t.Errorf("Metric = %q, want 'latency_p95'", resultP95.Metric)
	}

	if !resultP99.Detected {
		t.Errorf("DetectLatencyKnee with useP99=true should detect knee")
	}
	if resultP99.Metric != "latency_p99" {
		t.Errorf("Metric = %q, want 'latency_p99'", resultP99.Metric)
	}

	// P99 values are higher, so knee point metric value should be higher
	if resultP99.KneePoint.MetricValue <= resultP95.KneePoint.MetricValue {
		t.Errorf("P99 metric value should be higher than P95")
	}
}

func TestDetectLatencyKnee_UnsortedInput(t *testing.T) {
	detector := NewKneeDetector()

	// Unsorted points - should be sorted by LoadLevel internally
	points := []TimeSeriesPoint{
		{LoadLevel: 50, LatencyP95: 400, LatencyP99: 500},
		{LoadLevel: 10, LatencyP95: 100, LatencyP99: 120},
		{LoadLevel: 30, LatencyP95: 110, LatencyP99: 130},
		{LoadLevel: 40, LatencyP95: 250, LatencyP99: 300},
		{LoadLevel: 20, LatencyP95: 105, LatencyP99: 125},
	}

	result := detector.DetectLatencyKnee(points, false)

	if !result.Detected {
		t.Errorf("DetectLatencyKnee should handle unsorted input")
	}
	if result.KneePoint == nil {
		t.Fatalf("KneePoint is nil")
	}
	// Knee should be detected at load level 40 (where inflection occurs)
	if result.KneePoint.LoadLevel < 30 || result.KneePoint.LoadLevel > 50 {
		t.Errorf("KneePoint LoadLevel = %d, expected in range [30, 50]", result.KneePoint.LoadLevel)
	}
}

func TestDetectErrorRateKnee_BelowThreshold(t *testing.T) {
	detector := NewKneeDetector()
	detector.SetErrorRateThreshold(0.05)

	// All error rates below threshold
	points := []TimeSeriesPoint{
		{LoadLevel: 10, ErrorRate: 0.001},
		{LoadLevel: 20, ErrorRate: 0.002},
		{LoadLevel: 30, ErrorRate: 0.003},
		{LoadLevel: 40, ErrorRate: 0.004},
		{LoadLevel: 50, ErrorRate: 0.005},
	}

	result := detector.DetectErrorRateKnee(points)

	if result.Detected {
		t.Errorf("DetectErrorRateKnee should not detect knee when all rates below threshold")
	}
	if result.AnalysisDetails != "error rate below threshold throughout" {
		t.Errorf("AnalysisDetails = %q, want 'error rate below threshold throughout'", result.AnalysisDetails)
	}
	if result.Threshold != 0.05 {
		t.Errorf("Threshold = %f, want 0.05", result.Threshold)
	}
}

func TestDetectErrorRateKnee_ExceedsThreshold(t *testing.T) {
	detector := NewKneeDetector()
	detector.SetErrorRateThreshold(0.05)

	// Error rate crosses threshold
	points := []TimeSeriesPoint{
		{LoadLevel: 10, ErrorRate: 0.001},
		{LoadLevel: 20, ErrorRate: 0.002},
		{LoadLevel: 30, ErrorRate: 0.03},
		{LoadLevel: 40, ErrorRate: 0.08},
		{LoadLevel: 50, ErrorRate: 0.15},
	}

	result := detector.DetectErrorRateKnee(points)

	if !result.Detected {
		t.Errorf("DetectErrorRateKnee should detect knee when threshold exceeded")
	}
	if result.KneePoint == nil {
		t.Fatalf("KneePoint is nil")
	}
	if result.AnalysisDetails != "error rate threshold exceeded" {
		t.Errorf("AnalysisDetails = %q, want 'error rate threshold exceeded'", result.AnalysisDetails)
	}
	if result.KneePoint.LoadLevel != 40 {
		t.Errorf("KneePoint LoadLevel = %d, want 40 (first to exceed threshold)", result.KneePoint.LoadLevel)
	}
	if result.KneePoint.MetricValue != 0.08 {
		t.Errorf("KneePoint MetricValue = %f, want 0.08", result.KneePoint.MetricValue)
	}
	// Significance should be MetricValue / threshold
	expectedSignificance := 0.08 / 0.05
	if math.Abs(result.KneePoint.Significance-expectedSignificance) > 0.001 {
		t.Errorf("Significance = %f, want %f", result.KneePoint.Significance, expectedSignificance)
	}
}

func TestDetectErrorRateKnee_ChangeRatio(t *testing.T) {
	detector := NewKneeDetector()
	detector.SetErrorRateThreshold(0.05)

	tests := []struct {
		name                string
		points              []TimeSeriesPoint
		expectedChangeRatio float64
	}{
		{
			name: "normal increase",
			points: []TimeSeriesPoint{
				{LoadLevel: 10, ErrorRate: 0.01},
				{LoadLevel: 20, ErrorRate: 0.02},
				{LoadLevel: 30, ErrorRate: 0.03},
				{LoadLevel: 40, ErrorRate: 0.10},
				{LoadLevel: 50, ErrorRate: 0.15},
			},
			expectedChangeRatio: 0.10 / 0.03, // 3.33
		},
		{
			name: "zero to threshold",
			points: []TimeSeriesPoint{
				{LoadLevel: 10, ErrorRate: 0.0},
				{LoadLevel: 20, ErrorRate: 0.0},
				{LoadLevel: 30, ErrorRate: 0.0},
				{LoadLevel: 40, ErrorRate: 0.10},
				{LoadLevel: 50, ErrorRate: 0.15},
			},
			expectedChangeRatio: math.Inf(1), // infinity when previous is 0
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.DetectErrorRateKnee(tt.points)
			if !result.Detected {
				t.Fatalf("DetectErrorRateKnee should detect knee")
			}
			if math.IsInf(tt.expectedChangeRatio, 1) {
				if !math.IsInf(result.KneePoint.ChangeRatio, 1) {
					t.Errorf("ChangeRatio should be infinity, got %f", result.KneePoint.ChangeRatio)
				}
			} else {
				if math.Abs(result.KneePoint.ChangeRatio-tt.expectedChangeRatio) > 0.01 {
					t.Errorf("ChangeRatio = %f, want %f", result.KneePoint.ChangeRatio, tt.expectedChangeRatio)
				}
			}
		})
	}
}

func TestDetectErrorRateKnee_UnsortedInput(t *testing.T) {
	detector := NewKneeDetector()
	detector.SetErrorRateThreshold(0.05)

	// Unsorted points
	points := []TimeSeriesPoint{
		{LoadLevel: 50, ErrorRate: 0.15},
		{LoadLevel: 10, ErrorRate: 0.001},
		{LoadLevel: 40, ErrorRate: 0.08},
		{LoadLevel: 20, ErrorRate: 0.002},
		{LoadLevel: 30, ErrorRate: 0.03},
	}

	result := detector.DetectErrorRateKnee(points)

	if !result.Detected {
		t.Errorf("DetectErrorRateKnee should handle unsorted input")
	}
	if result.KneePoint == nil {
		t.Fatalf("KneePoint is nil")
	}
	// Should find first point where error rate >= threshold (at load level 40)
	if result.KneePoint.LoadLevel != 40 {
		t.Errorf("KneePoint LoadLevel = %d, want 40", result.KneePoint.LoadLevel)
	}
}

func TestDetectErrorRateKnee_InsufficientData(t *testing.T) {
	detector := NewKneeDetector()
	detector.SetMinDataPoints(5)

	points := []TimeSeriesPoint{
		{LoadLevel: 10, ErrorRate: 0.001},
		{LoadLevel: 20, ErrorRate: 0.002},
		{LoadLevel: 30, ErrorRate: 0.10},
	}

	result := detector.DetectErrorRateKnee(points)

	if result.Detected {
		t.Errorf("DetectErrorRateKnee should not detect with insufficient data")
	}
	if result.AnalysisDetails != "insufficient data points" {
		t.Errorf("AnalysisDetails = %q, want 'insufficient data points'", result.AnalysisDetails)
	}
}

func TestDetectCombinedKnee_ErrorPriority(t *testing.T) {
	detector := NewKneeDetector()

	// Both latency and error rate knees present - error should take priority
	points := []TimeSeriesPoint{
		{LoadLevel: 10, LatencyP95: 100, LatencyP99: 120, ErrorRate: 0.001},
		{LoadLevel: 20, LatencyP95: 105, LatencyP99: 125, ErrorRate: 0.002},
		{LoadLevel: 30, LatencyP95: 110, LatencyP99: 130, ErrorRate: 0.03},
		{LoadLevel: 40, LatencyP95: 250, LatencyP99: 300, ErrorRate: 0.08},
		{LoadLevel: 50, LatencyP95: 400, LatencyP99: 500, ErrorRate: 0.15},
	}

	result := detector.DetectCombinedKnee(points)

	if !result.Detected {
		t.Errorf("DetectCombinedKnee should detect knee")
	}
	if result.Metric != "error_rate" {
		t.Errorf("Metric = %q, want 'error_rate' (error takes priority)", result.Metric)
	}
	if result.AnalysisDetails != "error rate threshold exceeded" {
		t.Errorf("AnalysisDetails = %q, want 'error rate threshold exceeded'", result.AnalysisDetails)
	}
}

func TestDetectCombinedKnee_LatencyFallback(t *testing.T) {
	detector := NewKneeDetector()

	// Only latency knee, no error rate knee
	points := []TimeSeriesPoint{
		{LoadLevel: 10, LatencyP95: 100, LatencyP99: 120, ErrorRate: 0.001},
		{LoadLevel: 20, LatencyP95: 105, LatencyP99: 125, ErrorRate: 0.002},
		{LoadLevel: 30, LatencyP95: 110, LatencyP99: 130, ErrorRate: 0.003},
		{LoadLevel: 40, LatencyP95: 250, LatencyP99: 300, ErrorRate: 0.004},
		{LoadLevel: 50, LatencyP95: 400, LatencyP99: 500, ErrorRate: 0.005},
	}

	result := detector.DetectCombinedKnee(points)

	if !result.Detected {
		t.Errorf("DetectCombinedKnee should detect latency knee when no error knee")
	}
	if result.Metric != "latency_p99" {
		t.Errorf("Metric = %q, want 'latency_p99' (uses P99 for combined)", result.Metric)
	}
	if result.AnalysisDetails != "latency inflection detected" {
		t.Errorf("AnalysisDetails = %q, want 'latency inflection detected'", result.AnalysisDetails)
	}
}

func TestDetectCombinedKnee_NoKnee(t *testing.T) {
	detector := NewKneeDetector()

	// Flat data - no knee detected
	points := []TimeSeriesPoint{
		{LoadLevel: 10, LatencyP95: 100, LatencyP99: 120, ErrorRate: 0.001},
		{LoadLevel: 20, LatencyP95: 101, LatencyP99: 121, ErrorRate: 0.002},
		{LoadLevel: 30, LatencyP95: 102, LatencyP99: 122, ErrorRate: 0.003},
		{LoadLevel: 40, LatencyP95: 103, LatencyP99: 123, ErrorRate: 0.004},
		{LoadLevel: 50, LatencyP95: 104, LatencyP99: 124, ErrorRate: 0.005},
	}

	result := detector.DetectCombinedKnee(points)

	if result.Detected {
		t.Errorf("DetectCombinedKnee should not detect knee with flat data")
	}
	if result.Metric != "combined" {
		t.Errorf("Metric = %q, want 'combined'", result.Metric)
	}
	if result.AnalysisDetails != "no knee point detected" {
		t.Errorf("AnalysisDetails = %q, want 'no knee point detected'", result.AnalysisDetails)
	}
}

func TestDetectCombinedKnee_DataPointsCount(t *testing.T) {
	detector := NewKneeDetector()

	points := []TimeSeriesPoint{
		{LoadLevel: 10, LatencyP95: 100, LatencyP99: 120, ErrorRate: 0.001},
		{LoadLevel: 20, LatencyP95: 105, LatencyP99: 125, ErrorRate: 0.002},
		{LoadLevel: 30, LatencyP95: 110, LatencyP99: 130, ErrorRate: 0.03},
		{LoadLevel: 40, LatencyP95: 250, LatencyP99: 300, ErrorRate: 0.08},
		{LoadLevel: 50, LatencyP95: 400, LatencyP99: 500, ErrorRate: 0.15},
	}

	result := detector.DetectCombinedKnee(points)

	if result.DataPoints != 5 {
		t.Errorf("DataPoints = %d, want 5", result.DataPoints)
	}
}

func TestDetectLatencyKnee_ChangeRatioBelowThreshold(t *testing.T) {
	detector := NewKneeDetector()
	detector.SetChangeRatioThreshold(2.0)

	// Gentle increase - change ratio below threshold
	points := []TimeSeriesPoint{
		{LoadLevel: 10, LatencyP95: 100, LatencyP99: 120},
		{LoadLevel: 20, LatencyP95: 110, LatencyP99: 130},
		{LoadLevel: 30, LatencyP95: 120, LatencyP99: 140},
		{LoadLevel: 40, LatencyP95: 130, LatencyP99: 150},
		{LoadLevel: 50, LatencyP95: 140, LatencyP99: 160},
	}

	result := detector.DetectLatencyKnee(points, false)

	if result.Detected {
		t.Errorf("DetectLatencyKnee should not detect when change ratio below threshold")
	}
	if result.AnalysisDetails != "no significant inflection point found" {
		t.Errorf("AnalysisDetails = %q, want 'no significant inflection point found'", result.AnalysisDetails)
	}
}

func TestDetectLatencyKnee_Threshold(t *testing.T) {
	detector := NewKneeDetector()

	points := []TimeSeriesPoint{
		{LoadLevel: 10, LatencyP95: 100, LatencyP99: 120},
		{LoadLevel: 20, LatencyP95: 105, LatencyP99: 125},
		{LoadLevel: 30, LatencyP95: 110, LatencyP99: 130},
		{LoadLevel: 40, LatencyP95: 250, LatencyP99: 300},
		{LoadLevel: 50, LatencyP95: 400, LatencyP99: 500},
	}

	result := detector.DetectLatencyKnee(points, false)

	if result.Threshold != detector.changeRatioThreshold {
		t.Errorf("Threshold = %f, want %f", result.Threshold, detector.changeRatioThreshold)
	}
}

func TestDetectErrorRateKnee_FirstExceedance(t *testing.T) {
	detector := NewKneeDetector()
	detector.SetErrorRateThreshold(0.05)

	// Error rate exceeds threshold at load level 30
	points := []TimeSeriesPoint{
		{LoadLevel: 10, ErrorRate: 0.001},
		{LoadLevel: 20, ErrorRate: 0.02},
		{LoadLevel: 30, ErrorRate: 0.06},
		{LoadLevel: 40, ErrorRate: 0.10},
		{LoadLevel: 50, ErrorRate: 0.20},
	}

	result := detector.DetectErrorRateKnee(points)

	if !result.Detected {
		t.Errorf("DetectErrorRateKnee should detect knee")
	}
	if result.KneePoint.LoadLevel != 30 {
		t.Errorf("KneePoint LoadLevel = %d, want 30 (first to exceed)", result.KneePoint.LoadLevel)
	}
	if result.KneePoint.MetricValue != 0.06 {
		t.Errorf("KneePoint MetricValue = %f, want 0.06", result.KneePoint.MetricValue)
	}
}

func TestDetectLatencyKnee_Significance(t *testing.T) {
	detector := NewKneeDetector()

	points := []TimeSeriesPoint{
		{LoadLevel: 10, LatencyP95: 100, LatencyP99: 120},
		{LoadLevel: 20, LatencyP95: 105, LatencyP99: 125},
		{LoadLevel: 30, LatencyP95: 110, LatencyP99: 130},
		{LoadLevel: 40, LatencyP95: 250, LatencyP99: 300},
		{LoadLevel: 50, LatencyP95: 400, LatencyP99: 500},
	}

	result := detector.DetectLatencyKnee(points, false)

	if !result.Detected {
		t.Fatalf("DetectLatencyKnee should detect knee")
	}
	if result.KneePoint.Significance <= 0 {
		t.Errorf("Significance = %f, want > 0", result.KneePoint.Significance)
	}
}

func TestDetectLatencyKnee_MetricValue(t *testing.T) {
	detector := NewKneeDetector()

	points := []TimeSeriesPoint{
		{LoadLevel: 10, LatencyP95: 100, LatencyP99: 120},
		{LoadLevel: 20, LatencyP95: 105, LatencyP99: 125},
		{LoadLevel: 30, LatencyP95: 110, LatencyP99: 130},
		{LoadLevel: 40, LatencyP95: 250, LatencyP99: 300},
		{LoadLevel: 50, LatencyP95: 400, LatencyP99: 500},
	}

	resultP95 := detector.DetectLatencyKnee(points, false)
	resultP99 := detector.DetectLatencyKnee(points, true)

	if !resultP95.Detected || !resultP99.Detected {
		t.Fatalf("Both should detect knee")
	}

	// MetricValue should match the latency at knee point (verify it's a valid latency value)
	if resultP95.KneePoint.MetricValue <= 0 {
		t.Errorf("P95 MetricValue = %f, want > 0", resultP95.KneePoint.MetricValue)
	}
	if resultP99.KneePoint.MetricValue <= 0 {
		t.Errorf("P99 MetricValue = %f, want > 0", resultP99.KneePoint.MetricValue)
	}
	// P99 values should be higher than P95 values
	if resultP99.KneePoint.MetricValue <= resultP95.KneePoint.MetricValue {
		t.Errorf("P99 MetricValue should be higher than P95")
	}
}
