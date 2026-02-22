package analysis

import (
	"sync"
	"testing"
)

func TestNewAggregator(t *testing.T) {
	agg := NewAggregator()
	if agg == nil {
		t.Fatal("NewAggregator returned nil")
	}
	if agg.OperationCount() != 0 {
		t.Errorf("expected 0 operations, got %d", agg.OperationCount())
	}
}

func TestAddOperation(t *testing.T) {
	agg := NewAggregator()

	agg.AddOperation(OperationResult{
		Operation: "initialize",
		LatencyMs: 100,
		OK:        true,
	})

	if agg.OperationCount() != 1 {
		t.Errorf("expected 1 operation, got %d", agg.OperationCount())
	}

	agg.AddOperation(OperationResult{
		Operation: "tools_call",
		ToolName:  "read_file",
		LatencyMs: 50,
		OK:        true,
	})

	if agg.OperationCount() != 2 {
		t.Errorf("expected 2 operations, got %d", agg.OperationCount())
	}
}

func TestComputeEmpty(t *testing.T) {
	agg := NewAggregator()
	metrics := agg.Compute()

	if metrics.TotalOps != 0 {
		t.Errorf("expected 0 total ops, got %d", metrics.TotalOps)
	}
	if metrics.SuccessOps != 0 {
		t.Errorf("expected 0 success ops, got %d", metrics.SuccessOps)
	}
	if metrics.FailureOps != 0 {
		t.Errorf("expected 0 failure ops, got %d", metrics.FailureOps)
	}
	if len(metrics.ByOperation) != 0 {
		t.Errorf("expected empty ByOperation map")
	}
	if len(metrics.ByTool) != 0 {
		t.Errorf("expected empty ByTool map")
	}
}

func TestComputeBasicMetrics(t *testing.T) {
	agg := NewAggregator()
	oneSecondMs := int64(1000)
	agg.SetTimeRange(0, oneSecondMs)

	agg.AddOperation(OperationResult{Operation: "initialize", LatencyMs: 100, OK: true})
	agg.AddOperation(OperationResult{Operation: "tools_list", LatencyMs: 50, OK: true})
	agg.AddOperation(OperationResult{Operation: "tools_call", ToolName: "read_file", LatencyMs: 200, OK: true})
	agg.AddOperation(OperationResult{Operation: "tools_call", ToolName: "read_file", LatencyMs: 150, OK: false, ErrorType: "timeout"})
	agg.AddOperation(OperationResult{Operation: "ping", LatencyMs: 10, OK: true})

	metrics := agg.Compute()

	if metrics.TotalOps != 5 {
		t.Errorf("expected 5 total ops, got %d", metrics.TotalOps)
	}
	if metrics.SuccessOps != 4 {
		t.Errorf("expected 4 success ops, got %d", metrics.SuccessOps)
	}
	if metrics.FailureOps != 1 {
		t.Errorf("expected 1 failure op, got %d", metrics.FailureOps)
	}
	if metrics.RPS != 5.0 {
		t.Errorf("expected 5.0 RPS, got %f", metrics.RPS)
	}
	if metrics.ErrorRate != 0.2 {
		t.Errorf("expected 0.2 (20%%) error rate, got %f", metrics.ErrorRate)
	}
}

