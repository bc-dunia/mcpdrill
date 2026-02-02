// Package analysis provides telemetry aggregation and metrics computation.
package analysis

import (
	"sort"
	"sync"
)

// OperationResult represents a single operation's telemetry data.
type OperationResult struct {
	Operation string // initialize, tools/list, tools/call, ping (MCP-style with slashes)
	ToolName  string // tool name for tools/call operations
	LatencyMs int    // operation latency in milliseconds
	OK        bool   // whether operation succeeded
	ErrorType string // error classification if failed
	SessionID string // session identifier for session metrics tracking
}

// normalizeOpName converts operation names to canonical form.
// Accepts both MCP-style (tools/list) and underscore-style (tools_list).
func normalizeOpName(op string) string {
	switch op {
	case "tools_list":
		return "tools/list"
	case "tools_call":
		return "tools/call"
	case "resources_list":
		return "resources/list"
	case "resources_read":
		return "resources/read"
	case "prompts_list":
		return "prompts/list"
	case "prompts_get":
		return "prompts/get"
	default:
		return op
	}
}

// OperationMetrics holds metrics for a specific operation or tool.
type OperationMetrics struct {
	TotalOps   int     `json:"total_ops"`
	SuccessOps int     `json:"success_ops"`
	FailureOps int     `json:"failure_ops"`
	LatencyP50 int     `json:"latency_p50"`
	LatencyP95 int     `json:"latency_p95"`
	LatencyP99 int     `json:"latency_p99"`
	ErrorRate  float64 `json:"error_rate"`
}

// AggregatedMetrics contains all computed metrics from telemetry data.
type AggregatedMetrics struct {
	TotalOps       int                          `json:"total_ops"`
	SuccessOps     int                          `json:"success_ops"`
	FailureOps     int                          `json:"failure_ops"`
	RPS            float64                      `json:"rps"`
	LatencyP50     int                          `json:"latency_p50"`
	LatencyP95     int                          `json:"latency_p95"`
	LatencyP99     int                          `json:"latency_p99"`
	ErrorRate      float64                      `json:"error_rate"`
	ByOperation    map[string]*OperationMetrics `json:"by_operation"`
	ByTool         map[string]*OperationMetrics `json:"by_tool"`
	SessionMetrics *SessionReportMetrics        `json:"session_metrics,omitempty"`
	WorkerHealth   *WorkerHealthMetrics         `json:"worker_health,omitempty"`
	ChurnMetrics   *ChurnReportMetrics          `json:"churn_metrics,omitempty"`
}

// SessionReportMetrics contains session-specific metrics for A/B comparison.
type SessionReportMetrics struct {
	SessionMode      string  `json:"session_mode"`
	TotalSessions    int     `json:"total_sessions"`
	OpsPerSession    float64 `json:"ops_per_session"`
	SessionReuseRate float64 `json:"session_reuse_rate"`
	AvgSessionLifeMs float64 `json:"avg_session_lifetime_ms,omitempty"`
	TotalCreated     int64   `json:"total_created,omitempty"`
	TotalEvicted     int64   `json:"total_evicted,omitempty"`
	Reconnects       int64   `json:"reconnects,omitempty"`
}

// WorkerHealthMetrics contains aggregated worker health metrics for detecting load generator bottlenecks.
type WorkerHealthMetrics struct {
	PeakCPUPercent     float64 `json:"peak_cpu_percent"`            // Max CPU usage across all workers/time
	PeakMemoryMB       float64 `json:"peak_memory_mb"`              // Max memory usage across all workers/time
	AvgActiveVUs       float64 `json:"avg_active_vus"`              // Average active VUs across run
	WorkerCount        int     `json:"worker_count"`                // Total workers used
	SaturationDetected bool    `json:"saturation_detected"`         // CPU > 80% or VUs at cap
	SaturationReason   string  `json:"saturation_reason,omitempty"` // Reason for saturation if detected
}

