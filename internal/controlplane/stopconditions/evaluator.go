package stopconditions

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/analysis"
	"github.com/bc-dunia/mcpdrill/internal/events"
	"github.com/bc-dunia/mcpdrill/internal/telemetry"
)

var latencyPool = sync.Pool{
	New: func() any {
		return make([]int, 0, 256)
	},
}

// Condition defines a single runtime stop condition.
type Condition struct {
	ID             string
	Metric         string
	Comparator     string
	Threshold      float64
	WindowMs       int64
	SustainWindows int
	Scope          map[string]string
}

// StreamingConfig holds streaming-specific stop condition thresholds.
type StreamingConfig struct {
	StreamStallSeconds int     // Trigger if no events for X seconds
	MinEventsPerSecond float64 // Trigger if event rate < threshold
}

// StreamingProvider provides access to streaming metrics.
type StreamingProvider interface {
	GetStreamingMetrics(runID string) (*telemetry.StreamingMetrics, error)
}

// StreamingProviderFunc is an adapter to allow ordinary functions to be used as StreamingProvider.
type StreamingProviderFunc func(runID string) (*telemetry.StreamingMetrics, error)

// GetStreamingMetrics returns streaming metrics for a run.
func (f StreamingProviderFunc) GetStreamingMetrics(runID string) (*telemetry.StreamingMetrics, error) {
	return f(runID)
}

// Trigger captures the details of a stop condition breach.
type Trigger struct {
	Condition   Condition
	Observed    float64
	WindowMs    int64
	TotalOps    int
	FailedOps   int
	LatencyP99  int
	TimestampMs int64
}

// TelemetryProvider provides access to operation telemetry.
type TelemetryProvider interface {
	GetOperations(runID string) ([]analysis.OperationResult, error)
}

// TelemetryProviderFunc adapts a function to TelemetryProvider.
type TelemetryProviderFunc func(runID string) ([]analysis.OperationResult, error)

// GetOperations returns telemetry operations for a run.
func (f TelemetryProviderFunc) GetOperations(runID string) ([]analysis.OperationResult, error) {
	return f(runID)
}

// Evaluator polls telemetry data and checks stop conditions.
type Evaluator struct {
	RunID           string
	StageID         string
	Conditions      []Condition
	PollInterval    time.Duration
	Telemetry       TelemetryProvider
	Streaming       StreamingProvider
	StreamingConfig *StreamingConfig
	OnTrigger       func(Trigger)

	lastSeen      int
	buffer        []timedOperation
	sustainCounts map[string]int
	maxWindowMs   int64
}

type timedOperation struct {
	op         analysis.OperationResult
	observedMs int64
}

// NewEvaluator creates a new evaluator instance.
func NewEvaluator(runID string, telemetry TelemetryProvider, conditions []Condition, pollInterval time.Duration) *Evaluator {
	copied := make([]Condition, len(conditions))
	copy(copied, conditions)

	maxWindow := int64(0)
	for _, cond := range copied {
		if cond.WindowMs > maxWindow {
			maxWindow = cond.WindowMs
		}
	}

	return &Evaluator{
		RunID:         runID,
		Conditions:    copied,
		PollInterval:  pollInterval,
		Telemetry:     telemetry,
		sustainCounts: make(map[string]int),
		maxWindowMs:   maxWindow,
	}
}

// Run starts the evaluation loop and exits when context is cancelled or a trigger fires.
func (e *Evaluator) Run(ctxDone <-chan struct{}) {
	interval := e.PollInterval
	if interval <= 0 {
		interval = 5 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctxDone:
			return
		case now := <-ticker.C:
			trigger, err := e.Evaluate(now.UnixMilli())
			if err != nil {
				continue
			}
			if trigger.Condition.Metric != "" {
				if e.OnTrigger != nil {
					e.OnTrigger(trigger)
				}
				return
			}
		}
	}
}

