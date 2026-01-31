package runmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/analysis"
	"github.com/bc-dunia/mcpdrill/internal/controlplane/stopconditions"
	"github.com/bc-dunia/mcpdrill/internal/events"
	"github.com/bc-dunia/mcpdrill/internal/telemetry"
)

const (
	DefaultPreflightMaxDurationMs = 600000   // 10 minutes
	DefaultBaselineMaxDurationMs  = 1800000  // 30 minutes
	DefaultRampMaxDurationMs      = 7200000  // 2 hours
	DefaultSoakMaxDurationMs      = 86400000 // 24 hours
)

func getStageMaxDuration(stage *parsedStage) time.Duration {
	if stage.MaxDurationMs > 0 {
		return time.Duration(stage.MaxDurationMs) * time.Millisecond
	}
	switch StageName(stage.Stage) {
	case StageNamePreflight:
		return time.Duration(DefaultPreflightMaxDurationMs) * time.Millisecond
	case StageNameBaseline:
		return time.Duration(DefaultBaselineMaxDurationMs) * time.Millisecond
	case StageNameRamp:
		return time.Duration(DefaultRampMaxDurationMs) * time.Millisecond
	case StageNameSoak:
		return time.Duration(DefaultSoakMaxDurationMs) * time.Millisecond
	default:
		return time.Duration(DefaultRampMaxDurationMs) * time.Millisecond
	}
}

func getEffectiveStageDuration(stage *parsedStage) time.Duration {
	return time.Duration(stage.DurationMs) * time.Millisecond
}

