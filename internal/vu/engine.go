package vu

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	cfgpkg "github.com/bc-dunia/mcpdrill/internal/config"
)

type Engine struct {
	config      *VUConfig
	sampler     *OperationSampler
	rateLimiter *RateLimiter
	metrics     *VUMetrics
	resultChan  chan *OperationResult

	vus       map[string]*VUInstance
	executors map[string]*VUExecutor
	vuMu      sync.RWMutex

	vuCounter atomic.Int64
	closed    atomic.Bool
	started   atomic.Bool

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewEngine(config *VUConfig) (*Engine, error) {
	if config == nil {
		return nil, ErrInvalidConfig
	}

	if config.OperationMix == nil || len(config.OperationMix.Operations) == 0 {
		return nil, ErrNoOperations
	}

	if config.SessionManager == nil {
		return nil, &VUEngineError{Op: "create", Err: fmt.Errorf("session manager is required")}
	}

	if config.InFlightPerVU <= 0 {
		config.InFlightPerVU = 1
	}

	sampler, err := NewOperationSampler(config.OperationMix, time.Now().UnixNano())
	if err != nil {
		return nil, err
	}

	rateLimiter := NewRateLimiter(config.Load.TargetRPS)

	return &Engine{
		config:      config,
		sampler:     sampler,
		rateLimiter: rateLimiter,
		metrics:     NewVUMetrics(),
		resultChan:  make(chan *OperationResult, cfgpkg.DefaultChannelBufferSize),
		vus:         make(map[string]*VUInstance),
		executors:   make(map[string]*VUExecutor),
	}, nil
}

func (e *Engine) Start(ctx context.Context) error {
	if e.closed.Load() {
		return ErrEngineClosed
	}

	if e.started.Swap(true) {
		return nil
	}

	e.ctx, e.cancel = context.WithCancel(ctx)

	switch e.config.Mode {
	case ModeSwarm:
		e.wg.Add(1)
		go e.runSwarmMode()
	default:
		e.startNormalMode()
	}

	return nil
}

func (e *Engine) Stop(ctx context.Context) error {
	if e.closed.Swap(true) {
		return nil
	}

	if e.cancel != nil {
		e.cancel()
	}

	e.vuMu.RLock()
	for _, executor := range e.executors {
		executor.Stop()
	}
	e.vuMu.RUnlock()

	done := make(chan struct{})
	go func() {
		e.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		return ctx.Err()
	}

	close(e.resultChan)
	return nil
}

func (e *Engine) UpdateLoad(target LoadTarget) {
	e.vuMu.Lock()
	defer e.vuMu.Unlock()

	e.config.Load = target

	if e.rateLimiter != nil {
		e.rateLimiter.UpdateTargetRPS(target.TargetRPS)
	}

	if e.config.Mode == ModeNormal {
		currentVUs := len(e.vus)
		targetVUs := target.TargetVUs

		if targetVUs > currentVUs {
			for i := currentVUs; i < targetVUs; i++ {
				e.spawnVULocked()
			}
		} else if targetVUs < currentVUs {
			toRemove := currentVUs - targetVUs
			removed := 0
			for vuID, executor := range e.executors {
				if removed >= toRemove {
					break
				}
				executor.Stop()
				delete(e.executors, vuID)
				delete(e.vus, vuID)
				e.metrics.TotalVUsTerminated.Add(1)
				removed++
			}
		}
	}
}

func (e *Engine) Metrics() *VUMetrics {
	return e.metrics
}

func (e *Engine) MetricsSnapshot() VUMetricsSnapshot {
	return e.metrics.Snapshot()
}

func (e *Engine) Results() <-chan *OperationResult {
	return e.resultChan
}

func (e *Engine) ActiveVUs() int {
	return int(e.metrics.ActiveVUs.Load())
}

func (e *Engine) startNormalMode() {
	e.vuMu.Lock()
	defer e.vuMu.Unlock()

	for i := 0; i < e.config.Load.TargetVUs; i++ {
		e.spawnVULocked()
	}
}

func (e *Engine) spawnVULocked() {
	vuNum := e.vuCounter.Add(1)
	vuID := fmt.Sprintf("%s-vu-%d", e.config.AssignmentID, vuNum)
	seed := time.Now().UnixNano() + vuNum

	vu := NewVUInstance(vuID, seed)
	e.vus[vuID] = vu
	e.metrics.TotalVUsCreated.Add(1)

	executor := NewVUExecutor(
		vu,
		e.config,
		e.sampler,
		e.rateLimiter,
		e.metrics,
		e.resultChan,
	)
	e.executors[vuID] = executor

	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		executor.Run(e.ctx)
		e.metrics.TotalVUsTerminated.Add(1)
	}()
}

func (e *Engine) runSwarmMode() {
	defer e.wg.Done()

	swarmConfig := e.config.SwarmConfig
	if swarmConfig == nil {
		swarmConfig = DefaultSwarmConfig()
	}

	spawnTicker := time.NewTicker(time.Duration(swarmConfig.SpawnIntervalMs) * time.Millisecond)
	defer spawnTicker.Stop()

	for {
		select {
		case <-e.ctx.Done():
			e.drainAllVUs()
			return
		case <-spawnTicker.C:
			e.vuMu.Lock()
			currentVUs := int(e.metrics.ActiveVUs.Load())
			if currentVUs < swarmConfig.MaxConcurrentVUs && currentVUs < e.config.Load.TargetVUs {
				e.spawnSwarmVULocked(swarmConfig.VULifetimeMs)
			}
			e.vuMu.Unlock()
		}
	}
}

func (e *Engine) spawnSwarmVULocked(lifetimeMs int64) {
	vuNum := e.vuCounter.Add(1)
	vuID := fmt.Sprintf("%s-swarm-vu-%d", e.config.AssignmentID, vuNum)
	seed := time.Now().UnixNano() + vuNum

	vu := NewVUInstance(vuID, seed)
	e.vus[vuID] = vu
	e.metrics.TotalVUsCreated.Add(1)

	executor := NewVUExecutor(
		vu,
		e.config,
		e.sampler,
		e.rateLimiter,
		e.metrics,
		e.resultChan,
	)
	e.executors[vuID] = executor

	vuCtx, vuCancel := context.WithTimeout(e.ctx, time.Duration(lifetimeMs)*time.Millisecond)
	vu.cancel = vuCancel

	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		executor.Run(vuCtx)

		e.vuMu.Lock()
		delete(e.vus, vuID)
		delete(e.executors, vuID)
		e.vuMu.Unlock()

		e.metrics.TotalVUsTerminated.Add(1)
	}()
}

func (e *Engine) drainAllVUs() {
	e.vuMu.RLock()
	for _, executor := range e.executors {
		executor.Stop()
	}
	e.vuMu.RUnlock()

	e.vuMu.RLock()
	for _, executor := range e.executors {
		executor.Wait()
	}
	e.vuMu.RUnlock()
}

func (e *Engine) VUCount() int {
	e.vuMu.RLock()
	defer e.vuMu.RUnlock()
	return len(e.vus)
}

func (e *Engine) Config() *VUConfig {
	return e.config
}

func (e *Engine) IsClosed() bool {
	return e.closed.Load()
}
