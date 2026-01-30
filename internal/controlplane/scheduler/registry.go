package scheduler

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/bc-dunia/mcpdrill/internal/types"
)

var (
	ErrWorkerNotFound = errors.New("worker not found")
	ErrRegistryClosed = errors.New("registry is closed")
)

type Registry struct {
	mu      sync.RWMutex
	workers map[WorkerID]*WorkerInfo
	counter atomic.Int64
	closed  atomic.Bool
}

func NewRegistry() *Registry {
	return &Registry{
		workers: make(map[WorkerID]*WorkerInfo),
	}
}

func (r *Registry) generateWorkerID() WorkerID {
	ts := NowMs()
	counter := r.counter.Add(1)
	return WorkerID(fmt.Sprintf("wkr_%x%x", ts, counter))
}

func (r *Registry) RegisterWorker(hostInfo types.HostInfo, capacity types.WorkerCapacity) (WorkerID, error) {
	if r.closed.Load() {
		return "", ErrRegistryClosed
	}

	workerID := r.generateWorkerID()
	nowMs := NowMs()

	worker := &WorkerInfo{
		WorkerID:          workerID,
		HostInfo:          hostInfo,
		Capacity:          capacity,
		EffectiveCapacity: capacity,
		Saturated:         false,
		RegisteredAt:      nowMs,
		LastHeartbeat:     nowMs,
		Health:            nil,
	}

	r.mu.Lock()
	r.workers[workerID] = worker
	r.mu.Unlock()

	return workerID, nil
}

func (r *Registry) Heartbeat(workerID WorkerID, health *types.WorkerHealth) error {
	if r.closed.Load() {
		return ErrRegistryClosed
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	worker, ok := r.workers[workerID]
	if !ok {
		return ErrWorkerNotFound
	}

	worker.LastHeartbeat = NowMs()
	if health != nil {
		healthCopy := *health
		worker.Health = &healthCopy
	}

	r.updateEffectiveCapacity(worker)

	return nil
}

func (r *Registry) updateEffectiveCapacity(worker *WorkerInfo) {
	if worker.Health == nil {
		worker.Saturated = false
		worker.EffectiveCapacity = worker.Capacity
		return
	}

	const saturateThreshold = 90.0
	const unsaturateThreshold = 80.0

	if worker.Saturated {
		if worker.Health.CPUPercent < unsaturateThreshold && worker.Health.ActiveVUs < worker.Capacity.MaxVUs {
			worker.Saturated = false
			worker.EffectiveCapacity = worker.Capacity
		} else {
			worker.EffectiveCapacity = types.WorkerCapacity{MaxVUs: 0}
		}
	} else {
		if worker.Health.CPUPercent > saturateThreshold || worker.Health.ActiveVUs >= worker.Capacity.MaxVUs {
			worker.Saturated = true
			worker.EffectiveCapacity = types.WorkerCapacity{MaxVUs: 0}
		} else {
			worker.EffectiveCapacity = worker.Capacity
		}
	}
}

func (r *Registry) GetWorker(workerID WorkerID) (*WorkerInfo, error) {
	if r.closed.Load() {
		return nil, ErrRegistryClosed
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	worker, ok := r.workers[workerID]
	if !ok {
		return nil, ErrWorkerNotFound
	}

	return worker.Copy(), nil
}

func (r *Registry) ListWorkers() []*WorkerInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*WorkerInfo, 0, len(r.workers))
	for _, worker := range r.workers {
		result = append(result, worker.Copy())
	}

	return result
}

func (r *Registry) RemoveWorker(workerID WorkerID) error {
	if r.closed.Load() {
		return ErrRegistryClosed
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.workers[workerID]; !ok {
		return ErrWorkerNotFound
	}

	delete(r.workers, workerID)
	return nil
}

func (r *Registry) WorkerCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.workers)
}

func (r *Registry) Close() error {
	if r.closed.Swap(true) {
		return nil
	}

	r.mu.Lock()
	r.workers = make(map[WorkerID]*WorkerInfo)
	r.mu.Unlock()

	return nil
}