func stopTimer(timer *time.Timer) {
	if timer == nil {
		return
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}

func (rm *RunManager) startStageProgression(runID string, config []byte, actor string) {
	parsedConfig, err := parseRunConfig(config)
	if err != nil {
		log.Printf("[RunManager] Stage progression disabled for run %s: %v", runID, err)
		return
	}

	preflightStage := findStageByName(parsedConfig, StageNamePreflight)
	baselineStage := findStageByName(parsedConfig, StageNameBaseline)
	rampStage := findStageByName(parsedConfig, StageNameRamp)
	if preflightStage == nil || baselineStage == nil || rampStage == nil {
		log.Printf("[RunManager] Stage progression disabled for run %s: missing required stages", runID)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())

	rm.mu.Lock()
	record, ok := rm.runs[runID]
	if !ok {
		rm.mu.Unlock()
		cancel()
		return
	}
	record.progressionCancel = cancel
	record.progressionTimers = nil
	record.ActiveStage = &ActiveStageInfo{Stage: string(StageNamePreflight), StageID: preflightStage.StageID}
	record.UpdatedAtMs = time.Now().UnixMilli()
	rm.mu.Unlock()

	go func() {
		if !rm.waitForStageDurationWithTimeout(ctx, runID, preflightStage, actor) {
			return
		}
		if err := rm.TransitionToBaseline(runID, actor); err != nil {
			log.Printf("[RunManager] Failed to transition run %s to baseline: %v, stopping run", runID, err)
			_ = rm.requestStopWithReason(runID, StopModeImmediate, string(ActorSystem), "preflight_passed_timeout", nil)
			return
		}

		if !rm.waitForStageDurationWithTimeout(ctx, runID, baselineStage, actor) {
			return
		}
		if err := rm.TransitionToRamp(runID, actor); err != nil {
			log.Printf("[RunManager] Failed to transition run %s to ramp: %v, stopping run", runID, err)
			_ = rm.requestStopWithReason(runID, StopModeImmediate, string(ActorSystem), "stage_transition_failed", nil)
			return
		}

		if !rm.waitForStageDurationWithTimeout(ctx, runID, rampStage, actor) {
			return
		}

		soakStage := findStageByName(parsedConfig, StageNameSoak)
		if soakStage != nil && soakStage.Enabled {
			if err := rm.TransitionToSoak(runID, actor); err != nil {
				log.Printf("[RunManager] Failed to transition run %s to soak: %v, stopping run", runID, err)
				_ = rm.requestStopWithReason(runID, StopModeImmediate, string(ActorSystem), "stage_transition_failed", nil)
				return
			}

			if !rm.waitForStageDurationWithTimeout(ctx, runID, soakStage, actor) {
				return
			}
		}

		if err := rm.RequestStop(runID, StopModeDrain, actor); err != nil {
			log.Printf("[RunManager] Failed to stop run %s after ramp: %v", runID, err)
		}
	}()
}

func (rm *RunManager) waitForStageDuration(ctx context.Context, runID string, duration time.Duration) bool {
	if duration <= 0 {
		return true
	}

	timer := time.NewTimer(duration)
	rm.addStageTimer(runID, timer)
	defer rm.removeStageTimer(runID, timer)

	select {
	case <-ctx.Done():
		stopTimer(timer)
		return false
	case <-timer.C:
		return true
	}
}

// waitForStageDurationWithTimeout waits for stage duration with a max_duration safety timeout.
// If max_duration fires first, emits STAGE_TIMEOUT and transitions to STOPPING.
func (rm *RunManager) waitForStageDurationWithTimeout(ctx context.Context, runID string, stage *parsedStage, actor string) bool {
	duration := time.Duration(stage.DurationMs) * time.Millisecond
	maxDuration := getStageMaxDuration(stage)

	if duration <= 0 {
		return true
	}

	plannedTimer := time.NewTimer(duration)
	rm.addStageTimer(runID, plannedTimer)
	defer rm.removeStageTimer(runID, plannedTimer)

	var maxTimer *time.Timer
	var maxTimerChan <-chan time.Time
	if maxDuration > 0 && maxDuration < duration {
		maxTimer = time.NewTimer(maxDuration)
		maxTimerChan = maxTimer.C
		rm.addStageTimer(runID, maxTimer)
		defer func() {
			if maxTimer != nil {
				rm.removeStageTimer(runID, maxTimer)
			}
		}()
	}

	select {
	case <-ctx.Done():
		stopTimer(plannedTimer)
		if maxTimer != nil {
			stopTimer(maxTimer)
		}
		return false
	case <-maxTimerChan:
		stopTimer(plannedTimer)
		rm.handleStageTimeout(runID, stage, actor)
		return false
	case <-plannedTimer.C:
		if maxTimer != nil {
			stopTimer(maxTimer)
		}
		return true
	}
}

// handleStageTimeout handles stage timeout by emitting event and transitioning to STOPPING.
func (rm *RunManager) handleStageTimeout(runID string, stage *parsedStage, actor string) {
	rm.mu.RLock()
	record, ok := rm.runs[runID]
	if !ok {
		rm.mu.RUnlock()
		return
	}
	eventLog := rm.eventLogs[runID]
	executionID := record.ExecutionID
	stageStartMs := record.UpdatedAtMs
	rm.mu.RUnlock()

	nowMs := time.Now().UnixMilli()
	elapsedMs := nowMs - stageStartMs
	effectiveTimeoutMs := int64(getStageMaxDuration(stage) / time.Millisecond)

	stageName := StageName(stage.Stage)
	stageID := stage.StageID

	payload, _ := json.Marshal(map[string]interface{}{
		"stage":      stage.Stage,
		"stage_id":   stage.StageID,
		"elapsed_ms": elapsedMs,
		"timeout_ms": effectiveTimeoutMs,
	})

	event := RunEvent{
		RunID:       runID,
		ExecutionID: executionID,
		Type:        EventTypeStageTimeout,
		Actor:       ActorSystem,
		Correlation: CorrelationContext{
			Stage:   &stageName,
			StageID: &stageID,
		},
		Payload:  payload,
		Evidence: []Evidence{{Kind: "timeout", Ref: fmt.Sprintf("max_duration_ms=%d", effectiveTimeoutMs)}},
	}
	_ = eventLog.Append(event)

	log.Printf("[RunManager] Stage %s timeout for run %s (max_duration_ms=%d)", stage.Stage, runID, effectiveTimeoutMs)
	_ = rm.requestStopWithReason(runID, StopModeImmediate, actor, "stage_timeout", event.Evidence)
}

func (rm *RunManager) addStageTimer(runID string, timer *time.Timer) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	record, ok := rm.runs[runID]
	if !ok {
		stopTimer(timer)
		return
	}
	record.progressionTimers = append(record.progressionTimers, timer)
}

