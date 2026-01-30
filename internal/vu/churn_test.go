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

type mockChurnAdapter struct {
	connectCount atomic.Int64
}

func (m *mockChurnAdapter) ID() string {
	return "mock-churn"
}

func (m *mockChurnAdapter) Connect(ctx context.Context, config *transport.TransportConfig) (transport.Connection, error) {
	m.connectCount.Add(1)
	return newMockChurnConnection(), nil
}

type mockChurnConnection struct {
	sessionID   string
	lastEventID string
	closed      atomic.Bool
	mu          sync.Mutex
}

var churnSessionCounter atomic.Int64

func newMockChurnConnection() *mockChurnConnection {
	n := churnSessionCounter.Add(1)
	return &mockChurnConnection{
		sessionID: "churn_ses_" + formatChurnInt64(n),
	}
}

func formatChurnInt64(n int64) string {
	const digits = "0123456789abcdef"
	var buf [16]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = digits[n&0xf]
		n >>= 4
	}
	if i == len(buf) {
		i--
		buf[i] = '0'
	}
	return string(buf[i:])
}

func (m *mockChurnConnection) Initialize(ctx context.Context, params *transport.InitializeParams) (*transport.OperationOutcome, error) {
	result, _ := json.Marshal(map[string]interface{}{
		"protocolVersion": "2025-03-26",
		"capabilities":    map[string]interface{}{},
		"serverInfo": map[string]interface{}{
			"name":    "mock-server",
			"version": "1.0.0",
		},
	})
	return &transport.OperationOutcome{
		Operation: transport.OpInitialize,
		OK:        true,
		Result:    result,
	}, nil
}

func (m *mockChurnConnection) SendInitialized(ctx context.Context) (*transport.OperationOutcome, error) {
	return &transport.OperationOutcome{
		Operation: transport.OpInitialized,
		OK:        true,
	}, nil
}

func (m *mockChurnConnection) ToolsList(ctx context.Context, cursor *string) (*transport.OperationOutcome, error) {
	return &transport.OperationOutcome{
		Operation: transport.OpToolsList,
		OK:        true,
	}, nil
}

func (m *mockChurnConnection) ToolsCall(ctx context.Context, params *transport.ToolsCallParams) (*transport.OperationOutcome, error) {
	return &transport.OperationOutcome{
		Operation: transport.OpToolsCall,
		OK:        true,
	}, nil
}

func (m *mockChurnConnection) Ping(ctx context.Context) (*transport.OperationOutcome, error) {
	return &transport.OperationOutcome{
		Operation: transport.OpPing,
		OK:        true,
	}, nil
}

func (m *mockChurnConnection) ResourcesList(ctx context.Context, cursor *string) (*transport.OperationOutcome, error) {
	return &transport.OperationOutcome{
		Operation: transport.OpResourcesList,
		OK:        true,
	}, nil
}

func (m *mockChurnConnection) ResourcesRead(ctx context.Context, params *transport.ResourcesReadParams) (*transport.OperationOutcome, error) {
	return &transport.OperationOutcome{
		Operation: transport.OpResourcesRead,
		OK:        true,
	}, nil
}

func (m *mockChurnConnection) PromptsList(ctx context.Context, cursor *string) (*transport.OperationOutcome, error) {
	return &transport.OperationOutcome{
		Operation: transport.OpPromptsList,
		OK:        true,
	}, nil
}

func (m *mockChurnConnection) PromptsGet(ctx context.Context, params *transport.PromptsGetParams) (*transport.OperationOutcome, error) {
	return &transport.OperationOutcome{
		Operation: transport.OpPromptsGet,
		OK:        true,
	}, nil
}

func (m *mockChurnConnection) Close() error {
	m.closed.Store(true)
	return nil
}

func (m *mockChurnConnection) SessionID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessionID
}

func (m *mockChurnConnection) SetSessionID(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessionID = sessionID
}

func (m *mockChurnConnection) SetLastEventID(eventID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastEventID = eventID
}

func TestChurnModeOpsBasedDefault(t *testing.T) {
	adapter := &mockChurnAdapter{}
	config := &session.SessionConfig{
		Mode:            session.ModeChurn,
		Adapter:         adapter,
		TransportConfig: &transport.TransportConfig{Endpoint: "http://localhost:8080"},
	}

	mgr, err := session.NewManager(config)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	ctx := context.Background()
	defer mgr.Close(ctx)

	sess1, err := mgr.Acquire(ctx, "vu_1")
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	sess1ID := sess1.ID

	mgr.Release(ctx, sess1)

	sess2, err := mgr.Acquire(ctx, "vu_1")
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}

	if sess2.ID == sess1ID {
		t.Error("Default churn mode (ops=1) should create new session after each operation")
	}

	if adapter.connectCount.Load() != 2 {
		t.Errorf("Expected 2 connections, got %d", adapter.connectCount.Load())
	}
}

