package runmanager

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/analysis"
	"github.com/bc-dunia/mcpdrill/internal/artifacts"
	"github.com/bc-dunia/mcpdrill/internal/validation"
)

func createTestValidator(t *testing.T) *validation.UnifiedValidator {
	t.Helper()
	validator, err := validation.NewUnifiedValidator(nil)
	if err != nil {
		t.Fatalf("failed to create validator: %v", err)
	}
	return validator
}

func getProjectRoot() string {
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

func createValidConfig() []byte {
	root := getProjectRoot()
	data, err := os.ReadFile(filepath.Join(root, "testdata/fixtures/valid/minimal_preflight_baseline_ramp.json"))
	if err != nil {
		panic("failed to read test fixture: " + err.Error())
	}
	return data
}

func createValidConfigWithDurations(t *testing.T, preflightMs, baselineMs, rampMs int64) []byte {
	t.Helper()

	config := createValidConfig()
	var parsed map[string]interface{}
	if err := json.Unmarshal(config, &parsed); err != nil {
		t.Fatalf("failed to parse config fixture: %v", err)
	}

	stages, ok := parsed["stages"].([]interface{})
	if !ok {
		t.Fatalf("expected stages array in config")
	}

	for _, stage := range stages {
		stageMap, ok := stage.(map[string]interface{})
		if !ok {
			continue
		}
		stageName, _ := stageMap["stage"].(string)
		switch stageName {
		case "preflight":
			stageMap["duration_ms"] = preflightMs
		case "baseline":
			stageMap["duration_ms"] = baselineMs
		case "ramp":
			stageMap["duration_ms"] = rampMs
		}
	}

	safety, ok := parsed["safety"].(map[string]interface{})
	if ok {
		if stopPolicy, ok := safety["stop_policy"].(map[string]interface{}); ok {
			stopPolicy["drain_timeout_ms"] = 100
		} else {
			safety["stop_policy"] = map[string]interface{}{
				"mode":             "drain",
				"drain_timeout_ms": 100,
			}
		}
	}

	updated, err := json.Marshal(parsed)
	if err != nil {
		t.Fatalf("failed to marshal updated config: %v", err)
	}
	return updated
}

func waitForRunState(t *testing.T, rm *RunManager, runID string, expected RunState, timeout time.Duration) {
	t.Helper()

	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.After(timeout)

	for {
		select {
		case <-deadline:
			view, _ := rm.GetRun(runID)
			t.Fatalf("expected state %s, got %s", expected, view.State)
		case <-ticker.C:
			view, err := rm.GetRun(runID)
			if err == nil && view.State == expected {
				return
			}
		}
	}
}

func TestNewRunManager(t *testing.T) {
	validator := createTestValidator(t)
	rm := NewRunManager(validator)

	if rm == nil {
		t.Fatal("expected non-nil RunManager")
	}
	if rm.runs == nil {
		t.Error("expected runs map to be initialized")
	}
	if rm.eventLogs == nil {
		t.Error("expected eventLogs map to be initialized")
	}
	if rm.validator != validator {
		t.Error("expected validator to be set")
	}
}

func TestNewRunManagerNilValidator(t *testing.T) {
	rm := NewRunManager(nil)

	if rm == nil {
		t.Fatal("expected non-nil RunManager")
	}
	if rm.validator != nil {
		t.Error("expected validator to be nil")
	}
}

func TestValidateRunConfig(t *testing.T) {
	validator := createTestValidator(t)
	rm := NewRunManager(validator)

	t.Run("valid config", func(t *testing.T) {
		config := createValidConfig()
		report := rm.ValidateRunConfig(config)
		if !report.OK {
			t.Errorf("expected valid config, got errors: %v", report.Errors)
		}
	})

	t.Run("invalid config", func(t *testing.T) {
		config := []byte(`{"invalid": "config"}`)
		report := rm.ValidateRunConfig(config)
		if report.OK {
			t.Error("expected validation to fail for invalid config")
		}
	})

	t.Run("nil validator", func(t *testing.T) {
		rmNoValidator := NewRunManager(nil)
		report := rmNoValidator.ValidateRunConfig([]byte(`{}`))
		if report.OK {
			t.Error("expected validation to fail when no validator configured")
		}
		if len(report.Errors) == 0 || report.Errors[0].Code != "VALIDATOR_NOT_CONFIGURED" {
			t.Error("expected VALIDATOR_NOT_CONFIGURED error")
		}
	})
}

func TestCreateRun(t *testing.T) {
	validator := createTestValidator(t)
	rm := NewRunManager(validator)

	t.Run("success", func(t *testing.T) {
		config := createValidConfig()
		runID, err := rm.CreateRun(config, "test-user")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if runID == "" {
			t.Error("expected non-empty run ID")
		}
		if !strings.HasPrefix(runID, "run_") {
			t.Errorf("expected run ID to start with 'run_', got %s", runID)
		}

		view, err := rm.GetRun(runID)
		if err != nil {
			t.Fatalf("failed to get run: %v", err)
		}
		if view.State != RunStateCreated {
			t.Errorf("expected state %s, got %s", RunStateCreated, view.State)
		}
		if view.ScenarioID != "scn_minimal_test" {
			t.Errorf("expected scenario_id 'scn_minimal_test', got %s", view.ScenarioID)
		}
		if view.ConfigHash == "" {
			t.Error("expected non-empty config hash")
		}
		if !strings.HasPrefix(view.ExecutionID, "exe_") {
			t.Errorf("expected execution ID to start with 'exe_', got %s", view.ExecutionID)
		}

		eventCount := rm.GetEventCount(runID)
		if eventCount != 1 {
			t.Errorf("expected 1 event (RUN_CREATED), got %d", eventCount)
		}
	})

	t.Run("validation failure", func(t *testing.T) {
		config := []byte(`{"invalid": "config"}`)
		runID, err := rm.CreateRun(config, "test-user")

		if err == nil {
			t.Error("expected validation error")
		}
		if runID != "" {
			t.Error("expected empty run ID on error")
		}

		validationErr, ok := err.(*validation.ValidationError)
		if !ok {
			t.Errorf("expected ValidationError, got %T", err)
		}
		if validationErr.Report.OK {
			t.Error("expected validation report to have errors")
		}
	})

	t.Run("unique IDs", func(t *testing.T) {
		config := createValidConfig()
		runID1, _ := rm.CreateRun(config, "user1")
		runID2, _ := rm.CreateRun(config, "user2")

		if runID1 == runID2 {
			t.Error("expected unique run IDs")
		}

		view1, _ := rm.GetRun(runID1)
		view2, _ := rm.GetRun(runID2)
		if view1.ExecutionID == view2.ExecutionID {
			t.Error("expected unique execution IDs")
		}
	})
}

func TestStartRun(t *testing.T) {
	validator := createTestValidator(t)
	rm := NewRunManager(validator)
	config := createValidConfig()

	t.Run("success", func(t *testing.T) {
		runID, _ := rm.CreateRun(config, "test-user")
		err := rm.StartRun(runID, "test-user")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		view, _ := rm.GetRun(runID)
		if view.State != RunStatePreflightRunning {
			t.Errorf("expected state %s, got %s", RunStatePreflightRunning, view.State)
		}

		eventCount := rm.GetEventCount(runID)
		if eventCount != 2 {
			t.Errorf("expected 2 events (RUN_CREATED + STATE_TRANSITION), got %d", eventCount)
		}
	})

	t.Run("run not found", func(t *testing.T) {
		err := rm.StartRun("nonexistent", "test-user")
		if err == nil {
			t.Error("expected error for nonexistent run")
		}
		if !strings.Contains(err.Error(), "run not found") {
			t.Errorf("expected 'run not found' error, got: %v", err)
		}
	})

	t.Run("idempotency - already started", func(t *testing.T) {
		runID, _ := rm.CreateRun(config, "test-user")
		_ = rm.StartRun(runID, "test-user")

		err := rm.StartRun(runID, "test-user")
		if err == nil {
			t.Error("expected error when starting already-started run")
		}
		if !strings.Contains(err.Error(), "cannot start run in state") {
			t.Errorf("expected state error, got: %v", err)
		}
	})
}

func TestAutomaticStageProgression(t *testing.T) {
	validator := createTestValidator(t)
	rm := NewRunManager(validator)
	config := createValidConfigWithDurations(t, 1000, 1000, 1000)

	artifactStore, _ := artifacts.NewFilesystemStore(t.TempDir())
	rm.SetArtifactStore(artifactStore)

	telemetryStore := &mockTelemetryStore{
		data: make(map[string]*TelemetryData),
	}
	rm.SetTelemetryStore(telemetryStore)

	runID, err := rm.CreateRun(config, "test-user")
	if err != nil {
		t.Fatalf("unexpected error creating run: %v", err)
	}
	telemetryStore.data[runID] = &TelemetryData{
		RunID:       runID,
		ScenarioID:  "scn_minimal_test",
		StartTimeMs: 1000,
		EndTimeMs:   2000,
		StopReason:  "completed",
		Operations:  []analysis.OperationResult{},
	}

	if err := rm.StartRun(runID, "test-user"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	waitForRunState(t, rm, runID, RunStateCompleted, 5*time.Second)

	events, _ := rm.TailEvents(runID, 0, 100)
	foundBaseline := false
	foundRamp := false
	foundAnalyzing := false
	foundCompleted := false
	for _, e := range events {
		if e.Type != EventTypeStateTransition {
			continue
		}
		var payload map[string]interface{}
		_ = json.Unmarshal(e.Payload, &payload)
		toState, _ := payload["to_state"].(string)
		switch toState {
		case string(RunStateBaselineRunning):
			foundBaseline = true
		case string(RunStateRampRunning):
			foundRamp = true
		case string(RunStateAnalyzing):
			foundAnalyzing = true
		case string(RunStateCompleted):
			foundCompleted = true
		}
	}

	if !foundBaseline {
		t.Error("expected baseline transition")
	}
	if !foundRamp {
		t.Error("expected ramp transition")
	}
	if !foundAnalyzing {
		t.Error("expected analyzing transition")
	}
	if !foundCompleted {
		t.Error("expected completed transition")
	}
}

func TestRequestStop(t *testing.T) {
	validator := createTestValidator(t)
	rm := NewRunManager(validator)
	config := createValidConfig()

	t.Run("success drain mode", func(t *testing.T) {
		runID, _ := rm.CreateRun(config, "test-user")
		_ = rm.StartRun(runID, "test-user")

		err := rm.RequestStop(runID, StopModeDrain, "test-user")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		view, _ := rm.GetRun(runID)
		if view.State != RunStateStopping {
			t.Errorf("expected state %s, got %s", RunStateStopping, view.State)
		}
		if view.StopReason == nil {
			t.Fatal("expected stop reason to be set")
		}
		if view.StopReason.Mode != StopModeDrain {
			t.Errorf("expected stop mode %s, got %s", StopModeDrain, view.StopReason.Mode)
		}
	})

	t.Run("success immediate mode", func(t *testing.T) {
		runID, _ := rm.CreateRun(config, "test-user")
		_ = rm.StartRun(runID, "test-user")

		err := rm.RequestStop(runID, StopModeImmediate, "test-user")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		view, _ := rm.GetRun(runID)
		if view.StopReason.Mode != StopModeImmediate {
			t.Errorf("expected stop mode %s, got %s", StopModeImmediate, view.StopReason.Mode)
		}
	})

	t.Run("idempotent - already stopping", func(t *testing.T) {
		runID, _ := rm.CreateRun(config, "test-user")
		_ = rm.StartRun(runID, "test-user")
		_ = rm.RequestStop(runID, StopModeDrain, "test-user")

		eventCountBefore := rm.GetEventCount(runID)
		err := rm.RequestStop(runID, StopModeDrain, "test-user")
		if err != nil {
			t.Fatalf("expected no error for idempotent stop, got: %v", err)
		}

		eventCountAfter := rm.GetEventCount(runID)
		if eventCountAfter != eventCountBefore+1 {
			t.Errorf("expected DECISION event to be emitted, events before: %d, after: %d", eventCountBefore, eventCountAfter)
		}

		events, _ := rm.TailEvents(runID, eventCountBefore, 1)
		if len(events) != 1 || events[0].Type != EventTypeDecision {
			t.Error("expected DECISION event for idempotent stop")
		}
	})

	t.Run("run not found", func(t *testing.T) {
		err := rm.RequestStop("nonexistent", StopModeDrain, "test-user")
		if err == nil {
			t.Error("expected error for nonexistent run")
		}
	})

	t.Run("terminal state error", func(t *testing.T) {
		runID, _ := rm.CreateRun(config, "test-user")
		rm.mu.Lock()
		rm.runs[runID].State = RunStateCompleted
		rm.mu.Unlock()

		err := rm.RequestStop(runID, StopModeDrain, "test-user")
		if err == nil {
			t.Error("expected error for terminal state")
		}
		if !strings.Contains(err.Error(), "terminal state") {
			t.Errorf("expected terminal state error, got: %v", err)
		}
	})
}

func TestEmergencyStop(t *testing.T) {
	validator := createTestValidator(t)
	rm := NewRunManager(validator)
	config := createValidConfig()

	t.Run("success", func(t *testing.T) {
		runID, _ := rm.CreateRun(config, "test-user")
		_ = rm.StartRun(runID, "test-user")

		err := rm.EmergencyStop(runID, "test-user")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		view, _ := rm.GetRun(runID)
		if view.State != RunStateStopping {
			t.Errorf("expected state %s, got %s", RunStateStopping, view.State)
		}
		if view.StopReason == nil {
			t.Fatal("expected stop reason to be set")
		}
		if view.StopReason.Mode != StopModeImmediate {
			t.Errorf("expected stop mode %s, got %s", StopModeImmediate, view.StopReason.Mode)
		}
		if view.StopReason.Reason != "emergency_stop" {
			t.Errorf("expected reason 'emergency_stop', got %s", view.StopReason.Reason)
		}
	})

	t.Run("escalation when already stopping", func(t *testing.T) {
		runID, _ := rm.CreateRun(config, "test-user")
		_ = rm.StartRun(runID, "test-user")
		_ = rm.RequestStop(runID, StopModeDrain, "test-user")

		viewBefore, _ := rm.GetRun(runID)
		if viewBefore.StopReason.Mode != StopModeDrain {
			t.Errorf("expected drain mode before escalation, got %s", viewBefore.StopReason.Mode)
		}

		eventCountBefore := rm.GetEventCount(runID)
		err := rm.EmergencyStop(runID, "test-user")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		viewAfter, _ := rm.GetRun(runID)
		if viewAfter.StopReason.Mode != StopModeImmediate {
			t.Errorf("expected immediate mode after escalation, got %s", viewAfter.StopReason.Mode)
		}

		eventCountAfter := rm.GetEventCount(runID)
		if eventCountAfter != eventCountBefore+1 {
			t.Errorf("expected DECISION event for escalation")
		}

		events, _ := rm.TailEvents(runID, eventCountBefore, 1)
		if len(events) != 1 || events[0].Type != EventTypeDecision {
			t.Error("expected DECISION event for escalation")
		}
	})

	t.Run("run not found", func(t *testing.T) {
		err := rm.EmergencyStop("nonexistent", "test-user")
		if err == nil {
			t.Error("expected error for nonexistent run")
		}
	})

	t.Run("terminal state error", func(t *testing.T) {
		runID, _ := rm.CreateRun(config, "test-user")
		rm.mu.Lock()
		rm.runs[runID].State = RunStateFailed
		rm.mu.Unlock()

		err := rm.EmergencyStop(runID, "test-user")
		if err == nil {
			t.Error("expected error for terminal state")
		}
	})
}

func TestGetRun(t *testing.T) {
	validator := createTestValidator(t)
	rm := NewRunManager(validator)
	config := createValidConfig()

	t.Run("success", func(t *testing.T) {
		runID, _ := rm.CreateRun(config, "test-user")

		view, err := rm.GetRun(runID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if view.RunID != runID {
			t.Errorf("expected run ID %s, got %s", runID, view.RunID)
		}
		if view.State != RunStateCreated {
			t.Errorf("expected state %s, got %s", RunStateCreated, view.State)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := rm.GetRun("nonexistent")
		if err == nil {
			t.Error("expected error for nonexistent run")
		}
		if !strings.Contains(err.Error(), "run not found") {
			t.Errorf("expected 'run not found' error, got: %v", err)
		}
	})

	t.Run("last decision event ID", func(t *testing.T) {
		runID, _ := rm.CreateRun(config, "test-user")
		_ = rm.StartRun(runID, "test-user")
		_ = rm.RequestStop(runID, StopModeDrain, "test-user")
		_ = rm.RequestStop(runID, StopModeDrain, "test-user")

		view, _ := rm.GetRun(runID)
		if view.LastDecisionEventID == nil {
			t.Error("expected last decision event ID to be set")
		}
	})
}

func TestTailEvents(t *testing.T) {
	validator := createTestValidator(t)
	rm := NewRunManager(validator)
	config := createValidConfig()

	t.Run("success", func(t *testing.T) {
		runID, _ := rm.CreateRun(config, "test-user")
		_ = rm.StartRun(runID, "test-user")

		events, err := rm.TailEvents(runID, 0, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(events) != 2 {
			t.Errorf("expected 2 events, got %d", len(events))
		}
		if events[0].Type != EventTypeRunCreated {
			t.Errorf("expected first event to be RUN_CREATED, got %s", events[0].Type)
		}
		if events[1].Type != EventTypeStateTransition {
			t.Errorf("expected second event to be STATE_TRANSITION, got %s", events[1].Type)
		}
	})

	t.Run("with cursor", func(t *testing.T) {
		runID, _ := rm.CreateRun(config, "test-user")
		_ = rm.StartRun(runID, "test-user")

		events, err := rm.TailEvents(runID, 1, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(events) != 1 {
			t.Errorf("expected 1 event, got %d", len(events))
		}
		if events[0].Type != EventTypeStateTransition {
			t.Errorf("expected STATE_TRANSITION event, got %s", events[0].Type)
		}
	})

	t.Run("with limit", func(t *testing.T) {
		runID, _ := rm.CreateRun(config, "test-user")
		_ = rm.StartRun(runID, "test-user")
		_ = rm.RequestStop(runID, StopModeDrain, "test-user")

		events, err := rm.TailEvents(runID, 0, 2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(events) != 2 {
			t.Errorf("expected 2 events (limited), got %d", len(events))
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := rm.TailEvents("nonexistent", 0, 10)
		if err == nil {
			t.Error("expected error for nonexistent run")
		}
	})
}

func TestGetEventCount(t *testing.T) {
	validator := createTestValidator(t)
	rm := NewRunManager(validator)
	config := createValidConfig()

	t.Run("success", func(t *testing.T) {
		runID, _ := rm.CreateRun(config, "test-user")
		count := rm.GetEventCount(runID)
		if count != 1 {
			t.Errorf("expected 1 event, got %d", count)
		}

		_ = rm.StartRun(runID, "test-user")
		count = rm.GetEventCount(runID)
		if count != 2 {
			t.Errorf("expected 2 events, got %d", count)
		}
	})

	t.Run("not found returns 0", func(t *testing.T) {
		count := rm.GetEventCount("nonexistent")
		if count != 0 {
			t.Errorf("expected 0 for nonexistent run, got %d", count)
		}
	})
}

func TestConcurrentOperations(t *testing.T) {
	validator := createTestValidator(t)
	rm := NewRunManager(validator)
	config := createValidConfig()

	t.Run("concurrent creates", func(t *testing.T) {
		var wg sync.WaitGroup
		runIDs := make(chan string, 100)

		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				runID, err := rm.CreateRun(config, "test-user")
				if err != nil {
					t.Errorf("concurrent create failed: %v", err)
					return
				}
				runIDs <- runID
			}(i)
		}

		wg.Wait()
		close(runIDs)

		uniqueIDs := make(map[string]bool)
		for runID := range runIDs {
			if uniqueIDs[runID] {
				t.Errorf("duplicate run ID: %s", runID)
			}
			uniqueIDs[runID] = true
		}

		if len(uniqueIDs) != 100 {
			t.Errorf("expected 100 unique run IDs, got %d", len(uniqueIDs))
		}
	})

	t.Run("concurrent reads and writes", func(t *testing.T) {
		runID, _ := rm.CreateRun(config, "test-user")
		_ = rm.StartRun(runID, "test-user")

		var wg sync.WaitGroup

		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, _ = rm.GetRun(runID)
			}()
		}

		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, _ = rm.TailEvents(runID, 0, 100)
			}()
		}

		wg.Wait()
	})
}

func TestStateTransitions(t *testing.T) {
	validator := createTestValidator(t)
	rm := NewRunManager(validator)
	config := createValidConfig()

	t.Run("CREATED to PREFLIGHT_RUNNING", func(t *testing.T) {
		runID, _ := rm.CreateRun(config, "test-user")
		err := rm.StartRun(runID, "test-user")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		view, _ := rm.GetRun(runID)
		if view.State != RunStatePreflightRunning {
			t.Errorf("expected state %s, got %s", RunStatePreflightRunning, view.State)
		}
	})

	t.Run("PREFLIGHT_RUNNING to STOPPING via request_stop", func(t *testing.T) {
		runID, _ := rm.CreateRun(config, "test-user")
		_ = rm.StartRun(runID, "test-user")
		err := rm.RequestStop(runID, StopModeDrain, "test-user")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		view, _ := rm.GetRun(runID)
		if view.State != RunStateStopping {
			t.Errorf("expected state %s, got %s", RunStateStopping, view.State)
		}
	})

	t.Run("PREFLIGHT_RUNNING to STOPPING via emergency_stop", func(t *testing.T) {
		runID, _ := rm.CreateRun(config, "test-user")
		_ = rm.StartRun(runID, "test-user")
		err := rm.EmergencyStop(runID, "test-user")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		view, _ := rm.GetRun(runID)
		if view.State != RunStateStopping {
			t.Errorf("expected state %s, got %s", RunStateStopping, view.State)
		}
	})

	t.Run("cannot start from non-CREATED state", func(t *testing.T) {
		runID, _ := rm.CreateRun(config, "test-user")
		_ = rm.StartRun(runID, "test-user")

		err := rm.StartRun(runID, "test-user")
		if err == nil {
			t.Error("expected error when starting from non-CREATED state")
		}
	})

	t.Run("cannot stop from CREATED state", func(t *testing.T) {
		runID, _ := rm.CreateRun(config, "test-user")

		err := rm.RequestStop(runID, StopModeDrain, "test-user")
		if err == nil {
			t.Error("expected error when stopping from CREATED state")
		}
	})
}

func TestEventEmission(t *testing.T) {
	validator := createTestValidator(t)
	rm := NewRunManager(validator)
	config := createValidConfig()

	t.Run("RUN_CREATED event on create", func(t *testing.T) {
		runID, _ := rm.CreateRun(config, "test-user")
		events, _ := rm.TailEvents(runID, 0, 10)

		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		if events[0].Type != EventTypeRunCreated {
			t.Errorf("expected RUN_CREATED event, got %s", events[0].Type)
		}
		if events[0].RunID != runID {
			t.Errorf("expected run ID %s in event, got %s", runID, events[0].RunID)
		}
	})

	t.Run("STATE_TRANSITION event on start", func(t *testing.T) {
		runID, _ := rm.CreateRun(config, "test-user")
		_ = rm.StartRun(runID, "test-user")
		events, _ := rm.TailEvents(runID, 1, 10)

		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		if events[0].Type != EventTypeStateTransition {
			t.Errorf("expected STATE_TRANSITION event, got %s", events[0].Type)
		}
	})

	t.Run("STOP_REQUESTED and STATE_TRANSITION events on request_stop", func(t *testing.T) {
		runID, _ := rm.CreateRun(config, "test-user")
		_ = rm.StartRun(runID, "test-user")
		_ = rm.RequestStop(runID, StopModeDrain, "test-user")
		events, _ := rm.TailEvents(runID, 2, 10)

		if len(events) != 2 {
			t.Fatalf("expected 2 events, got %d", len(events))
		}
		if events[0].Type != EventTypeStopRequested {
			t.Errorf("expected STOP_REQUESTED event, got %s", events[0].Type)
		}
		if events[1].Type != EventTypeStateTransition {
			t.Errorf("expected STATE_TRANSITION event, got %s", events[1].Type)
		}
	})

	t.Run("EMERGENCY_STOP and STATE_TRANSITION events on emergency_stop", func(t *testing.T) {
		runID, _ := rm.CreateRun(config, "test-user")
		_ = rm.StartRun(runID, "test-user")
		_ = rm.EmergencyStop(runID, "test-user")
		events, _ := rm.TailEvents(runID, 2, 10)

		if len(events) != 2 {
			t.Fatalf("expected 2 events, got %d", len(events))
		}
		if events[0].Type != EventTypeEmergencyStop {
			t.Errorf("expected EMERGENCY_STOP event, got %s", events[0].Type)
		}
		if events[1].Type != EventTypeStateTransition {
			t.Errorf("expected STATE_TRANSITION event, got %s", events[1].Type)
		}
	})

	t.Run("DECISION event on idempotent stop", func(t *testing.T) {
		runID, _ := rm.CreateRun(config, "test-user")
		_ = rm.StartRun(runID, "test-user")
		_ = rm.RequestStop(runID, StopModeDrain, "test-user")
		eventCountBefore := rm.GetEventCount(runID)
		_ = rm.RequestStop(runID, StopModeDrain, "test-user")
		events, _ := rm.TailEvents(runID, eventCountBefore, 10)

		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		if events[0].Type != EventTypeDecision {
			t.Errorf("expected DECISION event, got %s", events[0].Type)
		}
	})

	t.Run("DECISION event on emergency escalation", func(t *testing.T) {
		runID, _ := rm.CreateRun(config, "test-user")
		_ = rm.StartRun(runID, "test-user")
		_ = rm.RequestStop(runID, StopModeDrain, "test-user")
		eventCountBefore := rm.GetEventCount(runID)
		_ = rm.EmergencyStop(runID, "test-user")
		events, _ := rm.TailEvents(runID, eventCountBefore, 10)

		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		if events[0].Type != EventTypeDecision {
			t.Errorf("expected DECISION event, got %s", events[0].Type)
		}
	})
}

func TestConfigHash(t *testing.T) {
	validator := createTestValidator(t)
	rm := NewRunManager(validator)
	config := createValidConfig()

	runID1, _ := rm.CreateRun(config, "test-user")
	runID2, _ := rm.CreateRun(config, "test-user")

	view1, _ := rm.GetRun(runID1)
	view2, _ := rm.GetRun(runID2)

	if view1.ConfigHash != view2.ConfigHash {
		t.Error("expected same config hash for same config")
	}
	if view1.ConfigHash == "" {
		t.Error("expected non-empty config hash")
	}
	if len(view1.ConfigHash) != 64 {
		t.Errorf("expected 64-char SHA256 hash, got %d chars", len(view1.ConfigHash))
	}
}

type mockTelemetryStore struct {
	data map[string]*TelemetryData
	err  error
}

func (m *mockTelemetryStore) GetTelemetryData(runID string) (*TelemetryData, error) {
	if m.err != nil {
		return nil, m.err
	}
	if data, ok := m.data[runID]; ok {
		return data, nil
	}
	return nil, nil
}

func (m *mockTelemetryStore) SetRunMetadata(runID, scenarioID, stopReason string) {}

func TestSetArtifactStore(t *testing.T) {
	rm := NewRunManager(nil)
	store, _ := artifacts.NewFilesystemStore(t.TempDir())

	rm.SetArtifactStore(store)

	rm.mu.RLock()
	defer rm.mu.RUnlock()
	if rm.artifactStore != store {
		t.Error("expected artifact store to be set")
	}
}

func TestSetTelemetryStore(t *testing.T) {
	rm := NewRunManager(nil)
	store := &mockTelemetryStore{}

	rm.SetTelemetryStore(store)

	rm.mu.RLock()
	defer rm.mu.RUnlock()
	if rm.telemetryStore != store {
		t.Error("expected telemetry store to be set")
	}
}

func TestTransitionToAnalyzing(t *testing.T) {
	validator := createTestValidator(t)
	rm := NewRunManager(validator)
	config := createValidConfig()

	t.Run("success from STOPPING", func(t *testing.T) {
		runID, _ := rm.CreateRun(config, "test-user")
		_ = rm.StartRun(runID, "test-user")
		_ = rm.RequestStop(runID, StopModeDrain, "test-user")

		artifactStore, _ := artifacts.NewFilesystemStore(t.TempDir())
		rm.SetArtifactStore(artifactStore)

		telemetryStore := &mockTelemetryStore{
			data: make(map[string]*TelemetryData),
		}
		rm.SetTelemetryStore(telemetryStore)
		telemetryStore.data[runID] = &TelemetryData{
			RunID:       runID,
			ScenarioID:  "test-scenario",
			StartTimeMs: 1000,
			EndTimeMs:   2000,
			StopReason:  "completed",
			Operations:  []analysis.OperationResult{},
		}

		err := rm.TransitionToAnalyzing(runID, "system")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		view, _ := rm.GetRun(runID)
		if view.State != RunStateCompleted {
			t.Errorf("expected state %s, got %s", RunStateCompleted, view.State)
		}

		events, _ := rm.TailEvents(runID, 0, 100)
		foundTransition := false
		foundAnalysisStarted := false
		for _, e := range events {
			if e.Type == EventTypeStateTransition {
				foundTransition = true
			}
			if e.Type == EventTypeAnalysisStarted {
				foundAnalysisStarted = true
			}
		}
		if !foundTransition {
			t.Error("expected STATE_TRANSITION event")
		}
		if !foundAnalysisStarted {
			t.Error("expected ANALYSIS_STARTED event")
		}
	})

	t.Run("run not found", func(t *testing.T) {
		err := rm.TransitionToAnalyzing("nonexistent", "system")
		if err == nil {
			t.Error("expected error for nonexistent run")
		}
	})

	t.Run("invalid state", func(t *testing.T) {
		runID, _ := rm.CreateRun(config, "test-user")

		err := rm.TransitionToAnalyzing(runID, "system")
		if err == nil {
			t.Error("expected error for invalid state")
		}
		if !strings.Contains(err.Error(), "cannot transition to analyzing") {
			t.Errorf("expected state error, got: %v", err)
		}
	})
}

func TestAnalyzeRun(t *testing.T) {
	validator := createTestValidator(t)

	t.Run("success full flow", func(t *testing.T) {
		rm := NewRunManager(validator)
		config := createValidConfig()

		artifactStore, _ := artifacts.NewFilesystemStore(t.TempDir())
		rm.SetArtifactStore(artifactStore)

		telemetryStore := &mockTelemetryStore{
			data: make(map[string]*TelemetryData),
		}
		rm.SetTelemetryStore(telemetryStore)

		runID, _ := rm.CreateRun(config, "test-user")
		_ = rm.StartRun(runID, "test-user")
		_ = rm.RequestStop(runID, StopModeDrain, "test-user")
		telemetryStore.data[runID] = &TelemetryData{
			RunID:       runID,
			ScenarioID:  "test-scenario",
			StartTimeMs: 1000,
			EndTimeMs:   2000,
			StopReason:  "completed",
			Operations: []analysis.OperationResult{
				{Operation: "tools_list", LatencyMs: 50, OK: true},
				{Operation: "tools_call", ToolName: "echo", LatencyMs: 100, OK: true},
				{Operation: "tools_call", ToolName: "echo", LatencyMs: 150, OK: false, ErrorType: "timeout"},
			},
		}

		err := rm.TransitionToAnalyzing(runID, "system")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		view, _ := rm.GetRun(runID)
		if view.State != RunStateCompleted {
			t.Errorf("expected state %s, got %s", RunStateCompleted, view.State)
		}

		artifactsList, _ := artifactStore.ListArtifacts(runID)
		if len(artifactsList) != 2 {
			t.Errorf("expected 2 artifacts, got %d", len(artifactsList))
		}

		foundJSON := false
		foundHTML := false
		for _, a := range artifactsList {
			if a.Filename == "report.json" {
				foundJSON = true
			}
			if a.Filename == "report.html" {
				foundHTML = true
			}
		}
		if !foundJSON {
			t.Error("expected report.json artifact")
		}
		if !foundHTML {
			t.Error("expected report.html artifact")
		}

		events, _ := rm.TailEvents(runID, 0, 100)
		foundReportGenerated := false
		foundAnalysisCompleted := false
		for _, e := range events {
			if e.Type == EventTypeReportGenerated {
				foundReportGenerated = true
				if len(e.Evidence) != 2 {
					t.Errorf("expected 2 evidence items, got %d", len(e.Evidence))
				}
			}
			if e.Type == EventTypeAnalysisCompleted {
				foundAnalysisCompleted = true
			}
		}
		if !foundReportGenerated {
			t.Error("expected REPORT_GENERATED event")
		}
		if !foundAnalysisCompleted {
			t.Error("expected ANALYSIS_COMPLETED event")
		}
	})

	t.Run("run not found", func(t *testing.T) {
		rm := NewRunManager(validator)
		err := rm.AnalyzeRun("nonexistent")
		if err == nil {
			t.Error("expected error for nonexistent run")
		}
	})

	t.Run("invalid state", func(t *testing.T) {
		rm := NewRunManager(validator)
		config := createValidConfig()
		runID, _ := rm.CreateRun(config, "test-user")

		err := rm.AnalyzeRun(runID)
		if err == nil {
			t.Error("expected error for invalid state")
		}
		if !strings.Contains(err.Error(), "cannot analyze run in state") {
			t.Errorf("expected state error, got: %v", err)
		}
	})

	t.Run("no telemetry store", func(t *testing.T) {
		rm := NewRunManager(validator)
		config := createValidConfig()

		artifactStore, _ := artifacts.NewFilesystemStore(t.TempDir())
		rm.SetArtifactStore(artifactStore)

		runID, _ := rm.CreateRun(config, "test-user")
		_ = rm.StartRun(runID, "test-user")
		_ = rm.RequestStop(runID, StopModeDrain, "test-user")

		err := rm.TransitionToAnalyzing(runID, "system")
		if err == nil {
			t.Error("expected error for missing telemetry store")
		}

		view, _ := rm.GetRun(runID)
		if view.State != RunStateFailed {
			t.Errorf("expected state %s, got %s", RunStateFailed, view.State)
		}
	})

	t.Run("no artifact store", func(t *testing.T) {
		rm := NewRunManager(validator)
		config := createValidConfig()

		telemetryStore := &mockTelemetryStore{
			data: make(map[string]*TelemetryData),
		}
		rm.SetTelemetryStore(telemetryStore)

		runID, _ := rm.CreateRun(config, "test-user")
		_ = rm.StartRun(runID, "test-user")
		_ = rm.RequestStop(runID, StopModeDrain, "test-user")
		telemetryStore.data[runID] = &TelemetryData{
			RunID:       runID,
			StartTimeMs: 1000,
			EndTimeMs:   2000,
			Operations:  []analysis.OperationResult{},
		}

		err := rm.TransitionToAnalyzing(runID, "system")
		if err == nil {
			t.Error("expected error for missing artifact store")
		}

		view, _ := rm.GetRun(runID)
		if view.State != RunStateFailed {
			t.Errorf("expected state %s, got %s", RunStateFailed, view.State)
		}
	})

	t.Run("telemetry retrieval failure", func(t *testing.T) {
		rm := NewRunManager(validator)
		config := createValidConfig()

		artifactStore, _ := artifacts.NewFilesystemStore(t.TempDir())
		rm.SetArtifactStore(artifactStore)

		telemetryStore := &mockTelemetryStore{
			err: os.ErrNotExist,
		}
		rm.SetTelemetryStore(telemetryStore)

		runID, _ := rm.CreateRun(config, "test-user")
		_ = rm.StartRun(runID, "test-user")
		_ = rm.RequestStop(runID, StopModeDrain, "test-user")

		err := rm.TransitionToAnalyzing(runID, "system")
		if err == nil {
			t.Error("expected error for telemetry retrieval failure")
		}

		view, _ := rm.GetRun(runID)
		if view.State != RunStateFailed {
			t.Errorf("expected state %s, got %s", RunStateFailed, view.State)
		}
	})

	t.Run("empty telemetry data", func(t *testing.T) {
		rm := NewRunManager(validator)
		config := createValidConfig()

		artifactStore, _ := artifacts.NewFilesystemStore(t.TempDir())
		rm.SetArtifactStore(artifactStore)

		telemetryStore := &mockTelemetryStore{
			data: make(map[string]*TelemetryData),
		}
		rm.SetTelemetryStore(telemetryStore)

		runID, _ := rm.CreateRun(config, "test-user")
		_ = rm.StartRun(runID, "test-user")
		_ = rm.RequestStop(runID, StopModeDrain, "test-user")
		telemetryStore.data[runID] = &TelemetryData{
			RunID:       runID,
			StartTimeMs: 1000,
			EndTimeMs:   2000,
			Operations:  []analysis.OperationResult{},
		}

		err := rm.TransitionToAnalyzing(runID, "system")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		view, _ := rm.GetRun(runID)
		if view.State != RunStateCompleted {
			t.Errorf("expected state %s, got %s", RunStateCompleted, view.State)
		}
	})
}

func TestAnalysisEventEmission(t *testing.T) {
	validator := createTestValidator(t)
	rm := NewRunManager(validator)
	config := createValidConfig()

	artifactStore, _ := artifacts.NewFilesystemStore(t.TempDir())
	rm.SetArtifactStore(artifactStore)

	telemetryStore := &mockTelemetryStore{
		data: make(map[string]*TelemetryData),
	}
	rm.SetTelemetryStore(telemetryStore)

	runID, _ := rm.CreateRun(config, "test-user")
	_ = rm.StartRun(runID, "test-user")
	_ = rm.RequestStop(runID, StopModeDrain, "test-user")
	telemetryStore.data[runID] = &TelemetryData{
		RunID:       runID,
		StartTimeMs: 1000,
		EndTimeMs:   2000,
		Operations: []analysis.OperationResult{
			{Operation: "ping", LatencyMs: 10, OK: true},
		},
	}

	_ = rm.TransitionToAnalyzing(runID, "system")

	events, _ := rm.TailEvents(runID, 0, 100)

	eventTypes := make(map[EventType]int)
	for _, e := range events {
		eventTypes[e.Type]++
	}

	if eventTypes[EventTypeRunCreated] != 1 {
		t.Errorf("expected 1 RUN_CREATED event, got %d", eventTypes[EventTypeRunCreated])
	}
	if eventTypes[EventTypeAnalysisStarted] != 1 {
		t.Errorf("expected 1 ANALYSIS_STARTED event, got %d", eventTypes[EventTypeAnalysisStarted])
	}
	if eventTypes[EventTypeReportGenerated] != 1 {
		t.Errorf("expected 1 REPORT_GENERATED event, got %d", eventTypes[EventTypeReportGenerated])
	}
	if eventTypes[EventTypeAnalysisCompleted] != 1 {
		t.Errorf("expected 1 ANALYSIS_COMPLETED event, got %d", eventTypes[EventTypeAnalysisCompleted])
	}
}
