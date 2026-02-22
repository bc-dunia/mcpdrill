package session

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/transport"
)

type mockAdapter struct {
	connectCount atomic.Int64
	connectErr   error
	connectDelay time.Duration
}

func (m *mockAdapter) ID() string {
	return "mock"
}

func (m *mockAdapter) Connect(ctx context.Context, config *transport.TransportConfig) (transport.Connection, error) {
	if m.connectDelay > 0 {
		time.Sleep(m.connectDelay)
	}
	m.connectCount.Add(1)
	if m.connectErr != nil {
		return nil, m.connectErr
	}
	return newMockConnection(), nil
}

type mockConnection struct {
	sessionID     string
	lastEventID   string
	closed        atomic.Bool
	initializeErr error
	mu            sync.Mutex
}

func newMockConnection() *mockConnection {
	return &mockConnection{
		sessionID: generateSessionID(),
	}
}

func (m *mockConnection) Initialize(ctx context.Context, params *transport.InitializeParams) (*transport.OperationOutcome, error) {
	if m.initializeErr != nil {
		return nil, m.initializeErr
	}
	result, _ := json.Marshal(map[string]interface{}{
		"protocolVersion": "2025-11-25",
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

func (m *mockConnection) SendInitialized(ctx context.Context) (*transport.OperationOutcome, error) {
	return &transport.OperationOutcome{
		Operation: transport.OpInitialized,
		OK:        true,
	}, nil
}

func (m *mockConnection) ToolsList(ctx context.Context, cursor *string) (*transport.OperationOutcome, error) {
	return &transport.OperationOutcome{
		Operation: transport.OpToolsList,
		OK:        true,
	}, nil
}

func (m *mockConnection) ToolsCall(ctx context.Context, params *transport.ToolsCallParams) (*transport.OperationOutcome, error) {
	return &transport.OperationOutcome{
		Operation: transport.OpToolsCall,
		OK:        true,
	}, nil
}

func (m *mockConnection) Ping(ctx context.Context) (*transport.OperationOutcome, error) {
	return &transport.OperationOutcome{
		Operation: transport.OpPing,
		OK:        true,
	}, nil
}

func (m *mockConnection) ResourcesList(ctx context.Context, cursor *string) (*transport.OperationOutcome, error) {
	return &transport.OperationOutcome{
		Operation: transport.OpResourcesList,
		OK:        true,
	}, nil
}

func (m *mockConnection) ResourcesRead(ctx context.Context, params *transport.ResourcesReadParams) (*transport.OperationOutcome, error) {
	return &transport.OperationOutcome{
		Operation: transport.OpResourcesRead,
		OK:        true,
	}, nil
}

func (m *mockConnection) PromptsList(ctx context.Context, cursor *string) (*transport.OperationOutcome, error) {
	return &transport.OperationOutcome{
		Operation: transport.OpPromptsList,
		OK:        true,
	}, nil
}

func (m *mockConnection) PromptsGet(ctx context.Context, params *transport.PromptsGetParams) (*transport.OperationOutcome, error) {
	return &transport.OperationOutcome{
		Operation: transport.OpPromptsGet,
		OK:        true,
	}, nil
}

func (m *mockConnection) Close() error {
	m.closed.Store(true)
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

func TestManagerCreation(t *testing.T) {
	adapter := &mockAdapter{}
	transportConfig := &transport.TransportConfig{
		Endpoint: "http://localhost:8080",
	}

	tests := []struct {
		name    string
		config  *SessionConfig
		wantErr bool
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name: "nil adapter",
			config: &SessionConfig{
				Mode:            ModeReuse,
				TransportConfig: transportConfig,
			},
			wantErr: true,
		},
		{
			name: "nil transport config",
			config: &SessionConfig{
				Mode:    ModeReuse,
				Adapter: adapter,
			},
			wantErr: true,
		},
		{
			name: "pool mode without pool size",
			config: &SessionConfig{
				Mode:            ModePool,
				PoolSize:        0,
				Adapter:         adapter,
				TransportConfig: transportConfig,
			},
			wantErr: true,
		},
		{
			name: "valid reuse mode",
			config: &SessionConfig{
				Mode:            ModeReuse,
				Adapter:         adapter,
				TransportConfig: transportConfig,
			},
			wantErr: false,
		},
		{
			name: "valid per_request mode",
			config: &SessionConfig{
				Mode:            ModePerRequest,
				Adapter:         adapter,
				TransportConfig: transportConfig,
			},
			wantErr: false,
		},
		{
			name: "valid pool mode",
			config: &SessionConfig{
				Mode:            ModePool,
				PoolSize:        10,
				Adapter:         adapter,
				TransportConfig: transportConfig,
			},
			wantErr: false,
		},
		{
			name: "valid churn mode",
			config: &SessionConfig{
				Mode:            ModeChurn,
				Adapter:         adapter,
				TransportConfig: transportConfig,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr, err := NewManager(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewManager() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && mgr == nil {
				t.Error("NewManager() returned nil manager without error")
			}
		})
	}
}

func TestReuseModeBasic(t *testing.T) {
	adapter := &mockAdapter{}
	config := &SessionConfig{
		Mode:            ModeReuse,
		TTLMs:           60000,
		MaxIdleMs:       30000,
		Adapter:         adapter,
		TransportConfig: &transport.TransportConfig{Endpoint: "http://localhost:8080"},
	}

	mgr, err := NewManager(config)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	ctx := context.Background()
	mgr.Start(ctx)
	defer mgr.Close(ctx)

	session1, err := mgr.Acquire(ctx, "vu_1")
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if session1 == nil {
		t.Fatal("Acquire() returned nil session")
	}

	session2, err := mgr.Acquire(ctx, "vu_1")
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if session2.ID != session1.ID {
		t.Errorf("Reuse mode should return same session, got different IDs: %s vs %s", session1.ID, session2.ID)
	}

	session3, err := mgr.Acquire(ctx, "vu_2")
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if session3.ID == session1.ID {
		t.Error("Different VUs should get different sessions")
	}

	if adapter.connectCount.Load() != 2 {
		t.Errorf("Expected 2 connections (one per VU), got %d", adapter.connectCount.Load())
	}
}

func TestPerRequestModeBasic(t *testing.T) {
	adapter := &mockAdapter{}
	config := &SessionConfig{
		Mode:            ModePerRequest,
		Adapter:         adapter,
		TransportConfig: &transport.TransportConfig{Endpoint: "http://localhost:8080"},
	}

	mgr, err := NewManager(config)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	ctx := context.Background()
	defer mgr.Close(ctx)

	session1, err := mgr.Acquire(ctx, "vu_1")
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}

	mgr.Release(ctx, session1)

	session2, err := mgr.Acquire(ctx, "vu_1")
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}

	if session2.ID == session1.ID {
		t.Error("Per-request mode should create new session each time")
	}

	if adapter.connectCount.Load() != 2 {
		t.Errorf("Expected 2 connections, got %d", adapter.connectCount.Load())
	}
}

func TestPoolModeBasic(t *testing.T) {
	adapter := &mockAdapter{}
	config := &SessionConfig{
		Mode:            ModePool,
		PoolSize:        5,
		TTLMs:           60000,
		MaxIdleMs:       30000,
		Adapter:         adapter,
		TransportConfig: &transport.TransportConfig{Endpoint: "http://localhost:8080"},
	}

	mgr, err := NewManager(config)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	ctx := context.Background()
	mgr.Start(ctx)
	defer mgr.Close(ctx)

	session1, err := mgr.Acquire(ctx, "vu_1")
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}

	mgr.Release(ctx, session1)

	session2, err := mgr.Acquire(ctx, "vu_2")
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}

	if session2.ID != session1.ID {
		t.Error("Pool mode should reuse released sessions")
	}

	if adapter.connectCount.Load() != 1 {
		t.Errorf("Expected 1 connection (reused), got %d", adapter.connectCount.Load())
	}
}