func TestChurnModeOpsBasedInterval(t *testing.T) {
	adapter := &mockChurnAdapter{}
	config := &session.SessionConfig{
		Mode:             session.ModeChurn,
		ChurnIntervalOps: 3,
		Adapter:          adapter,
		TransportConfig:  &transport.TransportConfig{Endpoint: "http://localhost:8080"},
	}

	mgr, err := session.NewManager(config)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	ctx := context.Background()
	defer mgr.Close(ctx)

	sess1, err := mgr.Acquire(ctx, "vu_1")
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	sess1ID := sess1.ID
	mgr.Release(ctx, sess1)

	sess2, err := mgr.Acquire(ctx, "vu_1")
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if sess2.ID != sess1ID {
		t.Error("Session should be reused before reaching churn interval")
	}
	mgr.Release(ctx, sess2)

	sess3, err := mgr.Acquire(ctx, "vu_1")
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if sess3.ID != sess1ID {
		t.Error("Session should be reused before reaching churn interval")
	}
	mgr.Release(ctx, sess3)

	sess4, err := mgr.Acquire(ctx, "vu_1")
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if sess4.ID == sess1ID {
		t.Error("Session should be churned after 3 operations")
	}

	if adapter.connectCount.Load() != 2 {
		t.Errorf("Expected 2 connections (initial + after churn), got %d", adapter.connectCount.Load())
	}
}

func TestChurnModeTimeBasedStillWorks(t *testing.T) {
	adapter := &mockChurnAdapter{}
	config := &session.SessionConfig{
		Mode:            session.ModeChurn,
		ChurnIntervalMs: 100,
		Adapter:         adapter,
		TransportConfig: &transport.TransportConfig{Endpoint: "http://localhost:8080"},
	}

	mgr, err := session.NewManager(config)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	ctx := context.Background()
	defer mgr.Close(ctx)

	sess1, err := mgr.Acquire(ctx, "vu_1")
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	sess1ID := sess1.ID
	mgr.Release(ctx, sess1)

	sess2, err := mgr.Acquire(ctx, "vu_1")
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if sess2.ID != sess1ID {
		t.Error("Session should be reused before time interval")
	}
	mgr.Release(ctx, sess2)

	time.Sleep(150 * time.Millisecond)

	sess3, err := mgr.Acquire(ctx, "vu_1")
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if sess3.ID == sess1ID {
		t.Error("Session should be churned after time interval")
	}
}

func TestChurnModeOpsBasedMultipleVUs(t *testing.T) {
	adapter := &mockChurnAdapter{}
	config := &session.SessionConfig{
		Mode:             session.ModeChurn,
		ChurnIntervalOps: 2,
		Adapter:          adapter,
		TransportConfig:  &transport.TransportConfig{Endpoint: "http://localhost:8080"},
	}

	mgr, err := session.NewManager(config)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	ctx := context.Background()
	defer mgr.Close(ctx)

	vu1Sess1, _ := mgr.Acquire(ctx, "vu_1")
	vu1Sess1ID := vu1Sess1.ID
	mgr.Release(ctx, vu1Sess1)

	vu2Sess1, _ := mgr.Acquire(ctx, "vu_2")
	vu2Sess1ID := vu2Sess1.ID
	mgr.Release(ctx, vu2Sess1)

	vu1Sess2, _ := mgr.Acquire(ctx, "vu_1")
	if vu1Sess2.ID != vu1Sess1ID {
		t.Error("VU1 session should be reused (only 1 op)")
	}
	mgr.Release(ctx, vu1Sess2)

	vu1Sess3, _ := mgr.Acquire(ctx, "vu_1")
	if vu1Sess3.ID == vu1Sess1ID {
		t.Error("VU1 session should be churned after 2 ops")
	}

	vu2Sess2, _ := mgr.Acquire(ctx, "vu_2")
	if vu2Sess2.ID != vu2Sess1ID {
		t.Error("VU2 session should still be reused (only 1 op)")
	}
}

