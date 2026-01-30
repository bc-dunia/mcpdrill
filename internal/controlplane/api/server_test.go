package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/controlplane/runmanager"
	"github.com/bc-dunia/mcpdrill/internal/validation"
)

func loadValidConfig(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("../../../testdata/fixtures/valid/minimal_preflight_baseline_ramp.json")
	if err != nil {
		t.Fatalf("failed to load valid config: %v", err)
	}
	return data
}

func newTestRunManager(t *testing.T) *runmanager.RunManager {
	t.Helper()
	validator, err := validation.NewUnifiedValidator(nil)
	if err != nil {
		t.Fatalf("failed to create validator: %v", err)
	}
	return runmanager.NewRunManager(validator)
}

func TestCreateRun_Success(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	config := loadValidConfig(t)
	reqBody := CreateRunRequest{
		Config: config,
		Actor:  "test-user",
	}
	body, _ := json.Marshal(reqBody)

	resp, err := http.Post(server.URL()+"/runs", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status 201, got %d: %s", resp.StatusCode, string(respBody))
	}

	var result CreateRunResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result.RunID == "" {
		t.Error("expected non-empty run_id")
	}
	if !strings.HasPrefix(result.RunID, "run_") {
		t.Errorf("expected run_id to start with 'run_', got %s", result.RunID)
	}
}

func TestCreateRun_InvalidJSON(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	resp, err := http.Post(server.URL()+"/runs", "application/json", strings.NewReader("{invalid"))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", resp.StatusCode)
	}

	var errResp ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}

	if errResp.ErrorType != ErrorTypeInvalidArgument {
		t.Errorf("expected error_type %s, got %s", ErrorTypeInvalidArgument, errResp.ErrorType)
	}
}

func TestCreateRun_EmptyConfig(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	reqBody := CreateRunRequest{
		Config: nil,
		Actor:  "test-user",
	}
	body, _ := json.Marshal(reqBody)

	resp, err := http.Post(server.URL()+"/runs", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", resp.StatusCode)
	}
}

func TestCreateRun_ValidationFailure(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	invalidConfig := []byte(`{"schema_version": "run-config/v1"}`)
	reqBody := CreateRunRequest{
		Config: invalidConfig,
		Actor:  "test-user",
	}
	body, _ := json.Marshal(reqBody)

	resp, err := http.Post(server.URL()+"/runs", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", resp.StatusCode)
	}

	var errResp ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}

	if errResp.ErrorCode != ErrorCodeValidationFailed {
		t.Errorf("expected error_code %s, got %s", ErrorCodeValidationFailed, errResp.ErrorCode)
	}
}

func TestCreateRun_MethodNotAllowed(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	req, err := http.NewRequest(http.MethodPut, server.URL()+"/runs", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", resp.StatusCode)
	}
}

func TestListRuns_Success(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	resp, err := http.Get(server.URL() + "/runs")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var result ListRunsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if result.Runs == nil {
		t.Fatal("expected runs array, got nil")
	}
}

func TestValidateConfig_Success(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	config := loadValidConfig(t)
	reqBody := ValidateConfigRequest{
		Config: config,
	}
	body, _ := json.Marshal(reqBody)

	resp, err := http.Post(server.URL()+"/runs/any/validate", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status 200, got %d: %s", resp.StatusCode, string(respBody))
	}

	var result ValidateConfigResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !result.OK {
		t.Errorf("expected OK=true, got false with errors: %v", result.Errors)
	}
}

