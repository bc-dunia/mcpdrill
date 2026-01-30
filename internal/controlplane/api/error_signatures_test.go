package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bc-dunia/mcpdrill/internal/types"
)

func TestHandleGetErrorSignatures_WithErrors(t *testing.T) {
	ts := NewTelemetryStore()
	runID := "run_0000000000000123"

	batch := TelemetryBatchRequest{
		RunID: runID,
		Operations: []types.OperationOutcome{
			{TimestampMs: 1000, Operation: "tools/call", ToolName: "api_client", OK: false, ErrorType: "connection refused to localhost:3000"},
			{TimestampMs: 2000, Operation: "tools/call", ToolName: "api_client", OK: false, ErrorType: "connection refused to localhost:3001"},
			{TimestampMs: 3000, Operation: "resources/read", ToolName: "file_reader", OK: false, ErrorType: "connection refused to localhost:8080"},
			{TimestampMs: 4000, Operation: "tools/call", ToolName: "api_client", OK: true},
			{TimestampMs: 5000, Operation: "tools/call", ToolName: "api_client", OK: false, ErrorType: "timeout after 5000ms"},
		},
	}
	ts.AddTelemetryBatch(runID, batch)

	server := &Server{telemetryStore: ts}

	req := httptest.NewRequest(http.MethodGet, "/runs/"+runID+"/errors/signatures", nil)
	w := httptest.NewRecorder()

	server.handleGetErrorSignatures(w, req, runID)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp ErrorSignaturesResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.RunID != runID {
		t.Errorf("RunID = %q, want %q", resp.RunID, runID)
	}

	if len(resp.Signatures) != 2 {
		t.Fatalf("Expected 2 signatures, got %d", len(resp.Signatures))
	}

	if resp.Signatures[0].Pattern != "connection refused to localhost:<NUM>" {
		t.Errorf("First signature pattern = %q, want 'connection refused to localhost:<NUM>'", resp.Signatures[0].Pattern)
	}
	if resp.Signatures[0].Count != 3 {
		t.Errorf("First signature count = %d, want 3", resp.Signatures[0].Count)
	}

	if resp.Signatures[1].Pattern != "timeout after <NUM>ms" {
		t.Errorf("Second signature pattern = %q, want 'timeout after <NUM>ms'", resp.Signatures[1].Pattern)
	}
	if resp.Signatures[1].Count != 1 {
		t.Errorf("Second signature count = %d, want 1", resp.Signatures[1].Count)
	}
}

func TestHandleGetErrorSignatures_NoErrors(t *testing.T) {
	ts := NewTelemetryStore()
	runID := "run_0000000000000123"

	batch := TelemetryBatchRequest{
		RunID: runID,
		Operations: []types.OperationOutcome{
			{TimestampMs: 1000, Operation: "tools/call", ToolName: "api_client", OK: true},
			{TimestampMs: 2000, Operation: "tools/call", ToolName: "api_client", OK: true},
		},
	}
	ts.AddTelemetryBatch(runID, batch)

	server := &Server{telemetryStore: ts}

	req := httptest.NewRequest(http.MethodGet, "/runs/"+runID+"/errors/signatures", nil)
	w := httptest.NewRecorder()

	server.handleGetErrorSignatures(w, req, runID)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp ErrorSignaturesResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(resp.Signatures) != 0 {
		t.Errorf("Expected 0 signatures, got %d", len(resp.Signatures))
	}
}

func TestHandleGetErrorSignatures_RunNotFound(t *testing.T) {
	ts := NewTelemetryStore()
	server := &Server{telemetryStore: ts}

	req := httptest.NewRequest(http.MethodGet, "/runs/nonexistent/errors/signatures", nil)
	w := httptest.NewRecorder()

	server.handleGetErrorSignatures(w, req, "nonexistent")

	if w.Code != http.StatusNotFound {
		t.Fatalf("Expected status 404, got %d: %s", w.Code, w.Body.String())
	}

	var errResp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if errResp.ErrorCode != ErrorCodeRunNotFound {
		t.Errorf("ErrorCode = %q, want %q", errResp.ErrorCode, ErrorCodeRunNotFound)
	}
}

