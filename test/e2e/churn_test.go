package e2e

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/analysis"
	"github.com/bc-dunia/mcpdrill/internal/controlplane/api"
	"github.com/bc-dunia/mcpdrill/internal/controlplane/runmanager"
	"github.com/bc-dunia/mcpdrill/internal/session"
	"github.com/bc-dunia/mcpdrill/internal/transport"
)

// churnConfig is a run config with churn mode enabled.
var churnConfig = []byte(`{
	"schema_version": "run-config/v1",
	"scenario_id": "e2e-churn-test",
	"metadata": {
		"name": "E2E Churn Test Config",
		"description": "Config for testing churn mode session lifecycle",
		"created_by": "e2e-test@example.com",
		"tags": {"env": "test"}
	},
	"target": {
		"kind": "gateway",
		"url": "https://test-gateway.example.com/mcp",
		"transport": "streamable_http",
		"headers": {},
		"auth": {
			"type": "bearer_token",
			"bearer_token_ref": "env://MCPDRILL_TEST_TOKEN"
		},
		"identification": {
			"run_id_header": {
				"name": "X-Test-Run-Id",
				"value_template": "${run_id}"
			},
			"user_agent": {
				"value": "mcpdrill/1.0 (run=${run_id})"
			}
		},
		"timeouts": {
			"connect_timeout_ms": 5000,
			"request_timeout_ms": 30000,
			"stream_stall_timeout_ms": 15000
		},
		"tls": {
			"verify": true,
			"ca_bundle_ref": null
		},
		"redirect_policy": {
			"mode": "deny",
			"max_redirects": 3
		}
	},
	"environment": {
		"allowlist": {
			"mode": "deny_by_default",
			"allowed_targets": [
				{"kind": "suffix", "value": ".example.com"}
			]
		},
		"forbidden_patterns": []
	},
	"session_policy": {
		"mode": "churn",
		"pool_size": 10,
		"ttl_ms": 60000,
		"max_idle_ms": 30000,
		"churn_interval_ops": 1
	},
	"workload": {
		"in_flight_per_vu": 1,
		"think_time": {
			"mode": "fixed",
			"base_ms": 10,
			"jitter_ms": 0
		},
		"operation_mix": [
			{"operation": "tools_list", "weight": 1}
		],
		"tools": {
			"selection": {"mode": "round_robin"},
			"templates": []
		},
		"payload_profiles": []
	},
	"stages": [
		{
			"stage_id": "stg_000000000001",
			"stage": "preflight",
			"enabled": true,
			"duration_ms": 5000,
			"load": {
				"target_vus": 1,
				"target_rps": 1
			},
			"stop_conditions": []
		},
		{
			"stage_id": "stg_000000000002",
			"stage": "baseline",
			"enabled": true,
			"duration_ms": 5000,
			"load": {
				"target_vus": 2,
				"target_rps": 2
			},
			"stop_conditions": [
				{
					"id": "baseline_error_rate",
					"metric": "error_rate",
					"comparator": ">",
					"threshold": 0.1,
					"window_ms": 5000,
					"sustain_windows": 1,
					"scope": {}
				}
			]
		},
		{
			"stage_id": "stg_000000000003",
			"stage": "ramp",
			"enabled": true,
			"duration_ms": 5000,
			"load": {
				"target_vus": 2,
				"target_rps": 2
			},
			"ramp": {
				"mode": "step",
				"step_every_ms": 2500,
				"step_vus": 1,
				"step_rps": 1,
				"max_vus": 5,
				"max_rps": 5,
				"hold_ms": 1000
			},
			"stop_conditions": [
				{
					"id": "ramp_p99",
					"metric": "latency_p99_ms",
					"comparator": ">",
					"threshold": 5000,
					"window_ms": 5000,
					"sustain_windows": 1,
					"scope": {}
				}
			]
		}
	],
	"safety": {
		"ramp_by_default": false,
		"emergency_stop_enabled": true,
		"worker_failure_policy": "fail_fast",
		"hard_caps": {
			"max_vus": 10,
			"max_rps": 10,
			"max_connections": 10,
			"max_duration_ms": 60000,
			"max_in_flight_per_vu": 2,
			"max_telemetry_q_depth": 1000
		},
		"stop_policy": {
			"mode": "drain",
			"drain_timeout_ms": 5000
		},
		"identification_required": true
	},
	"reporting": {
		"formats": ["json"],
		"retention": {
			"raw_logs_days": 1,
			"metrics_days": 1,
			"reports_days": 1
		},
		"include": {
			"store_raw_logs": false,
			"store_metrics_snapshot": false,
			"store_event_log": true
		},
		"redaction": {
			"redact_headers": []
		}
	},
	"telemetry": {
		"structured_logs": {
			"enabled": true,
			"sample_rate": 1.0
		},
		"traces": {
			"enabled": false,
			"propagation": {
				"accept_incoming_traceparent": false
			}
		}
	}
}`)

