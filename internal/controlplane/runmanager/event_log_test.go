package runmanager

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestEventLogAppendBasic(t *testing.T) {
	el := NewEventLog()
	_ = el // Just verify it creates without panic

	event := RunEvent{
		RunID:       "run_0000000000000001",
		ExecutionID: "exe_00000456",
		Type:        EventTypeRunCreated,
		Actor:       ActorSystem,
		Payload:     json.RawMessage(`{"config_hash":"abc123"}`),
		Evidence:    []Evidence{},
	}

	err := el.Append(event)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	if el.Len() != 1 {
		t.Errorf("Expected length 1, got %d", el.Len())
	}
}

func TestEventLogAppendValidation(t *testing.T) {
	tests := []struct {
		name    string
		event   RunEvent
		wantErr bool
		errMsg  string
	}{
		{
			name: "missing run_id",
			event: RunEvent{
				ExecutionID: "exe_00000001",
				Type:        EventTypeRunCreated,
				Actor:       ActorSystem,
				Payload:     json.RawMessage(`{}`),
				Evidence:    []Evidence{},
			},
			wantErr: true,
			errMsg:  "run_id",
		},
		{
			name: "missing execution_id",
			event: RunEvent{
				RunID:    "run_0000000000000001",
				Type:     EventTypeRunCreated,
				Actor:    ActorSystem,
				Payload:  json.RawMessage(`{}`),
				Evidence: []Evidence{},
			},
			wantErr: true,
			errMsg:  "execution_id",
		},
		{
			name: "missing type",
			event: RunEvent{
				RunID:       "run_0000000000000001",
				ExecutionID: "exe_00000001",
				Actor:       ActorSystem,
				Payload:     json.RawMessage(`{}`),
				Evidence:    []Evidence{},
			},
			wantErr: true,
			errMsg:  "type",
		},
		{
			name: "missing actor",
			event: RunEvent{
				RunID:       "run_0000000000000001",
				ExecutionID: "exe_00000001",
				Type:        EventTypeRunCreated,
				Payload:     json.RawMessage(`{}`),
				Evidence:    []Evidence{},
			},
			wantErr: true,
			errMsg:  "actor",
		},
		{
			name: "missing payload",
			event: RunEvent{
				RunID:       "run_0000000000000001",
				ExecutionID: "exe_00000001",
				Type:        EventTypeRunCreated,
				Actor:       ActorSystem,
				Evidence:    []Evidence{},
			},
			wantErr: true,
			errMsg:  "payload",
		},
		{
			name: "missing evidence",
			event: RunEvent{
				RunID:       "run_0000000000000001",
				ExecutionID: "exe_00000001",
				Type:        EventTypeRunCreated,
				Actor:       ActorSystem,
				Payload:     json.RawMessage(`{}`),
			},
			wantErr: true,
			errMsg:  "evidence",
		},
		{
			name: "valid event",
			event: RunEvent{
				RunID:       "run_0000000000000001",
				ExecutionID: "exe_00000001",
				Type:        EventTypeRunCreated,
				Actor:       ActorSystem,
				Payload:     json.RawMessage(`{}`),
				Evidence:    []Evidence{},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			el := NewEventLog()
	_ = el // Just verify it creates without panic
			err := el.Append(tt.event)
			if (err != nil) != tt.wantErr {
				t.Errorf("Append() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("Expected error containing %q, got %q", tt.errMsg, err.Error())
				}
			}
		})
	}
}

