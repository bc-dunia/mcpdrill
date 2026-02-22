package api

import (
	"sync"
	"testing"

	"github.com/bc-dunia/mcpdrill/internal/types"
)

func TestTelemetryStore_AddAndGet(t *testing.T) {
	ts := NewTelemetryStore()

	batch := TelemetryBatchRequest{
		Operations: []types.OperationOutcome{
			{
				OpID:        "op1",
				Operation:   "tools_list",
				LatencyMs:   100,
				OK:          true,
				TimestampMs: 1000,
			},
			{
				OpID:        "op2",
				Operation:   "tools_call",
				ToolName:    "echo",
				LatencyMs:   200,
				OK:          true,
				TimestampMs: 1200,
			},
		},
	}

	ts.AddTelemetryBatch("run_0000000000000001", batch)

	data, err := ts.GetTelemetryData("run_0000000000000001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if data.RunID != "run_0000000000000001" {
		t.Errorf("expected run_id 'run_0000000000000001', got '%s'", data.RunID)
	}

	if len(data.Operations) != 2 {
		t.Errorf("expected 2 operations, got %d", len(data.Operations))
	}

	if data.StartTimeMs != 1000 {
		t.Errorf("expected start_time_ms 1000, got %d", data.StartTimeMs)
	}

	if data.EndTimeMs != 1200 {
		t.Errorf("expected end_time_ms 1200, got %d", data.EndTimeMs)
	}

	if data.Operations[0].Operation != "tools_list" {
		t.Errorf("expected first operation 'tools_list', got '%s'", data.Operations[0].Operation)
	}

	if data.Operations[1].ToolName != "echo" {
		t.Errorf("expected second operation tool_name 'echo', got '%s'", data.Operations[1].ToolName)
	}
}

func TestTelemetryStore_PreservesSessionIDInTelemetryData(t *testing.T) {
	ts := NewTelemetryStore()

	batch := TelemetryBatchRequest{
		Operations: []types.OperationOutcome{
			{
				OpID:        "op1",
				Operation:   "tools_call",
				LatencyMs:   100,
				OK:          true,
				TimestampMs: 1000,
				SessionID:   "sess_00000000000001",
			},
		},
	}

	ts.AddTelemetryBatch("run_0000000000000e111", batch)

	data, err := ts.GetTelemetryData("run_0000000000000e111")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(data.Operations) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(data.Operations))
	}
	if data.Operations[0].SessionID != "sess_00000000000001" {
		t.Fatalf("expected session_id %q, got %q", "sess_00000000000001", data.Operations[0].SessionID)
	}
}

func TestAddTelemetryBatchWithContext_DoesNotMutateInputBatch(t *testing.T) {
	ts := NewTelemetryStore()

	batch := TelemetryBatchRequest{
		Operations: []types.OperationOutcome{
			{
				OpID:        "op1",
				Operation:   "tools/call",
				LatencyMs:   10,
				OK:          true,
				TimestampMs: 1000,
			},
		},
	}

	ts.AddTelemetryBatchWithContext("run_0000000000000e221", batch, "worker-a", "stage-a", "stg_0000000000a221", "1")
	ts.AddTelemetryBatchWithContext("run_0000000000000e222", batch, "worker-b", "stage-b", "stg_0000000000a222", "2")

	logsA, _, err := ts.QueryLogs("run_0000000000000e221", LogFilters{Limit: 10, Offset: 0, Order: "desc"})
	if err != nil {
		t.Fatalf("unexpected error querying run A: %v", err)
	}
	logsB, _, err := ts.QueryLogs("run_0000000000000e222", LogFilters{Limit: 10, Offset: 0, Order: "desc"})
	if err != nil {
		t.Fatalf("unexpected error querying run B: %v", err)
	}

	if len(logsA) != 1 || len(logsB) != 1 {
		t.Fatalf("expected one log per run, got A=%d B=%d", len(logsA), len(logsB))
	}

	if logsA[0].WorkerID != "worker-a" || logsA[0].Stage != "stage-a" || logsA[0].StageID != "stg_0000000000a221" || logsA[0].VUID != "1" {
		t.Fatalf("run A context mismatch: worker=%q stage=%q stage_id=%q vu_id=%q", logsA[0].WorkerID, logsA[0].Stage, logsA[0].StageID, logsA[0].VUID)
	}
	if logsB[0].WorkerID != "worker-b" || logsB[0].Stage != "stage-b" || logsB[0].StageID != "stg_0000000000a222" || logsB[0].VUID != "2" {
		t.Fatalf("run B context mismatch: worker=%q stage=%q stage_id=%q vu_id=%q", logsB[0].WorkerID, logsB[0].Stage, logsB[0].StageID, logsB[0].VUID)
	}

	if batch.Operations[0].WorkerID != "" || batch.Operations[0].Stage != "" || batch.Operations[0].StageID != "" || batch.Operations[0].VUID != "" {
		t.Fatalf("input batch must remain unchanged: worker=%q stage=%q stage_id=%q vu_id=%q", batch.Operations[0].WorkerID, batch.Operations[0].Stage, batch.Operations[0].StageID, batch.Operations[0].VUID)
	}
}