func TestValidateConfig_Invalid(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	invalidConfig := []byte(`{"schema_version": "run-config/v1"}`)
	reqBody := ValidateConfigRequest{
		Config: invalidConfig,
	}
	body, _ := json.Marshal(reqBody)

	resp, err := http.Post(server.URL()+"/runs/any/validate", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var result ValidateConfigResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result.OK {
		t.Error("expected OK=false for invalid config")
	}
	if len(result.Errors) == 0 {
		t.Error("expected validation errors")
	}
}

func TestStartRun_Success(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	config := loadValidConfig(t)
	runID, err := rm.CreateRun(config, "test")
	if err != nil {
		t.Fatalf("failed to create run: %v", err)
	}

	reqBody := StartRunRequest{Actor: "test-user"}
	body, _ := json.Marshal(reqBody)

	resp, err := http.Post(server.URL()+"/runs/"+runID+"/start", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status 200, got %d: %s", resp.StatusCode, string(respBody))
	}

	var result StartRunResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result.RunID != runID {
		t.Errorf("expected run_id %s, got %s", runID, result.RunID)
	}
	if result.State != string(runmanager.RunStatePreflightRunning) {
		t.Errorf("expected state %s, got %s", runmanager.RunStatePreflightRunning, result.State)
	}
}

func TestStartRun_NotFound(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	reqBody := StartRunRequest{Actor: "test-user"}
	body, _ := json.Marshal(reqBody)

	resp, err := http.Post(server.URL()+"/runs/nonexistent/start", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", resp.StatusCode)
	}

	var errResp ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}

	if errResp.ErrorCode != ErrorCodeRunNotFound {
		t.Errorf("expected error_code %s, got %s", ErrorCodeRunNotFound, errResp.ErrorCode)
	}
}

func TestStartRun_InvalidState(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	config := loadValidConfig(t)
	runID, _ := rm.CreateRun(config, "test")
	rm.StartRun(runID, "test")

	reqBody := StartRunRequest{Actor: "test-user"}
	body, _ := json.Marshal(reqBody)

	resp, err := http.Post(server.URL()+"/runs/"+runID+"/start", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected status 409, got %d", resp.StatusCode)
	}
}

func TestStopRun_Drain(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	config := loadValidConfig(t)
	runID, _ := rm.CreateRun(config, "test")
	rm.StartRun(runID, "test")

	reqBody := StopRunRequest{Mode: "drain", Actor: "test-user"}
	body, _ := json.Marshal(reqBody)

	resp, err := http.Post(server.URL()+"/runs/"+runID+"/stop", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status 200, got %d: %s", resp.StatusCode, string(respBody))
	}

	var result StopRunResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result.State != string(runmanager.RunStateStopping) {
		t.Errorf("expected state %s, got %s", runmanager.RunStateStopping, result.State)
	}
}

func TestStopRun_Immediate(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	config := loadValidConfig(t)
	runID, _ := rm.CreateRun(config, "test")
	rm.StartRun(runID, "test")

	reqBody := StopRunRequest{Mode: "immediate", Actor: "test-user"}
	body, _ := json.Marshal(reqBody)

	resp, err := http.Post(server.URL()+"/runs/"+runID+"/stop", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestStopRun_InvalidMode(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	config := loadValidConfig(t)
	runID, _ := rm.CreateRun(config, "test")
	rm.StartRun(runID, "test")

	reqBody := StopRunRequest{Mode: "invalid", Actor: "test-user"}
	body, _ := json.Marshal(reqBody)

	resp, err := http.Post(server.URL()+"/runs/"+runID+"/stop", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", resp.StatusCode)
	}

	var errResp ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}

	if errResp.ErrorCode != ErrorCodeInvalidStopMode {
		t.Errorf("expected error_code %s, got %s", ErrorCodeInvalidStopMode, errResp.ErrorCode)
	}
}

func TestStopRun_DefaultMode(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	config := loadValidConfig(t)
	runID, _ := rm.CreateRun(config, "test")
	rm.StartRun(runID, "test")

	reqBody := StopRunRequest{Actor: "test-user"}
	body, _ := json.Marshal(reqBody)

	resp, err := http.Post(server.URL()+"/runs/"+runID+"/stop", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestEmergencyStop_Success(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	config := loadValidConfig(t)
	runID, _ := rm.CreateRun(config, "test")
	rm.StartRun(runID, "test")

	reqBody := EmergencyStopRequest{Actor: "test-user"}
	body, _ := json.Marshal(reqBody)

	resp, err := http.Post(server.URL()+"/runs/"+runID+"/emergency-stop", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status 200, got %d: %s", resp.StatusCode, string(respBody))
	}

	var result EmergencyStopResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result.State != string(runmanager.RunStateStopping) {
		t.Errorf("expected state %s, got %s", runmanager.RunStateStopping, result.State)
	}
}

func TestEmergencyStop_NotFound(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	reqBody := EmergencyStopRequest{Actor: "test-user"}
	body, _ := json.Marshal(reqBody)

	resp, err := http.Post(server.URL()+"/runs/nonexistent/emergency-stop", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", resp.StatusCode)
	}
}

func TestGetRun_Success(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	config := loadValidConfig(t)
	runID, _ := rm.CreateRun(config, "test")

	resp, err := http.Get(server.URL() + "/runs/" + runID)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status 200, got %d: %s", resp.StatusCode, string(respBody))
	}

	var result GetRunResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result.RunID != runID {
		t.Errorf("expected run_id %s, got %s", runID, result.RunID)
	}
	if result.State != runmanager.RunStateCreated {
		t.Errorf("expected state %s, got %s", runmanager.RunStateCreated, result.State)
	}
}

func TestGetRun_NotFound(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	resp, err := http.Get(server.URL() + "/runs/nonexistent")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", resp.StatusCode)
	}

	var errResp ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}

	if errResp.ErrorCode != ErrorCodeRunNotFound {
		t.Errorf("expected error_code %s, got %s", ErrorCodeRunNotFound, errResp.ErrorCode)
	}
}