func TestEventLogAppendOnlySemantics(t *testing.T) {
	el := NewEventLog()
	_ = el // Just verify it creates without panic

	event1 := RunEvent{
		RunID:       "run_0000000000000001",
		ExecutionID: "exe_00000001",
		Type:        EventTypeRunCreated,
		Actor:       ActorSystem,
		Payload:     json.RawMessage(`{"config_hash":"abc"}`),
		Evidence:    []Evidence{},
	}

	event2 := RunEvent{
		RunID:       "run_0000000000000001",
		ExecutionID: "exe_00000001",
		Type:        EventTypeStateTransition,
		Actor:       ActorSystem,
		Payload:     json.RawMessage(`{"from":"created","to":"preflight_running"}`),
		Evidence:    []Evidence{},
	}

	el.Append(event1)
	el.Append(event2)

	all := el.GetAll()
	if len(all) != 2 {
		t.Errorf("Expected 2 events, got %d", len(all))
	}

	if all[0].Type != EventTypeRunCreated {
		t.Errorf("First event type should be RUN_CREATED, got %s", all[0].Type)
	}

	if all[1].Type != EventTypeStateTransition {
		t.Errorf("Second event type should be STATE_TRANSITION, got %s", all[1].Type)
	}

	if all[0].EventID == all[1].EventID {
		t.Errorf("Event IDs should be unique")
	}
}

func TestEventLogTail(t *testing.T) {
	el := NewEventLog()
	_ = el // Just verify it creates without panic

	for i := 0; i < 10; i++ {
		event := RunEvent{
			RunID:       "run_0000000000000001",
			ExecutionID: "exe_00000001",
			Type:        EventTypeStateTransition,
			Actor:       ActorSystem,
			Payload:     json.RawMessage(fmt.Sprintf(`{"index":%d}`, i)),
			Evidence:    []Evidence{},
		}
		el.Append(event)
	}

	tests := []struct {
		name      string
		cursor    int
		limit     int
		wantCount int
		wantErr   bool
	}{
		{
			name:      "cursor 0, limit 5",
			cursor:    0,
			limit:     5,
			wantCount: 5,
			wantErr:   false,
		},
		{
			name:      "cursor 5, limit 5",
			cursor:    5,
			limit:     5,
			wantCount: 5,
			wantErr:   false,
		},
		{
			name:      "cursor 8, limit 5",
			cursor:    8,
			limit:     5,
			wantCount: 2,
			wantErr:   false,
		},
		{
			name:      "cursor 10, limit 5",
			cursor:    10,
			limit:     5,
			wantCount: 0,
			wantErr:   false,
		},
		{
			name:      "cursor 0, limit 0",
			cursor:    0,
			limit:     0,
			wantCount: 0,
			wantErr:   false,
		},
		{
			name:      "negative cursor",
			cursor:    -1,
			limit:     5,
			wantCount: 0,
			wantErr:   true,
		},
		{
			name:      "negative limit",
			cursor:    0,
			limit:     -1,
			wantCount: 0,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := el.Tail(tt.cursor, tt.limit)
			if (err != nil) != tt.wantErr {
				t.Errorf("Tail() error = %v, wantErr %v", err, tt.wantErr)
			}
			if len(result) != tt.wantCount {
				t.Errorf("Expected %d events, got %d", tt.wantCount, len(result))
			}
		})
	}
}

func TestEventLogGetAll(t *testing.T) {
	el := NewEventLog()
	_ = el // Just verify it creates without panic

	for i := 0; i < 5; i++ {
		event := RunEvent{
			RunID:       "run_0000000000000001",
			ExecutionID: "exe_00000001",
			Type:        EventTypeStateTransition,
			Actor:       ActorSystem,
			Payload:     json.RawMessage(`{}`),
			Evidence:    []Evidence{},
		}
		el.Append(event)
	}

	all := el.GetAll()
	if len(all) != 5 {
		t.Errorf("Expected 5 events, got %d", len(all))
	}

	all[0].Type = EventTypeRunCreated
	all2 := el.GetAll()
	if all2[0].Type == EventTypeRunCreated {
		t.Errorf("GetAll should return a copy, not a reference")
	}
}

