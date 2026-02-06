// Package metrics provides Prometheus metrics exposition for MCP Drill.
package metrics

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/analysis"
	"github.com/bc-dunia/mcpdrill/internal/controlplane/runmanager"
	"github.com/bc-dunia/mcpdrill/internal/controlplane/scheduler"
)

// RunProvider provides access to run data for metrics collection.
type RunProvider interface {
	ListRuns() []*runmanager.RunView
}

// WorkerProvider provides access to worker data for metrics collection.
type WorkerProvider interface {
	ListWorkers() []*scheduler.WorkerInfo
}

// TelemetryProvider provides access to telemetry data for metrics collection.
type TelemetryProvider interface {
	GetTelemetryData(runID string) (*runmanager.TelemetryData, error)
}

// Collector collects and exposes MCP Drill metrics in Prometheus format.
// Thread-safe for concurrent access.
//
// Lock Strategy: Collector uses a single RWMutex for thread-safety. While this creates some lock
// contention under high load, it's necessary because Go maps are not atomic-safe. Alternative
// approaches (sync.Map, sharded maps) add complexity without clear benefit for our access patterns.
// The RWMutex allows concurrent reads via Expose() while serializing writes from hot-path methods
// like RecordOperation(). This is a reasonable trade-off between simplicity and performance.
type Collector struct {
	mu sync.RWMutex

	// Providers for data access
	runProvider       RunProvider
	workerProvider    WorkerProvider
	telemetryProvider TelemetryProvider

	// Cached metrics data
	runCounts          map[string]int64             // scenario_id -> count
	runDurations       map[string]*histogramData    // scenario_id -> histogram
	runStates          map[runStateKey]int          // (scenario_id, state) -> gauge
	workerHealth       map[string]*workerHealthData // worker_id -> health
	operationCounts    map[opKey]int64              // (operation, tool_name) -> count
	operationDurations map[opKey]*histogramData     // (operation, tool_name) -> histogram
	operationErrors    map[opKey]int64              // (operation, tool_name) -> count
	stageDurations     map[stageKey]float64         // (run_id, stage_id) -> duration_seconds
	stageVUs           map[stageKey]int             // (run_id, stage_id) -> vus

	// Time function for testing
	nowFunc func() time.Time
}

// runStateKey is a composite key for run state metrics.
type runStateKey struct {
	scenarioID string
	state      string
}

// opKey is a composite key for operation metrics.
type opKey struct {
	operation string
	toolName  string
}

// stageKey is a composite key for stage metrics.
type stageKey struct {
	runID   string
	stageID string
}

// histogramData holds histogram data for Prometheus exposition.
type histogramData struct {
	sum   float64
	count int64
}

// workerHealthData holds worker health metrics.
type workerHealthData struct {
	cpuPercent float64
	memoryMB   float64
	activeVUs  int
}

// NewCollector creates a new metrics Collector.
func NewCollector() *Collector {
	return &Collector{
		runCounts:          make(map[string]int64),
		runDurations:       make(map[string]*histogramData),
		runStates:          make(map[runStateKey]int),
		workerHealth:       make(map[string]*workerHealthData),
		operationCounts:    make(map[opKey]int64),
		operationDurations: make(map[opKey]*histogramData),
		operationErrors:    make(map[opKey]int64),
		stageDurations:     make(map[stageKey]float64),
		stageVUs:           make(map[stageKey]int),
		nowFunc:            time.Now,
	}
}

// SetRunProvider sets the run provider for metrics collection.
func (c *Collector) SetRunProvider(p RunProvider) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.runProvider = p
}

// SetWorkerProvider sets the worker provider for metrics collection.
func (c *Collector) SetWorkerProvider(p WorkerProvider) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.workerProvider = p
}

// SetTelemetryProvider sets the telemetry provider for metrics collection.
func (c *Collector) SetTelemetryProvider(p TelemetryProvider) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.telemetryProvider = p
}