func (rm *RunManager) removeStageTimer(runID string, timer *time.Timer) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	record, ok := rm.runs[runID]
	if !ok {
		return
	}
	for i, t := range record.progressionTimers {
		if t == timer {
			record.progressionTimers = append(record.progressionTimers[:i], record.progressionTimers[i+1:]...)
			return
		}
	}
}

func (rm *RunManager) cancelStageProgressionLocked(record *RunRecord) {
	if record.progressionCancel != nil {
		record.progressionCancel()
		record.progressionCancel = nil
	}
	if record.rampCancel != nil {
		record.rampCancel()
		record.rampCancel = nil
	}
	for _, timer := range record.progressionTimers {
		stopTimer(timer)
	}
	record.progressionTimers = nil
}

func (rm *RunManager) stopStopConditionEvaluatorLocked(record *RunRecord) {
	if record.stopConditionsCancel != nil {
		record.stopConditionsCancel()
		record.stopConditionsCancel = nil
	}
}

func (rm *RunManager) startStopConditionEvaluator(runID string, stage *parsedStage) {
	// Start if either classic stop conditions OR streaming config present
	if stage == nil || (len(stage.StopConditions) == 0 && stage.StreamingStopConfig == nil) {
		return
	}

	rm.mu.RLock()
	record, ok := rm.runs[runID]
	telemetryStore := rm.telemetryStore
	rm.mu.RUnlock()
	if !ok {
		return
	}
	if telemetryStore == nil {
		log.Printf("[RunManager] Stop conditions evaluator disabled for run %s: telemetry store not configured", runID)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())

	rm.mu.Lock()
	record, ok = rm.runs[runID]
	if !ok {
		rm.mu.Unlock()
		cancel()
		return
	}
	rm.stopStopConditionEvaluatorLocked(record)
	record.stopConditionsCancel = cancel
	rm.mu.Unlock()

	conditions := make([]stopconditions.Condition, len(stage.StopConditions))
	for i, sc := range stage.StopConditions {
		conditions[i] = stopconditions.Condition{
			ID:             sc.ID,
			Metric:         sc.Metric,
			Comparator:     sc.Comparator,
			Threshold:      sc.Threshold,
			WindowMs:       sc.WindowMs,
			SustainWindows: sc.SustainWindows,
			Scope:          sc.Scope,
		}
	}

	evaluator := stopconditions.NewEvaluator(
		runID,
		stopconditions.TelemetryProviderFunc(func(id string) ([]analysis.OperationResult, error) {
			data, err := telemetryStore.GetTelemetryData(id)
			if err != nil {
				return nil, err
			}
			return data.Operations, nil
		}),
		conditions,
		5*time.Second,
	)
	evaluator.StageID = stage.StageID

	if stage.StreamingStopConfig != nil {
		evaluator.StreamingConfig = &stopconditions.StreamingConfig{
			StreamStallSeconds: stage.StreamingStopConfig.StreamStallSeconds,
			MinEventsPerSecond: stage.StreamingStopConfig.MinEventsPerSecond,
		}
	}

	if streamingProvider, ok := telemetryStore.(interface {
		GetStreamingMetrics(string) (*telemetry.StreamingMetrics, error)
	}); ok {
		evaluator.Streaming = stopconditions.StreamingProviderFunc(streamingProvider.GetStreamingMetrics)
	}

	evaluator.OnTrigger = func(trigger stopconditions.Trigger) {
		rm.handleStopConditionTrigger(runID, stage, trigger)
	}

	go evaluator.Run(ctx.Done())
}