func TestComputeByOperation(t *testing.T) {
	agg := NewAggregator()

	agg.AddOperation(OperationResult{Operation: "initialize", LatencyMs: 100, OK: true})
	agg.AddOperation(OperationResult{Operation: "initialize", LatencyMs: 120, OK: true})
	agg.AddOperation(OperationResult{Operation: "tools_call", ToolName: "write_file", LatencyMs: 200, OK: true})
	agg.AddOperation(OperationResult{Operation: "tools_call", ToolName: "write_file", LatencyMs: 300, OK: false})

	metrics := agg.Compute()

	initMetrics, ok := metrics.ByOperation["initialize"]
	if !ok {
		t.Fatal("missing initialize metrics")
	}
	if initMetrics.TotalOps != 2 {
		t.Errorf("expected 2 initialize ops, got %d", initMetrics.TotalOps)
	}
	if initMetrics.SuccessOps != 2 {
		t.Errorf("expected 2 initialize success, got %d", initMetrics.SuccessOps)
	}
	if initMetrics.ErrorRate != 0.0 {
		t.Errorf("expected 0.0 error rate for initialize, got %f", initMetrics.ErrorRate)
	}

	toolsMetrics, ok := metrics.ByOperation["tools/call"]
	if !ok {
		t.Fatal("missing tools/call metrics")
	}
	if toolsMetrics.TotalOps != 2 {
		t.Errorf("expected 2 tools/call ops, got %d", toolsMetrics.TotalOps)
	}
	if toolsMetrics.FailureOps != 1 {
		t.Errorf("expected 1 tools/call failure, got %d", toolsMetrics.FailureOps)
	}
	if toolsMetrics.ErrorRate != 0.5 {
		t.Errorf("expected 0.5 (50%%) error rate for tools/call, got %f", toolsMetrics.ErrorRate)
	}
}

func TestComputeByTool(t *testing.T) {
	agg := NewAggregator()

	agg.AddOperation(OperationResult{Operation: "tools_call", ToolName: "read_file", LatencyMs: 50, OK: true})
	agg.AddOperation(OperationResult{Operation: "tools_call", ToolName: "read_file", LatencyMs: 60, OK: true})
	agg.AddOperation(OperationResult{Operation: "tools_call", ToolName: "write_file", LatencyMs: 100, OK: true})
	agg.AddOperation(OperationResult{Operation: "tools_call", ToolName: "write_file", LatencyMs: 150, OK: false})
	agg.AddOperation(OperationResult{Operation: "initialize", LatencyMs: 200, OK: true})

	metrics := agg.Compute()

	if len(metrics.ByTool) != 2 {
		t.Errorf("expected 2 tools, got %d", len(metrics.ByTool))
	}

	readMetrics, ok := metrics.ByTool["read_file"]
	if !ok {
		t.Fatal("missing read_file metrics")
	}
	if readMetrics.TotalOps != 2 {
		t.Errorf("expected 2 read_file ops, got %d", readMetrics.TotalOps)
	}
	if readMetrics.ErrorRate != 0.0 {
		t.Errorf("expected 0.0 error rate for read_file, got %f", readMetrics.ErrorRate)
	}

	writeMetrics, ok := metrics.ByTool["write_file"]
	if !ok {
		t.Fatal("missing write_file metrics")
	}
	if writeMetrics.TotalOps != 2 {
		t.Errorf("expected 2 write_file ops, got %d", writeMetrics.TotalOps)
	}
	if writeMetrics.ErrorRate != 0.5 {
		t.Errorf("expected 0.5 (50%%) error rate for write_file, got %f", writeMetrics.ErrorRate)
	}
}

func TestComputePercentile(t *testing.T) {
	tests := []struct {
		name      string
		latencies []int
		p         float64
		expected  int
	}{
		{"empty", []int{}, 50, 0},
		{"single", []int{100}, 50, 100},
		{"single p99", []int{100}, 99, 100},
		{"two values p50", []int{100, 200}, 50, 200},
		{"ten values p50", []int{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}, 50, 60},
		{"ten values p95", []int{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}, 95, 100},
		{"ten values p99", []int{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}, 99, 100},
		{"unsorted input", []int{50, 10, 90, 30, 70}, 50, 50},
		{"negative percentile clamps to min", []int{10, 20, 30}, -5, 10},
		{"over-100 percentile clamps to max", []int{10, 20, 30}, 120, 30},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := computePercentile(tt.latencies, tt.p)
			if result != tt.expected {
				t.Errorf("computePercentile(%v, %f) = %d, expected %d", tt.latencies, tt.p, result, tt.expected)
			}
		})
	}
}

