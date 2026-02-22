package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bc-dunia/mcpdrill/internal/controlplane/runmanager"
	"github.com/bc-dunia/mcpdrill/internal/types"
	"github.com/bc-dunia/mcpdrill/internal/validation"
)

func newTestRunManagerForLogs(t *testing.T) *runmanager.RunManager {
	t.Helper()
	validator, err := validation.NewUnifiedValidator(nil)
	if err != nil {
		t.Fatalf("failed to create validator: %v", err)
	}
	return runmanager.NewRunManager(validator)
}

func TestQueryLogs_BasicFiltering(t *testing.T) {
	ts := NewTelemetryStore()

	batch := TelemetryBatchRequest{
		RunID: "run_0000000000000001",
		Operations: []types.OperationOutcome{
			{OpID: "op1", Operation: "tools/list", LatencyMs: 100, OK: true, TimestampMs: 1000},
			{OpID: "op2", Operation: "tools/call", ToolName: "read_file", LatencyMs: 200, OK: true, TimestampMs: 2000},
			{OpID: "op3", Operation: "tools/call", ToolName: "write_file", LatencyMs: 300, OK: false, ErrorType: "timeout", TimestampMs: 3000},
		},
	}
	ts.AddTelemetryBatchWithContext("run_0000000000000001", batch, "worker-1", "baseline", "stg_000000000002", "5")

	tests := []struct {
		name          string
		filters       LogFilters
		expectedCount int
		expectedTotal int
	}{
		{
			name:          "no filters returns all",
			filters:       LogFilters{Limit: 100, Order: "desc"},
			expectedCount: 3,
			expectedTotal: 3,
		},
		{
			name:          "filter by operation",
			filters:       LogFilters{Operation: "tools/call", Limit: 100, Order: "desc"},
			expectedCount: 2,
			expectedTotal: 2,
		},
		{
			name:          "filter by tool_name",
			filters:       LogFilters{ToolName: "read_file", Limit: 100, Order: "desc"},
			expectedCount: 1,
			expectedTotal: 1,
		},
		{
			name:          "filter by error_type",
			filters:       LogFilters{ErrorType: "timeout", Limit: 100, Order: "desc"},
			expectedCount: 1,
			expectedTotal: 1,
		},
		{
			name:          "filter by stage",
			filters:       LogFilters{Stage: "baseline", Limit: 100, Order: "desc"},
			expectedCount: 3,
			expectedTotal: 3,
		},
		{
			name:          "filter by worker_id",
			filters:       LogFilters{WorkerID: "worker-1", Limit: 100, Order: "desc"},
			expectedCount: 3,
			expectedTotal: 3,
		},
		{
			name:          "filter by non-existent worker",
			filters:       LogFilters{WorkerID: "worker-999", Limit: 100, Order: "desc"},
			expectedCount: 0,
			expectedTotal: 0,
		},
		{
			name:          "filter by vu_id",
			filters:       LogFilters{VUID: "5", Limit: 100, Order: "desc"},
			expectedCount: 3,
			expectedTotal: 3,
		},
		{
			name:          "filter by non-existent vu_id",
			filters:       LogFilters{VUID: "999", Limit: 100, Order: "desc"},
			expectedCount: 0,
			expectedTotal: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs, total, err := ts.QueryLogs("run_0000000000000001", tt.filters)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(logs) != tt.expectedCount {
				t.Errorf("expected %d logs, got %d", tt.expectedCount, len(logs))
			}
			if total != tt.expectedTotal {
				t.Errorf("expected total %d, got %d", tt.expectedTotal, total)
			}
		})
	}
}

