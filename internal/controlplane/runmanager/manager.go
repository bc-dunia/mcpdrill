package runmanager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strings"
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
	return &RunManager{
		runs:      make(map[string]*RunRecord),
		eventLogs: make(map[string]*EventLog),
		validator: validator,
	}
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
			ExecutionID: record.ExecutionID,
			Type:        EventTypeStateTransition,
			Actor:       ActorType(actor),
			Payload:     payload,
			Evidence:    []Evidence{},
		}
		_ = eventLog.Append(event)
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
	_ = eventLog.Append(event)
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
		_ = eventLog.Append(event)
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
	_ = eventLog.Append(stopEvent)

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
	_ = eventLog.Append(transitionEvent)

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
		_ = eventLog.Append(escalationEvent)

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
	_ = eventLog.Append(emergencyEvent)

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
	_ = eventLog.Append(transitionEvent)

	// Set stop reason in telemetry for emergency stop
	if rm.telemetryStore != nil {
		rm.telemetryStore.SetRunMetadata(runID, "", "emergency_stop")
	}

	go rm.finalizeRun(runID, 0, actor)

	return nil
}

// ListRuns returns a list of all runs.
func (rm *RunManager) ListRuns() []*RunView {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	result := make([]*RunView, 0, len(rm.runs))
	for _, record := range rm.runs {
		var lastDecisionEventID *string
		if eventLog, ok := rm.eventLogs[record.RunID]; ok {
			events := eventLog.GetAll()
			for i := len(events) - 1; i >= 0; i-- {
				if events[i].Type == EventTypeDecision {
					lastDecisionEventID = &events[i].EventID
					break
				}
			}
		}

		view := &RunView{
			RunID:               record.RunID,
			ExecutionID:         record.ExecutionID,
			State:               record.State,
			ScenarioID:          record.ScenarioID,
			ConfigHash:          record.ConfigHash,
			CreatedAtMs:         record.CreatedAtMs,
			UpdatedAtMs:         record.UpdatedAtMs,
			ActiveStage:         record.ActiveStage,
			StopReason:          record.StopReason,
			LastDecisionEventID: lastDecisionEventID,
		}
		result = append(result, view)
	}
	return result
}

func (rm *RunManager) ListStoppingRunsForWorker(workerID string) []string {
	rm.mu.RLock()
	leaseManager := rm.leaseManager
	rm.mu.RUnlock()

	if leaseManager == nil {
		return nil
	}

	workerRunIDs := leaseManager.ListWorkerRunIDs(scheduler.WorkerID(workerID))

	rm.mu.RLock()
	defer rm.mu.RUnlock()

	var stoppingRuns []string
	for _, runID := range workerRunIDs {
		if record, ok := rm.runs[runID]; ok {
			if record.State == RunStateStopping ||
				record.State == RunStateCompleted ||
				record.State == RunStateFailed ||
				record.State == RunStateAborted {
				stoppingRuns = append(stoppingRuns, runID)
			}
		}
	}
	return stoppingRuns
}

func (rm *RunManager) ListImmediateStopRunsForWorker(workerID string) []string {
	rm.mu.RLock()
	leaseManager := rm.leaseManager
	rm.mu.RUnlock()

	if leaseManager == nil {
		return nil
	}

	workerRunIDs := leaseManager.ListWorkerRunIDs(scheduler.WorkerID(workerID))

	rm.mu.RLock()
	defer rm.mu.RUnlock()

	var immediateStopRuns []string
	for _, runID := range workerRunIDs {
		if record, ok := rm.runs[runID]; ok {
			if record.immediateStop {
				immediateStopRuns = append(immediateStopRuns, runID)
			}
		}
	}
	return immediateStopRuns
}

