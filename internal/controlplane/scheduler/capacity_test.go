package scheduler

import (
	"testing"

	"github.com/bc-dunia/mcpdrill/internal/types"
)

func TestUpdateEffectiveCapacity_NotSaturated(t *testing.T) {
	registry := NewRegistry()
	workerID, _ := registry.RegisterWorker(
		types.HostInfo{Hostname: "host1"},
		types.WorkerCapacity{MaxVUs: 100},
	)

	health := &types.WorkerHealth{
		CPUPercent: 50.0,
		ActiveVUs:  30,
	}
	_ = registry.Heartbeat(workerID, health)

	worker, _ := registry.GetWorker(workerID)
	if worker.Saturated {
		t.Error("expected worker to not be saturated")
	}
	if worker.EffectiveCapacity.MaxVUs != 100 {
		t.Errorf("expected EffectiveCapacity.MaxVUs=100, got %d", worker.EffectiveCapacity.MaxVUs)
	}
}

func TestUpdateEffectiveCapacity_SaturatedByCPU(t *testing.T) {
	registry := NewRegistry()
	workerID, _ := registry.RegisterWorker(
		types.HostInfo{Hostname: "host1"},
		types.WorkerCapacity{MaxVUs: 100},
	)

	health := &types.WorkerHealth{
		CPUPercent: 95.0,
		ActiveVUs:  30,
	}
	_ = registry.Heartbeat(workerID, health)

	worker, _ := registry.GetWorker(workerID)
	if !worker.Saturated {
		t.Error("expected worker to be saturated by CPU")
	}
	if worker.EffectiveCapacity.MaxVUs != 0 {
		t.Errorf("expected EffectiveCapacity.MaxVUs=0, got %d", worker.EffectiveCapacity.MaxVUs)
	}
}

func TestUpdateEffectiveCapacity_SaturatedByVUs(t *testing.T) {
	registry := NewRegistry()
	workerID, _ := registry.RegisterWorker(
		types.HostInfo{Hostname: "host1"},
		types.WorkerCapacity{MaxVUs: 100},
	)

	health := &types.WorkerHealth{
		CPUPercent: 50.0,
		ActiveVUs:  100,
	}
	_ = registry.Heartbeat(workerID, health)

	worker, _ := registry.GetWorker(workerID)
	if !worker.Saturated {
		t.Error("expected worker to be saturated by VUs")
	}
	if worker.EffectiveCapacity.MaxVUs != 0 {
		t.Errorf("expected EffectiveCapacity.MaxVUs=0, got %d", worker.EffectiveCapacity.MaxVUs)
	}
}

func TestUpdateEffectiveCapacity_Hysteresis_SaturateAt90(t *testing.T) {
	registry := NewRegistry()
	workerID, _ := registry.RegisterWorker(
		types.HostInfo{Hostname: "host1"},
		types.WorkerCapacity{MaxVUs: 100},
	)

	health85 := &types.WorkerHealth{CPUPercent: 85.0, ActiveVUs: 30}
	_ = registry.Heartbeat(workerID, health85)
	worker, _ := registry.GetWorker(workerID)
	if worker.Saturated {
		t.Error("expected worker to not be saturated at 85% CPU")
	}

	health90 := &types.WorkerHealth{CPUPercent: 90.0, ActiveVUs: 30}
	_ = registry.Heartbeat(workerID, health90)
	worker, _ = registry.GetWorker(workerID)
	if worker.Saturated {
		t.Error("expected worker to not be saturated at exactly 90% CPU (threshold is >90)")
	}

	health91 := &types.WorkerHealth{CPUPercent: 91.0, ActiveVUs: 30}
	_ = registry.Heartbeat(workerID, health91)
	worker, _ = registry.GetWorker(workerID)
	if !worker.Saturated {
		t.Error("expected worker to be saturated at 91% CPU")
	}
}

