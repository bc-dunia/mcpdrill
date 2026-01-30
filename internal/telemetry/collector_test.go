package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/transport"
)

func TestBoundedQueue_BasicOperations(t *testing.T) {
	q := NewBoundedQueue(100)

	record := &TelemetryRecord{
		Type: "op_log",
		Tier: Tier1Operation,
		OpLog: &OpLog{
			Version:   OpLogVersion,
			Timestamp: time.Now(),
			Operation: "test",
		},
	}

	if !q.Enqueue(record) {
		t.Fatal("expected enqueue to succeed")
	}

	if q.Len() != 1 {
		t.Fatalf("expected len 1, got %d", q.Len())
	}

	dequeued := q.TryDequeue()
	if dequeued == nil {
		t.Fatal("expected dequeue to return record")
	}

	if dequeued.Type != "op_log" {
		t.Fatalf("expected type op_log, got %s", dequeued.Type)
	}

	if q.Len() != 0 {
		t.Fatalf("expected len 0, got %d", q.Len())
	}
}

func TestBoundedQueue_Capacity(t *testing.T) {
	q := NewBoundedQueue(10)

	for i := 0; i < 10; i++ {
		record := &TelemetryRecord{
			Type: "op_log",
			Tier: Tier0Lifecycle,
			OpLog: &OpLog{
				Version:   OpLogVersion,
				Operation: "test",
			},
		}
		if !q.Enqueue(record) {
			t.Fatalf("expected enqueue %d to succeed", i)
		}
	}

	if q.Len() != 10 {
		t.Fatalf("expected len 10, got %d", q.Len())
	}

	if q.Capacity() != 10 {
		t.Fatalf("expected capacity 10, got %d", q.Capacity())
	}
}

func TestBoundedQueue_BackpressureTier2Shed(t *testing.T) {
	q := NewBoundedQueue(5)

	for i := 0; i < 3; i++ {
		q.Enqueue(&TelemetryRecord{
			Type: "op_log",
			Tier: Tier0Lifecycle,
			OpLog: &OpLog{
				Version:   OpLogVersion,
				Operation: "tier0",
			},
		})
	}

	for i := 0; i < 2; i++ {
		q.Enqueue(&TelemetryRecord{
			Type: "op_log",
			Tier: Tier2Verbose,
			OpLog: &OpLog{
				Version:   OpLogVersion,
				Operation: "tier2",
			},
		})
	}

	if q.Len() != 5 {
		t.Fatalf("expected len 5, got %d", q.Len())
	}

	newRecord := &TelemetryRecord{
		Type: "op_log",
		Tier: Tier1Operation,
		OpLog: &OpLog{
			Version:   OpLogVersion,
			Operation: "tier1_new",
		},
	}

	if !q.Enqueue(newRecord) {
		t.Fatal("expected tier1 enqueue to succeed by shedding tier2")
	}

	stats := q.Stats()
	if stats.DroppedTier2 != 1 {
		t.Fatalf("expected 1 dropped tier2, got %d", stats.DroppedTier2)
	}

	if q.Len() != 5 {
		t.Fatalf("expected len 5 after shed, got %d", q.Len())
	}
}

func TestBoundedQueue_BackpressureTier2Dropped(t *testing.T) {
	q := NewBoundedQueue(3)

	for i := 0; i < 3; i++ {
		q.Enqueue(&TelemetryRecord{
			Type: "op_log",
			Tier: Tier0Lifecycle,
			OpLog: &OpLog{
				Version:   OpLogVersion,
				Operation: "tier0",
			},
		})
	}

	tier2Record := &TelemetryRecord{
		Type: "op_log",
		Tier: Tier2Verbose,
		OpLog: &OpLog{
			Version:   OpLogVersion,
			Operation: "tier2_dropped",
		},
	}

	if q.Enqueue(tier2Record) {
		t.Fatal("expected tier2 enqueue to fail when queue full of tier0")
	}

	stats := q.Stats()
	if stats.DroppedTier2 != 1 {
		t.Fatalf("expected 1 dropped tier2, got %d", stats.DroppedTier2)
	}
}

