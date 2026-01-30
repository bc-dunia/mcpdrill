package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/artifacts"
	"github.com/bc-dunia/mcpdrill/internal/controlplane/api"
	"github.com/bc-dunia/mcpdrill/internal/controlplane/runmanager"
	"github.com/bc-dunia/mcpdrill/internal/types"
	"github.com/bc-dunia/mcpdrill/internal/validation"
)

var validConfig = []byte(`{
	"schema_version": "run-config/v1",
	"scenario_id": "e2e-test-scenario",
	"metadata": {
		"name": "E2E Test Config",
		"description": "Minimal config for E2E testing",
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
			"duration_ms": 10000,
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
			"duration_ms": 10000,
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
			"duration_ms": 10000,
			"load": {
				"target_vus": 2,
				"target_rps": 2
			},
			"ramp": {
				"mode": "step",
				"step_every_ms": 5000,
				"step_vus": 1,
				"step_rps": 1,
				"max_vus": 5,
				"max_rps": 5,
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

// TestBasicE2E tests the full run lifecycle through the HTTP API.
// Scenario: Create run -> Start run -> Verify events -> Stop run
func TestBasicE2E(t *testing.T) {
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

	// Test: Create run
	runID := createRun(t, baseURL, validConfig)
	t.Logf("Created run: %s", runID)

	// Test: Verify initial state is CREATED
	status := getRun(t, baseURL, runID)
	assertState(t, status, "created")
	t.Logf("Run state after create: %s", status.State)

	events := getEvents(t, baseURL, runID)
	assertEventExists(t, events, "RUN_CREATED")
	t.Logf("Found %d events after create", len(events))

	startRun(t, baseURL, runID)
	t.Logf("Started run: %s", runID)

	status = getRun(t, baseURL, runID)
	assertState(t, status, "preflight_running")
	t.Logf("Run state after start: %s", status.State)

	events = getEvents(t, baseURL, runID)
	assertEventExists(t, events, "STATE_TRANSITION")
	t.Logf("Found %d events after start", len(events))

	stopRun(t, baseURL, runID, "drain")
	t.Logf("Stopped run: %s", runID)

	status = getRun(t, baseURL, runID)
	assertState(t, status, "stopping")
	t.Logf("Run state after stop: %s", status.State)

	events = getEvents(t, baseURL, runID)
	assertEventExists(t, events, "STOP_REQUESTED")
	t.Logf("Found %d events after stop", len(events))

	assertEventExists(t, events, "RUN_CREATED")
	assertEventExists(t, events, "STATE_TRANSITION")
	assertEventExists(t, events, "STOP_REQUESTED")

	t.Logf("E2E test completed successfully with %d total events", len(events))
}

// TestE2EEmergencyStop tests the emergency stop flow.
func TestE2EEmergencyStop(t *testing.T) {
	validator := createTestValidator(t)
	rm := runmanager.NewRunManager(validator)
	server, cleanup, err := api.StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	baseURL := server.URL()

	// Create and start run
	runID := createRun(t, baseURL, validConfig)
	startRun(t, baseURL, runID)

	status := getRun(t, baseURL, runID)
	assertState(t, status, "preflight_running")

	emergencyStop(t, baseURL, runID)

	status = getRun(t, baseURL, runID)
	if status.State != "stopping" && status.State != "completed" {
		t.Errorf("Expected state stopping or completed, got %s", status.State)
	}

	events := getEvents(t, baseURL, runID)
	assertEventExists(t, events, "EMERGENCY_STOP")
}

// TestE2ERunNotFound tests 404 handling for non-existent runs.
func TestE2ERunNotFound(t *testing.T) {
	validator := createTestValidator(t)
	rm := runmanager.NewRunManager(validator)
	server, cleanup, err := api.StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	baseURL := server.URL()

	// Try to get non-existent run
	resp, err := http.Get(baseURL + "/runs/nonexistent-run-id")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected 404, got %d", resp.StatusCode)
	}
}

// TestE2EInvalidStateTransition tests that invalid state transitions return errors.
func TestE2EInvalidStateTransition(t *testing.T) {
	validator := createTestValidator(t)
	rm := runmanager.NewRunManager(validator)
	server, cleanup, err := api.StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	baseURL := server.URL()

	// Create run (state: CREATED)
	runID := createRun(t, baseURL, validConfig)

	// Try to stop without starting (should fail - can't stop CREATED run)
	resp := doPost(t, baseURL+"/runs/"+runID+"/stop", map[string]string{"mode": "drain"})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Errorf("Expected 409 Conflict for invalid state transition, got %d", resp.StatusCode)
	}
}

// TestE2EValidationFailure tests that invalid configs are rejected.
func TestE2EValidationFailure(t *testing.T) {
	validator := createTestValidator(t)
	rm := runmanager.NewRunManager(validator)
	server, cleanup, err := api.StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	baseURL := server.URL()

	// Invalid config (missing required fields)
	invalidConfig := []byte(`{"scenario_id": "test"}`)

	resp := doPost(t, baseURL+"/runs", map[string]interface{}{
		"config": json.RawMessage(invalidConfig),
		"actor":  "test",
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected 400 Bad Request for invalid config, got %d", resp.StatusCode)
	}
}

// TestE2EHealthEndpoints tests the health check endpoints.
func TestE2EHealthEndpoints(t *testing.T) {
	validator := createTestValidator(t)
	rm := runmanager.NewRunManager(validator)
	server, cleanup, err := api.StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	baseURL := server.URL()

	// Test /healthz
	resp, err := http.Get(baseURL + "/healthz")
	if err != nil {
		t.Fatalf("Failed to get /healthz: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 for /healthz, got %d", resp.StatusCode)
	}

	var healthResp map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&healthResp); err != nil {
		t.Fatalf("Failed to decode health response: %v", err)
	}
	if healthResp["status"] != "ok" {
		t.Errorf("Expected status 'ok', got '%s'", healthResp["status"])
	}

	// Test /readyz
	resp2, err := http.Get(baseURL + "/readyz")
	if err != nil {
		t.Fatalf("Failed to get /readyz: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 for /readyz, got %d", resp2.StatusCode)
	}

	var readyResp map[string]interface{}
	if err := json.NewDecoder(resp2.Body).Decode(&readyResp); err != nil {
		t.Fatalf("Failed to decode ready response: %v", err)
	}
	if readyResp["ready"] != true {
		t.Errorf("Expected ready=true, got %v", readyResp["ready"])
	}
}

// TestE2EMultipleRuns tests creating and managing multiple runs concurrently.
func TestE2EMultipleRuns(t *testing.T) {
	validator := createTestValidator(t)
	rm := runmanager.NewRunManager(validator)
	server, cleanup, err := api.StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	baseURL := server.URL()

	// Create multiple runs
	runIDs := make([]string, 3)
	for i := 0; i < 3; i++ {
		runIDs[i] = createRun(t, baseURL, validConfig)
		t.Logf("Created run %d: %s", i, runIDs[i])
	}

	for i, runID := range runIDs {
		status := getRun(t, baseURL, runID)
		assertState(t, status, "created")
		t.Logf("Run %d state: %s", i, status.State)
	}

	for i, runID := range runIDs {
		startRun(t, baseURL, runID)
		t.Logf("Started run %d", i)
	}

	for i, runID := range runIDs {
		status := getRun(t, baseURL, runID)
		assertState(t, status, "preflight_running")
		t.Logf("Run %d state after start: %s", i, status.State)
	}

	for i, runID := range runIDs {
		stopRun(t, baseURL, runID, "drain")
		t.Logf("Stopped run %d", i)
	}

	for i, runID := range runIDs {
		status := getRun(t, baseURL, runID)
		assertState(t, status, "stopping")
		t.Logf("Run %d state after stop: %s", i, status.State)
	}
}

// =============================================================================
// Test Helpers
// =============================================================================

// createTestValidator creates a UnifiedValidator for testing.
func createTestValidator(t *testing.T) *validation.UnifiedValidator {
	t.Helper()

	policy := validation.DefaultSystemPolicy()
	policy.AllowPrivateNetworks = []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "127.0.0.0/8"}

	validator, err := validation.NewUnifiedValidator(policy)
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}
	return validator
}

// createRun creates a new run via the API and returns the run ID.
func createRun(t *testing.T, baseURL string, config []byte) string {
	t.Helper()

	reqBody := map[string]interface{}{
		"config": json.RawMessage(config),
		"actor":  "e2e-test",
	}

	resp := doPost(t, baseURL+"/runs", reqBody)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to create run: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var result struct {
		RunID string `json:"run_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode create response: %v", err)
	}

	if result.RunID == "" {
		t.Fatal("Create run returned empty run_id")
	}

	return result.RunID
}