func (rm *RunManager) handleStopConditionTrigger(runID string, stage *parsedStage, trigger stopconditions.Trigger) {
	rm.mu.RLock()
	record, ok := rm.runs[runID]
	if !ok {
		rm.mu.RUnlock()
		return
	}
	eventLog := rm.eventLogs[runID]
	executionID := record.ExecutionID
	telemetryStore := rm.telemetryStore
	rm.mu.RUnlock()

	stageName := StageName(stage.Stage)
	stageID := stage.StageID

	payload, _ := json.Marshal(map[string]interface{}{
		"condition_id": trigger.Condition.ID,
		"metric":       trigger.Condition.Metric,
		"comparator":   trigger.Condition.Comparator,
		"threshold":    trigger.Condition.Threshold,
		"observed":     trigger.Observed,
		"window_ms":    trigger.WindowMs,
		"total_ops":    trigger.TotalOps,
		"failed_ops":   trigger.FailedOps,
		"latency_p99":  trigger.LatencyP99,
		"stage":        stage.Stage,
		"stage_id":     stage.StageID,
	})

	evidenceNote := fmt.Sprintf("observed=%v threshold=%v window_ms=%d", trigger.Observed, trigger.Condition.Threshold, trigger.WindowMs)
	triggerEvent := RunEvent{
		RunID:       runID,
		ExecutionID: executionID,
		Type:        EventTypeStopConditionTriggered,
		Actor:       ActorSystem,
		Correlation: CorrelationContext{
			Stage:   &stageName,
			StageID: &stageID,
		},
		Payload: payload,
		Evidence: []Evidence{
			{Kind: "metric", Ref: trigger.Condition.Metric, Note: stringPtr(evidenceNote)},
		},
	}
	_ = eventLog.Append(triggerEvent)

	reason := fmt.Sprintf("stop_condition_triggered: %s %s %.4f (observed %.4f)",
		trigger.Condition.Metric,
		trigger.Condition.Comparator,
		trigger.Condition.Threshold,
		trigger.Observed,
	)

	// Set stop reason in telemetry
	if telemetryStore != nil {
		stopReasonMsg := fmt.Sprintf("%s threshold exceeded: %.2f > %.2f", trigger.Condition.Metric, trigger.Observed, trigger.Condition.Threshold)
		telemetryStore.SetRunMetadata(runID, "", stopReasonMsg)
	}

	_ = rm.requestStopWithReason(runID, StopModeDrain, string(ActorSystem), reason, triggerEvent.Evidence)
}

