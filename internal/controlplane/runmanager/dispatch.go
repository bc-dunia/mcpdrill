package runmanager

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/controlplane/scheduler"
	"github.com/bc-dunia/mcpdrill/internal/types"
)

// tryAllocateForStage attempts allocation without dispatching assignments.
// Returns true if allocation succeeds, false otherwise.
func (rm *RunManager) tryAllocateForStage(runID, executionID string, config []byte, eventLog *EventLog, stageName StageName) bool {
	log.Printf("[RunManager] Attempting allocation for %s stage of run %s", stageName, runID)

	parsedConfig, err := parseRunConfig(config)
	if err != nil {
		log.Printf("[RunManager] Failed to parse config for run %s: %v", runID, err)
		rm.emitAllocationFailedEvent(runID, executionID, eventLog, "config_parse_error", err.Error())
		return false
	}

	stage := findStageByName(parsedConfig, stageName)
	if stage == nil {
		log.Printf("[RunManager] No enabled %s stage found for run %s", stageName, runID)
		rm.emitAllocationFailedEvent(runID, executionID, eventLog, "stage_not_found", fmt.Sprintf("no enabled %s stage found", stageName))
		return false
	}

	targetVUs := stage.Load.TargetVUs
	if targetVUs <= 0 {
		log.Printf("[RunManager] Invalid target VUs (%d) for run %s", targetVUs, runID)
		rm.emitAllocationFailedEvent(runID, executionID, eventLog, "invalid_target_vus", fmt.Sprintf("target_vus must be > 0, got %d", targetVUs))
		return false
	}

	if parsedConfig.Safety.HardCaps.MaxVUs > 0 && targetVUs > parsedConfig.Safety.HardCaps.MaxVUs {
		targetVUs = parsedConfig.Safety.HardCaps.MaxVUs
	}

	rm.mu.RLock()
	registry := rm.registry
	allocator := rm.allocator
	rm.mu.RUnlock()

	if registry == nil || allocator == nil {
		log.Printf("[RunManager] Scheduler components not configured for run %s", runID)
		return false
	}

	workers := registry.ListWorkers()
	if len(workers) == 0 {
		log.Printf("[RunManager] No workers available for run %s", runID)
		rm.emitAllocationFailedEvent(runID, executionID, eventLog, "no_workers", "no workers registered")
		return false
	}

	workerIDs := make([]scheduler.WorkerID, len(workers))
	for i, w := range workers {
		workerIDs[i] = w.WorkerID
	}

	_, _, err = allocator.AllocateAssignments(runID, stage.StageID, targetVUs, workerIDs)
	if err != nil {
		log.Printf("[RunManager] Allocation failed for run %s: %v", runID, err)
		rm.emitAllocationFailedEvent(runID, executionID, eventLog, "allocation_error", err.Error())
		return false
	}

	return true
}

