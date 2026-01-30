package scheduler

import (
	"sync"
	"testing"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/types"
)

func TestNewHeartbeatMonitor(t *testing.T) {
	registry := NewRegistry()
	leaseManager := NewLeaseManager(60000)

	t.Run("with defaults", func(t *testing.T) {
		monitor := NewHeartbeatMonitor(registry, leaseManager, 0, 0)
		if monitor == nil {
			t.Fatal("expected non-nil monitor")
		}
		if monitor.Timeout() != DefaultHeartbeatTimeout {
			t.Errorf("timeout = %v, want %v", monitor.Timeout(), DefaultHeartbeatTimeout)
		}
		if monitor.Interval() != DefaultMonitorInterval {
			t.Errorf("interval = %v, want %v", monitor.Interval(), DefaultMonitorInterval)
		}
	})

	t.Run("with custom values", func(t *testing.T) {
		monitor := NewHeartbeatMonitor(registry, leaseManager, 5*time.Second, 1*time.Second)
		if monitor.Timeout() != 5*time.Second {
			t.Errorf("timeout = %v, want 5s", monitor.Timeout())
		}
		if monitor.Interval() != 1*time.Second {
			t.Errorf("interval = %v, want 1s", monitor.Interval())
		}
	})
}

func TestHeartbeatMonitor_StartStop(t *testing.T) {
	registry := NewRegistry()
	leaseManager := NewLeaseManager(60000)
	monitor := NewHeartbeatMonitor(registry, leaseManager, 100*time.Millisecond, 50*time.Millisecond)

	t.Run("start and stop", func(t *testing.T) {
		if monitor.IsRunning() {
			t.Error("monitor should not be running initially")
		}

		monitor.Start()
		if !monitor.IsRunning() {
			t.Error("monitor should be running after Start")
		}

		monitor.Stop()
		if monitor.IsRunning() {
			t.Error("monitor should not be running after Stop")
		}
	})

	t.Run("multiple starts are no-ops", func(t *testing.T) {
		monitor.Start()
		monitor.Start()
		monitor.Start()
		if !monitor.IsRunning() {
			t.Error("monitor should be running")
		}
		monitor.Stop()
	})

	t.Run("multiple stops are no-ops", func(t *testing.T) {
		monitor.Start()
		monitor.Stop()
		monitor.Stop()
		monitor.Stop()
		if monitor.IsRunning() {
			t.Error("monitor should not be running")
		}
	})

	t.Run("can restart after stop", func(t *testing.T) {
		monitor.Start()
		monitor.Stop()
		monitor.Start()
		if !monitor.IsRunning() {
			t.Error("monitor should be running after restart")
		}
		monitor.Stop()
	})
}

func TestHeartbeatMonitor_DetectsTimeout(t *testing.T) {
	registry := NewRegistry()
	leaseManager := NewLeaseManager(60000)
	monitor := NewHeartbeatMonitor(registry, leaseManager, 50*time.Millisecond, 20*time.Millisecond)

	workerID, err := registry.RegisterWorker(
		types.HostInfo{Hostname: "host1"},
		types.WorkerCapacity{MaxVUs: 100},
	)
	if err != nil {
		t.Fatalf("failed to register worker: %v", err)
	}

	registry.mu.Lock()
	registry.workers[workerID].LastHeartbeat = NowMs() - 100
	registry.mu.Unlock()

	monitor.Start()
	defer monitor.Stop()

	time.Sleep(100 * time.Millisecond)

	if registry.WorkerCount() != 0 {
		t.Errorf("expected worker to be removed, got count %d", registry.WorkerCount())
	}
}

func TestHeartbeatMonitor_RemovesDeadWorker(t *testing.T) {
	registry := NewRegistry()
	leaseManager := NewLeaseManager(60000)
	monitor := NewHeartbeatMonitor(registry, leaseManager, 50*time.Millisecond, 20*time.Millisecond)

	workerID1, _ := registry.RegisterWorker(
		types.HostInfo{Hostname: "host1"},
		types.WorkerCapacity{MaxVUs: 100},
	)
	workerID2, _ := registry.RegisterWorker(
		types.HostInfo{Hostname: "host2"},
		types.WorkerCapacity{MaxVUs: 100},
	)

	registry.mu.Lock()
	registry.workers[workerID1].LastHeartbeat = NowMs() - 100
	registry.workers[workerID2].LastHeartbeat = NowMs() + 10000
	registry.mu.Unlock()

	monitor.Start()
	defer monitor.Stop()

	time.Sleep(100 * time.Millisecond)

	if registry.WorkerCount() != 1 {
		t.Errorf("expected 1 worker remaining, got %d", registry.WorkerCount())
	}

	_, err := registry.GetWorker(workerID1)
	if err != ErrWorkerNotFound {
		t.Errorf("expected dead worker to be removed, got err: %v", err)
	}

	_, err = registry.GetWorker(workerID2)
	if err != nil {
		t.Errorf("expected healthy worker to remain, got err: %v", err)
	}
}

