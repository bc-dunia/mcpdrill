package runmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/analysis"
	"github.com/bc-dunia/mcpdrill/internal/artifacts"
)

func (rm *RunManager) analyzeRunWithTimeout(runID string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- rm.analyzeRunWithContext(ctx, runID)
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		rm.failAnalysis(runID, "analysis_timeout", fmt.Sprintf("analysis exceeded %v timeout", timeout))
		return fmt.Errorf("analysis timeout after %v", timeout)
	}
}

// analyzeRunWithContext performs analysis with context cancellation support.
// It checks context at key points to allow early termination.
func (rm *RunManager) analyzeRunWithContext(ctx context.Context, runID string) error {
	// Check context before starting
	if err := ctx.Err(); err != nil {
		return err
	}

	rm.mu.RLock()
	record, ok := rm.runs[runID]
	if !ok {
		rm.mu.RUnlock()
		return NewNotFoundError(runID)
	}

	if record.State != RunStateAnalyzing {
		rm.mu.RUnlock()
		return NewInvalidStateError(runID, record.State, RunStateAnalyzing, "analyze")
	}

	telemetryStore := rm.telemetryStore
	artifactStore := rm.artifactStore
	eventLog := rm.eventLogs[runID]
	executionID := record.ExecutionID
	scenarioID := record.ScenarioID
	rm.mu.RUnlock()

	if telemetryStore == nil {
		rm.failAnalysis(runID, "telemetry_store_not_configured", "no telemetry store configured")
		return fmt.Errorf("telemetry store not configured")
	}

	if artifactStore == nil {
		rm.failAnalysis(runID, "artifact_store_not_configured", "no artifact store configured")
		return fmt.Errorf("artifact store not configured")
	}

	// Check context before telemetry retrieval
	if err := ctx.Err(); err != nil {
		return err
	}

	telemetryData, err := telemetryStore.GetTelemetryData(runID)
	if err != nil {
		rm.failAnalysis(runID, "telemetry_retrieval_failed", err.Error())
		return fmt.Errorf("failed to retrieve telemetry data: %w", err)
	}

	// Check context before aggregation
	if err := ctx.Err(); err != nil {
		return err
	}

	aggregator := analysis.NewAggregator()
	aggregator.SetTimeRange(telemetryData.StartTimeMs, telemetryData.EndTimeMs)
	for _, op := range telemetryData.Operations {
		aggregator.AddOperation(op)
	}
	metrics := aggregator.Compute()

	// Check context before report generation
	if err := ctx.Err(); err != nil {
		return err
	}

	report := &analysis.Report{
		RunID:      runID,
		ScenarioID: scenarioID,
		StartTime:  telemetryData.StartTimeMs,
		EndTime:    telemetryData.EndTimeMs,
		Duration:   telemetryData.EndTimeMs - telemetryData.StartTimeMs,
		Metrics:    metrics,
		StopReason: telemetryData.StopReason,
	}

	reporter := analysis.NewReporter()

	jsonData, err := reporter.GenerateJSON(report)
	if err != nil {
		rm.failAnalysis(runID, "json_report_generation_failed", err.Error())
		return fmt.Errorf("failed to generate JSON report: %w", err)
	}

	// Check context before HTML generation
	if err := ctx.Err(); err != nil {
		return err
	}

	htmlData, err := reporter.GenerateHTML(report)
	if err != nil {
		rm.failAnalysis(runID, "html_report_generation_failed", err.Error())
		return fmt.Errorf("failed to generate HTML report: %w", err)
	}

	// Check context before artifact storage
	if err := ctx.Err(); err != nil {
		return err
	}

	jsonInfo, err := artifactStore.SaveArtifact(runID, artifacts.ArtifactTypeReport, "report.json", jsonData)
	if err != nil {
		rm.failAnalysis(runID, "json_artifact_storage_failed", err.Error())
		return fmt.Errorf("failed to store JSON report: %w", err)
	}

	htmlInfo, err := artifactStore.SaveArtifact(runID, artifacts.ArtifactTypeReport, "report.html", htmlData)
	if err != nil {
		rm.failAnalysis(runID, "html_artifact_storage_failed", err.Error())
		return fmt.Errorf("failed to store HTML report: %w", err)
	}

	rm.emitReportGeneratedEvent(runID, executionID, eventLog, jsonInfo, htmlInfo)

	rm.completeAnalysis(runID)

	return nil
}