// GetRun returns the current view of a run.
// Returns an error if the run is not found.
func (rm *RunManager) GetRun(runID string) (*RunView, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	record, ok := rm.runs[runID]
	if !ok {
		return nil, NewNotFoundError(runID)
	}

	var lastDecisionEventID *string
	if eventLog, ok := rm.eventLogs[runID]; ok {
		events := eventLog.GetAll()
		for i := len(events) - 1; i >= 0; i-- {
			if events[i].Type == EventTypeDecision {
				lastDecisionEventID = &events[i].EventID
				break
			}
		}
	}

	view := &RunView{
		RunID:               record.RunID,
		ExecutionID:         record.ExecutionID,
		State:               record.State,
		ScenarioID:          record.ScenarioID,
		ConfigHash:          record.ConfigHash,
		CreatedAtMs:         record.CreatedAtMs,
		UpdatedAtMs:         record.UpdatedAtMs,
		ActiveStage:         record.ActiveStage,
		StopReason:          record.StopReason,
		LastDecisionEventID: lastDecisionEventID,
	}

	return view, nil
}

// GetRunServerTelemetryPairKey returns the server_telemetry.pair_key from a run's config.
// Returns empty string if the run is not found or has no pair_key configured.
func (rm *RunManager) GetRunServerTelemetryPairKey(runID string) string {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	record, ok := rm.runs[runID]
	if !ok || record.Config == nil {
		return ""
	}

	// Parse the config to extract server_telemetry.pair_key
	var config struct {
		ServerTelemetry struct {
			Enabled bool   `json:"enabled"`
			PairKey string `json:"pair_key"`
		} `json:"server_telemetry"`
	}
	if err := json.Unmarshal(record.Config, &config); err != nil {
		return ""
	}

	return config.ServerTelemetry.PairKey
}

// TailEvents returns events from the run's event log starting from cursor.
// cursor is the 0-based index to start from.
// limit is the maximum number of events to return.
// Returns an error if the run is not found.
func (rm *RunManager) TailEvents(runID string, cursor, limit int) ([]RunEvent, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	eventLog, ok := rm.eventLogs[runID]
	if !ok {
		return nil, NewNotFoundError(runID)
	}

	return eventLog.Tail(cursor, limit)
}

// GetEventCount returns the number of events in a run's event log.
// Returns 0 if the run is not found.
func (rm *RunManager) GetEventCount(runID string) int {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	eventLog, ok := rm.eventLogs[runID]
	if !ok {
		return 0
	}

	return eventLog.Len()
}

// FindEventIndex finds the index of an event by its event_id.
// Returns -1 if the run or event is not found.
func (rm *RunManager) FindEventIndex(runID, eventID string) int {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	eventLog, ok := rm.eventLogs[runID]
	if !ok {
		return -1
	}

	return eventLog.FindEventIndex(eventID)
}

type parsedRunConfig struct {
	Target        parsedTarget        `json:"target"`
	Stages        []parsedStage       `json:"stages"`
	Workload      parsedWorkload      `json:"workload"`
	SessionPolicy parsedSessionPolicy `json:"session_policy"`
	Safety        parsedSafety        `json:"safety"`
}

type parsedRedirectPolicy struct {
	Mode         string   `json:"mode"`
	MaxRedirects int      `json:"max_redirects,omitempty"`
	Allowlist    []string `json:"allowlist,omitempty"`
}

type parsedAuth struct {
	Type   string   `json:"type"`
	Tokens []string `json:"tokens,omitempty"`
}

type parsedTarget struct {
	URL            string                `json:"url"`
	Transport      string                `json:"transport"`
	Headers        map[string]string     `json:"headers,omitempty"`
	Auth           *parsedAuth           `json:"auth,omitempty"`
	Identification *parsedIdentification `json:"identification,omitempty"`
	RedirectPolicy *parsedRedirectPolicy `json:"redirect_policy,omitempty"`
}

type parsedIdentification struct {
	RunIDHeader *parsedRunIDHeader `json:"run_id_header,omitempty"`
	UserAgent   *parsedUserAgent   `json:"user_agent,omitempty"`
}

type parsedRunIDHeader struct {
	Name          string `json:"name"`
	ValueTemplate string `json:"value_template"`
}

type parsedUserAgent struct {
	Value string `json:"value"`
}

type parsedStreamingConfig struct {
	StreamStallSeconds int     `json:"stream_stall_seconds,omitempty"`
	MinEventsPerSecond float64 `json:"min_events_per_second,omitempty"`
}

