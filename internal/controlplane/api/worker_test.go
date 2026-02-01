package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/bc-dunia/mcpdrill/internal/controlplane/scheduler"
	"github.com/bc-dunia/mcpdrill/internal/types"
)

func setupWorkerTestServer(t *testing.T) (*Server, *scheduler.Registry) {
	t.Helper()
	registry := scheduler.NewRegistry()
	server := NewServer("127.0.0.1:0", nil)
	server.SetRegistry(registry)
	server.SetWorkerAuthEnabled(false)
	return server, registry
}

func registerWorkerWithToken(t *testing.T, server *Server, registry *scheduler.Registry, hostname string) (scheduler.WorkerID, string) {
	t.Helper()
	workerID, err := registry.RegisterWorker(
		types.HostInfo{Hostname: hostname},
		types.WorkerCapacity{MaxVUs: 100},
	)
	if err != nil {
		t.Fatalf("failed to register worker: %v", err)
	}
	token, err := server.issueWorkerToken(string(workerID))
	if err != nil {
		t.Fatalf("failed to issue worker token: %v", err)
	}
	return workerID, token
}

func TestRegisterWorker_Success(t *testing.T) {
	server, _ := setupWorkerTestServer(t)

	req := RegisterWorkerRequest{
		HostInfo: types.HostInfo{
			Hostname: "worker-1",
			IPAddr:   "192.168.1.100",
			Platform: "linux",
		},
		Capacity: types.WorkerCapacity{
			MaxVUs:           100,
			MaxConcurrentOps: 50,
			MaxRPS:           1000.0,
		},
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/workers/register", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleRegisterWorker(w, httpReq)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d", http.StatusCreated, w.Code)
	}

	var resp RegisterWorkerResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.WorkerID == "" {
		t.Error("expected non-empty worker_id")
	}
	if resp.WorkerToken == "" {
		t.Error("expected non-empty worker_token")
	}
}

func TestRegisterWorker_InvalidJSON(t *testing.T) {
	server, _ := setupWorkerTestServer(t)

	httpReq := httptest.NewRequest(http.MethodPost, "/workers/register", bytes.NewReader([]byte("invalid json")))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleRegisterWorker(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	var errResp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}

	if errResp.ErrorCode != ErrorCodeInvalidRequest {
		t.Errorf("expected error code %s, got %s", ErrorCodeInvalidRequest, errResp.ErrorCode)
	}
}

