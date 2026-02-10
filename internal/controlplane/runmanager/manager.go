package runmanager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/analysis"
	"github.com/bc-dunia/mcpdrill/internal/artifacts"
	"github.com/bc-dunia/mcpdrill/internal/controlplane/scheduler"
	"github.com/bc-dunia/mcpdrill/internal/types"
	"github.com/bc-dunia/mcpdrill/internal/validation"
)

// StopMode represents the mode for stopping a run.
type StopMode string

const (
	StopModeDrain     StopMode = "drain"
	StopModeImmediate StopMode = "immediate"
)

// StopReason contains information about why a run was stopped.
type StopReason struct {
	Mode   StopMode `json:"mode"`
	Reason string   `json:"reason"`
	Actor  string   `json:"actor"`
	AtMs   int64    `json:"at_ms"`
}

// ActiveStageInfo contains information about the currently active stage.
type ActiveStageInfo struct {
	Stage   string `json:"stage"`
	StageID string `json:"stage_id"`
}

// RunRecord is the internal representation of a run.
type RunRecord struct {
	RunID       string           `json:"run_id"`
	ExecutionID string           `json:"execution_id"`
	State       RunState         `json:"state"`
	ConfigHash  string           `json:"config_hash"`
	ScenarioID  string           `json:"scenario_id"`
	CreatedAtMs int64            `json:"created_at_ms"`
	UpdatedAtMs int64            `json:"updated_at_ms"`
	ActiveStage *ActiveStageInfo `json:"active_stage,omitempty"`
	StopReason  *StopReason      `json:"stop_reason,omitempty"`
	Actor       string           `json:"actor"`
	Config      json.RawMessage  `json:"-"` // Raw config for assignment creation

	progressionCancel    context.CancelFunc
	progressionTimers    []*time.Timer
	stopConditionsCancel context.CancelFunc
	rampCancel           context.CancelFunc
	drainCancel          chan struct{} // Channel to cancel drain wait early (for emergency stop or worker loss)
	immediateStop        bool          // True if emergency_stop escalated while in STOPPING (workers should terminate immediately)
}

// RunView is the external representation of a run (matches run-view/v1 schema).
type RunView struct {
	RunID               string           `json:"run_id"`
	ExecutionID         string           `json:"execution_id"`
	State               RunState         `json:"state"`
	ScenarioID          string           `json:"scenario_id"`
	ConfigHash          string           `json:"config_hash"`
	CreatedAtMs         int64            `json:"created_at_ms"`
	UpdatedAtMs         int64            `json:"updated_at_ms"`
	ActiveStage         *ActiveStageInfo `json:"active_stage,omitempty"`
	StopReason          *StopReason      `json:"stop_reason,omitempty"`
	LastDecisionEventID *string          `json:"last_decision_event_id,omitempty"`
}

// AssignmentSender is an interface for sending assignments to workers.
type AssignmentSender interface {
	AddAssignment(workerID string, assignment types.WorkerAssignment)
}

// TelemetryData represents telemetry collected during a run for analysis.
type TelemetryData struct {
	RunID       string
	ScenarioID  string
	StartTimeMs int64
	EndTimeMs   int64
	StopReason  string
	Operations  []analysis.OperationResult
}

// TelemetryStore provides access to telemetry data for a run.
type TelemetryStore interface {
	GetTelemetryData(runID string) (*TelemetryData, error)
	SetRunMetadata(runID, scenarioID, stopReason string)
}

// RunManager is the core control plane component for managing run lifecycle.
type RunManager struct {
	mu        sync.RWMutex
	runs      map[string]*RunRecord
	eventLogs map[string]*EventLog
	validator *validation.UnifiedValidator

	ctx    context.Context
	cancel context.CancelFunc

	registry         *scheduler.Registry
	allocator        *scheduler.Allocator
	leaseManager     *scheduler.LeaseManager
	assignmentSender AssignmentSender

	artifactStore  artifacts.Store
	telemetryStore TelemetryStore

	runIDCounter atomic.Int64
	exeIDCounter atomic.Int64
}

