package vu

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/session"
	"github.com/bc-dunia/mcpdrill/internal/transport"
)

type mockAdapter struct {
	id string
}

func (m *mockAdapter) ID() string {
	return m.id
}

func (m *mockAdapter) Connect(ctx context.Context, config *transport.TransportConfig) (transport.Connection, error) {
	return &mockConnection{sessionID: "mock-session-123"}, nil
}

type mockConnection struct {
	sessionID   string
	lastEventID string
	callCount   atomic.Int64
	failNext    atomic.Bool
	latencyMs   int64
	mu          sync.Mutex
}

func (m *mockConnection) Initialize(ctx context.Context, params *transport.InitializeParams) (*transport.OperationOutcome, error) {
	return &transport.OperationOutcome{
		Operation: transport.OpInitialize,
		OK:        true,
		SessionID: m.sessionID,
		Result:    json.RawMessage(`{"protocolVersion":"2024-11-05","capabilities":{},"serverInfo":{"name":"mock","version":"1.0"}}`),
	}, nil
}

func (m *mockConnection) SendInitialized(ctx context.Context) (*transport.OperationOutcome, error) {
	return &transport.OperationOutcome{
		Operation: transport.OpInitialized,
		OK:        true,
	}, nil
}

func (m *mockConnection) ToolsList(ctx context.Context, cursor *string) (*transport.OperationOutcome, error) {
	m.callCount.Add(1)

	if m.latencyMs > 0 {
		time.Sleep(time.Duration(m.latencyMs) * time.Millisecond)
	}

	if m.failNext.Load() {
		m.failNext.Store(false)
		return &transport.OperationOutcome{
			Operation: transport.OpToolsList,
			OK:        false,
			Error: &transport.OperationError{
				Type:    transport.ErrorTypeHTTP,
				Code:    transport.CodeHTTPServerError,
				Message: "server error",
			},
		}, nil
	}

	return &transport.OperationOutcome{
		Operation: transport.OpToolsList,
		OK:        true,
		Result:    json.RawMessage(`{"tools":[{"name":"echo","description":"Echo tool"}]}`),
	}, nil
}

func (m *mockConnection) ToolsCall(ctx context.Context, params *transport.ToolsCallParams) (*transport.OperationOutcome, error) {
	m.callCount.Add(1)

	if m.latencyMs > 0 {
		time.Sleep(time.Duration(m.latencyMs) * time.Millisecond)
	}

	if m.failNext.Load() {
		m.failNext.Store(false)
		return &transport.OperationOutcome{
			Operation: transport.OpToolsCall,
			ToolName:  params.Name,
			OK:        false,
			Error: &transport.OperationError{
				Type:    transport.ErrorTypeTool,
				Code:    transport.CodeToolError,
				Message: "tool error",
			},
		}, nil
	}

	return &transport.OperationOutcome{
		Operation: transport.OpToolsCall,
		ToolName:  params.Name,
		OK:        true,
		Result:    json.RawMessage(`{"content":[{"type":"text","text":"hello"}]}`),
	}, nil
}

func (m *mockConnection) Ping(ctx context.Context) (*transport.OperationOutcome, error) {
	m.callCount.Add(1)

	if m.latencyMs > 0 {
		time.Sleep(time.Duration(m.latencyMs) * time.Millisecond)
	}

	return &transport.OperationOutcome{
		Operation: transport.OpPing,
		OK:        true,
	}, nil
}

func (m *mockConnection) ResourcesList(ctx context.Context, cursor *string) (*transport.OperationOutcome, error) {
	m.callCount.Add(1)
	return &transport.OperationOutcome{
		Operation: transport.OpResourcesList,
		OK:        true,
	}, nil
}

func (m *mockConnection) ResourcesRead(ctx context.Context, params *transport.ResourcesReadParams) (*transport.OperationOutcome, error) {
	m.callCount.Add(1)
	return &transport.OperationOutcome{
		Operation: transport.OpResourcesRead,
		OK:        true,
	}, nil
}