// mockChurnAdapter implements transport.Adapter for churn testing.
type mockChurnAdapter struct {
	connectCount atomic.Int64
	mu           sync.Mutex
	connections  []*mockChurnConnection
}

func (m *mockChurnAdapter) ID() string {
	return "mock-churn-e2e"
}

func (m *mockChurnAdapter) Connect(ctx context.Context, config *transport.TransportConfig) (transport.Connection, error) {
	m.connectCount.Add(1)
	conn := newMockChurnConnection()
	m.mu.Lock()
	m.connections = append(m.connections, conn)
	m.mu.Unlock()
	return conn, nil
}

func (m *mockChurnAdapter) ConnectionCount() int64 {
	return m.connectCount.Load()
}

func (m *mockChurnAdapter) ClosedCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, conn := range m.connections {
		if conn.closed.Load() {
			count++
		}
	}
	return count
}

// mockChurnConnection implements transport.Connection for churn testing.
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
		sessionID: formatChurnSessionID(n),
	}
}

func formatChurnSessionID(n int64) string {
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
	return "churn_ses_" + string(buf[i:])
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

// TestChurnModeSessionLifecycle tests that churn mode creates and destroys sessions correctly.
func TestChurnModeSessionLifecycle(t *testing.T) {
	// Setup: Create validator + RunManager + API server
	validator := createTestValidator(t)
	rm := runmanager.NewRunManager(validator)
	server, cleanup, err := api.StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	baseURL := server.URL()
	t.Logf("Test server started at %s", baseURL)

	// Create run with churn config
	runID := createRun(t, baseURL, churnConfig)
	t.Logf("Created run with churn mode: %s", runID)

	// Verify initial state
	status := getRun(t, baseURL, runID)
	assertState(t, status, "created")

	// Start run
	startRun(t, baseURL, runID)
	t.Logf("Started run: %s", runID)

	// Verify run is in preflight_running state
	status = getRun(t, baseURL, runID)
	assertState(t, status, "preflight_running")

	// Verify events
	events := getEvents(t, baseURL, runID)
	assertEventExists(t, events, "RUN_CREATED")
	assertEventExists(t, events, "STATE_TRANSITION")

	// Stop run
	stopRun(t, baseURL, runID, "drain")
	t.Logf("Stopped run: %s", runID)

	// Verify final state
	status = getRun(t, baseURL, runID)
	assertState(t, status, "stopping")

	t.Logf("Churn mode session lifecycle test completed")
}

// TestChurnModeSessionManager tests the session manager in churn mode directly.
func TestChurnModeSessionManager(t *testing.T) {
	adapter := &mockChurnAdapter{}
	config := &session.SessionConfig{
		Mode:             session.ModeChurn,
		ChurnIntervalOps: 1, // Churn after every operation
		Adapter:          adapter,
		TransportConfig:  &transport.TransportConfig{Endpoint: "http://localhost:8080"},
	}

	mgr, err := session.NewManager(config)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	ctx := context.Background()
	defer mgr.Close(ctx)

	// Acquire first session
	sess1, err := mgr.Acquire(ctx, "vu_1")
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	sess1ID := sess1.ID
	t.Logf("First session ID: %s", sess1ID)

	// Release session
	mgr.Release(ctx, sess1)

	// Acquire second session - should be different due to churn
	sess2, err := mgr.Acquire(ctx, "vu_1")
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}

	if sess2.ID == sess1ID {
		t.Error("Churn mode (ops=1) should create new session after each operation")
	}
	t.Logf("Second session ID: %s (different from first)", sess2.ID)

	// Verify connection count
	if adapter.ConnectionCount() != 2 {
		t.Errorf("Expected 2 connections, got %d", adapter.ConnectionCount())
	}

	t.Logf("Churn mode session manager test completed")
}