func TestUpdateEffectiveCapacity_Hysteresis_UnsaturateAt80(t *testing.T) {
	registry := NewRegistry()
	workerID, _ := registry.RegisterWorker(
		types.HostInfo{Hostname: "host1"},
		types.WorkerCapacity{MaxVUs: 100},
	)

	health95 := &types.WorkerHealth{CPUPercent: 95.0, ActiveVUs: 30}
	_ = registry.Heartbeat(workerID, health95)
	worker, _ := registry.GetWorker(workerID)
	if !worker.Saturated {
		t.Error("expected worker to be saturated at 95% CPU")
	}

	health85 := &types.WorkerHealth{CPUPercent: 85.0, ActiveVUs: 30}
	_ = registry.Heartbeat(workerID, health85)
	worker, _ = registry.GetWorker(workerID)
	if !worker.Saturated {
		t.Error("expected worker to remain saturated at 85% CPU (hysteresis)")
	}

	health80 := &types.WorkerHealth{CPUPercent: 80.0, ActiveVUs: 30}
	_ = registry.Heartbeat(workerID, health80)
	worker, _ = registry.GetWorker(workerID)
	if !worker.Saturated {
		t.Error("expected worker to remain saturated at exactly 80% CPU (threshold is <80)")
	}

	health79 := &types.WorkerHealth{CPUPercent: 79.0, ActiveVUs: 30}
	_ = registry.Heartbeat(workerID, health79)
	worker, _ = registry.GetWorker(workerID)
	if worker.Saturated {
		t.Error("expected worker to unsaturate at 79% CPU")
	}
	if worker.EffectiveCapacity.MaxVUs != 100 {
		t.Errorf("expected EffectiveCapacity.MaxVUs=100 after unsaturate, got %d", worker.EffectiveCapacity.MaxVUs)
	}
}

func TestUpdateEffectiveCapacity_NoHealthData(t *testing.T) {
	registry := NewRegistry()
	workerID, _ := registry.RegisterWorker(
		types.HostInfo{Hostname: "host1"},
		types.WorkerCapacity{MaxVUs: 100},
	)

	_ = registry.Heartbeat(workerID, nil)

	worker, _ := registry.GetWorker(workerID)
	if worker.Saturated {
		t.Error("expected worker to not be saturated with no health data")
	}
	if worker.EffectiveCapacity.MaxVUs != 100 {
		t.Errorf("expected EffectiveCapacity.MaxVUs=100, got %d", worker.EffectiveCapacity.MaxVUs)
	}
}

func TestUpdateEffectiveCapacity_Hysteresis_VUsBlockUnsaturate(t *testing.T) {
	registry := NewRegistry()
	workerID, _ := registry.RegisterWorker(
		types.HostInfo{Hostname: "host1"},
		types.WorkerCapacity{MaxVUs: 100},
	)

	health95 := &types.WorkerHealth{CPUPercent: 95.0, ActiveVUs: 30}
	_ = registry.Heartbeat(workerID, health95)
	worker, _ := registry.GetWorker(workerID)
	if !worker.Saturated {
		t.Error("expected worker to be saturated")
	}

	healthLowCPUHighVUs := &types.WorkerHealth{CPUPercent: 50.0, ActiveVUs: 100}
	_ = registry.Heartbeat(workerID, healthLowCPUHighVUs)
	worker, _ = registry.GetWorker(workerID)
	if !worker.Saturated {
		t.Error("expected worker to remain saturated due to high VUs")
	}
}