// startRun starts a run via the API.
func startRun(t *testing.T, baseURL, runID string) {
	t.Helper()

	resp := doPost(t, baseURL+"/runs/"+runID+"/start", map[string]string{"actor": "e2e-test"})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to start run: status=%d, body=%s", resp.StatusCode, string(body))
	}
}

// stopRun stops a run via the API.
func stopRun(t *testing.T, baseURL, runID, mode string) {
	t.Helper()

	resp := doPost(t, baseURL+"/runs/"+runID+"/stop", map[string]string{
		"mode":  mode,
		"actor": "e2e-test",
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to stop run: status=%d, body=%s", resp.StatusCode, string(body))
	}
}

// emergencyStop performs an emergency stop via the API.
func emergencyStop(t *testing.T, baseURL, runID string) {
	t.Helper()

	resp := doPost(t, baseURL+"/runs/"+runID+"/emergency-stop", map[string]string{"actor": "e2e-test"})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to emergency stop run: status=%d, body=%s", resp.StatusCode, string(body))
	}
}

// getRun gets the current run status via the API.
func getRun(t *testing.T, baseURL, runID string) *runmanager.RunView {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/runs/"+runID, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to get run: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to get run: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var result runmanager.RunView
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode run response: %v", err)
	}

	return &result
}

