// Package metrics provides connection stability tracking for MCP Drill.
package metrics

import (
	"sync"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/config"
)

// ConnectionEventType represents the type of connection event.
type ConnectionEventType string

const (
	EventTypeCreated    ConnectionEventType = "created"
	EventTypeActive     ConnectionEventType = "active"
	EventTypeDropped    ConnectionEventType = "dropped"
	EventTypeTerminated ConnectionEventType = "terminated"
	EventTypeReconnect  ConnectionEventType = "reconnect"
)

// DropReason represents the reason for a connection drop.
type DropReason string

const (
	DropReasonTimeout     DropReason = "timeout"
	DropReasonServerError DropReason = "server_error"
	DropReasonClientClose DropReason = "client_close"
	DropReasonProtocol    DropReason = "protocol_error"
	DropReasonNetwork     DropReason = "network_error"
	DropReasonUnknown     DropReason = "unknown"
)

// ConnectionEvent represents a single connection lifecycle event.
type ConnectionEvent struct {
	SessionID  string              `json:"session_id"`
	EventType  ConnectionEventType `json:"event_type"`
	Timestamp  time.Time           `json:"timestamp"`
	Reason     DropReason          `json:"reason,omitempty"`
	DurationMs int64               `json:"duration_ms,omitempty"`
}

// ConnectionMetrics holds metrics for a single connection/session.
type ConnectionMetrics struct {
	SessionID      string     `json:"session_id"`
	CreatedAt      time.Time  `json:"created_at"`
	LastActiveAt   time.Time  `json:"last_active_at"`
	TerminatedAt   *time.Time `json:"terminated_at,omitempty"`
	RequestCount   int64      `json:"request_count"`
	SuccessCount   int64      `json:"success_count"`
	ErrorCount     int64      `json:"error_count"`
	ReconnectCount int32      `json:"reconnect_count"`
	ProtocolErrors int32      `json:"protocol_errors"`
	AvgLatencyMs   float64    `json:"avg_latency_ms"`
	State          string     `json:"state"`
}

// StabilityMetrics contains aggregated connection stability data.
type StabilityMetrics struct {
	TotalSessions        int64                `json:"total_sessions"`
	ActiveSessions       int64                `json:"active_sessions"`
	DroppedSessions      int64                `json:"dropped_sessions"`
	TerminatedSessions   int64                `json:"terminated_sessions"`
	AvgSessionLifetimeMs float64              `json:"avg_session_lifetime_ms"`
	ReconnectRate        float64              `json:"reconnect_rate"`
	ProtocolErrorRate    float64              `json:"protocol_error_rate"`
	ConnectionChurnRate  float64              `json:"connection_churn_rate"`
	StabilityScore       float64              `json:"stability_score"`
	DropRate             float64              `json:"drop_rate"`
	Events               []ConnectionEvent    `json:"events,omitempty"`
	SessionMetrics       []ConnectionMetrics  `json:"session_metrics,omitempty"`
	TimeSeriesData       []StabilityTimePoint `json:"time_series,omitempty"`
}

// StabilityTimePoint represents a point-in-time snapshot of connection stability.
type StabilityTimePoint struct {
	Timestamp       int64   `json:"timestamp"`
	ActiveSessions  int32   `json:"active_sessions"`
	NewSessions     int32   `json:"new_sessions"`
	DroppedSessions int32   `json:"dropped_sessions"`
	Reconnects      int32   `json:"reconnects"`
	AvgSessionAge   float64 `json:"avg_session_age_ms"`
}

// MetricsTimePoint represents a point-in-time snapshot of performance metrics.
type MetricsTimePoint struct {
	Timestamp   int64   `json:"timestamp"`
	SuccessOps  int64   `json:"success_ops"`
	FailedOps   int64   `json:"failed_ops"`
	Throughput  float64 `json:"throughput"`
	LatencyP50  float64 `json:"latency_p50"`
	LatencyP95  float64 `json:"latency_p95"`
	LatencyP99  float64 `json:"latency_p99"`
	LatencyMean float64 `json:"latency_mean"`
	ErrorRate   float64 `json:"error_rate"`
}