// NewRunManager creates a new RunManager with the given validator.
func NewRunManager(validator *validation.UnifiedValidator) *RunManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &RunManager{
		runs:      make(map[string]*RunRecord),
		eventLogs: make(map[string]*EventLog),
		validator: validator,
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Shutdown cancels all background goroutines (stage progression, ramp, analysis).
func (rm *RunManager) Shutdown() {
	rm.cancel()
}

// SetScheduler configures the scheduler components for assignment creation.
func (rm *RunManager) SetScheduler(registry *scheduler.Registry, allocator *scheduler.Allocator, leaseManager *scheduler.LeaseManager) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.registry = registry
	rm.allocator = allocator
	rm.leaseManager = leaseManager
}

// SetAssignmentSender configures the assignment sender for dispatching work to workers.
func (rm *RunManager) SetAssignmentSender(sender AssignmentSender) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.assignmentSender = sender
}

// SetArtifactStore configures the artifact store for report storage.
func (rm *RunManager) SetArtifactStore(store artifacts.Store) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.artifactStore = store
}

// SetTelemetryStore configures the telemetry store for analysis.
func (rm *RunManager) SetTelemetryStore(store TelemetryStore) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.telemetryStore = store
}

// generateRunID generates a unique run ID.
// Format: run_{20 hex chars} to match pattern ^run_[0-9a-f]{16,64}$
func (rm *RunManager) generateRunID() string {
	ts := time.Now().UnixNano()
	counter := rm.runIDCounter.Add(1)
	// Create a deterministic 20-char hex suffix from timestamp and counter
	// Using hex encoding (0-9a-f) for compact representation
	suffix := fmt.Sprintf("%016x%04x", ts, counter&0xFFFF)
	return fmt.Sprintf("run_%s", suffix)
}

// generateExecutionID generates a unique execution ID.
// Format: exe_{16 hex chars} to match pattern ^exe_[0-9a-f]{8,64}$
func (rm *RunManager) generateExecutionID() string {
	ts := time.Now().UnixNano()
	counter := rm.exeIDCounter.Add(1)
	// Create a deterministic 16-char hex suffix from timestamp and counter
	suffix := fmt.Sprintf("%012x%04x", ts&0xFFFFFFFFFFFF, counter&0xFFFF)
	return fmt.Sprintf("exe_%s", suffix)
}

// computeConfigHash computes SHA256 hash of config bytes.
func computeConfigHash(config []byte) string {
	hash := sha256.Sum256(config)
	return hex.EncodeToString(hash[:])
}

// extractScenarioID extracts scenario_id from config JSON.
func extractScenarioID(config []byte) string {
	var parsed map[string]interface{}
	if err := json.Unmarshal(config, &parsed); err != nil {
		return ""
	}
	if id, ok := parsed["scenario_id"].(string); ok {
		return id
	}
	return ""
}

// ValidateRunConfig validates a run configuration and returns a validation report.
func (rm *RunManager) ValidateRunConfig(config []byte) *validation.ValidationReport {
	if rm.validator == nil {
		report := validation.NewValidationReport()
		report.AddError("VALIDATOR_NOT_CONFIGURED", "No validator configured", "")
		return report
	}
	return rm.validator.ValidateRunConfig(config)
}