func TestTelemetryStore_MultipleBatches(t *testing.T) {
	ts := NewTelemetryStore()

	batch1 := TelemetryBatchRequest{
		Operations: []types.OperationOutcome{
			{OpID: "op1", Operation: "tools_list", LatencyMs: 100, OK: true, TimestampMs: 1000},
		},
	}

	batch2 := TelemetryBatchRequest{
		Operations: []types.OperationOutcome{
			{OpID: "op2", Operation: "tools_call", LatencyMs: 200, OK: true, TimestampMs: 2000},
			{OpID: "op3", Operation: "ping", LatencyMs: 50, OK: true, TimestampMs: 2100},
		},
	}

	ts.AddTelemetryBatch("run_0000000000000001", batch1)
	ts.AddTelemetryBatch("run_0000000000000001", batch2)

	data, err := ts.GetTelemetryData("run_0000000000000001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(data.Operations) != 3 {
		t.Errorf("expected 3 operations, got %d", len(data.Operations))
	}

	if data.StartTimeMs != 1000 {
		t.Errorf("expected start_time_ms 1000, got %d", data.StartTimeMs)
	}

	if data.EndTimeMs != 2100 {
		t.Errorf("expected end_time_ms 2100, got %d", data.EndTimeMs)
	}
}

func TestTelemetryStore_RunNotFound(t *testing.T) {
	ts := NewTelemetryStore()

	_, err := ts.GetTelemetryData("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent run")
	}
}

func TestTelemetryStore_SetRunMetadata(t *testing.T) {
	ts := NewTelemetryStore()

	batch := TelemetryBatchRequest{
		Operations: []types.OperationOutcome{
			{OpID: "op1", Operation: "tools_list", LatencyMs: 100, OK: true, TimestampMs: 1000},
		},
	}

	ts.AddTelemetryBatch("run_0000000000000001", batch)
	ts.SetRunMetadata("run_0000000000000001", "scenario_test", "user_requested")

	data, err := ts.GetTelemetryData("run_0000000000000001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if data.ScenarioID != "scenario_test" {
		t.Errorf("expected scenario_id 'scenario_test', got '%s'", data.ScenarioID)
	}

	if data.StopReason != "user_requested" {
		t.Errorf("expected stop_reason 'user_requested', got '%s'", data.StopReason)
	}
}

func TestTelemetryStore_SetRunMetadataCreatesRun(t *testing.T) {
	ts := NewTelemetryStore()

	ts.SetRunMetadata("run_000000000000000a", "scenario_new", "completed")

	data, err := ts.GetTelemetryData("run_000000000000000a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if data.ScenarioID != "scenario_new" {
		t.Errorf("expected scenario_id 'scenario_new', got '%s'", data.ScenarioID)
	}

	if len(data.Operations) != 0 {
		t.Errorf("expected 0 operations, got %d", len(data.Operations))
	}
}