// Evaluate runs a single evaluation pass and returns a trigger if breached.
func (e *Evaluator) Evaluate(nowMs int64) (Trigger, error) {
	if streamTrigger := e.evaluateStreamingConditions(nowMs); streamTrigger != nil {
		return *streamTrigger, nil
	}

	if e.Telemetry == nil {
		return Trigger{}, fmt.Errorf("telemetry provider not configured")
	}

	operations, err := e.Telemetry.GetOperations(e.RunID)
	if err != nil {
		return Trigger{}, err
	}

	if len(operations) < e.lastSeen {
		e.lastSeen = 0
		e.buffer = nil
		e.sustainCounts = make(map[string]int)
	}

	if e.lastSeen < len(operations) {
		for _, op := range operations[e.lastSeen:] {
			e.buffer = append(e.buffer, timedOperation{op: op, observedMs: nowMs})
		}
		e.lastSeen = len(operations)
	}

	if e.maxWindowMs > 0 && len(e.buffer) > 0 {
		cutoff := nowMs - e.maxWindowMs
		idx := 0
		for idx < len(e.buffer) && e.buffer[idx].observedMs < cutoff {
			idx++
		}
		if idx > 0 {
			e.buffer = e.buffer[idx:]
		}
	}

	for i, cond := range e.Conditions {
		if cond.WindowMs <= 0 {
			e.sustainCounts[e.conditionKey(cond, i)] = 0
			continue
		}

		totalOps, failedOps, latencies := e.windowStats(nowMs, cond.WindowMs)
		if totalOps == 0 {
			e.sustainCounts[e.conditionKey(cond, i)] = 0
			if latencies != nil {
				latencyPool.Put(latencies[:0])
			}
			continue
		}

		observed, latencyP99 := evaluateMetric(cond.Metric, totalOps, failedOps, latencies)
		if latencies != nil {
			latencyPool.Put(latencies[:0])
		}
		if !compare(observed, cond.Comparator, cond.Threshold) {
			e.sustainCounts[e.conditionKey(cond, i)] = 0
			continue
		}

		key := e.conditionKey(cond, i)
		sustain := cond.SustainWindows
		if sustain <= 0 {
			sustain = 1
		}
		e.sustainCounts[key]++
		if e.sustainCounts[key] < sustain {
			continue
		}

		trigger := Trigger{
			Condition:   cond,
			Observed:    observed,
			WindowMs:    cond.WindowMs,
			TotalOps:    totalOps,
			FailedOps:   failedOps,
			LatencyP99:  latencyP99,
			TimestampMs: nowMs,
		}

		if l := events.GetGlobalEventLogger(); l != nil {
			l.LogStopCondition(e.StageID, cond.Metric, observed, cond.Threshold, "metric_threshold_exceeded")
		}

		return trigger, nil
	}

	return Trigger{}, nil
}

func (e *Evaluator) conditionKey(cond Condition, index int) string {
	if cond.ID != "" {
		return cond.ID
	}
	return fmt.Sprintf("%s-%d", cond.Metric, index)
}

func (e *Evaluator) windowStats(nowMs int64, windowMs int64) (int, int, []int) {
	if len(e.buffer) == 0 {
		return 0, 0, nil
	}

	cutoff := nowMs - windowMs
	totalOps := 0
	failedOps := 0
	latencies := latencyPool.Get().([]int)
	if cap(latencies) < len(e.buffer) {
		latencies = make([]int, 0, len(e.buffer))
	} else {
		latencies = latencies[:0]
	}
	for _, entry := range e.buffer {
		if entry.observedMs < cutoff {
			continue
		}
		totalOps++
		if !entry.op.OK {
			failedOps++
		}
		latencies = append(latencies, entry.op.LatencyMs)
	}
	return totalOps, failedOps, latencies
}