func TestComputePercentileDoesNotModifyInput(t *testing.T) {
	original := []int{50, 10, 90, 30, 70}
	input := make([]int, len(original))
	copy(input, original)

	computePercentile(input, 50)

	for i := range original {
		if input[i] != original[i] {
			t.Errorf("input was modified: expected %v, got %v", original, input)
			break
		}
	}
}

func TestConcurrentAddOperation(t *testing.T) {
	agg := NewAggregator()
	var wg sync.WaitGroup
	numGoroutines := 100
	opsPerGoroutine := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				agg.AddOperation(OperationResult{
					Operation: "tools_call",
					ToolName:  "test_tool",
					LatencyMs: id*100 + j,
					OK:        j%2 == 0,
				})
			}
		}(i)
	}

	wg.Wait()

	expectedOps := numGoroutines * opsPerGoroutine
	if agg.OperationCount() != expectedOps {
		t.Errorf("expected %d operations, got %d", expectedOps, agg.OperationCount())
	}

	metrics := agg.Compute()
	if metrics.TotalOps != expectedOps {
		t.Errorf("expected %d total ops in metrics, got %d", expectedOps, metrics.TotalOps)
	}
}

func TestConcurrentComputeWhileAdding(t *testing.T) {
	agg := NewAggregator()
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			agg.AddOperation(OperationResult{
				Operation: "ping",
				LatencyMs: i,
				OK:        true,
			})
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = agg.Compute()
		}
	}()

	wg.Wait()
}

func TestReset(t *testing.T) {
	agg := NewAggregator()
	agg.SetTimeRange(0, 1000)
	agg.AddOperation(OperationResult{Operation: "ping", LatencyMs: 10, OK: true})
	agg.AddOperation(OperationResult{Operation: "ping", LatencyMs: 20, OK: true})

	if agg.OperationCount() != 2 {
		t.Errorf("expected 2 operations before reset, got %d", agg.OperationCount())
	}

	agg.Reset()

	if agg.OperationCount() != 0 {
		t.Errorf("expected 0 operations after reset, got %d", agg.OperationCount())
	}

	metrics := agg.Compute()
	if metrics.TotalOps != 0 {
		t.Errorf("expected 0 total ops after reset, got %d", metrics.TotalOps)
	}
	if metrics.RPS != 0 {
		t.Errorf("expected 0 RPS after reset, got %f", metrics.RPS)
	}
}

func TestLatencyPercentiles(t *testing.T) {
	agg := NewAggregator()

	latencies := []int{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}
	for _, lat := range latencies {
		agg.AddOperation(OperationResult{
			Operation: "ping",
			LatencyMs: lat,
			OK:        true,
		})
	}

	metrics := agg.Compute()

	if metrics.LatencyP50 != 60 {
		t.Errorf("expected P50=60, got %d", metrics.LatencyP50)
	}
	if metrics.LatencyP95 != 100 {
		t.Errorf("expected P95=100, got %d", metrics.LatencyP95)
	}
	if metrics.LatencyP99 != 100 {
		t.Errorf("expected P99=100, got %d", metrics.LatencyP99)
	}
}

func TestRPSCalculation(t *testing.T) {
	agg := NewAggregator()
	twoSecondsMs := int64(2000)
	agg.SetTimeRange(0, twoSecondsMs)

	for i := 0; i < 10; i++ {
		agg.AddOperation(OperationResult{Operation: "ping", LatencyMs: 10, OK: true})
	}

	metrics := agg.Compute()

	if metrics.RPS != 5.0 {
		t.Errorf("expected 5.0 RPS (10 ops / 2 sec), got %f", metrics.RPS)
	}
}

func TestRPSWithZeroDuration(t *testing.T) {
	agg := NewAggregator()
	sameTimestamp := int64(1000)
	agg.SetTimeRange(sameTimestamp, sameTimestamp)

	agg.AddOperation(OperationResult{Operation: "ping", LatencyMs: 10, OK: true})

	metrics := agg.Compute()

	if metrics.RPS != 0 {
		t.Errorf("expected 0 RPS with zero duration, got %f", metrics.RPS)
	}
}

