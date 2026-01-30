package stopconditions

import (
	"testing"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/telemetry"
)

type fakeStreamingProvider struct {
	metrics *telemetry.StreamingMetrics
}

func (f *fakeStreamingProvider) GetStreamingMetrics(runID string) (*telemetry.StreamingMetrics, error) {
	return f.metrics, nil
}

func TestEvaluateStreamStall_NoConfig(t *testing.T) {
	evaluator := &Evaluator{
		RunID:           "run_0000000000000001",
		StreamingConfig: nil,
	}

	trigger := evaluator.evaluateStreamingConditions(time.Now().UnixMilli())
	if trigger != nil {
		t.Fatalf("expected nil trigger when no config, got %+v", trigger)
	}
}

func TestEvaluateStreamStall_NoProvider(t *testing.T) {
	evaluator := &Evaluator{
		RunID:           "run_0000000000000001",
		StreamingConfig: &StreamingConfig{StreamStallSeconds: 30},
		Streaming:       nil,
	}

	trigger := evaluator.evaluateStreamingConditions(time.Now().UnixMilli())
	if trigger != nil {
		t.Fatalf("expected nil trigger when no provider, got %+v", trigger)
	}
}

func TestEvaluateStreamStall_NoEventsYet(t *testing.T) {
	provider := &fakeStreamingProvider{
		metrics: &telemetry.StreamingMetrics{
			EventsReceived:  0,
			LastEventTimeMs: 0,
		},
	}

	evaluator := &Evaluator{
		RunID:           "run_0000000000000001",
		StreamingConfig: &StreamingConfig{StreamStallSeconds: 30},
		Streaming:       provider,
	}

	trigger := evaluator.evaluateStreamingConditions(time.Now().UnixMilli())
	if trigger != nil {
		t.Fatalf("expected nil trigger when no events yet, got %+v", trigger)
	}
}

func TestEvaluateStreamStall_NoStall(t *testing.T) {
	nowMs := time.Now().UnixMilli()
	provider := &fakeStreamingProvider{
		metrics: &telemetry.StreamingMetrics{
			EventsReceived:  100,
			LastEventTimeMs: nowMs - 5000, // 5 seconds ago
		},
	}

	evaluator := &Evaluator{
		RunID:           "run_0000000000000001",
		StreamingConfig: &StreamingConfig{StreamStallSeconds: 30},
		Streaming:       provider,
	}

	trigger := evaluator.evaluateStreamingConditions(nowMs)
	if trigger != nil {
		t.Fatalf("expected nil trigger when no stall, got %+v", trigger)
	}
}

func TestEvaluateStreamStall_StallDetected(t *testing.T) {
	nowMs := time.Now().UnixMilli()
	provider := &fakeStreamingProvider{
		metrics: &telemetry.StreamingMetrics{
			EventsReceived:  100,
			LastEventTimeMs: nowMs - 35000, // 35 seconds ago
		},
	}

	evaluator := &Evaluator{
		RunID:           "run_0000000000000001",
		StreamingConfig: &StreamingConfig{StreamStallSeconds: 30},
		Streaming:       provider,
	}

	trigger := evaluator.evaluateStreamingConditions(nowMs)
	if trigger == nil {
		t.Fatal("expected trigger when stall detected")
	}
	if trigger.Condition.Metric != "stream_stall_seconds" {
		t.Fatalf("expected stream_stall_seconds metric, got %s", trigger.Condition.Metric)
	}
	if trigger.Observed < 35.0 {
		t.Fatalf("expected observed >= 35 seconds, got %f", trigger.Observed)
	}
}

func TestEvaluateMinEventsPerSecond_Disabled(t *testing.T) {
	nowMs := time.Now().UnixMilli()
	provider := &fakeStreamingProvider{
		metrics: &telemetry.StreamingMetrics{
			EventsReceived:    10,
			StreamStartTimeMs: nowMs - 10000,
		},
	}

	evaluator := &Evaluator{
		RunID:           "run_0000000000000001",
		StreamingConfig: &StreamingConfig{MinEventsPerSecond: 0},
		Streaming:       provider,
	}

	trigger := evaluator.evaluateStreamingConditions(nowMs)
	if trigger != nil {
		t.Fatalf("expected nil trigger when min_events_per_second disabled, got %+v", trigger)
	}
}

