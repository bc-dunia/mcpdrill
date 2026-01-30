package runmanager

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/controlplane/scheduler"
	"github.com/bc-dunia/mcpdrill/internal/types"
)

// WorkerFailurePolicy represents the policy for handling worker failures.
type WorkerFailurePolicy string

const (
	// PolicyFailFast immediately stops the run when a worker fails.
	PolicyFailFast WorkerFailurePolicy = "fail_fast"
	// PolicyReplaceIfPossible attempts to reallocate work to other workers.
	PolicyReplaceIfPossible WorkerFailurePolicy = "replace_if_possible"
	// PolicyBestEffort continues with reduced capacity when a worker fails.
	PolicyBestEffort WorkerFailurePolicy = "best_effort"
)

// ValidWorkerFailurePolicies contains all valid worker failure policy values.
var ValidWorkerFailurePolicies = []WorkerFailurePolicy{
	PolicyFailFast,
	PolicyReplaceIfPossible,
	PolicyBestEffort,
}

// IsValidWorkerFailurePolicy checks if a policy string is valid.
func IsValidWorkerFailurePolicy(policy string) bool {
	for _, valid := range ValidWorkerFailurePolicies {
		if policy == string(valid) {
			return true
		}
	}
	return false
}

// parsedSafetyConfig represents the safety section of run config.
type parsedSafetyConfig struct {
	WorkerFailurePolicy string `json:"worker_failure_policy"`
}

// getWorkerFailurePolicy extracts the worker_failure_policy from run config.
// Returns "fail_fast" as default if not specified.
func getWorkerFailurePolicy(config []byte) WorkerFailurePolicy {
	var parsed struct {
		Safety parsedSafetyConfig `json:"safety"`
	}
	if err := json.Unmarshal(config, &parsed); err != nil {
		return PolicyFailFast
	}
	if parsed.Safety.WorkerFailurePolicy == "" {
		return PolicyFailFast
	}
	return WorkerFailurePolicy(parsed.Safety.WorkerFailurePolicy)
}

// HandleWorkerCapacityLost handles a worker capacity loss event according to the
// configured worker_failure_policy. This method is called by the heartbeat monitor
// when a worker times out.
//
// Policies:
//   - fail_fast: Immediately transition run to STOPPING state
//   - replace_if_possible: Attempt reallocation (stub for now, falls back to fail_fast)
//   - best_effort: Log warning and continue with reduced capacity
func (rm *RunManager) HandleWorkerCapacityLost(runID string, workerID string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	record, ok := rm.runs[runID]
	if !ok {
		return fmt.Errorf("run not found: %s", runID)
	}

	// Per ref/11-state-machine.md: If worker lost during STOPPING, immediately trigger drain timeout
	if record.State == RunStateStopping {
		log.Printf("[RunManager] Worker %s lost during STOPPING, triggering immediate drain timeout", workerID)
		if record.drainCancel != nil {
			close(record.drainCancel)
			record.drainCancel = nil
		}
		return nil
	}

	// Only handle if run is in a running state (not already stopping/stopped)
	if !isRunningState(record.State) {
		log.Printf("[RunManager] Ignoring worker capacity loss for run %s in state %s", runID, record.State)
		return nil
	}

	// Per ref/11-state-machine.md: Preflight ALWAYS uses fail_fast regardless of config.
	// This ensures any worker loss during preflight immediately stops the run.
	if record.State == RunStatePreflightRunning {
		log.Printf("[RunManager] Worker %s lost during preflight, forcing fail_fast policy", workerID)
		return rm.handleFailFastLocked(record, workerID)
	}

	policy := getWorkerFailurePolicy(record.Config)

	switch policy {
	case PolicyFailFast:
		return rm.handleFailFastLocked(record, workerID)
	case PolicyReplaceIfPossible:
		return rm.handleReplaceIfPossibleLocked(record, workerID)
	case PolicyBestEffort:
		return rm.handleBestEffortLocked(record, workerID)
	default:
		// Unknown policy, fall back to fail_fast for safety
		log.Printf("[RunManager] Unknown worker_failure_policy '%s', falling back to fail_fast", policy)
		return rm.handleFailFastLocked(record, workerID)
	}
}

func isRunningState(state RunState) bool {
	switch state {
	case RunStatePreflightRunning, RunStateBaselineRunning, RunStateRampRunning, RunStateSoakRunning:
		return true
	default:
		return false
	}
}