// ConnectionTracker tracks connection events and computes stability metrics.
type ConnectionTracker struct {
	mu sync.RWMutex

	events        []ConnectionEvent
	maxEvents     int
	sessions      map[string]*ConnectionMetrics
	timeSeries    []StabilityTimePoint
	maxTimeSeries int

	totalCreated        int64
	totalDropped        int64
	totalTerminated     int64
	totalReconnects     int64
	totalProtocolErrors int64
	totalRequests       int64

	startTime time.Time
	nowFunc   func() time.Time
}

// NewConnectionTracker creates a new ConnectionTracker.
func NewConnectionTracker() *ConnectionTracker {
	return &ConnectionTracker{
		events:        make([]ConnectionEvent, 0, config.DefaultEventBufferSize),
		maxEvents:     config.DefaultEventBufferSize,
		sessions:      make(map[string]*ConnectionMetrics),
		timeSeries:    make([]StabilityTimePoint, 0, 3600),
		maxTimeSeries: 3600,
		startTime:     time.Now(),
		nowFunc:       time.Now,
	}
}

// RecordEvent records a connection event.
func (ct *ConnectionTracker) RecordEvent(event ConnectionEvent) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	if event.Timestamp.IsZero() {
		event.Timestamp = ct.nowFunc()
	}

	if len(ct.events) >= ct.maxEvents {
		ct.events = ct.events[1:]
	}
	ct.events = append(ct.events, event)

	switch event.EventType {
	case EventTypeCreated:
		ct.totalCreated++
		ct.sessions[event.SessionID] = &ConnectionMetrics{
			SessionID:    event.SessionID,
			CreatedAt:    event.Timestamp,
			LastActiveAt: event.Timestamp,
			State:        "active",
		}

	case EventTypeActive:
		if session, ok := ct.sessions[event.SessionID]; ok {
			session.LastActiveAt = event.Timestamp
			session.RequestCount++
			ct.totalRequests++
		}

	case EventTypeDropped:
		ct.totalDropped++
		if session, ok := ct.sessions[event.SessionID]; ok {
			session.State = "dropped"
			t := event.Timestamp
			session.TerminatedAt = &t
		}

	case EventTypeTerminated:
		ct.totalTerminated++
		if session, ok := ct.sessions[event.SessionID]; ok {
			session.State = "terminated"
			t := event.Timestamp
			session.TerminatedAt = &t
		}

	case EventTypeReconnect:
		ct.totalReconnects++
		if session, ok := ct.sessions[event.SessionID]; ok {
			session.ReconnectCount++
		}
	}
}

// RecordSuccess records a successful operation for a session.
func (ct *ConnectionTracker) RecordSuccess(sessionID string, latencyMs int64) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	if session, ok := ct.sessions[sessionID]; ok {
		session.SuccessCount++
		session.LastActiveAt = ct.nowFunc()
		session.AvgLatencyMs = (session.AvgLatencyMs*float64(session.SuccessCount-1) + float64(latencyMs)) / float64(session.SuccessCount)
	}
}

// RecordError records a failed operation for a session.
func (ct *ConnectionTracker) RecordError(sessionID string, isProtocolError bool) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	if session, ok := ct.sessions[sessionID]; ok {
		session.ErrorCount++
		session.LastActiveAt = ct.nowFunc()
		if isProtocolError {
			session.ProtocolErrors++
			ct.totalProtocolErrors++
		}
	}
}

// RecordTimePoint records a time-series data point.
func (ct *ConnectionTracker) RecordTimePoint(point StabilityTimePoint) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	if len(ct.timeSeries) >= ct.maxTimeSeries {
		ct.timeSeries = ct.timeSeries[1:]
	}
	ct.timeSeries = append(ct.timeSeries, point)
}

