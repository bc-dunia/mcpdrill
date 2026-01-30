package e2e

import (
	"testing"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/analysis"
	"github.com/bc-dunia/mcpdrill/internal/controlplane/stopconditions"
	"github.com/bc-dunia/mcpdrill/internal/telemetry"
)

type fakeStreamingProvider struct {
	metrics *telemetry.StreamingMetrics
}

func (f *fakeStreamingProvider) GetStreamingMetrics(runID string) (*telemetry.StreamingMetrics, error) {
	return f.metrics, nil
}

type fakeTelemetryProvider struct {
	ops []analysis.OperationResult
}

func (f *fakeTelemetryProvider) GetOperations(runID string) ([]analysis.OperationResult, error) {
	return f.ops, nil
}

func TestStreamStallDetection(t *testing.T) {
	nowMs := time.Now().UnixMilli()

	provider := &fakeStreamingProvider{
		metrics: &telemetry.StreamingMetrics{
			EventsReceived:  100,
			LastEventTimeMs: nowMs - 15000, // 15 seconds ago (stall)
		},
	}

	evaluator := &stopconditions.Evaluator{
		RunID: "run_00000000000000a1",
		StreamingConfig: &stopconditions.StreamingConfig{
			StreamStallSeconds: 10, // Trigger if no events for 10 seconds
		},
		Streaming: provider,
	}

	trigger, err := evaluator.Evaluate(nowMs)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	if trigger.Condition.Metric != "stream_stall_seconds" {
		t.Fatalf("Expected stream_stall_seconds trigger, got %s", trigger.Condition.Metric)
	}

	if trigger.Observed < 15.0 {
		t.Errorf("Expected observed >= 15 seconds, got %f", trigger.Observed)
	}

	if trigger.Condition.Threshold != 10.0 {
		t.Errorf("Expected threshold 10, got %f", trigger.Condition.Threshold)
	}

	t.Logf("Stream stall detected: observed=%.2f seconds, threshold=%.2f seconds",
		trigger.Observed, trigger.Condition.Threshold)
}

func TestStreamStallNoTriggerWhenRecent(t *testing.T) {
	nowMs := time.Now().UnixMilli()

	provider := &fakeStreamingProvider{
		metrics: &telemetry.StreamingMetrics{
			EventsReceived:  100,
			LastEventTimeMs: nowMs - 5000, // 5 seconds ago (no stall)
		},
	}

	evaluator := &stopconditions.Evaluator{
		RunID: "run_00000000000000b1",
		StreamingConfig: &stopconditions.StreamingConfig{
			StreamStallSeconds: 10,
		},
		Streaming: provider,
		Telemetry: &fakeTelemetryProvider{},
	}

	trigger, err := evaluator.Evaluate(nowMs)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	if trigger.Condition.Metric != "" {
		t.Errorf("Expected no trigger when events are recent, got %s", trigger.Condition.Metric)
	}

	t.Logf("No stall detected (events received 5 seconds ago, threshold 10 seconds)")
}

func TestStreamStallNoEventsYet(t *testing.T) {
	nowMs := time.Now().UnixMilli()

	provider := &fakeStreamingProvider{
		metrics: &telemetry.StreamingMetrics{
			EventsReceived:  0,
			LastEventTimeMs: 0, // No events yet
		},
	}

	evaluator := &stopconditions.Evaluator{
		RunID: "run_00000000000000b2",
		StreamingConfig: &stopconditions.StreamingConfig{
			StreamStallSeconds: 10,
		},
		Streaming: provider,
		Telemetry: &fakeTelemetryProvider{},
	}

	trigger, err := evaluator.Evaluate(nowMs)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	if trigger.Condition.Metric != "" {
		t.Errorf("Expected no trigger when no events yet, got %s", trigger.Condition.Metric)
	}

	t.Logf("No stall detected (no events received yet)")
}

func TestMinEventsPerSecondBelowThreshold(t *testing.T) {
	nowMs := time.Now().UnixMilli()

	provider := &fakeStreamingProvider{
		metrics: &telemetry.StreamingMetrics{
			EventsReceived:    10,
			StreamStartTimeMs: nowMs - 10000, // 10 seconds = 1 event/sec
		},
	}

	evaluator := &stopconditions.Evaluator{
		RunID: "run_00000000000000c1",
		StreamingConfig: &stopconditions.StreamingConfig{
			MinEventsPerSecond: 5, // Require at least 5 events/sec
		},
		Streaming: provider,
	}

	trigger, err := evaluator.Evaluate(nowMs)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	if trigger.Condition.Metric != "min_events_per_second" {
		t.Fatalf("Expected min_events_per_second trigger, got %s", trigger.Condition.Metric)
	}

	if trigger.Observed >= 5.0 {
		t.Errorf("Expected observed < 5 events/sec, got %f", trigger.Observed)
	}

	if trigger.Condition.Threshold != 5.0 {
		t.Errorf("Expected threshold 5, got %f", trigger.Condition.Threshold)
	}

	t.Logf("Min events/sec trigger: observed=%.2f events/sec, threshold=%.2f events/sec",
		trigger.Observed, trigger.Condition.Threshold)
}