// WorkerHealthSample represents a single health sample from a worker.
type WorkerHealthSample struct {
	WorkerID       string
	CPUPercent     float64
	MemBytes       int64
	ActiveVUs      int
	ActiveSessions int
}

// ChurnReportMetrics contains churn-specific metrics for reports.
type ChurnReportMetrics struct {
	SessionsCreated   int64   `json:"sessions_created"`
	SessionsDestroyed int64   `json:"sessions_destroyed"`
	ActiveSessions    int     `json:"active_sessions"`
	ReconnectAttempts int64   `json:"reconnect_attempts"`
	ChurnRate         float64 `json:"churn_rate"`
}

// ChurnSample represents a point-in-time churn metrics sample.
type ChurnSample struct {
	SessionsCreated   int64
	SessionsDestroyed int64
	ActiveSessions    int
	ReconnectAttempts int64
}

// Aggregator collects operation results and computes metrics.
type Aggregator struct {
	mu             sync.RWMutex
	operations     []OperationResult
	startTime      int64
	endTime        int64
	sessionMode    string
	sessionMetrics *SessionManagerMetrics
	healthSamples  []WorkerHealthSample
	workersSeen    map[string]struct{}
	maxVUsConfig   int
	churnSamples   []ChurnSample
}

// SessionManagerMetrics holds metrics from the session manager for reporting.
type SessionManagerMetrics struct {
	TotalCreated int64
	TotalEvicted int64
	Reconnects   int64
}

// NewAggregator creates a new Aggregator instance.
func NewAggregator() *Aggregator {
	return &Aggregator{
		operations:    make([]OperationResult, 0),
		healthSamples: make([]WorkerHealthSample, 0),
		workersSeen:   make(map[string]struct{}),
		churnSamples:  make([]ChurnSample, 0),
	}
}

// SetTimeRange sets the time range for RPS calculation.
func (a *Aggregator) SetTimeRange(startMs, endMs int64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.startTime = startMs
	a.endTime = endMs
}

// SetSessionInfo sets session mode and metrics for reporting.
func (a *Aggregator) SetSessionInfo(mode string, metrics *SessionManagerMetrics) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sessionMode = mode
	a.sessionMetrics = metrics
}

// SetMaxVUsConfig sets the configured max VUs for saturation detection.
func (a *Aggregator) SetMaxVUsConfig(maxVUs int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.maxVUsConfig = maxVUs
}

// AddWorkerHealth adds a worker health sample for aggregation.
func (a *Aggregator) AddWorkerHealth(sample WorkerHealthSample) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.healthSamples = append(a.healthSamples, sample)
	if sample.WorkerID != "" {
		a.workersSeen[sample.WorkerID] = struct{}{}
	}
}

// AddChurnSample adds a churn metrics sample for aggregation.
func (a *Aggregator) AddChurnSample(sample ChurnSample) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.churnSamples = append(a.churnSamples, sample)
}

// AddOperation adds an operation result to the aggregator.
// Thread-safe for concurrent ingestion.
func (a *Aggregator) AddOperation(op OperationResult) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.operations = append(a.operations, op)
}

