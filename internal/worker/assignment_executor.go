// Package worker provides the worker runtime for executing load test assignments.
package worker

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/mcp"
	"github.com/bc-dunia/mcpdrill/internal/session"
	"github.com/bc-dunia/mcpdrill/internal/transport"
	"github.com/bc-dunia/mcpdrill/internal/types"
	"github.com/bc-dunia/mcpdrill/internal/vu"
)

// AssignmentExecutor manages the execution of work assignments from the control plane.
type AssignmentExecutor struct {
	workerID         string
	allowPrivateNets []string
	telemetryShipper *TelemetryShipper

	mu        sync.RWMutex
	active    map[string]*runningAssignment  // LeaseID -> assignment
	runLeases map[string]map[string]struct{} // RunID -> set of LeaseIDs
}

// runningAssignment tracks a currently executing assignment.
type runningAssignment struct {
	assignment  types.WorkerAssignment
	engine      *vu.Engine
	sessionMgr  *session.Manager
	cancel      context.CancelFunc
	startedAt   time.Time
	telemetryCh chan *vu.OperationResult
}

// NewAssignmentExecutor creates a new assignment executor.
func NewAssignmentExecutor(workerID string, allowPrivateNets []string, shipper *TelemetryShipper) *AssignmentExecutor {
	return &AssignmentExecutor{
		workerID:         workerID,
		allowPrivateNets: allowPrivateNets,
		telemetryShipper: shipper,
		active:           make(map[string]*runningAssignment),
		runLeases:        make(map[string]map[string]struct{}),
	}
}

// Execute starts executing an assignment. It is idempotent - calling with the same
// LeaseID will be a no-op if already running.
func (e *AssignmentExecutor) Execute(ctx context.Context, a types.WorkerAssignment) error {
	// Dedupe by LeaseID
	e.mu.Lock()
	if _, exists := e.active[a.LeaseID]; exists {
		e.mu.Unlock()
		return nil // Already running
	}

	// Validate VU count
	vuCount := a.VUIDEnd - a.VUIDStart
	if vuCount <= 0 {
		e.mu.Unlock()
		return fmt.Errorf("invalid VU range: start=%d, end=%d", a.VUIDStart, a.VUIDEnd)
	}

	// Create assignment-scoped context
	assignCtx, cancel := context.WithCancel(ctx)

	// Register assignment
	running := &runningAssignment{
		assignment:  a,
		cancel:      cancel,
		startedAt:   time.Now(),
		telemetryCh: make(chan *vu.OperationResult, 1000),
	}
	e.active[a.LeaseID] = running

	// Track lease by run for stop signals
	if e.runLeases[a.RunID] == nil {
		e.runLeases[a.RunID] = make(map[string]struct{})
	}
	e.runLeases[a.RunID][a.LeaseID] = struct{}{}
	e.mu.Unlock()

	// Execute in goroutine
	go func() {
		defer e.cleanupAssignment(a.RunID, a.LeaseID)

		if err := e.executeAssignment(assignCtx, running); err != nil {
			log.Printf("[Worker] Assignment %s failed: %v", a.LeaseID, err)
		}
	}()

	return nil
}