func TestBoundedQueue_Tier0NeverDropped(t *testing.T) {
	q := NewBoundedQueue(3)

	for i := 0; i < 3; i++ {
		q.Enqueue(&TelemetryRecord{
			Type: "op_log",
			Tier: Tier0Lifecycle,
			OpLog: &OpLog{
				Version:   OpLogVersion,
				Operation: "tier0",
			},
		})
	}

	tier0Record := &TelemetryRecord{
		Type: "op_log",
		Tier: Tier0Lifecycle,
		OpLog: &OpLog{
			Version:   OpLogVersion,
			Operation: "tier0_extra",
		},
	}

	if !q.Enqueue(tier0Record) {
		t.Fatal("expected tier0 enqueue to always succeed")
	}

	if q.Len() != 4 {
		t.Fatalf("expected len 4 (tier0 exceeds capacity), got %d", q.Len())
	}

	stats := q.Stats()
	if stats.DroppedTier2 != 0 || stats.DroppedTier1 != 0 {
		t.Fatal("expected no drops for tier0 records")
	}
}

func TestBoundedQueue_DequeueBatch(t *testing.T) {
	q := NewBoundedQueue(100)

	for i := 0; i < 10; i++ {
		q.Enqueue(&TelemetryRecord{
			Type: "op_log",
			Tier: Tier1Operation,
			OpLog: &OpLog{
				Version:   OpLogVersion,
				Operation: "test",
			},
		})
	}

	batch := q.TryDequeueBatch(5)
	if len(batch) != 5 {
		t.Fatalf("expected batch of 5, got %d", len(batch))
	}

	if q.Len() != 5 {
		t.Fatalf("expected 5 remaining, got %d", q.Len())
	}

	batch = q.TryDequeueBatch(10)
	if len(batch) != 5 {
		t.Fatalf("expected batch of 5 (remaining), got %d", len(batch))
	}

	if q.Len() != 0 {
		t.Fatalf("expected 0 remaining, got %d", q.Len())
	}
}

func TestBoundedQueue_Close(t *testing.T) {
	q := NewBoundedQueue(100)

	q.Enqueue(&TelemetryRecord{
		Type: "op_log",
		Tier: Tier1Operation,
	})

	q.Close()

	if !q.IsClosed() {
		t.Fatal("expected queue to be closed")
	}

	if q.Enqueue(&TelemetryRecord{Type: "op_log", Tier: Tier1Operation}) {
		t.Fatal("expected enqueue to fail after close")
	}

	record := q.TryDequeue()
	if record == nil {
		t.Fatal("expected to dequeue existing record after close")
	}

	record = q.TryDequeue()
	if record != nil {
		t.Fatal("expected nil after queue empty and closed")
	}
}

func TestBoundedQueue_ConcurrentAccess(t *testing.T) {
	q := NewBoundedQueue(1000)
	var wg sync.WaitGroup

	producers := 10
	recordsPerProducer := 100

	for i := 0; i < producers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < recordsPerProducer; j++ {
				tier := LogTier(j % 3)
				q.Enqueue(&TelemetryRecord{
					Type: "op_log",
					Tier: tier,
					OpLog: &OpLog{
						Version:   OpLogVersion,
						Operation: "concurrent",
					},
				})
			}
		}(i)
	}

	consumers := 5
	consumed := make([]int, consumers)
	for i := 0; i < consumers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for {
				record := q.TryDequeue()
				if record == nil {
					time.Sleep(time.Millisecond)
					if q.Len() == 0 {
						return
					}
					continue
				}
				consumed[id]++
			}
		}(i)
	}

	wg.Wait()

	stats := q.Stats()
	totalConsumed := 0
	for _, c := range consumed {
		totalConsumed += c
	}

	expectedTotal := producers * recordsPerProducer
	actualTotal := totalConsumed + int(stats.DroppedTier2) + int(stats.DroppedTier1) + q.Len()

	if actualTotal != expectedTotal {
		t.Fatalf("expected total %d, got consumed=%d dropped=%d remaining=%d",
			expectedTotal, totalConsumed, stats.DroppedTier2+stats.DroppedTier1, q.Len())
	}
}