func TestHeartbeatMonitor_RevokesLeases(t *testing.T) {
	registry := NewRegistry()
	leaseManager := NewLeaseManager(60000)
	monitor := NewHeartbeatMonitor(registry, leaseManager, 50*time.Millisecond, 20*time.Millisecond)

	workerID, _ := registry.RegisterWorker(
		types.HostInfo{Hostname: "host1"},
		types.WorkerCapacity{MaxVUs: 100},
	)

	leaseID, err := leaseManager.IssueLease(workerID, Assignment{
		RunID:     "run_0000000000000001",
		StageID:   "stage_1",
		VUIDRange: VUIDRange{0, 100},
	})
	if err != nil {
		t.Fatalf("failed to issue lease: %v", err)
	}

	registry.mu.Lock()
	registry.workers[workerID].LastHeartbeat = NowMs() - 100
	registry.mu.Unlock()

	monitor.Start()
	defer monitor.Stop()

	time.Sleep(100 * time.Millisecond)

	lease, err := leaseManager.GetLease(leaseID)
	if err != nil {
		t.Fatalf("failed to get lease: %v", err)
	}
	if lease.State != LeaseStateRevoked {
		t.Errorf("expected lease to be revoked, got state %s", lease.State)
	}
}

func TestHeartbeatMonitor_GracefulShutdown(t *testing.T) {
	registry := NewRegistry()
	leaseManager := NewLeaseManager(60000)
	monitor := NewHeartbeatMonitor(registry, leaseManager, 100*time.Millisecond, 10*time.Millisecond)

	monitor.Start()

	done := make(chan struct{})
	go func() {
		monitor.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Error("Stop did not complete within timeout")
	}
}

func TestHeartbeatMonitor_NoFalsePositives(t *testing.T) {
	registry := NewRegistry()
	leaseManager := NewLeaseManager(60000)
	monitor := NewHeartbeatMonitor(registry, leaseManager, 100*time.Millisecond, 20*time.Millisecond)

	workerID, _ := registry.RegisterWorker(
		types.HostInfo{Hostname: "host1"},
		types.WorkerCapacity{MaxVUs: 100},
	)

	monitor.Start()
	defer monitor.Stop()

	time.Sleep(80 * time.Millisecond)

	if registry.WorkerCount() != 1 {
		t.Errorf("expected worker to remain, got count %d", registry.WorkerCount())
	}

	_, err := registry.GetWorker(workerID)
	if err != nil {
		t.Errorf("expected worker to exist, got err: %v", err)
	}
}

func TestHeartbeatMonitor_MultipleDeadWorkers(t *testing.T) {
	registry := NewRegistry()
	leaseManager := NewLeaseManager(60000)
	monitor := NewHeartbeatMonitor(registry, leaseManager, 50*time.Millisecond, 20*time.Millisecond)

	workerID1, _ := registry.RegisterWorker(
		types.HostInfo{Hostname: "host1"},
		types.WorkerCapacity{MaxVUs: 100},
	)
	workerID2, _ := registry.RegisterWorker(
		types.HostInfo{Hostname: "host2"},
		types.WorkerCapacity{MaxVUs: 100},
	)
	workerID3, _ := registry.RegisterWorker(
		types.HostInfo{Hostname: "host3"},
		types.WorkerCapacity{MaxVUs: 100},
	)

	registry.mu.Lock()
	registry.workers[workerID1].LastHeartbeat = NowMs() - 100
	registry.workers[workerID2].LastHeartbeat = NowMs() - 100
	registry.workers[workerID3].LastHeartbeat = NowMs() + 10000
	registry.mu.Unlock()

	monitor.Start()
	defer monitor.Stop()

	time.Sleep(100 * time.Millisecond)

	if registry.WorkerCount() != 1 {
		t.Errorf("expected 1 worker remaining, got %d", registry.WorkerCount())
	}

	_, err := registry.GetWorker(workerID3)
	if err != nil {
		t.Errorf("expected healthy worker to remain, got err: %v", err)
	}
}

