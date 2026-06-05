package worker

import (
	"testing"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/transport"
	"github.com/bc-dunia/mcpdrill/internal/types"
	"github.com/bc-dunia/mcpdrill/internal/vu"
)

func TestConvertToOutcomePreservesFailureDetails(t *testing.T) {
	status := 503
	result := &vu.OperationResult{
		TraceID:   "op-error",
		Operation: vu.OpToolsList,
		StartTime: time.UnixMilli(1000),
		EndTime:   time.UnixMilli(1025),
		VUID:      "vu-1",
		SessionID: "sess-1",
		Outcome: &transport.OperationOutcome{
			OK:         false,
			LatencyMs:  25,
			HTTPStatus: &status,
			Error: &transport.OperationError{
				Type:    transport.ErrorTypeHTTP,
				Code:    transport.CodeHTTPServerError,
				Message: "target returned 503",
			},
		},
	}
	assignment := types.WorkerAssignment{
		ExecutionID: "exec-1",
		Stage:       "baseline",
		StageID:     "stg-1",
	}

	outcome := ConvertToOutcome(result, assignment, "worker-1")

	if outcome.ErrorType != string(transport.ErrorTypeHTTP) {
		t.Fatalf("ErrorType = %q, want %q", outcome.ErrorType, transport.ErrorTypeHTTP)
	}
	if outcome.ErrorCode != string(transport.CodeHTTPServerError) {
		t.Fatalf("ErrorCode = %q, want %q", outcome.ErrorCode, transport.CodeHTTPServerError)
	}
	if outcome.ErrorMessage != "target returned 503" {
		t.Fatalf("ErrorMessage = %q", outcome.ErrorMessage)
	}
	if outcome.HTTPStatus != status {
		t.Fatalf("HTTPStatus = %d, want %d", outcome.HTTPStatus, status)
	}
}