func TestOpLog_CorrelationKeys(t *testing.T) {
	keys := CorrelationKeys{
		RunID:       "run_0000000000000001",
		ExecutionID: "exe_12345678",
		Stage:       "soak",
		StageID:     "stg_0000000000000001",
		WorkerID:    "wkr_0000000000000001",
		VUID:        "vu-1",
		SessionID:   "session-123",
		OpID:        "op-456",
		Attempt:     1,
	}

	outcome := &transport.OperationOutcome{
		Operation: transport.OpToolsCall,
		ToolName:  "test_tool",
		LatencyMs: 100,
		OK:        true,
		Transport: "streamable-http",
	}

	log := NewOpLogFromOutcome(outcome, keys, Tier1Operation)

	if log.Version != OpLogVersion {
		t.Fatalf("expected version %s, got %s", OpLogVersion, log.Version)
	}

	if log.RunID != keys.RunID {
		t.Fatalf("expected run_id %s, got %s", keys.RunID, log.RunID)
	}

	if log.ExecutionID != keys.ExecutionID {
		t.Fatalf("expected execution_id %s, got %s", keys.ExecutionID, log.ExecutionID)
	}

	if log.Stage != keys.Stage {
		t.Fatalf("expected stage %s, got %s", keys.Stage, log.Stage)
	}

	if log.StageID != keys.StageID {
		t.Fatalf("expected stage_id %s, got %s", keys.StageID, log.StageID)
	}

	if log.WorkerID != keys.WorkerID {
		t.Fatalf("expected worker_id %s, got %s", keys.WorkerID, log.WorkerID)
	}

	if log.VUID != keys.VUID {
		t.Fatalf("expected vu_id %s, got %s", keys.VUID, log.VUID)
	}

	if log.SessionID != keys.SessionID {
		t.Fatalf("expected session_id %s, got %s", keys.SessionID, log.SessionID)
	}
}

func TestOpLog_MarshalJSONL(t *testing.T) {
	keys := CorrelationKeys{
		RunID:       "run_0000000000000001",
		ExecutionID: "exe_12345678",
		Stage:       "soak",
		StageID:     "stg_0000000000000001",
		WorkerID:    "wkr_0000000000000001",
		VUID:        "vu-1",
		SessionID:   "session-123",
	}

	log := &OpLog{
		Version:         OpLogVersion,
		Timestamp:       time.Now(),
		Tier:            Tier1Operation,
		CorrelationKeys: keys,
		Operation:       "tools/call",
		ToolName:        "test_tool",
		LatencyMs:       100,
		OK:              true,
	}

	data, err := log.MarshalJSONL()
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	if strings.Contains(string(data), "\n") {
		t.Fatal("JSONL should not contain newlines")
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	requiredKeys := []string{"run_id", "execution_id", "stage", "stage_id", "worker_id", "vu_id", "session_id"}
	for _, key := range requiredKeys {
		if _, exists := parsed[key]; !exists {
			t.Fatalf("missing required correlation key: %s", key)
		}
	}
}

func TestOpLog_ErrorFields(t *testing.T) {
	keys := CorrelationKeys{
		RunID:       "run_0000000000000001",
		ExecutionID: "exe_12345678",
		Stage:       "soak",
		StageID:     "stg_0000000000000001",
		WorkerID:    "wkr_0000000000000001",
		VUID:        "vu-1",
		SessionID:   "session-123",
	}

	outcome := &transport.OperationOutcome{
		Operation: transport.OpToolsCall,
		ToolName:  "test_tool",
		LatencyMs: 100,
		OK:        false,
		Error: &transport.OperationError{
			Type:    transport.ErrorTypeTimeout,
			Code:    transport.CodeRequestTimeout,
			Message: "request timed out",
		},
	}

	log := NewOpLogFromOutcome(outcome, keys, Tier0Lifecycle)

	if log.OK {
		t.Fatal("expected OK to be false")
	}

	if log.ErrorType != string(transport.ErrorTypeTimeout) {
		t.Fatalf("expected error_type %s, got %s", transport.ErrorTypeTimeout, log.ErrorType)
	}

	if log.ErrorCode != string(transport.CodeRequestTimeout) {
		t.Fatalf("expected error_code %s, got %s", transport.CodeRequestTimeout, log.ErrorCode)
	}

	if log.ErrorMessage != "request timed out" {
		t.Fatalf("expected error_message 'request timed out', got %s", log.ErrorMessage)
	}
}

func TestEmitter_WriteJSONL(t *testing.T) {
	var buf bytes.Buffer
	emitter := NewEmitterWithWriter(&buf, nil)

	keys := CorrelationKeys{
		RunID:       "run_0000000000000001",
		ExecutionID: "exe_12345678",
		Stage:       "soak",
		StageID:     "stg_0000000000000001",
		WorkerID:    "wkr_0000000000000001",
		VUID:        "vu-1",
		SessionID:   "session-123",
	}

	log := &OpLog{
		Version:         OpLogVersion,
		Timestamp:       time.Now(),
		Tier:            Tier1Operation,
		CorrelationKeys: keys,
		Operation:       "tools/call",
		LatencyMs:       100,
		OK:              true,
	}

	if err := emitter.EmitOpLog(log); err != nil {
		t.Fatalf("emit failed: %v", err)
	}

	if err := emitter.Flush(); err != nil {
		t.Fatalf("flush failed: %v", err)
	}

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(lines[0]), &parsed); err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if parsed["version"] != OpLogVersion {
		t.Fatalf("expected version %s, got %v", OpLogVersion, parsed["version"])
	}
}

