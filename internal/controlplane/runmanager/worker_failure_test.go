package runmanager

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/bc-dunia/mcpdrill/internal/controlplane/scheduler"
	"github.com/bc-dunia/mcpdrill/internal/types"
	"github.com/bc-dunia/mcpdrill/internal/validation"
)

func getProjectRootForWorkerTest() string {
	_, filename, _, _ := runtime.Caller(0)
	dir := filepath.Dir(filename)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func createTestValidatorForWorkerTest(t *testing.T) *validation.UnifiedValidator {
	t.Helper()
	validator, err := validation.NewUnifiedValidator(nil)
	if err != nil {
		t.Fatalf("failed to create validator: %v", err)
	}
	return validator
}

func createTestRunWithPolicy(t *testing.T, rm *RunManager, policy string) string {
	t.Helper()

	root := getProjectRootForWorkerTest()
	data, err := os.ReadFile(filepath.Join(root, "testdata/fixtures/valid/minimal_preflight_baseline_ramp.json"))
	if err != nil {
		t.Fatalf("failed to read test fixture: %v", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("failed to unmarshal test fixture: %v", err)
	}

	if policy != "" {
		safety, ok := config["safety"].(map[string]interface{})
		if !ok {
			safety = make(map[string]interface{})
			config["safety"] = safety
		}
		safety["worker_failure_policy"] = policy
	}

	configBytes, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}

	runID, err := rm.CreateRun(configBytes, "test")
	if err != nil {
		t.Fatalf("failed to create run: %v", err)
	}

	return runID
}

func setRunState(t *testing.T, rm *RunManager, runID string, state RunState) {
	t.Helper()
	rm.mu.Lock()
	defer rm.mu.Unlock()
	record, ok := rm.runs[runID]
	if !ok {
		t.Fatalf("run not found: %s", runID)
	}
	record.State = state
}

func TestHandleWorkerCapacityLost_FailFast(t *testing.T) {
	rm := NewRunManager(createTestValidatorForWorkerTest(t))
	runID := createTestRunWithPolicy(t, rm, "fail_fast")
	setRunState(t, rm, runID, RunStateRampRunning)

	err := rm.HandleWorkerCapacityLost(runID, "worker-1")
	if err != nil {
		t.Fatalf("HandleWorkerCapacityLost failed: %v", err)
	}

	view, err := rm.GetRun(runID)
	if err != nil {
		t.Fatalf("GetRun failed: %v", err)
	}

	if view.State != RunStateStopping {
		t.Errorf("expected state %s, got %s", RunStateStopping, view.State)
	}

	if view.StopReason == nil {
		t.Fatal("expected StopReason to be set")
	}

	if view.StopReason.Mode != StopModeImmediate {
		t.Errorf("expected stop mode %s, got %s", StopModeImmediate, view.StopReason.Mode)
	}

	events, err := rm.TailEvents(runID, 0, 100)
	if err != nil {
		t.Fatalf("TailEvents failed: %v", err)
	}

	hasStopRequested := false
	hasStateTransition := false
	for _, e := range events {
		if e.Type == EventTypeStopRequested {
			hasStopRequested = true
			var payload map[string]interface{}
			if err := json.Unmarshal(e.Payload, &payload); err == nil {
				if payload["policy"] != string(PolicyFailFast) {
					t.Errorf("expected policy %s in event, got %v", PolicyFailFast, payload["policy"])
				}
				if payload["worker_id"] != "worker-1" {
					t.Errorf("expected worker_id worker-1 in event, got %v", payload["worker_id"])
				}
			}
		}
		if e.Type == EventTypeStateTransition {
			var payload map[string]interface{}
			if err := json.Unmarshal(e.Payload, &payload); err == nil {
				if payload["to_state"] == string(RunStateStopping) {
					hasStateTransition = true
				}
			}
		}
	}

	if !hasStopRequested {
		t.Error("expected STOP_REQUESTED event")
	}
	if !hasStateTransition {
		t.Error("expected STATE_TRANSITION event to STOPPING")
	}
}

func TestHandleWorkerCapacityLost_ReplaceIfPossible_NoActiveStage(t *testing.T) {
	rm := NewRunManager(createTestValidatorForWorkerTest(t))
	runID := createTestRunWithPolicy(t, rm, "replace_if_possible")
	setRunState(t, rm, runID, RunStateBaselineRunning)

	err := rm.HandleWorkerCapacityLost(runID, "worker-2")
	if err != nil {
		t.Fatalf("HandleWorkerCapacityLost failed: %v", err)
	}

	view, err := rm.GetRun(runID)
	if err != nil {
		t.Fatalf("GetRun failed: %v", err)
	}

	if view.State != RunStateStopping {
		t.Errorf("expected state %s (fallback to fail_fast), got %s", RunStateStopping, view.State)
	}

	events, err := rm.TailEvents(runID, 0, 100)
	if err != nil {
		t.Fatalf("TailEvents failed: %v", err)
	}

	hasDecision := false
	for _, e := range events {
		if e.Type == EventTypeDecision {
			var payload map[string]interface{}
			if err := json.Unmarshal(e.Payload, &payload); err == nil {
				if payload["decision_type"] == "reallocation_failed" {
					hasDecision = true
					if payload["policy"] != string(PolicyReplaceIfPossible) {
						t.Errorf("expected policy %s, got %v", PolicyReplaceIfPossible, payload["policy"])
					}
					if payload["fallback"] != string(PolicyFailFast) {
						t.Errorf("expected fallback %s, got %v", PolicyFailFast, payload["fallback"])
					}
					if payload["reason"] != "no_active_stage" {
						t.Errorf("expected reason no_active_stage, got %v", payload["reason"])
					}
				}
			}
		}
	}

	if !hasDecision {
		t.Error("expected DECISION event documenting reallocation failure")
	}
}

func TestHandleWorkerCapacityLost_BestEffort(t *testing.T) {
	rm := NewRunManager(createTestValidatorForWorkerTest(t))
	runID := createTestRunWithPolicy(t, rm, "best_effort")
	setRunState(t, rm, runID, RunStateRampRunning)

	err := rm.HandleWorkerCapacityLost(runID, "worker-3")
	if err != nil {
		t.Fatalf("HandleWorkerCapacityLost failed: %v", err)
	}

	view, err := rm.GetRun(runID)
	if err != nil {
		t.Fatalf("GetRun failed: %v", err)
	}

	if view.State != RunStateRampRunning {
		t.Errorf("expected state %s (continue running), got %s", RunStateRampRunning, view.State)
	}

	if view.StopReason != nil {
		t.Error("expected StopReason to be nil for best_effort")
	}

	events, err := rm.TailEvents(runID, 0, 100)
	if err != nil {
		t.Fatalf("TailEvents failed: %v", err)
	}

	hasCapacityLost := false
	hasWarning := false
	for _, e := range events {
		if e.Type == EventTypeWorkerCapacityLost {
			hasCapacityLost = true
			var payload map[string]interface{}
			if err := json.Unmarshal(e.Payload, &payload); err == nil {
				if payload["policy"] != string(PolicyBestEffort) {
					t.Errorf("expected policy %s, got %v", PolicyBestEffort, payload["policy"])
				}
				if payload["action"] != "continue" {
					t.Errorf("expected action continue, got %v", payload["action"])
				}
			}
		}
		if e.Type == EventTypeSystemWarning {
			hasWarning = true
		}
	}

	if !hasCapacityLost {
		t.Error("expected WORKER_CAPACITY_LOST event")
	}
	if !hasWarning {
		t.Error("expected SYSTEM_WARNING event")
	}
}

func TestHandleWorkerCapacityLost_RunNotFound(t *testing.T) {
	rm := NewRunManager(createTestValidatorForWorkerTest(t))

	err := rm.HandleWorkerCapacityLost("nonexistent-run", "worker-1")
	if err == nil {
		t.Fatal("expected error for nonexistent run")
	}

	expectedMsg := "run not found: nonexistent-run"
	if err.Error() != expectedMsg {
		t.Errorf("expected error message %q, got %q", expectedMsg, err.Error())
	}
}

func TestHandleWorkerCapacityLost_RunAlreadyStopping(t *testing.T) {
	rm := NewRunManager(createTestValidatorForWorkerTest(t))
	runID := createTestRunWithPolicy(t, rm, "fail_fast")
	setRunState(t, rm, runID, RunStateStopping)

	initialEventCount := rm.GetEventCount(runID)

	err := rm.HandleWorkerCapacityLost(runID, "worker-1")
	if err != nil {
		t.Fatalf("HandleWorkerCapacityLost failed: %v", err)
	}

	view, err := rm.GetRun(runID)
	if err != nil {
		t.Fatalf("GetRun failed: %v", err)
	}

	if view.State != RunStateStopping {
		t.Errorf("expected state to remain %s, got %s", RunStateStopping, view.State)
	}

	finalEventCount := rm.GetEventCount(runID)
	if finalEventCount != initialEventCount {
		t.Errorf("expected no new events, got %d new events", finalEventCount-initialEventCount)
	}
}

func TestHandleWorkerCapacityLost_RunCompleted(t *testing.T) {
	rm := NewRunManager(createTestValidatorForWorkerTest(t))
	runID := createTestRunWithPolicy(t, rm, "fail_fast")
	setRunState(t, rm, runID, RunStateCompleted)

	initialEventCount := rm.GetEventCount(runID)

	err := rm.HandleWorkerCapacityLost(runID, "worker-1")
	if err != nil {
		t.Fatalf("HandleWorkerCapacityLost failed: %v", err)
	}

	view, err := rm.GetRun(runID)
	if err != nil {
		t.Fatalf("GetRun failed: %v", err)
	}

	if view.State != RunStateCompleted {
		t.Errorf("expected state to remain %s, got %s", RunStateCompleted, view.State)
	}

	finalEventCount := rm.GetEventCount(runID)
	if finalEventCount != initialEventCount {
		t.Errorf("expected no new events, got %d new events", finalEventCount-initialEventCount)
	}
}

func TestHandleWorkerCapacityLost_DefaultPolicy(t *testing.T) {
	rm := NewRunManager(createTestValidatorForWorkerTest(t))
	runID := createTestRunWithPolicy(t, rm, "")
	setRunState(t, rm, runID, RunStatePreflightRunning)

	err := rm.HandleWorkerCapacityLost(runID, "worker-1")
	if err != nil {
		t.Fatalf("HandleWorkerCapacityLost failed: %v", err)
	}

	view, err := rm.GetRun(runID)
	if err != nil {
		t.Fatalf("GetRun failed: %v", err)
	}

	if view.State != RunStateStopping {
		t.Errorf("expected default policy (fail_fast) to stop run, got state %s", view.State)
	}
}

func TestHandleWorkerCapacityLost_UnknownPolicyFallback(t *testing.T) {
	rm := NewRunManager(createTestValidatorForWorkerTest(t))
	runID := createTestRunWithPolicy(t, rm, "fail_fast")
	setRunState(t, rm, runID, RunStateRampRunning)

	rm.mu.Lock()
	record := rm.runs[runID]
	var config map[string]interface{}
	json.Unmarshal(record.Config, &config)
	safety := config["safety"].(map[string]interface{})
	safety["worker_failure_policy"] = "unknown_policy"
	record.Config, _ = json.Marshal(config)
	rm.mu.Unlock()

	err := rm.HandleWorkerCapacityLost(runID, "worker-1")
	if err != nil {
		t.Fatalf("HandleWorkerCapacityLost failed: %v", err)
	}

	view, err := rm.GetRun(runID)
	if err != nil {
		t.Fatalf("GetRun failed: %v", err)
	}

	if view.State != RunStateStopping {
		t.Errorf("expected unknown policy to fall back to fail_fast, got state %s", view.State)
	}
}

func TestHandleReplaceIfPossible_Success(t *testing.T) {
	rm := NewRunManager(createTestValidatorForWorkerTest(t))

	registry := scheduler.NewRegistry()
	lm := scheduler.NewLeaseManager(60000)
	allocator := scheduler.NewAllocator(registry, lm)
	rm.SetScheduler(registry, allocator, lm)

	mockSender := &mockAssignmentSender{assignments: make(map[string][]types.WorkerAssignment)}
	rm.SetAssignmentSender(mockSender)

	wid1, _ := registry.RegisterWorker(types.HostInfo{Hostname: "host1"}, types.WorkerCapacity{MaxVUs: 50})
	wid2, _ := registry.RegisterWorker(types.HostInfo{Hostname: "host2"}, types.WorkerCapacity{MaxVUs: 50})
	wid3, _ := registry.RegisterWorker(types.HostInfo{Hostname: "host3"}, types.WorkerCapacity{MaxVUs: 50})

	runID := createTestRunWithPolicy(t, rm, "replace_if_possible")
	setRunState(t, rm, runID, RunStateBaselineRunning)
	setActiveStage(t, rm, runID, "baseline", "stg_000000000002")

	err := rm.HandleWorkerCapacityLost(runID, string(wid1))
	if err != nil {
		t.Fatalf("HandleWorkerCapacityLost failed: %v", err)
	}

	view, err := rm.GetRun(runID)
	if err != nil {
		t.Fatalf("GetRun failed: %v", err)
	}

	if view.State != RunStateBaselineRunning {
		t.Errorf("expected state %s (run should continue), got %s", RunStateBaselineRunning, view.State)
	}

	events, err := rm.TailEvents(runID, 0, 100)
	if err != nil {
		t.Fatalf("TailEvents failed: %v", err)
	}

	hasWorkerReplaced := false
	hasDecisionSuccess := false
	for _, e := range events {
		if e.Type == EventTypeWorkerReplaced {
			hasWorkerReplaced = true
			var payload map[string]interface{}
			if err := json.Unmarshal(e.Payload, &payload); err == nil {
				if payload["lost_worker"] != string(wid1) {
					t.Errorf("expected lost_worker %s, got %v", wid1, payload["lost_worker"])
				}
			}
		}
		if e.Type == EventTypeDecision {
			var payload map[string]interface{}
			if err := json.Unmarshal(e.Payload, &payload); err == nil {
				if payload["decision_type"] == "reallocation_success" {
					hasDecisionSuccess = true
				}
			}
		}
	}

	if !hasWorkerReplaced {
		t.Error("expected WORKER_REPLACED event")
	}
	if !hasDecisionSuccess {
		t.Error("expected DECISION event with reallocation_success")
	}

	totalAssignments := 0
	for _, assignments := range mockSender.assignments {
		totalAssignments += len(assignments)
	}
	if totalAssignments == 0 {
		t.Error("expected assignments to be issued to remaining workers")
	}

	_ = wid2
	_ = wid3
}

func TestHandleReplaceIfPossible_InsufficientCapacity(t *testing.T) {
	rm := NewRunManager(createTestValidatorForWorkerTest(t))

	registry := scheduler.NewRegistry()
	lm := scheduler.NewLeaseManager(60000)
	allocator := scheduler.NewAllocator(registry, lm)
	rm.SetScheduler(registry, allocator, lm)

	mockSender := &mockAssignmentSender{assignments: make(map[string][]types.WorkerAssignment)}
	rm.SetAssignmentSender(mockSender)

	wid1, _ := registry.RegisterWorker(types.HostInfo{Hostname: "host1"}, types.WorkerCapacity{MaxVUs: 3})
	wid2, _ := registry.RegisterWorker(types.HostInfo{Hostname: "host2"}, types.WorkerCapacity{MaxVUs: 3})

	runID := createTestRunWithPolicy(t, rm, "replace_if_possible")
	setRunState(t, rm, runID, RunStateBaselineRunning)
	setActiveStage(t, rm, runID, "baseline", "stg_000000000002")

	err := rm.HandleWorkerCapacityLost(runID, string(wid1))
	if err != nil {
		t.Fatalf("HandleWorkerCapacityLost failed: %v", err)
	}

	view, err := rm.GetRun(runID)
	if err != nil {
		t.Fatalf("GetRun failed: %v", err)
	}

	if view.State != RunStateStopping {
		t.Errorf("expected state %s (fallback to fail_fast), got %s", RunStateStopping, view.State)
	}

	events, err := rm.TailEvents(runID, 0, 100)
	if err != nil {
		t.Fatalf("TailEvents failed: %v", err)
	}

	hasReallocationFailed := false
	for _, e := range events {
		if e.Type == EventTypeDecision {
			var payload map[string]interface{}
			if err := json.Unmarshal(e.Payload, &payload); err == nil {
				if payload["decision_type"] == "reallocation_failed" {
					hasReallocationFailed = true
					if payload["reason"] != "insufficient total capacity for target VUs" {
						t.Errorf("expected reason 'insufficient total capacity for target VUs', got %v", payload["reason"])
					}
				}
			}
		}
	}

	if !hasReallocationFailed {
		t.Error("expected DECISION event with reallocation_failed")
	}

	_ = wid2
}

func TestHandleReplaceIfPossible_NoWorkersRemaining(t *testing.T) {
	rm := NewRunManager(createTestValidatorForWorkerTest(t))

	registry := scheduler.NewRegistry()
	lm := scheduler.NewLeaseManager(60000)
	allocator := scheduler.NewAllocator(registry, lm)
	rm.SetScheduler(registry, allocator, lm)

	mockSender := &mockAssignmentSender{assignments: make(map[string][]types.WorkerAssignment)}
	rm.SetAssignmentSender(mockSender)

	wid1, _ := registry.RegisterWorker(types.HostInfo{Hostname: "host1"}, types.WorkerCapacity{MaxVUs: 50})

	runID := createTestRunWithPolicy(t, rm, "replace_if_possible")
	setRunState(t, rm, runID, RunStateBaselineRunning)
	setActiveStage(t, rm, runID, "baseline", "stg_000000000002")

	err := rm.HandleWorkerCapacityLost(runID, string(wid1))
	if err != nil {
		t.Fatalf("HandleWorkerCapacityLost failed: %v", err)
	}

	view, err := rm.GetRun(runID)
	if err != nil {
		t.Fatalf("GetRun failed: %v", err)
	}

	if view.State != RunStateStopping {
		t.Errorf("expected state %s (fallback to fail_fast), got %s", RunStateStopping, view.State)
	}

	events, err := rm.TailEvents(runID, 0, 100)
	if err != nil {
		t.Fatalf("TailEvents failed: %v", err)
	}

	hasReallocationFailed := false
	for _, e := range events {
		if e.Type == EventTypeDecision {
			var payload map[string]interface{}
			if err := json.Unmarshal(e.Payload, &payload); err == nil {
				if payload["decision_type"] == "reallocation_failed" {
					hasReallocationFailed = true
					if payload["reason"] != "no workers available" {
						t.Errorf("expected reason 'no workers available', got %v", payload["reason"])
					}
				}
			}
		}
	}

	if !hasReallocationFailed {
		t.Error("expected DECISION event with reallocation_failed")
	}
}

type mockAssignmentSender struct {
	assignments map[string][]types.WorkerAssignment
}

func (m *mockAssignmentSender) AddAssignment(workerID string, assignment types.WorkerAssignment) {
	m.assignments[workerID] = append(m.assignments[workerID], assignment)
}

func setActiveStage(t *testing.T, rm *RunManager, runID, stage, stageID string) {
	t.Helper()
	rm.mu.Lock()
	defer rm.mu.Unlock()
	record, ok := rm.runs[runID]
	if !ok {
		t.Fatalf("run not found: %s", runID)
	}
	record.ActiveStage = &ActiveStageInfo{Stage: stage, StageID: stageID}
}

func TestIsValidWorkerFailurePolicy(t *testing.T) {
	tests := []struct {
		policy string
		valid  bool
	}{
		{"fail_fast", true},
		{"replace_if_possible", true},
		{"best_effort", true},
		{"", false},
		{"invalid", false},
		{"FAIL_FAST", false},
	}

	for _, tt := range tests {
		t.Run(tt.policy, func(t *testing.T) {
			result := IsValidWorkerFailurePolicy(tt.policy)
			if result != tt.valid {
				t.Errorf("IsValidWorkerFailurePolicy(%q) = %v, want %v", tt.policy, result, tt.valid)
			}
		})
	}
}

func TestGetRunsForWorker(t *testing.T) {
	rm := NewRunManager(createTestValidatorForWorkerTest(t))

	// Set up scheduler components for lease-backed mapping
	registry := scheduler.NewRegistry()
	leaseManager := scheduler.NewLeaseManager(60000)
	allocator := scheduler.NewAllocator(registry, leaseManager)
	rm.SetScheduler(registry, allocator, leaseManager)

	run1 := createTestRunWithPolicy(t, rm, "fail_fast")
	setRunState(t, rm, run1, RunStateRampRunning)

	run2 := createTestRunWithPolicy(t, rm, "best_effort")
	setRunState(t, rm, run2, RunStateBaselineRunning)

	run3 := createTestRunWithPolicy(t, rm, "fail_fast")
	setRunState(t, rm, run3, RunStateCompleted)

	// Create leases for worker-1 on run1 and run2
	workerID := scheduler.WorkerID("worker-1")
	_, _ = leaseManager.IssueLease(workerID, scheduler.Assignment{
		RunID:     run1,
		StageID:   "stage-1",
		VUIDRange: scheduler.VUIDRange{Start: 0, End: 10},
	})
	_, _ = leaseManager.IssueLease(workerID, scheduler.Assignment{
		RunID:     run2,
		StageID:   "stage-1",
		VUIDRange: scheduler.VUIDRange{Start: 0, End: 10},
	})
	// run3 has no lease for worker-1

	runIDs := rm.GetRunsForWorker("worker-1")

	if len(runIDs) != 2 {
		t.Errorf("expected 2 running runs, got %d", len(runIDs))
	}

	runIDSet := make(map[string]bool)
	for _, id := range runIDs {
		runIDSet[id] = true
	}

	if !runIDSet[run1] {
		t.Errorf("expected run1 (%s) to be in running runs", run1)
	}
	if !runIDSet[run2] {
		t.Errorf("expected run2 (%s) to be in running runs", run2)
	}
	if runIDSet[run3] {
		t.Errorf("expected run3 (%s) to NOT be in running runs (completed)", run3)
	}
}

func TestIsRunningState(t *testing.T) {
	tests := []struct {
		state   RunState
		running bool
	}{
		{RunStateCreated, false},
		{RunStatePreflightRunning, true},
		{RunStatePreflightPassed, false},
		{RunStatePreflightFailed, false},
		{RunStateBaselineRunning, true},
		{RunStateRampRunning, true},
		{RunStateSoakRunning, true},
		{RunStateStopping, false},
		{RunStateAnalyzing, false},
		{RunStateCompleted, false},
		{RunStateFailed, false},
		{RunStateAborted, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			result := isRunningState(tt.state)
			if result != tt.running {
				t.Errorf("isRunningState(%s) = %v, want %v", tt.state, result, tt.running)
			}
		})
	}
}