func (m *mockConnection) PromptsList(ctx context.Context, cursor *string) (*transport.OperationOutcome, error) {
	m.callCount.Add(1)
	return &transport.OperationOutcome{
		Operation: transport.OpPromptsList,
		OK:        true,
	}, nil
}

func (m *mockConnection) PromptsGet(ctx context.Context, params *transport.PromptsGetParams) (*transport.OperationOutcome, error) {
	m.callCount.Add(1)
	return &transport.OperationOutcome{
		Operation: transport.OpPromptsGet,
		OK:        true,
	}, nil
}

func (m *mockConnection) Close() error {
	return nil
}

func (m *mockConnection) SessionID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessionID
}

func (m *mockConnection) SetSessionID(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessionID = sessionID
}

func (m *mockConnection) SetLastEventID(eventID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastEventID = eventID
}

func createTestConfig(t *testing.T) *VUConfig {
	adapter := &mockAdapter{id: "mock"}
	transportConfig := &transport.TransportConfig{
		Endpoint: "http://localhost:8080",
		Timeouts: transport.DefaultTimeoutConfig(),
	}

	sessionConfig := &session.SessionConfig{
		Mode:            session.ModeReuse,
		PoolSize:        10,
		TTLMs:           60000,
		MaxIdleMs:       30000,
		Adapter:         adapter,
		TransportConfig: transportConfig,
	}

	sessionMgr, err := session.NewManager(sessionConfig)
	if err != nil {
		t.Fatalf("failed to create session manager: %v", err)
	}

	return &VUConfig{
		RunID:        "test-run",
		StageID:      "test-stage",
		AssignmentID: "test-assignment",
		WorkerID:     "test-worker",
		LeaseID:      "test-lease",
		Load: LoadTarget{
			TargetVUs: 2,
			TargetRPS: 0,
		},
		OperationMix: &OperationMix{
			Operations: []OperationWeight{
				{Operation: OpToolsList, Weight: 40},
				{Operation: OpToolsCall, Weight: 50, ToolName: "echo"},
				{Operation: OpPing, Weight: 10},
			},
		},
		InFlightPerVU:    2,
		ThinkTime:        ThinkTimeConfig{BaseMs: 10, JitterMs: 5},
		SessionManager:   sessionMgr,
		TransportAdapter: adapter,
		TransportConfig:  transportConfig,
		Mode:             ModeNormal,
	}
}

func TestOperationSampler_WeightedDistribution(t *testing.T) {
	mix := &OperationMix{
		Operations: []OperationWeight{
			{Operation: OpToolsList, Weight: 10},
			{Operation: OpToolsCall, Weight: 50, ToolName: "echo"},
			{Operation: OpPing, Weight: 5},
		},
	}

	sampler, err := NewOperationSampler(mix, 12345)
	if err != nil {
		t.Fatalf("failed to create sampler: %v", err)
	}

	if sampler.TotalWeight() != 65 {
		t.Errorf("expected total weight 65, got %d", sampler.TotalWeight())
	}

	counts := make(map[OperationType]int)
	iterations := 10000

	for i := 0; i < iterations; i++ {
		op := sampler.Sample()
		counts[op.Operation]++
	}

	toolsListPct := float64(counts[OpToolsList]) / float64(iterations) * 100
	toolsCallPct := float64(counts[OpToolsCall]) / float64(iterations) * 100
	pingPct := float64(counts[OpPing]) / float64(iterations) * 100

	if toolsListPct < 10 || toolsListPct > 20 {
		t.Errorf("tools/list percentage %.1f%% outside expected range [10%%, 20%%]", toolsListPct)
	}

	if toolsCallPct < 70 || toolsCallPct > 85 {
		t.Errorf("tools/call percentage %.1f%% outside expected range [70%%, 85%%]", toolsCallPct)
	}

	if pingPct < 3 || pingPct > 12 {
		t.Errorf("ping percentage %.1f%% outside expected range [3%%, 12%%]", pingPct)
	}
}