// getEvents gets all events for a run via the API.
// Note: This uses the TailEvents method directly since SSE streaming
// would require more complex handling for tests.
func getEvents(t *testing.T, baseURL, runID string) []runmanager.RunEvent {
	t.Helper()

	// For E2E tests, we access events through the RunManager directly
	// since SSE streaming is more complex to test synchronously.
	// The SSE endpoint is tested separately in the API tests.

	// We'll use a simple HTTP GET to the events endpoint with a short timeout
	// and parse any events that come through.
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/runs/"+runID+"/events", nil)
	if err != nil {
		t.Fatalf("Failed to create events request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// Context timeout is expected - we just want to collect events
		if ctx.Err() == context.DeadlineExceeded {
			return nil
		}
		t.Fatalf("Failed to get events: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to get events: status=%d, body=%s", resp.StatusCode, string(body))
	}

	// Parse SSE events
	var events []runmanager.RunEvent
	body, err := io.ReadAll(resp.Body)
	if err != nil && ctx.Err() != context.DeadlineExceeded {
		t.Fatalf("Failed to read events body: %v", err)
	}

	// Parse SSE format: "id: N\ndata: {...}\n\n"
	lines := strings.Split(string(body), "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			var event runmanager.RunEvent
			if err := json.Unmarshal([]byte(data), &event); err == nil {
				events = append(events, event)
			}
		}
	}

	return events
}

// assertState asserts that the run is in the expected state.
func assertState(t *testing.T, run *runmanager.RunView, expectedState string) {
	t.Helper()
	if string(run.State) != expectedState {
		t.Errorf("Expected state %s, got %s", expectedState, run.State)
	}
}

// assertEventExists asserts that an event of the given type exists in the events list.
func assertEventExists(t *testing.T, events []runmanager.RunEvent, eventType string) {
	t.Helper()
	for _, event := range events {
		if string(event.Type) == eventType {
			return
		}
	}
	t.Errorf("Expected event type %s not found in events", eventType)
}

// doPost performs a POST request with JSON body.
func doPost(t *testing.T, url string, body interface{}) *http.Response {
	t.Helper()

	jsonBody, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("Failed to marshal request body: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to make POST request to %s: %v", url, err)
	}

	return resp
}