func TestTelemetryStore_ConcurrentSubmissions(t *testing.T) {
	ts := NewTelemetryStore()

	var wg sync.WaitGroup
	numWorkers := 10
	opsPerWorker := 100

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < opsPerWorker; i++ {
				batch := TelemetryBatchRequest{
					Operations: []types.OperationOutcome{
						{
							OpID:        "op",
							Operation:   "tools_list",
							LatencyMs:   100,
							OK:          true,
							TimestampMs: int64(workerID*1000 + i),
						},
					},
				}
				ts.AddTelemetryBatch("run_00000000000000cc", batch)
			}
		}(w)
	}

	wg.Wait()

	data, err := ts.GetTelemetryData("run_00000000000000cc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedOps := numWorkers * opsPerWorker
	if len(data.Operations) != expectedOps {
		t.Errorf("expected %d operations, got %d", expectedOps, len(data.Operations))
	}
}

func TestTelemetryStore_GetOperationCount(t *testing.T) {
	ts := NewTelemetryStore()

	if ts.GetOperationCount("nonexistent") != 0 {
		t.Error("expected 0 for nonexistent run")
	}

	batch := TelemetryBatchRequest{
		Operations: []types.OperationOutcome{
			{OpID: "op1", Operation: "tools_list", LatencyMs: 100, OK: true, TimestampMs: 1000},
			{OpID: "op2", Operation: "ping", LatencyMs: 50, OK: true, TimestampMs: 1100},
		},
	}

	ts.AddTelemetryBatch("run_0000000000000001", batch)

	if ts.GetOperationCount("run_0000000000000001") != 2 {
		t.Errorf("expected 2 operations, got %d", ts.GetOperationCount("run_0000000000000001"))
	}
}

func TestTelemetryStore_HasRun(t *testing.T) {
	ts := NewTelemetryStore()

	if ts.HasRun("nonexistent") {
		t.Error("expected false for nonexistent run")
	}

	batch := TelemetryBatchRequest{
		Operations: []types.OperationOutcome{
			{OpID: "op1", Operation: "tools_list", LatencyMs: 100, OK: true, TimestampMs: 1000},
		},
	}

	ts.AddTelemetryBatch("run_0000000000000001", batch)

	if !ts.HasRun("run_0000000000000001") {
		t.Error("expected true for existing run")
	}
}