func TestPoolModeBoundedSize(t *testing.T) {
	adapter := &mockAdapter{}
	poolSize := 3
	config := &SessionConfig{
		Mode:            ModePool,
		PoolSize:        poolSize,
		Adapter:         adapter,
		TransportConfig: &transport.TransportConfig{Endpoint: "http://localhost:8080"},
	}

	mgr, err := NewManager(config)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	ctx := context.Background()
	mgr.Start(ctx)
	defer mgr.Close(ctx)

	sessions := make([]*SessionInfo, poolSize)
	for i := 0; i < poolSize; i++ {
		session, err := mgr.Acquire(ctx, "vu_"+string(rune('0'+i)))
		if err != nil {
			t.Fatalf("Acquire() error = %v", err)
		}
		sessions[i] = session
	}

	ctx2, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	_, err = mgr.Acquire(ctx2, "vu_blocked")
	if err == nil {
		t.Error("Expected timeout when pool is exhausted")
	}

	mgr.Release(ctx, sessions[0])

	session, err := mgr.Acquire(ctx, "vu_new")
	if err != nil {
		t.Fatalf("Acquire() after release error = %v", err)
	}
	if session.ID != sessions[0].ID {
		t.Error("Should reuse released session")
	}
}

func TestChurnModeBasic(t *testing.T) {
	adapter := &mockAdapter{}
	config := &SessionConfig{
		Mode:            ModeChurn,
		ChurnIntervalMs: 100,
		Adapter:         adapter,
		TransportConfig: &transport.TransportConfig{Endpoint: "http://localhost:8080"},
	}

	mgr, err := NewManager(config)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	ctx := context.Background()
	defer mgr.Close(ctx)

	session1, err := mgr.Acquire(ctx, "vu_1")
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}

	session2, err := mgr.Acquire(ctx, "vu_1")
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if session2.ID != session1.ID {
		t.Error("Should reuse session before churn interval")
	}

	time.Sleep(150 * time.Millisecond)

	session3, err := mgr.Acquire(ctx, "vu_1")
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if session3.ID == session1.ID {
		t.Error("Should create new session after churn interval")
	}
}

