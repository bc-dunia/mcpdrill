package scheduler

import (
	"strings"
	"sync"
	"testing"

	"github.com/bc-dunia/mcpdrill/internal/types"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry returned nil")
	}
	if r.WorkerCount() != 0 {
		t.Errorf("expected 0 workers, got %d", r.WorkerCount())
	}
}

func TestRegisterWorker(t *testing.T) {
	r := NewRegistry()

	hostInfo := types.HostInfo{
		Hostname: "worker-1",
		IPAddr:   "192.168.1.100",
		Platform: "linux/amd64",
	}
	capacity := types.WorkerCapacity{
		MaxVUs:           100,
		MaxConcurrentOps: 50,
		MaxRPS:           1000.0,
	}

	workerID, err := r.RegisterWorker(hostInfo, capacity)
	if err != nil {
		t.Fatalf("RegisterWorker failed: %v", err)
	}

	if workerID == "" {
		t.Error("expected non-empty worker ID")
	}

	if r.WorkerCount() != 1 {
		t.Errorf("expected 1 worker, got %d", r.WorkerCount())
	}

	worker, err := r.GetWorker(workerID)
	if err != nil {
		t.Fatalf("GetWorker failed: %v", err)
	}

	if worker.WorkerID != workerID {
		t.Errorf("expected worker ID %s, got %s", workerID, worker.WorkerID)
	}
	if worker.HostInfo.Hostname != hostInfo.Hostname {
		t.Errorf("expected hostname %s, got %s", hostInfo.Hostname, worker.HostInfo.Hostname)
	}
	if worker.Capacity.MaxVUs != capacity.MaxVUs {
		t.Errorf("expected max VUs %d, got %d", capacity.MaxVUs, worker.Capacity.MaxVUs)
	}
	if worker.RegisteredAt == 0 {
		t.Error("expected non-zero registered_at timestamp")
	}
	if worker.LastHeartbeat == 0 {
		t.Error("expected non-zero last_heartbeat timestamp")
	}
}

func TestRegisterMultipleWorkers(t *testing.T) {
	r := NewRegistry()

	ids := make(map[WorkerID]bool)
	for i := 0; i < 10; i++ {
		hostInfo := types.HostInfo{Hostname: "worker", IPAddr: "192.168.1.100", Platform: "linux"}
		capacity := types.WorkerCapacity{MaxVUs: 100, MaxConcurrentOps: 50, MaxRPS: 1000.0}

		workerID, err := r.RegisterWorker(hostInfo, capacity)
		if err != nil {
			t.Fatalf("RegisterWorker failed: %v", err)
		}

		if ids[workerID] {
			t.Errorf("duplicate worker ID: %s", workerID)
		}
		ids[workerID] = true
	}

	if r.WorkerCount() != 10 {
		t.Errorf("expected 10 workers, got %d", r.WorkerCount())
	}
}

func TestHeartbeat(t *testing.T) {
	r := NewRegistry()

	hostInfo := types.HostInfo{Hostname: "worker-1", IPAddr: "192.168.1.100", Platform: "linux"}
	capacity := types.WorkerCapacity{MaxVUs: 100, MaxConcurrentOps: 50, MaxRPS: 1000.0}

	workerID, _ := r.RegisterWorker(hostInfo, capacity)

	worker, _ := r.GetWorker(workerID)
	initialHeartbeat := worker.LastHeartbeat

	health := &types.WorkerHealth{
		CPUPercent:     45.5,
		MemBytes:       1024 * 1024 * 512,
		ActiveVUs:      25,
		ActiveSessions: 10,
		InFlightOps:    5,
		QueueDepth:     3,
	}

	err := r.Heartbeat(workerID, health)
	if err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}

	worker, _ = r.GetWorker(workerID)
	if worker.LastHeartbeat < initialHeartbeat {
		t.Error("expected last_heartbeat to be updated")
	}
	if worker.Health == nil {
		t.Fatal("expected health to be set")
	}
	if worker.Health.CPUPercent != health.CPUPercent {
		t.Errorf("expected CPU percent %f, got %f", health.CPUPercent, worker.Health.CPUPercent)
	}
	if worker.Health.ActiveVUs != health.ActiveVUs {
		t.Errorf("expected active VUs %d, got %d", health.ActiveVUs, worker.Health.ActiveVUs)
	}
}