func TestToolNameOnlyForToolsCall(t *testing.T) {
	agg := NewAggregator()

	agg.AddOperation(OperationResult{Operation: "initialize", ToolName: "should_be_ignored", LatencyMs: 100, OK: true})
	agg.AddOperation(OperationResult{Operation: "tools_call", ToolName: "actual_tool", LatencyMs: 50, OK: true})

	metrics := agg.Compute()

	if len(metrics.ByTool) != 1 {
		t.Errorf("expected 1 tool, got %d", len(metrics.ByTool))
	}
	if _, ok := metrics.ByTool["should_be_ignored"]; ok {
		t.Error("should_be_ignored should not be in ByTool")
	}
	if _, ok := metrics.ByTool["actual_tool"]; !ok {
		t.Error("actual_tool should be in ByTool")
	}
}

func TestEmptyToolName(t *testing.T) {
	agg := NewAggregator()

	agg.AddOperation(OperationResult{Operation: "tools_call", ToolName: "", LatencyMs: 50, OK: true})
	agg.AddOperation(OperationResult{Operation: "tools_call", ToolName: "real_tool", LatencyMs: 60, OK: true})

	metrics := agg.Compute()

	if len(metrics.ByTool) != 1 {
		t.Errorf("expected 1 tool (empty tool name ignored), got %d", len(metrics.ByTool))
	}
}

func TestSessionMetricsNoSessionData(t *testing.T) {
	agg := NewAggregator()
	agg.AddOperation(OperationResult{Operation: "ping", LatencyMs: 10, OK: true})

	metrics := agg.Compute()

	if metrics.SessionMetrics != nil {
		t.Error("expected nil session metrics when no session data")
	}
}

func TestSessionMetricsWithSessionIDs(t *testing.T) {
	agg := NewAggregator()

	agg.AddOperation(OperationResult{Operation: "ping", LatencyMs: 10, OK: true, SessionID: "sess-1"})
	agg.AddOperation(OperationResult{Operation: "ping", LatencyMs: 20, OK: true, SessionID: "sess-1"})
	agg.AddOperation(OperationResult{Operation: "ping", LatencyMs: 30, OK: true, SessionID: "sess-1"})
	agg.AddOperation(OperationResult{Operation: "ping", LatencyMs: 40, OK: true, SessionID: "sess-2"})
	agg.AddOperation(OperationResult{Operation: "ping", LatencyMs: 50, OK: true, SessionID: "sess-2"})

	metrics := agg.Compute()

	if metrics.SessionMetrics == nil {
		t.Fatal("expected session metrics")
	}
	if metrics.SessionMetrics.TotalSessions != 2 {
		t.Errorf("expected 2 sessions, got %d", metrics.SessionMetrics.TotalSessions)
	}
	if metrics.SessionMetrics.OpsPerSession != 2.5 {
		t.Errorf("expected 2.5 ops/session, got %f", metrics.SessionMetrics.OpsPerSession)
	}
	if metrics.SessionMetrics.SessionMode != "unknown" {
		t.Errorf("expected 'unknown' mode, got %s", metrics.SessionMetrics.SessionMode)
	}
}

