package api

import (
	"fmt"
	"log/slog"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/analysis"
	"github.com/bc-dunia/mcpdrill/internal/controlplane/runmanager"
	"github.com/bc-dunia/mcpdrill/internal/metrics"
	"github.com/bc-dunia/mcpdrill/internal/telemetry"
)

// TelemetryStoreConfig configures memory limits for the telemetry store.
type TelemetryStoreConfig struct {
	// MaxOperationsPerRun limits operations stored per run. 0 = unlimited.
	MaxOperationsPerRun int
	// MaxLogsPerRun limits logs stored per run. 0 = unlimited.
	MaxLogsPerRun int
	// MaxTotalRuns limits total runs in memory. 0 = unlimited.
	// When exceeded, oldest runs are evicted.
	MaxTotalRuns int
}

// DefaultTelemetryStoreConfig returns sensible defaults.
func DefaultTelemetryStoreConfig() *TelemetryStoreConfig {
	return &TelemetryStoreConfig{
		MaxOperationsPerRun: 100000, // 100K operations per run
		MaxLogsPerRun:       100000, // 100K logs per run
		MaxTotalRuns:        100,    // 100 runs max in memory
	}
}

type TelemetryStore struct {
	mu     sync.RWMutex
	runs   map[string]*runTelemetry
	config *TelemetryStoreConfig
	// runOrder tracks insertion order for LRU eviction
	runOrder []string
}

type runTelemetry struct {
	runID       string
	scenarioID  string
	startTimeMs int64
	endTimeMs   int64
	stopReason  string
	operations  []analysis.OperationResult
	logs        []OperationLog
	// truncated flags indicate if data was dropped due to limits
	operationsTruncated bool
	logsTruncated       bool
}

func NewTelemetryStore() *TelemetryStore {
	return NewTelemetryStoreWithConfig(DefaultTelemetryStoreConfig())
}

func NewTelemetryStoreWithConfig(config *TelemetryStoreConfig) *TelemetryStore {
	if config == nil {
		config = DefaultTelemetryStoreConfig()
	}
	return &TelemetryStore{
		runs:     make(map[string]*runTelemetry),
		config:   config,
		runOrder: make([]string, 0),
	}
}

func (ts *TelemetryStore) AddTelemetryBatch(runID string, batch TelemetryBatchRequest) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	rt := ts.getOrCreateRunTelemetry(runID)

	for _, op := range batch.Operations {
		if rt.startTimeMs == 0 || op.TimestampMs < rt.startTimeMs {
			rt.startTimeMs = op.TimestampMs
		}
		if op.TimestampMs > rt.endTimeMs {
			rt.endTimeMs = op.TimestampMs
		}

		// Check operations limit
		if ts.config.MaxOperationsPerRun > 0 && len(rt.operations) >= ts.config.MaxOperationsPerRun {
			if !rt.operationsTruncated {
				rt.operationsTruncated = true
				slog.Warn("telemetry_operations_truncated",
					"run_id", runID,
					"limit", ts.config.MaxOperationsPerRun)
			}
		} else {
			result := analysis.OperationResult{
				Operation: op.Operation,
				ToolName:  op.ToolName,
				LatencyMs: op.LatencyMs,
				OK:        op.OK,
				ErrorType: op.ErrorType,
			}
			rt.operations = append(rt.operations, result)
		}

		// Use explicit stage from operation (Issue #19: no more guessing)
		stage := op.Stage
		if stage == "" {
			stage = extractStageName(op.StageID) // Fallback for backward compatibility
		}

		// Check logs limit
		if ts.config.MaxLogsPerRun > 0 && len(rt.logs) >= ts.config.MaxLogsPerRun {
			if !rt.logsTruncated {
				rt.logsTruncated = true
				slog.Warn("telemetry_logs_truncated",
					"run_id", runID,
					"limit", ts.config.MaxLogsPerRun)
			}
		} else {
			log := OperationLog{
				TimestampMs: op.TimestampMs,
				RunID:       runID,
				Stage:       stage,
				StageID:     op.StageID,
				WorkerID:    op.WorkerID,
				VUID:        op.VUID,
				SessionID:   op.SessionID,
				Operation:   op.Operation,
				ToolName:    op.ToolName,
				LatencyMs:   op.LatencyMs,
				OK:          op.OK,
				ErrorType:   op.ErrorType,
				ErrorCode:   op.ErrorCode,
				Stream:      op.Stream,
			}
			rt.logs = append(rt.logs, log)
		}
	}
}