// CreateRun creates a new run with the given configuration.
// Returns the run ID on success, or an error if validation fails.
func (rm *RunManager) CreateRun(config []byte, actor string) (string, error) {
	report := rm.ValidateRunConfig(config)
	if !report.OK {
		return "", &validation.ValidationError{Report: report}
	}

	runID := rm.generateRunID()
	executionID := rm.generateExecutionID()
	configHash := computeConfigHash(config)
	scenarioID := extractScenarioID(config)
	nowMs := time.Now().UnixMilli()

	record := &RunRecord{
		RunID:       runID,
		ExecutionID: executionID,
		State:       RunStateCreated,
		ConfigHash:  configHash,
		ScenarioID:  scenarioID,
		CreatedAtMs: nowMs,
		UpdatedAtMs: nowMs,
		Actor:       actor,
		Config:      config,
	}

	eventLog := NewEventLog()

	rm.mu.Lock()
	rm.runs[runID] = record
	rm.eventLogs[runID] = eventLog
	rm.mu.Unlock()

	payload, err := json.Marshal(map[string]interface{}{
		"config_hash": configHash,
		"scenario_id": scenarioID,
		"actor":       actor,
	})
	if err != nil {
		log.Printf("[RunManager] Failed to marshal CreateRun event payload for run %s: %v", runID, err)
		payload = []byte("{}")
	}

	event := RunEvent{
		RunID:       runID,
		ExecutionID: executionID,
		Type:        EventTypeRunCreated,
		Actor:       ActorType(actor),
		Payload:     payload,
		Evidence:    []Evidence{},
	}
	if err := eventLog.Append(event); err != nil {
		log.Printf("[RunManager] CRITICAL: Failed to append RUN_CREATED event for run %s: %v", runID, err)
	}

	return runID, nil
}

// StartRun transitions a run from CREATED to PREFLIGHT_RUNNING.
// Returns an error if the run is not in CREATED state or if allocation fails.
// Per spec: allocation must succeed before transitioning to PREFLIGHT_RUNNING.
func (rm *RunManager) StartRun(runID, actor string) error {
	var (
		configCopy       []byte
		executionID      string
		eventLog         *EventLog
		registry         *scheduler.Registry
		allocator        *scheduler.Allocator
		leaseManager     *scheduler.LeaseManager
		assignmentSender AssignmentSender
		err              error
	)

	func() {
		rm.mu.Lock()
		defer rm.mu.Unlock()

		record, ok := rm.runs[runID]
		if !ok {
			err = NewNotFoundError(runID)
			return
		}

		if record.State != RunStateCreated {
			err = NewInvalidStateError(runID, record.State, RunStateCreated, "start")
			return
		}

		if !CanTransition(record.State, RunStatePreflightRunning) {
			err = NewInvalidTransitionError(runID, record.State, RunStatePreflightRunning)
			return
		}

		configCopy = make([]byte, len(record.Config))
		copy(configCopy, record.Config)
		executionID = record.ExecutionID
		eventLog = rm.eventLogs[runID]

		registry = rm.registry
		allocator = rm.allocator
		leaseManager = rm.leaseManager
		assignmentSender = rm.assignmentSender
	}()

	if err != nil {
		return err
	}

	// If scheduler components are partially configured, that's a misconfiguration
	schedulerConfigured := registry != nil || allocator != nil || leaseManager != nil || assignmentSender != nil
	schedulerComplete := registry != nil && allocator != nil && leaseManager != nil && assignmentSender != nil
	if schedulerConfigured && !schedulerComplete {
		rm.transitionToFailedFromCreated(runID, executionID, eventLog, actor, "scheduler_misconfiguration")
		return fmt.Errorf("scheduler partially configured for run %s: some components are nil", runID)
	}

	// Per spec: attempt allocation BEFORE state transition
	if registry != nil && allocator != nil && leaseManager != nil && assignmentSender != nil {
		if !rm.tryAllocateForStage(runID, executionID, configCopy, eventLog, StageNamePreflight) {
			rm.transitionToFailedFromCreated(runID, executionID, eventLog, actor, "allocation_failed")
			return fmt.Errorf("allocation failed for run %s", runID)
		}
	}

	// Allocation succeeded, now transition to PREFLIGHT_RUNNING
	func() {
		rm.mu.Lock()
		defer rm.mu.Unlock()

		record, ok := rm.runs[runID]
		if !ok {
			err = NewNotFoundError(runID)
			return
		}
		if record.State != RunStateCreated {
			err = fmt.Errorf("run state changed during allocation: %s", record.State)
			return
		}

		oldState := record.State
		record.State = RunStatePreflightRunning
		record.UpdatedAtMs = time.Now().UnixMilli()

		payload, _ := json.Marshal(map[string]interface{}{
			"from_state": oldState,
			"to_state":   record.State,
			"trigger":    "start_run",
			"actor":      actor,
		})

		event := RunEvent{
			RunID:       runID,
			ExecutionID: executionID,
			Type:        EventTypeStateTransition,
			Actor:       ActorType(actor),
			Payload:     payload,
			Evidence:    []Evidence{},
		}
		appendEventWithLog(eventLog, event, "transitionToPreflightRunning")
	}()

	if err != nil {
		return err
	}

	// Dispatch assignments after state transition
	if registry != nil && allocator != nil && leaseManager != nil && assignmentSender != nil {
		rm.createAndDispatchAssignmentsForStage(runID, executionID, configCopy, eventLog, StageNamePreflight)
	}

	rm.startStageProgression(runID, configCopy, string(ActorAutoramp))

	return nil
}

