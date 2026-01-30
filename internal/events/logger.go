package events

import (
	"io"
	"log/slog"
	"os"
	"sync"
)

// EventLogger provides structured logging for key events in mcpdrill.
type EventLogger struct {
	logger   *slog.Logger
	runID    string
	workerID string
}

// NewEventLogger creates a new EventLogger with JSON output to stdout.
// It includes base attributes: run_id and worker_id.
func NewEventLogger(runID, workerID string) *EventLogger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	logger := slog.New(handler).With(
		"run_id", runID,
		"worker_id", workerID,
	)
	return &EventLogger{
		logger:   logger,
		runID:    runID,
		workerID: workerID,
	}
}

// NewEventLoggerWithWriter creates a new EventLogger with JSON output to a custom writer.
// Useful for testing or redirecting output.
func NewEventLoggerWithWriter(runID, workerID string, w io.Writer) *EventLogger {
	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	logger := slog.New(handler).With(
		"run_id", runID,
		"worker_id", workerID,
	)
	return &EventLogger{
		logger:   logger,
		runID:    runID,
		workerID: workerID,
	}
}

// LogReconnect logs a reconnection event.
// event: "reconnect"
// Attributes: session_id, attempt, reason, backoff_ms
func (el *EventLogger) LogReconnect(sessionID string, attempt int, reason string, backoffMs int64) {
	el.logger.Info("reconnect",
		"session_id", sessionID,
		"attempt", attempt,
		"reason", reason,
		"backoff_ms", backoffMs,
	)
}

// LogStallTrigger logs when a stream stall is detected.
// event: "stall_trigger"
// Attributes: session_id, stall_seconds, threshold_seconds
func (el *EventLogger) LogStallTrigger(sessionID string, stallSeconds float64, threshold float64) {
	el.logger.Warn("stall_trigger",
		"session_id", sessionID,
		"stall_seconds", stallSeconds,
		"threshold_seconds", threshold,
	)
}

// LogStopCondition logs when a stop condition is activated.
// event: "stop_condition_activated"
// Attributes: stage_id, metric, value, threshold, reason
func (el *EventLogger) LogStopCondition(stageID, metric string, value, threshold float64, reason string) {
	el.logger.Warn("stop_condition_activated",
		"stage_id", stageID,
		"metric", metric,
		"value", value,
		"threshold", threshold,
		"reason", reason,
	)
}

// LogStageTransition logs a transition between stages.
// event: "stage_transition"
// Attributes: from_stage, to_stage, stage_id, reason
func (el *EventLogger) LogStageTransition(fromStage, toStage, stageID, reason string) {
	el.logger.Info("stage_transition",
		"from_stage", fromStage,
		"to_stage", toStage,
		"stage_id", stageID,
		"reason", reason,
	)
}

// LogSessionCreated logs when a session is created.
// event: "session_created"
// Attributes: session_id, mode
func (el *EventLogger) LogSessionCreated(sessionID, mode string) {
	el.logger.Info("session_created",
		"session_id", sessionID,
		"mode", mode,
	)
}

// LogSessionDestroyed logs when a session is destroyed.
// event: "session_destroyed"
// Attributes: session_id, reason, lifetime_ms
func (el *EventLogger) LogSessionDestroyed(sessionID, reason string, lifetimeMs int64) {
	el.logger.Info("session_destroyed",
		"session_id", sessionID,
		"reason", reason,
		"lifetime_ms", lifetimeMs,
	)
}

// Global logger management
var (
	globalLogger *EventLogger
	globalMu     sync.RWMutex
)

// SetGlobalEventLogger sets the global event logger instance.
func SetGlobalEventLogger(l *EventLogger) {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalLogger = l
}

// GetGlobalEventLogger returns the global event logger instance.
// If no logger is set, returns a no-op logger.
func GetGlobalEventLogger() *EventLogger {
	globalMu.RLock()
	defer globalMu.RUnlock()
	if globalLogger != nil {
		return globalLogger
	}
	return NoopEventLogger()
}

// NoopEventLogger returns an event logger that discards all events.
// Useful for testing or when event logging is disabled.
func NoopEventLogger() *EventLogger {
	handler := slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	logger := slog.New(handler)
	return &EventLogger{
		logger:   logger,
		runID:    "",
		workerID: "",
	}
}