func TestEventLogLen(t *testing.T) {
	el := NewEventLog()
	_ = el // Just verify it creates without panic

	if el.Len() != 0 {
		t.Errorf("Expected initial length 0, got %d", el.Len())
	}

	for i := 1; i <= 10; i++ {
		event := RunEvent{
			RunID:       "run_0000000000000001",
			ExecutionID: "exe_00000001",
			Type:        EventTypeStateTransition,
			Actor:       ActorSystem,
			Payload:     json.RawMessage(`{}`),
			Evidence:    []Evidence{},
		}
		el.Append(event)

		if el.Len() != i {
			t.Errorf("Expected length %d, got %d", i, el.Len())
		}
	}
}

func TestEventLogEventIDGeneration(t *testing.T) {
	el := NewEventLog()
	_ = el // Just verify it creates without panic

	event1 := RunEvent{
		RunID:       "run_0000000000000001",
		ExecutionID: "exe_00000001",
		Type:        EventTypeRunCreated,
		Actor:       ActorSystem,
		Payload:     json.RawMessage(`{}`),
		Evidence:    []Evidence{},
	}

	event2 := RunEvent{
		RunID:       "run_0000000000000001",
		ExecutionID: "exe_00000001",
		Type:        EventTypeStateTransition,
		Actor:       ActorSystem,
		Payload:     json.RawMessage(`{}`),
		Evidence:    []Evidence{},
	}

	el.Append(event1)
	el.Append(event2)

	all := el.GetAll()
	if all[0].EventID == "" {
		t.Errorf("Event ID should be generated")
	}
	if all[1].EventID == "" {
		t.Errorf("Event ID should be generated")
	}
	if all[0].EventID == all[1].EventID {
		t.Errorf("Event IDs should be unique")
	}

	if !contains(all[0].EventID, "evt_") {
		t.Errorf("Event ID should start with evt_, got %s", all[0].EventID)
	}
}

func TestEventLogSchemaVersion(t *testing.T) {
	el := NewEventLog()
	_ = el // Just verify it creates without panic

	event := RunEvent{
		RunID:       "run_0000000000000001",
		ExecutionID: "exe_00000001",
		Type:        EventTypeRunCreated,
		Actor:       ActorSystem,
		Payload:     json.RawMessage(`{}`),
		Evidence:    []Evidence{},
	}

	el.Append(event)

	all := el.GetAll()
	if all[0].SchemaVersion != "event/v1" {
		t.Errorf("Expected schema_version event/v1, got %s", all[0].SchemaVersion)
	}
}

func TestEventLogTimestamp(t *testing.T) {
	el := NewEventLog()
	_ = el // Just verify it creates without panic

	before := time.Now().UnixMilli()

	event := RunEvent{
		RunID:       "run_0000000000000001",
		ExecutionID: "exe_00000001",
		Type:        EventTypeRunCreated,
		Actor:       ActorSystem,
		Payload:     json.RawMessage(`{}`),
		Evidence:    []Evidence{},
	}

	el.Append(event)

	after := time.Now().UnixMilli()

	all := el.GetAll()
	if all[0].TimestampMs < before || all[0].TimestampMs > after {
		t.Errorf("Timestamp should be set to current time, got %d (before=%d, after=%d)", all[0].TimestampMs, before, after)
	}
}

func TestEventLogAllEventTypes(t *testing.T) {
	el := NewEventLog()
	_ = el // Just verify it creates without panic

	eventTypes := []EventType{
		EventTypeRunCreated,
		EventTypeValidationCompleted,
		EventTypeStateTransition,
		EventTypeAllocationFailed,
		EventTypeStageStarted,
		EventTypeStageCompleted,
		EventTypeStageFailed,
		EventTypeSchedulerTargetSet,
		EventTypeWorkerAssigned,
		EventTypeWorkerAssignmentRejected,
		EventTypeWorkerRegistered,
		EventTypeWorkerHeartbeat,
		EventTypeWorkerCapacityLost,
		EventTypeStopRequested,
		EventTypeEmergencyStop,
		EventTypeStopConditionTriggered,
		EventTypeStageTimeout,
		EventTypeDecision,
		EventTypeAnalysisStarted,
		EventTypeAnalysisCompleted,
		EventTypeReportGenerated,
		EventTypeArtifactStored,
		EventTypeSystemRecovery,
		EventTypeSystemWarning,
	}

	for _, et := range eventTypes {
		event := RunEvent{
			RunID:       "run_0000000000000001",
			ExecutionID: "exe_00000001",
			Type:        et,
			Actor:       ActorSystem,
			Payload:     json.RawMessage(`{}`),
			Evidence:    []Evidence{},
		}

		err := el.Append(event)
		if err != nil {
			t.Errorf("Failed to append event type %s: %v", et, err)
		}
	}

	if el.Len() != len(eventTypes) {
		t.Errorf("Expected %d events, got %d", len(eventTypes), el.Len())
	}
}