func TestMinEventsPerSecondAboveThreshold(t *testing.T) {
	nowMs := time.Now().UnixMilli()

	provider := &fakeStreamingProvider{
		metrics: &telemetry.StreamingMetrics{
			EventsReceived:    100,
			StreamStartTimeMs: nowMs - 10000, // 10 seconds = 10 events/sec
		},
	}

	evaluator := &stopconditions.Evaluator{
		RunID: "run_00000000000000a2",
		StreamingConfig: &stopconditions.StreamingConfig{
			MinEventsPerSecond: 5,
		},
		Streaming: provider,
		Telemetry: &fakeTelemetryProvider{},
	}

	trigger, err := evaluator.Evaluate(nowMs)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	if trigger.Condition.Metric != "" {
		t.Errorf("Expected no trigger when above threshold, got %s", trigger.Condition.Metric)
	}

	t.Logf("No trigger (10 events/sec >= 5 events/sec threshold)")
}

func TestMinEventsPerSecondNoStartTime(t *testing.T) {
	nowMs := time.Now().UnixMilli()

	provider := &fakeStreamingProvider{
		metrics: &telemetry.StreamingMetrics{
			EventsReceived:    10,
			StreamStartTimeMs: 0, // No start time
		},
	}

	evaluator := &stopconditions.Evaluator{
		RunID: "run_00000000000000c2",
		StreamingConfig: &stopconditions.StreamingConfig{
			MinEventsPerSecond: 5,
		},
		Streaming: provider,
		Telemetry: &fakeTelemetryProvider{},
	}

	trigger, err := evaluator.Evaluate(nowMs)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	if trigger.Condition.Metric != "" {
		t.Errorf("Expected no trigger when no start time, got %s", trigger.Condition.Metric)
	}

	t.Logf("No trigger (no stream start time recorded)")
}

func TestStreamStallTakesPrecedenceOverMinEvents(t *testing.T) {
	nowMs := time.Now().UnixMilli()

	provider := &fakeStreamingProvider{
		metrics: &telemetry.StreamingMetrics{
			EventsReceived:    10,
			LastEventTimeMs:   nowMs - 60000, // 60 seconds stall
			StreamStartTimeMs: nowMs - 100000,
		},
	}

	evaluator := &stopconditions.Evaluator{
		RunID: "run_00000000000000c3",
		StreamingConfig: &stopconditions.StreamingConfig{
			StreamStallSeconds: 30,
			MinEventsPerSecond: 5,
		},
		Streaming: provider,
	}

	trigger, err := evaluator.Evaluate(nowMs)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	if trigger.Condition.Metric != "stream_stall_seconds" {
		t.Fatalf("Expected stream_stall_seconds to take precedence, got %s", trigger.Condition.Metric)
	}

	t.Logf("Stream stall takes precedence over min events/sec")
}

func TestStreamingConditionsDisabled(t *testing.T) {
	nowMs := time.Now().UnixMilli()

	provider := &fakeStreamingProvider{
		metrics: &telemetry.StreamingMetrics{
			EventsReceived:    10,
			LastEventTimeMs:   nowMs - 60000,
			StreamStartTimeMs: nowMs - 100000,
		},
	}

	evaluator := &stopconditions.Evaluator{
		RunID: "run_00000000000000c4",
		StreamingConfig: &stopconditions.StreamingConfig{
			StreamStallSeconds: 0, // Disabled
			MinEventsPerSecond: 0, // Disabled
		},
		Streaming: provider,
		Telemetry: &fakeTelemetryProvider{},
	}

	trigger, err := evaluator.Evaluate(nowMs)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	if trigger.Condition.Metric != "" {
		t.Errorf("Expected no trigger when conditions disabled, got %s", trigger.Condition.Metric)
	}

	t.Logf("No trigger when streaming conditions are disabled")
}

func TestStreamingNoProvider(t *testing.T) {
	nowMs := time.Now().UnixMilli()

	evaluator := &stopconditions.Evaluator{
		RunID: "run_00000000000000c5",
		StreamingConfig: &stopconditions.StreamingConfig{
			StreamStallSeconds: 30,
		},
		Streaming: nil, // No provider
		Telemetry: &fakeTelemetryProvider{},
	}

	trigger, err := evaluator.Evaluate(nowMs)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	if trigger.Condition.Metric != "" {
		t.Errorf("Expected no trigger when no provider, got %s", trigger.Condition.Metric)
	}

	t.Logf("No trigger when streaming provider is nil")
}

func TestStreamingNoConfig(t *testing.T) {
	nowMs := time.Now().UnixMilli()

	provider := &fakeStreamingProvider{
		metrics: &telemetry.StreamingMetrics{
			EventsReceived:  100,
			LastEventTimeMs: nowMs - 60000,
		},
	}

	evaluator := &stopconditions.Evaluator{
		RunID:           "run_00000000000000c6",
		StreamingConfig: nil, // No config
		Streaming:       provider,
		Telemetry:       &fakeTelemetryProvider{},
	}

	trigger, err := evaluator.Evaluate(nowMs)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	if trigger.Condition.Metric != "" {
		t.Errorf("Expected no trigger when no config, got %s", trigger.Condition.Metric)
	}

	t.Logf("No trigger when streaming config is nil")
}

