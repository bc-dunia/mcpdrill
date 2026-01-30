package analysis

import (
	"math"
	"sort"
)

type KneePoint struct {
	LoadLevel    int     `json:"load_level"`
	MetricValue  float64 `json:"metric_value"`
	ChangeRatio  float64 `json:"change_ratio"`
	Significance float64 `json:"significance"`
}

type KneeDetectionResult struct {
	Detected        bool       `json:"detected"`
	KneePoint       *KneePoint `json:"knee_point,omitempty"`
	Metric          string     `json:"metric"`
	Threshold       float64    `json:"threshold"`
	DataPoints      int        `json:"data_points"`
	AnalysisDetails string     `json:"analysis_details,omitempty"`
}

type TimeSeriesPoint struct {
	LoadLevel  int
	Timestamp  int64
	LatencyP95 int
	LatencyP99 int
	ErrorRate  float64
	Throughput float64
}

type KneeDetector struct {
	minDataPoints        int
	changeRatioThreshold float64
	errorRateThreshold   float64
}

func NewKneeDetector() *KneeDetector {
	return &KneeDetector{
		minDataPoints:        5,
		changeRatioThreshold: 1.5,
		errorRateThreshold:   0.05,
	}
}

func (d *KneeDetector) SetMinDataPoints(n int) {
	if n > 0 {
		d.minDataPoints = n
	}
}

func (d *KneeDetector) SetChangeRatioThreshold(ratio float64) {
	if ratio > 1.0 {
		d.changeRatioThreshold = ratio
	}
}

func (d *KneeDetector) SetErrorRateThreshold(threshold float64) {
	if threshold > 0 && threshold < 1.0 {
		d.errorRateThreshold = threshold
	}
}

func (d *KneeDetector) DetectLatencyKnee(points []TimeSeriesPoint, useP99 bool) *KneeDetectionResult {
	result := &KneeDetectionResult{
		Metric:     "latency_p95",
		Threshold:  d.changeRatioThreshold,
		DataPoints: len(points),
	}

	if useP99 {
		result.Metric = "latency_p99"
	}

	if len(points) < d.minDataPoints {
		result.AnalysisDetails = "insufficient data points"
		return result
	}

	sortedPoints := make([]TimeSeriesPoint, len(points))
	copy(sortedPoints, points)
	sort.Slice(sortedPoints, func(i, j int) bool {
		return sortedPoints[i].LoadLevel < sortedPoints[j].LoadLevel
	})

	values := make([]float64, len(sortedPoints))
	for i, p := range sortedPoints {
		if useP99 {
			values[i] = float64(p.LatencyP99)
		} else {
			values[i] = float64(p.LatencyP95)
		}
	}

	kneeIdx := d.findKneeByMaxCurvature(values)
	if kneeIdx < 0 {
		result.AnalysisDetails = "no significant inflection point found"
		return result
	}

	changeRatio := d.calculateChangeRatio(values, kneeIdx)
	if changeRatio < d.changeRatioThreshold {
		result.AnalysisDetails = "change ratio below threshold"
		return result
	}

	significance := d.calculateSignificance(values, kneeIdx)

	result.Detected = true
	result.KneePoint = &KneePoint{
		LoadLevel:    sortedPoints[kneeIdx].LoadLevel,
		MetricValue:  values[kneeIdx],
		ChangeRatio:  changeRatio,
		Significance: significance,
	}
	result.AnalysisDetails = "latency inflection detected"

	return result
}