func TestHeartbeatNotFound(t *testing.T) {
	r := NewRegistry()

	err := r.Heartbeat("nonexistent", nil)
	if err != ErrWorkerNotFound {
		t.Errorf("expected ErrWorkerNotFound, got %v", err)
	}
}

func TestHeartbeatNilHealth(t *testing.T) {
	r := NewRegistry()

	hostInfo := types.HostInfo{Hostname: "worker-1", IPAddr: "192.168.1.100", Platform: "linux"}
	capacity := types.WorkerCapacity{MaxVUs: 100, MaxConcurrentOps: 50, MaxRPS: 1000.0}

	workerID, _ := r.RegisterWorker(hostInfo, capacity)

	err := r.Heartbeat(workerID, nil)
	if err != nil {
		t.Fatalf("Heartbeat with nil health failed: %v", err)
	}

	worker, _ := r.GetWorker(workerID)
	if worker.Health != nil {
		t.Error("expected health to remain nil")
	}
}

func TestGetWorkerNotFound(t *testing.T) {
	r := NewRegistry()

	_, err := r.GetWorker("nonexistent")
	if err != ErrWorkerNotFound {
		t.Errorf("expected ErrWorkerNotFound, got %v", err)
	}
}

func TestGetWorkerReturnsCopy(t *testing.T) {
	r := NewRegistry()

	hostInfo := types.HostInfo{Hostname: "worker-1", IPAddr: "192.168.1.100", Platform: "linux"}
	capacity := types.WorkerCapacity{MaxVUs: 100, MaxConcurrentOps: 50, MaxRPS: 1000.0}

	workerID, _ := r.RegisterWorker(hostInfo, capacity)

	worker1, _ := r.GetWorker(workerID)
	worker2, _ := r.GetWorker(workerID)

	worker1.HostInfo.Hostname = "modified"

	if worker2.HostInfo.Hostname == "modified" {
		t.Error("GetWorker should return a copy, not a reference")
	}
}

func TestListWorkersEmpty(t *testing.T) {
	r := NewRegistry()

	workers := r.ListWorkers()
	if len(workers) != 0 {
		t.Errorf("expected 0 workers, got %d", len(workers))
	}
}

func TestListWorkersMultiple(t *testing.T) {
	r := NewRegistry()

	for i := 0; i < 5; i++ {
		hostInfo := types.HostInfo{Hostname: "worker", IPAddr: "192.168.1.100", Platform: "linux"}
		capacity := types.WorkerCapacity{MaxVUs: 100, MaxConcurrentOps: 50, MaxRPS: 1000.0}
		r.RegisterWorker(hostInfo, capacity)
	}

	workers := r.ListWorkers()
	if len(workers) != 5 {
		t.Errorf("expected 5 workers, got %d", len(workers))
	}
}

func TestListWorkersReturnsCopies(t *testing.T) {
	r := NewRegistry()

	hostInfo := types.HostInfo{Hostname: "worker-1", IPAddr: "192.168.1.100", Platform: "linux"}
	capacity := types.WorkerCapacity{MaxVUs: 100, MaxConcurrentOps: 50, MaxRPS: 1000.0}
	r.RegisterWorker(hostInfo, capacity)

	workers := r.ListWorkers()
	workers[0].HostInfo.Hostname = "modified"

	workersAgain := r.ListWorkers()
	if workersAgain[0].HostInfo.Hostname == "modified" {
		t.Error("ListWorkers should return copies, not references")
	}
}