// handleFailFastLocked handles worker failure with fail_fast policy.
// Must be called with rm.mu held. Starts drain/finalization goroutine.
func (rm *RunManager) handleFailFastLocked(record *RunRecord, workerID string) error {
	log.Printf("[RunManager] Worker %s lost, stopping run %s (fail_fast policy)", workerID, record.RunID)

	rm.cancelStageProgressionLocked(record)
	rm.stopStopConditionEvaluatorLocked(record)

	oldState := record.State
	record.State = RunStateStopping
	record.UpdatedAtMs = time.Now().UnixMilli()
	record.StopReason = &StopReason{
		Mode:   StopModeImmediate,
		Reason: fmt.Sprintf("worker_failure: worker %s lost", workerID),
		Actor:  string(ActorSystem),
		AtMs:   record.UpdatedAtMs,
	}

	// Initialize drainCancel when entering STOPPING to avoid race with HandleWorkerCapacityLost
	if record.drainCancel == nil {
		record.drainCancel = make(chan struct{})
	}

	eventLog := rm.eventLogs[record.RunID]

	stopPayload, _ := json.Marshal(map[string]interface{}{
		"mode":      StopModeImmediate,
		"actor":     ActorSystem,
		"reason":    "worker_failure",
		"worker_id": workerID,
		"policy":    PolicyFailFast,
	})
	stopEvent := RunEvent{
		RunID:       record.RunID,
		ExecutionID: record.ExecutionID,
		Type:        EventTypeStopRequested,
		Actor:       ActorSystem,
		Payload:     stopPayload,
		Evidence: []Evidence{
			{Kind: "worker", Ref: workerID, Note: stringPtr("worker heartbeat timeout")},
		},
	}
	_ = eventLog.Append(stopEvent)

	transitionPayload, _ := json.Marshal(map[string]interface{}{
		"from_state": oldState,
		"to_state":   record.State,
		"trigger":    "worker_failure",
		"actor":      ActorSystem,
		"worker_id":  workerID,
		"policy":     PolicyFailFast,
	})
	transitionEvent := RunEvent{
		RunID:       record.RunID,
		ExecutionID: record.ExecutionID,
		Type:        EventTypeStateTransition,
		Actor:       ActorSystem,
		Payload:     transitionPayload,
		Evidence:    []Evidence{},
	}
	_ = eventLog.Append(transitionEvent)

	runID := record.RunID
	configCopy := make([]byte, len(record.Config))
	copy(configCopy, record.Config)
	drainTimeout := getDrainTimeout(configCopy)

	go rm.finalizeRun(runID, drainTimeout, string(ActorSystem))

	return nil
}

