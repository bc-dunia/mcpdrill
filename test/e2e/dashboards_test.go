package e2e

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/analysis"
	"github.com/bc-dunia/mcpdrill/internal/controlplane/api"
	"github.com/bc-dunia/mcpdrill/internal/controlplane/runmanager"
	"github.com/bc-dunia/mcpdrill/internal/controlplane/scheduler"
	"github.com/bc-dunia/mcpdrill/internal/metrics"
	"github.com/bc-dunia/mcpdrill/internal/types"
)

// TestPrometheusMetrics tests the /metrics endpoint returns valid Prometheus format.
func TestPrometheusMetrics(t *testing.T) {
	// Setup: Create validator + RunManager + API server with metrics collector
	validator := createTestValidator(t)
	rm := runmanager.NewRunManager(validator)
	telemetryStore := api.NewTelemetryStore()
	registry := scheduler.NewRegistry()

	server, cleanup, err := api.StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	// Configure server with telemetry store and registry
	server.SetTelemetryStore(telemetryStore)
	server.SetRegistry(registry)

	// Create metrics collector with providers
	metricsCollector := metrics.NewCollector()
	metricsCollector.SetRunProvider(rm)
	metricsCollector.SetWorkerProvider(&registryAdapter{registry: registry})
	metricsCollector.SetTelemetryProvider(telemetryStore)
	server.SetMetricsCollector(metricsCollector)

	baseURL := server.URL()
	t.Logf("Test server started at %s", baseURL)

	// Step 1: Create a run
	runID := createRun(t, baseURL, validConfig)
	t.Logf("Created run: %s", runID)

	// Step 2: Register a worker
	workerID := registerWorker(t, baseURL, "test-host", 10)
	t.Logf("Registered worker: %s", workerID)

	// Step 3: Send heartbeat with health data
	sendHeartbeat(t, baseURL, workerID, 45.5, 1024*1024*512, 5)
	t.Logf("Sent heartbeat for worker: %s", workerID)

	// Step 4: Start the run and inject telemetry
	startRun(t, baseURL, runID)
	now := time.Now().UnixMilli()
	injectTelemetryWithContext(t, telemetryStore, runID, now, "worker-1", "baseline", "stg_000000000002", "1")
	t.Logf("Injected telemetry data")

	// Step 5: Record metrics directly
	metricsCollector.RecordRunCreated("e2e-test-scenario")
	metricsCollector.RecordOperation("tools/list", "", 100, true)
	metricsCollector.RecordOperation("tools/call", "test_tool", 200, false)
	metricsCollector.RecordStageMetrics(runID, "stg_000000000002", 10.5, 5)

	// Step 6: Scrape /metrics endpoint
	resp, err := http.Get(baseURL + "/metrics")
	if err != nil {
		t.Fatalf("Failed to get /metrics: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 for /metrics, got %d", resp.StatusCode)
	}

	// Verify content type
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/plain") {
		t.Errorf("Expected text/plain content type, got %s", contentType)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}
	bodyStr := string(body)
	t.Logf("Metrics response (%d bytes)", len(body))

	// Step 7: Verify Prometheus format
	verifyPrometheusFormat(t, bodyStr)

	// Step 8: Verify specific metrics are present
	verifyMetricPresent(t, bodyStr, "mcpdrill_runs_total")
	verifyMetricPresent(t, bodyStr, "mcpdrill_workers_total")
	verifyMetricPresent(t, bodyStr, "mcpdrill_operations_total")
	verifyMetricPresent(t, bodyStr, "mcpdrill_worker_health_cpu_percent")
	verifyMetricPresent(t, bodyStr, "mcpdrill_worker_health_memory_mb")
	verifyMetricPresent(t, bodyStr, "mcpdrill_worker_health_active_vus")
	verifyMetricPresent(t, bodyStr, "mcpdrill_stage_duration_seconds")
	verifyMetricPresent(t, bodyStr, "mcpdrill_stage_vus")

	t.Logf("Prometheus metrics test passed")
}