func TestEvaluateMinEventsPerSecond_AboveThreshold(t *testing.T) {
	nowMs := time.Now().UnixMilli()
	provider := &fakeStreamingProvider{
		metrics: &telemetry.StreamingMetrics{
			EventsReceived:    100,
			StreamStartTimeMs: nowMs - 10000, // 10 seconds = 10 events/sec
		},
	}

	evaluator := &Evaluator{
		RunID:           "run_0000000000000001",
		StreamingConfig: &StreamingConfig{MinEventsPerSecond: 5},
		Streaming:       provider,
	}

	trigger := evaluator.evaluateStreamingConditions(nowMs)
	if trigger != nil {
		t.Fatalf("expected nil trigger when above threshold, got %+v", trigger)
	}
}

func TestEvaluateMinEventsPerSecond_BelowThreshold(t *testing.T) {
	nowMs := time.Now().UnixMilli()
	provider := &fakeStreamingProvider{
		metrics: &telemetry.StreamingMetrics{
			EventsReceived:    10,
			StreamStartTimeMs: nowMs - 10000, // 10 seconds = 1 event/sec
		},
	}

	evaluator := &Evaluator{
		RunID:           "run_0000000000000001",
		StreamingConfig: &StreamingConfig{MinEventsPerSecond: 5},
		Streaming:       provider,
	}

	trigger := evaluator.evaluateStreamingConditions(nowMs)
	if trigger == nil {
		t.Fatal("expected trigger when below threshold")
	}
	if trigger.Condition.Metric != "min_events_per_second" {
		t.Fatalf("expected min_events_per_second metric, got %s", trigger.Condition.Metric)
	}
	if trigger.Observed >= 5.0 {
		t.Fatalf("expected observed < 5, got %f", trigger.Observed)
	}
}

func TestEvaluateMinEventsPerSecond_NoStartTime(t *testing.T) {
	nowMs := time.Now().UnixMilli()
	provider := &fakeStreamingProvider{
		metrics: &telemetry.StreamingMetrics{
			EventsReceived:    10,
			StreamStartTimeMs: 0,
		},
	}

	evaluator := &Evaluator{
		RunID:           "run_0000000000000001",
		StreamingConfig: &StreamingConfig{MinEventsPerSecond: 5},
		Streaming:       provider,
	}

	trigger := evaluator.evaluateStreamingConditions(nowMs)
	if trigger != nil {
		t.Fatalf("expected nil trigger when no start time, got %+v", trigger)
	}
}

func TestEvaluateStreamingConditions_StallTakesPrecedence(t *testing.T) {
	nowMs := time.Now().UnixMilli()
	provider := &fakeStreamingProvider{
		metrics: &telemetry.StreamingMetrics{
			EventsReceived:    10,
			LastEventTimeMs:   nowMs - 60000, // 60 seconds stall
			StreamStartTimeMs: nowMs - 100000,
		},
	}

	evaluator := &Evaluator{
		RunID: "run_0000000000000001",
		StreamingConfig: &StreamingConfig{
			StreamStallSeconds:   30,
			MinEventsPerSecond:   5,
		},
		Streaming: provider,
	}

	trigger := evaluator.evaluateStreamingConditions(nowMs)
	if trigger == nil {
		t.Fatal("expected trigger")
	}
	if trigger.Condition.Metric != "stream_stall_seconds" {
		t.Fatalf("expected stream_stall_seconds to take precedence, got %s", trigger.Condition.Metric)
	}
}

func TestEvaluate_IntegrationWithStreaming(t *testing.T) {
	nowMs := time.Now().UnixMilli()
	
	streamProvider := &fakeStreamingProvider{
		metrics: &telemetry.StreamingMetrics{
			EventsReceived:  100,
			LastEventTimeMs: nowMs - 45000, // 45 seconds stall
		},
	}

	evaluator := &Evaluator{
		RunID:           "run_0000000000000001",
		StreamingConfig: &StreamingConfig{StreamStallSeconds: 30},
		Streaming:       streamProvider,
		Telemetry:       &fakeTelemetry{},
		Conditions:      []Condition{},
		PollInterval:    time.Second,
		sustainCounts:   make(map[string]int),
	}

	trigger, err := evaluator.Evaluate(nowMs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if trigger.Condition.Metric != "stream_stall_seconds" {
		t.Fatalf("expected stream_stall_seconds trigger, got %+v", trigger)
	}
}