func TestEmitter_Stats(t *testing.T) {
	var buf bytes.Buffer
	emitter := NewEmitterWithWriter(&buf, nil)

	for i := 0; i < 5; i++ {
		log := &OpLog{
			Version:   OpLogVersion,
			Timestamp: time.Now(),
			Operation: "test",
			OK:        true,
		}
		emitter.EmitOpLog(log)
	}

	emitter.Flush()

	stats := emitter.Stats()
	if stats.TotalWritten != 5 {
		t.Fatalf("expected 5 written, got %d", stats.TotalWritten)
	}

	if stats.TotalBytes == 0 {
		t.Fatal("expected non-zero bytes")
	}

	if stats.WriteErrors != 0 {
		t.Fatalf("expected 0 errors, got %d", stats.WriteErrors)
	}
}

func TestCollector_RecordOperation(t *testing.T) {
	var buf bytes.Buffer
	emitter := NewEmitterWithWriter(&buf, nil)

	config := &CollectorConfig{
		QueueSize:     100,
		BatchSize:     10,
		FlushInterval: 10 * time.Millisecond,
		WorkerID:      "wkr_0000000000000001",
	}

	collector := NewCollector(config, emitter)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := collector.Start(ctx); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	keys := CorrelationKeys{
		RunID:       "run_0000000000000001",
		ExecutionID: "exe_12345678",
		Stage:       "soak",
		StageID:     "stg_0000000000000001",
		WorkerID:    "wkr_0000000000000001",
		VUID:        "vu-1",
		SessionID:   "session-123",
	}

	outcome := &transport.OperationOutcome{
		Operation: transport.OpToolsCall,
		ToolName:  "test_tool",
		LatencyMs: 100,
		OK:        true,
	}

	collector.RecordOperation(outcome, keys, Tier1Operation)

	time.Sleep(50 * time.Millisecond)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
	defer stopCancel()

	if err := collector.Stop(stopCtx); err != nil {
		t.Fatalf("stop failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "run_0000000000000001") {
		t.Fatal("expected output to contain run_id")
	}

	if !strings.Contains(output, "tools/call") {
		t.Fatal("expected output to contain operation")
	}
}

func TestCollector_QueueStats(t *testing.T) {
	config := &CollectorConfig{
		QueueSize:     100,
		BatchSize:     10,
		FlushInterval: time.Hour,
	}

	collector := NewCollector(config, nil)

	keys := CorrelationKeys{
		RunID:       "run_0000000000000001",
		ExecutionID: "exe_12345678",
		Stage:       "soak",
		StageID:     "stg_0000000000000001",
		WorkerID:    "wkr_0000000000000001",
		VUID:        "vu-1",
		SessionID:   "session-123",
	}

	for i := 0; i < 5; i++ {
		outcome := &transport.OperationOutcome{
			Operation: transport.OpPing,
			LatencyMs: 10,
			OK:        true,
		}
		collector.RecordOperation(outcome, keys, Tier1Operation)
	}

	stats := collector.QueueStats()
	if stats.Depth != 5 {
		t.Fatalf("expected depth 5, got %d", stats.Depth)
	}

	if stats.Capacity != 100 {
		t.Fatalf("expected capacity 100, got %d", stats.Capacity)
	}
}

func TestWorkerHealth_MarshalJSONL(t *testing.T) {
	health := &WorkerHealth{
		Timestamp:      time.Now(),
		WorkerID:       "wkr_0000000000000001",
		CPUPercent:     45.5,
		MemBytes:       1024 * 1024 * 100,
		ActiveVUs:      10,
		ActiveSessions: 8,
		InFlightOps:    5,
		QueueDepth:     50,
		QueueCapacity:  1000,
		DroppedTier2:   3,
	}

	data, err := health.MarshalJSONL()
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	if strings.Contains(string(data), "\n") {
		t.Fatal("JSONL should not contain newlines")
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if parsed["worker_id"] != "wkr_0000000000000001" {
		t.Fatalf("expected worker_id wkr_0000000000000001, got %v", parsed["worker_id"])
	}

	if parsed["active_vus"].(float64) != 10 {
		t.Fatalf("expected active_vus 10, got %v", parsed["active_vus"])
	}
}

func TestCollector_LifecycleEvent(t *testing.T) {
	var buf bytes.Buffer
	emitter := NewEmitterWithWriter(&buf, nil)

	config := &CollectorConfig{
		QueueSize:     100,
		BatchSize:     10,
		FlushInterval: 10 * time.Millisecond,
	}

	collector := NewCollector(config, emitter)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	collector.Start(ctx)

	keys := CorrelationKeys{
		RunID:       "run_0000000000000001",
		ExecutionID: "exe_12345678",
		Stage:       "ramp-up",
		StageID:     "stg_000000000003",
		WorkerID:    "wkr_0000000000000001",
		VUID:        "vu-1",
		SessionID:   "session-123",
	}

	collector.RecordLifecycleEvent("vu_started", keys, nil)

	time.Sleep(50 * time.Millisecond)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
	defer stopCancel()
	collector.Stop(stopCtx)

	output := buf.String()
	if !strings.Contains(output, "vu_started") {
		t.Fatal("expected output to contain lifecycle event")
	}
}

type mockHealthProvider struct {
	activeVUs      int
	activeSessions int64
	inFlightOps    int64
}

func (m *mockHealthProvider) ActiveVUs() int {
	return m.activeVUs
}

func (m *mockHealthProvider) ActiveSessions() int64 {
	return m.activeSessions
}

func (m *mockHealthProvider) InFlightOps() int64 {
	return m.inFlightOps
}

func TestCollector_HealthSnapshots(t *testing.T) {
	var buf bytes.Buffer
	emitter := NewEmitterWithWriter(&buf, nil)

	config := &CollectorConfig{
		QueueSize:              100,
		BatchSize:              10,
		FlushInterval:          10 * time.Millisecond,
		WorkerID:               "wkr_0000000000000001",
		HealthSnapshotInterval: 20 * time.Millisecond,
	}

	collector := NewCollector(config, emitter)
	collector.SetHealthProvider(&mockHealthProvider{
		activeVUs:      5,
		activeSessions: 4,
		inFlightOps:    3,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	collector.Start(ctx)

	time.Sleep(100 * time.Millisecond)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
	defer stopCancel()
	collector.Stop(stopCtx)

	output := buf.String()
	if !strings.Contains(output, "wkr_0000000000000001") {
		t.Fatal("expected output to contain worker health")
	}

	if !strings.Contains(output, "active_vus") {
		t.Fatal("expected output to contain active_vus field")
	}
}

func TestBoundedQueue_BlockingDequeue(t *testing.T) {
	q := NewBoundedQueue(100)

	done := make(chan struct{})
	var received *TelemetryRecord

	go func() {
		received = q.Dequeue()
		close(done)
	}()

	time.Sleep(10 * time.Millisecond)

	select {
	case <-done:
		t.Fatal("dequeue should be blocking")
	default:
	}

	q.Enqueue(&TelemetryRecord{
		Type: "op_log",
		Tier: Tier1Operation,
	})

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("dequeue should have unblocked")
	}

	if received == nil {
		t.Fatal("expected to receive record")
	}
}

func TestBoundedQueue_CloseUnblocksDequeue(t *testing.T) {
	q := NewBoundedQueue(100)

	done := make(chan struct{})
	var received *TelemetryRecord

	go func() {
		received = q.Dequeue()
		close(done)
	}()

	time.Sleep(10 * time.Millisecond)

	q.Close()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("close should unblock dequeue")
	}

	if received != nil {
		t.Fatal("expected nil from closed empty queue")
	}
}

func TestDefaultConfigs(t *testing.T) {
	collectorConfig := DefaultCollectorConfig()
	if collectorConfig.QueueSize != 10000 {
		t.Fatalf("expected queue size 10000, got %d", collectorConfig.QueueSize)
	}
	if collectorConfig.BatchSize != 100 {
		t.Fatalf("expected batch size 100, got %d", collectorConfig.BatchSize)
	}

	emitterConfig := DefaultEmitterConfig()
	if emitterConfig.BufferSize != 64*1024 {
		t.Fatalf("expected buffer size 64KB, got %d", emitterConfig.BufferSize)
	}
}

func TestOpLog_StreamInfo(t *testing.T) {
	keys := CorrelationKeys{
		RunID:       "run_0000000000000001",
		ExecutionID: "exe_12345678",
		Stage:       "soak",
		StageID:     "stg_0000000000000001",
		WorkerID:    "wkr_0000000000000001",
		VUID:        "vu-1",
		SessionID:   "session-123",
	}

	outcome := &transport.OperationOutcome{
		Operation: transport.OpToolsCall,
		ToolName:  "streaming_tool",
		LatencyMs: 500,
		OK:        true,
		Stream: &transport.StreamSignals{
			IsStreaming:   true,
			EventsCount:   10,
			EndedNormally: true,
		},
	}

	log := NewOpLogFromOutcome(outcome, keys, Tier1Operation)

	if log.Stream == nil {
		t.Fatal("expected stream info to be set")
	}

	if !log.Stream.IsStreaming {
		t.Fatal("expected is_streaming to be true")
	}

	if log.Stream.EventsCount != 10 {
		t.Fatalf("expected events_count 10, got %d", log.Stream.EventsCount)
	}

	if !log.Stream.EndedNormally {
		t.Fatal("expected ended_normally to be true")
	}
}

func TestEmitter_EmitBatch(t *testing.T) {
	var buf bytes.Buffer
	emitter := NewEmitterWithWriter(&buf, nil)

	batch := &TelemetryBatch{
		BatchID:   "batch-123",
		CreatedAt: time.Now(),
		Records: []*OpLog{
			{
				Version:   OpLogVersion,
				Timestamp: time.Now(),
				Operation: "op1",
				OK:        true,
			},
			{
				Version:   OpLogVersion,
				Timestamp: time.Now(),
				Operation: "op2",
				OK:        true,
			},
		},
		WorkerHealth: &WorkerHealth{
			Timestamp:  time.Now(),
			WorkerID:   "wkr_0000000000000002",
			ActiveVUs:  5,
			QueueDepth: 10,
		},
	}

	if err := emitter.EmitBatch(batch); err != nil {
		t.Fatalf("emit batch failed: %v", err)
	}

	if err := emitter.Flush(); err != nil {
		t.Fatalf("flush failed: %v", err)
	}

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	if len(lines) != 3 {
		t.Fatalf("expected 3 lines (2 ops + 1 health), got %d", len(lines))
	}
}

func TestBoundedQueue_ResetDropCounts(t *testing.T) {
	q := NewBoundedQueue(2)

	q.Enqueue(&TelemetryRecord{Type: "op_log", Tier: Tier0Lifecycle})
	q.Enqueue(&TelemetryRecord{Type: "op_log", Tier: Tier0Lifecycle})

	q.Enqueue(&TelemetryRecord{Type: "op_log", Tier: Tier2Verbose})
	q.Enqueue(&TelemetryRecord{Type: "op_log", Tier: Tier2Verbose})

	stats := q.Stats()
	if stats.DroppedTier2 != 2 {
		t.Fatalf("expected 2 dropped tier2, got %d", stats.DroppedTier2)
	}

	dropped2, dropped1 := q.ResetDropCounts()
	if dropped2 != 2 {
		t.Fatalf("expected reset to return 2, got %d", dropped2)
	}
	if dropped1 != 0 {
		t.Fatalf("expected reset to return 0 for tier1, got %d", dropped1)
	}

	stats = q.Stats()
	if stats.DroppedTier2 != 0 {
		t.Fatalf("expected 0 dropped tier2 after reset, got %d", stats.DroppedTier2)
	}
}

func TestBoundedQueue_Tier1Backpressure(t *testing.T) {
	q := NewBoundedQueue(3)

	for i := 0; i < 2; i++ {
		q.Enqueue(&TelemetryRecord{Type: "op_log", Tier: Tier0Lifecycle})
	}
	q.Enqueue(&TelemetryRecord{Type: "op_log", Tier: Tier1Operation})

	tier1Record := &TelemetryRecord{Type: "op_log", Tier: Tier1Operation}
	if !q.Enqueue(tier1Record) {
		t.Fatal("expected tier1 to succeed by shedding existing tier1")
	}

	stats := q.Stats()
	if stats.DroppedTier1 != 1 {
		t.Fatalf("expected 1 dropped tier1, got %d", stats.DroppedTier1)
	}
}

func TestBoundedQueue_Tier1DroppedWhenFullOfTier0(t *testing.T) {
	q := NewBoundedQueue(3)

	for i := 0; i < 3; i++ {
		q.Enqueue(&TelemetryRecord{Type: "op_log", Tier: Tier0Lifecycle})
	}

	tier1Record := &TelemetryRecord{Type: "op_log", Tier: Tier1Operation}
	if q.Enqueue(tier1Record) {
		t.Fatal("expected tier1 to be dropped when queue full of tier0")
	}

	stats := q.Stats()
	if stats.DroppedTier1 != 1 {
		t.Fatalf("expected 1 dropped tier1, got %d", stats.DroppedTier1)
	}
}

func TestBoundedQueue_DequeueBatchBlocking(t *testing.T) {
	q := NewBoundedQueue(100)

	done := make(chan struct{})
	var batch []*TelemetryRecord

	go func() {
		batch = q.DequeueBatch(5)
		close(done)
	}()

	time.Sleep(10 * time.Millisecond)

	select {
	case <-done:
		t.Fatal("DequeueBatch should be blocking")
	default:
	}

	for i := 0; i < 3; i++ {
		q.Enqueue(&TelemetryRecord{Type: "op_log", Tier: Tier1Operation})
	}

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("DequeueBatch should have unblocked")
	}

	if len(batch) != 3 {
		t.Fatalf("expected batch of 3, got %d", len(batch))
	}
}

func TestBoundedQueue_DequeueBatchClosedEmpty(t *testing.T) {
	q := NewBoundedQueue(100)

	done := make(chan struct{})
	var batch []*TelemetryRecord

	go func() {
		batch = q.DequeueBatch(5)
		close(done)
	}()

	time.Sleep(10 * time.Millisecond)
	q.Close()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("DequeueBatch should unblock on close")
	}

	if batch != nil {
		t.Fatal("expected nil batch from closed empty queue")
	}
}