// executeAssignment performs the actual work for an assignment.
func (e *AssignmentExecutor) executeAssignment(ctx context.Context, running *runningAssignment) error {
	a := running.assignment

	log.Printf("[Worker] Starting assignment: run=%s stage=%s lease=%s vus=%d-%d duration=%dms",
		a.RunID, a.Stage, a.LeaseID, a.VUIDStart, a.VUIDEnd, a.DurationMs)

	// 1. Build transport config
	transportCfg := e.buildTransportConfig(a)

	// 2. Build and create transport adapter
	adapter := transport.NewStreamableHTTPAdapter()

	// 3. Build session config
	sessionCfg := e.buildSessionConfig(a, transportCfg, adapter)

	// 4. Create session manager
	sessionMgr, err := session.NewManager(sessionCfg)
	if err != nil {
		return fmt.Errorf("create session manager: %w", err)
	}
	running.sessionMgr = sessionMgr
	sessionMgr.Start(ctx)

	// 5. Build VU config
	vuCfg := e.buildVUConfig(a, sessionMgr, adapter, transportCfg)

	// 6. Create VU engine
	engine, err := vu.NewEngine(vuCfg)
	if err != nil {
		sessionMgr.Close(ctx)
		return fmt.Errorf("create VU engine: %w", err)
	}
	running.engine = engine

	// 7. Start telemetry collection (reader -> shipper)
	go e.collectResults(ctx, running)

	// 8. Start the engine
	if err := engine.Start(ctx); err != nil {
		sessionMgr.Close(ctx)
		return fmt.Errorf("start VU engine: %w", err)
	}

	// 9. Wait for duration or cancellation
	durationTimer := time.NewTimer(time.Duration(a.DurationMs) * time.Millisecond)
	defer durationTimer.Stop()

	select {
	case <-durationTimer.C:
		log.Printf("[Worker] Assignment %s completed (duration expired)", a.LeaseID)
	case <-ctx.Done():
		log.Printf("[Worker] Assignment %s stopped (context cancelled)", a.LeaseID)
	}

	// 10. Graceful shutdown
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer stopCancel()

	if err := engine.Stop(stopCtx); err != nil {
		log.Printf("[Worker] Engine stop error: %v", err)
	}

	if err := sessionMgr.Close(stopCtx); err != nil {
		log.Printf("[Worker] Session manager close error: %v", err)
	}

	// Close telemetry channel to signal reader to exit
	close(running.telemetryCh)

	return nil
}

// collectResults reads from engine results and forwards to telemetry shipper.
// This runs in a separate goroutine to avoid blocking the engine.
func (e *AssignmentExecutor) collectResults(ctx context.Context, running *runningAssignment) {
	a := running.assignment
	results := running.engine.Results()

	for {
		select {
		case result, ok := <-results:
			if !ok {
				// Engine results channel closed, flush remaining
				return
			}

			// Convert to OperationOutcome
			outcome := ConvertToOutcome(result, a, e.workerID)

			// Send to shipper (non-blocking via buffered channel)
			e.telemetryShipper.Ship(a.RunID, outcome)

		case <-ctx.Done():
			return
		}
	}
}

// buildTransportConfig creates transport configuration from assignment.
func (e *AssignmentExecutor) buildTransportConfig(a types.WorkerAssignment) *transport.TransportConfig {
	cfg := &transport.TransportConfig{
		Endpoint:             a.Target.URL,
		Headers:              a.Target.GetHeadersWithAuth(),
		AllowPrivateNetworks: e.allowPrivateNets,
		Timeouts:             transport.DefaultTimeoutConfig(),
	}

	// Map redirect policy if present
	if a.Target.RedirectPolicy != nil {
		cfg.RedirectPolicy = &transport.RedirectPolicyConfig{
			Mode:         a.Target.RedirectPolicy.Mode,
			MaxRedirects: a.Target.RedirectPolicy.MaxRedirects,
			Allowlist:    a.Target.RedirectPolicy.Allowlist,
		}
	}

	return cfg
}

// buildSessionConfig creates session configuration from assignment.
func (e *AssignmentExecutor) buildSessionConfig(a types.WorkerAssignment, transportCfg *transport.TransportConfig, adapter transport.Adapter) *session.SessionConfig {
	cfg := &session.SessionConfig{
		Mode:                  mapSessionMode(a.SessionPolicy.Mode),
		PoolSize:              a.SessionPolicy.PoolSize,
		TTLMs:                 a.SessionPolicy.TTLMs,
		MaxIdleMs:             a.SessionPolicy.MaxIdleMs,
		TransportConfig:       transportCfg,
		Adapter:               adapter,
		ProtocolVersion:       a.Target.ProtocolVersion,
		ProtocolVersionPolicy: mcp.ParseVersionPolicy(a.Target.ProtocolVersionPolicy),
	}

	// Set defaults if not specified
	if cfg.PoolSize <= 0 && cfg.Mode == session.ModePool {
		cfg.PoolSize = 10
	}

	return cfg
}