// RecordRunCreated records a new run creation.
func (c *Collector) RecordRunCreated(scenarioID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.runCounts[scenarioID]++
}

// RecordRunDuration records a run's duration.
func (c *Collector) RecordRunDuration(scenarioID string, durationSeconds float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.runDurations[scenarioID] == nil {
		c.runDurations[scenarioID] = &histogramData{}
	}
	c.runDurations[scenarioID].sum += durationSeconds
	c.runDurations[scenarioID].count++
}

// RecordOperation records an operation execution.
func (c *Collector) RecordOperation(operation, toolName string, durationMs int, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := opKey{operation: operation, toolName: toolName}
	c.operationCounts[key]++

	if c.operationDurations[key] == nil {
		c.operationDurations[key] = &histogramData{}
	}
	c.operationDurations[key].sum += float64(durationMs) / 1000.0
	c.operationDurations[key].count++

	if !ok {
		c.operationErrors[key]++
	}
}

// RecordStageMetrics records stage-level metrics.
func (c *Collector) RecordStageMetrics(runID, stageID string, durationSeconds float64, vus int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := stageKey{runID: runID, stageID: stageID}
	c.stageDurations[key] = durationSeconds
	c.stageVUs[key] = vus
}

// SyncFromProviders synchronizes metrics from configured providers.
// This should be called periodically or on-demand before metrics exposition.
func (c *Collector) SyncFromProviders() {
	c.mu.Lock()
	runProvider := c.runProvider
	workerProvider := c.workerProvider
	telemetryProvider := c.telemetryProvider
	c.mu.Unlock()

	// Sync run states
	if runProvider != nil {
		runs := runProvider.ListRuns()
		c.syncRunStates(runs)
	}

	// Sync worker health
	if workerProvider != nil {
		workers := workerProvider.ListWorkers()
		c.syncWorkerHealth(workers)
	}

	// Sync telemetry data
	if runProvider != nil && telemetryProvider != nil {
		runs := runProvider.ListRuns()
		c.syncTelemetryData(runs, telemetryProvider)
	}
}

func (c *Collector) syncRunStates(runs []*runmanager.RunView) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.runStates = make(map[runStateKey]int)

	for _, run := range runs {
		key := runStateKey{scenarioID: run.ScenarioID, state: string(run.State)}
		c.runStates[key]++
	}
}

func (c *Collector) syncWorkerHealth(workers []*scheduler.WorkerInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.workerHealth = make(map[string]*workerHealthData)

	for _, worker := range workers {
		data := &workerHealthData{}
		if worker.Health != nil {
			data.cpuPercent = worker.Health.CPUPercent
			data.memoryMB = float64(worker.Health.MemBytes) / (1024 * 1024)
			data.activeVUs = worker.Health.ActiveVUs
		}
		c.workerHealth[string(worker.WorkerID)] = data
	}
}