func TestTelemetryStore_DeleteRun(t *testing.T) {
	ts := NewTelemetryStore()

	batch := TelemetryBatchRequest{
		Operations: []types.OperationOutcome{
			{OpID: "op1", Operation: "tools_list", LatencyMs: 100, OK: true, TimestampMs: 1000},
		},
	}

	ts.AddTelemetryBatch("run_0000000000000001", batch)

	if !ts.HasRun("run_0000000000000001") {
		t.Error("expected run to exist before delete")
	}

	ts.DeleteRun("run_0000000000000001")

	if ts.HasRun("run_0000000000000001") {
		t.Error("expected run to not exist after delete")
	}

	_, err := ts.GetTelemetryData("run_0000000000000001")
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestTelemetryStore_RunCount(t *testing.T) {
	ts := NewTelemetryStore()

	if ts.RunCount() != 0 {
		t.Errorf("expected 0 runs, got %d", ts.RunCount())
	}

	batch := TelemetryBatchRequest{
		Operations: []types.OperationOutcome{
			{OpID: "op1", Operation: "tools_list", LatencyMs: 100, OK: true, TimestampMs: 1000},
		},
	}

	ts.AddTelemetryBatch("run_0000000000000001", batch)
	ts.AddTelemetryBatch("run_0000000000000002", batch)

	if ts.RunCount() != 2 {
		t.Errorf("expected 2 runs, got %d", ts.RunCount())
	}

	ts.DeleteRun("run_0000000000000001")

	if ts.RunCount() != 1 {
		t.Errorf("expected 1 run after delete, got %d", ts.RunCount())
	}
}

func TestTelemetryStore_OperationConversion(t *testing.T) {
	ts := NewTelemetryStore()

	batch := TelemetryBatchRequest{
		Operations: []types.OperationOutcome{
			{
				OpID:        "op1",
				Operation:   "tools_call",
				ToolName:    "echo",
				LatencyMs:   150,
				OK:          false,
				ErrorType:   "timeout",
				ErrorCode:   "TIMEOUT",
				HTTPStatus:  504,
				TimestampMs: 1000,
			},
		},
	}

	ts.AddTelemetryBatch("run_0000000000000001", batch)

	data, err := ts.GetTelemetryData("run_0000000000000001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	op := data.Operations[0]
	if op.Operation != "tools_call" {
		t.Errorf("expected operation 'tools_call', got '%s'", op.Operation)
	}
	if op.ToolName != "echo" {
		t.Errorf("expected tool_name 'echo', got '%s'", op.ToolName)
	}
	if op.LatencyMs != 150 {
		t.Errorf("expected latency_ms 150, got %d", op.LatencyMs)
	}
	if op.OK {
		t.Error("expected OK to be false")
	}
	if op.ErrorType != "timeout" {
		t.Errorf("expected error_type 'timeout', got '%s'", op.ErrorType)
	}
}

func TestTelemetryStore_TimeRangeTracking(t *testing.T) {
	ts := NewTelemetryStore()

	batch1 := TelemetryBatchRequest{
		Operations: []types.OperationOutcome{
			{OpID: "op1", Operation: "tools_list", LatencyMs: 100, OK: true, TimestampMs: 5000},
		},
	}

	batch2 := TelemetryBatchRequest{
		Operations: []types.OperationOutcome{
			{OpID: "op2", Operation: "tools_list", LatencyMs: 100, OK: true, TimestampMs: 2000},
		},
	}

	batch3 := TelemetryBatchRequest{
		Operations: []types.OperationOutcome{
			{OpID: "op3", Operation: "tools_list", LatencyMs: 100, OK: true, TimestampMs: 8000},
		},
	}

	ts.AddTelemetryBatch("run_0000000000000001", batch1)
	ts.AddTelemetryBatch("run_0000000000000001", batch2)
	ts.AddTelemetryBatch("run_0000000000000001", batch3)

	data, err := ts.GetTelemetryData("run_0000000000000001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if data.StartTimeMs != 2000 {
		t.Errorf("expected start_time_ms 2000 (earliest), got %d", data.StartTimeMs)
	}

	if data.EndTimeMs != 8000 {
		t.Errorf("expected end_time_ms 8000 (latest), got %d", data.EndTimeMs)
	}
}

func TestTelemetryStore_MultipleRuns(t *testing.T) {
	ts := NewTelemetryStore()

	batch1 := TelemetryBatchRequest{
		Operations: []types.OperationOutcome{
			{OpID: "op1", Operation: "tools_list", LatencyMs: 100, OK: true, TimestampMs: 1000},
		},
	}

	batch2 := TelemetryBatchRequest{
		Operations: []types.OperationOutcome{
			{OpID: "op2", Operation: "ping", LatencyMs: 50, OK: true, TimestampMs: 2000},
		},
	}

	ts.AddTelemetryBatch("run_0000000000000001", batch1)
	ts.AddTelemetryBatch("run_0000000000000002", batch2)

	data1, err := ts.GetTelemetryData("run_0000000000000001")
	if err != nil {
		t.Fatalf("unexpected error for run_1: %v", err)
	}

	data2, err := ts.GetTelemetryData("run_0000000000000002")
	if err != nil {
		t.Fatalf("unexpected error for run_2: %v", err)
	}

	if len(data1.Operations) != 1 || data1.Operations[0].Operation != "tools_list" {
		t.Error("run_1 data incorrect")
	}

	if len(data2.Operations) != 1 || data2.Operations[0].Operation != "ping" {
		t.Error("run_2 data incorrect")
	}
}

func TestTelemetryStore_ListRunsForRetention(t *testing.T) {
	ts := NewTelemetryStore()

	batch1 := TelemetryBatchRequest{
		Operations: []types.OperationOutcome{
			{OpID: "op1", Operation: "tools_list", LatencyMs: 100, OK: true, TimestampMs: 1000},
		},
	}

	batch2 := TelemetryBatchRequest{
		Operations: []types.OperationOutcome{
			{OpID: "op2", Operation: "ping", LatencyMs: 50, OK: true, TimestampMs: 2000},
			{OpID: "op3", Operation: "ping", LatencyMs: 60, OK: true, TimestampMs: 3000},
		},
	}

	ts.AddTelemetryBatch("run_0000000000000001", batch1)
	ts.AddTelemetryBatch("run_0000000000000002", batch2)

	runs := ts.ListRunsForRetention()
	if len(runs) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(runs))
	}

	runMap := make(map[string]int64)
	for _, r := range runs {
		runMap[r.RunID] = r.EndTimeMs
	}

	if runMap["run_0000000000000001"] != 1000 {
		t.Errorf("expected run_1 end time 1000, got %d", runMap["run_0000000000000001"])
	}
	if runMap["run_0000000000000002"] != 3000 {
		t.Errorf("expected run_2 end time 3000, got %d", runMap["run_0000000000000002"])
	}
}