// evictIfNeeded removes oldest runs if MaxTotalRuns is exceeded.
// Must be called with lock held.
func (ts *TelemetryStore) evictIfNeeded() {
	if ts.config.MaxTotalRuns <= 0 {
		return
	}

	for len(ts.runs) >= ts.config.MaxTotalRuns && len(ts.runOrder) > 0 {
		// Evict oldest run
		oldestID := ts.runOrder[0]
		ts.runOrder = ts.runOrder[1:]
		delete(ts.runs, oldestID)
		slog.Info("telemetry_run_evicted", "run_id", oldestID, "reason", "max_runs_exceeded")
	}
}

// getOrCreateRunTelemetry returns the run telemetry entry, creating it if needed.
// Must be called with lock held so eviction and run order are consistent.
func (ts *TelemetryStore) getOrCreateRunTelemetry(runID string) *runTelemetry {
	if rt, ok := ts.runs[runID]; ok {
		return rt
	}

	// Check if we need to evict
	ts.evictIfNeeded()

	rt := &runTelemetry{
		runID:       runID,
		startTimeMs: 0,
		endTimeMs:   0,
		operations:  make([]analysis.OperationResult, 0),
		logs:        make([]OperationLog, 0),
	}
	ts.runs[runID] = rt
	ts.runOrder = append(ts.runOrder, runID)
	return rt
}

// extractStageName is deprecated - stage should be explicitly provided in telemetry.
// This function is kept for backward compatibility but returns empty string
// to encourage explicit stage reporting.
func extractStageName(stageID string) string {
	return ""
}

func (ts *TelemetryStore) AddTelemetryBatchWithContext(runID string, batch TelemetryBatchRequest, workerID, stage, stageID string, vuID string) {
	for i := range batch.Operations {
		if batch.Operations[i].WorkerID == "" {
			batch.Operations[i].WorkerID = workerID
		}
		if batch.Operations[i].Stage == "" {
			batch.Operations[i].Stage = stage
		}
		if batch.Operations[i].StageID == "" {
			batch.Operations[i].StageID = stageID
		}
		if batch.Operations[i].VUID == "" && vuID != "" {
			batch.Operations[i].VUID = vuID
		}
	}
	ts.AddTelemetryBatch(runID, batch)
}

func (ts *TelemetryStore) SetRunMetadata(runID, scenarioID, stopReason string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	rt := ts.getOrCreateRunTelemetry(runID)

	if scenarioID != "" {
		rt.scenarioID = scenarioID
	}
	if stopReason != "" {
		rt.stopReason = stopReason
	}
}

func (ts *TelemetryStore) GetTelemetryData(runID string) (*runmanager.TelemetryData, error) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	rt, ok := ts.runs[runID]
	if !ok {
		return nil, fmt.Errorf("telemetry not found for run: %s", runID)
	}

	// Copy operations to avoid data races
	operations := make([]analysis.OperationResult, len(rt.operations))
	copy(operations, rt.operations)

	return &runmanager.TelemetryData{
		RunID:       rt.runID,
		ScenarioID:  rt.scenarioID,
		StartTimeMs: rt.startTimeMs,
		EndTimeMs:   rt.endTimeMs,
		StopReason:  rt.stopReason,
		Operations:  operations,
	}, nil
}

func (ts *TelemetryStore) GetOperationCount(runID string) int {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	rt, ok := ts.runs[runID]
	if !ok {
		return 0
	}
	return len(rt.operations)
}

func (ts *TelemetryStore) HasRun(runID string) bool {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	_, ok := ts.runs[runID]
	return ok
}

func (ts *TelemetryStore) DeleteRun(runID string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	delete(ts.runs, runID)
	// Remove from order tracking
	for i, id := range ts.runOrder {
		if id == runID {
			ts.runOrder = append(ts.runOrder[:i], ts.runOrder[i+1:]...)
			break
		}
	}
}

func (ts *TelemetryStore) RunCount() int {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	return len(ts.runs)
}

func (ts *TelemetryStore) QueryLogs(runID string, filters LogFilters) ([]OperationLog, int, error) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	rt, ok := ts.runs[runID]
	if !ok {
		return nil, 0, fmt.Errorf("run not found: %s", runID)
	}

	filtered := make([]OperationLog, 0, len(rt.logs))
	for _, log := range rt.logs {
		if !matchesFilters(log, filters) {
			continue
		}
		filtered = append(filtered, log)
	}

	total := len(filtered)

	if filters.Order == "asc" {
		sort.Slice(filtered, func(i, j int) bool {
			return filtered[i].TimestampMs < filtered[j].TimestampMs
		})
	} else {
		sort.Slice(filtered, func(i, j int) bool {
			return filtered[i].TimestampMs > filtered[j].TimestampMs
		})
	}

	if filters.Offset >= len(filtered) {
		return []OperationLog{}, total, nil
	}

	end := filters.Offset + filters.Limit
	if end > len(filtered) {
		end = len(filtered)
	}

	return filtered[filters.Offset:end], total, nil
}