func TestBoundedQueue_DefaultCapacity(t *testing.T) {
	q := NewBoundedQueue(0)
	if q.Capacity() != 10000 {
		t.Fatalf("expected default capacity 10000, got %d", q.Capacity())
	}

	q2 := NewBoundedQueue(-5)
	if q2.Capacity() != 10000 {
		t.Fatalf("expected default capacity 10000 for negative, got %d", q2.Capacity())
	}
}

func TestEmitter_EmitWorkerHealth(t *testing.T) {
	var buf bytes.Buffer
	emitter := NewEmitterWithWriter(&buf, nil)

	health := &WorkerHealth{
		Timestamp:      time.Now(),
		WorkerID:       "wkr_0000000000000001",
		CPUPercent:     50.0,
		MemBytes:       1024 * 1024,
		ActiveVUs:      10,
		ActiveSessions: 8,
	}

	if err := emitter.EmitWorkerHealth(health); err != nil {
		t.Fatalf("emit worker health failed: %v", err)
	}

	emitter.Flush()

	output := buf.String()
	if !strings.Contains(output, "wkr_0000000000000001") {
		t.Fatal("expected output to contain worker_id")
	}
}

func TestEmitter_EmitRecordTypes(t *testing.T) {
	var buf bytes.Buffer
	emitter := NewEmitterWithWriter(&buf, nil)

	opLogRecord := &TelemetryRecord{
		Type: "op_log",
		OpLog: &OpLog{
			Version:   OpLogVersion,
			Timestamp: time.Now(),
			Operation: "test",
			OK:        true,
		},
	}
	if err := emitter.EmitRecord(opLogRecord); err != nil {
		t.Fatalf("emit op_log record failed: %v", err)
	}

	healthRecord := &TelemetryRecord{
		Type: "worker_health",
		WorkerHealth: &WorkerHealth{
			Timestamp: time.Now(),
			WorkerID:  "wkr_0000000000000002",
		},
	}
	if err := emitter.EmitRecord(healthRecord); err != nil {
		t.Fatalf("emit worker_health record failed: %v", err)
	}

	unknownRecord := &TelemetryRecord{
		Type: "unknown",
	}
	if err := emitter.EmitRecord(unknownRecord); err != nil {
		t.Fatalf("emit unknown record failed: %v", err)
	}

	emitter.Flush()

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
}