// TransitionToBaseline transitions a run from PREFLIGHT_RUNNING to BASELINE_RUNNING.
func (rm *RunManager) TransitionToBaseline(runID, actor string) error {
	var (
		configCopy        []byte
		executionID       string
		eventLog          *EventLog
		oldState          RunState
		transitionPayload []byte
		transitionEvent   RunEvent
		err               error
	)

	func() {
		rm.mu.Lock()
		defer rm.mu.Unlock()

		record, ok := rm.runs[runID]
		if !ok {
			err = NewNotFoundError(runID)
			return
		}

		if record.State != RunStatePreflightRunning {
			err = NewInvalidStateError(runID, record.State, RunStatePreflightRunning, "transition to baseline")
			return
		}

		if !CanTransition(record.State, RunStatePreflightPassed) {
			err = NewInvalidTransitionError(runID, record.State, RunStatePreflightPassed)
			return
		}

		oldState = record.State
		record.State = RunStatePreflightPassed
		record.UpdatedAtMs = time.Now().UnixMilli()

		eventLog = rm.eventLogs[runID]
		transitionPayload, _ := json.Marshal(map[string]interface{}{
			"from_state": oldState,
			"to_state":   record.State,
			"trigger":    "preflight_completed",
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

		if !CanTransition(record.State, RunStateBaselineRunning) {
			err = NewInvalidTransitionError(runID, record.State, RunStateBaselineRunning)
			return
		}

		oldState = record.State
		record.State = RunStateBaselineRunning
		record.UpdatedAtMs = time.Now().UnixMilli()

		configCopy = make([]byte, len(record.Config))
		copy(configCopy, record.Config)
		executionID = record.ExecutionID
	}()

	if err != nil {
		return err
	}

	parsedConfig, err := parseRunConfig(configCopy)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}
	baselineStage := findStageByName(parsedConfig, StageNameBaseline)
	if baselineStage == nil {
		return fmt.Errorf("no enabled baseline stage found")
	}

	rm.mu.Lock()
	func() {
		defer rm.mu.Unlock()
		record, ok := rm.runs[runID]
		if ok {
			record.ActiveStage = &ActiveStageInfo{Stage: string(StageNameBaseline), StageID: baselineStage.StageID}
		}
	}()

	if l := events.GetGlobalEventLogger(); l != nil {
		l.LogStageTransition(string(StageNamePreflight), string(StageNameBaseline), baselineStage.StageID, "preflight_completed")
	}

	transitionPayload, _ = json.Marshal(map[string]interface{}{
		"from_state": oldState,
		"to_state":   RunStateBaselineRunning,
		"trigger":    "baseline_started",
		"actor":      actor,
	})
	transitionEvent = RunEvent{
		RunID:       runID,
		ExecutionID: executionID,
		Type:        EventTypeStateTransition,
		Actor:       ActorType(actor),
		Payload:     transitionPayload,
		Evidence:    []Evidence{},
	}
	_ = eventLog.Append(transitionEvent)

	schedulerReady := false
	func() {
		rm.mu.RLock()
		defer rm.mu.RUnlock()
		schedulerReady = rm.registry != nil && rm.allocator != nil && rm.leaseManager != nil && rm.assignmentSender != nil
	}()

	if schedulerReady {
		rm.createAndDispatchAssignmentsForStage(runID, executionID, configCopy, eventLog, StageNameBaseline)
	}

	rm.startStopConditionEvaluator(runID, baselineStage)

	return nil
}

// TransitionToRamp transitions a run from BASELINE_RUNNING to RAMP_RUNNING.
func (rm *RunManager) TransitionToRamp(runID, actor string) error {
	var (
		configCopy  []byte
		executionID string
		eventLog    *EventLog
		oldState    RunState
		err         error
	)

	func() {
		rm.mu.Lock()
		defer rm.mu.Unlock()

		record, ok := rm.runs[runID]
		if !ok {
			err = NewNotFoundError(runID)
			return
		}

		if record.State != RunStateBaselineRunning {
			err = NewInvalidStateError(runID, record.State, RunStateBaselineRunning, "transition to ramp")
			return
		}

		if !CanTransition(record.State, RunStateRampRunning) {
			err = NewInvalidTransitionError(runID, record.State, RunStateRampRunning)
			return
		}

		oldState = record.State
		record.State = RunStateRampRunning
		record.UpdatedAtMs = time.Now().UnixMilli()

		configCopy = make([]byte, len(record.Config))
		copy(configCopy, record.Config)
		executionID = record.ExecutionID
		eventLog = rm.eventLogs[runID]
	}()

	if err != nil {
		return err
	}

	parsedConfig, err := parseRunConfig(configCopy)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}
	rampStage := findStageByName(parsedConfig, StageNameRamp)
	if rampStage == nil {
		return fmt.Errorf("no enabled ramp stage found")
	}

	rm.mu.Lock()
	func() {
		defer rm.mu.Unlock()
		record, ok := rm.runs[runID]
		if ok {
			record.ActiveStage = &ActiveStageInfo{Stage: string(StageNameRamp), StageID: rampStage.StageID}
		}
	}()

	if l := events.GetGlobalEventLogger(); l != nil {
		l.LogStageTransition(string(StageNameBaseline), string(StageNameRamp), rampStage.StageID, "baseline_completed")
	}

	transitionPayload, _ := json.Marshal(map[string]interface{}{
		"from_state": oldState,
		"to_state":   RunStateRampRunning,
		"trigger":    "ramp_started",
		"actor":      actor,
	})
	transitionEvent := RunEvent{
		RunID:       runID,
		ExecutionID: executionID,
		Type:        EventTypeStateTransition,
		Actor:       ActorType(actor),
		Payload:     transitionPayload,
		Evidence:    []Evidence{},
	}
	_ = eventLog.Append(transitionEvent)

	schedulerReady := false
	func() {
		rm.mu.RLock()
		defer rm.mu.RUnlock()
		schedulerReady = rm.registry != nil && rm.allocator != nil && rm.leaseManager != nil && rm.assignmentSender != nil
	}()

	if schedulerReady {
		rm.startAutoRamp(runID, executionID, configCopy, eventLog, rampStage, parsedConfig)
	}

	rm.startStopConditionEvaluator(runID, rampStage)

	return nil
}