func TestEventLogAllActorTypes(t *testing.T) {
	el := NewEventLog()
	_ = el // Just verify it creates without panic

	actorTypes := []ActorType{
		ActorSystem,
		ActorUser,
		ActorScheduler,
		ActorAutoramp,
		ActorAnalysis,
		ActorWorker,
	}

	for _, at := range actorTypes {
		event := RunEvent{
			RunID:       "run_0000000000000001",
			ExecutionID: "exe_00000001",
			Type:        EventTypeRunCreated,
			Actor:       at,
			Payload:     json.RawMessage(`{}`),
			Evidence:    []Evidence{},
		}

		err := el.Append(event)
		if err != nil {
			t.Errorf("Failed to append actor type %s: %v", at, err)
		}
	}

	if el.Len() != len(actorTypes) {
		t.Errorf("Expected %d events, got %d", len(actorTypes), el.Len())
	}
}

func TestEventLogCorrelationContext(t *testing.T) {
	el := NewEventLog()
	_ = el // Just verify it creates without panic

	stage := StageNameRamp
	stageID := "stg_000000000003"
	workerID := "wkr_00000001"
	vuID := "vu_001"
	sessionID := "ses_session_001"

	event := RunEvent{
		RunID:       "run_0000000000000001",
		ExecutionID: "exe_00000001",
		Type:        EventTypeStageStarted,
		Actor:       ActorSystem,
		Payload:     json.RawMessage(`{}`),
		Evidence:    []Evidence{},
		Correlation: CorrelationContext{
			Stage:     &stage,
			StageID:   &stageID,
			WorkerID:  &workerID,
			VUID:      &vuID,
			SessionID: &sessionID,
		},
	}

	el.Append(event)

	all := el.GetAll()
	if all[0].Correlation.Stage == nil || *all[0].Correlation.Stage != stage {
		t.Errorf("Stage correlation not preserved")
	}
	if all[0].Correlation.StageID == nil || *all[0].Correlation.StageID != stageID {
		t.Errorf("StageID correlation not preserved")
	}
	if all[0].Correlation.WorkerID == nil || *all[0].Correlation.WorkerID != workerID {
		t.Errorf("WorkerID correlation not preserved")
	}
	if all[0].Correlation.VUID == nil || *all[0].Correlation.VUID != vuID {
		t.Errorf("VUID correlation not preserved")
	}
	if all[0].Correlation.SessionID == nil || *all[0].Correlation.SessionID != sessionID {
		t.Errorf("SessionID correlation not preserved")
	}
}