func TestSessionMetricsWithModeSet(t *testing.T) {
	agg := NewAggregator()
	agg.SetSessionInfo("reuse", &SessionManagerMetrics{
		TotalCreated: 5,
		TotalEvicted: 2,
		Reconnects:   1,
	})

	agg.AddOperation(OperationResult{Operation: "ping", LatencyMs: 10, OK: true, SessionID: "sess-1"})
	agg.AddOperation(OperationResult{Operation: "ping", LatencyMs: 20, OK: true, SessionID: "sess-1"})
	agg.AddOperation(OperationResult{Operation: "ping", LatencyMs: 30, OK: true, SessionID: "sess-1"})
	agg.AddOperation(OperationResult{Operation: "ping", LatencyMs: 40, OK: true, SessionID: "sess-1"})

	metrics := agg.Compute()

	if metrics.SessionMetrics == nil {
		t.Fatal("expected session metrics")
	}
	if metrics.SessionMetrics.SessionMode != "reuse" {
		t.Errorf("expected 'reuse' mode, got %s", metrics.SessionMetrics.SessionMode)
	}
	if metrics.SessionMetrics.TotalSessions != 1 {
		t.Errorf("expected 1 session, got %d", metrics.SessionMetrics.TotalSessions)
	}
	if metrics.SessionMetrics.OpsPerSession != 4.0 {
		t.Errorf("expected 4.0 ops/session, got %f", metrics.SessionMetrics.OpsPerSession)
	}
	if metrics.SessionMetrics.TotalCreated != 5 {
		t.Errorf("expected 5 created, got %d", metrics.SessionMetrics.TotalCreated)
	}
	if metrics.SessionMetrics.TotalEvicted != 2 {
		t.Errorf("expected 2 evicted, got %d", metrics.SessionMetrics.TotalEvicted)
	}
	if metrics.SessionMetrics.Reconnects != 1 {
		t.Errorf("expected 1 reconnect, got %d", metrics.SessionMetrics.Reconnects)
	}
}

func TestSessionMetricsPerRequestMode(t *testing.T) {
	agg := NewAggregator()
	agg.SetSessionInfo("per_request", nil)

	agg.AddOperation(OperationResult{Operation: "ping", LatencyMs: 10, OK: true, SessionID: "sess-1"})
	agg.AddOperation(OperationResult{Operation: "ping", LatencyMs: 20, OK: true, SessionID: "sess-2"})
	agg.AddOperation(OperationResult{Operation: "ping", LatencyMs: 30, OK: true, SessionID: "sess-3"})

	metrics := agg.Compute()

	if metrics.SessionMetrics == nil {
		t.Fatal("expected session metrics")
	}
	if metrics.SessionMetrics.SessionMode != "per_request" {
		t.Errorf("expected 'per_request' mode, got %s", metrics.SessionMetrics.SessionMode)
	}
	if metrics.SessionMetrics.TotalSessions != 3 {
		t.Errorf("expected 3 sessions, got %d", metrics.SessionMetrics.TotalSessions)
	}
	if metrics.SessionMetrics.OpsPerSession != 1.0 {
		t.Errorf("expected 1.0 ops/session (per_request), got %f", metrics.SessionMetrics.OpsPerSession)
	}
}

func TestSessionMetricsPoolMode(t *testing.T) {
	agg := NewAggregator()
	agg.SetSessionInfo("pool", &SessionManagerMetrics{
		TotalCreated: 10,
	})

	for i := 0; i < 100; i++ {
		sessionID := "pool-sess-" + string(rune('0'+i%5))
		agg.AddOperation(OperationResult{Operation: "tools_call", ToolName: "test", LatencyMs: 10, OK: true, SessionID: sessionID})
	}

	metrics := agg.Compute()

	if metrics.SessionMetrics == nil {
		t.Fatal("expected session metrics")
	}
	if metrics.SessionMetrics.SessionMode != "pool" {
		t.Errorf("expected 'pool' mode, got %s", metrics.SessionMetrics.SessionMode)
	}
	if metrics.SessionMetrics.TotalSessions != 5 {
		t.Errorf("expected 5 sessions, got %d", metrics.SessionMetrics.TotalSessions)
	}
	if metrics.SessionMetrics.OpsPerSession != 20.0 {
		t.Errorf("expected 20.0 ops/session, got %f", metrics.SessionMetrics.OpsPerSession)
	}
}