// startAutoRamp implements progressive VU scaling during ramp stage
func (rm *RunManager) startAutoRamp(runID, executionID string, config []byte, eventLog *EventLog, stage *parsedStage, parsedConfig *parsedRunConfig) {
	targetVUs := stage.Load.TargetVUs
	if targetVUs <= 0 {
		log.Printf("[RunManager] Invalid target VUs for auto-ramp: %d", targetVUs)
		return
	}

	if parsedConfig.Safety.HardCaps.MaxVUs > 0 && targetVUs > parsedConfig.Safety.HardCaps.MaxVUs {
		log.Printf("[RunManager] Capping ramp target VUs from %d to hard cap %d for run %s", targetVUs, parsedConfig.Safety.HardCaps.MaxVUs, runID)
		targetVUs = parsedConfig.Safety.HardCaps.MaxVUs
	}

	startVUs := stage.Load.StartVUs
	if startVUs <= 0 {
		startVUs = max(1, targetVUs/10)
	}
	if startVUs > targetVUs {
		startVUs = targetVUs
	}

	rampSteps := stage.Load.RampSteps
	if rampSteps <= 0 {
		rampSteps = 5
	}

	stepHoldMs := stage.Load.StepHoldMs
	if stepHoldMs <= 0 && stage.DurationMs > 0 {
		stepHoldMs = int(stage.DurationMs) / rampSteps
	}
	if stepHoldMs <= 0 {
		stepHoldMs = 10000
	}

	vuIncrement := (targetVUs - startVUs) / rampSteps
	if vuIncrement <= 0 {
		vuIncrement = 1
	}

	log.Printf("[RunManager] Starting auto-ramp for run %s: %d -> %d VUs in %d steps, %dms per step",
		runID, startVUs, targetVUs, rampSteps, stepHoldMs)

	ctx, cancel := context.WithCancel(context.Background())
	rm.mu.Lock()
	if record, ok := rm.runs[runID]; ok {
		record.rampCancel = cancel
	}
	rm.mu.Unlock()

	rm.emitRampStepEvent(runID, executionID, eventLog, 0, startVUs, targetVUs, "ramp_started")

	rm.dispatchRampAssignments(runID, executionID, config, eventLog, stage, parsedConfig, startVUs, 0)

	go func() {
		defer cancel()
		currentVUs := startVUs
		for step := 1; step <= rampSteps; step++ {
			timer := time.NewTimer(time.Duration(stepHoldMs) * time.Millisecond)
			rm.addStageTimer(runID, timer)

			select {
			case <-ctx.Done():
				timer.Stop()
				rm.removeStageTimer(runID, timer)
				log.Printf("[RunManager] Auto-ramp cancelled for run %s", runID)
				return
			case <-timer.C:
				rm.removeStageTimer(runID, timer)
			}

			rm.mu.RLock()
			record, ok := rm.runs[runID]
			rm.mu.RUnlock()
			if !ok || record.State != RunStateRampRunning {
				log.Printf("[RunManager] Auto-ramp stopped for run %s: state changed", runID)
				return
			}

			nextVUs := startVUs + (vuIncrement * step)
			if nextVUs > targetVUs {
				nextVUs = targetVUs
			}

			additionalVUs := nextVUs - currentVUs
			if additionalVUs > 0 {
				log.Printf("[RunManager] Ramp step %d/%d: scaling from %d to %d VUs (+%d)",
					step, rampSteps, currentVUs, nextVUs, additionalVUs)

				rm.emitRampStepEvent(runID, executionID, eventLog, step, nextVUs, targetVUs, "ramp_step")

				rm.dispatchRampAssignments(runID, executionID, config, eventLog, stage, parsedConfig, additionalVUs, currentVUs)
				currentVUs = nextVUs
			}

			if currentVUs >= targetVUs {
				log.Printf("[RunManager] Auto-ramp complete for run %s: reached target %d VUs", runID, targetVUs)
				rm.emitRampStepEvent(runID, executionID, eventLog, rampSteps, targetVUs, targetVUs, "ramp_complete")
				return
			}
		}
	}()
}