func evaluateMetric(metric string, totalOps, failedOps int, latencies []int) (float64, int) {
	switch metric {
	case "error_rate":
		if totalOps == 0 {
			return 0, 0
		}
		return float64(failedOps) / float64(totalOps), 0
	case "latency_p50_ms":
		p50 := percentile(latencies, 50)
		return float64(p50), p50
	case "latency_p95_ms":
		p95 := percentile(latencies, 95)
		return float64(p95), p95
	case "latency_p99_ms":
		p99 := percentile(latencies, 99)
		return float64(p99), p99
	default:
		// Unknown metric - return -1 to indicate invalid metric
		// This prevents false triggers from 0 comparison
		return -1, 0
	}
}

func compare(observed float64, comparator string, threshold float64) bool {
	// Invalid metric value - never trigger
	if observed < 0 {
		return false
	}
	switch comparator {
	case ">":
		return observed > threshold
	case ">=":
		return observed >= threshold
	case "<":
		return observed < threshold
	case "<=":
		return observed <= threshold
	default:
		return observed > threshold
	}
}

func percentile(values []int, p float64) int {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]int, len(values))
	copy(sorted, values)
	sort.Ints(sorted)
	rank := (p / 100.0) * float64(len(sorted))
	index := int(rank)
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	if index < 0 {
		index = 0
	}
	return sorted[index]
}

func (e *Evaluator) evaluateStreamingConditions(nowMs int64) *Trigger {
	if e.StreamingConfig == nil || e.Streaming == nil {
		return nil
	}

	metrics, err := e.Streaming.GetStreamingMetrics(e.RunID)
	if err != nil || metrics == nil {
		return nil
	}

	if trigger := e.evaluateStreamStall(nowMs, metrics); trigger != nil {
		return trigger
	}

	if trigger := e.evaluateMinEventsPerSecond(nowMs, metrics); trigger != nil {
		return trigger
	}

	return nil
}

func (e *Evaluator) evaluateStreamStall(nowMs int64, metrics *telemetry.StreamingMetrics) *Trigger {
	if e.StreamingConfig.StreamStallSeconds <= 0 {
		return nil
	}

	if metrics.LastEventTimeMs == 0 {
		return nil
	}

	stallDurationMs := nowMs - metrics.LastEventTimeMs
	thresholdMs := int64(e.StreamingConfig.StreamStallSeconds) * 1000

	if stallDurationMs > thresholdMs {
		stallSeconds := float64(stallDurationMs) / 1000.0
		if l := events.GetGlobalEventLogger(); l != nil {
			l.LogStallTrigger(e.RunID, stallSeconds, float64(e.StreamingConfig.StreamStallSeconds))
		}
		return &Trigger{
			Condition: Condition{
				ID:         "stream_stall",
				Metric:     "stream_stall_seconds",
				Comparator: ">",
				Threshold:  float64(e.StreamingConfig.StreamStallSeconds),
			},
			Observed:    stallSeconds,
			TimestampMs: nowMs,
		}
	}

	return nil
}

func (e *Evaluator) evaluateMinEventsPerSecond(nowMs int64, metrics *telemetry.StreamingMetrics) *Trigger {
	if e.StreamingConfig.MinEventsPerSecond <= 0 {
		return nil
	}

	if metrics.StreamStartTimeMs == 0 || metrics.EventsReceived == 0 {
		return nil
	}

	durationMs := nowMs - metrics.StreamStartTimeMs
	if durationMs <= 0 {
		return nil
	}

	eventsPerSecond := float64(metrics.EventsReceived) / (float64(durationMs) / 1000.0)

	if eventsPerSecond < e.StreamingConfig.MinEventsPerSecond {
		if l := events.GetGlobalEventLogger(); l != nil {
			l.LogStopCondition(e.StageID, "min_events_per_second", eventsPerSecond, e.StreamingConfig.MinEventsPerSecond, "event_rate_below_threshold")
		}
		return &Trigger{
			Condition: Condition{
				ID:         "min_events_per_second",
				Metric:     "min_events_per_second",
				Comparator: "<",
				Threshold:  e.StreamingConfig.MinEventsPerSecond,
			},
			Observed:    eventsPerSecond,
			TimestampMs: nowMs,
		}
	}

	return nil
}
