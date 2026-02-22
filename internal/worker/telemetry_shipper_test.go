package worker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/types"
)

func TestTelemetryShipperCloseFlushesBufferedOperations(t *testing.T) {
	var received atomic.Int64

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}

		var req struct {
			RunID      string                   `json:"run_id"`
			Operations []types.OperationOutcome `json:"operations"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		received.Add(int64(len(req.Operations)))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]int{"accepted": len(req.Operations)})
	}))
	defer server.Close()

	retryClient := NewRetryHTTPClient(context.Background(), server.URL, server.Client(), RetryConfig{
		MaxRetries: 0,
		Backoff:    10 * time.Millisecond,
		MaxBackoff: 10 * time.Millisecond,
	})
	shipper := NewTelemetryShipper(context.Background(), "worker-1", retryClient)

	shipper.Ship("run-1", types.OperationOutcome{Operation: "ping", OK: true})
	shipper.Ship("run-1", types.OperationOutcome{Operation: "tools/list", OK: true})
	shipper.Close()

	shipped, dropped := shipper.Stats()
	if dropped != 0 {
		t.Fatalf("expected dropped=0, got %d", dropped)
	}
	if shipped != 2 {
		t.Fatalf("expected shipped=2, got %d", shipped)
	}
	if received.Load() != 2 {
		t.Fatalf("expected server to receive 2 ops, got %d", received.Load())
	}
}

func TestTelemetryShipperShipAfterCloseDrops(t *testing.T) {
	retryClient := NewRetryHTTPClient(context.Background(), "http://127.0.0.1:1", http.DefaultClient, RetryConfig{
		MaxRetries: 0,
		Backoff:    10 * time.Millisecond,
		MaxBackoff: 10 * time.Millisecond,
	})
	shipper := NewTelemetryShipper(context.Background(), "worker-1", retryClient)
	shipper.Close()

	shipper.Ship("run-1", types.OperationOutcome{Operation: "ping", OK: true})

	shipped, dropped := shipper.Stats()
	if shipped != 0 {
		t.Fatalf("expected shipped=0, got %d", shipped)
	}
	if dropped == 0 {
		t.Fatal("expected dropped > 0 after ship on closed shipper")
	}
}