func TestTelemetryStore_MemoryLimits(t *testing.T) {
	config := &TelemetryStoreConfig{
		MaxOperationsPerRun: 5,
		MaxLogsPerRun:       5,
		MaxTotalRuns:        2,
	}
	ts := NewTelemetryStoreWithConfig(config)

	// Add operations to first run
	batch := TelemetryBatchRequest{
		Operations: []types.OperationOutcome{
			{TimestampMs: 1000, Operation: "op1", ToolName: "tool1", LatencyMs: 100, OK: true},
			{TimestampMs: 1001, Operation: "op2", ToolName: "tool1", LatencyMs: 100, OK: true},
			{TimestampMs: 1002, Operation: "op3", ToolName: "tool1", LatencyMs: 100, OK: true},
			{TimestampMs: 1003, Operation: "op4", ToolName: "tool1", LatencyMs: 100, OK: true},
			{TimestampMs: 1004, Operation: "op5", ToolName: "tool1", LatencyMs: 100, OK: true},
			{TimestampMs: 1005, Operation: "op6", ToolName: "tool1", LatencyMs: 100, OK: true}, // Should be truncated
			{TimestampMs: 1006, Operation: "op7", ToolName: "tool1", LatencyMs: 100, OK: true}, // Should be truncated
		},
	}
	ts.AddTelemetryBatch("run_1", batch)

	// Check that only 5 operations were stored
	count := ts.GetOperationCount("run_1")
	if count != 5 {
		t.Errorf("Expected 5 operations (limit), got %d", count)
	}

	// Check truncation flag
	opsTrunc, logsTrunc := ts.IsTruncated("run_1")
	if !opsTrunc {
		t.Error("Expected operations to be truncated")
	}
	if !logsTrunc {
		t.Error("Expected logs to be truncated")
	}
}

func TestTelemetryStore_RunEviction(t *testing.T) {
	config := &TelemetryStoreConfig{
		MaxOperationsPerRun: 100,
		MaxLogsPerRun:       100,
		MaxTotalRuns:        2,
	}
	ts := NewTelemetryStoreWithConfig(config)

	// Add first run
	ts.AddTelemetryBatch("run_1", TelemetryBatchRequest{
		Operations: []types.OperationOutcome{
			{TimestampMs: 1000, Operation: "op1", OK: true},
		},
	})

	// Add second run
	ts.AddTelemetryBatch("run_2", TelemetryBatchRequest{
		Operations: []types.OperationOutcome{
			{TimestampMs: 2000, Operation: "op2", OK: true},
		},
	})

	// Both runs should exist
	if !ts.HasRun("run_1") {
		t.Error("Expected run_1 to exist")
	}
	if !ts.HasRun("run_2") {
		t.Error("Expected run_2 to exist")
	}

	// Add third run - should evict run_1
	ts.AddTelemetryBatch("run_3", TelemetryBatchRequest{
		Operations: []types.OperationOutcome{
			{TimestampMs: 3000, Operation: "op3", OK: true},
		},
	})

	// run_1 should be evicted
	if ts.HasRun("run_1") {
		t.Error("Expected run_1 to be evicted")
	}
	if !ts.HasRun("run_2") {
		t.Error("Expected run_2 to still exist")
	}
	if !ts.HasRun("run_3") {
		t.Error("Expected run_3 to exist")
	}

	if ts.RunCount() != 2 {
		t.Errorf("Expected 2 runs, got %d", ts.RunCount())
	}
}