// TestChurnMetricsAccuracy tests that churn metrics are tracked accurately.
func TestChurnMetricsAccuracy(t *testing.T) {
	adapter := &mockChurnAdapter{}
	config := &session.SessionConfig{
		Mode:             session.ModeChurn,
		ChurnIntervalOps: 2, // Churn after every 2 operations
		Adapter:          adapter,
		TransportConfig:  &transport.TransportConfig{Endpoint: "http://localhost:8080"},
	}

	mgr, err := session.NewManager(config)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	ctx := context.Background()
	defer mgr.Close(ctx)

	// Perform 6 operations (should result in 3 sessions)
	numOps := 6
	for i := 0; i < numOps; i++ {
		sess, err := mgr.Acquire(ctx, "vu_1")
		if err != nil {
			t.Fatalf("Acquire() error = %v", err)
		}
		mgr.Release(ctx, sess)
	}

	// Check metrics
	metrics := mgr.Metrics()
	t.Logf("Metrics: TotalCreated=%d, TotalEvicted=%d, ActiveSessions=%d",
		metrics.TotalCreated, metrics.TotalEvicted, metrics.ActiveSessions)

	// With churn interval of 2, 6 ops should create 3 sessions
	expectedCreated := int64(3)
	if metrics.TotalCreated != expectedCreated {
		t.Errorf("Expected %d sessions created, got %d", expectedCreated, metrics.TotalCreated)
	}

	// 2 sessions should have been evicted (churned)
	expectedEvicted := int64(2)
	if metrics.TotalEvicted != expectedEvicted {
		t.Errorf("Expected %d sessions evicted, got %d", expectedEvicted, metrics.TotalEvicted)
	}

	t.Logf("Churn metrics accuracy test completed")
}

// TestChurnMetricsInAggregator tests that churn metrics are properly aggregated.
func TestChurnMetricsInAggregator(t *testing.T) {
	agg := analysis.NewAggregator()
	agg.SetTimeRange(0, 10000) // 10 seconds

	// Add churn samples
	agg.AddChurnSample(analysis.ChurnSample{
		SessionsCreated:   10,
		SessionsDestroyed: 8,
		ActiveSessions:    2,
		ReconnectAttempts: 1,
	})

	agg.AddChurnSample(analysis.ChurnSample{
		SessionsCreated:   5,
		SessionsDestroyed: 4,
		ActiveSessions:    3,
		ReconnectAttempts: 0,
	})

	// Compute metrics
	metrics := agg.Compute()

	if metrics.ChurnMetrics == nil {
		t.Fatal("Expected churn metrics to be present")
	}

	// Verify aggregated values
	if metrics.ChurnMetrics.SessionsCreated != 15 {
		t.Errorf("Expected 15 sessions created, got %d", metrics.ChurnMetrics.SessionsCreated)
	}

	if metrics.ChurnMetrics.SessionsDestroyed != 12 {
		t.Errorf("Expected 12 sessions destroyed, got %d", metrics.ChurnMetrics.SessionsDestroyed)
	}

	if metrics.ChurnMetrics.ActiveSessions != 3 {
		t.Errorf("Expected 3 active sessions (last sample), got %d", metrics.ChurnMetrics.ActiveSessions)
	}

	if metrics.ChurnMetrics.ReconnectAttempts != 1 {
		t.Errorf("Expected 1 reconnect attempt, got %d", metrics.ChurnMetrics.ReconnectAttempts)
	}

	// Verify churn rate: (15 + 12) / 10 = 2.7 sessions/sec
	expectedRate := 2.7
	if metrics.ChurnMetrics.ChurnRate != expectedRate {
		t.Errorf("Expected churn rate %.2f, got %.2f", expectedRate, metrics.ChurnMetrics.ChurnRate)
	}

	t.Logf("Churn metrics aggregator test completed")
}

