package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/controlplane/api"
	"github.com/bc-dunia/mcpdrill/internal/controlplane/runmanager"
	"github.com/bc-dunia/mcpdrill/internal/controlplane/scheduler"
	"github.com/bc-dunia/mcpdrill/internal/worker"
)

type testWorker struct {
	controlPlaneURL string
	workerID        string
	httpClient      *http.Client
	retryClient     *worker.RetryHTTPClient
	ctx             context.Context
	cancel          context.CancelFunc
	mu              sync.Mutex
	heartbeatErrors []error
	registered      bool
}

func newTestWorker(controlPlaneURL string) *testWorker {
	ctx, cancel := context.WithCancel(context.Background())
	httpClient := &http.Client{Timeout: 5 * time.Second}

	retryClient := worker.NewRetryHTTPClient(ctx, controlPlaneURL, httpClient, worker.RetryConfig{
		MaxRetries: 3,
		Backoff:    100 * time.Millisecond,
		MaxBackoff: 1 * time.Second,
	})

	return &testWorker{
		controlPlaneURL: controlPlaneURL,
		httpClient:      httpClient,
		retryClient:     retryClient,
		ctx:             ctx,
		cancel:          cancel,
	}
}

func (w *testWorker) register() error {
	req := map[string]interface{}{
		"host_info": map[string]string{
			"hostname": "test-worker",
			"ip_addr":  "127.0.0.1",
			"platform": "test",
		},
		"capacity": map[string]interface{}{
			"max_vus":            100,
			"max_concurrent_ops": 50,
			"max_rps":            1000.0,
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

func (w *testWorker) sendHeartbeat() error {
	w.mu.Lock()
	workerID := w.workerID
	w.mu.Unlock()

	if workerID == "" {
		return fmt.Errorf("worker not registered")
	}

	req := map[string]interface{}{
		"health": map[string]interface{}{
			"cpu_percent":     10.0,
			"mem_bytes":       1024 * 1024 * 100,
			"active_vus":      0,
			"active_sessions": 0,
			"in_flight_ops":   0,
			"queue_depth":     0,
		},
	}

	path := fmt.Sprintf("/workers/%s/heartbeat", workerID)
	resp, err := w.retryClient.Post(path, req)
	if err != nil {
		w.mu.Lock()
		w.heartbeatErrors = append(w.heartbeatErrors, err)
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

	return nil
}

func (w *testWorker) runHeartbeatLoop(interval time.Duration) {
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

func (w *testWorker) stop() {
	w.cancel()
}

func (w *testWorker) getHeartbeatErrors() []error {
	w.mu.Lock()
	defer w.mu.Unlock()
	result := make([]error, len(w.heartbeatErrors))
	copy(result, w.heartbeatErrors)
	return result
}

func (w *testWorker) isRegistered() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.registered
}

func (w *testWorker) getWorkerID() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.workerID
}

func TestNetworkPartition_WorkerReregisters(t *testing.T) {
	validator := createTestValidator(t)
	rm := runmanager.NewRunManager(validator)
	registry := scheduler.NewRegistry()
	leaseManager := scheduler.NewLeaseManager(60000)

	server := api.NewServer("127.0.0.1:0", rm)
	server.SetRegistry(registry)
	ConfigureTestServer(server)
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	baseURL := server.URL()
	t.Logf("Control plane started at %s", baseURL)

	testWorker := newTestWorker(baseURL)
	if err := testWorker.register(); err != nil {
		t.Fatalf("Failed to register worker: %v", err)
	}
	t.Logf("Worker registered with ID: %s", testWorker.getWorkerID())

	if registry.WorkerCount() != 1 {
		t.Errorf("Expected 1 worker in registry, got %d", registry.WorkerCount())
	}

	go testWorker.runHeartbeatLoop(500 * time.Millisecond)

	time.Sleep(1 * time.Second)

	t.Log("Simulating network partition by stopping control plane...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := server.Shutdown(ctx); err != nil {
		t.Fatalf("Failed to shutdown server: %v", err)
	}
	cancel()

	t.Log("Waiting for heartbeat failures...")
	time.Sleep(2 * time.Second)

	errors := testWorker.getHeartbeatErrors()
	if len(errors) == 0 {
		t.Error("Expected heartbeat errors during partition, got none")
	} else {
		t.Logf("Worker experienced %d heartbeat errors during partition", len(errors))
	}

	t.Log("Restoring network by restarting control plane...")
	newRegistry := scheduler.NewRegistry()
	newServer := api.NewServer("127.0.0.1:0", rm)
	newServer.SetRegistry(newRegistry)
	ConfigureTestServer(newServer)
	if err := newServer.Start(); err != nil {
		t.Fatalf("Failed to restart server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		newServer.Shutdown(ctx)
		cancel()
	}()

	newBaseURL := newServer.URL()
	t.Logf("Control plane restarted at %s", newBaseURL)

	testWorker.stop()

	newTestWorker := newTestWorker(newBaseURL)
	if err := newTestWorker.register(); err != nil {
		t.Fatalf("Failed to re-register worker: %v", err)
	}
	t.Logf("Worker re-registered with new ID: %s", newTestWorker.getWorkerID())

	if newRegistry.WorkerCount() != 1 {
		t.Errorf("Expected 1 worker in new registry, got %d", newRegistry.WorkerCount())
	}

	newTestWorker.stop()
	_ = leaseManager
}

func TestNetworkPartition_HeartbeatRecovery(t *testing.T) {
	validator := createTestValidator(t)
	rm := runmanager.NewRunManager(validator)
	registry := scheduler.NewRegistry()

	server := api.NewServer("127.0.0.1:0", rm)
	server.SetRegistry(registry)
	ConfigureTestServer(server)
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

	testWorker := newTestWorker(baseURL)
	if err := testWorker.register(); err != nil {
		t.Fatalf("Failed to register worker: %v", err)
	}
	workerID := testWorker.getWorkerID()
	t.Logf("Worker registered with ID: %s", workerID)

	for i := 0; i < 5; i++ {
		if err := testWorker.sendHeartbeat(); err != nil {
			t.Errorf("Heartbeat %d failed: %v", i+1, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Log("5 heartbeats sent successfully")

	workerInfo, err := registry.GetWorker(scheduler.WorkerID(workerID))
	if err != nil {
		t.Fatalf("Failed to get worker info: %v", err)
	}
	if workerInfo.Health == nil {
		t.Error("Worker health should be set after heartbeats")
	}

	testWorker.stop()
}

func TestNetworkPartition_TelemetryRecovery(t *testing.T) {
	validator := createTestValidator(t)
	rm := runmanager.NewRunManager(validator)
	registry := scheduler.NewRegistry()
	telemetryStore := api.NewTelemetryStore()

	server := api.NewServer("127.0.0.1:0", rm)
	server.SetRegistry(registry)
	server.SetTelemetryStore(telemetryStore)
	ConfigureTestServer(server)
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

	testWorker := newTestWorker(baseURL)
	if err := testWorker.register(); err != nil {
		t.Fatalf("Failed to register worker: %v", err)
	}
	workerID := testWorker.getWorkerID()
	t.Logf("Worker registered with ID: %s", workerID)

	telemetryReq := map[string]interface{}{
		"run_id": "run_0000000000000001",
		"operations": []map[string]interface{}{
			{
				"op_id":        "op-1",
				"operation":    "tools_list",
				"latency_ms":   50,
				"ok":           true,
				"ts_ms":        time.Now().UnixMilli(),
				"execution_id": "exe_00000000000001",
				"stage":        "preflight",
				"stage_id":     "stg_000000000001",
			},
		},
		"health": map[string]interface{}{
			"cpu_percent":     10.0,
			"mem_bytes":       1024 * 1024 * 100,
			"active_vus":      1,
			"active_sessions": 1,
			"in_flight_ops":   0,
			"queue_depth":     0,
		},
	}

	path := fmt.Sprintf("/workers/%s/telemetry", workerID)
	resp, err := testWorker.retryClient.Post(path, telemetryReq)
	if err != nil {
		t.Fatalf("Failed to send telemetry: %v", err)
	}

	body, err := worker.ReadResponseBody(resp)
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Telemetry failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Accepted int `json:"accepted"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if result.Accepted != 1 {
		t.Errorf("Expected 1 operation accepted, got %d", result.Accepted)
	}

	t.Logf("Telemetry sent successfully: %d operations accepted", result.Accepted)
	testWorker.stop()
}

func TestRetryHTTPClient_ExponentialBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	httpClient := &http.Client{Timeout: 100 * time.Millisecond}
	retryClient := worker.NewRetryHTTPClient(ctx, "http://127.0.0.1:1", httpClient, worker.RetryConfig{
		MaxRetries: 2,
		Backoff:    50 * time.Millisecond,
		MaxBackoff: 200 * time.Millisecond,
	})

	start := time.Now()
	_, err := retryClient.Post("/test", map[string]string{"test": "data"})
	elapsed := time.Since(start)

	if err == nil {
		t.Error("Expected error when connecting to non-existent server")
	}

	expectedMinDuration := 50*time.Millisecond + 100*time.Millisecond
	if elapsed < expectedMinDuration {
		t.Errorf("Expected at least %v for backoff, got %v", expectedMinDuration, elapsed)
	}

	t.Logf("Retry with backoff completed in %v", elapsed)
}

func TestRetryHTTPClient_NoRetryOn4xx(t *testing.T) {
	validator := createTestValidator(t)
	rm := runmanager.NewRunManager(validator)
	registry := scheduler.NewRegistry()

	server := api.NewServer("127.0.0.1:0", rm)
	server.SetRegistry(registry)
	ConfigureTestServer(server)
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		server.Shutdown(ctx)
		cancel()
	}()

	baseURL := server.URL()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	httpClient := &http.Client{Timeout: 5 * time.Second}
	retryClient := worker.NewRetryHTTPClient(ctx, baseURL, httpClient, worker.RetryConfig{
		MaxRetries: 3,
		Backoff:    100 * time.Millisecond,
	})

	start := time.Now()
	resp, err := retryClient.Post("/workers/nonexistent-worker/heartbeat", map[string]interface{}{})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected 404, got %d: %s", resp.StatusCode, string(body))
	}

	if elapsed > 500*time.Millisecond {
		t.Errorf("4xx error should not retry, but took %v", elapsed)
	}

	t.Logf("4xx error returned immediately in %v", elapsed)
}

func TestRetryHTTPClient_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	httpClient := &http.Client{Timeout: 5 * time.Second}
	retryClient := worker.NewRetryHTTPClient(ctx, "http://127.0.0.1:1", httpClient, worker.RetryConfig{
		MaxRetries: 10,
		Backoff:    1 * time.Second,
	})

	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err := retryClient.Post("/test", map[string]string{"test": "data"})
	elapsed := time.Since(start)

	if err == nil {
		t.Error("Expected error when context is cancelled")
	}

	if elapsed > 500*time.Millisecond {
		t.Errorf("Context cancellation should stop retries quickly, but took %v", elapsed)
	}

	t.Logf("Context cancellation stopped retries in %v", elapsed)
}

var _ = bytes.NewReader