func (rm *RunManager) handleReplaceIfPossibleLocked(record *RunRecord, workerID string) error {
	log.Printf("[RunManager] Worker %s lost, attempting reallocation (replace_if_possible policy)", workerID)

	eventLog := rm.eventLogs[record.RunID]

	if record.ActiveStage == nil {
		log.Printf("[RunManager] No active stage, falling back to fail_fast")
		rm.emitReallocationFailedDecision(eventLog, record, workerID, "no_active_stage")
		return rm.handleFailFastLocked(record, workerID)
	}

	stageID := record.ActiveStage.StageID

	parsedConfig, err := parseRunConfig(record.Config)
	if err != nil {
		log.Printf("[RunManager] Failed to parse config: %v, falling back to fail_fast", err)
		rm.emitReallocationFailedDecision(eventLog, record, workerID, "config_parse_error")
		return rm.handleFailFastLocked(record, workerID)
	}

	var targetVUs int
	for _, stage := range parsedConfig.Stages {
		if stage.StageID == stageID {
			targetVUs = stage.Load.TargetVUs
			break
		}
	}

	if targetVUs == 0 {
		log.Printf("[RunManager] Target VUs not found for stage %s, falling back to fail_fast", stageID)
		rm.emitReallocationFailedDecision(eventLog, record, workerID, "target_vus_not_found")
		return rm.handleFailFastLocked(record, workerID)
	}

	if parsedConfig.Safety.HardCaps.MaxVUs > 0 && targetVUs > parsedConfig.Safety.HardCaps.MaxVUs {
		log.Printf("[RunManager] Capping reallocation target VUs from %d to hard cap %d", targetVUs, parsedConfig.Safety.HardCaps.MaxVUs)
		targetVUs = parsedConfig.Safety.HardCaps.MaxVUs
	}

	if rm.allocator == nil {
		log.Printf("[RunManager] Allocator not configured, falling back to fail_fast")
		rm.emitReallocationFailedDecision(eventLog, record, workerID, "allocator_not_configured")
		return rm.handleFailFastLocked(record, workerID)
	}

	assignments, workerAssignments, err := rm.allocator.ReallocateAssignments(
		record.RunID,
		stageID,
		targetVUs,
		[]scheduler.WorkerID{scheduler.WorkerID(workerID)},
	)

	if err != nil {
		log.Printf("[RunManager] Reallocation failed: %v, falling back to fail_fast", err)
		rm.emitReallocationFailedDecision(eventLog, record, workerID, err.Error())
		return rm.handleFailFastLocked(record, workerID)
	}

	if rm.leaseManager == nil || rm.assignmentSender == nil {
		log.Printf("[RunManager] Lease manager or assignment sender not configured, falling back to fail_fast")
		rm.emitReallocationFailedDecision(eventLog, record, workerID, "scheduler_not_configured")
		return rm.handleFailFastLocked(record, workerID)
	}

	for wid, assignment := range workerAssignments {
		leaseID, err := rm.leaseManager.IssueLease(wid, assignment)
		if err != nil {
			log.Printf("[RunManager] Failed to issue lease for worker %s: %v", wid, err)
			continue
		}

		workerAssignment := types.WorkerAssignment{
			RunID:       assignment.RunID,
			ExecutionID: record.ExecutionID,
			StageID:     assignment.StageID,
			Stage:       string(record.ActiveStage.Stage),
			LeaseID:     string(leaseID),
			VUIDStart:   assignment.VUIDRange.Start,
			VUIDEnd:     assignment.VUIDRange.End,
			DurationMs:  rm.getStageDuration(parsedConfig, stageID),
			Target: types.TargetConfig{
				URL:            parsedConfig.Target.URL,
				Transport:      parsedConfig.Target.Transport,
				Headers:        buildTargetHeaders(record.RunID, &parsedConfig.Target),
				RedirectPolicy: buildRedirectPolicy(parsedConfig.Target.RedirectPolicy),
				Auth:           buildAuthConfig(parsedConfig.Target.Auth),
			},
			Workload: types.WorkloadConfig{
				OpMix: convertOpMix(parsedConfig.Workload.OpMix),
			},
			SessionPolicy: types.SessionPolicyConfig{
				Mode:      parsedConfig.SessionPolicy.Mode,
				PoolSize:  parsedConfig.SessionPolicy.PoolSize,
				TTLMs:     parsedConfig.SessionPolicy.TTLMs,
				MaxIdleMs: parsedConfig.SessionPolicy.MaxIdleMs,
			},
		}

		rm.assignmentSender.AddAssignment(string(wid), workerAssignment)

		rm.emitWorkerAssignedEvent(record.RunID, record.ExecutionID, eventLog, string(wid), string(leaseID),
			assignment.VUIDRange.Start, assignment.VUIDRange.End, stageID, StageName(record.ActiveStage.Stage))

		log.Printf("[RunManager] Reassigned VUs [%d, %d) to worker %s with lease %s",
			assignment.VUIDRange.Start, assignment.VUIDRange.End, wid, leaseID)
	}

	replacedPayload, _ := json.Marshal(map[string]interface{}{
		"lost_worker":     workerID,
		"new_assignments": len(assignments),
		"target_vus":      targetVUs,
		"stage_id":        stageID,
		"policy":          PolicyReplaceIfPossible,
	})
	replacedEvent := RunEvent{
		RunID:       record.RunID,
		ExecutionID: record.ExecutionID,
		Type:        EventTypeWorkerReplaced,
		Actor:       ActorSystem,
		Payload:     replacedPayload,
		Evidence: []Evidence{
			{Kind: "worker", Ref: workerID, Note: stringPtr("worker replaced via reallocation")},
		},
	}
	_ = eventLog.Append(replacedEvent)

	decisionPayload, _ := json.Marshal(map[string]interface{}{
		"decision_type": "reallocation_success",
		"policy":        PolicyReplaceIfPossible,
		"lost_worker":   workerID,
		"assignments":   len(assignments),
	})
	decisionEvent := RunEvent{
		RunID:       record.RunID,
		ExecutionID: record.ExecutionID,
		Type:        EventTypeDecision,
		Actor:       ActorSystem,
		Payload:     decisionPayload,
		Evidence:    []Evidence{},
	}
	_ = eventLog.Append(decisionEvent)

	log.Printf("[RunManager] Worker %s replaced, %d new assignments issued", workerID, len(assignments))
	return nil
}