// buildVUConfig creates VU engine configuration from assignment.
func (e *AssignmentExecutor) buildVUConfig(a types.WorkerAssignment, sessionMgr *session.Manager, adapter transport.Adapter, transportCfg *transport.TransportConfig) *vu.VUConfig {
	vuCount := a.VUIDEnd - a.VUIDStart

	return &vu.VUConfig{
		RunID:            a.RunID,
		StageID:          a.StageID,
		AssignmentID:     a.LeaseID, // Use LeaseID for unique VU IDs
		WorkerID:         e.workerID,
		LeaseID:          a.LeaseID,
		Load:             vu.LoadTarget{TargetVUs: vuCount},
		OperationMix:     mapOperationMix(a.Workload.OpMix),
		InFlightPerVU:    1,
		ThinkTime:        vu.ThinkTimeConfig{BaseMs: 0, JitterMs: 0},
		SessionManager:   sessionMgr,
		TransportAdapter: adapter,
		TransportConfig:  transportCfg,
		Mode:             vu.ModeNormal,
		UserJourney:      vu.DefaultUserJourneyConfig(),
	}
}

// cleanupAssignment removes an assignment from tracking.
func (e *AssignmentExecutor) cleanupAssignment(runID, leaseID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	delete(e.active, leaseID)

	if leases, ok := e.runLeases[runID]; ok {
		delete(leases, leaseID)
		if len(leases) == 0 {
			delete(e.runLeases, runID)
		}
	}
}

// StopRun stops all assignments for a given run.
// If immediate is true, contexts are cancelled immediately.
// Otherwise, engines are stopped gracefully (drain mode).
func (e *AssignmentExecutor) StopRun(runID string, immediate bool) {
	e.mu.RLock()
	leases, ok := e.runLeases[runID]
	if !ok {
		e.mu.RUnlock()
		return
	}

	// Collect running assignments to stop
	toStop := make([]*runningAssignment, 0, len(leases))
	for leaseID := range leases {
		if running, exists := e.active[leaseID]; exists {
			toStop = append(toStop, running)
		}
	}
	e.mu.RUnlock()

	// Stop each assignment
	for _, running := range toStop {
		if immediate {
			// Immediate: cancel context first
			running.cancel()
		} else {
			// Drain: just cancel context, executeAssignment will handle graceful shutdown
			running.cancel()
		}
	}

	if len(toStop) > 0 {
		log.Printf("[Worker] Stopped %d assignment(s) for run %s (immediate=%v)", len(toStop), runID, immediate)
	}
}

// ActiveAssignments returns the number of currently active assignments.
func (e *AssignmentExecutor) ActiveAssignments() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.active)
}

// mapSessionMode converts string session mode to session.SessionMode.
func mapSessionMode(mode string) session.SessionMode {
	switch mode {
	case "reuse":
		return session.ModeReuse
	case "per_request":
		return session.ModePerRequest
	case "pool":
		return session.ModePool
	case "churn":
		return session.ModeChurn
	default:
		return session.ModeReuse
	}
}

// mapOperationMix converts types.OpMixEntry to vu.OperationMix.
func mapOperationMix(entries []types.OpMixEntry) *vu.OperationMix {
	ops := make([]vu.OperationWeight, len(entries))
	for i, e := range entries {
		ops[i] = vu.OperationWeight{
			Operation:  vu.OperationType(e.Operation),
			Weight:     e.Weight,
			ToolName:   e.ToolName,
			Arguments:  e.Arguments,
			URI:        e.URI,
			PromptName: e.PromptName,
		}
	}
	return &vu.OperationMix{Operations: ops}
}