func TestRemoveWorker(t *testing.T) {
	r := NewRegistry()

	hostInfo := types.HostInfo{Hostname: "worker-1", IPAddr: "192.168.1.100", Platform: "linux"}
	capacity := types.WorkerCapacity{MaxVUs: 100, MaxConcurrentOps: 50, MaxRPS: 1000.0}

	workerID, _ := r.RegisterWorker(hostInfo, capacity)

	if r.WorkerCount() != 1 {
		t.Errorf("expected 1 worker, got %d", r.WorkerCount())
	}

	err := r.RemoveWorker(workerID)
	if err != nil {
		t.Fatalf("RemoveWorker failed: %v", err)
	}

	if r.WorkerCount() != 0 {
		t.Errorf("expected 0 workers, got %d", r.WorkerCount())
	}

	_, err = r.GetWorker(workerID)
	if err != ErrWorkerNotFound {
		t.Errorf("expected ErrWorkerNotFound after removal, got %v", err)
	}
}

func TestRemoveWorkerNotFound(t *testing.T) {
	r := NewRegistry()

	err := r.RemoveWorker("nonexistent")
	if err != ErrWorkerNotFound {
		t.Errorf("expected ErrWorkerNotFound, got %v", err)
	}
}

func TestRegistryClose(t *testing.T) {
	r := NewRegistry()

	hostInfo := types.HostInfo{Hostname: "worker-1", IPAddr: "192.168.1.100", Platform: "linux"}
	capacity := types.WorkerCapacity{MaxVUs: 100, MaxConcurrentOps: 50, MaxRPS: 1000.0}

	workerID, _ := r.RegisterWorker(hostInfo, capacity)

	err := r.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	_, err = r.RegisterWorker(hostInfo, capacity)
	if err != ErrRegistryClosed {
		t.Errorf("expected ErrRegistryClosed after close, got %v", err)
	}

	_, err = r.GetWorker(workerID)
	if err != ErrRegistryClosed {
		t.Errorf("expected ErrRegistryClosed after close, got %v", err)
	}

	err = r.Heartbeat(workerID, nil)
	if err != ErrRegistryClosed {
		t.Errorf("expected ErrRegistryClosed after close, got %v", err)
	}

	err = r.RemoveWorker(workerID)
	if err != ErrRegistryClosed {
		t.Errorf("expected ErrRegistryClosed after close, got %v", err)
	}
}

func TestRegistryCloseIdempotent(t *testing.T) {
	r := NewRegistry()

	err := r.Close()
	if err != nil {
		t.Fatalf("first Close failed: %v", err)
	}

	err = r.Close()
	if err != nil {
		t.Fatalf("second Close failed: %v", err)
	}
}

func TestConcurrentRegister(t *testing.T) {
	r := NewRegistry()

	var wg sync.WaitGroup
	numWorkers := 100

	ids := make(chan WorkerID, numWorkers)

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hostInfo := types.HostInfo{Hostname: "worker", IPAddr: "192.168.1.100", Platform: "linux"}
			capacity := types.WorkerCapacity{MaxVUs: 100, MaxConcurrentOps: 50, MaxRPS: 1000.0}

			workerID, err := r.RegisterWorker(hostInfo, capacity)
			if err != nil {
				t.Errorf("RegisterWorker failed: %v", err)
				return
			}
			ids <- workerID
		}()
	}

	wg.Wait()
	close(ids)

	if r.WorkerCount() != numWorkers {
		t.Errorf("expected %d workers, got %d", numWorkers, r.WorkerCount())
	}

	uniqueIDs := make(map[WorkerID]bool)
	for id := range ids {
		if uniqueIDs[id] {
			t.Errorf("duplicate worker ID: %s", id)
		}
		uniqueIDs[id] = true
	}
}