func TestQueryLogs_Pagination(t *testing.T) {
	ts := NewTelemetryStore()

	batch := TelemetryBatchRequest{
		RunID:      "run_0000000000000001",
		Operations: make([]types.OperationOutcome, 25),
	}
	for i := 0; i < 25; i++ {
		batch.Operations[i] = types.OperationOutcome{
			OpID:        "op" + string(rune('a'+i)),
			Operation:   "tools/list",
			LatencyMs:   100 + i,
			OK:          true,
			TimestampMs: int64(1000 + i*100),
		}
	}
	ts.AddTelemetryBatch("run_0000000000000001", batch)

	tests := []struct {
		name          string
		limit         int
		offset        int
		expectedCount int
		expectedTotal int
	}{
		{
			name:          "first page",
			limit:         10,
			offset:        0,
			expectedCount: 10,
			expectedTotal: 25,
		},
		{
			name:          "second page",
			limit:         10,
			offset:        10,
			expectedCount: 10,
			expectedTotal: 25,
		},
		{
			name:          "last page partial",
			limit:         10,
			offset:        20,
			expectedCount: 5,
			expectedTotal: 25,
		},
		{
			name:          "offset beyond total",
			limit:         10,
			offset:        100,
			expectedCount: 0,
			expectedTotal: 25,
		},
		{
			name:          "limit larger than total",
			limit:         100,
			offset:        0,
			expectedCount: 25,
			expectedTotal: 25,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filters := LogFilters{Limit: tt.limit, Offset: tt.offset, Order: "desc"}
			logs, total, err := ts.QueryLogs("run_0000000000000001", filters)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(logs) != tt.expectedCount {
				t.Errorf("expected %d logs, got %d", tt.expectedCount, len(logs))
			}
			if total != tt.expectedTotal {
				t.Errorf("expected total %d, got %d", tt.expectedTotal, total)
			}
		})
	}
}

func TestQueryLogs_Ordering(t *testing.T) {
	ts := NewTelemetryStore()

	batch := TelemetryBatchRequest{
		RunID: "run_0000000000000001",
		Operations: []types.OperationOutcome{
			{OpID: "op1", Operation: "tools/list", LatencyMs: 100, OK: true, TimestampMs: 1000},
			{OpID: "op2", Operation: "tools/list", LatencyMs: 200, OK: true, TimestampMs: 3000},
			{OpID: "op3", Operation: "tools/list", LatencyMs: 300, OK: true, TimestampMs: 2000},
		},
	}
	ts.AddTelemetryBatch("run_0000000000000001", batch)

	t.Run("descending order", func(t *testing.T) {
		filters := LogFilters{Limit: 100, Order: "desc"}
		logs, _, err := ts.QueryLogs("run_0000000000000001", filters)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(logs) != 3 {
			t.Fatalf("expected 3 logs, got %d", len(logs))
		}
		if logs[0].TimestampMs != 3000 || logs[1].TimestampMs != 2000 || logs[2].TimestampMs != 1000 {
			t.Errorf("logs not in descending order: %v, %v, %v", logs[0].TimestampMs, logs[1].TimestampMs, logs[2].TimestampMs)
		}
	})

	t.Run("ascending order", func(t *testing.T) {
		filters := LogFilters{Limit: 100, Order: "asc"}
		logs, _, err := ts.QueryLogs("run_0000000000000001", filters)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(logs) != 3 {
			t.Fatalf("expected 3 logs, got %d", len(logs))
		}
		if logs[0].TimestampMs != 1000 || logs[1].TimestampMs != 2000 || logs[2].TimestampMs != 3000 {
			t.Errorf("logs not in ascending order: %v, %v, %v", logs[0].TimestampMs, logs[1].TimestampMs, logs[2].TimestampMs)
		}
	})
}

func TestQueryLogs_RunNotFound(t *testing.T) {
	ts := NewTelemetryStore()

	_, _, err := ts.QueryLogs("nonexistent", LogFilters{Limit: 100, Order: "desc"})
	if err == nil {
		t.Fatal("expected error for nonexistent run")
	}
}

func TestQueryLogs_CombinedFilters(t *testing.T) {
	ts := NewTelemetryStore()

	batch1 := TelemetryBatchRequest{
		RunID: "run_0000000000000001",
		Operations: []types.OperationOutcome{
			{OpID: "op1", Operation: "tools/call", ToolName: "read_file", LatencyMs: 100, OK: true, TimestampMs: 1000},
			{OpID: "op2", Operation: "tools/call", ToolName: "write_file", LatencyMs: 200, OK: false, ErrorType: "timeout", TimestampMs: 2000},
		},
	}
	ts.AddTelemetryBatchWithContext("run_0000000000000001", batch1, "worker-1", "baseline", "stg_000000000002", "1")

	batch2 := TelemetryBatchRequest{
		RunID: "run_0000000000000001",
		Operations: []types.OperationOutcome{
			{OpID: "op3", Operation: "tools/call", ToolName: "read_file", LatencyMs: 150, OK: true, TimestampMs: 3000},
			{OpID: "op4", Operation: "tools/call", ToolName: "read_file", LatencyMs: 250, OK: false, ErrorType: "timeout", TimestampMs: 4000},
		},
	}
	ts.AddTelemetryBatchWithContext("run_0000000000000001", batch2, "worker-2", "ramp", "stg_000000000003", "2")

	tests := []struct {
		name          string
		filters       LogFilters
		expectedCount int
	}{
		{
			name:          "operation AND tool_name",
			filters:       LogFilters{Operation: "tools/call", ToolName: "read_file", Limit: 100, Order: "desc"},
			expectedCount: 3,
		},
		{
			name:          "stage AND worker_id",
			filters:       LogFilters{Stage: "baseline", WorkerID: "worker-1", Limit: 100, Order: "desc"},
			expectedCount: 2,
		},
		{
			name:          "tool_name AND error_type",
			filters:       LogFilters{ToolName: "read_file", ErrorType: "timeout", Limit: 100, Order: "desc"},
			expectedCount: 1,
		},
		{
			name:          "stage AND vu_id",
			filters:       LogFilters{Stage: "ramp", VUID: "2", Limit: 100, Order: "desc"},
			expectedCount: 2,
		},
		{
			name:          "no match with combined filters",
			filters:       LogFilters{Stage: "baseline", WorkerID: "worker-2", Limit: 100, Order: "desc"},
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs, _, err := ts.QueryLogs("run_0000000000000001", tt.filters)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(logs) != tt.expectedCount {
				t.Errorf("expected %d logs, got %d", tt.expectedCount, len(logs))
			}
		})
	}
}

