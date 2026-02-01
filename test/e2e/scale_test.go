package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/controlplane/api"
	"github.com/bc-dunia/mcpdrill/internal/controlplane/runmanager"
	"github.com/bc-dunia/mcpdrill/internal/controlplane/scheduler"
	"github.com/bc-dunia/mcpdrill/internal/worker"
)

func TestScaleValidation_1000VUs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping scale test in short mode")
	}

	const (
		numWorkers       = 10
		vusPerWorker     = 100
		totalVUs         = numWorkers * vusPerWorker
		registrationTime = 5 * time.Second
		testTimeout      = 5 * time.Minute
		maxCPUPercent    = 50.0
		maxMemoryMB      = 500.0
	)

	validator := createTestValidator(t)
	rm := runmanager.NewRunManager(validator)
	registry := scheduler.NewRegistry()
	leaseManager := scheduler.NewLeaseManager(60000)
	allocator := scheduler.NewAllocator(registry, leaseManager)
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

	monitorCtx, monitorCancel := context.WithCancel(context.Background())
	resourceStatsCh := MonitorResources(monitorCtx)

	workers := make([]*scaleTestWorker, numWorkers)
	var wg sync.WaitGroup
	var registrationErrors atomic.Int32

	registrationStart := time.Now()
	for i := 0; i < numWorkers; i++ {
		workers[i] = newScaleTestWorker(i, baseURL, vusPerWorker)
		wg.Add(1)
		go func(w *scaleTestWorker) {
			defer wg.Done()
			if err := registerScaleWorker(w, baseURL); err != nil {
				registrationErrors.Add(1)
				w.addError(err)
			}
		}(workers[i])
	}
	wg.Wait()
	registrationDuration := time.Since(registrationStart)

	if registrationErrors.Load() > 0 {
		t.Fatalf("Failed to register %d workers", registrationErrors.Load())
	}

	if registrationDuration > registrationTime {
		t.Errorf("Registration took %v, expected < %v", registrationDuration, registrationTime)
	}
	t.Logf("All %d workers registered in %v", numWorkers, registrationDuration)

	registeredCount := registry.WorkerCount()
	if registeredCount != numWorkers {
		t.Errorf("Expected %d workers in registry, got %d", numWorkers, registeredCount)
	}

	for _, w := range workers {
		go runScaleWorkerHeartbeat(w, baseURL, 1*time.Second)
	}

	time.Sleep(2 * time.Second)

	workerIDs := make([]scheduler.WorkerID, 0, numWorkers)
	for _, w := range workers {
		workerIDs = append(workerIDs, scheduler.WorkerID(w.getWorkerID()))
	}

	assignments, workerAssignmentsMap, err := allocator.AllocateAssignments("run_0000000000000001", "scale-stage", totalVUs, workerIDs)
	if err != nil {
		t.Fatalf("Failed to allocate assignments: %v", err)
	}

	if len(assignments) != numWorkers {
		t.Errorf("Expected %d assignments, got %d", numWorkers, len(assignments))
	}

	totalAssignedVUs := 0
	for workerID, a := range workerAssignmentsMap {
		vuCount := a.VUIDRange.End - a.VUIDRange.Start
		totalAssignedVUs += vuCount
		_, err := leaseManager.IssueLease(workerID, a)
		if err != nil {
			t.Errorf("Failed to issue lease for worker %s: %v", workerID, err)
		}
	}

	if totalAssignedVUs != totalVUs {
		t.Errorf("Expected %d total VUs assigned, got %d", totalVUs, totalAssignedVUs)
	}
	t.Logf("Allocated %d VUs across %d workers", totalAssignedVUs, len(assignments))

	var telemetryWg sync.WaitGroup
	var telemetryErrors atomic.Int32
	telemetryStart := time.Now()

	for _, w := range workers {
		telemetryWg.Add(1)
		go func(w *scaleTestWorker) {
			defer telemetryWg.Done()
			if err := sendScaleTelemetry(w, baseURL, "run_0000000000000001", vusPerWorker); err != nil {
				telemetryErrors.Add(1)
				w.addError(err)
			}
		}(w)
	}
	telemetryWg.Wait()
	telemetryDuration := time.Since(telemetryStart)

	if telemetryErrors.Load() > 0 {
		t.Errorf("Telemetry errors: %d", telemetryErrors.Load())
	}
	t.Logf("Telemetry from all workers collected in %v", telemetryDuration)

	for _, w := range workers {
		w.stop()
	}

	monitorCancel()
	resourceStats := <-resourceStatsCh

	t.Logf("Resource stats: PeakCPU=%.2f%%, PeakMemory=%.2fMB, Samples=%d, PeakGoroutines=%d",
		resourceStats.PeakCPU, resourceStats.PeakMemory, resourceStats.Samples, resourceStats.PeakGoroutines)

	if resourceStats.PeakCPU > maxCPUPercent {
		t.Errorf("Peak CPU %.2f%% exceeded limit of %.2f%%", resourceStats.PeakCPU, maxCPUPercent)
	}

	if resourceStats.PeakMemory > maxMemoryMB {
		t.Errorf("Peak memory %.2fMB exceeded limit of %.2fMB", resourceStats.PeakMemory, maxMemoryMB)
	}

	for i, w := range workers {
		if !w.isRegistered() {
			t.Errorf("Worker %d not registered", i)
		}
		if errs := w.getErrors(); len(errs) > 0 {
			t.Errorf("Worker %d had %d errors: %v", i, len(errs), errs)
		}
	}

	t.Logf("Scale validation passed: %d VUs across %d workers", totalVUs, numWorkers)
	t.Logf("  Registration: %v (limit: %v)", registrationDuration, registrationTime)
	t.Logf("  Peak CPU: %.2f%% (limit: %.2f%%)", resourceStats.PeakCPU, maxCPUPercent)
	t.Logf("  Peak Memory: %.2fMB (limit: %.2fMB)", resourceStats.PeakMemory, maxMemoryMB)
	t.Logf("  Peak Goroutines: %d", resourceStats.PeakGoroutines)

	_ = leaseManager
}

