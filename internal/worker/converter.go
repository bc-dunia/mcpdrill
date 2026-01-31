package worker

import (
	"time"

	"github.com/bc-dunia/mcpdrill/internal/types"
	"github.com/bc-dunia/mcpdrill/internal/vu"
)

func ConvertToOutcome(result *vu.OperationResult, a types.WorkerAssignment, workerID string) types.OperationOutcome {
	outcome := types.OperationOutcome{
		OpID:        result.TraceID,
		Operation:   string(result.Operation),
		ToolName:    result.ToolName,
		LatencyMs:   int(result.EndTime.Sub(result.StartTime).Milliseconds()),
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
	const hexChars = "0123456789abcdef"
	b := make([]byte, n)
	for i := range b {
		b[i] = hexChars[time.Now().UnixNano()%16]
	}
	return string(b)
}
