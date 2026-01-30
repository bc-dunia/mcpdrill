package runmanager

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// EventType represents the type of run event.
type EventType string

const (
	EventTypeRunCreated              EventType = "RUN_CREATED"
	EventTypeValidationCompleted     EventType = "VALIDATION_COMPLETED"
	EventTypeStateTransition         EventType = "STATE_TRANSITION"
	EventTypeAllocationFailed        EventType = "ALLOCATION_FAILED"
	EventTypeStageStarted            EventType = "STAGE_STARTED"
	EventTypeStageCompleted          EventType = "STAGE_COMPLETED"
	EventTypeStageFailed             EventType = "STAGE_FAILED"
	EventTypeSchedulerTargetSet      EventType = "SCHEDULER_TARGET_SET"
	EventTypeWorkerAssigned          EventType = "WORKER_ASSIGNED"
	EventTypeWorkerAssignmentRejected EventType = "WORKER_ASSIGNMENT_REJECTED"
	EventTypeWorkerRegistered        EventType = "WORKER_REGISTERED"
	EventTypeWorkerHeartbeat         EventType = "WORKER_HEARTBEAT"
	EventTypeWorkerCapacityLost      EventType = "WORKER_CAPACITY_LOST"
	EventTypeWorkerReplaced          EventType = "WORKER_REPLACED"
	EventTypeStopRequested           EventType = "STOP_REQUESTED"
	EventTypeEmergencyStop           EventType = "EMERGENCY_STOP"
	EventTypeStopConditionTriggered  EventType = "STOP_CONDITION_TRIGGERED"
	EventTypeStageTimeout            EventType = "STAGE_TIMEOUT"
	EventTypeDecision                EventType = "DECISION"
	EventTypeAnalysisStarted         EventType = "ANALYSIS_STARTED"
	EventTypeAnalysisCompleted       EventType = "ANALYSIS_COMPLETED"
	EventTypeReportGenerated         EventType = "REPORT_GENERATED"
	EventTypeArtifactStored          EventType = "ARTIFACT_STORED"
	EventTypeSystemRecovery          EventType = "SYSTEM_RECOVERY"
	EventTypeSystemWarning           EventType = "SYSTEM_WARNING"
)

// ActorType represents who triggered the event.
type ActorType string

const (
	ActorSystem    ActorType = "system"
	ActorUser      ActorType = "user"
	ActorScheduler ActorType = "scheduler"
	ActorAutoramp  ActorType = "autoramp"
	ActorAnalysis  ActorType = "analysis"
	ActorWorker    ActorType = "worker"
)

// StageName represents the stage name in correlation context.
type StageName string

const (
	StageNamePreflight StageName = "preflight"
	StageNameBaseline  StageName = "baseline"
	StageNameRamp      StageName = "ramp"
	StageNameSoak      StageName = "soak"
	StageNameSpike     StageName = "spike"
	StageNameCustom    StageName = "custom"
)

// CorrelationContext holds correlation keys for event attribution.
type CorrelationContext struct {
	Stage     *StageName `json:"stage"`
	StageID   *string    `json:"stage_id"`
	WorkerID  *string    `json:"worker_id"`
	VUID      *string    `json:"vu_id"`
	SessionID *string    `json:"session_id"`
}

// Evidence represents a single piece of evidence supporting an event.
type Evidence struct {
	Kind string  `json:"kind"`
	Ref  string  `json:"ref"`
	Note *string `json:"note"`
}

// RunEvent represents a single event in the run lifecycle.
type RunEvent struct {
	SchemaVersion string               `json:"schema_version"`
	EventID       string               `json:"event_id"`
	TimestampMs   int64                `json:"ts_ms"`
	RunID         string               `json:"run_id"`
	ExecutionID   string               `json:"execution_id"`
	Type          EventType            `json:"type"`
	Actor         ActorType            `json:"actor"`
	Correlation   CorrelationContext   `json:"correlation"`
	Payload       json.RawMessage      `json:"payload"`
	Evidence      []Evidence           `json:"evidence"`
}

// DefaultMaxEventsPerLog is the default maximum events per EventLog.
const DefaultMaxEventsPerLog = 10000

// EventLog is an append-only log of run events with configurable memory limits.
type EventLog struct {
	mu        sync.RWMutex
	events    []RunEvent
	counter   atomic.Int64
	maxEvents int
	truncated bool
	runID     string // For logging purposes
}