func TestScaleValidation_WorkerRegistrationThroughput(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping scale test in short mode")
	}

	const (
		numWorkers       = 50
		registrationTime = 10 * time.Second
	)

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

	workers := make([]*scaleTestWorker, numWorkers)
	var wg sync.WaitGroup
	var registrationErrors atomic.Int32

	registrationStart := time.Now()
	for i := 0; i < numWorkers; i++ {
		workers[i] = newScaleTestWorker(i, baseURL, 100)
		wg.Add(1)
		go func(w *scaleTestWorker) {
			defer wg.Done()
			if err := registerScaleWorker(w, baseURL); err != nil {
				registrationErrors.Add(1)
				w.addError(err)
			}
		}(workers[i])
	}
	wg.Wait()
	registrationDuration := time.Since(registrationStart)

	if registrationErrors.Load() > 0 {
		t.Errorf("Failed to register %d workers", registrationErrors.Load())
	}

	if registrationDuration > registrationTime {
		t.Errorf("Registration took %v, expected < %v", registrationDuration, registrationTime)
	}

	registeredCount := registry.WorkerCount()
	if registeredCount != numWorkers {
		t.Errorf("Expected %d workers in registry, got %d", numWorkers, registeredCount)
	}

	throughput := float64(numWorkers) / registrationDuration.Seconds()
	t.Logf("Registered %d workers in %v (%.2f workers/sec)", numWorkers, registrationDuration, throughput)

	for _, w := range workers {
		w.stop()
	}
}

func TestScaleValidation_ConcurrentHeartbeats(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping scale test in short mode")
	}

	const (
		numWorkers          = 20
		heartbeatsPerWorker = 10
	)

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

	workers := make([]*scaleTestWorker, numWorkers)
	for i := 0; i < numWorkers; i++ {
		workers[i] = newScaleTestWorker(i, baseURL, 100)
		if err := registerScaleWorker(workers[i], baseURL); err != nil {
			t.Fatalf("Failed to register worker %d: %v", i, err)
		}
	}

	var wg sync.WaitGroup
	var heartbeatErrors atomic.Int32

	heartbeatStart := time.Now()
	for _, w := range workers {
		wg.Add(1)
		go func(w *scaleTestWorker) {
			defer wg.Done()
			for j := 0; j < heartbeatsPerWorker; j++ {
				if err := sendScaleHeartbeat(w, baseURL); err != nil {
					heartbeatErrors.Add(1)
					w.addError(err)
				} else {
					w.incrementHeartbeat()
				}
				time.Sleep(50 * time.Millisecond)
			}
		}(w)
	}
	wg.Wait()
	heartbeatDuration := time.Since(heartbeatStart)

	totalHeartbeats := numWorkers * heartbeatsPerWorker
	successfulHeartbeats := totalHeartbeats - int(heartbeatErrors.Load())

	if heartbeatErrors.Load() > 0 {
		t.Errorf("Heartbeat errors: %d/%d", heartbeatErrors.Load(), totalHeartbeats)
	}

	throughput := float64(successfulHeartbeats) / heartbeatDuration.Seconds()
	t.Logf("Sent %d heartbeats in %v (%.2f heartbeats/sec)", successfulHeartbeats, heartbeatDuration, throughput)

	for _, w := range workers {
		w.stop()
	}
}