func TestHeartbeatMonitor_ConcurrentAccess(t *testing.T) {
	registry := NewRegistry()
	leaseManager := NewLeaseManager(60000)
	monitor := NewHeartbeatMonitor(registry, leaseManager, 50*time.Millisecond, 10*time.Millisecond)

	monitor.Start()
	defer monitor.Stop()

	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				workerID, _ := registry.RegisterWorker(
					types.HostInfo{Hostname: "host"},
					types.WorkerCapacity{MaxVUs: 100},
				)
				registry.Heartbeat(workerID, nil)
				time.Sleep(5 * time.Millisecond)
			}
		}()
	}

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				registry.ListWorkers()
				time.Sleep(2 * time.Millisecond)
			}
		}()
	}

	wg.Wait()
}

func TestRevokeWorkerLeases(t *testing.T) {
	t.Run("revokes all active leases for worker", func(t *testing.T) {
		lm := NewLeaseManager(60000)

		leaseID1, _ := lm.IssueLease("worker_1", Assignment{
			RunID:     "run_0000000000000001",
			StageID:   "stage_1",
			VUIDRange: VUIDRange{0, 50},
		})
		leaseID2, _ := lm.IssueLease("worker_1", Assignment{
			RunID:     "run_0000000000000002",
			StageID:   "stage_1",
			VUIDRange: VUIDRange{0, 50},
		})
		leaseID3, _ := lm.IssueLease("worker_2", Assignment{
			RunID:     "run_0000000000000001",
			StageID:   "stage_1",
			VUIDRange: VUIDRange{50, 100},
		})

		err := lm.RevokeWorkerLeases("worker_1")
		if err != nil {
			t.Fatalf("RevokeWorkerLeases failed: %v", err)
		}

		lease1, _ := lm.GetLease(leaseID1)
		if lease1.State != LeaseStateRevoked {
			t.Errorf("lease1 should be revoked, got %s", lease1.State)
		}

		lease2, _ := lm.GetLease(leaseID2)
		if lease2.State != LeaseStateRevoked {
			t.Errorf("lease2 should be revoked, got %s", lease2.State)
		}

		lease3, _ := lm.GetLease(leaseID3)
		if lease3.State != LeaseStateActive {
			t.Errorf("lease3 should remain active, got %s", lease3.State)
		}
	})

	t.Run("no-op for worker with no leases", func(t *testing.T) {
		lm := NewLeaseManager(60000)

		err := lm.RevokeWorkerLeases("nonexistent_worker")
		if err != nil {
			t.Errorf("expected no error for worker with no leases, got %v", err)
		}
	})

	t.Run("skips already revoked leases", func(t *testing.T) {
		lm := NewLeaseManager(60000)

		leaseID, _ := lm.IssueLease("worker_1", Assignment{
			RunID:     "run_0000000000000001",
			StageID:   "stage_1",
			VUIDRange: VUIDRange{0, 50},
		})

		lm.RevokeLease(leaseID)
		lease1, _ := lm.GetLease(leaseID)
		originalRevokedAt := *lease1.RevokedAt

		time.Sleep(1 * time.Millisecond)

		err := lm.RevokeWorkerLeases("worker_1")
		if err != nil {
			t.Fatalf("RevokeWorkerLeases failed: %v", err)
		}

		lease2, _ := lm.GetLease(leaseID)
		if *lease2.RevokedAt != originalRevokedAt {
			t.Error("RevokedAt should not change for already revoked lease")
		}
	})

	t.Run("closed manager", func(t *testing.T) {
		lm := NewLeaseManager(60000)
		lm.Close()

		err := lm.RevokeWorkerLeases("worker_1")
		if err != ErrLeaseManagerClosed {
			t.Errorf("expected ErrLeaseManagerClosed, got %v", err)
		}
	})

	t.Run("concurrent revocation", func(t *testing.T) {
		lm := NewLeaseManager(60000)

		for i := 0; i < 10; i++ {
			lm.IssueLease("worker_1", Assignment{
				RunID:     "run_0000000000000001",
				StageID:   "stage_1",
				VUIDRange: VUIDRange{i * 10, i*10 + 10},
			})
		}

		var wg sync.WaitGroup
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				lm.RevokeWorkerLeases("worker_1")
			}()
		}
		wg.Wait()

		if lm.ActiveLeaseCount() != 0 {
			t.Errorf("expected 0 active leases, got %d", lm.ActiveLeaseCount())
		}
	})
}