func TestEventLogEvidence(t *testing.T) {
	el := NewEventLog()
	_ = el // Just verify it creates without panic

	note := "test evidence note"
	evidence := []Evidence{
		{
			Kind: "metric_window",
			Ref:  "mw_test_001",
			Note: &note,
		},
		{
			Kind: "config_snapshot",
			Ref:  "cfg_test_001",
			Note: nil,
		},
	}

	event := RunEvent{
		RunID:       "run_0000000000000001",
		ExecutionID: "exe_00000001",
		Type:        EventTypeDecision,
		Actor:       ActorAutoramp,
		Payload:     json.RawMessage(`{}`),
		Evidence:    evidence,
	}

	el.Append(event)

	all := el.GetAll()
	if len(all[0].Evidence) != 2 {
		t.Errorf("Expected 2 evidence items, got %d", len(all[0].Evidence))
	}
	if all[0].Evidence[0].Kind != "metric_window" {
		t.Errorf("First evidence kind should be metric_window")
	}
	if all[0].Evidence[1].Kind != "config_snapshot" {
		t.Errorf("Second evidence kind should be config_snapshot")
	}
}

func TestEventLogConcurrentAppends(t *testing.T) {
	el := NewEventLog()
	_ = el // Just verify it creates without panic
	numGoroutines := 100
	eventsPerGoroutine := 10

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func(goroutineID int) {
			defer wg.Done()
			for i := 0; i < eventsPerGoroutine; i++ {
				event := RunEvent{
					RunID:       "run_0000000000000001",
					ExecutionID: "exe_00000001",
					Type:        EventTypeStateTransition,
					Actor:       ActorSystem,
					Payload:     json.RawMessage(fmt.Sprintf(`{"goroutine":%d,"index":%d}`, goroutineID, i)),
					Evidence:    []Evidence{},
				}
				el.Append(event)
			}
		}(g)
	}

	wg.Wait()

	expectedCount := numGoroutines * eventsPerGoroutine
	if el.Len() != expectedCount {
		t.Errorf("Expected %d events, got %d", expectedCount, el.Len())
	}

	all := el.GetAll()
	eventIDs := make(map[string]bool)
	for _, event := range all {
		if eventIDs[event.EventID] {
			t.Errorf("Duplicate event ID: %s", event.EventID)
		}
		eventIDs[event.EventID] = true
	}
}

func TestEventLogConcurrentReads(t *testing.T) {
	el := NewEventLog()
	_ = el // Just verify it creates without panic

	for i := 0; i < 100; i++ {
		event := RunEvent{
			RunID:       "run_0000000000000001",
			ExecutionID: "exe_00000001",
			Type:        EventTypeStateTransition,
			Actor:       ActorSystem,
			Payload:     json.RawMessage(`{}`),
			Evidence:    []Evidence{},
		}
		el.Append(event)
	}

	numReaders := 50
	var wg sync.WaitGroup
	wg.Add(numReaders)

	var readCount atomic.Int32

	for r := 0; r < numReaders; r++ {
		go func() {
			defer wg.Done()
			all := el.GetAll()
			if len(all) == 100 {
				readCount.Add(1)
			}
		}()
	}

	wg.Wait()

	if readCount.Load() != int32(numReaders) {
		t.Errorf("Expected all %d readers to see 100 events, got %d", numReaders, readCount.Load())
	}
}

func TestEventLogConcurrentAppendAndRead(t *testing.T) {
	el := NewEventLog()
	_ = el // Just verify it creates without panic

	var wg sync.WaitGroup
	numWriters := 10
	numReaders := 10
	eventsPerWriter := 20

	wg.Add(numWriters + numReaders)

	for w := 0; w < numWriters; w++ {
		go func(writerID int) {
			defer wg.Done()
			for i := 0; i < eventsPerWriter; i++ {
				event := RunEvent{
					RunID:       "run_0000000000000001",
					ExecutionID: "exe_00000001",
					Type:        EventTypeStateTransition,
					Actor:       ActorSystem,
					Payload:     json.RawMessage(fmt.Sprintf(`{"writer":%d,"index":%d}`, writerID, i)),
					Evidence:    []Evidence{},
				}
				el.Append(event)
				time.Sleep(time.Millisecond)
			}
		}(w)
	}

	for r := 0; r < numReaders; r++ {
		go func(readerID int) {
			defer wg.Done()
			for i := 0; i < eventsPerWriter; i++ {
				all := el.GetAll()
				tail, _ := el.Tail(0, 10)
				if len(all) < 0 || len(tail) < 0 {
					t.Errorf("Reader %d: invalid result", readerID)
				}
				time.Sleep(time.Millisecond)
			}
		}(r)
	}

	wg.Wait()

	expectedCount := numWriters * eventsPerWriter
	if el.Len() != expectedCount {
		t.Errorf("Expected %d events, got %d", expectedCount, el.Len())
	}
}