func TestTTLEviction(t *testing.T) {
	adapter := &mockAdapter{}
	config := &SessionConfig{
		Mode:            ModeReuse,
		TTLMs:           100,
		MaxIdleMs:       0,
		Adapter:         adapter,
		TransportConfig: &transport.TransportConfig{Endpoint: "http://localhost:8080"},
	}

	mgr, err := NewManager(config)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	ctx := context.Background()
	mgr.Start(ctx)
	defer mgr.Close(ctx)

	session1, err := mgr.Acquire(ctx, "vu_1")
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	originalID := session1.ID

	time.Sleep(200 * time.Millisecond)

	session2, err := mgr.Acquire(ctx, "vu_1")
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}

	if session2.ID == originalID {
		t.Error("Session should have been evicted due to TTL")
	}
}

func TestIdleEviction(t *testing.T) {
	adapter := &mockAdapter{}
	config := &SessionConfig{
		Mode:            ModeReuse,
		TTLMs:           0,
		MaxIdleMs:       100,
		Adapter:         adapter,
		TransportConfig: &transport.TransportConfig{Endpoint: "http://localhost:8080"},
	}

	mgr, err := NewManager(config)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	ctx := context.Background()
	mgr.Start(ctx)
	defer mgr.Close(ctx)

	session1, err := mgr.Acquire(ctx, "vu_1")
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	originalID := session1.ID

	mgr.Release(ctx, session1)

	time.Sleep(200 * time.Millisecond)

	session2, err := mgr.Acquire(ctx, "vu_1")
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}

	if session2.ID == originalID {
		t.Error("Session should have been evicted due to idle timeout")
	}
}