// transitionToFailedFromCreated transitions a run from CREATED to FAILED state.
func (rm *RunManager) transitionToFailedFromCreated(runID, executionID string, eventLog *EventLog, actor, reason string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	record, ok := rm.runs[runID]
	if !ok || record.State != RunStateCreated {
		return
	}

	oldState := record.State
	record.State = RunStateFailed
	record.UpdatedAtMs = time.Now().UnixMilli()

	payload, _ := json.Marshal(map[string]interface{}{
		"from_state": oldState,
		"to_state":   record.State,
		"trigger":    reason,
		"actor":      actor,
	})

	event := RunEvent{
		RunID:       runID,
		ExecutionID: executionID,
		Type:        EventTypeStateTransition,
		Actor:       ActorType(actor),
		Payload:     payload,
		Evidence:    []Evidence{{Kind: "reason", Ref: reason}},
	}
	appendEventWithLog(eventLog, event, "transitionToFailedFromCreated")
}

// RequestStop transitions a run to STOPPING state with the specified mode.
// Returns an error if the transition is not valid.
func (rm *RunManager) RequestStop(runID string, mode StopMode, actor string) error {
	return rm.requestStopWithReason(runID, mode, actor, "stop_requested", nil)
}

func (rm *RunManager) requestStopWithReason(runID string, mode StopMode, actor string, reason string, evidence []Evidence) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	record, ok := rm.runs[runID]
	if !ok {
		return NewNotFoundError(runID)
	}

	if record.State == RunStateCompleted || record.State == RunStateFailed || record.State == RunStateAborted {
		return NewTerminalStateError(runID, record.State, "stop")
	}

	rm.cancelStageProgressionLocked(record)
	rm.stopStopConditionEvaluatorLocked(record)

	eventLog := rm.eventLogs[runID]
	if record.State == RunStateStopping {
		payload, _ := json.Marshal(map[string]interface{}{
			"decision_type": "stop_trigger_ignored",
			"reason":        "already_stopping",
			"actor":         actor,
		})
		event := RunEvent{
			RunID:       runID,
			ExecutionID: record.ExecutionID,
			Type:        EventTypeDecision,
			Actor:       ActorType(actor),
			Payload:     payload,
			Evidence:    []Evidence{},
		}
		appendEventWithLog(eventLog, event, "requestStopWithReason")
		return nil
	}

	if !CanTransition(record.State, RunStateStopping) {
		return NewInvalidTransitionError(runID, record.State, RunStateStopping)
	}

	oldState := record.State
	record.State = RunStateStopping
	record.UpdatedAtMs = time.Now().UnixMilli()
	record.StopReason = &StopReason{
		Mode:   mode,
		Reason: reason,
		Actor:  actor,
		AtMs:   record.UpdatedAtMs,
	}

	// Initialize drainCancel when entering STOPPING to avoid race with HandleWorkerCapacityLost
	if record.drainCancel == nil {
		record.drainCancel = make(chan struct{})
	}

	stopPayload, _ := json.Marshal(map[string]interface{}{
		"mode":   mode,
		"actor":  actor,
		"reason": reason,
	})
	stopEvent := RunEvent{
		RunID:       runID,
		ExecutionID: record.ExecutionID,
		Type:        EventTypeStopRequested,
		Actor:       ActorType(actor),
		Payload:     stopPayload,
		Evidence:    nonNilEvidence(evidence),
	}
	appendEventWithLog(eventLog, stopEvent, "requestStopWithReason")

	transitionPayload, _ := json.Marshal(map[string]interface{}{
		"from_state": oldState,
		"to_state":   record.State,
		"trigger":    "stop_requested",
		"actor":      actor,
	})
	transitionEvent := RunEvent{
		RunID:       runID,
		ExecutionID: record.ExecutionID,
		Type:        EventTypeStateTransition,
		Actor:       ActorType(actor),
		Payload:     transitionPayload,
		Evidence:    []Evidence{},
	}
	appendEventWithLog(eventLog, transitionEvent, "requestStopWithReason")

	// Set stop reason in telemetry for manual stop
	if rm.telemetryStore != nil {
		rm.telemetryStore.SetRunMetadata(runID, "", reason)
	}

	drainTimeout := getDrainTimeout(record.Config)
	go rm.finalizeRun(runID, drainTimeout, actor)

	return nil
}