func TestTelemetryStore_DefaultConfig(t *testing.T) {
	config := DefaultTelemetryStoreConfig()

	if config.MaxOperationsPerRun != 20000000 {
		t.Errorf("Expected MaxOperationsPerRun=20000000, got %d", config.MaxOperationsPerRun)
	}
	if config.MaxLogsPerRun != 20000000 {
		t.Errorf("Expected MaxLogsPerRun=20000000, got %d", config.MaxLogsPerRun)
	}
	if config.MaxTotalRuns != 100 {
		t.Errorf("Expected MaxTotalRuns=100, got %d", config.MaxTotalRuns)
	}
}

func TestTelemetryStore_CalculateBucketSize_LongRunTargetsAbout25Points(t *testing.T) {
	ts := NewTelemetryStore()

	logs := []OperationLog{
		{TimestampMs: 0},
		{TimestampMs: 3600000}, // 1 hour
	}

	bucketSize := ts.calculateBucketSize(logs)
	expected := int64(3600000 / 25)
	if bucketSize != expected {
		t.Fatalf("expected bucket size %dms for 1h run, got %dms", expected, bucketSize)
	}
}

func TestTelemetryStore_GetStabilityMetrics_IncludeEvents(t *testing.T) {
	ts := NewTelemetryStore()
	runID := "run_0000000000000abc1"

	ts.AddTelemetryBatch(runID, TelemetryBatchRequest{
		Operations: []types.OperationOutcome{
			{
				TimestampMs: 1000,
				Operation:   "tools_call",
				SessionID:   "sess_1",
				OK:          true,
				LatencyMs:   10,
			},
			{
				TimestampMs: 1100,
				Operation:   "tools_call",
				SessionID:   "sess_1",
				OK:          false,
				ErrorType:   "connection_dropped",
				LatencyMs:   15,
			},
		},
	})

	withoutEvents := ts.GetStabilityMetrics(runID, false, false)
	if withoutEvents == nil {
		t.Fatal("expected stability metrics")
	}
	if len(withoutEvents.Events) != 0 {
		t.Fatalf("expected no events when include_events=false, got %d", len(withoutEvents.Events))
	}

	withEvents := ts.GetStabilityMetrics(runID, true, false)
	if withEvents == nil {
		t.Fatal("expected stability metrics")
	}
	if len(withEvents.Events) == 0 {
		t.Fatal("expected events when include_events=true")
	}

	foundCreated := false
	foundDropped := false
	for _, event := range withEvents.Events {
		if event.EventType == "created" {
			foundCreated = true
		}
		if event.EventType == "dropped" {
			foundDropped = true
		}
	}
	if !foundCreated {
		t.Error("expected a created event")
	}
	if !foundDropped {
		t.Error("expected a dropped event")
	}
}

func TestTelemetryStore_GetStabilityMetrics_TimeSeriesCountsDroppedSessionsUniquelyPerBucket(t *testing.T) {
	ts := NewTelemetryStore()
	runID := "run_0000000000000abc2"

	ts.AddTelemetryBatch(runID, TelemetryBatchRequest{
		Operations: []types.OperationOutcome{
			{
				TimestampMs: 1000,
				Operation:   "tools_call",
				SessionID:   "sess_1",
				OK:          false,
				ErrorType:   "connection_dropped",
				LatencyMs:   10,
			},
			{
				TimestampMs: 1050,
				Operation:   "tools_call",
				SessionID:   "sess_1",
				OK:          false,
				ErrorType:   "connection_dropped",
				LatencyMs:   12,
			},
		},
	})

	stability := ts.GetStabilityMetrics(runID, false, true)
	if stability == nil {
		t.Fatal("expected stability metrics")
	}
	if len(stability.TimeSeriesData) != 1 {
		t.Fatalf("expected 1 time-series point, got %d", len(stability.TimeSeriesData))
	}
	if stability.TimeSeriesData[0].DroppedSessions != 1 {
		t.Fatalf("expected dropped_sessions 1 for repeated drops in same bucket, got %d", stability.TimeSeriesData[0].DroppedSessions)
	}
}