func TestRegisterWorker_MissingHostname(t *testing.T) {
	server, _ := setupWorkerTestServer(t)

	req := RegisterWorkerRequest{
		HostInfo: types.HostInfo{
			IPAddr:   "192.168.1.100",
			Platform: "linux",
		},
		Capacity: types.WorkerCapacity{
			MaxVUs: 100,
		},
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/workers/register", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleRegisterWorker(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestRegisterWorker_InvalidCapacity(t *testing.T) {
	server, _ := setupWorkerTestServer(t)

	req := RegisterWorkerRequest{
		HostInfo: types.HostInfo{
			Hostname: "worker-1",
		},
		Capacity: types.WorkerCapacity{
			MaxVUs: 0,
		},
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/workers/register", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleRegisterWorker(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestRegisterWorker_MethodNotAllowed(t *testing.T) {
	server, _ := setupWorkerTestServer(t)

	httpReq := httptest.NewRequest(http.MethodGet, "/workers/register", nil)
	w := httptest.NewRecorder()

	server.handleRegisterWorker(w, httpReq)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestRegisterWorker_NoRegistry(t *testing.T) {
	server := NewServer("127.0.0.1:0", nil)

	req := RegisterWorkerRequest{
		HostInfo: types.HostInfo{Hostname: "worker-1"},
		Capacity: types.WorkerCapacity{MaxVUs: 100},
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/workers/register", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleRegisterWorker(w, httpReq)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}
}

func TestHeartbeat_Success(t *testing.T) {
	server, registry := setupWorkerTestServer(t)

	workerID, token := registerWorkerWithToken(t, server, registry, "worker-1")

	req := HeartbeatRequest{
		Health: &types.WorkerHealth{
			CPUPercent: 50.0,
			MemBytes:   1024 * 1024 * 512,
			ActiveVUs:  10,
		},
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/workers/"+string(workerID)+"/heartbeat", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Worker-Token", token)
	w := httptest.NewRecorder()

	server.handleWorkerHeartbeat(w, httpReq, string(workerID))

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp HeartbeatResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !resp.OK {
		t.Error("expected OK to be true")
	}
}

func TestHeartbeat_EmptyBody(t *testing.T) {
	server, registry := setupWorkerTestServer(t)

	workerID, token := registerWorkerWithToken(t, server, registry, "worker-1")

	httpReq := httptest.NewRequest(http.MethodPost, "/workers/"+string(workerID)+"/heartbeat", nil)
	httpReq.Header.Set("X-Worker-Token", token)
	w := httptest.NewRecorder()

	server.handleWorkerHeartbeat(w, httpReq, string(workerID))

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestHeartbeat_WorkerNotFound(t *testing.T) {
	server, _ := setupWorkerTestServer(t)

	req := HeartbeatRequest{}
	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/workers/nonexistent/heartbeat", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleWorkerHeartbeat(w, httpReq, "nonexistent")

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}

	var errResp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}

	if errResp.ErrorCode != ErrorCodeWorkerNotFound {
		t.Errorf("expected error code %s, got %s", ErrorCodeWorkerNotFound, errResp.ErrorCode)
	}
}

func TestHeartbeat_InvalidJSON(t *testing.T) {
	server, registry := setupWorkerTestServer(t)

	workerID, token := registerWorkerWithToken(t, server, registry, "worker-1")

	httpReq := httptest.NewRequest(http.MethodPost, "/workers/"+string(workerID)+"/heartbeat", bytes.NewReader([]byte("invalid")))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Worker-Token", token)
	w := httptest.NewRecorder()

	server.handleWorkerHeartbeat(w, httpReq, string(workerID))

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestHeartbeat_MethodNotAllowed(t *testing.T) {
	server, _ := setupWorkerTestServer(t)

	httpReq := httptest.NewRequest(http.MethodGet, "/workers/worker-1/heartbeat", nil)
	w := httptest.NewRecorder()

	server.handleWorkerHeartbeat(w, httpReq, "worker-1")

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestHeartbeat_NoRegistry(t *testing.T) {
	server := NewServer("127.0.0.1:0", nil)
	server.SetWorkerAuthEnabled(false)

	httpReq := httptest.NewRequest(http.MethodPost, "/workers/worker-1/heartbeat", nil)
	w := httptest.NewRecorder()

	server.handleWorkerHeartbeat(w, httpReq, "worker-1")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}
}

func TestTelemetry_Success(t *testing.T) {
	server, registry := setupWorkerTestServer(t)

	workerID, token := registerWorkerWithToken(t, server, registry, "worker-1")

	req := TelemetryBatchRequest{
		RunID: "run_0000000000000001",
		Operations: []types.OperationOutcome{
			{
				OpID:        "op-1",
				Operation:   "tools_call",
				ToolName:    "echo",
				LatencyMs:   50,
				OK:          true,
				TimestampMs: 1234567890,
				ExecutionID: "exe_00000000000001",
				Stage:       "preflight",
				StageID:     "stg_000000000001",
			},
			{
				OpID:        "op-2",
				Operation:   "tools_call",
				ToolName:    "fetch",
				LatencyMs:   100,
				OK:          false,
				ErrorType:   "timeout",
				ErrorCode:   "READ_TIMEOUT",
				TimestampMs: 1234567891,
				ExecutionID: "exe_00000000000001",
				Stage:       "preflight",
				StageID:     "stg_000000000001",
			},
		},
		Health: &types.WorkerHealth{
			CPUPercent: 60.0,
			MemBytes:   1024 * 1024 * 768,
		},
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/workers/"+string(workerID)+"/telemetry", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Worker-Token", token)
	w := httptest.NewRecorder()

	server.handleWorkerTelemetry(w, httpReq, string(workerID))

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp TelemetryBatchResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Accepted != 2 {
		t.Errorf("expected 2 accepted, got %d", resp.Accepted)
	}
}

func TestTelemetry_WorkerNotFound(t *testing.T) {
	server, _ := setupWorkerTestServer(t)

	req := TelemetryBatchRequest{
		RunID:      "run_0000000000000001",
		Operations: []types.OperationOutcome{},
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/workers/nonexistent/telemetry", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleWorkerTelemetry(w, httpReq, "nonexistent")

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestTelemetry_InvalidJSON(t *testing.T) {
	server, registry := setupWorkerTestServer(t)

	workerID, token := registerWorkerWithToken(t, server, registry, "worker-1")

	httpReq := httptest.NewRequest(http.MethodPost, "/workers/"+string(workerID)+"/telemetry", bytes.NewReader([]byte("invalid")))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Worker-Token", token)
	w := httptest.NewRecorder()

	server.handleWorkerTelemetry(w, httpReq, string(workerID))

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestTelemetry_MethodNotAllowed(t *testing.T) {
	server, _ := setupWorkerTestServer(t)

	httpReq := httptest.NewRequest(http.MethodGet, "/workers/worker-1/telemetry", nil)
	w := httptest.NewRecorder()

	server.handleWorkerTelemetry(w, httpReq, "worker-1")

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestTelemetry_NoRegistry(t *testing.T) {
	server := NewServer("127.0.0.1:0", nil)
	server.SetWorkerAuthEnabled(false)

	req := TelemetryBatchRequest{RunID: "run_0000000000000001", Operations: []types.OperationOutcome{}}
	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/workers/worker-1/telemetry", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleWorkerTelemetry(w, httpReq, "worker-1")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}
}

func TestRouteWorkers_Register(t *testing.T) {
	server, _ := setupWorkerTestServer(t)

	req := RegisterWorkerRequest{
		HostInfo: types.HostInfo{Hostname: "worker-1"},
		Capacity: types.WorkerCapacity{MaxVUs: 100},
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/workers/register", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.routeWorkers(w, httpReq)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d", http.StatusCreated, w.Code)
	}
}

func TestRouteWorkers_Heartbeat(t *testing.T) {
	server, registry := setupWorkerTestServer(t)

	workerID, token := registerWorkerWithToken(t, server, registry, "worker-1")

	httpReq := httptest.NewRequest(http.MethodPost, "/workers/"+string(workerID)+"/heartbeat", nil)
	httpReq.Header.Set("X-Worker-Token", token)
	w := httptest.NewRecorder()

	server.routeWorkers(w, httpReq)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestRouteWorkers_Telemetry(t *testing.T) {
	server, registry := setupWorkerTestServer(t)

	workerID, token := registerWorkerWithToken(t, server, registry, "worker-1")

	req := TelemetryBatchRequest{RunID: "run_0000000000000001", Operations: []types.OperationOutcome{}}
	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/workers/"+string(workerID)+"/telemetry", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Worker-Token", token)
	w := httptest.NewRecorder()

	server.routeWorkers(w, httpReq)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestRouteWorkers_UnknownAction(t *testing.T) {
	server, _ := setupWorkerTestServer(t)

	httpReq := httptest.NewRequest(http.MethodPost, "/workers/worker-1/unknown", nil)
	w := httptest.NewRecorder()

	server.routeWorkers(w, httpReq)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestRouteWorkers_WorkerIDOnly(t *testing.T) {
	server, _ := setupWorkerTestServer(t)

	httpReq := httptest.NewRequest(http.MethodGet, "/workers/worker-1", nil)
	w := httptest.NewRecorder()

	server.routeWorkers(w, httpReq)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestConcurrentWorkerOperations(t *testing.T) {
	server, registry := setupWorkerTestServer(t)

	var wg sync.WaitGroup
	workerCount := 10
	workerIDs := make([]scheduler.WorkerID, workerCount)
	workerTokens := make([]string, workerCount)

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			req := RegisterWorkerRequest{
				HostInfo: types.HostInfo{Hostname: "worker-" + string(rune('a'+idx))},
				Capacity: types.WorkerCapacity{MaxVUs: 100},
			}

			body, _ := json.Marshal(req)
			httpReq := httptest.NewRequest(http.MethodPost, "/workers/register", bytes.NewReader(body))
			httpReq.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			server.handleRegisterWorker(w, httpReq)

			if w.Code != http.StatusCreated {
				t.Errorf("worker %d: expected status %d, got %d", idx, http.StatusCreated, w.Code)
				return
			}

			var resp RegisterWorkerResponse
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
				t.Errorf("worker %d: failed to decode response: %v", idx, err)
				return
			}

			workerIDs[idx] = scheduler.WorkerID(resp.WorkerID)
			workerTokens[idx] = resp.WorkerToken
		}(i)
	}

	wg.Wait()

	if registry.WorkerCount() != workerCount {
		t.Errorf("expected %d workers, got %d", workerCount, registry.WorkerCount())
	}

	for i := 0; i < workerCount; i++ {
		if workerIDs[i] == "" {
			continue
		}
		token := workerTokens[i]

		wg.Add(1)
		go func(workerID scheduler.WorkerID, token string) {
			defer wg.Done()

			for j := 0; j < 5; j++ {
				req := HeartbeatRequest{
					Health: &types.WorkerHealth{CPUPercent: float64(j * 10)},
				}

				body, _ := json.Marshal(req)
				httpReq := httptest.NewRequest(http.MethodPost, "/workers/"+string(workerID)+"/heartbeat", bytes.NewReader(body))
				httpReq.Header.Set("Content-Type", "application/json")
				httpReq.Header.Set("X-Worker-Token", token)
				w := httptest.NewRecorder()

				server.handleWorkerHeartbeat(w, httpReq, string(workerID))

				if w.Code != http.StatusOK {
					t.Errorf("heartbeat for %s: expected status %d, got %d", workerID, http.StatusOK, w.Code)
				}
			}
		}(workerIDs[i], token)
	}

	wg.Wait()
}

func TestTelemetry_UpdatesHealth(t *testing.T) {
	server, registry := setupWorkerTestServer(t)

	workerID, token := registerWorkerWithToken(t, server, registry, "worker-1")

	req := TelemetryBatchRequest{
		RunID: "run_0000000000000001",
		Operations: []types.OperationOutcome{
			{OpID: "op-1", Operation: "ping", OK: true, LatencyMs: 10, TimestampMs: 1234567890, ExecutionID: "exe_00000000000001", Stage: "preflight", StageID: "stg_000000000001"},
		},
		Health: &types.WorkerHealth{
			CPUPercent: 75.5,
			MemBytes:   1024 * 1024 * 1024,
			ActiveVUs:  50,
		},
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/workers/"+string(workerID)+"/telemetry", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Worker-Token", token)
	w := httptest.NewRecorder()

	server.handleWorkerTelemetry(w, httpReq, string(workerID))

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	worker, err := registry.GetWorker(workerID)
	if err != nil {
		t.Fatalf("failed to get worker: %v", err)
	}

	if worker.Health == nil {
		t.Fatal("expected health to be updated")
	}

	if worker.Health.CPUPercent != 75.5 {
		t.Errorf("expected CPU percent 75.5, got %f", worker.Health.CPUPercent)
	}
}
