package api

import (
	"testing"

	"github.com/bc-dunia/mcpdrill/internal/types"
)

func TestTelemetryStore_GetStreamingMetrics(t *testing.T) {
	ts := NewTelemetryStore()
	runID := "run_00000000000000d1"

	batch := TelemetryBatchRequest{
		Operations: []types.OperationOutcome{
			{
				OpID:        "op1",
				Operation:   "tools_list",
				TimestampMs: 1000,
				LatencyMs:   50,
				OK:          true,
				Stream: &types.StreamInfo{
					IsStreaming:   true,
					EventsCount:   5,
					EndedNormally: true,
					Stalled:       false,
				},
			},
			{
				OpID:        "op2",
				Operation:   "tools_call",
				ToolName:    "echo",
				TimestampMs: 2000,
				LatencyMs:   100,
				OK:          true,
				Stream: &types.StreamInfo{
					IsStreaming:     true,
					EventsCount:     10,
					EndedNormally:   false,
					Stalled:         true,
					StallDurationMs: 3000,
				},
			},
			{
				OpID:        "op3",
				Operation:   "ping",
				TimestampMs: 3000,
				LatencyMs:   10,
				OK:          true,
				Stream:      nil,
			},
		},
	}

	ts.AddTelemetryBatch(runID, batch)

	metrics, err := ts.GetStreamingMetrics(runID)
	if err != nil {
		t.Fatalf("Failed to get streaming metrics: %v", err)
	}

	if metrics.EventsReceived != 15 {
		t.Errorf("Expected EventsReceived=15, got %d", metrics.EventsReceived)
	}

	if metrics.StreamStartTimeMs != 1000 {
		t.Errorf("Expected StreamStartTimeMs=1000, got %d", metrics.StreamStartTimeMs)
	}

	if metrics.LastEventTimeMs != 2000 {
		t.Errorf("Expected LastEventTimeMs=2000, got %d", metrics.LastEventTimeMs)
	}

	if metrics.StreamStallCount != 1 {
		t.Errorf("Expected StreamStallCount=1, got %d", metrics.StreamStallCount)
	}
}

func TestTelemetryStore_GetStreamingMetrics_NoRun(t *testing.T) {
	ts := NewTelemetryStore()

	_, err := ts.GetStreamingMetrics("nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent run")
	}
}

func TestTelemetryStore_GetStreamingMetrics_NoStreamingOps(t *testing.T) {
	ts := NewTelemetryStore()
	runID := "run_00000000000000d2"

	batch := TelemetryBatchRequest{
		Operations: []types.OperationOutcome{
			{
				OpID:        "op1",
				Operation:   "tools_list",
				TimestampMs: 1000,
				LatencyMs:   50,
				OK:          true,
				Stream:      nil,
			},
			{
				OpID:        "op2",
				Operation:   "ping",
				TimestampMs: 2000,
				LatencyMs:   10,
				OK:          true,
				Stream:      nil,
			},
		},
	}

	ts.AddTelemetryBatch(runID, batch)

	metrics, err := ts.GetStreamingMetrics(runID)
	if err != nil {
		t.Fatalf("Failed to get streaming metrics: %v", err)
	}

	if metrics.EventsReceived != 0 {
		t.Errorf("Expected EventsReceived=0, got %d", metrics.EventsReceived)
	}

	if metrics.StreamStartTimeMs != 0 {
		t.Errorf("Expected StreamStartTimeMs=0, got %d", metrics.StreamStartTimeMs)
	}

	if metrics.LastEventTimeMs != 0 {
		t.Errorf("Expected LastEventTimeMs=0, got %d", metrics.LastEventTimeMs)
	}

	if metrics.StreamStallCount != 0 {
		t.Errorf("Expected StreamStallCount=0, got %d", metrics.StreamStallCount)
	}
}