func TestSessionMetricsChurnMode(t *testing.T) {
	agg := NewAggregator()
	agg.SetSessionInfo("churn", &SessionManagerMetrics{
		TotalCreated: 20,
		TotalEvicted: 15,
	})

	for i := 0; i < 50; i++ {
		sessionID := "churn-sess-" + string(rune('0'+i%10))
		agg.AddOperation(OperationResult{Operation: "ping", LatencyMs: 5, OK: true, SessionID: sessionID})
	}

	metrics := agg.Compute()

	if metrics.SessionMetrics == nil {
		t.Fatal("expected session metrics")
	}
	if metrics.SessionMetrics.SessionMode != "churn" {
		t.Errorf("expected 'churn' mode, got %s", metrics.SessionMetrics.SessionMode)
	}
	if metrics.SessionMetrics.TotalCreated != 20 {
		t.Errorf("expected 20 created, got %d", metrics.SessionMetrics.TotalCreated)
	}
	if metrics.SessionMetrics.TotalEvicted != 15 {
		t.Errorf("expected 15 evicted, got %d", metrics.SessionMetrics.TotalEvicted)
	}
}

func TestSessionMetricsEmptyWithModeSet(t *testing.T) {
	agg := NewAggregator()
	agg.SetSessionInfo("reuse", nil)

	agg.AddOperation(OperationResult{Operation: "ping", LatencyMs: 10, OK: true})

	metrics := agg.Compute()

	if metrics.SessionMetrics == nil {
		t.Fatal("expected session metrics even with no session IDs")
	}
	if metrics.SessionMetrics.SessionMode != "reuse" {
		t.Errorf("expected 'reuse' mode, got %s", metrics.SessionMetrics.SessionMode)
	}
	if metrics.SessionMetrics.TotalSessions != 0 {
		t.Errorf("expected 0 sessions, got %d", metrics.SessionMetrics.TotalSessions)
	}
}

func TestWorkerHealthMetricsNoSamples(t *testing.T) {
	agg := NewAggregator()
	agg.AddOperation(OperationResult{Operation: "ping", LatencyMs: 10, OK: true})

	metrics := agg.Compute()

	if metrics.WorkerHealth != nil {
		t.Error("expected nil worker health when no samples")
	}
}

func TestWorkerHealthMetricsBasic(t *testing.T) {
	agg := NewAggregator()

	agg.AddWorkerHealth(WorkerHealthSample{
		WorkerID:   "worker-1",
		CPUPercent: 50.0,
		MemBytes:   1024 * 1024 * 100, // 100 MB
		ActiveVUs:  5,
	})
	agg.AddWorkerHealth(WorkerHealthSample{
		WorkerID:   "worker-1",
		CPUPercent: 70.0,
		MemBytes:   1024 * 1024 * 200, // 200 MB
		ActiveVUs:  10,
	})
	agg.AddWorkerHealth(WorkerHealthSample{
		WorkerID:   "worker-2",
		CPUPercent: 60.0,
		MemBytes:   1024 * 1024 * 150, // 150 MB
		ActiveVUs:  8,
	})

	metrics := agg.Compute()

	if metrics.WorkerHealth == nil {
		t.Fatal("expected worker health metrics")
	}
	if metrics.WorkerHealth.PeakCPUPercent != 70.0 {
		t.Errorf("expected peak CPU 70.0, got %f", metrics.WorkerHealth.PeakCPUPercent)
	}
	if metrics.WorkerHealth.PeakMemoryMB != 200.0 {
		t.Errorf("expected peak memory 200.0 MB, got %f", metrics.WorkerHealth.PeakMemoryMB)
	}
	expectedAvgVUs := (5.0 + 10.0 + 8.0) / 3.0
	if metrics.WorkerHealth.AvgActiveVUs != expectedAvgVUs {
		t.Errorf("expected avg VUs %f, got %f", expectedAvgVUs, metrics.WorkerHealth.AvgActiveVUs)
	}
	if metrics.WorkerHealth.WorkerCount != 2 {
		t.Errorf("expected 2 workers, got %d", metrics.WorkerHealth.WorkerCount)
	}
	if metrics.WorkerHealth.SaturationDetected {
		t.Error("expected no saturation detected")
	}
}

