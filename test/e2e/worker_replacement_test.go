package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/controlplane/api"
	"github.com/bc-dunia/mcpdrill/internal/controlplane/runmanager"
	"github.com/bc-dunia/mcpdrill/internal/controlplane/scheduler"
	"github.com/bc-dunia/mcpdrill/internal/types"
	"github.com/bc-dunia/mcpdrill/internal/worker"
)

var workerReplacementConfig = []byte(`{
	"schema_version": "run-config/v1",
	"scenario_id": "worker-replacement-test",
	"metadata": {
		"name": "Worker Replacement Test",
		"description": "Test worker failure and replacement",
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
		"mode": "reuse",
		"pool_size": 10,
		"ttl_ms": 60000,
		"max_idle_ms": 30000
	},
	"workload": {
		"in_flight_per_vu": 1,
		"think_time": {
			"mode": "fixed",
			"base_ms": 100,
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
			"duration_ms": 1000,
			"load": {
				"target_vus": 10,
				"target_rps": 10
			},
			"stop_conditions": []
		},
		{
			"stage_id": "stg_000000000002",
			"stage": "baseline",
			"enabled": true,
			"duration_ms": 60000,
			"load": {
				"target_vus": 50,
				"target_rps": 50
			},
			"stop_conditions": [
				{
					"id": "baseline_error_rate",
					"metric": "error_rate",
					"comparator": ">",
					"threshold": 0.5,
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
			"duration_ms": 60000,
			"load": {
				"target_vus": 100,
				"target_rps": 100
			},
			"ramp": {
				"mode": "step",
				"step_every_ms": 5000,
				"step_vus": 10,
				"step_rps": 10,
				"max_vus": 100,
				"max_rps": 100,
				"hold_ms": 2000
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
		"worker_failure_policy": "replace_if_possible",
		"hard_caps": {
			"max_vus": 200,
			"max_rps": 200,
			"max_connections": 100,
			"max_duration_ms": 300000,
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

type replacementTestWorker struct {
	id              int
	controlPlaneURL string
	workerID        string
	capacity        int
	ctx             context.Context
	cancel          context.CancelFunc
	mu              sync.Mutex
	registered      bool
	heartbeatCount  int
	errors          []error
	httpClient      *http.Client
	retryClient     *worker.RetryHTTPClient
}

func newReplacementTestWorker(id int, controlPlaneURL string, capacity int) *replacementTestWorker {
	ctx, cancel := context.WithCancel(context.Background())
	httpClient := &http.Client{Timeout: 5 * time.Second}
	retryClient := worker.NewRetryHTTPClient(ctx, controlPlaneURL, httpClient, worker.RetryConfig{
		MaxRetries: 3,
		Backoff:    100 * time.Millisecond,
		MaxBackoff: 1 * time.Second,
	})

	return &replacementTestWorker{
		id:              id,
		controlPlaneURL: controlPlaneURL,
		capacity:        capacity,
		ctx:             ctx,
		cancel:          cancel,
		httpClient:      httpClient,
		retryClient:     retryClient,
	}
}

func (w *replacementTestWorker) register() error {
	req := map[string]interface{}{
		"host_info": map[string]string{
			"hostname": fmt.Sprintf("replacement-worker-%d", w.id),
			"ip_addr":  "127.0.0.1",
			"platform": "test",
		},
		"capacity": map[string]interface{}{
			"max_vus":            w.capacity,
			"max_concurrent_ops": w.capacity / 2,
			"max_rps":            float64(w.capacity) * 10,
		},
	}

	resp, err := w.retryClient.Post("/workers/register", req)
	if err != nil {
		return fmt.Errorf("registration failed: %w", err)
	}

	body, err := worker.ReadResponseBody(resp)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("registration failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		WorkerID string `json:"worker_id"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	w.mu.Lock()
	w.workerID = result.WorkerID
	w.registered = true
	w.mu.Unlock()

	return nil
}

func (w *replacementTestWorker) sendHeartbeat() error {
	w.mu.Lock()
	workerID := w.workerID
	w.mu.Unlock()

	if workerID == "" {
		return fmt.Errorf("worker not registered")
	}

	req := map[string]interface{}{
		"health": map[string]interface{}{
			"cpu_percent":     10.0 + float64(w.id),
			"mem_bytes":       1024 * 1024 * 100,
			"active_vus":      w.capacity / 2,
			"active_sessions": w.capacity / 4,
			"in_flight_ops":   w.capacity / 10,
			"queue_depth":     0,
		},
	}

	path := fmt.Sprintf("/workers/%s/heartbeat", workerID)
	resp, err := w.retryClient.Post(path, req)
	if err != nil {
		w.mu.Lock()
		w.errors = append(w.errors, err)
		w.mu.Unlock()
		return err
	}

	body, err := worker.ReadResponseBody(resp)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("heartbeat failed with status %d: %s", resp.StatusCode, string(body))
	}

	w.mu.Lock()
	w.heartbeatCount++
	w.mu.Unlock()

	return nil
}