// createAndDispatchAssignmentsForStage handles the assignment creation flow after run starts.
func (rm *RunManager) createAndDispatchAssignmentsForStage(runID, executionID string, config []byte, eventLog *EventLog, stageName StageName) *parsedStage {
	log.Printf("[RunManager] Creating %s assignments for run %s", stageName, runID)

	parsedConfig, err := parseRunConfig(config)
	if err != nil {
		log.Printf("[RunManager] Failed to parse config for run %s: %v", runID, err)
		rm.emitAllocationFailedEvent(runID, executionID, eventLog, "config_parse_error", err.Error())
		return nil
	}

	stage := findStageByName(parsedConfig, stageName)
	if stage == nil {
		log.Printf("[RunManager] No enabled %s stage found for run %s", stageName, runID)
		rm.emitAllocationFailedEvent(runID, executionID, eventLog, "stage_not_found", fmt.Sprintf("no enabled %s stage found", stageName))
		return nil
	}

	targetVUs := stage.Load.TargetVUs
	if targetVUs <= 0 {
		log.Printf("[RunManager] Invalid target VUs (%d) for run %s", targetVUs, runID)
		rm.emitAllocationFailedEvent(runID, executionID, eventLog, "invalid_target_vus", fmt.Sprintf("target_vus must be > 0, got %d", targetVUs))
		return nil
	}

	// Enforce hard cap on max VUs
	if parsedConfig.Safety.HardCaps.MaxVUs > 0 && targetVUs > parsedConfig.Safety.HardCaps.MaxVUs {
		log.Printf("[RunManager] Capping target VUs from %d to hard cap %d for run %s", targetVUs, parsedConfig.Safety.HardCaps.MaxVUs, runID)
		targetVUs = parsedConfig.Safety.HardCaps.MaxVUs
	}

	rm.mu.RLock()
	registry := rm.registry
	allocator := rm.allocator
	leaseManager := rm.leaseManager
	assignmentSender := rm.assignmentSender
	rm.mu.RUnlock()

	if registry == nil || allocator == nil || leaseManager == nil || assignmentSender == nil {
		log.Printf("[RunManager] Scheduler components not configured for run %s", runID)
		return nil
	}

	if err := leaseManager.RevokeLeasesByRun(runID); err != nil {
		log.Printf("[RunManager] Failed to revoke existing leases for run %s: %v", runID, err)
	}

	workers := registry.ListWorkers()
	if len(workers) == 0 {
		log.Printf("[RunManager] No workers available for run %s", runID)
		rm.emitAllocationFailedEvent(runID, executionID, eventLog, "no_workers", "no workers registered")
		return nil
	}

	workerIDs := make([]scheduler.WorkerID, len(workers))
	for i, w := range workers {
		workerIDs[i] = w.WorkerID
	}

	log.Printf("[RunManager] Allocating %d VUs across %d workers for run %s", targetVUs, len(workers), runID)

	_, workerAssignmentsMap, err := allocator.AllocateAssignments(runID, stage.StageID, targetVUs, workerIDs)
	if err != nil {
		log.Printf("[RunManager] Allocation failed for run %s: %v", runID, err)
		rm.emitAllocationFailedEvent(runID, executionID, eventLog, "allocation_error", err.Error())
		return nil
	}

	log.Printf("[RunManager] Created %d assignments for run %s", len(workerAssignmentsMap), runID)

	for workerID, assignment := range workerAssignmentsMap {
		leaseID, err := leaseManager.IssueLease(workerID, assignment)
		if err != nil {
			log.Printf("[RunManager] Failed to issue lease for worker %s: %v", workerID, err)
			rm.emitAllocationFailedEvent(runID, executionID, eventLog, "lease_error", fmt.Sprintf("worker %s: %v", workerID, err))
			continue
		}

		workerAssignment := types.WorkerAssignment{
			RunID:       assignment.RunID,
			ExecutionID: executionID,
			Stage:       string(stageName),
			StageID:     assignment.StageID,
			LeaseID:     string(leaseID),
			VUIDStart:   assignment.VUIDRange.Start,
			VUIDEnd:     assignment.VUIDRange.End,
			DurationMs:  stage.DurationMs,
			Target: types.TargetConfig{
				URL:            parsedConfig.Target.URL,
				Transport:      parsedConfig.Target.Transport,
				Headers:        buildTargetHeaders(runID, &parsedConfig.Target),
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

		assignmentSender.AddAssignment(string(workerID), workerAssignment)

		rm.emitWorkerAssignedEvent(runID, executionID, eventLog, string(workerID), string(leaseID), assignment.VUIDRange.Start, assignment.VUIDRange.End, stage.StageID, stageName)

		log.Printf("[RunManager] Assigned VUs [%d, %d) to worker %s with lease %s", assignment.VUIDRange.Start, assignment.VUIDRange.End, workerID, leaseID)
	}

	log.Printf("[RunManager] Assignment dispatch complete for run %s", runID)
	return stage
}

func (rm *RunManager) emitAllocationFailedEvent(runID, executionID string, eventLog *EventLog, reason, details string) {
	payload, _ := json.Marshal(map[string]interface{}{
		"reason":  reason,
		"details": details,
	})

	event := RunEvent{
		RunID:       runID,
		ExecutionID: executionID,
		Type:        EventTypeAllocationFailed,
		Actor:       ActorScheduler,
		Payload:     payload,
		Evidence:    []Evidence{},
	}
	_ = eventLog.Append(event)
}

func (rm *RunManager) emitWorkerAssignedEvent(runID, executionID string, eventLog *EventLog, workerID, leaseID string, vuStart, vuEnd int, stageID string, stageName StageName) {
	payload, _ := json.Marshal(map[string]interface{}{
		"worker_id": workerID,
		"lease_id":  leaseID,
		"vu_start":  vuStart,
		"vu_end":    vuEnd,
		"stage_id":  stageID,
	})

	event := RunEvent{
		RunID:       runID,
		ExecutionID: executionID,
		Type:        EventTypeWorkerAssigned,
		Actor:       ActorScheduler,
		Correlation: CorrelationContext{
			Stage:    &stageName,
			StageID:  &stageID,
			WorkerID: &workerID,
		},
		Payload:  payload,
		Evidence: []Evidence{},
	}
	_ = eventLog.Append(event)
}

func (rm *RunManager) dispatchRampAssignments(runID, executionID string, config []byte, eventLog *EventLog, stage *parsedStage, parsedConfig *parsedRunConfig, numVUs int, vuOffset int) {
	rm.mu.RLock()
	registry := rm.registry
	allocator := rm.allocator
	leaseManager := rm.leaseManager
	assignmentSender := rm.assignmentSender
	rm.mu.RUnlock()

	if registry == nil || allocator == nil || leaseManager == nil || assignmentSender == nil {
		return
	}

	workers := registry.ListWorkers()
	if len(workers) == 0 {
		log.Printf("[RunManager] No workers available for ramp assignments")
		return
	}

	workerIDs := make([]scheduler.WorkerID, len(workers))
	for i, w := range workers {
		workerIDs[i] = w.WorkerID
	}

	_, workerAssignmentsMap, err := allocator.AllocateAssignments(runID, stage.StageID, numVUs, workerIDs)
	if err != nil {
		log.Printf("[RunManager] Ramp allocation failed: %v", err)
		return
	}

	remainingDurationMs := stage.DurationMs
	rm.mu.RLock()
	if record, ok := rm.runs[runID]; ok && record.ActiveStage != nil {
		elapsed := time.Now().UnixMilli() - record.UpdatedAtMs
		remainingDurationMs = stage.DurationMs - elapsed
		if remainingDurationMs < 1000 {
			remainingDurationMs = 1000
		}
	}
	rm.mu.RUnlock()

	for workerID, assignment := range workerAssignmentsMap {
		offsetAssignment := scheduler.Assignment{
			RunID:   assignment.RunID,
			StageID: assignment.StageID,
			VUIDRange: scheduler.VUIDRange{
				Start: assignment.VUIDRange.Start + vuOffset,
				End:   assignment.VUIDRange.End + vuOffset,
			},
		}

		leaseID, err := leaseManager.IssueLease(workerID, offsetAssignment)
		if err != nil {
			log.Printf("[RunManager] Failed to issue lease for VU range [%d, %d): %v",
				offsetAssignment.VUIDRange.Start, offsetAssignment.VUIDRange.End, err)
			continue
		}

		workerAssignment := types.WorkerAssignment{
			RunID:       offsetAssignment.RunID,
			ExecutionID: executionID,
			Stage:       string(StageNameRamp),
			StageID:     offsetAssignment.StageID,
			LeaseID:     string(leaseID),
			VUIDStart:   offsetAssignment.VUIDRange.Start,
			VUIDEnd:     offsetAssignment.VUIDRange.End,
			DurationMs:  remainingDurationMs,
			Target: types.TargetConfig{
				URL:            parsedConfig.Target.URL,
				Transport:      parsedConfig.Target.Transport,
				Headers:        buildTargetHeaders(runID, &parsedConfig.Target),
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

		assignmentSender.AddAssignment(string(workerID), workerAssignment)

		rm.emitWorkerAssignedEvent(runID, executionID, eventLog, string(workerID), string(leaseID),
			offsetAssignment.VUIDRange.Start, offsetAssignment.VUIDRange.End, stage.StageID, StageNameRamp)
	}
}

func (rm *RunManager) emitRampStepEvent(runID, executionID string, eventLog *EventLog, step, currentVUs, targetVUs int, trigger string) {
	payload, _ := json.Marshal(map[string]interface{}{
		"step":        step,
		"current_vus": currentVUs,
		"target_vus":  targetVUs,
		"trigger":     trigger,
	})

	stageName := StageNameRamp
	event := RunEvent{
		RunID:       runID,
		ExecutionID: executionID,
		Type:        EventTypeDecision,
		Actor:       ActorAutoramp,
		Correlation: CorrelationContext{
			Stage: &stageName,
		},
		Payload:  payload,
		Evidence: []Evidence{},
	}
	_ = eventLog.Append(event)
}