func TestSessionInvalidation(t *testing.T) {
	adapter := &mockAdapter{}
	config := &SessionConfig{
		Mode:            ModeReuse,
		Adapter:         adapter,
		TransportConfig: &transport.TransportConfig{Endpoint: "http://localhost:8080"},
	}

	mgr, err := NewManager(config)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	ctx := context.Background()
	mgr.Start(ctx)
	defer mgr.Close(ctx)

	session1, err := mgr.Acquire(ctx, "vu_1")
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	originalID := session1.ID

	err = mgr.Invalidate(ctx, session1)
	if err != nil {
		t.Fatalf("Invalidate() error = %v", err)
	}

	session2, err := mgr.Acquire(ctx, "vu_1")
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}

	if session2.ID == originalID {
		t.Error("Should create new session after invalidation")
	}
}

func TestReuseModeSessionToVUIndex(t *testing.T) {
	// Test that the sessionToVU reverse index is maintained correctly
	adapter := &mockAdapter{}
	config := &SessionConfig{
		Mode:            ModeReuse,
		TTLMs:           60000,
		MaxIdleMs:       30000,
		Adapter:         adapter,
		TransportConfig: &transport.TransportConfig{Endpoint: "http://localhost:8080"},
	}

	mgr, err := NewManager(config)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	ctx := context.Background()
	mgr.Start(ctx)
	defer mgr.Close(ctx)

	// Create sessions for multiple VUs
	session1, err := mgr.Acquire(ctx, "vu_1")
	if err != nil {
		t.Fatalf("Acquire(vu_1) error = %v", err)
	}
	session2, err := mgr.Acquire(ctx, "vu_2")
	if err != nil {
		t.Fatalf("Acquire(vu_2) error = %v", err)
	}
	session3, err := mgr.Acquire(ctx, "vu_3")
	if err != nil {
		t.Fatalf("Acquire(vu_3) error = %v", err)
	}

	// Invalidate session2 - should use O(1) lookup via sessionToVU
	err = mgr.Invalidate(ctx, session2)
	if err != nil {
		t.Fatalf("Invalidate() error = %v", err)
	}

	// Verify vu_2 gets a new session (old one was invalidated)
	newSession2, err := mgr.Acquire(ctx, "vu_2")
	if err != nil {
		t.Fatalf("Acquire(vu_2) after invalidation error = %v", err)
	}
	if newSession2.ID == session2.ID {
		t.Error("vu_2 should get new session after invalidation")
	}

	// Verify vu_1 and vu_3 still have their original sessions
	reacquired1, err := mgr.Acquire(ctx, "vu_1")
	if err != nil {
		t.Fatalf("Acquire(vu_1) error = %v", err)
	}
	if reacquired1.ID != session1.ID {
		t.Error("vu_1 should still have original session")
	}

	reacquired3, err := mgr.Acquire(ctx, "vu_3")
	if err != nil {
		t.Fatalf("Acquire(vu_3) error = %v", err)
	}
	if reacquired3.ID != session3.ID {
		t.Error("vu_3 should still have original session")
	}
}

func TestReuseModeMultipleInvalidations(t *testing.T) {
	// Test that multiple invalidations don't cause issues
	adapter := &mockAdapter{}
	config := &SessionConfig{
		Mode:            ModeReuse,
		TTLMs:           60000,
		MaxIdleMs:       30000,
		Adapter:         adapter,
		TransportConfig: &transport.TransportConfig{Endpoint: "http://localhost:8080"},
	}

	mgr, err := NewManager(config)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	ctx := context.Background()
	mgr.Start(ctx)
	defer mgr.Close(ctx)

	session, err := mgr.Acquire(ctx, "vu_1")
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}

	// First invalidation should succeed
	err = mgr.Invalidate(ctx, session)
	if err != nil {
		t.Fatalf("First Invalidate() error = %v", err)
	}

	// Second invalidation of same session should be safe (no-op)
	err = mgr.Invalidate(ctx, session)
	if err != nil {
		t.Fatalf("Second Invalidate() error = %v", err)
	}

	// Should be able to acquire a new session
	newSession, err := mgr.Acquire(ctx, "vu_1")
	if err != nil {
		t.Fatalf("Acquire() after double invalidation error = %v", err)
	}
	if newSession.ID == session.ID {
		t.Error("Should get new session after invalidation")
	}
}