func TestHealthz(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	resp, err := http.Get(server.URL() + "/healthz")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var result HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result.Status != "ok" {
		t.Errorf("expected status 'ok', got %s", result.Status)
	}
}

func TestReadyz(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	resp, err := http.Get(server.URL() + "/readyz")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var result ReadyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !result.Ready {
		t.Error("expected ready=true")
	}
}

func TestHealthz_MethodNotAllowed(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	resp, err := http.Post(server.URL()+"/healthz", "application/json", nil)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", resp.StatusCode)
	}
}

func TestServerLifecycle(t *testing.T) {
	rm := newTestRunManager(t)
	server := NewServer("127.0.0.1:0", rm)

	if server.IsRunning() {
		t.Error("server should not be running before Start")
	}

	if err := server.Start(); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}

	if !server.IsRunning() {
		t.Error("server should be running after Start")
	}

	if err := server.Start(); err == nil {
		t.Error("expected error when starting already running server")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		t.Fatalf("failed to shutdown server: %v", err)
	}

	if server.IsRunning() {
		t.Error("server should not be running after Shutdown")
	}

	if err := server.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown on stopped server should not error: %v", err)
	}
}

func TestServerAddr(t *testing.T) {
	rm := newTestRunManager(t)
	server := NewServer("127.0.0.1:0", rm)

	if server.Addr() != "127.0.0.1:0" {
		t.Errorf("expected addr 127.0.0.1:0 before start, got %s", server.Addr())
	}

	if err := server.Start(); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(ctx)
	}()

	addr := server.Addr()
	if !strings.HasPrefix(addr, "127.0.0.1:") {
		t.Errorf("expected addr to start with 127.0.0.1:, got %s", addr)
	}
	if addr == "127.0.0.1:0" {
		t.Error("expected addr to have assigned port, got :0")
	}
}

func TestConcurrentRequests(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	config := loadValidConfig(t)

	var wg sync.WaitGroup
	errors := make(chan error, 50)

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			reqBody := CreateRunRequest{
				Config: config,
				Actor:  "test-user",
			}
			body, _ := json.Marshal(reqBody)

			resp, err := http.Post(server.URL()+"/runs", "application/json", bytes.NewReader(body))
			if err != nil {
				errors <- err
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusCreated {
				respBody, _ := io.ReadAll(resp.Body)
				errors <- &testError{msg: string(respBody)}
				return
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent request failed: %v", err)
	}
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

func TestEndpointNotFound(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	resp, err := http.Get(server.URL() + "/runs/some-id/unknown-action")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", resp.StatusCode)
	}
}

func TestEmptyBody(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	config := loadValidConfig(t)
	runID, _ := rm.CreateRun(config, "test")

	resp, err := http.Post(server.URL()+"/runs/"+runID+"/start", "application/json", nil)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status 200, got %d: %s", resp.StatusCode, string(respBody))
	}
}

func TestDefaultActor(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	config := loadValidConfig(t)
	reqBody := CreateRunRequest{
		Config: config,
	}
	body, _ := json.Marshal(reqBody)

	resp, err := http.Post(server.URL()+"/runs", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", resp.StatusCode)
	}
}

func TestHTTPTestRecorder(t *testing.T) {
	rm := newTestRunManager(t)
	server := NewServer("127.0.0.1:0", rm)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", server.handleHealthz)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var result HealthResponse
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result.Status != "ok" {
		t.Errorf("expected status 'ok', got %s", result.Status)
	}
}