// TestE2EReportVerification tests that report fields are populated correctly after a run completes.
// This test verifies the full analysis pipeline: telemetry ingestion -> aggregation -> report generation.
func TestE2EReportVerification(t *testing.T) {
	// Setup: Create validator + RunManager + API server with telemetry and artifact stores
	validator := createTestValidator(t)
	rm := runmanager.NewRunManager(validator)

	// Create telemetry store and configure it
	telemetryStore := api.NewTelemetryStore()

	// Create artifact store in temp directory
	artifactDir := t.TempDir()
	artifactStore, err := createTestArtifactStore(t, artifactDir)
	if err != nil {
		t.Fatalf("Failed to create artifact store: %v", err)
	}

	// Configure RunManager with stores
	rm.SetTelemetryStore(telemetryStore)
	rm.SetArtifactStore(artifactStore)

	server, cleanup, err := api.StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	// Configure server's telemetry store for ingestion
	server.SetTelemetryStore(telemetryStore)

	baseURL := server.URL()
	t.Logf("Test server started at %s", baseURL)

	// Step 1: Create run
	runID := createRun(t, baseURL, validConfig)
	t.Logf("Created run: %s", runID)

	// Step 2: Start run
	startRun(t, baseURL, runID)
	t.Logf("Started run: %s", runID)

	// Step 3: Inject telemetry data directly into telemetry store
	now := time.Now().UnixMilli()
	injectTelemetryDirect(t, telemetryStore, runID, now)
	t.Logf("Injected telemetry data")

	// Step 4: Stop run to trigger analysis
	stopRun(t, baseURL, runID, "drain")
	t.Logf("Stopped run: %s", runID)

	// Step 5: Set run metadata for analysis
	telemetryStore.SetRunMetadata(runID, "e2e-test-scenario", "user_requested")

	// Step 6: Transition to analyzing state and trigger analysis
	err = rm.TransitionToAnalyzing(runID, "e2e-test")
	if err != nil {
		t.Fatalf("Failed to transition to analyzing: %v", err)
	}
	t.Logf("Transitioned to analyzing state")

	// Step 7: Wait for analysis to complete (check state)
	var finalState string
	for i := 0; i < 10; i++ {
		status := getRun(t, baseURL, runID)
		finalState = string(status.State)
		if finalState == "completed" || finalState == "failed" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Logf("Final state: %s", finalState)

	if finalState != "completed" {
		t.Fatalf("Expected run to complete, got state: %s", finalState)
	}

	// Step 8: Retrieve and parse JSON report
	reportData, err := artifactStore.GetArtifact(runID, "reports", "report.json")
	if err != nil {
		t.Fatalf("Failed to retrieve JSON report: %v", err)
	}
	t.Logf("Retrieved report (%d bytes)", len(reportData))

	var report reportJSON
	if err := json.Unmarshal(reportData, &report); err != nil {
		t.Fatalf("Failed to parse JSON report: %v", err)
	}

	// Step 9: Verify report fields
	verifyReportFields(t, report, runID)
	t.Logf("Report verification passed")
}

// reportJSON represents the JSON report structure for verification.
type reportJSON struct {
	RunID      string `json:"run_id"`
	ScenarioID string `json:"scenario_id"`
	StartTime  int64  `json:"start_time"`
	EndTime    int64  `json:"end_time"`
	Duration   int64  `json:"duration_ms"`
	StopReason string `json:"stop_reason"`
	Metrics    struct {
		TotalOps    int     `json:"total_ops"`
		SuccessOps  int     `json:"success_ops"`
		FailureOps  int     `json:"failure_ops"`
		RPS         float64 `json:"rps"`
		LatencyP50  int     `json:"latency_p50"`
		LatencyP95  int     `json:"latency_p95"`
		LatencyP99  int     `json:"latency_p99"`
		ErrorRate   float64 `json:"error_rate"`
		ByOperation map[string]struct {
			TotalOps   int     `json:"total_ops"`
			SuccessOps int     `json:"success_ops"`
			FailureOps int     `json:"failure_ops"`
			LatencyP50 int     `json:"latency_p50"`
			LatencyP95 int     `json:"latency_p95"`
			LatencyP99 int     `json:"latency_p99"`
			ErrorRate  float64 `json:"error_rate"`
		} `json:"by_operation"`
		ByTool         map[string]interface{} `json:"by_tool"`
		SessionMetrics *struct {
			SessionMode      string  `json:"session_mode"`
			TotalSessions    int     `json:"total_sessions"`
			OpsPerSession    float64 `json:"ops_per_session"`
			SessionReuseRate float64 `json:"session_reuse_rate"`
		} `json:"session_metrics,omitempty"`
	} `json:"metrics"`
}

// verifyReportFields validates all required report fields.
func verifyReportFields(t *testing.T, report reportJSON, expectedRunID string) {
	t.Helper()

	// Verify run_id matches
	if report.RunID != expectedRunID {
		t.Errorf("run_id mismatch: expected %s, got %s", expectedRunID, report.RunID)
	}

	// Verify scenario_id present
	if report.ScenarioID == "" {
		t.Error("scenario_id is empty")
	}
	if report.ScenarioID != "e2e-test-scenario" {
		t.Errorf("scenario_id mismatch: expected e2e-test-scenario, got %s", report.ScenarioID)
	}

	// Verify timestamps present
	if report.StartTime == 0 {
		t.Error("start_time is zero")
	}
	if report.EndTime == 0 {
		t.Error("end_time is zero")
	}
	if report.Duration <= 0 {
		t.Errorf("duration_ms should be positive, got %d", report.Duration)
	}

	// Verify stop_reason present
	if report.StopReason == "" {
		t.Error("stop_reason is empty")
	}
	if report.StopReason != "user_requested" {
		t.Errorf("stop_reason mismatch: expected user_requested, got %s", report.StopReason)
	}

	// Verify metrics exist and are reasonable
	if report.Metrics.TotalOps <= 0 {
		t.Errorf("total_ops should be positive, got %d", report.Metrics.TotalOps)
	}
	if report.Metrics.TotalOps != 10 {
		t.Errorf("total_ops mismatch: expected 10, got %d", report.Metrics.TotalOps)
	}

	// Verify success/failure counts
	if report.Metrics.SuccessOps != 8 {
		t.Errorf("success_ops mismatch: expected 8, got %d", report.Metrics.SuccessOps)
	}
	if report.Metrics.FailureOps != 2 {
		t.Errorf("failure_ops mismatch: expected 2, got %d", report.Metrics.FailureOps)
	}

	// Verify throughput (RPS) is positive
	if report.Metrics.RPS <= 0 {
		t.Errorf("rps should be positive, got %f", report.Metrics.RPS)
	}

	// Verify latency percentiles exist
	if report.Metrics.LatencyP50 <= 0 {
		t.Errorf("latency_p50 should be positive, got %d", report.Metrics.LatencyP50)
	}
	if report.Metrics.LatencyP95 <= 0 {
		t.Errorf("latency_p95 should be positive, got %d", report.Metrics.LatencyP95)
	}
	if report.Metrics.LatencyP99 <= 0 {
		t.Errorf("latency_p99 should be positive, got %d", report.Metrics.LatencyP99)
	}

	// Verify latency ordering: p50 <= p95 <= p99
	if report.Metrics.LatencyP50 > report.Metrics.LatencyP95 {
		t.Errorf("latency_p50 (%d) should be <= latency_p95 (%d)", report.Metrics.LatencyP50, report.Metrics.LatencyP95)
	}
	if report.Metrics.LatencyP95 > report.Metrics.LatencyP99 {
		t.Errorf("latency_p95 (%d) should be <= latency_p99 (%d)", report.Metrics.LatencyP95, report.Metrics.LatencyP99)
	}

	// Verify error_rate is in valid range (0-1 ratio)
	if report.Metrics.ErrorRate < 0 || report.Metrics.ErrorRate > 1 {
		t.Errorf("error_rate should be 0-1 (ratio), got %f", report.Metrics.ErrorRate)
	}
	// We injected 2 failures out of 10, so error rate should be 0.2 (20%)
	expectedErrorRate := 0.2
	if report.Metrics.ErrorRate != expectedErrorRate {
		t.Errorf("error_rate mismatch: expected %f, got %f", expectedErrorRate, report.Metrics.ErrorRate)
	}

	// Verify by_operation breakdown exists
	if len(report.Metrics.ByOperation) == 0 {
		t.Error("by_operation is empty")
	}

	if toolsList, ok := report.Metrics.ByOperation["tools/list"]; ok {
		if toolsList.TotalOps != 10 {
			t.Errorf("tools/list total_ops mismatch: expected 10, got %d", toolsList.TotalOps)
		}
	} else {
		t.Error("tools/list operation not found in by_operation")
	}

	t.Logf("Report verification complete: run_id=%s, scenario_id=%s, total_ops=%d, rps=%.2f, error_rate=%.2f%%",
		report.RunID, report.ScenarioID, report.Metrics.TotalOps, report.Metrics.RPS, report.Metrics.ErrorRate)
}

func injectTelemetryDirect(t *testing.T, store *api.TelemetryStore, runID string, startTimeMs int64) {
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

	store.AddTelemetryBatch(runID, batch)
}

// createTestArtifactStore creates a filesystem artifact store for testing.
func createTestArtifactStore(t *testing.T, dir string) (*artifacts.FilesystemStore, error) {
	t.Helper()
	return artifacts.NewFilesystemStore(dir)
}

// Suppress unused import warning
var _ = fmt.Sprintf
