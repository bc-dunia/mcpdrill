package metrics

import (
	"testing"
	"time"
)

func TestConnectionTrackerGetStabilityMetricsIncludeFlags(t *testing.T) {
	ct := NewConnectionTracker()
	base := time.Unix(1700000000, 0).UTC()
	now := base
	ct.nowFunc = func() time.Time { return now }
	ct.startTime = base.Add(-2 * time.Minute)

	ct.RecordEvent(ConnectionEvent{
		SessionID: "sess_1",
		EventType: EventTypeCreated,
		Timestamp: base,
	})
	ct.RecordEvent(ConnectionEvent{
		SessionID: "sess_1",
		EventType: EventTypeActive,
		Timestamp: base.Add(5 * time.Second),
	})
	ct.RecordSuccess("sess_1", 100)
	ct.RecordError("sess_1", true)
	ct.RecordTimePoint(StabilityTimePoint{
		Timestamp:      base.UnixMilli(),
		ActiveSessions: 1,
		NewSessions:    1,
	})
	ct.RecordEvent(ConnectionEvent{
		SessionID: "sess_1",
		EventType: EventTypeDropped,
		Timestamp: base.Add(10 * time.Second),
		Reason:    DropReasonNetwork,
	})
	now = base.Add(20 * time.Second)

	withoutOptional := ct.GetStabilityMetrics(false, false)
	if withoutOptional == nil {
		t.Fatal("expected stability metrics")
	}
	if withoutOptional.TotalSessions != 1 {
		t.Fatalf("expected total sessions 1, got %d", withoutOptional.TotalSessions)
	}
	if withoutOptional.DroppedSessions != 1 {
		t.Fatalf("expected dropped sessions 1, got %d", withoutOptional.DroppedSessions)
	}
	if len(withoutOptional.Events) != 0 {
		t.Fatalf("expected no events when includeEvents=false, got %d", len(withoutOptional.Events))
	}
	if len(withoutOptional.TimeSeriesData) != 0 {
		t.Fatalf("expected no time series when includeTimeSeries=false, got %d", len(withoutOptional.TimeSeriesData))
	}

	withOptional := ct.GetStabilityMetrics(true, true)
	if withOptional == nil {
		t.Fatal("expected stability metrics")
	}
	if len(withOptional.Events) == 0 {
		t.Fatal("expected events when includeEvents=true")
	}
	if len(withOptional.TimeSeriesData) != 1 {
		t.Fatalf("expected 1 time series point, got %d", len(withOptional.TimeSeriesData))
	}
	if withOptional.ProtocolErrorRate <= 0 {
		t.Fatalf("expected protocol error rate > 0, got %f", withOptional.ProtocolErrorRate)
	}
}

func TestConnectionTrackerGetStabilityMetricsReturnsCopies(t *testing.T) {
	ct := NewConnectionTracker()
	base := time.Unix(1700000100, 0).UTC()
	ct.nowFunc = func() time.Time { return base.Add(5 * time.Second) }
	ct.startTime = base.Add(-time.Minute)

	ct.RecordEvent(ConnectionEvent{
		SessionID: "sess_1",
		EventType: EventTypeCreated,
		Timestamp: base,
	})
	ct.RecordEvent(ConnectionEvent{
		SessionID: "sess_1",
		EventType: EventTypeDropped,
		Timestamp: base.Add(2 * time.Second),
		Reason:    DropReasonTimeout,
	})
	ct.RecordTimePoint(StabilityTimePoint{
		Timestamp:      base.UnixMilli(),
		ActiveSessions: 1,
	})

	first := ct.GetStabilityMetrics(true, true)
	if first == nil {
		t.Fatal("expected stability metrics")
	}
	if len(first.Events) == 0 || len(first.SessionMetrics) == 0 || len(first.TimeSeriesData) == 0 {
		t.Fatal("expected events, session metrics and time series data")
	}

	first.Events[0].SessionID = "mutated_event"
	first.SessionMetrics[0].SessionID = "mutated_session"
	first.TimeSeriesData[0].Timestamp = 0

	second := ct.GetStabilityMetrics(true, true)
	if second == nil {
		t.Fatal("expected stability metrics")
	}
	if len(second.Events) == 0 || len(second.SessionMetrics) == 0 || len(second.TimeSeriesData) == 0 {
		t.Fatal("expected events, session metrics and time series data")
	}
	if second.Events[0].SessionID == "mutated_event" {
		t.Fatal("events should be returned as copy")
	}
	if second.SessionMetrics[0].SessionID == "mutated_session" {
		t.Fatal("session metrics should be returned as copy")
	}
	if second.TimeSeriesData[0].Timestamp == 0 {
		t.Fatal("time series should be returned as copy")
	}
}