func (c *Collector) syncTelemetryData(runs []*runmanager.RunView, telemetryProvider TelemetryProvider) {
	type telemetryResult struct {
		runID      string
		scenarioID string
		data       *runmanager.TelemetryData
	}
	var results []telemetryResult

	for _, run := range runs {
		data, err := telemetryProvider.GetTelemetryData(run.RunID)
		if err != nil {
			continue
		}
		results = append(results, telemetryResult{
			runID:      run.RunID,
			scenarioID: run.ScenarioID,
			data:       data,
		})
	}

	runDurations := make(map[string]*histogramData)
	for _, result := range results {
		durationMs := result.data.EndTimeMs - result.data.StartTimeMs
		if durationMs <= 0 {
			continue
		}
		durationSeconds := float64(durationMs) / 1000.0
		data := runDurations[result.scenarioID]
		if data == nil {
			data = &histogramData{}
			runDurations[result.scenarioID] = data
		}
		data.sum += durationSeconds
		data.count++
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.runDurations = runDurations
}

// Expose returns the metrics in Prometheus text exposition format.
func (c *Collector) Expose() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var sb strings.Builder
	timestamp := c.nowFunc().UnixMilli()

	// mcpdrill_runs_total
	c.writeRunsTotal(&sb, timestamp)

	// mcpdrill_run_duration_seconds
	c.writeRunDuration(&sb, timestamp)

	// mcpdrill_run_state
	c.writeRunState(&sb, timestamp)

	// mcpdrill_workers_total
	c.writeWorkersTotal(&sb, timestamp)

	// mcpdrill_worker_health_*
	c.writeWorkerHealth(&sb, timestamp)

	// mcpdrill_operations_total
	c.writeOperationsTotal(&sb, timestamp)

	// mcpdrill_operation_duration_seconds
	c.writeOperationDuration(&sb, timestamp)

	// mcpdrill_operation_errors_total
	c.writeOperationErrors(&sb, timestamp)

	// mcpdrill_stage_duration_seconds
	c.writeStageDuration(&sb, timestamp)

	// mcpdrill_stage_vus
	c.writeStageVUs(&sb, timestamp)

	return sb.String()
}

func (c *Collector) writeRunsTotal(sb *strings.Builder, timestamp int64) {
	sb.WriteString("# HELP mcpdrill_runs_total Total number of runs created\n")
	sb.WriteString("# TYPE mcpdrill_runs_total counter\n")

	// Sort keys for deterministic output
	keys := make([]string, 0, len(c.runCounts))
	for k := range c.runCounts {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, scenarioID := range keys {
		count := c.runCounts[scenarioID]
		fmt.Fprintf(sb, "mcpdrill_runs_total{scenario_id=%q} %d %d\n", scenarioID, count, timestamp)
	}
}

func (c *Collector) writeRunDuration(sb *strings.Builder, timestamp int64) {
	sb.WriteString("# HELP mcpdrill_run_duration_seconds Duration of runs in seconds\n")
	sb.WriteString("# TYPE mcpdrill_run_duration_seconds histogram\n")

	// Sort keys for deterministic output
	keys := make([]string, 0, len(c.runDurations))
	for k := range c.runDurations {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, scenarioID := range keys {
		data := c.runDurations[scenarioID]
		fmt.Fprintf(sb, "mcpdrill_run_duration_seconds_sum{scenario_id=%q} %.6f %d\n", scenarioID, data.sum, timestamp)
		fmt.Fprintf(sb, "mcpdrill_run_duration_seconds_count{scenario_id=%q} %d %d\n", scenarioID, data.count, timestamp)
	}
}

func (c *Collector) writeRunState(sb *strings.Builder, timestamp int64) {
	sb.WriteString("# HELP mcpdrill_run_state Current state of runs (1 = in this state)\n")
	sb.WriteString("# TYPE mcpdrill_run_state gauge\n")

	// Sort keys for deterministic output
	type sortKey struct {
		scenarioID string
		state      string
	}
	keys := make([]sortKey, 0, len(c.runStates))
	for k := range c.runStates {
		keys = append(keys, sortKey{scenarioID: k.scenarioID, state: k.state})
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].scenarioID != keys[j].scenarioID {
			return keys[i].scenarioID < keys[j].scenarioID
		}
		return keys[i].state < keys[j].state
	})

	for _, k := range keys {
		key := runStateKey{scenarioID: k.scenarioID, state: k.state}
		count := c.runStates[key]
		fmt.Fprintf(sb, "mcpdrill_run_state{scenario_id=%q,state=%q} %d %d\n", k.scenarioID, k.state, count, timestamp)
	}
}

func (c *Collector) writeWorkersTotal(sb *strings.Builder, timestamp int64) {
	sb.WriteString("# HELP mcpdrill_workers_total Total number of registered workers\n")
	sb.WriteString("# TYPE mcpdrill_workers_total gauge\n")
	fmt.Fprintf(sb, "mcpdrill_workers_total %d %d\n", len(c.workerHealth), timestamp)
}