func TestOperationSampler_EmptyMix(t *testing.T) {
	_, err := NewOperationSampler(nil, 12345)
	if err != ErrNoOperations {
		t.Errorf("expected ErrNoOperations, got %v", err)
	}

	_, err = NewOperationSampler(&OperationMix{}, 12345)
	if err != ErrNoOperations {
		t.Errorf("expected ErrNoOperations for empty mix, got %v", err)
	}
}

func TestThinkTimeSampler(t *testing.T) {
	config := ThinkTimeConfig{BaseMs: 100, JitterMs: 50}
	sampler := NewThinkTimeSampler(config, 12345)

	if sampler.BaseMs() != 100 {
		t.Errorf("expected base 100, got %d", sampler.BaseMs())
	}

	if sampler.JitterMs() != 50 {
		t.Errorf("expected jitter 50, got %d", sampler.JitterMs())
	}

	for i := 0; i < 100; i++ {
		thinkTime := sampler.Sample()
		if thinkTime < 100 || thinkTime >= 150 {
			t.Errorf("think time %d outside expected range [100, 150)", thinkTime)
		}
	}
}

func TestRateLimiter_Disabled(t *testing.T) {
	limiter := NewRateLimiter(0)

	if limiter.Enabled() {
		t.Error("expected limiter to be disabled")
	}

	ctx := context.Background()
	if err := limiter.Acquire(ctx); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !limiter.TryAcquire() {
		t.Error("expected TryAcquire to succeed when disabled")
	}
}

func TestRateLimiter_Enabled(t *testing.T) {
	limiter := NewRateLimiter(100)

	if !limiter.Enabled() {
		t.Error("expected limiter to be enabled")
	}

	if limiter.TargetRPS() != 100 {
		t.Errorf("expected target RPS 100, got %f", limiter.TargetRPS())
	}

	for i := 0; i < 50; i++ {
		if !limiter.TryAcquire() {
			t.Errorf("expected TryAcquire to succeed for request %d", i)
		}
	}
}