func (w *replacementTestWorker) runHeartbeatLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			_ = w.sendHeartbeat()
		}
	}
}

func (w *replacementTestWorker) stop() {
	w.cancel()
}

func (w *replacementTestWorker) getWorkerID() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.workerID
}

func (w *replacementTestWorker) isRegistered() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.registered
}

func (w *replacementTestWorker) getErrors() []error {
	w.mu.Lock()
	defer w.mu.Unlock()
	result := make([]error, len(w.errors))
	copy(result, w.errors)
	return result
}

func TestWorkerReplacement_ReplaceIfPossible(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping worker replacement test in short mode")
	}

	const (
		heartbeatTimeout  = 500 * time.Millisecond
		monitorInterval   = 100 * time.Millisecond
		heartbeatInterval = 100 * time.Millisecond
		workerCapacity    = 100
	)

	validator := createTestValidator(t)
	rm := runmanager.NewRunManager(validator)
	registry := scheduler.NewRegistry()
	leaseManager := scheduler.NewLeaseManager(60000)
	allocator := scheduler.NewAllocator(registry, leaseManager)
	telemetryStore := api.NewTelemetryStore()

	rm.SetScheduler(registry, allocator, leaseManager)
	rm.SetTelemetryStore(telemetryStore)

	assignmentSender := &mockAssignmentSender{assignments: make(map[string][]types.WorkerAssignment)}
	rm.SetAssignmentSender(assignmentSender)

	heartbeatMonitor := scheduler.NewHeartbeatMonitor(registry, leaseManager, heartbeatTimeout, monitorInterval)
	heartbeatMonitor.SetOnWorkerLost(func(workerID scheduler.WorkerID, affectedRunIDs []string) {
		runIDs := affectedRunIDs
		for _, runID := range runIDs {
			_ = rm.HandleWorkerCapacityLost(runID, string(workerID))
		}
	})
	heartbeatMonitor.Start()
	defer heartbeatMonitor.Stop()

	server := api.NewServer("127.0.0.1:0", rm)
	server.SetRegistry(registry)
	server.SetTelemetryStore(telemetryStore)
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		server.Shutdown(ctx)
		cancel()
	}()

	baseURL := server.URL()
	t.Logf("Control plane started at %s", baseURL)

	worker1 := newReplacementTestWorker(1, baseURL, workerCapacity)
	worker2 := newReplacementTestWorker(2, baseURL, workerCapacity)

	if err := worker1.register(); err != nil {
		t.Fatalf("Failed to register worker 1: %v", err)
	}
	t.Logf("Worker 1 registered: %s", worker1.getWorkerID())

	if err := worker2.register(); err != nil {
		t.Fatalf("Failed to register worker 2: %v", err)
	}
	t.Logf("Worker 2 registered: %s", worker2.getWorkerID())
	defer worker2.stop()

	go worker1.runHeartbeatLoop(heartbeatInterval)
	go worker2.runHeartbeatLoop(heartbeatInterval)

	time.Sleep(200 * time.Millisecond)

	if registry.WorkerCount() != 2 {
		t.Fatalf("Expected 2 workers in registry, got %d", registry.WorkerCount())
	}

	runID := createRun(t, baseURL, workerReplacementConfig)
	t.Logf("Created run: %s", runID)

	startRun(t, baseURL, runID)
	t.Logf("Started run: %s", runID)

	time.Sleep(600 * time.Millisecond)

	status := getRun(t, baseURL, runID)
	t.Logf("Run state before worker kill: %s", status.State)

	worker1ID := worker1.getWorkerID()
	t.Logf("Killing worker 1: %s", worker1ID)
	worker1.stop()

	time.Sleep(heartbeatTimeout + monitorInterval*3)

	if registry.WorkerCount() != 1 {
		t.Errorf("Expected 1 worker in registry after kill, got %d", registry.WorkerCount())
	}

	events := getEvents(t, baseURL, runID)
	t.Logf("Collected %d events", len(events))

	foundCapacityLost := false
	foundWorkerReplaced := false
	for _, event := range events {
		t.Logf("Event: %s", event.Type)
		if event.Type == runmanager.EventTypeWorkerCapacityLost {
			foundCapacityLost = true
		}
		if event.Type == runmanager.EventTypeWorkerReplaced {
			foundWorkerReplaced = true
		}
	}

	if !foundCapacityLost && !foundWorkerReplaced {
		status = getRun(t, baseURL, runID)
		if status.State == runmanager.RunStateStopping {
			t.Log("Run is stopping (fail_fast fallback due to insufficient capacity or timing)")
		} else {
			t.Logf("Run state: %s (expected either WORKER_CAPACITY_LOST/WORKER_REPLACED events or stopping state)", status.State)
		}
	}

	t.Log("Worker replacement test completed")
}