func TestHandleGetErrorSignatures_Top10Limit(t *testing.T) {
	ts := NewTelemetryStore()
	runID := "run_0000000000000123"

	ops := make([]types.OperationOutcome, 0, 15)
	for i := 0; i < 15; i++ {
		ops = append(ops, types.OperationOutcome{
			TimestampMs: int64(1000 + i*1000),
			Operation:   "tools/call",
			OK:          false,
			ErrorType:   "unique error " + string(rune('A'+i)),
		})
	}

	batch := TelemetryBatchRequest{RunID: runID, Operations: ops}
	ts.AddTelemetryBatch(runID, batch)

	server := &Server{telemetryStore: ts}

	req := httptest.NewRequest(http.MethodGet, "/runs/"+runID+"/errors/signatures", nil)
	w := httptest.NewRecorder()

	server.handleGetErrorSignatures(w, req, runID)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp ErrorSignaturesResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(resp.Signatures) != 10 {
		t.Errorf("Expected 10 signatures (max), got %d", len(resp.Signatures))
	}
}

func TestHandleGetErrorSignatures_MethodNotAllowed(t *testing.T) {
	ts := NewTelemetryStore()
	runID := "run_0000000000000123"
	ts.AddTelemetryBatch(runID, TelemetryBatchRequest{RunID: runID})

	server := &Server{telemetryStore: ts}

	req := httptest.NewRequest(http.MethodPost, "/runs/"+runID+"/errors/signatures", nil)
	w := httptest.NewRecorder()

	server.handleGetErrorSignatures(w, req, runID)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("Expected status 405, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleGetErrorSignatures_NoTelemetryStore(t *testing.T) {
	server := &Server{telemetryStore: nil}

	req := httptest.NewRequest(http.MethodGet, "/runs/run_0000000000000123/errors/signatures", nil)
	w := httptest.NewRecorder()

	server.handleGetErrorSignatures(w, req, "run_0000000000000123")

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("Expected status 503, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleGetErrorSignatures_AffectedOperationsAndTools(t *testing.T) {
	ts := NewTelemetryStore()
	runID := "run_0000000000000123"

	batch := TelemetryBatchRequest{
		RunID: runID,
		Operations: []types.OperationOutcome{
			{TimestampMs: 1000, Operation: "tools/call", ToolName: "api_client", OK: false, ErrorType: "connection refused to localhost:3000"},
			{TimestampMs: 2000, Operation: "resources/read", ToolName: "file_reader", OK: false, ErrorType: "connection refused to localhost:3001"},
			{TimestampMs: 3000, Operation: "tools/list", ToolName: "", OK: false, ErrorType: "connection refused to localhost:8080"},
		},
	}
	ts.AddTelemetryBatch(runID, batch)

	server := &Server{telemetryStore: ts}

	req := httptest.NewRequest(http.MethodGet, "/runs/"+runID+"/errors/signatures", nil)
	w := httptest.NewRecorder()

	server.handleGetErrorSignatures(w, req, runID)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp ErrorSignaturesResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(resp.Signatures) != 1 {
		t.Fatalf("Expected 1 signature, got %d", len(resp.Signatures))
	}

	sig := resp.Signatures[0]
	if len(sig.AffectedOperations) != 3 {
		t.Errorf("Expected 3 affected operations, got %d: %v", len(sig.AffectedOperations), sig.AffectedOperations)
	}
	if len(sig.AffectedTools) != 2 {
		t.Errorf("Expected 2 affected tools, got %d: %v", len(sig.AffectedTools), sig.AffectedTools)
	}
}

func TestHandleGetErrorSignatures_TimestampTracking(t *testing.T) {
	ts := NewTelemetryStore()
	runID := "run_0000000000000123"

	batch := TelemetryBatchRequest{
		RunID: runID,
		Operations: []types.OperationOutcome{
			{TimestampMs: 5000, Operation: "tools/call", OK: false, ErrorType: "error A"},
			{TimestampMs: 1000, Operation: "tools/call", OK: false, ErrorType: "error A"},
			{TimestampMs: 3000, Operation: "tools/call", OK: false, ErrorType: "error A"},
		},
	}
	ts.AddTelemetryBatch(runID, batch)

	server := &Server{telemetryStore: ts}

	req := httptest.NewRequest(http.MethodGet, "/runs/"+runID+"/errors/signatures", nil)
	w := httptest.NewRecorder()

	server.handleGetErrorSignatures(w, req, runID)

	var resp ErrorSignaturesResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	sig := resp.Signatures[0]
	if sig.FirstSeenMs != 1000 {
		t.Errorf("FirstSeenMs = %d, want 1000", sig.FirstSeenMs)
	}
	if sig.LastSeenMs != 5000 {
		t.Errorf("LastSeenMs = %d, want 5000", sig.LastSeenMs)
	}
}

func TestGetErrorLogs(t *testing.T) {
	ts := NewTelemetryStore()
	runID := "run_0000000000000123"

	batch := TelemetryBatchRequest{
		RunID: runID,
		Operations: []types.OperationOutcome{
			{TimestampMs: 1000, Operation: "tools/call", ToolName: "api_client", OK: true},
			{TimestampMs: 2000, Operation: "tools/call", ToolName: "api_client", OK: false, ErrorType: "error 1"},
			{TimestampMs: 3000, Operation: "resources/read", ToolName: "file_reader", OK: false, ErrorType: "error 2"},
			{TimestampMs: 4000, Operation: "tools/list", OK: true},
		},
	}
	ts.AddTelemetryBatch(runID, batch)

	errorLogs, err := ts.GetErrorLogs(runID)
	if err != nil {
		t.Fatalf("GetErrorLogs failed: %v", err)
	}

	if len(errorLogs) != 2 {
		t.Fatalf("Expected 2 error logs, got %d", len(errorLogs))
	}

	if errorLogs[0].ErrorType != "error 1" {
		t.Errorf("First error = %q, want 'error 1'", errorLogs[0].ErrorType)
	}
	if errorLogs[1].ErrorType != "error 2" {
		t.Errorf("Second error = %q, want 'error 2'", errorLogs[1].ErrorType)
	}
}

func TestGetErrorLogs_RunNotFound(t *testing.T) {
	ts := NewTelemetryStore()

	_, err := ts.GetErrorLogs("nonexistent")
	if err == nil {
		t.Fatal("Expected error for nonexistent run")
	}
}

func TestRouteRuns_ErrorSignatures(t *testing.T) {
	ts := NewTelemetryStore()
	runID := "run_0000000000000123"

	batch := TelemetryBatchRequest{
		RunID: runID,
		Operations: []types.OperationOutcome{
			{TimestampMs: 1000, Operation: "tools/call", OK: false, ErrorType: "test error"},
		},
	}
	ts.AddTelemetryBatch(runID, batch)

	server := &Server{telemetryStore: ts}

	req := httptest.NewRequest(http.MethodGet, "/runs/"+runID+"/errors/signatures", nil)
	w := httptest.NewRecorder()

	server.routeRuns(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp ErrorSignaturesResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(resp.Signatures) != 1 {
		t.Errorf("Expected 1 signature, got %d", len(resp.Signatures))
	}
}

func TestRouteRuns_ErrorsWithoutSignatures(t *testing.T) {
	ts := NewTelemetryStore()
	runID := "run_0000000000000123"
	ts.AddTelemetryBatch(runID, TelemetryBatchRequest{RunID: runID})

	server := &Server{telemetryStore: ts}

	req := httptest.NewRequest(http.MethodGet, "/runs/"+runID+"/errors", nil)
	w := httptest.NewRecorder()

	server.routeRuns(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("Expected status 404 for /errors without /signatures, got %d", w.Code)
	}
}