func TestAllocator_PrefersNonSaturatedWorkers(t *testing.T) {
	registry := NewRegistry()
	lm := NewLeaseManager(60000)

	worker1ID, _ := registry.RegisterWorker(
		types.HostInfo{Hostname: "host1"},
		types.WorkerCapacity{MaxVUs: 50},
	)
	worker2ID, _ := registry.RegisterWorker(
		types.HostInfo{Hostname: "host2"},
		types.WorkerCapacity{MaxVUs: 50},
	)

	health1 := &types.WorkerHealth{CPUPercent: 95.0, ActiveVUs: 30}
	_ = registry.Heartbeat(worker1ID, health1)

	health2 := &types.WorkerHealth{CPUPercent: 50.0, ActiveVUs: 10}
	_ = registry.Heartbeat(worker2ID, health2)

	allocator := NewAllocator(registry, lm)
	assignments, workerAssignments, err := allocator.ReallocateAssignments("run1", "stage1", 30, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(assignments) != 1 {
		t.Errorf("expected 1 assignment, got %d", len(assignments))
	}

	if _, ok := workerAssignments[worker1ID]; ok {
		t.Error("expected saturated worker1 to not receive assignment")
	}
	if _, ok := workerAssignments[worker2ID]; !ok {
		t.Error("expected non-saturated worker2 to receive assignment")
	}
}

func TestAllocator_SkipsSaturatedWorkers(t *testing.T) {
	registry := NewRegistry()
	lm := NewLeaseManager(60000)

	worker1ID, _ := registry.RegisterWorker(
		types.HostInfo{Hostname: "host1"},
		types.WorkerCapacity{MaxVUs: 50},
	)
	worker2ID, _ := registry.RegisterWorker(
		types.HostInfo{Hostname: "host2"},
		types.WorkerCapacity{MaxVUs: 50},
	)

	health1 := &types.WorkerHealth{CPUPercent: 95.0, ActiveVUs: 30}
	_ = registry.Heartbeat(worker1ID, health1)

	health2 := &types.WorkerHealth{CPUPercent: 95.0, ActiveVUs: 30}
	_ = registry.Heartbeat(worker2ID, health2)

	allocator := NewAllocator(registry, lm)
	_, _, err := allocator.ReallocateAssignments("run1", "stage1", 30, nil)
	if err != ErrInsufficientCapacity {
		t.Errorf("expected ErrInsufficientCapacity when all workers saturated, got %v", err)
	}
}

func TestAllocator_AllocateAssignments_UseEffectiveCapacity(t *testing.T) {
	registry := NewRegistry()
	lm := NewLeaseManager(60000)

	worker1ID, _ := registry.RegisterWorker(
		types.HostInfo{Hostname: "host1"},
		types.WorkerCapacity{MaxVUs: 100},
	)
	worker2ID, _ := registry.RegisterWorker(
		types.HostInfo{Hostname: "host2"},
		types.WorkerCapacity{MaxVUs: 100},
	)

	health1 := &types.WorkerHealth{CPUPercent: 95.0, ActiveVUs: 30}
	_ = registry.Heartbeat(worker1ID, health1)

	health2 := &types.WorkerHealth{CPUPercent: 50.0, ActiveVUs: 10}
	_ = registry.Heartbeat(worker2ID, health2)

	allocator := NewAllocator(registry, lm)
	assignments, _, err := allocator.AllocateAssignments("run1", "stage1", 50, []WorkerID{worker1ID, worker2ID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(assignments) != 1 {
		t.Errorf("expected 1 assignment (only non-saturated worker), got %d", len(assignments))
	}

	if assignments[0].VUIDRange.End-assignments[0].VUIDRange.Start != 50 {
		t.Errorf("expected assignment for 50 VUs, got %d", assignments[0].VUIDRange.End-assignments[0].VUIDRange.Start)
	}
}

func TestAllocator_AllocateAssignments_InsufficientCapacityWhenSaturated(t *testing.T) {
	registry := NewRegistry()
	lm := NewLeaseManager(60000)

	worker1ID, _ := registry.RegisterWorker(
		types.HostInfo{Hostname: "host1"},
		types.WorkerCapacity{MaxVUs: 100},
	)

	health1 := &types.WorkerHealth{CPUPercent: 95.0, ActiveVUs: 30}
	_ = registry.Heartbeat(worker1ID, health1)

	allocator := NewAllocator(registry, lm)
	_, _, err := allocator.AllocateAssignments("run1", "stage1", 50, []WorkerID{worker1ID})
	if err != ErrInsufficientCapacity {
		t.Errorf("expected ErrInsufficientCapacity when worker saturated, got %v", err)
	}
}
