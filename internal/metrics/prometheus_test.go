package metrics

import (
	"strings"
	"testing"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/analysis"
	"github.com/bc-dunia/mcpdrill/internal/controlplane/runmanager"
	"github.com/bc-dunia/mcpdrill/internal/controlplane/scheduler"
	"github.com/bc-dunia/mcpdrill/internal/types"
)

func TestNewCollector(t *testing.T) {
	c := NewCollector()
	if c == nil {
		t.Fatal("NewCollector returned nil")
	}
	if c.runCounts == nil {
		t.Error("runCounts not initialized")
	}
	if c.workerHealth == nil {
		t.Error("workerHealth not initialized")
	}
}

func TestRecordRunCreated(t *testing.T) {
	c := NewCollector()
	c.RecordRunCreated("scenario-1")
	c.RecordRunCreated("scenario-1")
	c.RecordRunCreated("scenario-2")

	if c.runCounts["scenario-1"] != 2 {
		t.Errorf("expected 2 runs for scenario-1, got %d", c.runCounts["scenario-1"])
	}
	if c.runCounts["scenario-2"] != 1 {
		t.Errorf("expected 1 run for scenario-2, got %d", c.runCounts["scenario-2"])
	}
}

func TestRecordRunDuration(t *testing.T) {
	c := NewCollector()
	c.RecordRunDuration("scenario-1", 10.5)
	c.RecordRunDuration("scenario-1", 20.5)

	data := c.runDurations["scenario-1"]
	if data == nil {
		t.Fatal("runDurations not recorded")
	}
	if data.sum != 31.0 {
		t.Errorf("expected sum 31.0, got %f", data.sum)
	}
	if data.count != 2 {
		t.Errorf("expected count 2, got %d", data.count)
	}
}

func TestRecordOperation(t *testing.T) {
	c := NewCollector()
	c.RecordOperation("tools_call", "echo", 100, true)
	c.RecordOperation("tools_call", "echo", 200, false)
	c.RecordOperation("tools_list", "", 50, true)

	key := opKey{operation: "tools_call", toolName: "echo"}
	if c.operationCounts[key] != 2 {
		t.Errorf("expected 2 operations, got %d", c.operationCounts[key])
	}
	if c.operationErrors[key] != 1 {
		t.Errorf("expected 1 error, got %d", c.operationErrors[key])
	}
	expectedSum := 0.3
	if c.operationDurations[key].sum < expectedSum-0.001 || c.operationDurations[key].sum > expectedSum+0.001 {
		t.Errorf("expected sum ~0.3, got %f", c.operationDurations[key].sum)
	}
}

func TestRecordStageMetrics(t *testing.T) {
	c := NewCollector()
	c.RecordStageMetrics("run_0000000000000001", "stg_0000000000000001", 30.0, 10)

	key := stageKey{runID: "run_0000000000000001", stageID: "stg_0000000000000001"}
	if c.stageDurations[key] != 30.0 {
		t.Errorf("expected duration 30.0, got %f", c.stageDurations[key])
	}
	if c.stageVUs[key] != 10 {
		t.Errorf("expected 10 VUs, got %d", c.stageVUs[key])
	}
}

func TestUpdateWorkerHealth(t *testing.T) {
	c := NewCollector()
	c.UpdateWorkerHealth("worker-1", 45.5, 1024*1024*512, 25)

	data := c.workerHealth["worker-1"]
	if data == nil {
		t.Fatal("worker health not recorded")
	}
	if data.cpuPercent != 45.5 {
		t.Errorf("expected CPU 45.5, got %f", data.cpuPercent)
	}
	if data.memoryMB != 512.0 {
		t.Errorf("expected memory 512.0 MB, got %f", data.memoryMB)
	}
	if data.activeVUs != 25 {
		t.Errorf("expected 25 VUs, got %d", data.activeVUs)
	}
}

func TestRemoveWorker(t *testing.T) {
	c := NewCollector()
	c.UpdateWorkerHealth("worker-1", 45.5, 1024*1024*512, 25)
	c.RemoveWorker("worker-1")

	if _, ok := c.workerHealth["worker-1"]; ok {
		t.Error("worker should have been removed")
	}
}

func TestIngestTelemetryBatch(t *testing.T) {
	c := NewCollector()
	ops := []analysis.OperationResult{
		{Operation: "tools_call", ToolName: "echo", LatencyMs: 100, OK: true},
		{Operation: "tools_call", ToolName: "echo", LatencyMs: 200, OK: false},
		{Operation: "tools_list", ToolName: "", LatencyMs: 50, OK: true},
	}
	c.IngestTelemetryBatch(ops)

	key := opKey{operation: "tools_call", toolName: "echo"}
	if c.operationCounts[key] != 2 {
		t.Errorf("expected 2 operations, got %d", c.operationCounts[key])
	}
	if c.operationErrors[key] != 1 {
		t.Errorf("expected 1 error, got %d", c.operationErrors[key])
	}
}