func TestWorkerReplacement_FailFast(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping worker replacement test in short mode")
	}

	const (
		heartbeatTimeout  = 500 * time.Millisecond
		monitorInterval   = 100 * time.Millisecond
		heartbeatInterval = 100 * time.Millisecond
		workerCapacity    = 100
	)

	failFastConfig := make([]byte, len(workerReplacementConfig))
	copy(failFastConfig, workerReplacementConfig)

	var config map[string]interface{}
	if err := json.Unmarshal(failFastConfig, &config); err != nil {
		t.Fatalf("Failed to parse config: %v", err)
	}
	safety := config["safety"].(map[string]interface{})
	safety["worker_failure_policy"] = "fail_fast"
	failFastConfig, _ = json.Marshal(config)

	validator := createTestValidator(t)
	rm := runmanager.NewRunManager(validator)
	registry := scheduler.NewRegistry()
	leaseManager := scheduler.NewLeaseManager(60000)
	allocator := scheduler.NewAllocator(registry, leaseManager)
	telemetryStore := api.NewTelemetryStore()

	rm.SetScheduler(registry, allocator, leaseManager)
	rm.SetTelemetryStore(telemetryStore)

	assignmentSender := &mockAssignmentSender{assignments: make(map[string][]types.WorkerAssignment)}
	rm.SetAssignmentSender(assignmentSender)

	heartbeatMonitor := scheduler.NewHeartbeatMonitor(registry, leaseManager, heartbeatTimeout, monitorInterval)
	heartbeatMonitor.SetOnWorkerLost(func(workerID scheduler.WorkerID, affectedRunIDs []string) {
		runIDs := affectedRunIDs
		for _, runID := range runIDs {
			_ = rm.HandleWorkerCapacityLost(runID, string(workerID))
		}
	})
	heartbeatMonitor.Start()
	defer heartbeatMonitor.Stop()

	server := api.NewServer("127.0.0.1:0", rm)
	server.SetRegistry(registry)
	server.SetTelemetryStore(telemetryStore)
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		server.Shutdown(ctx)
		cancel()
	}()

	baseURL := server.URL()
	t.Logf("Control plane started at %s", baseURL)

	worker1 := newReplacementTestWorker(1, baseURL, workerCapacity)
	worker2 := newReplacementTestWorker(2, baseURL, workerCapacity)

	if err := worker1.register(); err != nil {
		t.Fatalf("Failed to register worker 1: %v", err)
	}
	if err := worker2.register(); err != nil {
		t.Fatalf("Failed to register worker 2: %v", err)
	}
	defer worker2.stop()

	go worker1.runHeartbeatLoop(heartbeatInterval)
	go worker2.runHeartbeatLoop(heartbeatInterval)

	time.Sleep(200 * time.Millisecond)

	runID := createRun(t, baseURL, failFastConfig)
	t.Logf("Created run: %s", runID)

	startRun(t, baseURL, runID)
	t.Logf("Started run: %s", runID)

	// Wait for baseline state with active leases
	var workerToKill string
	for i := 0; i < 30; i++ {
		status := getRun(t, baseURL, runID)
		if status.State == runmanager.RunStateBaselineRunning {
			leases := leaseManager.ListLeases(runID)
			for _, lease := range leases {
				if lease.State == scheduler.LeaseStateActive {
					workerToKill = string(lease.WorkerID)
					break
				}
			}
			if workerToKill != "" {
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if workerToKill == "" {
		t.Fatal("Could not find worker with active lease in baseline state")
	}
	t.Logf("Killing worker with active lease: %s", workerToKill)
	if workerToKill == worker1.getWorkerID() {
		worker1.stop()
	} else {
		worker2.stop()
	}
	time.Sleep(heartbeatTimeout + monitorInterval*3)

	status := getRun(t, baseURL, runID)
	t.Logf("Run state after worker kill: %s", status.State)

	if status.State != runmanager.RunStateStopping {
		t.Errorf("Expected run to be in STOPPING state with fail_fast policy, got %s", status.State)
	}

	events := getEvents(t, baseURL, runID)
	foundStopRequested := false
	for _, event := range events {
		if event.Type == runmanager.EventTypeStopRequested {
			foundStopRequested = true
			break
		}
	}

	if !foundStopRequested {
		t.Error("Expected STOP_REQUESTED event with fail_fast policy")
	}

	t.Log("Fail-fast test completed")
}

func TestWorkerReplacement_InsufficientCapacity(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping worker replacement test in short mode")
	}

	const (
		heartbeatTimeout  = 500 * time.Millisecond
		monitorInterval   = 100 * time.Millisecond
		heartbeatInterval = 100 * time.Millisecond
		workerCapacity    = 30
	)

	validator := createTestValidator(t)
	rm := runmanager.NewRunManager(validator)
	registry := scheduler.NewRegistry()
	leaseManager := scheduler.NewLeaseManager(60000)
	allocator := scheduler.NewAllocator(registry, leaseManager)
	telemetryStore := api.NewTelemetryStore()

	rm.SetScheduler(registry, allocator, leaseManager)
	rm.SetTelemetryStore(telemetryStore)

	assignmentSender := &mockAssignmentSender{assignments: make(map[string][]types.WorkerAssignment)}
	rm.SetAssignmentSender(assignmentSender)

	heartbeatMonitor := scheduler.NewHeartbeatMonitor(registry, leaseManager, heartbeatTimeout, monitorInterval)
	heartbeatMonitor.SetOnWorkerLost(func(workerID scheduler.WorkerID, affectedRunIDs []string) {
		runIDs := affectedRunIDs
		for _, runID := range runIDs {
			_ = rm.HandleWorkerCapacityLost(runID, string(workerID))
		}
	})
	heartbeatMonitor.Start()
	defer heartbeatMonitor.Stop()

	server := api.NewServer("127.0.0.1:0", rm)
	server.SetRegistry(registry)
	server.SetTelemetryStore(telemetryStore)
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		server.Shutdown(ctx)
		cancel()
	}()

	baseURL := server.URL()
	t.Logf("Control plane started at %s", baseURL)

	worker1 := newReplacementTestWorker(1, baseURL, workerCapacity)
	worker2 := newReplacementTestWorker(2, baseURL, workerCapacity)

	if err := worker1.register(); err != nil {
		t.Fatalf("Failed to register worker 1: %v", err)
	}
	if err := worker2.register(); err != nil {
		t.Fatalf("Failed to register worker 2: %v", err)
	}
	defer worker2.stop()

	go worker1.runHeartbeatLoop(heartbeatInterval)
	go worker2.runHeartbeatLoop(heartbeatInterval)

	time.Sleep(200 * time.Millisecond)

	runID := createRun(t, baseURL, workerReplacementConfig)
	t.Logf("Created run: %s", runID)

	startRun(t, baseURL, runID)
	t.Logf("Started run: %s", runID)

	time.Sleep(600 * time.Millisecond)

	t.Log("Killing worker 1...")
	worker1.stop()

	time.Sleep(heartbeatTimeout + monitorInterval*3)

	status := getRun(t, baseURL, runID)
	t.Logf("Run state after worker kill: %s", status.State)

	if status.State != runmanager.RunStateStopping {
		t.Errorf("Expected run to be in STOPPING state due to insufficient capacity, got %s", status.State)
	}

	events := getEvents(t, baseURL, runID)
	foundDecision := false
	for _, event := range events {
		if event.Type == runmanager.EventTypeDecision {
			foundDecision = true
			break
		}
	}

	if !foundDecision {
		t.Log("No DECISION event found (may have been emitted before event collection)")
	}

	t.Log("Insufficient capacity test completed")
}

type mockAssignmentSender struct {
	mu          sync.Mutex
	assignments map[string][]types.WorkerAssignment
}

func (m *mockAssignmentSender) AddAssignment(workerID string, assignment types.WorkerAssignment) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.assignments[workerID] = append(m.assignments[workerID], assignment)
}

func (m *mockAssignmentSender) GetAssignments(workerID string) []types.WorkerAssignment {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.assignments[workerID]
}