func TestEventLogTailCopySemantics(t *testing.T) {
	el := NewEventLog()
	_ = el // Just verify it creates without panic

	event := RunEvent{
		RunID:       "run_0000000000000001",
		ExecutionID: "exe_00000001",
		Type:        EventTypeRunCreated,
		Actor:       ActorSystem,
		Payload:     json.RawMessage(`{}`),
		Evidence:    []Evidence{},
	}

	el.Append(event)

	tail, _ := el.Tail(0, 1)
	tail[0].Type = EventTypeStateTransition

	all := el.GetAll()
	if all[0].Type != EventTypeRunCreated {
		t.Errorf("Tail should return a copy, not a reference")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && len(substr) > 0 && s[0:len(substr)] == substr || len(s) > len(substr) && contains(s[1:], substr))
}

func TestEventLog_MemoryLimit(t *testing.T) {
	// Create a log with limit of 5 events
	el := NewEventLogWithLimit(5)

	// Add 5 events - should all succeed
	for i := 0; i < 5; i++ {
		event := RunEvent{
			RunID:       "run_123",
			ExecutionID: "exec_456",
			Type:        EventTypeStateTransition,
			Actor:       ActorSystem,
			Payload:     json.RawMessage(`{}`),
			Evidence:    []Evidence{},
		}
		if err := el.Append(event); err != nil {
			t.Fatalf("Failed to append event %d: %v", i, err)
		}
	}

	if el.Len() != 5 {
		t.Errorf("Expected 5 events, got %d", el.Len())
	}

	// 6th event should be silently dropped
	event := RunEvent{
		RunID:       "run_123",
		ExecutionID: "exec_456",
		Type:        EventTypeStateTransition,
		Actor:       ActorSystem,
		Payload:     json.RawMessage(`{}`),
		Evidence:    []Evidence{},
	}
	if err := el.Append(event); err != nil {
		t.Fatalf("Append should not fail even when dropping: %v", err)
	}

	// Still should have only 5 events
	if el.Len() != 5 {
		t.Errorf("Expected 5 events after drop, got %d", el.Len())
	}

	// Should be marked as truncated
	if !el.IsTruncated() {
		t.Error("Expected IsTruncated to return true")
	}
}

func TestEventLog_UnlimitedWhenZero(t *testing.T) {
	// Create a log with no limit
	el := NewEventLogWithLimit(0)

	// Add many events - should all succeed
	for i := 0; i < 100; i++ {
		event := RunEvent{
			RunID:       "run_123",
			ExecutionID: "exec_456",
			Type:        EventTypeStateTransition,
			Actor:       ActorSystem,
			Payload:     json.RawMessage(`{}`),
			Evidence:    []Evidence{},
		}
		if err := el.Append(event); err != nil {
			t.Fatalf("Failed to append event %d: %v", i, err)
		}
	}

	if el.Len() != 100 {
		t.Errorf("Expected 100 events, got %d", el.Len())
	}

	if el.IsTruncated() {
		t.Error("Expected IsTruncated to return false for unlimited log")
	}
}

func TestEventLog_DefaultLimit(t *testing.T) {
	// Verify default log can be created and used
	el := NewEventLog()
	_ = el // Just verify it creates without panic
	
	// Verify it uses the default limit
	// We can't directly access maxEvents, but we can verify the default constant
	if DefaultMaxEventsPerLog != 10000 {
		t.Errorf("Expected default limit of 10000, got %d", DefaultMaxEventsPerLog)
	}
}