func TestExposeFormat(t *testing.T) {
	c := NewCollector()
	c.nowFunc = func() time.Time {
		return time.Unix(1706380800, 0)
	}

	c.RecordRunCreated("load-test-1")
	c.RecordRunDuration("load-test-1", 60.0)
	c.RecordOperation("tools_call", "echo", 100, true)
	c.UpdateWorkerHealth("worker-1", 45.5, 1024*1024*512, 25)
	c.RecordStageMetrics("run_0000000000000001", "stg_0000000000000001", 30.0, 10)

	output := c.Expose()

	expectedPatterns := []string{
		"# HELP mcpdrill_runs_total",
		"# TYPE mcpdrill_runs_total counter",
		`mcpdrill_runs_total{scenario_id="load-test-1"} 1`,
		"# HELP mcpdrill_run_duration_seconds",
		"# TYPE mcpdrill_run_duration_seconds histogram",
		`mcpdrill_run_duration_seconds_sum{scenario_id="load-test-1"}`,
		`mcpdrill_run_duration_seconds_count{scenario_id="load-test-1"} 1`,
		"# HELP mcpdrill_workers_total",
		"# TYPE mcpdrill_workers_total gauge",
		"mcpdrill_workers_total 1",
		"# HELP mcpdrill_worker_health_cpu_percent",
		"# TYPE mcpdrill_worker_health_cpu_percent gauge",
		`mcpdrill_worker_health_cpu_percent{worker_id="worker-1"} 45.50`,
		"# HELP mcpdrill_worker_health_memory_mb",
		`mcpdrill_worker_health_memory_mb{worker_id="worker-1"} 512.00`,
		"# HELP mcpdrill_worker_health_active_vus",
		`mcpdrill_worker_health_active_vus{worker_id="worker-1"} 25`,
		"# HELP mcpdrill_operations_total",
		"# TYPE mcpdrill_operations_total counter",
		`mcpdrill_operations_total{operation="tools_call",tool_name="echo"} 1`,
		"# HELP mcpdrill_operation_duration_seconds",
		"# TYPE mcpdrill_operation_duration_seconds histogram",
		"# HELP mcpdrill_operation_errors_total",
		"# TYPE mcpdrill_operation_errors_total counter",
		"# HELP mcpdrill_stage_duration_seconds",
		"# TYPE mcpdrill_stage_duration_seconds gauge",
		`mcpdrill_stage_duration_seconds{run_id="run_0000000000000001",stage_id="stg_0000000000000001"}`,
		"# HELP mcpdrill_stage_vus",
		"# TYPE mcpdrill_stage_vus gauge",
		`mcpdrill_stage_vus{run_id="run_0000000000000001",stage_id="stg_0000000000000001"} 10`,
	}

	for _, pattern := range expectedPatterns {
		if !strings.Contains(output, pattern) {
			t.Errorf("output missing expected pattern: %s", pattern)
		}
	}

	if !strings.Contains(output, "1706380800000") {
		t.Error("output missing timestamp")
	}
}

func TestExposeEmptyCollector(t *testing.T) {
	c := NewCollector()
	c.nowFunc = func() time.Time {
		return time.Unix(1706380800, 0)
	}

	output := c.Expose()

	if !strings.Contains(output, "# HELP mcpdrill_runs_total") {
		t.Error("empty collector should still have HELP lines")
	}
	if !strings.Contains(output, "mcpdrill_workers_total 0") {
		t.Error("empty collector should show 0 workers")
	}
}

func TestReset(t *testing.T) {
	c := NewCollector()
	c.RecordRunCreated("scenario-1")
	c.UpdateWorkerHealth("worker-1", 45.5, 1024*1024*512, 25)
	c.RecordOperation("tools_call", "echo", 100, true)

	c.Reset()

	if len(c.runCounts) != 0 {
		t.Error("runCounts not reset")
	}
	if len(c.workerHealth) != 0 {
		t.Error("workerHealth not reset")
	}
	if len(c.operationCounts) != 0 {
		t.Error("operationCounts not reset")
	}
}

type mockRunProvider struct {
	runs []*runmanager.RunView
}

func (m *mockRunProvider) ListRuns() []*runmanager.RunView {
	return m.runs
}

type mockWorkerProvider struct {
	workers []*scheduler.WorkerInfo
}

func (m *mockWorkerProvider) ListWorkers() []*scheduler.WorkerInfo {
	return m.workers
}

type mockTelemetryProvider struct {
	data map[string]*runmanager.TelemetryData
}

func (m *mockTelemetryProvider) GetTelemetryData(runID string) (*runmanager.TelemetryData, error) {
	if data, ok := m.data[runID]; ok {
		return data, nil
	}
	return nil, nil
}