// TransitionToSoak transitions a run from RAMP_RUNNING to SOAK_RUNNING.
func (rm *RunManager) TransitionToSoak(runID, actor string) error {
	rm.mu.Lock()

	record, ok := rm.runs[runID]
	if !ok {
		rm.mu.Unlock()
		return NewNotFoundError(runID)
	}

	if record.State != RunStateRampRunning {
		rm.mu.Unlock()
		return NewInvalidStateError(runID, record.State, RunStateRampRunning, "transition to soak")
	}

	if !CanTransition(record.State, RunStateSoakRunning) {
		rm.mu.Unlock()
		return NewInvalidTransitionError(runID, record.State, RunStateSoakRunning)
	}

	oldState := record.State
	record.State = RunStateSoakRunning
	record.UpdatedAtMs = time.Now().UnixMilli()

	configCopy := make([]byte, len(record.Config))
	copy(configCopy, record.Config)
	executionID := record.ExecutionID
	eventLog := rm.eventLogs[runID]
	rm.mu.Unlock()

	parsedConfig, err := parseRunConfig(configCopy)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}
	soakStage := findStageByName(parsedConfig, StageNameSoak)
	if soakStage == nil {
		return fmt.Errorf("no enabled soak stage found")
	}

	rm.mu.Lock()
	record, ok = rm.runs[runID]
	if ok {
		record.ActiveStage = &ActiveStageInfo{Stage: string(StageNameSoak), StageID: soakStage.StageID}
	}
	rm.mu.Unlock()

	if l := events.GetGlobalEventLogger(); l != nil {
		l.LogStageTransition(string(StageNameRamp), string(StageNameSoak), soakStage.StageID, "ramp_completed")
	}

	transitionPayload, _ := json.Marshal(map[string]interface{}{
		"from_state": oldState,
		"to_state":   RunStateSoakRunning,
		"trigger":    "soak_started",
		"actor":      actor,
	})
	transitionEvent := RunEvent{
		RunID:       runID,
		ExecutionID: executionID,
		Type:        EventTypeStateTransition,
		Actor:       ActorType(actor),
		Payload:     transitionPayload,
		Evidence:    []Evidence{},
	}
	_ = eventLog.Append(transitionEvent)

	rm.mu.RLock()
	registry := rm.registry
	allocator := rm.allocator
	leaseManager := rm.leaseManager
	assignmentSender := rm.assignmentSender
	rm.mu.RUnlock()

	if registry != nil && allocator != nil && leaseManager != nil && assignmentSender != nil {
		rm.createAndDispatchAssignmentsForStage(runID, executionID, configCopy, eventLog, StageNameSoak)
	}

	rm.startStopConditionEvaluator(runID, soakStage)

	return nil
}

// TransitionToAnalyzing transitions a run from STOPPING to ANALYZING state.
func (rm *RunManager) TransitionToAnalyzing(runID, actor string) error {
	rm.mu.Lock()

	record, ok := rm.runs[runID]
	if !ok {
		rm.mu.Unlock()
		return NewNotFoundError(runID)
	}

	if record.State != RunStateStopping {
		rm.mu.Unlock()
		return NewInvalidStateError(runID, record.State, RunStateStopping, "transition to analyzing")
	}

	if !CanTransition(record.State, RunStateAnalyzing) {
		rm.mu.Unlock()
		return NewInvalidTransitionError(runID, record.State, RunStateAnalyzing)
	}

	oldState := record.State
	record.State = RunStateAnalyzing
	record.UpdatedAtMs = time.Now().UnixMilli()

	eventLog := rm.eventLogs[runID]

	transitionPayload, _ := json.Marshal(map[string]interface{}{
		"from_state": oldState,
		"to_state":   record.State,
		"trigger":    "workers_drained",
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

	analysisPayload, _ := json.Marshal(map[string]interface{}{
		"run_id": runID,
	})
	analysisEvent := RunEvent{
		RunID:       runID,
		ExecutionID: record.ExecutionID,
		Type:        EventTypeAnalysisStarted,
		Actor:       ActorAnalysis,
		Payload:     analysisPayload,
		Evidence:    []Evidence{},
	}
	_ = eventLog.Append(analysisEvent)

	configCopy := make([]byte, len(record.Config))
	copy(configCopy, record.Config)

	rm.mu.Unlock()

	analysisTimeout := getAnalysisTimeout(configCopy)
	return rm.analyzeRunWithTimeout(runID, analysisTimeout)
}
