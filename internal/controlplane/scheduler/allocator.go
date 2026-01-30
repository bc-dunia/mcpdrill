package scheduler

import (
	"errors"
	"sort"
)

var (
	ErrNoWorkersAvailable     = errors.New("no workers available")
	ErrInsufficientCapacity   = errors.New("insufficient total capacity for target VUs")
	ErrInvalidTargetVUs       = errors.New("target VUs must be greater than 0")
	ErrWorkerNotInRegistry    = errors.New("worker not found in registry")
)

// Allocator computes VU range assignments using a "pack" placement algorithm.
// It assigns max VUs to each worker (sorted by capacity descending) until targetVUs reached.
// Allocator is read-only and thread-safe; it does NOT issue leases.
type Allocator struct {
	registry     *Registry
	leaseManager *LeaseManager
}

func NewAllocator(registry *Registry, lm *LeaseManager) *Allocator {
	return &Allocator{
		registry:     registry,
		leaseManager: lm,
	}
}

type workerCapacity struct {
	workerID WorkerID
	maxVUs   int
}

// ReallocateAssignments computes VU range assignments excluding specified workers.
// Returns assignments and a map of worker ID to assignment for dispatching.
func (a *Allocator) ReallocateAssignments(runID, stageID string, targetVUs int, excludeWorkers []WorkerID) ([]Assignment, map[WorkerID]Assignment, error) {
	if targetVUs <= 0 {
		return nil, nil, ErrInvalidTargetVUs
	}

	allWorkers := a.registry.ListWorkers()

	excludeMap := make(map[WorkerID]bool)
	for _, wid := range excludeWorkers {
		excludeMap[wid] = true
	}

	availableWorkers := make([]workerCapacity, 0, len(allWorkers))
	totalCapacity := 0
	for _, w := range allWorkers {
		if !excludeMap[w.WorkerID] {
			availableWorkers = append(availableWorkers, workerCapacity{
				workerID: w.WorkerID,
				maxVUs:   w.EffectiveCapacity.MaxVUs,
			})
			totalCapacity += w.EffectiveCapacity.MaxVUs
		}
	}

	if len(availableWorkers) == 0 {
		return nil, nil, ErrNoWorkersAvailable
	}

	if totalCapacity < targetVUs {
		return nil, nil, ErrInsufficientCapacity
	}

	sort.Slice(availableWorkers, func(i, j int) bool {
		return availableWorkers[i].maxVUs > availableWorkers[j].maxVUs
	})

	assignments := make([]Assignment, 0)
	workerAssignments := make(map[WorkerID]Assignment)
	vuStart := 0
	remaining := targetVUs

	for _, w := range availableWorkers {
		if remaining <= 0 {
			break
		}

		vuCount := w.maxVUs
		if vuCount > remaining {
			vuCount = remaining
		}

		assignment := Assignment{
			RunID:   runID,
			StageID: stageID,
			VUIDRange: VUIDRange{
				Start: vuStart,
				End:   vuStart + vuCount,
			},
		}
		assignments = append(assignments, assignment)
		workerAssignments[w.workerID] = assignment

		vuStart += vuCount
		remaining -= vuCount
	}

	return assignments, workerAssignments, nil
}

func (a *Allocator) AllocateAssignments(runID, stageID string, targetVUs int, workerIDs []WorkerID) ([]Assignment, map[WorkerID]Assignment, error) {
	if targetVUs <= 0 {
		return nil, nil, ErrInvalidTargetVUs
	}

	if len(workerIDs) == 0 {
		return nil, nil, ErrNoWorkersAvailable
	}

	workers := make([]workerCapacity, 0, len(workerIDs))
	totalCapacity := 0

	for _, wid := range workerIDs {
		worker, err := a.registry.GetWorker(wid)
		if err != nil {
			if errors.Is(err, ErrWorkerNotFound) {
				return nil, nil, ErrWorkerNotInRegistry
			}
			return nil, nil, err
		}
		workers = append(workers, workerCapacity{
			workerID: wid,
			maxVUs:   worker.EffectiveCapacity.MaxVUs,
		})
		totalCapacity += worker.EffectiveCapacity.MaxVUs
	}

	if totalCapacity < targetVUs {
		return nil, nil, ErrInsufficientCapacity
	}

	sort.Slice(workers, func(i, j int) bool {
		return workers[i].maxVUs > workers[j].maxVUs
	})

	assignments := make([]Assignment, 0)
	workerAssignments := make(map[WorkerID]Assignment)
	vuStart := 0
	remaining := targetVUs

	for _, w := range workers {
		if remaining <= 0 {
			break
		}

		vuCount := w.maxVUs
		if vuCount > remaining {
			vuCount = remaining
		}

		assignment := Assignment{
			RunID:   runID,
			StageID: stageID,
			VUIDRange: VUIDRange{
				Start: vuStart,
				End:   vuStart + vuCount,
			},
		}
		assignments = append(assignments, assignment)
		workerAssignments[w.workerID] = assignment

		vuStart += vuCount
		remaining -= vuCount
	}

	return assignments, workerAssignments, nil
}