func (c *Collector) writeWorkerHealth(sb *strings.Builder, timestamp int64) {
	// CPU
	sb.WriteString("# HELP mcpdrill_worker_health_cpu_percent Worker CPU usage percentage\n")
	sb.WriteString("# TYPE mcpdrill_worker_health_cpu_percent gauge\n")

	// Sort worker IDs for deterministic output
	workerIDs := make([]string, 0, len(c.workerHealth))
	for id := range c.workerHealth {
		workerIDs = append(workerIDs, id)
	}
	sort.Strings(workerIDs)

	for _, workerID := range workerIDs {
		data := c.workerHealth[workerID]
		fmt.Fprintf(sb, "mcpdrill_worker_health_cpu_percent{worker_id=%q} %.2f %d\n", workerID, data.cpuPercent, timestamp)
	}

	// Memory
	sb.WriteString("# HELP mcpdrill_worker_health_memory_mb Worker memory usage in MB\n")
	sb.WriteString("# TYPE mcpdrill_worker_health_memory_mb gauge\n")
	for _, workerID := range workerIDs {
		data := c.workerHealth[workerID]
		fmt.Fprintf(sb, "mcpdrill_worker_health_memory_mb{worker_id=%q} %.2f %d\n", workerID, data.memoryMB, timestamp)
	}

	// Active VUs
	sb.WriteString("# HELP mcpdrill_worker_health_active_vus Worker active virtual users\n")
	sb.WriteString("# TYPE mcpdrill_worker_health_active_vus gauge\n")
	for _, workerID := range workerIDs {
		data := c.workerHealth[workerID]
		fmt.Fprintf(sb, "mcpdrill_worker_health_active_vus{worker_id=%q} %d %d\n", workerID, data.activeVUs, timestamp)
	}
}

func (c *Collector) writeOperationsTotal(sb *strings.Builder, timestamp int64) {
	sb.WriteString("# HELP mcpdrill_operations_total Total number of operations executed\n")
	sb.WriteString("# TYPE mcpdrill_operations_total counter\n")

	keys := make([]opKey, 0, len(c.operationCounts))
	for k := range c.operationCounts {
		keys = append(keys, k)
	}
	sortOpKeys(keys)
	for _, k := range keys {
		count := c.operationCounts[k]
		fmt.Fprintf(sb, "mcpdrill_operations_total{operation=%q,tool_name=%q} %d %d\n", k.operation, k.toolName, count, timestamp)
	}
}

func (c *Collector) writeOperationDuration(sb *strings.Builder, timestamp int64) {
	sb.WriteString("# HELP mcpdrill_operation_duration_seconds Duration of operations in seconds\n")
	sb.WriteString("# TYPE mcpdrill_operation_duration_seconds histogram\n")

	keys := make([]opKey, 0, len(c.operationDurations))
	for k := range c.operationDurations {
		keys = append(keys, k)
	}
	sortOpKeys(keys)
	for _, k := range keys {
		data := c.operationDurations[k]
		fmt.Fprintf(sb, "mcpdrill_operation_duration_seconds_sum{operation=%q,tool_name=%q} %.6f %d\n", k.operation, k.toolName, data.sum, timestamp)
		fmt.Fprintf(sb, "mcpdrill_operation_duration_seconds_count{operation=%q,tool_name=%q} %d %d\n", k.operation, k.toolName, data.count, timestamp)
	}
}

func (c *Collector) writeOperationErrors(sb *strings.Builder, timestamp int64) {
	sb.WriteString("# HELP mcpdrill_operation_errors_total Total number of operation errors\n")
	sb.WriteString("# TYPE mcpdrill_operation_errors_total counter\n")

	keys := make([]opKey, 0, len(c.operationErrors))
	for k := range c.operationErrors {
		keys = append(keys, k)
	}
	sortOpKeys(keys)
	for _, k := range keys {
		count := c.operationErrors[k]
		fmt.Fprintf(sb, "mcpdrill_operation_errors_total{operation=%q,tool_name=%q} %d %d\n", k.operation, k.toolName, count, timestamp)
	}
}

