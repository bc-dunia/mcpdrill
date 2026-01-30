package stopconditions

import (
	"testing"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/analysis"
)

type fakeTelemetry struct {
	ops []analysis.OperationResult
}

func (f *fakeTelemetry) GetOperations(runID string) ([]analysis.OperationResult, error) {
	copied := make([]analysis.OperationResult, len(f.ops))
	copy(copied, f.ops)
	return copied, nil
}

func TestEvaluatorErrorRateTrigger(t *testing.T) {
	telemetry := &fakeTelemetry{}
	cond := Condition{
		ID:             "err_rate",
		Metric:         "error_rate",
		Comparator:     ">",
		Threshold:      0.5,
		WindowMs:       1000,
		SustainWindows: 1,
	}

	evaluator := NewEvaluator("run_0000000000000001", telemetry, []Condition{cond}, time.Second)

	telemetry.ops = []analysis.OperationResult{
		{Operation: "ping", OK: true, LatencyMs: 10},
		{Operation: "ping", OK: false, LatencyMs: 12},
		{Operation: "ping", OK: true, LatencyMs: 9},
		{Operation: "ping", OK: false, LatencyMs: 11},
	}

	trigger, err := evaluator.Evaluate(1000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if trigger.Condition.Metric != "" {
		t.Fatalf("expected no trigger, got %+v", trigger)
	}

	telemetry.ops = append(telemetry.ops, analysis.OperationResult{Operation: "ping", OK: false, LatencyMs: 15})
	trigger, err = evaluator.Evaluate(1100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if trigger.Condition.Metric != "error_rate" {
		t.Fatalf("expected error_rate trigger, got %+v", trigger)
	}
	if trigger.Observed <= cond.Threshold {
		t.Fatalf("expected observed error rate above threshold, got %f", trigger.Observed)
	}
}

func TestEvaluatorLatencyP99Trigger(t *testing.T) {
	telemetry := &fakeTelemetry{}
	cond := Condition{
		ID:             "lat_p99",
		Metric:         "latency_p99_ms",
		Comparator:     ">",
		Threshold:      250,
		WindowMs:       1000,
		SustainWindows: 1,
	}

	evaluator := NewEvaluator("run_0000000000000002", telemetry, []Condition{cond}, time.Second)

	telemetry.ops = []analysis.OperationResult{
		{Operation: "tools_call", OK: true, LatencyMs: 100},
		{Operation: "tools_call", OK: true, LatencyMs: 200},
		{Operation: "tools_call", OK: true, LatencyMs: 300},
	}

	trigger, err := evaluator.Evaluate(2000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if trigger.Condition.Metric != "latency_p99_ms" {
		t.Fatalf("expected latency_p99_ms trigger, got %+v", trigger)
	}
	if trigger.LatencyP99 != 300 {
		t.Fatalf("expected latency p99 300, got %d", trigger.LatencyP99)
	}
}

func TestEvaluatorSustainWindows(t *testing.T) {
	telemetry := &fakeTelemetry{}
	cond := Condition{
		ID:             "err_rate",
		Metric:         "error_rate",
		Comparator:     ">=",
		Threshold:      0.5,
		WindowMs:       1000,
		SustainWindows: 2,
	}

	evaluator := NewEvaluator("run_0000000000000003", telemetry, []Condition{cond}, time.Second)

	telemetry.ops = []analysis.OperationResult{
		{Operation: "ping", OK: true, LatencyMs: 10},
		{Operation: "ping", OK: false, LatencyMs: 12},
	}

	trigger, err := evaluator.Evaluate(3000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if trigger.Condition.Metric != "" {
		t.Fatalf("expected no trigger on first breach, got %+v", trigger)
	}

	trigger, err = evaluator.Evaluate(3100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if trigger.Condition.Metric != "error_rate" {
		t.Fatalf("expected trigger after sustain windows, got %+v", trigger)
	}
}