func registerScaleWorker(w *scaleTestWorker, baseURL string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	httpClient := &http.Client{Timeout: 5 * time.Second}
	retryClient := worker.NewRetryHTTPClient(ctx, baseURL, httpClient, worker.RetryConfig{
		MaxRetries: 3,
		Backoff:    100 * time.Millisecond,
		MaxBackoff: 1 * time.Second,
	})

	req := map[string]interface{}{
		"host_info": map[string]string{
			"hostname": fmt.Sprintf("scale-worker-%d", w.id),
			"ip_addr":  "127.0.0.1",
			"platform": "test",
		},
		"capacity": map[string]interface{}{
			"max_vus":            w.capacity,
			"max_concurrent_ops": w.capacity / 2,
			"max_rps":            float64(w.capacity) * 10,
		},
	}

	resp, err := retryClient.Post("/workers/register", req)
	if err != nil {
		return fmt.Errorf("registration request failed: %w", err)
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

	w.setRegistered(result.WorkerID)
	return nil
}

func runScaleWorkerHeartbeat(w *scaleTestWorker, baseURL string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			if err := sendScaleHeartbeat(w, baseURL); err != nil {
				w.addError(err)
			} else {
				w.incrementHeartbeat()
			}
		}
	}
}

func sendScaleHeartbeat(w *scaleTestWorker, baseURL string) error {
	workerID := w.getWorkerID()
	if workerID == "" {
		return fmt.Errorf("worker not registered")
	}

	httpClient := &http.Client{Timeout: 5 * time.Second}
	retryClient := worker.NewRetryHTTPClient(w.ctx, baseURL, httpClient, worker.RetryConfig{
		MaxRetries: 2,
		Backoff:    100 * time.Millisecond,
		MaxBackoff: 500 * time.Millisecond,
	})

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
	resp, err := retryClient.Post(path, req)
	if err != nil {
		return err
	}

	_, err = worker.ReadResponseBody(resp)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("heartbeat failed with status %d", resp.StatusCode)
	}

	return nil
}

func sendScaleTelemetry(w *scaleTestWorker, baseURL, runID string, numOps int) error {
	workerID := w.getWorkerID()
	if workerID == "" {
		return fmt.Errorf("worker not registered")
	}

	httpClient := &http.Client{Timeout: 10 * time.Second}
	retryClient := worker.NewRetryHTTPClient(w.ctx, baseURL, httpClient, worker.RetryConfig{
		MaxRetries: 3,
		Backoff:    100 * time.Millisecond,
		MaxBackoff: 1 * time.Second,
	})

	operations := make([]map[string]interface{}, numOps)
	now := time.Now().UnixMilli()
	for i := 0; i < numOps; i++ {
		operations[i] = map[string]interface{}{
			"op_id":        fmt.Sprintf("op-%d-%d", w.id, i),
			"operation":    "tools_list",
			"latency_ms":   50 + (i % 100),
			"ok":           i%10 != 0,
			"ts_ms":        now + int64(i*10),
			"execution_id": "exe_00000000000001",
			"stage":        "preflight",
			"stage_id":     "stg_000000000001",
		}
	}

	req := map[string]interface{}{
		"run_id":     runID,
		"operations": operations,
		"health": map[string]interface{}{
			"cpu_percent":     20.0 + float64(w.id),
			"mem_bytes":       1024 * 1024 * 150,
			"active_vus":      numOps,
			"active_sessions": numOps / 2,
			"in_flight_ops":   numOps / 10,
			"queue_depth":     0,
		},
	}

	path := fmt.Sprintf("/workers/%s/telemetry", workerID)
	resp, err := retryClient.Post(path, req)
	if err != nil {
		return err
	}

	body, err := worker.ReadResponseBody(resp)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telemetry failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