// TestPrometheusMetricsNotConfigured tests that /metrics returns 503 when not configured.
func TestPrometheusMetricsNotConfigured(t *testing.T) {
	validator := createTestValidator(t)
	rm := runmanager.NewRunManager(validator)
	server, cleanup, err := api.StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	baseURL := server.URL()

	resp, err := http.Get(baseURL + "/metrics")
	if err != nil {
		t.Fatalf("Failed to get /metrics: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Expected 503 when metrics not configured, got %d", resp.StatusCode)
	}
}

// TestLogQueryAPI tests the log query API with various filters.
func TestLogQueryAPI(t *testing.T) {
	validator := createTestValidator(t)
	rm := runmanager.NewRunManager(validator)
	telemetryStore := api.NewTelemetryStore()

	server, cleanup, err := api.StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	server.SetTelemetryStore(telemetryStore)

	baseURL := server.URL()
	t.Logf("Test server started at %s", baseURL)

	// Step 1: Create and start run
	runID := createRun(t, baseURL, validConfig)
	startRun(t, baseURL, runID)
	t.Logf("Created and started run: %s", runID)

	// Step 2: Inject telemetry with various operations
	now := time.Now().UnixMilli()
	injectVariedTelemetry(t, telemetryStore, runID, now)
	t.Logf("Injected varied telemetry data")

	// Step 3: Query logs with no filters
	logs := queryLogs(t, baseURL, runID, "")
	if logs.Total == 0 {
		t.Error("Expected logs to be returned, got 0")
	}
	t.Logf("Query with no filters returned %d logs (total: %d)", len(logs.Logs), logs.Total)

	// Step 4: Query with operation filter
	logs = queryLogs(t, baseURL, runID, "operation=tools/list")
	for _, log := range logs.Logs {
		if log.Operation != "tools/list" {
			t.Errorf("Expected operation tools/list, got %s", log.Operation)
		}
	}
	t.Logf("Query with operation filter returned %d logs", len(logs.Logs))

	// Step 5: Query with tool_name filter
	logs = queryLogs(t, baseURL, runID, "tool_name=test_tool")
	for _, log := range logs.Logs {
		if log.ToolName != "test_tool" {
			t.Errorf("Expected tool_name test_tool, got %s", log.ToolName)
		}
	}
	t.Logf("Query with tool_name filter returned %d logs", len(logs.Logs))

	// Step 6: Query with stage filter
	logs = queryLogs(t, baseURL, runID, "stage=baseline")
	for _, log := range logs.Logs {
		if log.Stage != "baseline" {
			t.Errorf("Expected stage baseline, got %s", log.Stage)
		}
	}
	t.Logf("Query with stage filter returned %d logs", len(logs.Logs))

	// Step 7: Query with pagination
	logs = queryLogs(t, baseURL, runID, "limit=5&offset=0")
	if len(logs.Logs) > 5 {
		t.Errorf("Expected at most 5 logs with limit=5, got %d", len(logs.Logs))
	}
	if logs.Limit != 5 {
		t.Errorf("Expected limit=5 in response, got %d", logs.Limit)
	}
	t.Logf("Query with pagination returned %d logs", len(logs.Logs))

	// Step 8: Query with offset
	logs = queryLogs(t, baseURL, runID, "limit=5&offset=5")
	if logs.Offset != 5 {
		t.Errorf("Expected offset=5 in response, got %d", logs.Offset)
	}
	t.Logf("Query with offset returned %d logs", len(logs.Logs))

	// Step 9: Query with ascending order
	logs = queryLogs(t, baseURL, runID, "order=asc")
	if len(logs.Logs) >= 2 {
		if logs.Logs[0].TimestampMs > logs.Logs[1].TimestampMs {
			t.Error("Expected ascending order by timestamp")
		}
	}
	t.Logf("Query with ascending order returned %d logs", len(logs.Logs))

	t.Logf("Log query API test passed")
}

// TestLogQueryAPIRunNotFound tests 404 handling for non-existent runs.
func TestLogQueryAPIRunNotFound(t *testing.T) {
	validator := createTestValidator(t)
	rm := runmanager.NewRunManager(validator)
	telemetryStore := api.NewTelemetryStore()

	server, cleanup, err := api.StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	server.SetTelemetryStore(telemetryStore)

	baseURL := server.URL()

	resp, err := http.Get(baseURL + "/runs/nonexistent-run/logs")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected 404, got %d", resp.StatusCode)
	}
}