func TestConcurrentAcquireRelease(t *testing.T) {
	adapter := &mockAdapter{}
	config := &SessionConfig{
		Mode:            ModePool,
		PoolSize:        10,
		TTLMs:           60000,
		MaxIdleMs:       30000,
		Adapter:         adapter,
		TransportConfig: &transport.TransportConfig{Endpoint: "http://localhost:8080"},
	}

	mgr, err := NewManager(config)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	ctx := context.Background()
	mgr.Start(ctx)
	defer mgr.Close(ctx)

	var wg sync.WaitGroup
	numGoroutines := 50
	numOps := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(vuID string) {
			defer wg.Done()
			for j := 0; j < numOps; j++ {
				session, err := mgr.Acquire(ctx, vuID)
				if err != nil {
					t.Errorf("Acquire() error = %v", err)
					return
				}
				time.Sleep(time.Microsecond)
				mgr.Release(ctx, session)
			}
		}("vu_" + string(rune('0'+i%10)))
	}

	wg.Wait()

	metrics := mgr.Metrics()
	if metrics.TotalCreated > int64(config.PoolSize) {
		t.Logf("Pool created %d sessions (pool size: %d)", metrics.TotalCreated, config.PoolSize)
	}
}

func TestReuseModeConcurrentAcquireSameVU(t *testing.T) {
	adapter := &mockAdapter{connectDelay: 50 * time.Millisecond}
	config := &SessionConfig{
		Mode:            ModeReuse,
		TTLMs:           60000,
		MaxIdleMs:       30000,
		Adapter:         adapter,
		TransportConfig: &transport.TransportConfig{Endpoint: "http://localhost:8080"},
	}

	mgr, err := NewManager(config)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	ctx := context.Background()
	mgr.Start(ctx)
	defer mgr.Close(ctx)

	start := make(chan struct{})
	results := make(chan *SessionInfo, 2)
	errs := make(chan error, 2)

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			session, err := mgr.Acquire(ctx, "vu_same")
			if err != nil {
				errs <- err
				return
			}
			results <- session
		}()
	}

	close(start)
	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		t.Fatalf("Acquire() error = %v", err)
	}

	var got []*SessionInfo
	for session := range results {
		got = append(got, session)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(got))
	}

	if got[0].ID != got[1].ID {
		t.Fatalf("expected same session for same vuID, got %s and %s", got[0].ID, got[1].ID)
	}
	metrics := mgr.Metrics()
	if metrics.TotalCreated != 1 {
		t.Fatalf("expected one tracked session creation, got %d", metrics.TotalCreated)
	}
}

func TestMetrics(t *testing.T) {
	adapter := &mockAdapter{}
	config := &SessionConfig{
		Mode:            ModeReuse,
		TTLMs:           60000,
		MaxIdleMs:       30000,
		Adapter:         adapter,
		TransportConfig: &transport.TransportConfig{Endpoint: "http://localhost:8080"},
	}

	mgr, err := NewManager(config)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	ctx := context.Background()
	mgr.Start(ctx)
	defer mgr.Close(ctx)

	metrics := mgr.Metrics()
	if metrics.TotalCreated != 0 {
		t.Errorf("Initial TotalCreated should be 0, got %d", metrics.TotalCreated)
	}

	session, _ := mgr.Acquire(ctx, "vu_1")
	metrics = mgr.Metrics()
	if metrics.TotalCreated != 1 {
		t.Errorf("TotalCreated should be 1, got %d", metrics.TotalCreated)
	}
	if metrics.ActiveSessions != 1 {
		t.Errorf("ActiveSessions should be 1, got %d", metrics.ActiveSessions)
	}

	mgr.Release(ctx, session)
	metrics = mgr.Metrics()
	if metrics.IdleSessions != 1 {
		t.Errorf("IdleSessions should be 1, got %d", metrics.IdleSessions)
	}
}

