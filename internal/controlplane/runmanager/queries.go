package runmanager

import (
	"encoding/json"

	"github.com/bc-dunia/mcpdrill/internal/controlplane/scheduler"
)

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

// GetRunConfig returns the raw configuration for a run.
// Returns an error if the run is not found.
func (rm *RunManager) GetRunConfig(runID string) ([]byte, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	record, ok := rm.runs[runID]
	if !ok {
		return nil, NewNotFoundError(runID)
	}

	if record.Config == nil {
		return nil, NewConfigNotAvailableError(runID)
	}

	configCopy := make([]byte, len(record.Config))
	copy(configCopy, record.Config)
	return configCopy, nil
}

// CloneRun creates a new run with the same configuration as an existing run.
// Returns the new run ID on success, or an error if the source run is not found.
func (rm *RunManager) CloneRun(sourceRunID, actor string) (string, error) {
	config, err := rm.GetRunConfig(sourceRunID)
	if err != nil {
		return "", err
	}

	return rm.CreateRun(config, actor)
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