func (c *Collector) writeStageDuration(sb *strings.Builder, timestamp int64) {
	sb.WriteString("# HELP mcpdrill_stage_duration_seconds Duration of stages in seconds\n")
	sb.WriteString("# TYPE mcpdrill_stage_duration_seconds gauge\n")

	keys := make([]stageKey, 0, len(c.stageDurations))
	for k := range c.stageDurations {
		keys = append(keys, k)
	}
	sortStageKeys(keys)
	for _, k := range keys {
		duration := c.stageDurations[k]
		fmt.Fprintf(sb, "mcpdrill_stage_duration_seconds{run_id=%q,stage_id=%q} %.6f %d\n", k.runID, k.stageID, duration, timestamp)
	}
}

func (c *Collector) writeStageVUs(sb *strings.Builder, timestamp int64) {
	sb.WriteString("# HELP mcpdrill_stage_vus Number of virtual users in a stage\n")
	sb.WriteString("# TYPE mcpdrill_stage_vus gauge\n")

	keys := make([]stageKey, 0, len(c.stageVUs))
	for k := range c.stageVUs {
		keys = append(keys, k)
	}
	sortStageKeys(keys)
	for _, k := range keys {
		vus := c.stageVUs[k]
		fmt.Fprintf(sb, "mcpdrill_stage_vus{run_id=%q,stage_id=%q} %d %d\n", k.runID, k.stageID, vus, timestamp)
	}
}

func sortOpKeys(keys []opKey) {
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].operation != keys[j].operation {
			return keys[i].operation < keys[j].operation
		}
		return keys[i].toolName < keys[j].toolName
	})
}

func sortStageKeys(keys []stageKey) {
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].runID != keys[j].runID {
			return keys[i].runID < keys[j].runID
		}
		return keys[i].stageID < keys[j].stageID
	})
}

// IngestTelemetryBatch processes a telemetry batch and updates metrics.
// This should be called when telemetry is received from workers.
func (c *Collector) IngestTelemetryBatch(operations []analysis.OperationResult) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, op := range operations {
		key := opKey{operation: op.Operation, toolName: op.ToolName}
		c.operationCounts[key]++

		if c.operationDurations[key] == nil {
			c.operationDurations[key] = &histogramData{}
		}
		c.operationDurations[key].sum += float64(op.LatencyMs) / 1000.0
		c.operationDurations[key].count++

		if !op.OK {
			c.operationErrors[key]++
		}
	}
}

// UpdateWorkerHealth updates health metrics for a specific worker.
// This should be called when worker heartbeats are received.
func (c *Collector) UpdateWorkerHealth(workerID string, cpuPercent float64, memBytes int64, activeVUs int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.workerHealth[workerID] = &workerHealthData{
		cpuPercent: cpuPercent,
		memoryMB:   float64(memBytes) / (1024 * 1024),
		activeVUs:  activeVUs,
	}
}

// RemoveWorker removes a worker from health metrics.
func (c *Collector) RemoveWorker(workerID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.workerHealth, workerID)
}

// Reset clears all collected metrics.
func (c *Collector) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.runCounts = make(map[string]int64)
	c.runDurations = make(map[string]*histogramData)
	c.runStates = make(map[runStateKey]int)
	c.workerHealth = make(map[string]*workerHealthData)
	c.operationCounts = make(map[opKey]int64)
	c.operationDurations = make(map[opKey]*histogramData)
	c.operationErrors = make(map[opKey]int64)
	c.stageDurations = make(map[stageKey]float64)
	c.stageVUs = make(map[stageKey]int)
}