func TestHandleGetLogs_Success(t *testing.T) {
	rm := newTestRunManagerForLogs(t)
	server := NewServer("127.0.0.1:0", rm)
	ts := NewTelemetryStore()
	server.SetTelemetryStore(ts)

	batch := TelemetryBatchRequest{
		RunID: "run_0000000000000001",
		Operations: []types.OperationOutcome{
			{OpID: "op1", Operation: "tools/list", LatencyMs: 100, OK: true, TimestampMs: 1000},
			{OpID: "op2", Operation: "tools/call", ToolName: "read_file", LatencyMs: 200, OK: true, TimestampMs: 2000},
		},
	}
	ts.AddTelemetryBatchWithContext("run_0000000000000001", batch, "worker-1", "baseline", "stg_000000000002", "5")

	req := httptest.NewRequest(http.MethodGet, "/runs/run_0000000000000001/logs?limit=10&offset=0&order=desc", nil)
	w := httptest.NewRecorder()

	server.handleGetLogs(w, req, "run_0000000000000001")

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp LogQueryResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.RunID != "run_0000000000000001" {
		t.Errorf("expected run_id 'run_0000000000000001', got '%s'", resp.RunID)
	}
	if resp.Total != 2 {
		t.Errorf("expected total 2, got %d", resp.Total)
	}
	if len(resp.Logs) != 2 {
		t.Errorf("expected 2 logs, got %d", len(resp.Logs))
	}
}