// NewEventLog creates a new append-only event log with default limits.
func NewEventLog() *EventLog {
	return NewEventLogWithLimit(DefaultMaxEventsPerLog)
}

// NewEventLogWithLimit creates a new event log with a custom limit.
// Set maxEvents to 0 for unlimited (not recommended for production).
func NewEventLogWithLimit(maxEvents int) *EventLog {
	return &EventLog{
		events:    make([]RunEvent, 0, 100),
		maxEvents: maxEvents,
	}
}

// Append adds an event to the log. Returns error if event is invalid.
// If the log has reached its maximum capacity, new events are dropped
// and a warning is logged (once per log).
func (el *EventLog) Append(event RunEvent) error {
	// Validate required fields
	if event.RunID == "" {
		return fmt.Errorf("event missing required field: run_id")
	}
	if event.ExecutionID == "" {
		return fmt.Errorf("event missing required field: execution_id")
	}
	if event.Type == "" {
		return fmt.Errorf("event missing required field: type")
	}
	if event.Actor == "" {
		return fmt.Errorf("event missing required field: actor")
	}
	if event.Payload == nil {
		return fmt.Errorf("event missing required field: payload")
	}
	if event.Evidence == nil {
		return fmt.Errorf("event missing required field: evidence")
	}

	// Generate event ID if not provided
	if event.EventID == "" {
		event.EventID = generateEventID()
	}

	// Set schema version
	event.SchemaVersion = "event/v1"

	// Set timestamp if not provided
	if event.TimestampMs == 0 {
		event.TimestampMs = time.Now().UnixMilli()
	}

	el.mu.Lock()
	defer el.mu.Unlock()

	// Store runID for logging
	if el.runID == "" {
		el.runID = event.RunID
	}

	// Check memory limit
	if el.maxEvents > 0 && len(el.events) >= el.maxEvents {
		if !el.truncated {
			el.truncated = true
			slog.Warn("event_log_truncated",
				"run_id", el.runID,
				"limit", el.maxEvents,
				"warning", "Event log has reached maximum capacity, new events will be dropped")
		}
		return nil // Silently drop - don't fail the operation
	}

	el.events = append(el.events, event)
	return nil
}

// Tail returns events starting from cursor with limit.
// cursor is the index to start from (0-based).
// limit is the maximum number of events to return.
// Returns empty slice if cursor is out of bounds.
func (el *EventLog) Tail(cursor int, limit int) ([]RunEvent, error) {
	if limit < 0 {
		return nil, fmt.Errorf("limit must be non-negative")
	}
	if cursor < 0 {
		return nil, fmt.Errorf("cursor must be non-negative")
	}

	el.mu.RLock()
	defer el.mu.RUnlock()

	if cursor >= len(el.events) {
		return []RunEvent{}, nil
	}

	end := cursor + limit
	if end > len(el.events) {
		end = len(el.events)
	}

	// Return a copy to prevent external modification
	result := make([]RunEvent, end-cursor)
	copy(result, el.events[cursor:end])
	return result, nil
}

// GetAll returns all events in the log.
func (el *EventLog) GetAll() []RunEvent {
	el.mu.RLock()
	defer el.mu.RUnlock()

	result := make([]RunEvent, len(el.events))
	copy(result, el.events)
	return result
}

// Len returns the number of events in the log.
func (el *EventLog) Len() int {
	el.mu.RLock()
	defer el.mu.RUnlock()

	return len(el.events)
}

// IsTruncated returns true if events were dropped due to memory limits.
func (el *EventLog) IsTruncated() bool {
	el.mu.RLock()
	defer el.mu.RUnlock()
	return el.truncated
}

// FindEventIndex finds the 0-based index of an event by its event_id.
// Returns -1 if the event_id is not found.
func (el *EventLog) FindEventIndex(eventID string) int {
	el.mu.RLock()
	defer el.mu.RUnlock()

	for i, event := range el.events {
		if event.EventID == eventID {
			return i
		}
	}
	return -1
}

// generateEventID generates a unique event ID.
// Format: evt_{timestamp}_{counter}
func generateEventID() string {
	ts := time.Now().UnixMilli()
	counter := eventIDCounter.Add(1)
	return fmt.Sprintf("evt_%x%x", ts, counter)
}

var eventIDCounter atomic.Int64