// EmergencyStop immediately stops a run.
// Returns an error if the run is not found or in a terminal state.
func (rm *RunManager) EmergencyStop(runID, actor string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	record, ok := rm.runs[runID]
	if !ok {
		return NewNotFoundError(runID)
	}

	if record.State == RunStateCompleted || record.State == RunStateFailed || record.State == RunStateAborted {
		return NewTerminalStateError(runID, record.State, "emergency-stop")
	}

	rm.cancelStageProgressionLocked(record)

	eventLog := rm.eventLogs[runID]

	if record.State == RunStateStopping {
		escalationPayload, _ := json.Marshal(map[string]interface{}{
			"decision_type": "stop_trigger_resolution",
			"details": map[string]interface{}{
				"escalated": true,
			},
			"actor": actor,
		})
		escalationEvent := RunEvent{
			RunID:       runID,
			ExecutionID: record.ExecutionID,
			Type:        EventTypeDecision,
			Actor:       ActorType(actor),
			Payload:     escalationPayload,
			Evidence:    []Evidence{},
		}
		appendEventWithLog(eventLog, escalationEvent, "EmergencyStop")

		record.StopReason = &StopReason{
			Mode:   StopModeImmediate,
			Reason: "emergency_stop",
			Actor:  actor,
			AtMs:   time.Now().UnixMilli(),
		}
		record.UpdatedAtMs = time.Now().UnixMilli()

		// Per ref/11-state-machine.md: emergency_stop while STOPPING reduces drain timeout to 5s
		// and sends immediate_stop to workers
		record.immediateStop = true
		if record.drainCancel != nil {
			close(record.drainCancel)
			record.drainCancel = nil
		}
		return nil
	}

	if !CanTransition(record.State, RunStateStopping) {
		return NewInvalidTransitionError(runID, record.State, RunStateStopping)
	}

	oldState := record.State
	record.State = RunStateStopping
	record.UpdatedAtMs = time.Now().UnixMilli()
	record.StopReason = &StopReason{
		Mode:   StopModeImmediate,
		Reason: "emergency_stop",
		Actor:  actor,
		AtMs:   record.UpdatedAtMs,
	}

	emergencyPayload, _ := json.Marshal(map[string]interface{}{
		"actor":  actor,
		"reason": "emergency_stop",
	})
	emergencyEvent := RunEvent{
		RunID:       runID,
		ExecutionID: record.ExecutionID,
		Type:        EventTypeEmergencyStop,
		Actor:       ActorType(actor),
		Payload:     emergencyPayload,
		Evidence:    []Evidence{},
	}
	appendEventWithLog(eventLog, emergencyEvent, "EmergencyStop")

	transitionPayload, _ := json.Marshal(map[string]interface{}{
		"from_state": oldState,
		"to_state":   record.State,
		"trigger":    "emergency_stop",
		"actor":      actor,
	})
	transitionEvent := RunEvent{
		RunID:       runID,
		ExecutionID: record.ExecutionID,
		Type:        EventTypeStateTransition,
		Actor:       ActorType(actor),
		Payload:     transitionPayload,
		Evidence:    []Evidence{},
	}
	appendEventWithLog(eventLog, transitionEvent, "EmergencyStop")

	// Set stop reason in telemetry for emergency stop
	if rm.telemetryStore != nil {
		rm.telemetryStore.SetRunMetadata(runID, "", "emergency_stop")
	}

	go rm.finalizeRun(runID, 0, actor)

	return nil
}

