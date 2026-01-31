package worker

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/types"
)

const (
	defaultBatchSize     = 100
	defaultFlushInterval = time.Second
	defaultBufferSize    = 10000
)

type TelemetryShipper struct {
	workerID string
	client   *RetryHTTPClient

	buffer      chan telemetryItem
	batchSize   int
	flushTicker *time.Ticker

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	droppedCount atomic.Int64
	shippedCount atomic.Int64
}

type telemetryItem struct {
	runID   string
	outcome types.OperationOutcome
}

type telemetryBatchRequest struct {
	RunID      string                   `json:"run_id"`
	Operations []types.OperationOutcome `json:"operations"`
}

func NewTelemetryShipper(ctx context.Context, workerID string, client *RetryHTTPClient) *TelemetryShipper {
	shipperCtx, cancel := context.WithCancel(ctx)

	s := &TelemetryShipper{
		workerID:    workerID,
		client:      client,
		buffer:      make(chan telemetryItem, defaultBufferSize),
		batchSize:   defaultBatchSize,
		flushTicker: time.NewTicker(defaultFlushInterval),
		ctx:         shipperCtx,
		cancel:      cancel,
	}

	s.wg.Add(1)
	go s.run()

	return s
}

func (s *TelemetryShipper) Ship(runID string, outcome types.OperationOutcome) {
	select {
	case s.buffer <- telemetryItem{runID: runID, outcome: outcome}:
	default:
		s.droppedCount.Add(1)
	}
}

func (s *TelemetryShipper) run() {
	defer s.wg.Done()

	batches := make(map[string][]types.OperationOutcome)

	flush := func() {
		for runID, ops := range batches {
			if len(ops) > 0 {
				s.shipBatch(runID, ops)
			}
		}
		batches = make(map[string][]types.OperationOutcome)
	}

	for {
		select {
		case item, ok := <-s.buffer:
			if !ok {
				flush()
				return
			}

			batches[item.runID] = append(batches[item.runID], item.outcome)

			if len(batches[item.runID]) >= s.batchSize {
				s.shipBatch(item.runID, batches[item.runID])
				delete(batches, item.runID)
			}

		case <-s.flushTicker.C:
			flush()

		case <-s.ctx.Done():
			flush()
			return
		}
	}
}

func (s *TelemetryShipper) shipBatch(runID string, ops []types.OperationOutcome) {
	if len(ops) == 0 {
		return
	}

	req := telemetryBatchRequest{
		RunID:      runID,
		Operations: ops,
	}

	path := "/workers/" + s.workerID + "/telemetry"
	resp, err := s.client.Post(path, req)
	if err != nil {
		log.Printf("[TelemetryShipper] Failed to ship batch: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := ReadResponseBody(resp)
		log.Printf("[TelemetryShipper] Ship failed: status=%d body=%s", resp.StatusCode, string(body))
		return
	}

	s.shippedCount.Add(int64(len(ops)))

	var result struct {
		Accepted int `json:"accepted"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err == nil {
		if result.Accepted != len(ops) {
			log.Printf("[TelemetryShipper] Partial accept: sent=%d accepted=%d", len(ops), result.Accepted)
		}
	}
}

func (s *TelemetryShipper) Close() {
	s.cancel()
	s.flushTicker.Stop()
	close(s.buffer)
	s.wg.Wait()
}

func (s *TelemetryShipper) Stats() (shipped, dropped int64) {
	return s.shippedCount.Load(), s.droppedCount.Load()
}