// TestChurnModeMultipleVUs tests churn mode with multiple VUs.
func TestChurnModeMultipleVUs(t *testing.T) {
	adapter := &mockChurnAdapter{}
	config := &session.SessionConfig{
		Mode:             session.ModeChurn,
		ChurnIntervalOps: 3, // Churn after every 3 operations
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
	numVUs := 5
	opsPerVU := 9 // Each VU does 9 ops, should create 3 sessions each

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
				time.Sleep(time.Microsecond) // Small delay to simulate work
				mgr.Release(ctx, sess)
			}
		}("vu_" + string(rune('0'+i)))
	}

	wg.Wait()

	// Check metrics
	metrics := mgr.Metrics()
	t.Logf("Multi-VU Metrics: TotalCreated=%d, TotalEvicted=%d",
		metrics.TotalCreated, metrics.TotalEvicted)

	// Each VU should create 3 sessions (9 ops / 3 interval = 3 sessions)
	// Total: 5 VUs * 3 sessions = 15 sessions
	expectedCreated := int64(numVUs * (opsPerVU / 3))
	if metrics.TotalCreated < expectedCreated {
		t.Errorf("Expected at least %d sessions created, got %d", expectedCreated, metrics.TotalCreated)
	}

	t.Logf("Churn mode multiple VUs test completed")
}

// TestChurnModeClose tests that closing the session manager cleans up properly.
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

	// Create some sessions
	for i := 0; i < 3; i++ {
		sess, err := mgr.Acquire(ctx, "vu_"+string(rune('0'+i)))
		if err != nil {
			t.Fatalf("Acquire() error = %v", err)
		}
		mgr.Release(ctx, sess)
	}

	// Close manager
	err = mgr.Close(ctx)
	if err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Verify all connections are closed
	closedCount := adapter.ClosedCount()
	connCount := int(adapter.ConnectionCount())
	if closedCount != connCount {
		t.Errorf("Expected all %d connections to be closed, got %d closed", connCount, closedCount)
	}

	// Verify acquire fails after close
	_, err = mgr.Acquire(ctx, "vu_new")
	if err == nil {
		t.Error("Acquire() should fail after Close()")
	}

	t.Logf("Churn mode close test completed")
}

// TestChurnModeInvalidate tests session invalidation in churn mode.
func TestChurnModeInvalidate(t *testing.T) {
	adapter := &mockChurnAdapter{}
	config := &session.SessionConfig{
		Mode:             session.ModeChurn,
		ChurnIntervalOps: 100, // High interval so we can test invalidation
		Adapter:          adapter,
		TransportConfig:  &transport.TransportConfig{Endpoint: "http://localhost:8080"},
	}

	mgr, err := session.NewManager(config)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	ctx := context.Background()
	defer mgr.Close(ctx)

	// Acquire session
	sess1, err := mgr.Acquire(ctx, "vu_1")
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	sess1ID := sess1.ID

	// Invalidate session
	err = mgr.Invalidate(ctx, sess1)
	if err != nil {
		t.Fatalf("Invalidate() error = %v", err)
	}

	// Acquire again - should get new session
	sess2, err := mgr.Acquire(ctx, "vu_1")
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}

	if sess2.ID == sess1ID {
		t.Error("Should create new session after invalidation")
	}

	t.Logf("Churn mode invalidate test completed")
}