func matchesFilters(log OperationLog, filters LogFilters) bool {
	if filters.Stage != "" && log.Stage != filters.Stage {
		return false
	}
	if filters.StageID != "" && log.StageID != filters.StageID {
		return false
	}
	if filters.WorkerID != "" && log.WorkerID != filters.WorkerID {
		return false
	}
	if filters.VUID != "" && log.VUID != filters.VUID {
		return false
	}
	if filters.SessionID != "" && log.SessionID != filters.SessionID {
		return false
	}
	if filters.Operation != "" && log.Operation != filters.Operation {
		return false
	}
	if filters.ToolName != "" && log.ToolName != filters.ToolName {
		return false
	}
	if filters.ErrorType != "" && log.ErrorType != filters.ErrorType {
		return false
	}
	if filters.ErrorCode != "" && log.ErrorCode != filters.ErrorCode {
		return false
	}
	return true
}

type RunRetentionInfo struct {
	RunID     string
	EndTimeMs int64
}

func (ts *TelemetryStore) ListRunsForRetention() []RunRetentionInfo {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	result := make([]RunRetentionInfo, 0, len(ts.runs))
	for runID, rt := range ts.runs {
		result = append(result, RunRetentionInfo{
			RunID:     runID,
			EndTimeMs: rt.endTimeMs,
		})
	}
	return result
}

func (ts *TelemetryStore) GetErrorLogs(runID string) ([]analysis.ErrorLog, error) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	rt, ok := ts.runs[runID]
	if !ok {
		return nil, fmt.Errorf("run not found: %s", runID)
	}

	errorLogs := make([]analysis.ErrorLog, 0)
	for _, log := range rt.logs {
		if !log.OK {
			errorLogs = append(errorLogs, analysis.ErrorLog{
				TimestampMs: log.TimestampMs,
				Operation:   log.Operation,
				ToolName:    log.ToolName,
				ErrorType:   log.ErrorType,
			})
		}
	}

	return errorLogs, nil
}

// GetStreamingMetrics computes streaming metrics from stored operation logs for a given run.
// It aggregates EventsReceived, StreamStartTimeMs, LastEventTimeMs, and StreamStallCount.
func (ts *TelemetryStore) GetStreamingMetrics(runID string) (*telemetry.StreamingMetrics, error) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	rt, ok := ts.runs[runID]
	if !ok {
		return nil, fmt.Errorf("run not found: %s", runID)
	}

	metrics := &telemetry.StreamingMetrics{}

	var minTime int64 = math.MaxInt64
	var maxTime int64 = 0

	for _, log := range rt.logs {
		if log.Stream == nil {
			continue
		}

		metrics.EventsReceived += int64(log.Stream.EventsCount)

		if log.Stream.IsStreaming && log.TimestampMs < minTime {
			minTime = log.TimestampMs
		}

		if log.Stream.IsStreaming && log.TimestampMs > maxTime {
			maxTime = log.TimestampMs
		}

		if log.Stream.Stalled {
			metrics.StreamStallCount++
		}
	}

	if minTime != math.MaxInt64 {
		metrics.StreamStartTimeMs = minTime
	}
	if maxTime != 0 {
		metrics.LastEventTimeMs = maxTime
	}

	return metrics, nil
}

// IsTruncated returns whether data was truncated for a run due to limits.
func (ts *TelemetryStore) IsTruncated(runID string) (operationsTruncated, logsTruncated bool) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	rt, ok := ts.runs[runID]
	if !ok {
		return false, false
	}
	return rt.operationsTruncated, rt.logsTruncated
}

