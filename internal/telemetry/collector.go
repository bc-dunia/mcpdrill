package telemetry

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/transport"
	"github.com/bc-dunia/mcpdrill/internal/vu"
)

type HealthProvider interface {
	ActiveVUs() int
	ActiveSessions() int64
	InFlightOps() int64
}

type Collector struct {
	config   *CollectorConfig
	queue    *BoundedQueue
	emitter  *Emitter
	provider HealthProvider

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	started atomic.Bool
	closed  atomic.Bool
}

func NewCollector(config *CollectorConfig, emitter *Emitter) *Collector {
	defaults := DefaultCollectorConfig()

	if config == nil {
		config = defaults
	}

	cfg := *config
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = defaults.QueueSize
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = defaults.BatchSize
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = defaults.FlushInterval
	}

	return &Collector{
		config:  &cfg,
		queue:   NewBoundedQueue(cfg.QueueSize),
		emitter: emitter,
	}
}

func (c *Collector) SetHealthProvider(provider HealthProvider) {
	c.provider = provider
}

func (c *Collector) Start(ctx context.Context) error {
	if c.started.Swap(true) {
		return nil
	}

	c.ctx, c.cancel = context.WithCancel(ctx)

	c.wg.Add(1)
	go c.processLoop()

	if c.config.HealthSnapshotInterval > 0 && c.provider != nil {
		c.wg.Add(1)
		go c.healthLoop()
	}

	return nil
}

func (c *Collector) Stop(ctx context.Context) error {
	if c.closed.Swap(true) {
		return nil
	}

	if c.cancel != nil {
		c.cancel()
	}

	c.queue.Close()

	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		return ctx.Err()
	}

	if c.emitter != nil {
		return c.emitter.Close()
	}

	return nil
}

func (c *Collector) RecordOperation(
	outcome *transport.OperationOutcome,
	keys CorrelationKeys,
	tier LogTier,
) {
	if c.closed.Load() {
		return
	}

	log := NewOpLogFromOutcome(outcome, keys, tier)
	record := &TelemetryRecord{
		Type:  "op_log",
		OpLog: log,
		Tier:  tier,
	}

	c.queue.Enqueue(record)
}

func (c *Collector) RecordVUOperation(
	result *vu.OperationResult,
	runID, executionID, stage, stageID, workerID string,
) {
	if c.closed.Load() || result == nil || result.Outcome == nil {
		return
	}

	keys := CorrelationKeys{
		RunID:       runID,
		ExecutionID: executionID,
		Stage:       stage,
		StageID:     stageID,
		WorkerID:    workerID,
		VUID:        result.VUID,
		SessionID:   result.SessionID,
	}

	tier := Tier1Operation
	if result.Outcome.Error != nil {
		tier = Tier0Lifecycle
	}

	c.RecordOperation(result.Outcome, keys, tier)
}

func (c *Collector) RecordLifecycleEvent(
	eventType string,
	keys CorrelationKeys,
	details map[string]interface{},
) {
	if c.closed.Load() {
		return
	}

	log := &OpLog{
		Version:         OpLogVersion,
		Timestamp:       time.Now(),
		Tier:            Tier0Lifecycle,
		CorrelationKeys: keys,
		Operation:       eventType,
		OK:              true,
	}

	record := &TelemetryRecord{
		Type:  "op_log",
		OpLog: log,
		Tier:  Tier0Lifecycle,
	}

	c.queue.Enqueue(record)
}

func (c *Collector) RecordWorkerHealth(health *WorkerHealth) {
	if c.closed.Load() {
		return
	}

	record := &TelemetryRecord{
		Type:         "worker_health",
		WorkerHealth: health,
		Tier:         Tier0Lifecycle,
	}

	c.queue.Enqueue(record)
}

func (c *Collector) processLoop() {
	defer c.wg.Done()

	ticker := time.NewTicker(c.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			c.drainQueue()
			return
		case <-ticker.C:
			c.processBatch()
		}
	}
}

func (c *Collector) processBatch() {
	records := c.queue.TryDequeueBatch(c.config.BatchSize)
	if len(records) == 0 {
		return
	}

	if c.emitter == nil {
		return
	}

	for _, record := range records {
		if err := c.emitter.EmitRecord(record); err != nil {
			continue
		}
	}
}

func (c *Collector) drainQueue() {
	for {
		records := c.queue.TryDequeueBatch(c.config.BatchSize)
		if len(records) == 0 {
			break
		}

		if c.emitter == nil {
			continue
		}

		for _, record := range records {
			c.emitter.EmitRecord(record)
		}
	}

	if c.emitter != nil {
		c.emitter.Flush()
	}
}

func (c *Collector) healthLoop() {
	defer c.wg.Done()

	ticker := time.NewTicker(c.config.HealthSnapshotInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.captureHealth()
		}
	}
}

func (c *Collector) captureHealth() {
	if c.provider == nil {
		return
	}

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	droppedTier2, _ := c.queue.ResetDropCounts()
	stats := c.queue.Stats()

	health := &WorkerHealth{
		Timestamp:      time.Now(),
		WorkerID:       c.config.WorkerID,
		CPUPercent:     0, // CPU measurement requires external package, deferred
		MemBytes:       int64(memStats.Alloc),
		ActiveVUs:      int64(c.provider.ActiveVUs()),
		ActiveSessions: c.provider.ActiveSessions(),
		InFlightOps:    c.provider.InFlightOps(),
		QueueDepth:     stats.Depth,
		QueueCapacity:  stats.Capacity,
		DroppedTier2:   droppedTier2,
	}

	c.RecordWorkerHealth(health)
}

func (c *Collector) QueueStats() QueueStats {
	return c.queue.Stats()
}

func (c *Collector) Queue() *BoundedQueue {
	return c.queue
}

func generateBatchID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

type VUEngineAdapter struct {
	engine *vu.Engine
}

func NewVUEngineAdapter(engine *vu.Engine) *VUEngineAdapter {
	return &VUEngineAdapter{engine: engine}
}

func (a *VUEngineAdapter) ActiveVUs() int {
	return a.engine.ActiveVUs()
}

func (a *VUEngineAdapter) ActiveSessions() int64 {
	return int64(a.engine.VUCount())
}

func (a *VUEngineAdapter) InFlightOps() int64 {
	return a.engine.Metrics().InFlightOperations.Load()
}

func StartVUResultsCollector(
	ctx context.Context,
	collector *Collector,
	engine *vu.Engine,
	runID, executionID, stage, stageID, workerID string,
) {
	go func() {
		results := engine.Results()
		for {
			select {
			case <-ctx.Done():
				return
			case result, ok := <-results:
				if !ok {
					return
				}
				collector.RecordVUOperation(result, runID, executionID, stage, stageID, workerID)
			}
		}
	}()
}