func (d *KneeDetector) DetectErrorRateKnee(points []TimeSeriesPoint) *KneeDetectionResult {
	result := &KneeDetectionResult{
		Metric:     "error_rate",
		Threshold:  d.errorRateThreshold,
		DataPoints: len(points),
	}

	if len(points) < d.minDataPoints {
		result.AnalysisDetails = "insufficient data points"
		return result
	}

	sortedPoints := make([]TimeSeriesPoint, len(points))
	copy(sortedPoints, points)
	sort.Slice(sortedPoints, func(i, j int) bool {
		return sortedPoints[i].LoadLevel < sortedPoints[j].LoadLevel
	})

	values := make([]float64, len(sortedPoints))
	for i, p := range sortedPoints {
		values[i] = p.ErrorRate
	}

	kneeIdx := -1
	for i, v := range values {
		if v >= d.errorRateThreshold {
			kneeIdx = i
			break
		}
	}

	if kneeIdx < 0 {
		result.AnalysisDetails = "error rate below threshold throughout"
		return result
	}

	changeRatio := 0.0
	if kneeIdx > 0 && values[kneeIdx-1] > 0 {
		changeRatio = values[kneeIdx] / values[kneeIdx-1]
	} else if kneeIdx > 0 {
		changeRatio = math.Inf(1)
	}

	result.Detected = true
	result.KneePoint = &KneePoint{
		LoadLevel:    sortedPoints[kneeIdx].LoadLevel,
		MetricValue:  values[kneeIdx],
		ChangeRatio:  changeRatio,
		Significance: values[kneeIdx] / d.errorRateThreshold,
	}
	result.AnalysisDetails = "error rate threshold exceeded"

	return result
}

func (d *KneeDetector) DetectCombinedKnee(points []TimeSeriesPoint) *KneeDetectionResult {
	latencyKnee := d.DetectLatencyKnee(points, true)
	errorKnee := d.DetectErrorRateKnee(points)

	if errorKnee.Detected {
		return errorKnee
	}

	if latencyKnee.Detected {
		return latencyKnee
	}

	return &KneeDetectionResult{
		Metric:          "combined",
		Threshold:       d.changeRatioThreshold,
		DataPoints:      len(points),
		AnalysisDetails: "no knee point detected",
	}
}

func (d *KneeDetector) findKneeByMaxCurvature(values []float64) int {
	if len(values) < 3 {
		return -1
	}

	minVal := values[0]
	maxVal := values[0]
	for _, v := range values {
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}

	if maxVal-minVal < 1.0 {
		return -1
	}

	normalized := make([]float64, len(values))
	for i, v := range values {
		normalized[i] = (v - minVal) / (maxVal - minVal)
	}

	maxCurvature := 0.0
	kneeIdx := -1

	for i := 1; i < len(normalized)-1; i++ {
		curvature := d.calculateCurvature(
			float64(i-1), normalized[i-1],
			float64(i), normalized[i],
			float64(i+1), normalized[i+1],
		)
		if curvature > maxCurvature {
			maxCurvature = curvature
			kneeIdx = i
		}
	}

	if maxCurvature < 0.1 {
		return -1
	}

	return kneeIdx
}

func (d *KneeDetector) calculateCurvature(x1, y1, x2, y2, x3, y3 float64) float64 {
	dx1 := x2 - x1
	dy1 := y2 - y1
	dx2 := x3 - x2
	dy2 := y3 - y2

	cross := dx1*dy2 - dy1*dx2

	len1 := math.Sqrt(dx1*dx1 + dy1*dy1)
	len2 := math.Sqrt(dx2*dx2 + dy2*dy2)

	if len1 < 1e-10 || len2 < 1e-10 {
		return 0
	}

	return math.Abs(cross) / (len1 * len2)
}

func (d *KneeDetector) calculateChangeRatio(values []float64, kneeIdx int) float64 {
	if kneeIdx <= 0 || kneeIdx >= len(values) {
		return 1.0
	}

	avgBefore := 0.0
	for i := 0; i < kneeIdx; i++ {
		avgBefore += values[i]
	}
	avgBefore /= float64(kneeIdx)

	avgAfter := 0.0
	count := len(values) - kneeIdx
	for i := kneeIdx; i < len(values); i++ {
		avgAfter += values[i]
	}
	avgAfter /= float64(count)

	if avgBefore < 1.0 {
		avgBefore = 1.0
	}

	return avgAfter / avgBefore
}

func (d *KneeDetector) calculateSignificance(values []float64, kneeIdx int) float64 {
	if kneeIdx <= 0 || kneeIdx >= len(values) {
		return 0
	}

	mean := 0.0
	for _, v := range values {
		mean += v
	}
	mean /= float64(len(values))

	variance := 0.0
	for _, v := range values {
		diff := v - mean
		variance += diff * diff
	}
	variance /= float64(len(values))
	stdDev := math.Sqrt(variance)

	if stdDev < 1.0 {
		return 0
	}

	return math.Abs(values[kneeIdx]-mean) / stdDev
}
