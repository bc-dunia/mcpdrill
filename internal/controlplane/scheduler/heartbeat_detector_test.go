package scheduler

import (
	"testing"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/types"
)

func TestDetectLostWorkers_NoWorkers(t *testing.T) {
	registry := NewRegistry()
	detector := NewHeartbeatDetector(registry)

	lost, err := detector.DetectLostWorkers(30000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lost) != 0 {
		t.Errorf("expected empty slice, got %v", lost)
	}
}

func TestDetectLostWorkers_AllHealthy(t *testing.T) {
	registry := NewRegistry()
	detector := NewHeartbeatDetector(registry)

	_, err := registry.RegisterWorker(
		types.HostInfo{Hostname: "host1"},
		types.WorkerCapacity{MaxVUs: 100},
	)
	if err != nil {
		t.Fatalf("failed to register worker: %v", err)
	}

	_, err = registry.RegisterWorker(
		types.HostInfo{Hostname: "host2"},
		types.WorkerCapacity{MaxVUs: 100},
	)
	if err != nil {
		t.Fatalf("failed to register worker: %v", err)
	}

	lost, err := detector.DetectLostWorkers(30000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lost) != 0 {
		t.Errorf("expected no lost workers, got %d", len(lost))
	}
}

func TestDetectLostWorkers_OneLost(t *testing.T) {
	registry := NewRegistry()
	detector := NewHeartbeatDetector(registry)

	id1, _ := registry.RegisterWorker(
		types.HostInfo{Hostname: "host1"},
		types.WorkerCapacity{MaxVUs: 100},
	)

	registry.mu.Lock()
	registry.workers[id1].LastHeartbeat = NowMs() - 60000
	registry.mu.Unlock()

	lost, err := detector.DetectLostWorkers(30000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lost) != 1 {
		t.Fatalf("expected 1 lost worker, got %d", len(lost))
	}
	if lost[0] != id1 {
		t.Errorf("expected lost worker %s, got %s", id1, lost[0])
	}
}

func TestDetectLostWorkers_MultipleLost(t *testing.T) {
	registry := NewRegistry()
	detector := NewHeartbeatDetector(registry)

	id1, _ := registry.RegisterWorker(
		types.HostInfo{Hostname: "host1"},
		types.WorkerCapacity{MaxVUs: 100},
	)
	id2, _ := registry.RegisterWorker(
		types.HostInfo{Hostname: "host2"},
		types.WorkerCapacity{MaxVUs: 100},
	)
	id3, _ := registry.RegisterWorker(
		types.HostInfo{Hostname: "host3"},
		types.WorkerCapacity{MaxVUs: 100},
	)

	registry.mu.Lock()
	registry.workers[id1].LastHeartbeat = NowMs() - 60000
	registry.workers[id2].LastHeartbeat = NowMs() - 45000
	registry.mu.Unlock()

	lost, err := detector.DetectLostWorkers(30000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lost) != 2 {
		t.Fatalf("expected 2 lost workers, got %d", len(lost))
	}

	lostMap := make(map[WorkerID]bool)
	for _, id := range lost {
		lostMap[id] = true
	}
	if !lostMap[id1] || !lostMap[id2] {
		t.Errorf("expected workers %s and %s to be lost, got %v", id1, id2, lost)
	}
	if lostMap[id3] {
		t.Errorf("worker %s should not be lost", id3)
	}
}

func TestDetectLostWorkers_MixedHealthyAndLost(t *testing.T) {
	registry := NewRegistry()
	detector := NewHeartbeatDetector(registry)

	healthy1, _ := registry.RegisterWorker(
		types.HostInfo{Hostname: "healthy1"},
		types.WorkerCapacity{MaxVUs: 100},
	)
	lost1, _ := registry.RegisterWorker(
		types.HostInfo{Hostname: "lost1"},
		types.WorkerCapacity{MaxVUs: 100},
	)
	healthy2, _ := registry.RegisterWorker(
		types.HostInfo{Hostname: "healthy2"},
		types.WorkerCapacity{MaxVUs: 100},
	)

	registry.mu.Lock()
	registry.workers[lost1].LastHeartbeat = NowMs() - 60000
	registry.mu.Unlock()

	lost, err := detector.DetectLostWorkers(30000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lost) != 1 {
		t.Fatalf("expected 1 lost worker, got %d", len(lost))
	}
	if lost[0] != lost1 {
		t.Errorf("expected lost worker %s, got %s", lost1, lost[0])
	}

	_ = healthy1
	_ = healthy2
}