// AnalyzeRun performs analysis on a run's telemetry data and generates reports.
// It aggregates telemetry, generates HTML and JSON reports, stores them as artifacts,
// and emits appropriate events. On success, transitions to COMPLETED state.
// On failure, transitions to FAILED state.
// Deprecated: Use analyzeRunWithContext for cancellation support.
func (rm *RunManager) AnalyzeRun(runID string) error {
	return rm.analyzeRunWithContext(context.Background(), runID)
}

func (rm *RunManager) emitReportGeneratedEvent(runID, executionID string, eventLog *EventLog, jsonInfo, htmlInfo *artifacts.ArtifactInfo) {
	payload, _ := json.Marshal(map[string]interface{}{
		"run_id": runID,
		"reports": []map[string]interface{}{
			{
				"type":     "json",
				"filename": jsonInfo.Filename,
				"path":     jsonInfo.Path,
				"size":     jsonInfo.SizeBytes,
			},
			{
				"type":     "html",
				"filename": htmlInfo.Filename,
				"path":     htmlInfo.Path,
				"size":     htmlInfo.SizeBytes,
			},
		},
	})

	event := RunEvent{
		RunID:       runID,
		ExecutionID: executionID,
		Type:        EventTypeReportGenerated,
		Actor:       ActorAnalysis,
		Payload:     payload,
		Evidence: []Evidence{
			{Kind: "artifact", Ref: jsonInfo.Path, Note: stringPtr("JSON report")},
			{Kind: "artifact", Ref: htmlInfo.Path, Note: stringPtr("HTML report")},
		},
	}
	_ = eventLog.Append(event)
}

func (rm *RunManager) completeAnalysis(runID string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	record, ok := rm.runs[runID]
	if !ok {
		return
	}

	if record.State != RunStateAnalyzing {
		return
	}

	oldState := record.State

	// Per state machine: emergency_stop should lead to ABORTED, not COMPLETED
	finalState := RunStateCompleted
	trigger := "analysis_completed"
	if record.StopReason != nil {
		if record.StopReason.Reason == "emergency_stop" ||
			(record.StopReason.Mode == StopModeImmediate && record.StopReason.Actor == "emergency") {
			finalState = RunStateAborted
			trigger = "emergency_stop"
		}
	}

	record.State = finalState
	record.UpdatedAtMs = time.Now().UnixMilli()

	eventLog := rm.eventLogs[runID]

	completedPayload, _ := json.Marshal(map[string]interface{}{
		"run_id": runID,
	})
	completedEvent := RunEvent{
		RunID:       runID,
		ExecutionID: record.ExecutionID,
		Type:        EventTypeAnalysisCompleted,
		Actor:       ActorAnalysis,
		Payload:     completedPayload,
		Evidence:    []Evidence{},
	}
	_ = eventLog.Append(completedEvent)

	transitionPayload, _ := json.Marshal(map[string]interface{}{
		"from_state": oldState,
		"to_state":   record.State,
		"trigger":    trigger,
		"actor":      "analysis",
	})
	transitionEvent := RunEvent{
		RunID:       runID,
		ExecutionID: record.ExecutionID,
		Type:        EventTypeStateTransition,
		Actor:       ActorAnalysis,
		Payload:     transitionPayload,
		Evidence:    []Evidence{},
	}
	_ = eventLog.Append(transitionEvent)
}

func (rm *RunManager) failAnalysis(runID, reason, details string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	record, ok := rm.runs[runID]
	if !ok {
		return
	}

	if record.State != RunStateAnalyzing {
		return
	}

	oldState := record.State
	record.State = RunStateFailed
	record.UpdatedAtMs = time.Now().UnixMilli()

	eventLog := rm.eventLogs[runID]

	failPayload, _ := json.Marshal(map[string]interface{}{
		"run_id":  runID,
		"reason":  reason,
		"details": details,
	})
	failEvent := RunEvent{
		RunID:       runID,
		ExecutionID: record.ExecutionID,
		Type:        EventTypeDecision,
		Actor:       ActorAnalysis,
		Payload:     failPayload,
		Evidence:    []Evidence{},
	}
	_ = eventLog.Append(failEvent)

	transitionPayload, _ := json.Marshal(map[string]interface{}{
		"from_state": oldState,
		"to_state":   record.State,
		"trigger":    "analysis_failed",
		"actor":      "analysis",
		"reason":     reason,
	})
	transitionEvent := RunEvent{
		RunID:       runID,
		ExecutionID: record.ExecutionID,
		Type:        EventTypeStateTransition,
		Actor:       ActorAnalysis,
		Payload:     transitionPayload,
		Evidence:    []Evidence{},
	}
	_ = eventLog.Append(transitionEvent)
}
