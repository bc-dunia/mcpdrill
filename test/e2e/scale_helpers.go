package e2e

import (
	"context"
	"runtime"
	"sync"
	"time"
)

// ResourceStats holds resource usage statistics collected during monitoring.
type ResourceStats struct {
	PeakCPU       float64 // Peak CPU usage (approximated via goroutine count)
	PeakMemory    float64 // Peak memory usage in MB
	AvgCPU        float64 // Average CPU usage
	AvgMemory     float64 // Average memory usage in MB
	Samples       int     // Number of samples collected
	PeakGoroutines int    // Peak goroutine count
}

// ResourceMonitor tracks resource usage over time.
type ResourceMonitor struct {
	mu           sync.Mutex
	stats        ResourceStats
	totalCPU     float64
	totalMemory  float64
	ctx          context.Context
	cancel       context.CancelFunc
	samplePeriod time.Duration
	done         chan struct{}
}

// NewResourceMonitor creates a new resource monitor with the given sample period.
func NewResourceMonitor(samplePeriod time.Duration) *ResourceMonitor {
	ctx, cancel := context.WithCancel(context.Background())
	return &ResourceMonitor{
		ctx:          ctx,
		cancel:       cancel,
		samplePeriod: samplePeriod,
		done:         make(chan struct{}),
	}
}

// Start begins resource monitoring in a background goroutine.
func (m *ResourceMonitor) Start() {
	go m.monitorLoop()
}

// Stop stops the resource monitor and returns the collected stats.
func (m *ResourceMonitor) Stop() *ResourceStats {
	m.cancel()
	<-m.done

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.stats.Samples > 0 {
		m.stats.AvgCPU = m.totalCPU / float64(m.stats.Samples)
		m.stats.AvgMemory = m.totalMemory / float64(m.stats.Samples)
	}

	return &ResourceStats{
		PeakCPU:        m.stats.PeakCPU,
		PeakMemory:     m.stats.PeakMemory,
		AvgCPU:         m.stats.AvgCPU,
		AvgMemory:      m.stats.AvgMemory,
		Samples:        m.stats.Samples,
		PeakGoroutines: m.stats.PeakGoroutines,
	}
}

func (m *ResourceMonitor) monitorLoop() {
	defer close(m.done)

	ticker := time.NewTicker(m.samplePeriod)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.sample()
		}
	}
}

func (m *ResourceMonitor) sample() {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	memMB := float64(memStats.Alloc) / 1024 / 1024

	goroutines := runtime.NumGoroutine()
	cpuProxy := float64(goroutines) / 10.0
	if cpuProxy > 100 {
		cpuProxy = 100
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.stats.Samples++
	m.totalCPU += cpuProxy
	m.totalMemory += memMB

	if memMB > m.stats.PeakMemory {
		m.stats.PeakMemory = memMB
	}
	if cpuProxy > m.stats.PeakCPU {
		m.stats.PeakCPU = cpuProxy
	}
	if goroutines > m.stats.PeakGoroutines {
		m.stats.PeakGoroutines = goroutines
	}
}

// GetCurrentStats returns a snapshot of current stats without stopping the monitor.
func (m *ResourceMonitor) GetCurrentStats() *ResourceStats {
	m.mu.Lock()
	defer m.mu.Unlock()

	avgCPU := 0.0
	avgMemory := 0.0
	if m.stats.Samples > 0 {
		avgCPU = m.totalCPU / float64(m.stats.Samples)
		avgMemory = m.totalMemory / float64(m.stats.Samples)
	}

	return &ResourceStats{
		PeakCPU:        m.stats.PeakCPU,
		PeakMemory:     m.stats.PeakMemory,
		AvgCPU:         avgCPU,
		AvgMemory:      avgMemory,
		Samples:        m.stats.Samples,
		PeakGoroutines: m.stats.PeakGoroutines,
	}
}

// MonitorResources is a convenience function that starts monitoring and returns
// a channel that will receive the final stats when the context is cancelled.
func MonitorResources(ctx context.Context) <-chan *ResourceStats {
	ch := make(chan *ResourceStats, 1)

	go func() {
		monitor := NewResourceMonitor(100 * time.Millisecond)
		monitor.Start()

		<-ctx.Done()
		stats := monitor.Stop()
		ch <- stats
	}()

	return ch
}

// scaleTestWorker represents a simulated worker for scale testing.
type scaleTestWorker struct {
	id              int
	controlPlaneURL string
	workerID        string
	capacity        int
	ctx             context.Context
	cancel          context.CancelFunc
	mu              sync.Mutex
	registered      bool
	heartbeatCount  int
	errors          []error
}

// newScaleTestWorker creates a new simulated worker for scale testing.
func newScaleTestWorker(id int, controlPlaneURL string, capacity int) *scaleTestWorker {
	ctx, cancel := context.WithCancel(context.Background())
	return &scaleTestWorker{
		id:              id,
		controlPlaneURL: controlPlaneURL,
		capacity:        capacity,
		ctx:             ctx,
		cancel:          cancel,
	}
}

// stop stops the worker.
func (w *scaleTestWorker) stop() {
	w.cancel()
}

// isRegistered returns whether the worker is registered.
func (w *scaleTestWorker) isRegistered() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.registered
}

// getWorkerID returns the worker ID.
func (w *scaleTestWorker) getWorkerID() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.workerID
}

// getHeartbeatCount returns the number of heartbeats sent.
func (w *scaleTestWorker) getHeartbeatCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.heartbeatCount
}

// getErrors returns any errors encountered.
func (w *scaleTestWorker) getErrors() []error {
	w.mu.Lock()
	defer w.mu.Unlock()
	result := make([]error, len(w.errors))
	copy(result, w.errors)
	return result
}

// setRegistered marks the worker as registered with the given ID.
func (w *scaleTestWorker) setRegistered(workerID string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.registered = true
	w.workerID = workerID
}

// incrementHeartbeat increments the heartbeat counter.
func (w *scaleTestWorker) incrementHeartbeat() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.heartbeatCount++
}

// addError adds an error to the error list.
func (w *scaleTestWorker) addError(err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.errors = append(w.errors, err)
}