func TestConcurrentHeartbeat(t *testing.T) {
	r := NewRegistry()

	hostInfo := types.HostInfo{Hostname: "worker-1", IPAddr: "192.168.1.100", Platform: "linux"}
	capacity := types.WorkerCapacity{MaxVUs: 100, MaxConcurrentOps: 50, MaxRPS: 1000.0}

	workerID, _ := r.RegisterWorker(hostInfo, capacity)

	var wg sync.WaitGroup
	numHeartbeats := 100

	for i := 0; i < numHeartbeats; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			health := &types.WorkerHealth{
				CPUPercent:     float64(i),
				MemBytes:       int64(i * 1024),
				ActiveVUs:      i,
				ActiveSessions: i,
				InFlightOps:    i,
				QueueDepth:     i,
			}
			err := r.Heartbeat(workerID, health)
			if err != nil {
				t.Errorf("Heartbeat failed: %v", err)
			}
		}(i)
	}

	wg.Wait()

	worker, _ := r.GetWorker(workerID)
	if worker.Health == nil {
		t.Error("expected health to be set after heartbeats")
	}
}

func TestConcurrentReadWrite(t *testing.T) {
	r := NewRegistry()

	hostInfo := types.HostInfo{Hostname: "worker-1", IPAddr: "192.168.1.100", Platform: "linux"}
	capacity := types.WorkerCapacity{MaxVUs: 100, MaxConcurrentOps: 50, MaxRPS: 1000.0}

	workerID, _ := r.RegisterWorker(hostInfo, capacity)

	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = r.GetWorker(workerID)
		}()
	}

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = r.ListWorkers()
		}()
	}

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			health := &types.WorkerHealth{CPUPercent: float64(i)}
			_ = r.Heartbeat(workerID, health)
		}(i)
	}

	wg.Wait()
}

func TestWorkerIDFormat(t *testing.T) {
	r := NewRegistry()

	hostInfo := types.HostInfo{Hostname: "worker-1", IPAddr: "192.168.1.100", Platform: "linux"}
	capacity := types.WorkerCapacity{MaxVUs: 100, MaxConcurrentOps: 50, MaxRPS: 1000.0}

	workerID, _ := r.RegisterWorker(hostInfo, capacity)

	if len(workerID) < 10 {
		t.Errorf("worker ID too short: %s", workerID)
	}

	if !strings.HasPrefix(string(workerID), "wkr_") {
		t.Errorf("worker ID should start with 'wkr_', got: %s", workerID)
	}
}

func TestWorkerInfoCopy(t *testing.T) {
	original := &WorkerInfo{
		WorkerID: "worker_123",
		HostInfo: types.HostInfo{
			Hostname: "host1",
			IPAddr:   "192.168.1.1",
			Platform: "linux",
		},
		Capacity: types.WorkerCapacity{
			MaxVUs:           100,
			MaxConcurrentOps: 50,
			MaxRPS:           1000.0,
		},
		RegisteredAt:  1000,
		LastHeartbeat: 2000,
		Health: &types.WorkerHealth{
			CPUPercent: 50.0,
			MemBytes:   1024,
		},
	}

	copied := original.Copy()

	if copied.WorkerID != original.WorkerID {
		t.Error("WorkerID not copied correctly")
	}

	copied.HostInfo.Hostname = "modified"
	if original.HostInfo.Hostname == "modified" {
		t.Error("Copy should be independent of original")
	}

	copied.Health.CPUPercent = 99.0
	if original.Health.CPUPercent == 99.0 {
		t.Error("Health copy should be independent of original")
	}
}

func TestWorkerInfoCopyNil(t *testing.T) {
	var w *WorkerInfo
	copied := w.Copy()
	if copied != nil {
		t.Error("Copy of nil should return nil")
	}
}

func TestWorkerInfoCopyNilHealth(t *testing.T) {
	original := &WorkerInfo{
		WorkerID: "worker_123",
		Health:   nil,
	}

	copied := original.Copy()
	if copied.Health != nil {
		t.Error("Copy should preserve nil Health")
	}
}
