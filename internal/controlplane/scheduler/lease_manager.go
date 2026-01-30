package scheduler

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
)

var (
	ErrLeaseNotFound      = errors.New("lease not found")
	ErrLeaseRevoked       = errors.New("lease is revoked")
	ErrLeaseExpired       = errors.New("lease is expired")
	ErrVUIDRangeOverlap   = errors.New("vu_id_range overlaps with existing active lease")
	ErrLeaseManagerClosed = errors.New("lease manager is closed")
)

const DefaultLeaseTTLMs = 60000

type LeaseManager struct {
	mu      sync.RWMutex
	leases  map[LeaseID]*Lease
	counter atomic.Int64
	ttlMs   int64
	closed  atomic.Bool
}

func NewLeaseManager(ttlMs int64) *LeaseManager {
	if ttlMs <= 0 {
		ttlMs = DefaultLeaseTTLMs
	}
	return &LeaseManager{
		leases: make(map[LeaseID]*Lease),
		ttlMs:  ttlMs,
	}
}

func (lm *LeaseManager) generateLeaseID() LeaseID {
	ts := NowMs()
	counter := lm.counter.Add(1)
	// Format: lse_{hex} to match spec regex ^lse_[0-9a-f]{8,64}$
	return LeaseID(fmt.Sprintf("lse_%x%x", ts, counter))
}

func (lm *LeaseManager) IssueLease(workerID WorkerID, assignment Assignment) (LeaseID, error) {
	if lm.closed.Load() {
		return "", ErrLeaseManagerClosed
	}

	lm.mu.Lock()
	defer lm.mu.Unlock()

	for _, lease := range lm.leases {
		if lease.State != LeaseStateActive {
			continue
		}
		if lease.Assignment.RunID != assignment.RunID {
			continue
		}
		// Only reject overlap for same run_id + same stage_id (multi-stage runs have sequential stages)
		if lease.Assignment.StageID != assignment.StageID {
			continue
		}
		if lease.Assignment.VUIDRange.Overlaps(assignment.VUIDRange) {
			return "", ErrVUIDRangeOverlap
		}
	}

	nowMs := NowMs()
	leaseID := lm.generateLeaseID()

	lease := &Lease{
		LeaseID:    leaseID,
		WorkerID:   workerID,
		Assignment: assignment,
		State:      LeaseStateActive,
		IssuedAt:   nowMs,
		ExpiresAt:  nowMs + lm.ttlMs,
	}

	lm.leases[leaseID] = lease
	return leaseID, nil
}

func (lm *LeaseManager) RenewLease(leaseID LeaseID) error {
	if lm.closed.Load() {
		return ErrLeaseManagerClosed
	}

	lm.mu.Lock()
	defer lm.mu.Unlock()

	lease, ok := lm.leases[leaseID]
	if !ok {
		return ErrLeaseNotFound
	}

	if lease.State == LeaseStateRevoked {
		return ErrLeaseRevoked
	}

	if lease.State == LeaseStateExpired {
		return ErrLeaseExpired
	}

	lease.ExpiresAt = NowMs() + lm.ttlMs
	return nil
}

func (lm *LeaseManager) RevokeLease(leaseID LeaseID) error {
	if lm.closed.Load() {
		return ErrLeaseManagerClosed
	}

	lm.mu.Lock()
	defer lm.mu.Unlock()

	lease, ok := lm.leases[leaseID]
	if !ok {
		return ErrLeaseNotFound
	}

	if lease.State == LeaseStateRevoked {
		return nil
	}

	nowMs := NowMs()
	lease.State = LeaseStateRevoked
	lease.RevokedAt = &nowMs
	return nil
}

func (lm *LeaseManager) RevokeWorkerLeases(workerID WorkerID) error {
	if lm.closed.Load() {
		return ErrLeaseManagerClosed
	}

	lm.mu.Lock()
	defer lm.mu.Unlock()

	nowMs := NowMs()
	for _, lease := range lm.leases {
		if lease.WorkerID == workerID && lease.State == LeaseStateActive {
			lease.State = LeaseStateRevoked
			lease.RevokedAt = &nowMs
		}
	}

	return nil
}

func (lm *LeaseManager) GetLease(leaseID LeaseID) (*Lease, error) {
	if lm.closed.Load() {
		return nil, ErrLeaseManagerClosed
	}

	lm.mu.RLock()
	defer lm.mu.RUnlock()

	lease, ok := lm.leases[leaseID]
	if !ok {
		return nil, ErrLeaseNotFound
	}

	return lease.Copy(), nil
}

func (lm *LeaseManager) ListLeases(runID string) []*Lease {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	var result []*Lease
	for _, lease := range lm.leases {
		if lease.Assignment.RunID == runID {
			result = append(result, lease.Copy())
		}
	}

	return result
}

func (lm *LeaseManager) ListWorkerRunIDs(workerID WorkerID) []string {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	runIDSet := make(map[string]struct{})
	for _, lease := range lm.leases {
		if lease.WorkerID == workerID && lease.State == LeaseStateActive {
			runIDSet[lease.Assignment.RunID] = struct{}{}
		}
	}

	result := make([]string, 0, len(runIDSet))
	for runID := range runIDSet {
		result = append(result, runID)
	}
	return result
}

func (lm *LeaseManager) ExpireLeases() []LeaseID {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	nowMs := NowMs()
	var expired []LeaseID

	for _, lease := range lm.leases {
		if lease.State == LeaseStateActive && nowMs > lease.ExpiresAt {
			lease.State = LeaseStateExpired
			expired = append(expired, lease.LeaseID)
		}
	}

	return expired
}

func (lm *LeaseManager) LeaseCount() int {
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	return len(lm.leases)
}

func (lm *LeaseManager) ActiveLeaseCount() int {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	count := 0
	for _, lease := range lm.leases {
		if lease.State == LeaseStateActive {
			count++
		}
	}
	return count
}

func (lm *LeaseManager) RevokeLeasesByRunAndStage(runID, stageID string) error {
	if lm.closed.Load() {
		return ErrLeaseManagerClosed
	}

	lm.mu.Lock()
	defer lm.mu.Unlock()

	nowMs := NowMs()
	for _, lease := range lm.leases {
		if lease.Assignment.RunID == runID &&
			lease.Assignment.StageID == stageID &&
			lease.State == LeaseStateActive {
			lease.State = LeaseStateRevoked
			lease.RevokedAt = &nowMs
		}
	}

	return nil
}

func (lm *LeaseManager) RevokeLeasesByRun(runID string) error {
	if lm.closed.Load() {
		return ErrLeaseManagerClosed
	}

	lm.mu.Lock()
	defer lm.mu.Unlock()

	nowMs := NowMs()
	for _, lease := range lm.leases {
		if lease.Assignment.RunID == runID && lease.State == LeaseStateActive {
			lease.State = LeaseStateRevoked
			lease.RevokedAt = &nowMs
		}
	}

	return nil
}

func (lm *LeaseManager) Close() error {
	if lm.closed.Swap(true) {
		return nil
	}

	lm.mu.Lock()
	lm.leases = make(map[LeaseID]*Lease)
	lm.mu.Unlock()

	return nil
}
