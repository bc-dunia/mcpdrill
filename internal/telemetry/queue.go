package telemetry

import (
	"sync"
	"sync/atomic"
)

// BoundedQueue is a thread-safe bounded queue with tier-based backpressure.
// When the queue is full, it sheds Tier 2 records first, then Tier 1.
// Tier 0 records are never dropped.
type BoundedQueue struct {
	capacity int
	records  []*TelemetryRecord
	mu       sync.Mutex
	notEmpty *sync.Cond

	totalEnqueued atomic.Int64
	totalDequeued atomic.Int64
	droppedTier2  atomic.Int64
	droppedTier1  atomic.Int64

	closed atomic.Bool
}

// NewBoundedQueue creates a new bounded queue with the specified capacity.
func NewBoundedQueue(capacity int) *BoundedQueue {
	if capacity <= 0 {
		capacity = 10000
	}
	q := &BoundedQueue{
		capacity: capacity,
		records:  make([]*TelemetryRecord, 0, capacity),
	}
	q.notEmpty = sync.NewCond(&q.mu)
	return q
}

// Enqueue adds a record to the queue with tier-based backpressure.
// Returns true if the record was enqueued, false if it was dropped.
// Tier 0 records are never dropped - they may cause the queue to exceed capacity.
func (q *BoundedQueue) Enqueue(record *TelemetryRecord) bool {
	if q.closed.Load() {
		return false
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed.Load() {
		return false
	}

	// Tier 0 records are never dropped
	if record.Tier == Tier0Lifecycle {
		q.records = append(q.records, record)
		q.totalEnqueued.Add(1)
		q.notEmpty.Signal()
		return true
	}

	// Check if we need to apply backpressure
	if len(q.records) >= q.capacity {
		// Try to shed a Tier 2 record first
		if q.shedTier2Locked() {
			q.records = append(q.records, record)
			q.totalEnqueued.Add(1)
			q.notEmpty.Signal()
			return true
		}

		// If incoming is Tier 2, drop it
		if record.Tier == Tier2Verbose {
			q.droppedTier2.Add(1)
			return false
		}

		// Try to shed a Tier 1 record for incoming Tier 1
		if record.Tier == Tier1Operation {
			if q.shedTier1Locked() {
				q.records = append(q.records, record)
				q.totalEnqueued.Add(1)
				q.notEmpty.Signal()
				return true
			}
			// Queue is full of Tier 0, drop this Tier 1
			q.droppedTier1.Add(1)
			return false
		}
	}

	q.records = append(q.records, record)
	q.totalEnqueued.Add(1)
	q.notEmpty.Signal()
	return true
}

// shedTier2Locked removes and drops the first Tier 2 record found.
// Must be called with mu held.
func (q *BoundedQueue) shedTier2Locked() bool {
	for i, r := range q.records {
		if r.Tier == Tier2Verbose {
			q.records = append(q.records[:i], q.records[i+1:]...)
			q.droppedTier2.Add(1)
			return true
		}
	}
	return false
}

// shedTier1Locked removes and drops the first Tier 1 record found.
// Must be called with mu held.
func (q *BoundedQueue) shedTier1Locked() bool {
	for i, r := range q.records {
		if r.Tier == Tier1Operation {
			q.records = append(q.records[:i], q.records[i+1:]...)
			q.droppedTier1.Add(1)
			return true
		}
	}
	return false
}

// Dequeue removes and returns the next record from the queue.
// Blocks until a record is available or the queue is closed.
// Returns nil if the queue is closed and empty.
func (q *BoundedQueue) Dequeue() *TelemetryRecord {
	q.mu.Lock()
	defer q.mu.Unlock()

	for len(q.records) == 0 && !q.closed.Load() {
		q.notEmpty.Wait()
	}

	if len(q.records) == 0 {
		return nil
	}

	record := q.records[0]
	q.records = q.records[1:]
	q.totalDequeued.Add(1)
	return record
}

// TryDequeue attempts to dequeue a record without blocking.
// Returns nil if the queue is empty.
func (q *BoundedQueue) TryDequeue() *TelemetryRecord {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.records) == 0 {
		return nil
	}

	record := q.records[0]
	q.records = q.records[1:]
	q.totalDequeued.Add(1)
	return record
}

// DequeueBatch removes and returns up to n records from the queue.
// Blocks until at least one record is available or the queue is closed.
// Returns nil if the queue is closed and empty.
func (q *BoundedQueue) DequeueBatch(n int) []*TelemetryRecord {
	q.mu.Lock()
	defer q.mu.Unlock()

	for len(q.records) == 0 && !q.closed.Load() {
		q.notEmpty.Wait()
	}

	if len(q.records) == 0 {
		return nil
	}

	count := n
	if count > len(q.records) {
		count = len(q.records)
	}

	batch := make([]*TelemetryRecord, count)
	copy(batch, q.records[:count])
	q.records = q.records[count:]
	q.totalDequeued.Add(int64(count))
	return batch
}

// TryDequeueBatch attempts to dequeue up to n records without blocking.
// Returns nil if the queue is empty.
func (q *BoundedQueue) TryDequeueBatch(n int) []*TelemetryRecord {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.records) == 0 {
		return nil
	}

	count := n
	if count > len(q.records) {
		count = len(q.records)
	}

	batch := make([]*TelemetryRecord, count)
	copy(batch, q.records[:count])
	q.records = q.records[count:]
	q.totalDequeued.Add(int64(count))
	return batch
}

// Len returns the current number of records in the queue.
func (q *BoundedQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.records)
}

// Capacity returns the maximum capacity of the queue.
func (q *BoundedQueue) Capacity() int {
	return q.capacity
}

// Stats returns current queue statistics.
func (q *BoundedQueue) Stats() QueueStats {
	q.mu.Lock()
	depth := len(q.records)
	q.mu.Unlock()

	return QueueStats{
		Depth:         depth,
		Capacity:      q.capacity,
		TotalEnqueued: q.totalEnqueued.Load(),
		TotalDequeued: q.totalDequeued.Load(),
		DroppedTier2:  q.droppedTier2.Load(),
		DroppedTier1:  q.droppedTier1.Load(),
	}
}

// Close closes the queue, waking up any blocked consumers.
// After Close, Enqueue returns false and Dequeue returns nil when empty.
func (q *BoundedQueue) Close() {
	q.closed.Store(true)
	q.notEmpty.Broadcast()
}

// IsClosed returns whether the queue has been closed.
func (q *BoundedQueue) IsClosed() bool {
	return q.closed.Load()
}

// ResetDropCounts resets the dropped record counters and returns the previous values.
func (q *BoundedQueue) ResetDropCounts() (droppedTier2, droppedTier1 int64) {
	droppedTier2 = q.droppedTier2.Swap(0)
	droppedTier1 = q.droppedTier1.Swap(0)
	return
}