type parsedStage struct {
	StageID             string                 `json:"stage_id"`
	Stage               string                 `json:"stage"`
	Enabled             bool                   `json:"enabled"`
	DurationMs          int64                  `json:"duration_ms"`
	MaxDurationMs       int64                  `json:"max_duration_ms,omitempty"`
	Load                parsedLoad             `json:"load"`
	StopConditions      []parsedStopCondition  `json:"stop_conditions"`
	StreamingStopConfig *parsedStreamingConfig `json:"streaming_stop_conditions,omitempty"`
}

type parsedStopCondition struct {
	ID             string            `json:"id"`
	Metric         string            `json:"metric"`
	Comparator     string            `json:"comparator"`
	Threshold      float64           `json:"threshold"`
	WindowMs       int64             `json:"window_ms"`
	SustainWindows int               `json:"sustain_windows"`
	Scope          map[string]string `json:"scope"`
}

type parsedLoad struct {
	TargetVUs  int `json:"target_vus"`
	StartVUs   int `json:"start_vus,omitempty"`    // Starting VUs for ramp (default: 10% of target)
	RampSteps  int `json:"ramp_steps,omitempty"`   // Number of steps to reach target (default: 5)
	StepHoldMs int `json:"step_hold_ms,omitempty"` // How long to hold each step (default: duration/steps)
}

type parsedWorkload struct {
	OpMix        []parsedOpMixEntry `json:"op_mix"`
	OperationMix []parsedOpMixEntry `json:"operation_mix"`
	Tools        *parsedToolsConfig `json:"tools,omitempty"`
}

type parsedToolsConfig struct {
	Selection parsedToolSelection  `json:"selection"`
	Templates []parsedToolTemplate `json:"templates"`
}

type parsedToolSelection struct {
	Mode string `json:"mode"`
}

type parsedToolTemplate struct {
	TemplateID string                 `json:"template_id"`
	ToolName   string                 `json:"tool_name"`
	Weight     int                    `json:"weight"`
	Arguments  map[string]interface{} `json:"arguments,omitempty"`
}

type parsedOpMixEntry struct {
	Operation  string                 `json:"operation"`
	Weight     int                    `json:"weight"`
	ToolName   string                 `json:"tool_name,omitempty"`
	Arguments  map[string]interface{} `json:"arguments,omitempty"`
	URI        string                 `json:"uri,omitempty"`
	PromptName string                 `json:"prompt_name,omitempty"`
}

type parsedSessionPolicy struct {
	Mode      string `json:"mode"`
	PoolSize  int    `json:"pool_size,omitempty"`
	TTLMs     int64  `json:"ttl_ms,omitempty"`
	MaxIdleMs int64  `json:"max_idle_ms,omitempty"`
}

type parsedSafety struct {
	HardCaps          parsedHardCaps   `json:"hard_caps"`
	StopPolicy        parsedStopPolicy `json:"stop_policy"`
	AnalysisTimeoutMs int64            `json:"analysis_timeout_ms"`
}

type parsedStopPolicy struct {
	Mode           string `json:"mode"`
	DrainTimeoutMs int64  `json:"drain_timeout_ms"`
}

type parsedHardCaps struct {
	MaxVUs        int   `json:"max_vus"`
	MaxDurationMs int64 `json:"max_duration_ms"`
	MaxErrors     int   `json:"max_errors"`
}

func parseRunConfig(config []byte) (*parsedRunConfig, error) {
	var parsed parsedRunConfig
	if err := json.Unmarshal(config, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse run config: %w", err)
	}

	if len(parsed.Workload.OpMix) == 0 && len(parsed.Workload.OperationMix) > 0 {
		parsed.Workload.OpMix = parsed.Workload.OperationMix
	}

	for i := range parsed.Workload.OpMix {
		parsed.Workload.OpMix[i].Operation = normalizeOperationName(parsed.Workload.OpMix[i].Operation)
	}

	parsed.Workload.OpMix = expandToolsTemplates(parsed.Workload.OpMix, parsed.Workload.Tools)

	return &parsed, nil
}