// GetStabilityMetrics computes and returns current stability metrics.
func (ct *ConnectionTracker) GetStabilityMetrics(includeEvents bool, includeTimeSeries bool) *StabilityMetrics {
	ct.mu.RLock()
	now := ct.nowFunc()
	startTime := ct.startTime
	totalCreated := ct.totalCreated
	totalDropped := ct.totalDropped
	totalTerminated := ct.totalTerminated
	totalReconnects := ct.totalReconnects
	totalProtocolErrors := ct.totalProtocolErrors
	totalRequests := ct.totalRequests

	sessionList := make([]ConnectionMetrics, 0, len(ct.sessions))
	for _, session := range ct.sessions {
		sessionList = append(sessionList, *session)
	}

	var events []ConnectionEvent
	if includeEvents {
		events = make([]ConnectionEvent, len(ct.events))
		copy(events, ct.events)
	}

	var timeSeries []StabilityTimePoint
	if includeTimeSeries {
		timeSeries = make([]StabilityTimePoint, len(ct.timeSeries))
		copy(timeSeries, ct.timeSeries)
	}
	ct.mu.RUnlock()

	elapsedMinutes := now.Sub(startTime).Minutes()
	if elapsedMinutes < 1 {
		elapsedMinutes = 1
	}

	var activeCount int64
	var totalLifetimeMs float64
	var sessionLifetimeCount int

	for i := range sessionList {
		session := &sessionList[i]
		if session.State == "active" {
			activeCount++
			lifetime := now.Sub(session.CreatedAt).Milliseconds()
			totalLifetimeMs += float64(lifetime)
			sessionLifetimeCount++
		} else if session.TerminatedAt != nil {
			lifetime := session.TerminatedAt.Sub(session.CreatedAt).Milliseconds()
			totalLifetimeMs += float64(lifetime)
			sessionLifetimeCount++
		}
	}

	avgLifetimeMs := float64(0)
	if sessionLifetimeCount > 0 {
		avgLifetimeMs = totalLifetimeMs / float64(sessionLifetimeCount)
	}

	reconnectRate := float64(0)
	if totalCreated > 0 {
		reconnectRate = float64(totalReconnects) / float64(totalCreated)
	}

	protocolErrorRate := float64(0)
	if totalRequests > 0 {
		protocolErrorRate = float64(totalProtocolErrors) / float64(totalRequests)
	}

	churnRate := float64(totalCreated) / elapsedMinutes

	dropRate := float64(0)
	if totalCreated > 0 {
		dropRate = float64(totalDropped) / float64(totalCreated)
	}

	stabilityScore := 100.0 - (dropRate*50 + reconnectRate*30 + protocolErrorRate*20)
	if stabilityScore < 0 {
		stabilityScore = 0
	}
	if stabilityScore > 100 {
		stabilityScore = 100
	}

	metrics := &StabilityMetrics{
		TotalSessions:        totalCreated,
		ActiveSessions:       activeCount,
		DroppedSessions:      totalDropped,
		TerminatedSessions:   totalTerminated,
		AvgSessionLifetimeMs: avgLifetimeMs,
		ReconnectRate:        reconnectRate,
		ProtocolErrorRate:    protocolErrorRate,
		ConnectionChurnRate:  churnRate,
		StabilityScore:       stabilityScore,
		DropRate:             dropRate,
		SessionMetrics:       sessionList,
	}

	if includeEvents {
		metrics.Events = events
	}

	if includeTimeSeries {
		metrics.TimeSeriesData = timeSeries
	}

	return metrics
}

// Reset clears all tracking data.
func (ct *ConnectionTracker) Reset() {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	ct.events = ct.events[:0]
	ct.sessions = make(map[string]*ConnectionMetrics)
	ct.timeSeries = ct.timeSeries[:0]
	ct.totalCreated = 0
	ct.totalDropped = 0
	ct.totalTerminated = 0
	ct.totalReconnects = 0
	ct.totalProtocolErrors = 0
	ct.totalRequests = 0
	ct.startTime = ct.nowFunc()
}

// GetRecentEvents returns the most recent N events.
func (ct *ConnectionTracker) GetRecentEvents(n int) []ConnectionEvent {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	if n <= 0 || len(ct.events) == 0 {
		return nil
	}

	start := len(ct.events) - n
	if start < 0 {
		start = 0
	}

	result := make([]ConnectionEvent, len(ct.events)-start)
	copy(result, ct.events[start:])
	return result
}

// GetSessionMetrics returns metrics for a specific session.
func (ct *ConnectionTracker) GetSessionMetrics(sessionID string) *ConnectionMetrics {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	if session, ok := ct.sessions[sessionID]; ok {
		copy := *session
		return &copy
	}
	return nil
}