func TestTelemetryStore_GetStreamingMetrics_MultipleStalls(t *testing.T) {
	ts := NewTelemetryStore()
	runID := "run_00000000000000d3"

	batch := TelemetryBatchRequest{
		Operations: []types.OperationOutcome{
			{
				OpID:        "op1",
				Operation:   "tools_call",
				TimestampMs: 1000,
				LatencyMs:   100,
				OK:          true,
				Stream: &types.StreamInfo{
					IsStreaming:     true,
					EventsCount:     5,
					EndedNormally:   false,
					Stalled:         true,
					StallDurationMs: 5000,
				},
			},
			{
				OpID:        "op2",
				Operation:   "tools_call",
				TimestampMs: 2000,
				LatencyMs:   100,
				OK:          true,
				Stream: &types.StreamInfo{
					IsStreaming:     true,
					EventsCount:     3,
					EndedNormally:   false,
					Stalled:         true,
					StallDurationMs: 3000,
				},
			},
			{
				OpID:        "op3",
				Operation:   "tools_call",
				TimestampMs: 3000,
				LatencyMs:   100,
				OK:          true,
				Stream: &types.StreamInfo{
					IsStreaming:     true,
					EventsCount:     7,
					EndedNormally:   false,
					Stalled:         true,
					StallDurationMs: 2000,
				},
			},
		},
	}

	ts.AddTelemetryBatch(runID, batch)

	metrics, err := ts.GetStreamingMetrics(runID)
	if err != nil {
		t.Fatalf("Failed to get streaming metrics: %v", err)
	}

	if metrics.StreamStallCount != 3 {
		t.Errorf("Expected StreamStallCount=3, got %d", metrics.StreamStallCount)
	}

	if metrics.EventsReceived != 15 {
		t.Errorf("Expected EventsReceived=15, got %d", metrics.EventsReceived)
	}
}

func TestTelemetryStore_GetStreamingMetrics_MultipleBatches(t *testing.T) {
	ts := NewTelemetryStore()
	runID := "run_00000000000000d4"

	batch1 := TelemetryBatchRequest{
		Operations: []types.OperationOutcome{
			{
				OpID:        "op1",
				Operation:   "tools_call",
				TimestampMs: 1000,
				LatencyMs:   100,
				OK:          true,
				Stream: &types.StreamInfo{
					IsStreaming:   true,
					EventsCount:   10,
					EndedNormally: true,
					Stalled:       false,
				},
			},
		},
	}

	batch2 := TelemetryBatchRequest{
		Operations: []types.OperationOutcome{
			{
				OpID:        "op2",
				Operation:   "tools_call",
				TimestampMs: 5000,
				LatencyMs:   100,
				OK:          true,
				Stream: &types.StreamInfo{
					IsStreaming:   true,
					EventsCount:   20,
					EndedNormally: true,
					Stalled:       false,
				},
			},
		},
	}

	ts.AddTelemetryBatch(runID, batch1)
	ts.AddTelemetryBatch(runID, batch2)

	metrics, err := ts.GetStreamingMetrics(runID)
	if err != nil {
		t.Fatalf("Failed to get streaming metrics: %v", err)
	}

	if metrics.EventsReceived != 30 {
		t.Errorf("Expected EventsReceived=30, got %d", metrics.EventsReceived)
	}

	if metrics.StreamStartTimeMs != 1000 {
		t.Errorf("Expected StreamStartTimeMs=1000, got %d", metrics.StreamStartTimeMs)
	}

	if metrics.LastEventTimeMs != 5000 {
		t.Errorf("Expected LastEventTimeMs=5000, got %d", metrics.LastEventTimeMs)
	}
}

func TestTelemetryStore_GetStreamingMetrics_OutOfOrderTimestamps(t *testing.T) {
	ts := NewTelemetryStore()
	runID := "run_00000000000000d5"

	batch := TelemetryBatchRequest{
		Operations: []types.OperationOutcome{
			{
				OpID:        "op1",
				Operation:   "tools_call",
				TimestampMs: 5000,
				LatencyMs:   100,
				OK:          true,
				Stream: &types.StreamInfo{
					IsStreaming:   true,
					EventsCount:   5,
					EndedNormally: true,
					Stalled:       false,
				},
			},
			{
				OpID:        "op2",
				Operation:   "tools_call",
				TimestampMs: 1000,
				LatencyMs:   100,
				OK:          true,
				Stream: &types.StreamInfo{
					IsStreaming:   true,
					EventsCount:   3,
					EndedNormally: true,
					Stalled:       false,
				},
			},
			{
				OpID:        "op3",
				Operation:   "tools_call",
				TimestampMs: 3000,
				LatencyMs:   100,
				OK:          true,
				Stream: &types.StreamInfo{
					IsStreaming:   true,
					EventsCount:   7,
					EndedNormally: true,
					Stalled:       false,
				},
			},
		},
	}

	ts.AddTelemetryBatch(runID, batch)

	metrics, err := ts.GetStreamingMetrics(runID)
	if err != nil {
		t.Fatalf("Failed to get streaming metrics: %v", err)
	}

	if metrics.StreamStartTimeMs != 1000 {
		t.Errorf("Expected StreamStartTimeMs=1000 (earliest), got %d", metrics.StreamStartTimeMs)
	}

	if metrics.LastEventTimeMs != 5000 {
		t.Errorf("Expected LastEventTimeMs=5000 (latest), got %d", metrics.LastEventTimeMs)
	}
}