func stringPtr(s string) *string {
	return &s
}

func nonNilEvidence(evidence []Evidence) []Evidence {
	if evidence == nil {
		return []Evidence{}
	}
	return evidence
}

func (rm *RunManager) finalizeRun(runID string, drainTimeout time.Duration, actor string) {
	if drainTimeout > 0 {
		rm.mu.Lock()
		record, ok := rm.runs[runID]
		if !ok {
			rm.mu.Unlock()
			return
		}
		// Use existing drainCancel if already initialized (avoids race condition)
		drainCancel := record.drainCancel
		if drainCancel == nil {
			drainCancel = make(chan struct{})
			record.drainCancel = drainCancel
		}
		rm.mu.Unlock()

		drainTimer := time.NewTimer(drainTimeout)
		select {
		case <-drainTimer.C:
		case <-drainCancel:
			drainTimer.Stop()
			log.Printf("[RunManager] Drain cancelled early for run %s (emergency stop or worker loss)", runID)
			// Only delay for emergency stop (immediateStop), not for worker loss
			rm.mu.RLock()
			isImmediateStop := false
			if rec, ok := rm.runs[runID]; ok {
				isImmediateStop = rec.immediateStop
			}
			rm.mu.RUnlock()
			if isImmediateStop {
				time.Sleep(5 * time.Second)
			}
		}
	}

	rm.mu.RLock()
	record, ok := rm.runs[runID]
	if !ok {
		rm.mu.RUnlock()
		return
	}
	if record.State != RunStateStopping {
		rm.mu.RUnlock()
		return
	}
	telemetryStore := rm.telemetryStore
	artifactStore := rm.artifactStore
	rm.mu.RUnlock()

	if telemetryStore == nil || artifactStore == nil {
		rm.transitionToCompleted(runID, actor, "no_telemetry")
		return
	}

	if err := rm.TransitionToAnalyzing(runID, actor); err != nil {
		log.Printf("[RunManager] Failed to transition run %s to analyzing: %v", runID, err)
	}
}

func (rm *RunManager) transitionToCompleted(runID, actor, reason string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	record, ok := rm.runs[runID]
	if !ok || record.State != RunStateStopping {
		return
	}

	oldState := record.State
	record.State = RunStateCompleted
	record.UpdatedAtMs = time.Now().UnixMilli()

	eventLog := rm.eventLogs[runID]
	payload, _ := json.Marshal(map[string]interface{}{
		"from_state": oldState,
		"to_state":   record.State,
		"trigger":    reason,
		"actor":      actor,
	})
	event := RunEvent{
		RunID:       runID,
		ExecutionID: record.ExecutionID,
		Type:        EventTypeStateTransition,
		Actor:       ActorType(actor),
		Payload:     payload,
		Evidence:    []Evidence{},
	}
	appendEventWithLog(eventLog, event, "transitionToStateWithActor")
}