// TestErrorSignatures tests the error signatures API.
func TestErrorSignatures(t *testing.T) {
	validator := createTestValidator(t)
	rm := runmanager.NewRunManager(validator)
	telemetryStore := api.NewTelemetryStore()

	server, cleanup, err := api.StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	server.SetTelemetryStore(telemetryStore)

	baseURL := server.URL()
	t.Logf("Test server started at %s", baseURL)

	// Step 1: Create and start run
	runID := createRun(t, baseURL, validConfig)
	startRun(t, baseURL, runID)
	t.Logf("Created and started run: %s", runID)

	// Step 2: Inject telemetry with errors
	now := time.Now().UnixMilli()
	injectTelemetryWithErrors(t, telemetryStore, runID, now)
	t.Logf("Injected telemetry with errors")

	// Step 3: Query error signatures
	signatures := getErrorSignatures(t, baseURL, runID)
	t.Logf("Got %d error signatures", len(signatures.Signatures))

	// Step 4: Verify signatures are returned
	if len(signatures.Signatures) == 0 {
		t.Error("Expected error signatures to be returned")
	}

	// Step 5: Verify normalization (UUIDs, numbers replaced)
	for _, sig := range signatures.Signatures {
		if strings.Contains(sig.Pattern, "12345") {
			t.Errorf("Expected numbers to be normalized, found raw number in pattern: %s", sig.Pattern)
		}
		t.Logf("Signature: %s (count: %d)", sig.Pattern, sig.Count)
	}

	// Step 6: Verify ranking by count
	for i := 1; i < len(signatures.Signatures); i++ {
		if signatures.Signatures[i].Count > signatures.Signatures[i-1].Count {
			t.Error("Expected signatures to be sorted by count descending")
		}
	}

	// Step 7: Verify top 10 limit
	if len(signatures.Signatures) > 10 {
		t.Errorf("Expected at most 10 signatures, got %d", len(signatures.Signatures))
	}

	t.Logf("Error signatures test passed")
}

// TestErrorSignaturesNoErrors tests the error signatures API with no errors.
func TestErrorSignaturesNoErrors(t *testing.T) {
	validator := createTestValidator(t)
	rm := runmanager.NewRunManager(validator)
	telemetryStore := api.NewTelemetryStore()

	server, cleanup, err := api.StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	server.SetTelemetryStore(telemetryStore)

	baseURL := server.URL()

	// Create and start run
	runID := createRun(t, baseURL, validConfig)
	startRun(t, baseURL, runID)

	// Inject telemetry with no errors
	now := time.Now().UnixMilli()
	injectTelemetryDirect(t, telemetryStore, runID, now)

	// Query error signatures
	signatures := getErrorSignatures(t, baseURL, runID)

	// Should return empty array, not error
	if signatures.Signatures == nil {
		t.Error("Expected empty array, got nil")
	}
	t.Logf("Got %d signatures (expected 0 or few)", len(signatures.Signatures))
}

// TestErrorSignaturesRunNotFound tests 404 handling for non-existent runs.
func TestErrorSignaturesRunNotFound(t *testing.T) {
	validator := createTestValidator(t)
	rm := runmanager.NewRunManager(validator)
	telemetryStore := api.NewTelemetryStore()

	server, cleanup, err := api.StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	server.SetTelemetryStore(telemetryStore)

	baseURL := server.URL()

	resp, err := http.Get(baseURL + "/runs/nonexistent-run/errors/signatures")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected 404, got %d", resp.StatusCode)
	}
}

// =============================================================================
// Test Helpers
// =============================================================================

// registryAdapter adapts scheduler.Registry to metrics.WorkerProvider.
type registryAdapter struct {
	registry *scheduler.Registry
}

func (a *registryAdapter) ListWorkers() []*scheduler.WorkerInfo {
	return a.registry.ListWorkers()
}