func (rm *RunManager) emitReallocationFailedDecision(eventLog *EventLog, record *RunRecord, workerID, reason string) {
	decisionPayload, _ := json.Marshal(map[string]interface{}{
		"decision_type": "reallocation_failed",
		"policy":        PolicyReplaceIfPossible,
		"fallback":      PolicyFailFast,
		"reason":        reason,
		"worker_id":     workerID,
	})
	decisionEvent := RunEvent{
		RunID:       record.RunID,
		ExecutionID: record.ExecutionID,
		Type:        EventTypeDecision,
		Actor:       ActorSystem,
		Payload:     decisionPayload,
		Evidence: []Evidence{
			{Kind: "worker", Ref: workerID, Note: stringPtr("worker heartbeat timeout")},
		},
	}
	_ = eventLog.Append(decisionEvent)
}

func (rm *RunManager) getStageDuration(config *parsedRunConfig, stageID string) int64 {
	for _, stage := range config.Stages {
		if stage.StageID == stageID {
			return stage.DurationMs
		}
	}
	return 0
}

// handleBestEffortLocked handles worker failure with best_effort policy.
// Logs a warning and continues with reduced capacity.
// Must be called with rm.mu held.
func (rm *RunManager) handleBestEffortLocked(record *RunRecord, workerID string) error {
	log.Printf("[RunManager] Worker %s lost, continuing with reduced capacity (best_effort policy)", workerID)

	eventLog := rm.eventLogs[record.RunID]

	// Emit WORKER_CAPACITY_LOST event
	capacityPayload, _ := json.Marshal(map[string]interface{}{
		"worker_id": workerID,
		"policy":    PolicyBestEffort,
		"action":    "continue",
		"reason":    "worker_heartbeat_timeout",
	})
	capacityEvent := RunEvent{
		RunID:       record.RunID,
		ExecutionID: record.ExecutionID,
		Type:        EventTypeWorkerCapacityLost,
		Actor:       ActorSystem,
		Payload:     capacityPayload,
		Evidence: []Evidence{
			{Kind: "worker", Ref: workerID, Note: stringPtr("worker heartbeat timeout, continuing with reduced capacity")},
		},
	}
	_ = eventLog.Append(capacityEvent)

	// Emit SYSTEM_WARNING event
	warningPayload, _ := json.Marshal(map[string]interface{}{
		"warning":   "worker_capacity_reduced",
		"worker_id": workerID,
		"policy":    PolicyBestEffort,
		"message":   fmt.Sprintf("Worker %s lost, run continuing with reduced capacity", workerID),
	})
	warningEvent := RunEvent{
		RunID:       record.RunID,
		ExecutionID: record.ExecutionID,
		Type:        EventTypeSystemWarning,
		Actor:       ActorSystem,
		Payload:     warningPayload,
		Evidence:    []Evidence{},
	}
	_ = eventLog.Append(warningEvent)

	return nil
}

// GetRunsForWorker returns all run IDs that have active assignments for the given worker.
// This is used by the heartbeat monitor to determine which runs are affected by worker loss.
func (rm *RunManager) GetRunsForWorker(workerID string) []string {
	rm.mu.RLock()
	leaseManager := rm.leaseManager
	rm.mu.RUnlock()

	if leaseManager == nil {
		return nil
	}

	// Use lease-backed mapping to get only runs with active leases for this worker
	workerRunIDs := leaseManager.ListWorkerRunIDs(scheduler.WorkerID(workerID))

	// Filter to only include runs in running states
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	var runIDs []string
	for _, runID := range workerRunIDs {
		if record, ok := rm.runs[runID]; ok {
			if isRunningState(record.State) {
				runIDs = append(runIDs, runID)
			}
		}
	}
	return runIDs
}