func TestTelemetryStore_GetStreamingMetrics_MixedStreamingAndNonStreaming(t *testing.T) {
	ts := NewTelemetryStore()
	runID := "run_00000000000000d6"

	batch := TelemetryBatchRequest{
		Operations: []types.OperationOutcome{
			{
				OpID:        "op1",
				Operation:   "ping",
				TimestampMs: 500,
				LatencyMs:   10,
				OK:          true,
				Stream:      nil,
			},
			{
				OpID:        "op2",
				Operation:   "tools_call",
				TimestampMs: 1000,
				LatencyMs:   100,
				OK:          true,
				Stream: &types.StreamInfo{
					IsStreaming:   true,
					EventsCount:   10,
					EndedNormally: true,
					Stalled:       false,
				},
			},
			{
				OpID:        "op3",
				Operation:   "ping",
				TimestampMs: 1500,
				LatencyMs:   10,
				OK:          true,
				Stream:      nil,
			},
			{
				OpID:        "op4",
				Operation:   "tools_call",
				TimestampMs: 2000,
				LatencyMs:   100,
				OK:          true,
				Stream: &types.StreamInfo{
					IsStreaming:   true,
					EventsCount:   20,
					EndedNormally: true,
					Stalled:       false,
				},
			},
			{
				OpID:        "op5",
				Operation:   "ping",
				TimestampMs: 2500,
				LatencyMs:   10,
				OK:          true,
				Stream:      nil,
			},
		},
	}

	ts.AddTelemetryBatch(runID, batch)

	metrics, err := ts.GetStreamingMetrics(runID)
	if err != nil {
		t.Fatalf("Failed to get streaming metrics: %v", err)
	}

	if metrics.EventsReceived != 30 {
		t.Errorf("Expected EventsReceived=30, got %d", metrics.EventsReceived)
	}

	if metrics.StreamStartTimeMs != 1000 {
		t.Errorf("Expected StreamStartTimeMs=1000, got %d", metrics.StreamStartTimeMs)
	}

	if metrics.LastEventTimeMs != 2000 {
		t.Errorf("Expected LastEventTimeMs=2000, got %d", metrics.LastEventTimeMs)
	}
}

func TestTelemetryStore_GetStreamingMetrics_NonStreamingFlaggedAsStreaming(t *testing.T) {
	ts := NewTelemetryStore()
	runID := "run_00000000000000d7"

	batch := TelemetryBatchRequest{
		Operations: []types.OperationOutcome{
			{
				OpID:        "op1",
				Operation:   "tools_call",
				TimestampMs: 1000,
				LatencyMs:   100,
				OK:          true,
				Stream: &types.StreamInfo{
					IsStreaming:   false,
					EventsCount:   5,
					EndedNormally: true,
					Stalled:       false,
				},
			},
		},
	}

	ts.AddTelemetryBatch(runID, batch)

	metrics, err := ts.GetStreamingMetrics(runID)
	if err != nil {
		t.Fatalf("Failed to get streaming metrics: %v", err)
	}

	if metrics.EventsReceived != 5 {
		t.Errorf("Expected EventsReceived=5, got %d", metrics.EventsReceived)
	}

	if metrics.StreamStartTimeMs != 0 {
		t.Errorf("Expected StreamStartTimeMs=0 (IsStreaming=false), got %d", metrics.StreamStartTimeMs)
	}

	if metrics.LastEventTimeMs != 0 {
		t.Errorf("Expected LastEventTimeMs=0 (IsStreaming=false), got %d", metrics.LastEventTimeMs)
	}
}

func TestTelemetryStore_GetStreamingMetrics_ZeroEventsCount(t *testing.T) {
	ts := NewTelemetryStore()
	runID := "run_00000000000000d8"

	batch := TelemetryBatchRequest{
		Operations: []types.OperationOutcome{
			{
				OpID:        "op1",
				Operation:   "tools_call",
				TimestampMs: 1000,
				LatencyMs:   100,
				OK:          true,
				Stream: &types.StreamInfo{
					IsStreaming:   true,
					EventsCount:   0,
					EndedNormally: true,
					Stalled:       false,
				},
			},
		},
	}

	ts.AddTelemetryBatch(runID, batch)

	metrics, err := ts.GetStreamingMetrics(runID)
	if err != nil {
		t.Fatalf("Failed to get streaming metrics: %v", err)
	}

	if metrics.EventsReceived != 0 {
		t.Errorf("Expected EventsReceived=0, got %d", metrics.EventsReceived)
	}

	if metrics.StreamStartTimeMs != 1000 {
		t.Errorf("Expected StreamStartTimeMs=1000, got %d", metrics.StreamStartTimeMs)
	}
}