// registerWorker registers a worker via the API.
func registerWorker(t *testing.T, baseURL, hostname string, maxVUs int) string {
	t.Helper()

	reqBody := map[string]interface{}{
		"host_info": map[string]string{
			"hostname": hostname,
			"ip_addr":  "127.0.0.1",
			"platform": "linux",
		},
		"capacity": map[string]interface{}{
			"max_vus":            maxVUs,
			"max_concurrent_ops": maxVUs * 2,
			"max_rps":            100.0,
		},
	}

	resp := doPost(t, baseURL+"/workers/register", reqBody)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to register worker: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var result struct {
		WorkerID string `json:"worker_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode register response: %v", err)
	}

	return result.WorkerID
}

// sendHeartbeat sends a heartbeat with health data.
func sendHeartbeat(t *testing.T, baseURL, workerID string, cpuPercent float64, memBytes int64, activeVUs int) {
	t.Helper()

	reqBody := map[string]interface{}{
		"health": map[string]interface{}{
			"cpu_percent":     cpuPercent,
			"mem_bytes":       memBytes,
			"active_vus":      activeVUs,
			"active_sessions": 0,
			"in_flight_ops":   0,
			"queue_depth":     0,
		},
	}

	resp := doPost(t, baseURL+"/workers/"+workerID+"/heartbeat", reqBody)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to send heartbeat: status=%d, body=%s", resp.StatusCode, string(body))
	}
}

// injectTelemetryWithContext injects telemetry with full context.
func injectTelemetryWithContext(t *testing.T, store *api.TelemetryStore, runID string, startTimeMs int64, workerID, stage, stageID string, vuID string) {
	t.Helper()

	operations := make([]types.OperationOutcome, 10)
	for i := range 10 {
		latency := 50 + i*10
		ok := i < 8
		errorType := ""
		if !ok {
			errorType = "timeout"
		}

		operations[i] = types.OperationOutcome{
			Operation:   "tools_list",
			ToolName:    "",
			LatencyMs:   latency,
			OK:          ok,
			ErrorType:   errorType,
			TimestampMs: startTimeMs + int64(i*100),
		}
	}

	batch := api.TelemetryBatchRequest{
		RunID:      runID,
		Operations: operations,
	}

	store.AddTelemetryBatchWithContext(runID, batch, workerID, stage, stageID, vuID)
}

// injectVariedTelemetry injects telemetry with various operations and stages.
func injectVariedTelemetry(t *testing.T, store *api.TelemetryStore, runID string, startTimeMs int64) {
	t.Helper()

	operations := []types.OperationOutcome{
		{Operation: "tools/list", ToolName: "", LatencyMs: 50, OK: true, TimestampMs: startTimeMs},
		{Operation: "tools/list", ToolName: "", LatencyMs: 60, OK: true, TimestampMs: startTimeMs + 100},
		{Operation: "tools/call", ToolName: "test_tool", LatencyMs: 100, OK: true, TimestampMs: startTimeMs + 200},
		{Operation: "tools/call", ToolName: "test_tool", LatencyMs: 150, OK: false, ErrorType: "timeout", TimestampMs: startTimeMs + 300},
		{Operation: "resources/list", ToolName: "", LatencyMs: 30, OK: true, TimestampMs: startTimeMs + 400},
		{Operation: "resources/read", ToolName: "", LatencyMs: 80, OK: true, TimestampMs: startTimeMs + 500},
		{Operation: "tools/list", ToolName: "", LatencyMs: 55, OK: true, TimestampMs: startTimeMs + 600},
		{Operation: "tools/call", ToolName: "other_tool", LatencyMs: 200, OK: true, TimestampMs: startTimeMs + 700},
	}

	batch := api.TelemetryBatchRequest{
		RunID:      runID,
		Operations: operations,
	}

	store.AddTelemetryBatchWithContext(runID, batch, "worker-1", "baseline", "stg_000000000002", "1")

	// Add more with different stage
	operations2 := []types.OperationOutcome{
		{Operation: "tools/list", ToolName: "", LatencyMs: 45, OK: true, TimestampMs: startTimeMs + 800},
		{Operation: "tools/call", ToolName: "test_tool", LatencyMs: 120, OK: true, TimestampMs: startTimeMs + 900},
	}

	batch2 := api.TelemetryBatchRequest{
		RunID:      runID,
		Operations: operations2,
	}

	store.AddTelemetryBatchWithContext(runID, batch2, "worker-2", "ramp", "stg_000000000003", "2")
}

// injectTelemetryWithErrors injects telemetry with various error types.
func injectTelemetryWithErrors(t *testing.T, store *api.TelemetryStore, runID string, startTimeMs int64) {
	t.Helper()

	operations := []types.OperationOutcome{
		{Operation: "tools/call", ToolName: "api_client", LatencyMs: 100, OK: false, ErrorType: "connection refused to localhost:3000", TimestampMs: startTimeMs},
		{Operation: "tools/call", ToolName: "api_client", LatencyMs: 100, OK: false, ErrorType: "connection refused to localhost:3000", TimestampMs: startTimeMs + 100},
		{Operation: "tools/call", ToolName: "api_client", LatencyMs: 100, OK: false, ErrorType: "connection refused to localhost:3001", TimestampMs: startTimeMs + 200},
		{Operation: "tools/call", ToolName: "file_reader", LatencyMs: 50, OK: false, ErrorType: "file not found: /tmp/test12345.txt", TimestampMs: startTimeMs + 300},
		{Operation: "tools/call", ToolName: "file_reader", LatencyMs: 50, OK: false, ErrorType: "file not found: /tmp/test67890.txt", TimestampMs: startTimeMs + 400},
		{Operation: "resources/read", ToolName: "", LatencyMs: 200, OK: false, ErrorType: "timeout after 5000ms", TimestampMs: startTimeMs + 500},
		{Operation: "resources/read", ToolName: "", LatencyMs: 200, OK: false, ErrorType: "timeout after 5000ms", TimestampMs: startTimeMs + 600},
		{Operation: "resources/read", ToolName: "", LatencyMs: 200, OK: false, ErrorType: "timeout after 5000ms", TimestampMs: startTimeMs + 700},
		{Operation: "tools/list", ToolName: "", LatencyMs: 30, OK: true, TimestampMs: startTimeMs + 800},
		{Operation: "tools/list", ToolName: "", LatencyMs: 35, OK: true, TimestampMs: startTimeMs + 900},
	}

	batch := api.TelemetryBatchRequest{
		RunID:      runID,
		Operations: operations,
	}

	store.AddTelemetryBatchWithContext(runID, batch, "worker-1", "baseline", "stg_000000000002", "1")
}

// queryLogs queries the log API with optional query string.
func queryLogs(t *testing.T, baseURL, runID, queryString string) *api.LogQueryResponse {
	t.Helper()

	url := baseURL + "/runs/" + runID + "/logs"
	if queryString != "" {
		url += "?" + queryString
	}

	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("Failed to query logs: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to query logs: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var result api.LogQueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode logs response: %v", err)
	}

	return &result
}

// getErrorSignatures queries the error signatures API.
func getErrorSignatures(t *testing.T, baseURL, runID string) *errorSignaturesResponse {
	t.Helper()

	resp, err := http.Get(baseURL + "/runs/" + runID + "/errors/signatures")
	if err != nil {
		t.Fatalf("Failed to get error signatures: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to get error signatures: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var result errorSignaturesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode error signatures response: %v", err)
	}

	return &result
}

type errorSignaturesResponse struct {
	RunID      string                    `json:"run_id"`
	Signatures []analysis.ErrorSignature `json:"signatures"`
}

// verifyPrometheusFormat verifies the response is valid Prometheus text format.
func verifyPrometheusFormat(t *testing.T, body string) {
	t.Helper()

	lines := strings.Split(body, "\n")
	hasHelp := false
	hasType := false
	hasMetric := false

	for _, line := range lines {
		if strings.HasPrefix(line, "# HELP") {
			hasHelp = true
		}
		if strings.HasPrefix(line, "# TYPE") {
			hasType = true
		}
		if strings.Contains(line, "mcpdrill_") && !strings.HasPrefix(line, "#") {
			hasMetric = true
		}
	}

	if !hasHelp {
		t.Error("Missing # HELP lines in Prometheus output")
	}
	if !hasType {
		t.Error("Missing # TYPE lines in Prometheus output")
	}
	if !hasMetric {
		t.Error("Missing metric lines in Prometheus output")
	}
}

// verifyMetricPresent verifies a specific metric is present in the output.
func verifyMetricPresent(t *testing.T, body, metricName string) {
	t.Helper()

	if !strings.Contains(body, metricName) {
		t.Errorf("Expected metric %s to be present in output", metricName)
	}
}