func TestWorkerHealthMetricsCPUSaturation(t *testing.T) {
	agg := NewAggregator()

	agg.AddWorkerHealth(WorkerHealthSample{
		WorkerID:   "worker-1",
		CPUPercent: 85.0,
		MemBytes:   1024 * 1024 * 100,
		ActiveVUs:  5,
	})

	metrics := agg.Compute()

	if metrics.WorkerHealth == nil {
		t.Fatal("expected worker health metrics")
	}
	if !metrics.WorkerHealth.SaturationDetected {
		t.Error("expected saturation detected for CPU > 80%")
	}
	if metrics.WorkerHealth.SaturationReason != "CPU usage exceeded 80%" {
		t.Errorf("unexpected saturation reason: %s", metrics.WorkerHealth.SaturationReason)
	}
}

func TestWorkerHealthMetricsVUSaturation(t *testing.T) {
	agg := NewAggregator()
	agg.SetMaxVUsConfig(10)

	agg.AddWorkerHealth(WorkerHealthSample{
		WorkerID:   "worker-1",
		CPUPercent: 50.0,
		MemBytes:   1024 * 1024 * 100,
		ActiveVUs:  10,
	})

	metrics := agg.Compute()

	if metrics.WorkerHealth == nil {
		t.Fatal("expected worker health metrics")
	}
	if !metrics.WorkerHealth.SaturationDetected {
		t.Error("expected saturation detected for VU cap reached")
	}
	if metrics.WorkerHealth.SaturationReason != "VU cap reached" {
		t.Errorf("unexpected saturation reason: %s", metrics.WorkerHealth.SaturationReason)
	}
}

func TestWorkerHealthMetricsBothSaturation(t *testing.T) {
	agg := NewAggregator()
	agg.SetMaxVUsConfig(10)

	agg.AddWorkerHealth(WorkerHealthSample{
		WorkerID:   "worker-1",
		CPUPercent: 90.0,
		MemBytes:   1024 * 1024 * 100,
		ActiveVUs:  10,
	})

	metrics := agg.Compute()

	if metrics.WorkerHealth == nil {
		t.Fatal("expected worker health metrics")
	}
	if !metrics.WorkerHealth.SaturationDetected {
		t.Error("expected saturation detected")
	}
	if metrics.WorkerHealth.SaturationReason != "CPU usage exceeded 80%; VU cap reached" {
		t.Errorf("unexpected saturation reason: %s", metrics.WorkerHealth.SaturationReason)
	}
}

func TestWorkerHealthMetricsReset(t *testing.T) {
	agg := NewAggregator()
	agg.SetMaxVUsConfig(10)

	agg.AddWorkerHealth(WorkerHealthSample{
		WorkerID:   "worker-1",
		CPUPercent: 50.0,
		MemBytes:   1024 * 1024 * 100,
		ActiveVUs:  5,
	})

	metrics := agg.Compute()
	if metrics.WorkerHealth == nil {
		t.Fatal("expected worker health metrics before reset")
	}

	agg.Reset()

	metrics = agg.Compute()
	if metrics.WorkerHealth != nil {
		t.Error("expected nil worker health after reset")
	}
}

func TestWorkerHealthMetricsConcurrent(t *testing.T) {
	agg := NewAggregator()
	var wg sync.WaitGroup
	numGoroutines := 10
	samplesPerGoroutine := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < samplesPerGoroutine; j++ {
				agg.AddWorkerHealth(WorkerHealthSample{
					WorkerID:   "worker-" + string(rune('0'+id%5)),
					CPUPercent: float64(id*10 + j%10),
					MemBytes:   int64(id*1024*1024 + j*1024),
					ActiveVUs:  id + j%5,
				})
			}
		}(i)
	}

	wg.Wait()

	metrics := agg.Compute()
	if metrics.WorkerHealth == nil {
		t.Fatal("expected worker health metrics")
	}
	if metrics.WorkerHealth.WorkerCount != 5 {
		t.Errorf("expected 5 unique workers, got %d", metrics.WorkerHealth.WorkerCount)
	}
}