// Compute calculates all aggregated metrics from collected operations.
func (a *Aggregator) Compute() *AggregatedMetrics {
	a.mu.RLock()
	defer a.mu.RUnlock()

	metrics := &AggregatedMetrics{
		ByOperation: make(map[string]*OperationMetrics, len(a.operations)),
		ByTool:      make(map[string]*OperationMetrics, len(a.operations)),
	}

	metrics.WorkerHealth = a.computeWorkerHealthMetrics()
	metrics.ChurnMetrics = a.computeChurnMetrics()

	if len(a.operations) == 0 {
		return metrics
	}

	// Collect all latencies for global percentiles
	allLatencies := make([]int, 0, len(a.operations))

	// Group operations by type and tool
	opLatencies := make(map[string][]int, len(a.operations))
	opSuccess := make(map[string]int, len(a.operations))
	opFailure := make(map[string]int, len(a.operations))

	toolLatencies := make(map[string][]int, len(a.operations))
	toolSuccess := make(map[string]int, len(a.operations))
	toolFailure := make(map[string]int, len(a.operations))

	for _, op := range a.operations {
		metrics.TotalOps++
		allLatencies = append(allLatencies, op.LatencyMs)

		if op.OK {
			metrics.SuccessOps++
		} else {
			metrics.FailureOps++
		}

		normalizedOp := normalizeOpName(op.Operation)

		opLatencies[normalizedOp] = append(opLatencies[normalizedOp], op.LatencyMs)
		if op.OK {
			opSuccess[normalizedOp]++
		} else {
			opFailure[normalizedOp]++
		}

		if (normalizedOp == "tools/call" || normalizedOp == "tools_call") && op.ToolName != "" {
			toolLatencies[op.ToolName] = append(toolLatencies[op.ToolName], op.LatencyMs)
			if op.OK {
				toolSuccess[op.ToolName]++
			} else {
				toolFailure[op.ToolName]++
			}
		}
	}

	// Compute global metrics
	metrics.LatencyP50 = computePercentile(allLatencies, 50)
	metrics.LatencyP95 = computePercentile(allLatencies, 95)
	metrics.LatencyP99 = computePercentile(allLatencies, 99)
	metrics.ErrorRate = float64(metrics.FailureOps) / float64(metrics.TotalOps)

	// Compute RPS
	if a.endTime > a.startTime {
		durationSec := float64(a.endTime-a.startTime) / 1000.0
		metrics.RPS = float64(metrics.TotalOps) / durationSec
	}

	// Compute per-operation metrics
	for opName, latencies := range opLatencies {
		total := opSuccess[opName] + opFailure[opName]
		metrics.ByOperation[opName] = &OperationMetrics{
			TotalOps:   total,
			SuccessOps: opSuccess[opName],
			FailureOps: opFailure[opName],
			LatencyP50: computePercentile(latencies, 50),
			LatencyP95: computePercentile(latencies, 95),
			LatencyP99: computePercentile(latencies, 99),
			ErrorRate:  float64(opFailure[opName]) / float64(total),
		}
	}

	// Compute per-tool metrics
	for toolName, latencies := range toolLatencies {
		total := toolSuccess[toolName] + toolFailure[toolName]
		metrics.ByTool[toolName] = &OperationMetrics{
			TotalOps:   total,
			SuccessOps: toolSuccess[toolName],
			FailureOps: toolFailure[toolName],
			LatencyP50: computePercentile(latencies, 50),
			LatencyP95: computePercentile(latencies, 95),
			LatencyP99: computePercentile(latencies, 99),
			ErrorRate:  float64(toolFailure[toolName]) / float64(total),
		}
	}

	metrics.SessionMetrics = a.computeSessionMetrics()

	return metrics
}

func (a *Aggregator) computeSessionMetrics() *SessionReportMetrics {
	if a.sessionMode == "" {
		uniqueSessions := make(map[string]struct{})
		for _, op := range a.operations {
			if op.SessionID != "" {
				uniqueSessions[op.SessionID] = struct{}{}
			}
		}
		if len(uniqueSessions) == 0 {
			return nil
		}
		totalSessions := len(uniqueSessions)
		totalOps := len(a.operations)
		opsPerSession := float64(totalOps) / float64(totalSessions)
		return &SessionReportMetrics{
			SessionMode:      "unknown",
			TotalSessions:    totalSessions,
			OpsPerSession:    opsPerSession,
			SessionReuseRate: opsPerSession,
		}
	}

	uniqueSessions := make(map[string]struct{})
	for _, op := range a.operations {
		if op.SessionID != "" {
			uniqueSessions[op.SessionID] = struct{}{}
		}
	}

	totalSessions := len(uniqueSessions)
	totalOps := len(a.operations)

	var opsPerSession float64
	if totalSessions > 0 {
		opsPerSession = float64(totalOps) / float64(totalSessions)
	}

	sessionMetrics := &SessionReportMetrics{
		SessionMode:      a.sessionMode,
		TotalSessions:    totalSessions,
		OpsPerSession:    opsPerSession,
		SessionReuseRate: opsPerSession,
	}

	if a.sessionMetrics != nil {
		sessionMetrics.TotalCreated = a.sessionMetrics.TotalCreated
		sessionMetrics.TotalEvicted = a.sessionMetrics.TotalEvicted
		sessionMetrics.Reconnects = a.sessionMetrics.Reconnects
	}

	return sessionMetrics
}