func expandToolsTemplates(opMix []parsedOpMixEntry, tools *parsedToolsConfig) []parsedOpMixEntry {
	if tools == nil || len(tools.Templates) == 0 {
		return opMix
	}

	var expanded []parsedOpMixEntry

	for _, op := range opMix {
		if op.Operation == "tools/call" && op.ToolName == "" {
			for _, tmpl := range tools.Templates {
				expanded = append(expanded, parsedOpMixEntry{
					Operation: "tools/call",
					Weight:    tmpl.Weight,
					ToolName:  tmpl.ToolName,
					Arguments: tmpl.Arguments,
				})
			}
		} else {
			expanded = append(expanded, op)
		}
	}

	return expanded
}

func normalizeOperationName(op string) string {
	switch op {
	case "tools_list":
		return "tools/list"
	case "tools_call":
		return "tools/call"
	case "resources_list":
		return "resources/list"
	case "resources_read":
		return "resources/read"
	case "prompts_list":
		return "prompts/list"
	case "prompts_get":
		return "prompts/get"
	case "initialize":
		return "initialize"
	case "ping":
		return "ping"
	default:
		return op
	}
}

func buildTargetHeaders(runID string, target *parsedTarget) map[string]string {
	headers := make(map[string]string)

	for k, v := range target.Headers {
		headers[k] = v
	}

	if target.Identification != nil {
		if target.Identification.RunIDHeader != nil {
			name := target.Identification.RunIDHeader.Name
			if name == "" {
				name = "X-Test-Run-Id"
			}
			value := target.Identification.RunIDHeader.ValueTemplate
			value = strings.ReplaceAll(value, "${run_id}", runID)
			headers[name] = value
		}

		if target.Identification.UserAgent != nil {
			value := target.Identification.UserAgent.Value
			value = strings.ReplaceAll(value, "${run_id}", runID)
			headers["User-Agent"] = value
		}
	}

	return headers
}

func buildRedirectPolicy(policy *parsedRedirectPolicy) *types.RedirectPolicyConfig {
	if policy == nil {
		return nil
	}
	return &types.RedirectPolicyConfig{
		Mode:         policy.Mode,
		MaxRedirects: policy.MaxRedirects,
		Allowlist:    policy.Allowlist,
	}
}

func buildAuthConfig(auth *parsedAuth) *types.AuthConfig {
	if auth == nil || auth.Type == "" || auth.Type == "none" {
		return nil
	}
	return &types.AuthConfig{
		Type:   auth.Type,
		Tokens: auth.Tokens,
	}
}

func findStageByName(config *parsedRunConfig, stageName StageName) *parsedStage {
	for i := range config.Stages {
		if config.Stages[i].Stage == string(stageName) && config.Stages[i].Enabled {
			return &config.Stages[i]
		}
	}
	return nil
}

const (
	DefaultDrainTimeoutMs    = 30000
	DefaultAnalysisTimeoutMs = 1800000
)

func getDrainTimeout(config []byte) time.Duration {
	parsed, err := parseRunConfig(config)
	if err != nil || parsed.Safety.StopPolicy.DrainTimeoutMs <= 0 {
		return time.Duration(DefaultDrainTimeoutMs) * time.Millisecond
	}
	return time.Duration(parsed.Safety.StopPolicy.DrainTimeoutMs) * time.Millisecond
}

func getAnalysisTimeout(config []byte) time.Duration {
	parsed, err := parseRunConfig(config)
	if err != nil || parsed.Safety.AnalysisTimeoutMs <= 0 {
		return time.Duration(DefaultAnalysisTimeoutMs) * time.Millisecond
	}
	return time.Duration(parsed.Safety.AnalysisTimeoutMs) * time.Millisecond
}

func convertOpMix(entries []parsedOpMixEntry) []types.OpMixEntry {
	result := make([]types.OpMixEntry, len(entries))
	for i, e := range entries {
		result[i] = types.OpMixEntry{
			Operation:  e.Operation,
			Weight:     e.Weight,
			ToolName:   e.ToolName,
			Arguments:  e.Arguments,
			URI:        e.URI,
			PromptName: e.PromptName,
		}
	}
	return result
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

		select {
		case <-time.After(drainTimeout):
		case <-drainCancel:
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
	_ = eventLog.Append(event)
}