func TestSyncFromProviders(t *testing.T) {
	c := NewCollector()

	runProvider := &mockRunProvider{
		runs: []*runmanager.RunView{
			{RunID: "run_0000000000000001", ScenarioID: "scenario-1", State: runmanager.RunStatePreflightRunning},
			{RunID: "run_0000000000000002", ScenarioID: "scenario-1", State: runmanager.RunStateCompleted},
			{RunID: "run_0000000000000003", ScenarioID: "scenario-2", State: runmanager.RunStatePreflightRunning},
		},
	}

	workerProvider := &mockWorkerProvider{
		workers: []*scheduler.WorkerInfo{
			{
				WorkerID: "worker-1",
				Health: &types.WorkerHealth{
					CPUPercent: 50.0,
					MemBytes:   1024 * 1024 * 256,
					ActiveVUs:  10,
				},
			},
			{
				WorkerID: "worker-2",
				Health:   nil,
			},
		},
	}

	c.SetRunProvider(runProvider)
	c.SetWorkerProvider(workerProvider)

	c.SyncFromProviders()

	key1 := runStateKey{scenarioID: "scenario-1", state: "preflight_running"}
	if c.runStates[key1] != 1 {
		t.Errorf("expected 1 run in preflight_running for scenario-1, got %d", c.runStates[key1])
	}

	key2 := runStateKey{scenarioID: "scenario-1", state: "completed"}
	if c.runStates[key2] != 1 {
		t.Errorf("expected 1 run in completed for scenario-1, got %d", c.runStates[key2])
	}

	if len(c.workerHealth) != 2 {
		t.Errorf("expected 2 workers, got %d", len(c.workerHealth))
	}

	if c.workerHealth["worker-1"].cpuPercent != 50.0 {
		t.Errorf("expected CPU 50.0, got %f", c.workerHealth["worker-1"].cpuPercent)
	}
}

func TestConcurrentAccess(t *testing.T) {
	c := NewCollector()
	done := make(chan bool)

	go func() {
		for i := 0; i < 100; i++ {
			c.RecordRunCreated("scenario-1")
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			c.RecordOperation("tools_call", "echo", 100, true)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			c.UpdateWorkerHealth("worker-1", 50.0, 1024*1024*256, 10)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			_ = c.Expose()
		}
		done <- true
	}()

	for i := 0; i < 4; i++ {
		<-done
	}

	if c.runCounts["scenario-1"] != 100 {
		t.Errorf("expected 100 runs, got %d", c.runCounts["scenario-1"])
	}
}

func TestDeterministicOutput(t *testing.T) {
	c := NewCollector()
	c.nowFunc = func() time.Time {
		return time.Unix(1706380800, 0)
	}

	c.RecordRunCreated("z-scenario")
	c.RecordRunCreated("a-scenario")
	c.RecordRunCreated("m-scenario")
	c.UpdateWorkerHealth("z-worker", 10.0, 1024, 1)
	c.UpdateWorkerHealth("a-worker", 20.0, 2048, 2)

	output1 := c.Expose()
	output2 := c.Expose()

	if output1 != output2 {
		t.Error("output should be deterministic")
	}

	lines := strings.Split(output1, "\n")
	var scenarioLines []string
	for _, line := range lines {
		if strings.Contains(line, "mcpdrill_runs_total{scenario_id=") {
			scenarioLines = append(scenarioLines, line)
		}
	}

	if len(scenarioLines) != 3 {
		t.Errorf("expected 3 scenario lines, got %d", len(scenarioLines))
	}

	if !strings.Contains(scenarioLines[0], "a-scenario") {
		t.Error("scenarios should be sorted alphabetically")
	}
}

func TestTimestampInOutput(t *testing.T) {
	c := NewCollector()
	fixedTime := time.Unix(1706380800, 0)
	c.nowFunc = func() time.Time {
		return fixedTime
	}

	c.RecordRunCreated("test-scenario")
	output := c.Expose()

	expectedTimestamp := "1706380800000"
	if !strings.Contains(output, expectedTimestamp) {
		t.Errorf("output should contain timestamp %s", expectedTimestamp)
	}
}

func TestRunStateGauge(t *testing.T) {
	c := NewCollector()
	c.nowFunc = func() time.Time {
		return time.Unix(1706380800, 0)
	}

	runProvider := &mockRunProvider{
		runs: []*runmanager.RunView{
			{RunID: "run_0000000000000001", ScenarioID: "test", State: runmanager.RunStatePreflightRunning},
			{RunID: "run_0000000000000002", ScenarioID: "test", State: runmanager.RunStatePreflightRunning},
			{RunID: "run_0000000000000003", ScenarioID: "test", State: runmanager.RunStateCompleted},
		},
	}
	c.SetRunProvider(runProvider)
	c.SyncFromProviders()

	output := c.Expose()

	if !strings.Contains(output, `mcpdrill_run_state{scenario_id="test",state="preflight_running"} 2`) {
		t.Error("should show 2 runs in preflight_running state")
	}
	if !strings.Contains(output, `mcpdrill_run_state{scenario_id="test",state="completed"} 1`) {
		t.Error("should show 1 run in completed state")
	}
}
