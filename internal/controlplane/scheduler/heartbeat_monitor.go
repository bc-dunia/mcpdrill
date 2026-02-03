package scheduler

import (
	"log"
	"sync"
	"time"
)

const (
	// DefaultHeartbeatTimeout is the default timeout for worker heartbeats (30s = 3x heartbeat interval).
	DefaultHeartbeatTimeout = 30 * time.Second
	// DefaultMonitorInterval is the default interval for checking worker heartbeats.
	DefaultMonitorInterval = 10 * time.Second
)

// WorkerLostCallback is called when a worker is detected as dead.
// The callback receives the worker ID that was lost.
type WorkerLostCallback func(workerID WorkerID, affectedRunIDs []string)

// HeartbeatMonitor monitors worker heartbeats and removes dead workers.
// It runs a background goroutine that periodically checks for workers
// whose heartbeat has timed out.
type HeartbeatMonitor struct {
	registry     *Registry
	leaseManager *LeaseManager
	detector     *HeartbeatDetector
	timeout      time.Duration
	interval     time.Duration
	stopCh       chan struct{}
	stoppedCh    chan struct{}
	mu           sync.Mutex
	running      bool
	onWorkerLost WorkerLostCallback
}

// NewHeartbeatMonitor creates a new HeartbeatMonitor.
// If timeout or interval are zero, defaults are used.
func NewHeartbeatMonitor(registry *Registry, leaseManager *LeaseManager, timeout, interval time.Duration) *HeartbeatMonitor {
	if timeout <= 0 {
		timeout = DefaultHeartbeatTimeout
	}
	if interval <= 0 {
		interval = DefaultMonitorInterval
	}

	return &HeartbeatMonitor{
		registry:     registry,
		leaseManager: leaseManager,
		detector:     NewHeartbeatDetector(registry),
		timeout:      timeout,
		interval:     interval,
		stopCh:       make(chan struct{}),
		stoppedCh:    make(chan struct{}),
	}
}

// Start begins the heartbeat monitoring loop in a background goroutine.
// It is safe to call Start multiple times; subsequent calls are no-ops.
func (m *HeartbeatMonitor) Start() {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.running = true
	m.stopCh = make(chan struct{})
	m.stoppedCh = make(chan struct{})
	m.mu.Unlock()

	go m.run()
}

// Stop stops the heartbeat monitoring loop.
// It blocks until the monitoring goroutine has exited.
// It is safe to call Stop multiple times; subsequent calls are no-ops.
func (m *HeartbeatMonitor) Stop() {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return
	}
	m.running = false
	close(m.stopCh)
	stoppedCh := m.stoppedCh
	m.mu.Unlock()

	// Wait for the goroutine to exit
	<-stoppedCh
}

// run is the main monitoring loop.
func (m *HeartbeatMonitor) run() {
	defer close(m.stoppedCh)

	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.checkWorkers()
		case <-m.stopCh:
			return
		}
	}
}

func (m *HeartbeatMonitor) checkWorkers() {
	timeoutMs := m.timeout.Milliseconds()
	deadWorkers, err := m.detector.DetectLostWorkers(timeoutMs)
	if err != nil {
		log.Printf("heartbeat monitor: failed to detect lost workers: %v", err)
		return
	}

	for _, workerID := range deadWorkers {
		m.handleDeadWorker(workerID)
	}

	if expired := m.leaseManager.ExpireLeases(); len(expired) > 0 {
		log.Printf("heartbeat monitor: expired %d stale leases", len(expired))
	}
}

// handleDeadWorker handles a worker that has been detected as dead.
// Order of operations:
// 1. Revoke leases FIRST (so allocator doesn't use dead worker)
// 2. Remove from registry
// 3. Call onWorkerLost callback if set
// 4. Log the removal
func (m *HeartbeatMonitor) handleDeadWorker(workerID WorkerID) {
	// 1. Get affected run IDs before revoking leases
	affectedRunIDs := m.leaseManager.ListWorkerRunIDs(workerID)

	// 2. Revoke leases
	if err := m.leaseManager.RevokeWorkerLeases(workerID); err != nil {
		log.Printf("heartbeat monitor: failed to revoke leases for worker %s: %v", workerID, err)
	}

	// 3. Remove from registry
	if err := m.registry.RemoveWorker(workerID); err != nil {
		if err != ErrWorkerNotFound {
			log.Printf("heartbeat monitor: failed to remove worker %s: %v", workerID, err)
		}
		return
	}

	// 4. Call onWorkerLost callback if set
	if m.onWorkerLost != nil {
		m.onWorkerLost(workerID, affectedRunIDs)
	}

	// 5. Log the removal
	log.Printf("heartbeat monitor: worker %s removed due to heartbeat timeout", workerID)
}

// IsRunning returns true if the monitor is currently running.
func (m *HeartbeatMonitor) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// Timeout returns the configured heartbeat timeout.
func (m *HeartbeatMonitor) Timeout() time.Duration {
	return m.timeout
}

// Interval returns the configured monitoring interval.
func (m *HeartbeatMonitor) Interval() time.Duration {
	return m.interval
}

// SetOnWorkerLost sets the callback to be invoked when a worker is detected as dead.
// Must be called before Start().
func (m *HeartbeatMonitor) SetOnWorkerLost(callback WorkerLostCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onWorkerLost = callback
}