func TestChurnModeMetrics(t *testing.T) {
	adapter := &mockChurnAdapter{}
	config := &session.SessionConfig{
		Mode:             session.ModeChurn,
		ChurnIntervalOps: 1,
		Adapter:          adapter,
		TransportConfig:  &transport.TransportConfig{Endpoint: "http://localhost:8080"},
	}

	mgr, err := session.NewManager(config)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	ctx := context.Background()
	defer mgr.Close(ctx)

	metrics := mgr.Metrics()
	if metrics.TotalCreated != 0 {
		t.Errorf("Initial TotalCreated should be 0, got %d", metrics.TotalCreated)
	}

	sess1, _ := mgr.Acquire(ctx, "vu_1")
	metrics = mgr.Metrics()
	if metrics.TotalCreated != 1 {
		t.Errorf("TotalCreated should be 1, got %d", metrics.TotalCreated)
	}
	if metrics.ActiveSessions != 1 {
		t.Errorf("ActiveSessions should be 1, got %d", metrics.ActiveSessions)
	}

	mgr.Release(ctx, sess1)
	metrics = mgr.Metrics()
	if metrics.IdleSessions != 1 {
		t.Errorf("IdleSessions should be 1, got %d", metrics.IdleSessions)
	}

	mgr.Acquire(ctx, "vu_1")
	metrics = mgr.Metrics()
	if metrics.TotalCreated != 2 {
		t.Errorf("TotalCreated should be 2 after churn, got %d", metrics.TotalCreated)
	}
	if metrics.TotalEvicted != 1 {
		t.Errorf("TotalEvicted should be 1 after churn, got %d", metrics.TotalEvicted)
	}
}

func TestChurnModeConcurrent(t *testing.T) {
	adapter := &mockChurnAdapter{}
	config := &session.SessionConfig{
		Mode:             session.ModeChurn,
		ChurnIntervalOps: 5,
		Adapter:          adapter,
		TransportConfig:  &transport.TransportConfig{Endpoint: "http://localhost:8080"},
	}

	mgr, err := session.NewManager(config)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	ctx := context.Background()
	defer mgr.Close(ctx)

	var wg sync.WaitGroup
	numVUs := 10
	opsPerVU := 20

	for i := 0; i < numVUs; i++ {
		wg.Add(1)
		go func(vuID string) {
			defer wg.Done()
			for j := 0; j < opsPerVU; j++ {
				sess, err := mgr.Acquire(ctx, vuID)
				if err != nil {
					t.Errorf("Acquire() error = %v", err)
					return
				}
				time.Sleep(time.Microsecond)
				mgr.Release(ctx, sess)
			}
		}("vu_" + string(rune('0'+i)))
	}

	wg.Wait()

	metrics := mgr.Metrics()
	expectedChurns := numVUs * (opsPerVU / 5)
	if metrics.TotalCreated < int64(expectedChurns) {
		t.Errorf("Expected at least %d sessions created, got %d", expectedChurns, metrics.TotalCreated)
	}
}

func TestChurnModeClose(t *testing.T) {
	adapter := &mockChurnAdapter{}
	config := &session.SessionConfig{
		Mode:             session.ModeChurn,
		ChurnIntervalOps: 10,
		Adapter:          adapter,
		TransportConfig:  &transport.TransportConfig{Endpoint: "http://localhost:8080"},
	}

	mgr, err := session.NewManager(config)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	ctx := context.Background()

	_, err = mgr.Acquire(ctx, "vu_1")
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}

	err = mgr.Close(ctx)
	if err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	_, err = mgr.Acquire(ctx, "vu_2")
	if err == nil {
		t.Error("Acquire() should fail after Close()")
	}
}

func TestChurnModeInvalidate(t *testing.T) {
	adapter := &mockChurnAdapter{}
	config := &session.SessionConfig{
		Mode:             session.ModeChurn,
		ChurnIntervalOps: 100,
		Adapter:          adapter,
		TransportConfig:  &transport.TransportConfig{Endpoint: "http://localhost:8080"},
	}

	mgr, err := session.NewManager(config)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	ctx := context.Background()
	defer mgr.Close(ctx)

	sess1, err := mgr.Acquire(ctx, "vu_1")
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	sess1ID := sess1.ID

	err = mgr.Invalidate(ctx, sess1)
	if err != nil {
		t.Fatalf("Invalidate() error = %v", err)
	}

	sess2, err := mgr.Acquire(ctx, "vu_1")
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}

	if sess2.ID == sess1ID {
		t.Error("Should create new session after invalidation")
	}
}