func (ts *TelemetryStore) GetStabilityMetrics(runID string, includeEvents, includeTimeSeries bool) *metrics.StabilityMetrics {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	rt, ok := ts.runs[runID]
	if !ok {
		return nil
	}

	type sessionState struct {
		firstSeen    int64
		lastSeen     int64
		dropped      bool
		requestCount int64
		successCount int64
		errorCount   int64
		totalLatency float64
	}
	sessionStates := make(map[string]*sessionState)

	for _, log := range rt.logs {
		if log.SessionID == "" {
			continue
		}
		state, exists := sessionStates[log.SessionID]
		if !exists {
			state = &sessionState{firstSeen: log.TimestampMs, lastSeen: log.TimestampMs}
			sessionStates[log.SessionID] = state
		}
		if log.TimestampMs < state.firstSeen {
			state.firstSeen = log.TimestampMs
		}
		if log.TimestampMs > state.lastSeen {
			state.lastSeen = log.TimestampMs
		}
		state.requestCount++
		if log.OK {
			state.successCount++
		} else {
			state.errorCount++
		}
		state.totalLatency += float64(log.LatencyMs)
		if !log.OK && log.ErrorType == "connection_dropped" {
			state.dropped = true
		}
	}

	var totalSessions, activeSessions, droppedSessions, terminatedSessions int64
	var totalLifetimeMs float64

	// Check if run has ended
	runEnded := rt.endTimeMs > 0 || rt.stopReason != ""

	var sessionMetricsList []metrics.ConnectionMetrics
	for sessionID, state := range sessionStates {
		totalSessions++
		lifetimeMs := float64(state.lastSeen - state.firstSeen)
		totalLifetimeMs += lifetimeMs

		sessionState := "active"
		if state.dropped {
			droppedSessions++
			sessionState = "dropped"
		} else if runEnded {
			// If the run has ended, non-dropped sessions are terminated
			terminatedSessions++
			sessionState = "terminated"
		} else {
			activeSessions++
		}

		avgLatency := float64(0)
		if state.requestCount > 0 {
			avgLatency = state.totalLatency / float64(state.requestCount)
		}

		sessionMetricsList = append(sessionMetricsList, metrics.ConnectionMetrics{
			SessionID:    sessionID,
			CreatedAt:    time.UnixMilli(state.firstSeen),
			LastActiveAt: time.UnixMilli(state.lastSeen),
			RequestCount: state.requestCount,
			SuccessCount: state.successCount,
			ErrorCount:   state.errorCount,
			AvgLatencyMs: avgLatency,
			State:        sessionState,
		})
	}

	avgSessionLifetimeMs := float64(0)
	if totalSessions > 0 {
		avgSessionLifetimeMs = totalLifetimeMs / float64(totalSessions)
	}

	dropRate := float64(0)
	if totalSessions > 0 {
		dropRate = float64(droppedSessions) / float64(totalSessions)
	}

	stabilityScore := 100.0 - (dropRate * 100)
	if stabilityScore < 0 {
		stabilityScore = 0
	}

	result := &metrics.StabilityMetrics{
		TotalSessions:        totalSessions,
		ActiveSessions:       activeSessions,
		DroppedSessions:      droppedSessions,
		TerminatedSessions:   terminatedSessions,
		AvgSessionLifetimeMs: avgSessionLifetimeMs,
		ReconnectRate:        0,
		ProtocolErrorRate:    0,
		ConnectionChurnRate:  0,
		StabilityScore:       stabilityScore,
		DropRate:             dropRate,
		SessionMetrics:       sessionMetricsList,
	}

	if includeTimeSeries && len(rt.logs) > 0 {
		bucketSize := ts.calculateBucketSize(rt)
		timeBuckets := make(map[int64]*metrics.StabilityTimePoint)

		for _, log := range rt.logs {
			if log.SessionID == "" {
				continue
			}
			bucketKey := (log.TimestampMs / bucketSize) * bucketSize
			point, exists := timeBuckets[bucketKey]
			if !exists {
				point = &metrics.StabilityTimePoint{Timestamp: bucketKey}
				timeBuckets[bucketKey] = point
			}
		}

		sessionsSeenBefore := make(map[string]bool)
		sessionsActiveInBucket := make(map[int64]map[string]bool)

		for _, log := range rt.logs {
			if log.SessionID == "" {
				continue
			}
			bucketKey := (log.TimestampMs / bucketSize) * bucketSize
			point := timeBuckets[bucketKey]

			if sessionsActiveInBucket[bucketKey] == nil {
				sessionsActiveInBucket[bucketKey] = make(map[string]bool)
			}
			sessionsActiveInBucket[bucketKey][log.SessionID] = true

			if !sessionsSeenBefore[log.SessionID] {
				sessionsSeenBefore[log.SessionID] = true
				point.NewSessions++
			}

			if !log.OK && log.ErrorType == "connection_dropped" {
				point.DroppedSessions++
			}
		}

		for bucketKey, point := range timeBuckets {
			if sessions := sessionsActiveInBucket[bucketKey]; sessions != nil {
				point.ActiveSessions = int32(len(sessions))
			}
		}

		bucketKeys := make([]int64, 0, len(timeBuckets))
		for key := range timeBuckets {
			bucketKeys = append(bucketKeys, key)
		}
		sort.Slice(bucketKeys, func(i, j int) bool {
			return bucketKeys[i] < bucketKeys[j]
		})

		timeSeriesData := make([]metrics.StabilityTimePoint, 0, len(bucketKeys))
		for _, key := range bucketKeys {
			timeSeriesData = append(timeSeriesData, *timeBuckets[key])
		}

		result.TimeSeriesData = timeSeriesData
	}

	return result
}