func TestStreamStallEdgeCase(t *testing.T) {
	nowMs := time.Now().UnixMilli()

	provider := &fakeStreamingProvider{
		metrics: &telemetry.StreamingMetrics{
			EventsReceived:  100,
			LastEventTimeMs: nowMs - 10000, // Exactly at threshold
		},
	}

	evaluator := &stopconditions.Evaluator{
		RunID: "run_00000000000000c7",
		StreamingConfig: &stopconditions.StreamingConfig{
			StreamStallSeconds: 10,
		},
		Streaming: provider,
		Telemetry: &fakeTelemetryProvider{},
	}

	trigger, err := evaluator.Evaluate(nowMs)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	// At exactly threshold, should NOT trigger (need to exceed)
	if trigger.Condition.Metric != "" {
		t.Errorf("Expected no trigger at exact threshold, got %s", trigger.Condition.Metric)
	}

	t.Logf("No trigger at exact threshold (10 seconds = 10 seconds)")
}

func TestStreamStallJustOverThreshold(t *testing.T) {
	nowMs := time.Now().UnixMilli()

	provider := &fakeStreamingProvider{
		metrics: &telemetry.StreamingMetrics{
			EventsReceived:  100,
			LastEventTimeMs: nowMs - 10001, // Just over threshold
		},
	}

	evaluator := &stopconditions.Evaluator{
		RunID: "run_00000000000000c8",
		StreamingConfig: &stopconditions.StreamingConfig{
			StreamStallSeconds: 10,
		},
		Streaming: provider,
	}

	trigger, err := evaluator.Evaluate(nowMs)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	if trigger.Condition.Metric != "stream_stall_seconds" {
		t.Fatalf("Expected trigger just over threshold, got %s", trigger.Condition.Metric)
	}

	t.Logf("Trigger just over threshold (10.001 seconds > 10 seconds)")
}

func TestMinEventsPerSecondEdgeCase(t *testing.T) {
	nowMs := time.Now().UnixMilli()

	provider := &fakeStreamingProvider{
		metrics: &telemetry.StreamingMetrics{
			EventsReceived:    50,
			StreamStartTimeMs: nowMs - 10000, // 10 seconds = exactly 5 events/sec
		},
	}

	evaluator := &stopconditions.Evaluator{
		RunID: "run_00000000000000c9",
		StreamingConfig: &stopconditions.StreamingConfig{
			MinEventsPerSecond: 5,
		},
		Streaming: provider,
		Telemetry: &fakeTelemetryProvider{},
	}

	trigger, err := evaluator.Evaluate(nowMs)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	// At exactly threshold, should NOT trigger (need to be below)
	if trigger.Condition.Metric != "" {
		t.Errorf("Expected no trigger at exact threshold, got %s", trigger.Condition.Metric)
	}

	t.Logf("No trigger at exact threshold (5 events/sec = 5 events/sec)")
}

func TestMinEventsPerSecondJustBelowThreshold(t *testing.T) {
	nowMs := time.Now().UnixMilli()

	provider := &fakeStreamingProvider{
		metrics: &telemetry.StreamingMetrics{
			EventsReceived:    49,
			StreamStartTimeMs: nowMs - 10000, // 10 seconds = 4.9 events/sec
		},
	}

	evaluator := &stopconditions.Evaluator{
		RunID: "run_00000000000000ca",
		StreamingConfig: &stopconditions.StreamingConfig{
			MinEventsPerSecond: 5,
		},
		Streaming: provider,
	}

	trigger, err := evaluator.Evaluate(nowMs)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	if trigger.Condition.Metric != "min_events_per_second" {
		t.Fatalf("Expected trigger just below threshold, got %s", trigger.Condition.Metric)
	}

	t.Logf("Trigger just below threshold (4.9 events/sec < 5 events/sec)")
}

func TestStreamingMetricsTracking(t *testing.T) {
	metrics := &telemetry.StreamingMetrics{
		EventsReceived:    1000,
		LastEventTimeMs:   time.Now().UnixMilli(),
		StreamStallCount:  2,
		StreamStartTimeMs: time.Now().UnixMilli() - 60000,
	}

	if metrics.EventsReceived != 1000 {
		t.Errorf("Expected 1000 events, got %d", metrics.EventsReceived)
	}

	if metrics.StreamStallCount != 2 {
		t.Errorf("Expected 2 stall count, got %d", metrics.StreamStallCount)
	}

	durationMs := metrics.LastEventTimeMs - metrics.StreamStartTimeMs
	if durationMs < 60000 {
		t.Errorf("Expected duration >= 60000ms, got %d", durationMs)
	}

	t.Logf("Streaming metrics tracking verified: events=%d, stalls=%d",
		metrics.EventsReceived, metrics.StreamStallCount)
}