func TestQueryLogs_ReturnsDeepCopiedPointerFields(t *testing.T) {
	ts := NewTelemetryStore()
	tokenIndex := 7
	eventsCount := 3

	batch := TelemetryBatchRequest{
		RunID: "run_0000000000000d111",
		Operations: []types.OperationOutcome{
			{
				OpID:        "op1",
				Operation:   "tools/call",
				ToolName:    "stream_tool",
				LatencyMs:   100,
				OK:          true,
				TimestampMs: 1000,
				Stream: &types.StreamInfo{
					IsStreaming: true,
					EventsCount: eventsCount,
				},
				TokenIndex: &tokenIndex,
			},
		},
	}
	ts.AddTelemetryBatch("run_0000000000000d111", batch)

	initial, _, err := ts.QueryLogs("run_0000000000000d111", LogFilters{Limit: 10, Offset: 0, Order: "desc"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(initial) != 1 {
		t.Fatalf("expected 1 log, got %d", len(initial))
	}
	if initial[0].Stream == nil || initial[0].TokenIndex == nil {
		t.Fatal("expected stream and token index to be present")
	}

	initial[0].Stream.EventsCount = 999
	*initial[0].TokenIndex = 888

	again, _, err := ts.QueryLogs("run_0000000000000d111", LogFilters{Limit: 10, Offset: 0, Order: "desc"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(again) != 1 {
		t.Fatalf("expected 1 log, got %d", len(again))
	}
	if again[0].Stream == nil || again[0].TokenIndex == nil {
		t.Fatal("expected stream and token index to be present")
	}
	if again[0].Stream.EventsCount != eventsCount {
		t.Fatalf("expected events_count %d, got %d", eventsCount, again[0].Stream.EventsCount)
	}
	if *again[0].TokenIndex != tokenIndex {
		t.Fatalf("expected token_index %d, got %d", tokenIndex, *again[0].TokenIndex)
	}
}

func TestQueryLogs_PreservesExecutionID(t *testing.T) {
	ts := NewTelemetryStore()

	batch := TelemetryBatchRequest{
		RunID: "run_0000000000000d112",
		Operations: []types.OperationOutcome{
			{
				OpID:        "op1",
				Operation:   "tools/call",
				ToolName:    "read_file",
				LatencyMs:   120,
				OK:          true,
				TimestampMs: 1000,
				ExecutionID: "exec_0000000000000001",
			},
		},
	}
	ts.AddTelemetryBatch("run_0000000000000d112", batch)

	logs, _, err := ts.QueryLogs("run_0000000000000d112", LogFilters{Limit: 10, Offset: 0, Order: "desc"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}
	if logs[0].ExecutionID != "exec_0000000000000001" {
		t.Fatalf("expected execution_id %q, got %q", "exec_0000000000000001", logs[0].ExecutionID)
	}
}

func TestHandleGetLogs_NotFound(t *testing.T) {
	rm := newTestRunManagerForLogs(t)
	server := NewServer("127.0.0.1:0", rm)
	ts := NewTelemetryStore()
	server.SetTelemetryStore(ts)

	req := httptest.NewRequest(http.MethodGet, "/runs/nonexistent/logs", nil)
	w := httptest.NewRecorder()

	server.handleGetLogs(w, req, "nonexistent")

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestHandleGetLogs_InvalidParams(t *testing.T) {
	rm := newTestRunManagerForLogs(t)
	server := NewServer("127.0.0.1:0", rm)
	ts := NewTelemetryStore()
	server.SetTelemetryStore(ts)

	ts.AddTelemetryBatch("run_0000000000000001", TelemetryBatchRequest{
		RunID:      "run_0000000000000001",
		Operations: []types.OperationOutcome{{OpID: "op1", Operation: "tools/list", OK: true, TimestampMs: 1000}},
	})

	tests := []struct {
		name        string
		queryString string
	}{
		{"invalid limit", "?limit=abc"},
		{"negative limit", "?limit=-1"},
		{"invalid offset", "?offset=abc"},
		{"negative offset", "?offset=-1"},
		{"invalid order", "?order=random"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/runs/run_0000000000000001/logs"+tt.queryString, nil)
			w := httptest.NewRecorder()

			server.handleGetLogs(w, req, "run_0000000000000001")

			if w.Code != http.StatusBadRequest {
				t.Errorf("expected status 400, got %d", w.Code)
			}
		})
	}
}

func TestHandleGetLogs_MethodNotAllowed(t *testing.T) {
	rm := newTestRunManagerForLogs(t)
	server := NewServer("127.0.0.1:0", rm)
	ts := NewTelemetryStore()
	server.SetTelemetryStore(ts)

	req := httptest.NewRequest(http.MethodPost, "/runs/run_0000000000000001/logs", nil)
	w := httptest.NewRecorder()

	server.handleGetLogs(w, req, "run_0000000000000001")

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandleGetLogs_TelemetryStoreNotConfigured(t *testing.T) {
	rm := newTestRunManagerForLogs(t)
	server := NewServer("127.0.0.1:0", rm)

	req := httptest.NewRequest(http.MethodGet, "/runs/run_0000000000000001/logs", nil)
	w := httptest.NewRecorder()

	server.handleGetLogs(w, req, "run_0000000000000001")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}
}

func TestHandleGetLogs_WithFilters(t *testing.T) {
	rm := newTestRunManagerForLogs(t)
	server := NewServer("127.0.0.1:0", rm)
	ts := NewTelemetryStore()
	server.SetTelemetryStore(ts)

	batch := TelemetryBatchRequest{
		RunID: "run_0000000000000001",
		Operations: []types.OperationOutcome{
			{OpID: "op1", Operation: "tools/list", LatencyMs: 100, OK: true, TimestampMs: 1000},
			{OpID: "op2", Operation: "tools/call", ToolName: "read_file", LatencyMs: 200, OK: true, TimestampMs: 2000},
			{OpID: "op3", Operation: "tools/call", ToolName: "write_file", LatencyMs: 300, OK: false, ErrorType: "timeout", TimestampMs: 3000},
		},
	}
	ts.AddTelemetryBatchWithContext("run_0000000000000001", batch, "worker-1", "baseline", "stg_000000000002", "5")

	req := httptest.NewRequest(http.MethodGet, "/runs/run_0000000000000001/logs?operation=tools/call&tool_name=read_file", nil)
	w := httptest.NewRecorder()

	server.handleGetLogs(w, req, "run_0000000000000001")

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp LogQueryResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Total != 1 {
		t.Errorf("expected total 1, got %d", resp.Total)
	}
	if len(resp.Logs) != 1 {
		t.Errorf("expected 1 log, got %d", len(resp.Logs))
	}
	if resp.Logs[0].ToolName != "read_file" {
		t.Errorf("expected tool_name 'read_file', got '%s'", resp.Logs[0].ToolName)
	}
}

func TestHandleGetLogs_LimitCapping(t *testing.T) {
	rm := newTestRunManagerForLogs(t)
	server := NewServer("127.0.0.1:0", rm)
	ts := NewTelemetryStore()
	server.SetTelemetryStore(ts)

	ts.AddTelemetryBatch("run_0000000000000001", TelemetryBatchRequest{
		RunID:      "run_0000000000000001",
		Operations: []types.OperationOutcome{{OpID: "op1", Operation: "tools/list", OK: true, TimestampMs: 1000}},
	})

	req := httptest.NewRequest(http.MethodGet, "/runs/run_0000000000000001/logs?limit=5000", nil)
	w := httptest.NewRecorder()

	server.handleGetLogs(w, req, "run_0000000000000001")

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp LogQueryResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Limit != 1000 {
		t.Errorf("expected limit to be capped at 1000, got %d", resp.Limit)
	}
}

func TestParseLogFilters(t *testing.T) {
	tests := []struct {
		name           string
		queryString    string
		expectedLimit  int
		expectedOffset int
		expectedOrder  string
		expectError    bool
	}{
		{
			name:           "defaults",
			queryString:    "",
			expectedLimit:  100,
			expectedOffset: 0,
			expectedOrder:  "desc",
			expectError:    false,
		},
		{
			name:           "custom values",
			queryString:    "?limit=50&offset=10&order=asc",
			expectedLimit:  50,
			expectedOffset: 10,
			expectedOrder:  "asc",
			expectError:    false,
		},
		{
			name:           "limit capped at 1000",
			queryString:    "?limit=2000",
			expectedLimit:  1000,
			expectedOffset: 0,
			expectedOrder:  "desc",
			expectError:    false,
		},
		{
			name:        "invalid limit",
			queryString: "?limit=abc",
			expectError: true,
		},
		{
			name:        "invalid offset",
			queryString: "?offset=xyz",
			expectError: true,
		},
		{
			name:        "invalid order",
			queryString: "?order=invalid",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/logs"+tt.queryString, nil)
			filters, err := parseLogFilters(req)

			if tt.expectError {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if filters.Limit != tt.expectedLimit {
				t.Errorf("expected limit %d, got %d", tt.expectedLimit, filters.Limit)
			}
			if filters.Offset != tt.expectedOffset {
				t.Errorf("expected offset %d, got %d", tt.expectedOffset, filters.Offset)
			}
			if filters.Order != tt.expectedOrder {
				t.Errorf("expected order '%s', got '%s'", tt.expectedOrder, filters.Order)
			}
		})
	}
}

func TestQueryLogs_EmptyResults(t *testing.T) {
	ts := NewTelemetryStore()

	batch := TelemetryBatchRequest{
		RunID: "run_0000000000000001",
		Operations: []types.OperationOutcome{
			{OpID: "op1", Operation: "tools/list", LatencyMs: 100, OK: true, TimestampMs: 1000},
		},
	}
	ts.AddTelemetryBatch("run_0000000000000001", batch)

	filters := LogFilters{Operation: "nonexistent", Limit: 100, Order: "desc"}
	logs, total, err := ts.QueryLogs("run_0000000000000001", filters)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(logs) != 0 {
		t.Errorf("expected 0 logs, got %d", len(logs))
	}
	if total != 0 {
		t.Errorf("expected total 0, got %d", total)
	}
}

func TestQueryLogs_ThreadSafety(t *testing.T) {
	ts := NewTelemetryStore()

	done := make(chan bool)

	go func() {
		for i := 0; i < 100; i++ {
			batch := TelemetryBatchRequest{
				RunID: "run_0000000000000001",
				Operations: []types.OperationOutcome{
					{OpID: "op", Operation: "tools/list", LatencyMs: 100, OK: true, TimestampMs: int64(i * 1000)},
				},
			}
			ts.AddTelemetryBatch("run_0000000000000001", batch)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			ts.QueryLogs("run_0000000000000001", LogFilters{Limit: 100, Order: "desc"})
		}
		done <- true
	}()

	<-done
	<-done
}