func TestValidateConfig_EmptyConfig(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	reqBody := ValidateConfigRequest{
		Config: nil,
	}
	body, _ := json.Marshal(reqBody)

	resp, err := http.Post(server.URL()+"/runs/any/validate", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var result ValidateConfigResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result.OK {
		t.Error("expected OK=false for empty config")
	}
}

func TestValidateConfig_MethodNotAllowed(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	resp, err := http.Get(server.URL() + "/runs/any/validate")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", resp.StatusCode)
	}
}

func TestStopRun_NotFound(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	reqBody := StopRunRequest{Mode: "drain", Actor: "test-user"}
	body, _ := json.Marshal(reqBody)

	resp, err := http.Post(server.URL()+"/runs/nonexistent/stop", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", resp.StatusCode)
	}
}

func TestGetRun_MethodNotAllowed(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	config := loadValidConfig(t)
	runID, _ := rm.CreateRun(config, "test")

	resp, err := http.Post(server.URL()+"/runs/"+runID, "application/json", nil)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", resp.StatusCode)
	}
}

func TestStartRun_MethodNotAllowed(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	config := loadValidConfig(t)
	runID, _ := rm.CreateRun(config, "test")

	resp, err := http.Get(server.URL() + "/runs/" + runID + "/start")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", resp.StatusCode)
	}
}

func TestStopRun_MethodNotAllowed(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	config := loadValidConfig(t)
	runID, _ := rm.CreateRun(config, "test")

	resp, err := http.Get(server.URL() + "/runs/" + runID + "/stop")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", resp.StatusCode)
	}
}

func TestEmergencyStop_MethodNotAllowed(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	config := loadValidConfig(t)
	runID, _ := rm.CreateRun(config, "test")

	resp, err := http.Get(server.URL() + "/runs/" + runID + "/emergency-stop")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", resp.StatusCode)
	}
}

func TestReadyz_MethodNotAllowed(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	resp, err := http.Post(server.URL()+"/readyz", "application/json", nil)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", resp.StatusCode)
	}
}

func TestExtractState(t *testing.T) {
	tests := []struct {
		errMsg   string
		expected string
	}{
		// Lowercase state strings (actual RunState values)
		{"cannot start run in state created", "created"},
		{"run in state preflight_running", "preflight_running"},
		{"state preflight_passed", "preflight_passed"},
		{"state preflight_failed", "preflight_failed"},
		{"state baseline_running", "baseline_running"},
		{"state ramp_running", "ramp_running"},
		{"state soak_running", "soak_running"},
		{"state stopping", "stopping"},
		{"state analyzing", "analyzing"},
		{"state completed", "completed"},
		{"state failed", "failed"},
		{"state aborted", "aborted"},
		// Uppercase state strings (legacy, should be normalized to lowercase)
		{"cannot start run in state CREATED", "created"},
		{"run in state PREFLIGHT_RUNNING", "preflight_running"},
		{"state STOPPING", "stopping"},
		{"some other error", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.errMsg, func(t *testing.T) {
			result := extractState(tt.errMsg)
			if result != tt.expected {
				t.Errorf("extractState(%q) = %q, want %q", tt.errMsg, result, tt.expected)
			}
		})
	}
}

func TestValidateConfig_InvalidJSON(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	resp, err := http.Post(server.URL()+"/runs/any/validate", "application/json", strings.NewReader("{invalid"))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", resp.StatusCode)
	}
}

func TestStartRun_InvalidJSON(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	config := loadValidConfig(t)
	runID, _ := rm.CreateRun(config, "test")

	resp, err := http.Post(server.URL()+"/runs/"+runID+"/start", "application/json", strings.NewReader("{invalid"))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", resp.StatusCode)
	}
}

func TestStopRun_InvalidJSON(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	config := loadValidConfig(t)
	runID, _ := rm.CreateRun(config, "test")

	resp, err := http.Post(server.URL()+"/runs/"+runID+"/stop", "application/json", strings.NewReader("{invalid"))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", resp.StatusCode)
	}
}

func TestEmergencyStop_InvalidJSON(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	config := loadValidConfig(t)
	runID, _ := rm.CreateRun(config, "test")

	resp, err := http.Post(server.URL()+"/runs/"+runID+"/emergency-stop", "application/json", strings.NewReader("{invalid"))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", resp.StatusCode)
	}
}
