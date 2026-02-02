package worker

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/types"
	"github.com/bc-dunia/mcpdrill/internal/vu"
)

func ConvertToOutcome(result *vu.OperationResult, a types.WorkerAssignment, workerID string) types.OperationOutcome {
	// Calculate latency: prefer transport-measured latency, fallback to executor timing
	// Use microseconds divided by 1000 to preserve sub-millisecond precision
	latencyMs := int(result.EndTime.Sub(result.StartTime).Microseconds() / 1000)
	if result.Outcome != nil && result.Outcome.LatencyMs > 0 {
		// Transport layer measured latency is more accurate (network timing)
		latencyMs = int(result.Outcome.LatencyMs)
	}
	// Ensure minimum 1ms for any completed operation to avoid 0 values
	if latencyMs == 0 && !result.EndTime.IsZero() && !result.StartTime.IsZero() {
		latencyMs = 1
	}

	outcome := types.OperationOutcome{
		OpID:        result.TraceID,
		Operation:   string(result.Operation),
		ToolName:    result.ToolName,
		LatencyMs:   latencyMs,
		OK:          result.Outcome != nil && result.Outcome.OK,
		TimestampMs: result.StartTime.UnixMilli(),
		WorkerID:    workerID,
		ExecutionID: a.ExecutionID,
		Stage:       a.Stage,
		StageID:     a.StageID,
		VUID:        result.VUID,
		SessionID:   result.SessionID,
	}

	if result.Outcome != nil {
		if result.Outcome.Error != nil {
			outcome.ErrorType = string(result.Outcome.Error.Type)
			outcome.ErrorCode = string(result.Outcome.Error.Code)
		}
		if result.Outcome.HTTPStatus != nil {
			outcome.HTTPStatus = *result.Outcome.HTTPStatus
		}
		if result.Outcome.Stream != nil {
			outcome.Stream = &types.StreamInfo{
				IsStreaming:     result.Outcome.Stream.IsStreaming,
				EventsCount:     result.Outcome.Stream.EventsCount,
				EndedNormally:   result.Outcome.Stream.EndedNormally,
				Stalled:         result.Outcome.Stream.Stalled,
				StallDurationMs: int64(result.Outcome.Stream.StallDurationMs),
			}
		}
	}

	if outcome.OpID == "" {
		outcome.OpID = generateOpID(result.StartTime)
	}

	return outcome
}

func generateOpID(t time.Time) string {
	return "op_" + t.Format("20060102150405") + "_" + randomHex(8)
}

func randomHex(n int) string {
	b := make([]byte, n/2)
	if _, err := rand.Read(b); err != nil {
		// Fallback to time-based if crypto/rand fails (should never happen)
		return time.Now().Format("20060102150405")[:n]
	}
	return hex.EncodeToString(b)[:n]
}