func TestManagerClose(t *testing.T) {
	adapter := &mockAdapter{}
	config := &SessionConfig{
		Mode:            ModeReuse,
		Adapter:         adapter,
		TransportConfig: &transport.TransportConfig{Endpoint: "http://localhost:8080"},
	}

	mgr, err := NewManager(config)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	ctx := context.Background()
	mgr.Start(ctx)

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

func TestSessionInfoExpiration(t *testing.T) {
	conn := newMockConnection()
	session := NewSessionInfo("test_session", conn, 100, 50)

	if session.IsExpired() {
		t.Error("New session should not be expired")
	}

	time.Sleep(60 * time.Millisecond)
	session.SetState(StateIdle)

	if !session.IsExpired() {
		t.Error("Session should be expired due to idle timeout")
	}

	session2 := NewSessionInfo("test_session_2", conn, 100, 0)
	time.Sleep(150 * time.Millisecond)

	if !session2.IsExpired() {
		t.Error("Session should be expired due to TTL")
	}
}

func TestSessionInfoTouch(t *testing.T) {
	conn := newMockConnection()
	session := NewSessionInfo("test_session", conn, 0, 100)

	initialIdleExpires := session.IdleExpiresAt

	time.Sleep(50 * time.Millisecond)
	session.Touch(100)

	if !session.IdleExpiresAt.After(initialIdleExpires) {
		t.Error("Touch() should extend idle expiration")
	}

	if session.OperationCount != 1 {
		t.Errorf("OperationCount should be 1, got %d", session.OperationCount)
	}
}

func TestEvictorBasic(t *testing.T) {
	evicted := make(chan *SessionInfo, 10)
	callback := func(session *SessionInfo, reason EvictionReason) {
		evicted <- session
	}

	evictor := NewEvictor(100, 0, callback)
	ctx := context.Background()
	evictor.Start(ctx)
	defer evictor.Stop()

	conn := newMockConnection()
	session := NewSessionInfo("test_session", conn, 100, 0)
	session.SetState(StateIdle)
	evictor.Track(session)

	select {
	case s := <-evicted:
		if s.ID != session.ID {
			t.Errorf("Wrong session evicted: %s", s.ID)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("Session should have been evicted due to TTL")
	}

	if evictor.TTLEvictions() != 1 {
		t.Errorf("TTLEvictions should be 1, got %d", evictor.TTLEvictions())
	}
}

func TestPoolBasic(t *testing.T) {
	pool := NewSessionPool(5, 60000, 30000)
	ctx := context.Background()
	pool.Start(ctx)
	defer pool.Close()

	conn := newMockConnection()
	session := NewSessionInfo("test_session", conn, 60000, 30000)
	pool.Add(session)

	if pool.Size() != 1 {
		t.Errorf("Pool size should be 1, got %d", pool.Size())
	}

	if pool.InUse() != 1 {
		t.Errorf("InUse should be 1, got %d", pool.InUse())
	}

	pool.Release(session)

	if pool.Available() != 1 {
		t.Errorf("Available should be 1 after release, got %d", pool.Available())
	}

	acquired, needsCreate, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if needsCreate {
		t.Error("Should not need to create, session was released to pool")
	}
	if acquired.ID != session.ID {
		t.Error("Should acquire the released session")
	}

	pool.Release(acquired)

	if pool.Available() != 1 {
		t.Errorf("Available should be 1, got %d", pool.Available())
	}
}

func TestSessionPoolAcquireHonorsContextCancelWhileWaiting(t *testing.T) {
	pool := NewSessionPool(1, 60000, 30000)
	ctx := context.Background()
	pool.Start(ctx)
	defer pool.Close()

	conn := newMockConnection()
	session := NewSessionInfo("held_session", conn, 60000, 30000)
	pool.Add(session)

	waitCtx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, _, err := pool.Acquire(waitCtx)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected acquire timeout error")
	}
	if elapsed >= 250*time.Millisecond {
		t.Fatalf("acquire should return promptly on context cancellation, elapsed=%v", elapsed)
	}
}

func TestSessionTimerTTL(t *testing.T) {
	evicted := make(chan EvictionReason, 1)
	callback := func(session *SessionInfo, reason EvictionReason) {
		evicted <- reason
	}

	conn := newMockConnection()
	session := NewSessionInfo("test_session", conn, 100, 0)
	session.SetState(StateIdle)
	timer := NewSessionTimer(session, 100, 0, callback)
	defer timer.Stop()

	select {
	case reason := <-evicted:
		if reason != EvictionTTL {
			t.Errorf("Expected TTL eviction, got %s", reason)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("Session should have been evicted due to TTL")
	}
}

func TestSessionTimerIdle(t *testing.T) {
	evicted := make(chan EvictionReason, 1)
	callback := func(session *SessionInfo, reason EvictionReason) {
		evicted <- reason
	}

	conn := newMockConnection()
	session := NewSessionInfo("test_session", conn, 0, 100)
	session.SetState(StateIdle)
	timer := NewSessionTimer(session, 0, 100, callback)
	defer timer.Stop()

	select {
	case reason := <-evicted:
		if reason != EvictionIdle {
			t.Errorf("Expected Idle eviction, got %s", reason)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("Session should have been evicted due to idle timeout")
	}
}

func TestSessionTimerTouch(t *testing.T) {
	evicted := make(chan EvictionReason, 1)
	callback := func(session *SessionInfo, reason EvictionReason) {
		evicted <- reason
	}

	conn := newMockConnection()
	session := NewSessionInfo("test_session", conn, 0, 100)
	session.SetState(StateIdle)
	timer := NewSessionTimer(session, 0, 100, callback)
	defer timer.Stop()

	time.Sleep(50 * time.Millisecond)
	timer.Touch(100)

	select {
	case <-evicted:
		t.Error("Session should not be evicted after Touch()")
	case <-time.After(80 * time.Millisecond):
	}

	select {
	case reason := <-evicted:
		if reason != EvictionIdle {
			t.Errorf("Expected Idle eviction, got %s", reason)
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("Session should have been evicted after idle timeout")
	}
}

func TestSessionTimerIdleDoesNotDisableTTL(t *testing.T) {
	evicted := make(chan EvictionReason, 1)
	callback := func(session *SessionInfo, reason EvictionReason) {
		evicted <- reason
	}

	conn := newMockConnection()
	session := NewSessionInfo("test_session", conn, 200, 50)
	// Session stays active (reuse mode behavior) — idle timer fires but must not disable TTL.
	timer := NewSessionTimer(session, 200, 50, callback)
	defer timer.Stop()

	// Wait for idle timer to fire (50ms) while session is active — it should be a no-op.
	time.Sleep(100 * time.Millisecond)

	// Now set idle so TTL can proceed.
	session.SetState(StateIdle)

	select {
	case reason := <-evicted:
		if reason != EvictionTTL {
			t.Errorf("Expected TTL eviction, got %s", reason)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("TTL eviction should still fire after idle timer was skipped on active session")
	}
}

func TestModeReturnsCorrectValue(t *testing.T) {
	adapter := &mockAdapter{}
	transportConfig := &transport.TransportConfig{Endpoint: "http://localhost:8080"}

	modes := []SessionMode{ModeReuse, ModePerRequest, ModePool, ModeChurn}

	for _, mode := range modes {
		config := &SessionConfig{
			Mode:            mode,
			PoolSize:        10,
			Adapter:         adapter,
			TransportConfig: transportConfig,
		}

		mgr, err := NewManager(config)
		if err != nil {
			t.Fatalf("NewManager() error = %v", err)
		}

		if mgr.Mode() != mode {
			t.Errorf("Mode() = %s, want %s", mgr.Mode(), mode)
		}
	}
}
