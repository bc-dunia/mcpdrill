package scheduler

// HeartbeatDetector detects workers with stale heartbeats.
// It is a stateless utility that reads from the Registry.
type HeartbeatDetector struct {
	registry *Registry
}

// NewHeartbeatDetector creates a new HeartbeatDetector.
func NewHeartbeatDetector(registry *Registry) *HeartbeatDetector {
	return &HeartbeatDetector{
		registry: registry,
	}
}

// DetectLostWorkers returns the IDs of workers whose last heartbeat
// is older than timeoutMs milliseconds.
// Returns an empty slice if no workers are lost.
// Returns an error if the registry is closed.
func (hd *HeartbeatDetector) DetectLostWorkers(timeoutMs int64) ([]WorkerID, error) {
	if hd.registry == nil {
		return nil, ErrRegistryClosed
	}

	workers := hd.registry.ListWorkers()
	now := NowMs()

	var lostWorkers []WorkerID
	for _, worker := range workers {
		age := now - worker.LastHeartbeat
		if age > timeoutMs {
			lostWorkers = append(lostWorkers, worker.WorkerID)
		}
	}

	return lostWorkers, nil
}