func TestRateLimiter_ContextCancellation(t *testing.T) {
	limiter := NewRateLimiter(1)

	for i := 0; i < 10; i++ {
		limiter.TryAcquire()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := limiter.Acquire(ctx)
	if err != context.DeadlineExceeded {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
}

func TestRateLimiter_UpdateTargetRPS(t *testing.T) {
	limiter := NewRateLimiter(100)

	limiter.UpdateTargetRPS(200)
	if limiter.TargetRPS() != 200 {
		t.Errorf("expected target RPS 200, got %f", limiter.TargetRPS())
	}

	limiter.UpdateTargetRPS(0)
	if limiter.Enabled() {
		t.Error("expected limiter to be disabled after setting RPS to 0")
	}
}

func TestInFlightLimiter_Basic(t *testing.T) {
	limiter := NewInFlightLimiter(3)

	if limiter.MaxInFlight() != 3 {
		t.Errorf("expected max in-flight 3, got %d", limiter.MaxInFlight())
	}

	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if err := limiter.Acquire(ctx); err != nil {
			t.Errorf("unexpected error on acquire %d: %v", i, err)
		}
	}

	if limiter.Current() != 3 {
		t.Errorf("expected current 3, got %d", limiter.Current())
	}

	if limiter.TryAcquire() {
		t.Error("expected TryAcquire to fail when at limit")
	}

	limiter.Release()

	if limiter.Current() != 2 {
		t.Errorf("expected current 2 after release, got %d", limiter.Current())
	}

	if !limiter.TryAcquire() {
		t.Error("expected TryAcquire to succeed after release")
	}
}

func TestInFlightLimiter_ContextCancellation(t *testing.T) {
	limiter := NewInFlightLimiter(1)

	ctx := context.Background()
	if err := limiter.Acquire(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := limiter.Acquire(ctx)
	if err != context.DeadlineExceeded {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
}

func TestInFlightLimiter_Concurrent(t *testing.T) {
	limiter := NewInFlightLimiter(5)
	ctx := context.Background()

	var wg sync.WaitGroup
	var maxConcurrent atomic.Int64

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			if err := limiter.Acquire(ctx); err != nil {
				return
			}
			defer limiter.Release()

			current := int64(limiter.Current())
			for {
				max := maxConcurrent.Load()
				if current <= max {
					break
				}
				if maxConcurrent.CompareAndSwap(max, current) {
					break
				}
			}

			time.Sleep(10 * time.Millisecond)
		}()
	}

	wg.Wait()

	if maxConcurrent.Load() > 5 {
		t.Errorf("max concurrent %d exceeded limit of 5", maxConcurrent.Load())
	}
}

func TestVUInstance_StateTransitions(t *testing.T) {
	vu := NewVUInstance("test-vu-1", 12345)

	if vu.State() != StateIdle {
		t.Errorf("expected initial state Idle, got %s", vu.State())
	}

	vu.SetState(StateInitializing)
	if vu.State() != StateInitializing {
		t.Errorf("expected state Initializing, got %s", vu.State())
	}

	vu.SetState(StateRunning)
	if vu.State() != StateRunning {
		t.Errorf("expected state Running, got %s", vu.State())
	}

	vu.SetState(StateDraining)
	if vu.State() != StateDraining {
		t.Errorf("expected state Draining, got %s", vu.State())
	}

	vu.SetState(StateStopped)
	if vu.State() != StateStopped {
		t.Errorf("expected state Stopped, got %s", vu.State())
	}
}

func TestVUMetrics_Snapshot(t *testing.T) {
	metrics := NewVUMetrics()

	metrics.ActiveVUs.Store(5)
	metrics.TotalVUsCreated.Store(10)
	metrics.TotalOperations.Store(100)
	metrics.SuccessfulOperations.Store(95)
	metrics.FailedOperations.Store(5)

	snapshot := metrics.Snapshot()

	if snapshot.ActiveVUs != 5 {
		t.Errorf("expected ActiveVUs 5, got %d", snapshot.ActiveVUs)
	}

	if snapshot.TotalVUsCreated != 10 {
		t.Errorf("expected TotalVUsCreated 10, got %d", snapshot.TotalVUsCreated)
	}

	if snapshot.TotalOperations != 100 {
		t.Errorf("expected TotalOperations 100, got %d", snapshot.TotalOperations)
	}

	if snapshot.SuccessfulOperations != 95 {
		t.Errorf("expected SuccessfulOperations 95, got %d", snapshot.SuccessfulOperations)
	}

	if snapshot.FailedOperations != 5 {
		t.Errorf("expected FailedOperations 5, got %d", snapshot.FailedOperations)
	}
}

func TestEngine_Creation(t *testing.T) {
	config := createTestConfig(t)

	engine, err := NewEngine(config)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	if engine.Config() != config {
		t.Error("config mismatch")
	}

	if engine.IsClosed() {
		t.Error("engine should not be closed initially")
	}
}

func TestEngine_InvalidConfig(t *testing.T) {
	_, err := NewEngine(nil)
	if err != ErrInvalidConfig {
		t.Errorf("expected ErrInvalidConfig, got %v", err)
	}

	_, err = NewEngine(&VUConfig{})
	if err != ErrNoOperations {
		t.Errorf("expected ErrNoOperations, got %v", err)
	}
}

func TestEngine_NormalMode(t *testing.T) {
	config := createTestConfig(t)
	config.Load.TargetVUs = 3
	config.ThinkTime = ThinkTimeConfig{BaseMs: 5, JitterMs: 2}

	engine, err := NewEngine(config)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	config.SessionManager.(*session.Manager).Start(ctx)

	if err := engine.Start(ctx); err != nil {
		t.Fatalf("failed to start engine: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	snapshot := engine.MetricsSnapshot()
	if snapshot.TotalVUsCreated != 3 {
		t.Errorf("expected 3 VUs created, got %d", snapshot.TotalVUsCreated)
	}

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()

	if err := engine.Stop(stopCtx); err != nil {
		t.Errorf("failed to stop engine: %v", err)
	}

	if !engine.IsClosed() {
		t.Error("engine should be closed after stop")
	}

	finalSnapshot := engine.MetricsSnapshot()
	if finalSnapshot.TotalOperations == 0 {
		t.Error("expected some operations to be executed")
	}
}

func TestEngine_SwarmMode(t *testing.T) {
	config := createTestConfig(t)
	config.Mode = ModeSwarm
	config.Load.TargetVUs = 5
	config.SwarmConfig = &SwarmConfig{
		SpawnIntervalMs:  50,
		VULifetimeMs:     100,
		MaxConcurrentVUs: 3,
	}
	config.ThinkTime = ThinkTimeConfig{BaseMs: 5, JitterMs: 2}

	engine, err := NewEngine(config)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	config.SessionManager.(*session.Manager).Start(ctx)

	if err := engine.Start(ctx); err != nil {
		t.Fatalf("failed to start engine: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	snapshot := engine.MetricsSnapshot()

	if snapshot.TotalVUsCreated < 2 {
		t.Errorf("expected at least 2 VUs created in swarm mode, got %d", snapshot.TotalVUsCreated)
	}

	if snapshot.TotalVUsTerminated == 0 {
		t.Error("expected some VUs to be terminated in swarm mode")
	}

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()

	if err := engine.Stop(stopCtx); err != nil {
		t.Errorf("failed to stop engine: %v", err)
	}
}

func TestEngine_UpdateLoad(t *testing.T) {
	config := createTestConfig(t)
	config.Load.TargetVUs = 2
	config.ThinkTime = ThinkTimeConfig{BaseMs: 10, JitterMs: 5}

	engine, err := NewEngine(config)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	config.SessionManager.(*session.Manager).Start(ctx)

	if err := engine.Start(ctx); err != nil {
		t.Fatalf("failed to start engine: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	engine.UpdateLoad(LoadTarget{TargetVUs: 4, TargetRPS: 0})

	time.Sleep(50 * time.Millisecond)

	snapshot := engine.MetricsSnapshot()
	if snapshot.TotalVUsCreated < 4 {
		t.Errorf("expected at least 4 VUs created after scale up, got %d", snapshot.TotalVUsCreated)
	}

	engine.UpdateLoad(LoadTarget{TargetVUs: 1, TargetRPS: 0})

	time.Sleep(50 * time.Millisecond)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()

	if err := engine.Stop(stopCtx); err != nil {
		t.Errorf("failed to stop engine: %v", err)
	}
}

func TestEngine_RateShaping(t *testing.T) {
	config := createTestConfig(t)
	config.Load.TargetVUs = 5
	config.Load.TargetRPS = 50
	config.ThinkTime = ThinkTimeConfig{BaseMs: 0, JitterMs: 0}
	config.InFlightPerVU = 1

	engine, err := NewEngine(config)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	config.SessionManager.(*session.Manager).Start(ctx)

	startTime := time.Now()

	if err := engine.Start(ctx); err != nil {
		t.Fatalf("failed to start engine: %v", err)
	}

	time.Sleep(1 * time.Second)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()

	if err := engine.Stop(stopCtx); err != nil {
		t.Errorf("failed to stop engine: %v", err)
	}

	elapsed := time.Since(startTime).Seconds()
	snapshot := engine.MetricsSnapshot()

	actualRPS := float64(snapshot.TotalOperations) / elapsed

	if actualRPS > 100 {
		t.Errorf("actual RPS %.1f significantly exceeds target 50", actualRPS)
	}
}

func TestEngine_InFlightCap(t *testing.T) {
	config := createTestConfig(t)
	config.Load.TargetVUs = 3
	config.InFlightPerVU = 2
	config.ThinkTime = ThinkTimeConfig{BaseMs: 0, JitterMs: 0}

	engine, err := NewEngine(config)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	config.SessionManager.(*session.Manager).Start(ctx)

	if err := engine.Start(ctx); err != nil {
		t.Fatalf("failed to start engine: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()

	if err := engine.Stop(stopCtx); err != nil {
		t.Errorf("failed to stop engine: %v", err)
	}

	snapshot := engine.MetricsSnapshot()

	maxExpected := int64(config.Load.TargetVUs * config.InFlightPerVU)
	if snapshot.MaxInFlightReached > maxExpected {
		t.Errorf("max in-flight %d exceeded expected cap %d", snapshot.MaxInFlightReached, maxExpected)
	}
}

func TestEngine_Results(t *testing.T) {
	config := createTestConfig(t)
	config.Load.TargetVUs = 1
	config.ThinkTime = ThinkTimeConfig{BaseMs: 10, JitterMs: 5}

	engine, err := NewEngine(config)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	config.SessionManager.(*session.Manager).Start(ctx)

	if err := engine.Start(ctx); err != nil {
		t.Fatalf("failed to start engine: %v", err)
	}

	var results []*OperationResult
	resultsDone := make(chan struct{})

	go func() {
		for result := range engine.Results() {
			results = append(results, result)
		}
		close(resultsDone)
	}()

	time.Sleep(100 * time.Millisecond)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()

	if err := engine.Stop(stopCtx); err != nil {
		t.Errorf("failed to stop engine: %v", err)
	}

	<-resultsDone

	if len(results) == 0 {
		t.Error("expected some results to be emitted")
	}

	for _, result := range results {
		if result.VUID == "" {
			t.Error("result missing VUID")
		}
		if result.Operation == "" {
			t.Error("result missing operation")
		}
	}
}

func TestEngine_DoubleStart(t *testing.T) {
	config := createTestConfig(t)

	engine, err := NewEngine(config)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	ctx := context.Background()
	config.SessionManager.(*session.Manager).Start(ctx)

	if err := engine.Start(ctx); err != nil {
		t.Fatalf("failed to start engine: %v", err)
	}

	if err := engine.Start(ctx); err != nil {
		t.Errorf("second start should be no-op, got error: %v", err)
	}

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()

	engine.Stop(stopCtx)
}

func TestEngine_DoubleStop(t *testing.T) {
	config := createTestConfig(t)

	engine, err := NewEngine(config)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	ctx := context.Background()
	config.SessionManager.(*session.Manager).Start(ctx)

	if err := engine.Start(ctx); err != nil {
		t.Fatalf("failed to start engine: %v", err)
	}

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()

	if err := engine.Stop(stopCtx); err != nil {
		t.Errorf("first stop failed: %v", err)
	}

	if err := engine.Stop(stopCtx); err != nil {
		t.Errorf("second stop should be no-op, got error: %v", err)
	}
}

func TestOperationMix_TotalWeight(t *testing.T) {
	mix := &OperationMix{
		Operations: []OperationWeight{
			{Operation: OpToolsList, Weight: 10},
			{Operation: OpToolsCall, Weight: 50},
			{Operation: OpPing, Weight: 5},
		},
	}

	if mix.TotalWeight() != 65 {
		t.Errorf("expected total weight 65, got %d", mix.TotalWeight())
	}
}

func TestDefaultSwarmConfig(t *testing.T) {
	config := DefaultSwarmConfig()

	if config.SpawnIntervalMs != 1000 {
		t.Errorf("expected spawn interval 1000, got %d", config.SpawnIntervalMs)
	}

	if config.VULifetimeMs != 30000 {
		t.Errorf("expected VU lifetime 30000, got %d", config.VULifetimeMs)
	}

	if config.MaxConcurrentVUs != 100 {
		t.Errorf("expected max concurrent VUs 100, got %d", config.MaxConcurrentVUs)
	}
}

func TestVUEngineError(t *testing.T) {
	err := &VUEngineError{Op: "test", VUID: "vu-1", Err: errEngineClosed}

	expected := "vu vu-1: test: engine closed"
	if err.Error() != expected {
		t.Errorf("expected error %q, got %q", expected, err.Error())
	}

	err2 := &VUEngineError{Op: "test", Err: errInvalidConfig}
	expected2 := "vu engine: test: invalid configuration"
	if err2.Error() != expected2 {
		t.Errorf("expected error %q, got %q", expected2, err2.Error())
	}

	if err.Unwrap() != errEngineClosed {
		t.Error("Unwrap should return underlying error")
	}
}