func (a *Aggregator) computeChurnMetrics() *ChurnReportMetrics {
	if len(a.churnSamples) == 0 {
		return nil
	}

	var totalCreated, totalDestroyed, totalReconnects int64
	var lastActiveSessions int

	for _, sample := range a.churnSamples {
		totalCreated += sample.SessionsCreated
		totalDestroyed += sample.SessionsDestroyed
		totalReconnects += sample.ReconnectAttempts
		lastActiveSessions = sample.ActiveSessions
	}

	var churnRate float64
	if a.endTime > a.startTime {
		durationSec := float64(a.endTime-a.startTime) / 1000.0
		churnRate = float64(totalCreated+totalDestroyed) / durationSec
	}

	return &ChurnReportMetrics{
		SessionsCreated:   totalCreated,
		SessionsDestroyed: totalDestroyed,
		ActiveSessions:    lastActiveSessions,
		ReconnectAttempts: totalReconnects,
		ChurnRate:         churnRate,
	}
}

func (a *Aggregator) computeWorkerHealthMetrics() *WorkerHealthMetrics {
	if len(a.healthSamples) == 0 {
		return nil
	}

	var peakCPU float64
	var peakMemBytes int64
	var totalVUs int
	sampleCount := len(a.healthSamples)

	for _, sample := range a.healthSamples {
		if sample.CPUPercent > peakCPU {
			peakCPU = sample.CPUPercent
		}
		if sample.MemBytes > peakMemBytes {
			peakMemBytes = sample.MemBytes
		}
		totalVUs += sample.ActiveVUs
	}

	avgVUs := float64(totalVUs) / float64(sampleCount)
	peakMemMB := float64(peakMemBytes) / (1024 * 1024)

	metrics := &WorkerHealthMetrics{
		PeakCPUPercent: peakCPU,
		PeakMemoryMB:   peakMemMB,
		AvgActiveVUs:   avgVUs,
		WorkerCount:    len(a.workersSeen),
	}

	const cpuSaturationThreshold = 80.0
	if peakCPU >= cpuSaturationThreshold {
		metrics.SaturationDetected = true
		metrics.SaturationReason = "CPU usage exceeded 80%"
	}

	if a.maxVUsConfig > 0 {
		for _, sample := range a.healthSamples {
			if sample.ActiveVUs >= a.maxVUsConfig {
				metrics.SaturationDetected = true
				if metrics.SaturationReason != "" {
					metrics.SaturationReason += "; VU cap reached"
				} else {
					metrics.SaturationReason = "VU cap reached"
				}
				break
			}
		}
	}

	return metrics
}

func computePercentile(latencies []int, p float64) int {
	if len(latencies) == 0 {
		return 0
	}

	sorted := make([]int, len(latencies))
	copy(sorted, latencies)
	sort.Ints(sorted)

	rank := (p / 100.0) * float64(len(sorted))
	index := int(rank)
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	if index < 0 {
		index = 0
	}

	return sorted[index]
}

// OperationCount returns the current number of operations.
// Thread-safe.
func (a *Aggregator) OperationCount() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.operations)
}

// Reset clears all collected operations.
// Thread-safe.
func (a *Aggregator) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.operations = make([]OperationResult, 0)
	a.healthSamples = make([]WorkerHealthSample, 0)
	a.workersSeen = make(map[string]struct{})
	a.churnSamples = make([]ChurnSample, 0)
	a.startTime = 0
	a.endTime = 0
	a.maxVUsConfig = 0
}