func TestEmitter_NilWriter(t *testing.T) {
	emitter := &Emitter{config: DefaultEmitterConfig()}

	log := &OpLog{
		Version:   OpLogVersion,
		Timestamp: time.Now(),
		Operation: "test",
	}

	if err := emitter.EmitOpLog(log); err != nil {
		t.Fatalf("emit with nil writer should not error: %v", err)
	}

	if err := emitter.Flush(); err != nil {
		t.Fatalf("flush with nil writer should not error: %v", err)
	}

	if err := emitter.Close(); err != nil {
		t.Fatalf("close with nil writer should not error: %v", err)
	}
}

func TestCollector_DoubleStart(t *testing.T) {
	collector := NewCollector(nil, nil)

	ctx := context.Background()
	if err := collector.Start(ctx); err != nil {
		t.Fatalf("first start failed: %v", err)
	}

	if err := collector.Start(ctx); err != nil {
		t.Fatalf("second start should be no-op: %v", err)
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	collector.Stop(stopCtx)
}

func TestCollector_DoubleStop(t *testing.T) {
	collector := NewCollector(nil, nil)

	ctx := context.Background()
	collector.Start(ctx)

	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := collector.Stop(stopCtx); err != nil {
		t.Fatalf("first stop failed: %v", err)
	}

	if err := collector.Stop(stopCtx); err != nil {
		t.Fatalf("second stop should be no-op: %v", err)
	}
}

func TestCollector_RecordAfterClose(t *testing.T) {
	collector := NewCollector(nil, nil)

	ctx := context.Background()
	collector.Start(ctx)

	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	collector.Stop(stopCtx)

	keys := CorrelationKeys{
		RunID:       "run_0000000000000001",
		ExecutionID: "exe_12345678",
		Stage:       "soak",
		StageID:     "stg_0000000000000001",
		WorkerID:    "wkr_0000000000000001",
		VUID:        "vu-1",
		SessionID:   "session-123",
	}

	outcome := &transport.OperationOutcome{
		Operation: transport.OpPing,
		LatencyMs: 10,
		OK:        true,
	}

	collector.RecordOperation(outcome, keys, Tier1Operation)

	stats := collector.QueueStats()
	if stats.Depth != 0 {
		t.Fatal("expected no records after close")
	}
}

func TestCollector_Queue(t *testing.T) {
	collector := NewCollector(nil, nil)
	queue := collector.Queue()

	if queue == nil {
		t.Fatal("expected non-nil queue")
	}

	if queue.Capacity() != 10000 {
		t.Fatalf("expected default capacity 10000, got %d", queue.Capacity())
	}
}

func TestEmitter_EmitBatchNoWriter(t *testing.T) {
	emitter := &Emitter{config: DefaultEmitterConfig()}

	batch := &TelemetryBatch{
		Records: []*OpLog{
			{Version: OpLogVersion, Operation: "test"},
		},
	}

	if err := emitter.EmitBatch(batch); err != nil {
		t.Fatalf("emit batch with nil writer should not error: %v", err)
	}
}

func TestCollector_RecordWorkerHealthAfterClose(t *testing.T) {
	collector := NewCollector(nil, nil)

	ctx := context.Background()
	collector.Start(ctx)

	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	collector.Stop(stopCtx)

	health := &WorkerHealth{
		Timestamp: time.Now(),
		WorkerID:  "wkr_0000000000000002",
	}

	collector.RecordWorkerHealth(health)

	stats := collector.QueueStats()
	if stats.Depth != 0 {
		t.Fatal("expected no records after close")
	}
}

func TestCollector_RecordLifecycleEventAfterClose(t *testing.T) {
	collector := NewCollector(nil, nil)

	ctx := context.Background()
	collector.Start(ctx)

	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	collector.Stop(stopCtx)

	keys := CorrelationKeys{
		RunID:       "run_0000000000000001",
		ExecutionID: "exe_12345678",
		Stage:       "soak",
		StageID:     "stg_0000000000000001",
		WorkerID:    "wkr_0000000000000001",
		VUID:        "vu-1",
		SessionID:   "session-123",
	}

	collector.RecordLifecycleEvent("test_event", keys, nil)

	stats := collector.QueueStats()
	if stats.Depth != 0 {
		t.Fatal("expected no records after close")
	}
}