func TestDetectLostWorkers_ExactTimeoutBoundary(t *testing.T) {
	registry := NewRegistry()
	detector := NewHeartbeatDetector(registry)

	id1, _ := registry.RegisterWorker(
		types.HostInfo{Hostname: "host1"},
		types.WorkerCapacity{MaxVUs: 100},
	)

	timeoutMs := int64(30000)

	registry.mu.Lock()
	registry.workers[id1].LastHeartbeat = NowMs() - timeoutMs
	registry.mu.Unlock()

	lost, err := detector.DetectLostWorkers(timeoutMs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lost) != 0 {
		t.Errorf("worker at exact timeout boundary should NOT be lost, got %v", lost)
	}

	registry.mu.Lock()
	registry.workers[id1].LastHeartbeat = NowMs() - timeoutMs - 1
	registry.mu.Unlock()

	lost, err = detector.DetectLostWorkers(timeoutMs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lost) != 1 {
		t.Errorf("worker 1ms past timeout should be lost, got %d workers", len(lost))
	}
}

func TestDetectLostWorkers_VeryOldHeartbeat(t *testing.T) {
	registry := NewRegistry()
	detector := NewHeartbeatDetector(registry)

	id1, _ := registry.RegisterWorker(
		types.HostInfo{Hostname: "host1"},
		types.WorkerCapacity{MaxVUs: 100},
	)

	registry.mu.Lock()
	registry.workers[id1].LastHeartbeat = NowMs() - (24 * 60 * 60 * 1000)
	registry.mu.Unlock()

	lost, err := detector.DetectLostWorkers(30000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lost) != 1 {
		t.Fatalf("expected 1 lost worker with very old heartbeat, got %d", len(lost))
	}
	if lost[0] != id1 {
		t.Errorf("expected lost worker %s, got %s", id1, lost[0])
	}
}

func TestDetectLostWorkers_NilRegistry(t *testing.T) {
	detector := &HeartbeatDetector{registry: nil}

	_, err := detector.DetectLostWorkers(30000)
	if err != ErrRegistryClosed {
		t.Errorf("expected ErrRegistryClosed, got %v", err)
	}
}

func TestDetectLostWorkers_ZeroTimeout(t *testing.T) {
	registry := NewRegistry()
	detector := NewHeartbeatDetector(registry)

	_, _ = registry.RegisterWorker(
		types.HostInfo{Hostname: "host1"},
		types.WorkerCapacity{MaxVUs: 100},
	)

	time.Sleep(1 * time.Millisecond)

	lost, err := detector.DetectLostWorkers(0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lost) != 1 {
		t.Errorf("with zero timeout, all workers should be lost, got %d", len(lost))
	}
}

func TestDetectLostWorkers_ConcurrentAccess(t *testing.T) {
	registry := NewRegistry()
	detector := NewHeartbeatDetector(registry)

	for i := 0; i < 10; i++ {
		_, _ = registry.RegisterWorker(
			types.HostInfo{Hostname: "host"},
			types.WorkerCapacity{MaxVUs: 100},
		)
	}

	done := make(chan bool)
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_, _ = detector.DetectLostWorkers(30000)
			}
			done <- true
		}()
	}

	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_, _ = registry.RegisterWorker(
					types.HostInfo{Hostname: "new"},
					types.WorkerCapacity{MaxVUs: 100},
				)
			}
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestNewHeartbeatDetector(t *testing.T) {
	registry := NewRegistry()
	detector := NewHeartbeatDetector(registry)

	if detector == nil {
		t.Fatal("expected non-nil detector")
	}
	if detector.registry != registry {
		t.Error("detector should reference the provided registry")
	}
}