func (ts *TelemetryStore) GetMetricsTimeSeries(runID string) []metrics.MetricsTimePoint {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	rt, ok := ts.runs[runID]
	if !ok || len(rt.logs) == 0 {
		return nil
	}

	// Calculate dynamic bucket size based on run duration
	// Target: 20-30 data points for useful charts
	bucketSize := ts.calculateBucketSize(rt)
	buckets := make(map[int64]*metricsTimeBucket)

	for _, log := range rt.logs {
		bucketKey := (log.TimestampMs / bucketSize) * bucketSize
		bucket, exists := buckets[bucketKey]
		if !exists {
			bucket = &metricsTimeBucket{
				timestamp:  bucketKey,
				latencies:  make([]int, 0, 128),
				successOps: 0,
				failedOps:  0,
			}
			buckets[bucketKey] = bucket
		}

		if log.OK {
			bucket.successOps++
		} else {
			bucket.failedOps++
		}
		bucket.latencies = append(bucket.latencies, log.LatencyMs)
	}

	bucketKeys := make([]int64, 0, len(buckets))
	for key := range buckets {
		bucketKeys = append(bucketKeys, key)
	}
	sort.Slice(bucketKeys, func(i, j int) bool {
		return bucketKeys[i] < bucketKeys[j]
	})

	result := make([]metrics.MetricsTimePoint, 0, len(bucketKeys))
	for _, key := range bucketKeys {
		bucket := buckets[key]
		totalOps := bucket.successOps + bucket.failedOps

		var errorRate float64
		if totalOps > 0 {
			errorRate = float64(bucket.failedOps) / float64(totalOps)
		}

		throughput := float64(totalOps) / (float64(bucketSize) / 1000.0)

		p50, p95, p99, mean := calculateLatencyPercentiles(bucket.latencies)

		result = append(result, metrics.MetricsTimePoint{
			Timestamp:   key,
			SuccessOps:  bucket.successOps,
			FailedOps:   bucket.failedOps,
			Throughput:  throughput,
			LatencyP50:  p50,
			LatencyP95:  p95,
			LatencyP99:  p99,
			LatencyMean: mean,
			ErrorRate:   errorRate,
		})
	}

	return result
}

type metricsTimeBucket struct {
	timestamp  int64
	latencies  []int
	successOps int64
	failedOps  int64
}

func (ts *TelemetryStore) calculateBucketSize(rt *runTelemetry) int64 {
	if len(rt.logs) < 2 {
		return 5000
	}

	minTs, maxTs := rt.logs[0].TimestampMs, rt.logs[0].TimestampMs
	for _, log := range rt.logs {
		if log.TimestampMs < minTs {
			minTs = log.TimestampMs
		}
		if log.TimestampMs > maxTs {
			maxTs = log.TimestampMs
		}
	}

	durationMs := maxTs - minTs
	if durationMs <= 0 {
		return 5000
	}

	const targetBuckets = 25
	bucketSize := durationMs / targetBuckets

	const minBucketSize, maxBucketSize = 100, 5000
	if bucketSize < minBucketSize {
		bucketSize = minBucketSize
	}
	if bucketSize > maxBucketSize {
		bucketSize = maxBucketSize
	}

	return bucketSize
}

func calculateLatencyPercentiles(latencies []int) (p50, p95, p99, mean float64) {
	if len(latencies) == 0 {
		return 0, 0, 0, 0
	}

	sorted := make([]int, len(latencies))
	copy(sorted, latencies)
	sort.Ints(sorted)

	var sum int64
	for _, v := range sorted {
		sum += int64(v)
	}
	mean = float64(sum) / float64(len(sorted))

	p50 = float64(sorted[len(sorted)*50/100])
	p95 = float64(sorted[len(sorted)*95/100])
	p99Index := len(sorted) * 99 / 100
	if p99Index >= len(sorted) {
		p99Index = len(sorted) - 1
	}
	p99 = float64(sorted[p99Index])

	return p50, p95, p99, mean
}
