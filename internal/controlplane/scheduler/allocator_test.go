package scheduler

import (
	"testing"

	"github.com/bc-dunia/mcpdrill/internal/types"
)

func TestNewAllocator(t *testing.T) {
	registry := NewRegistry()
	lm := NewLeaseManager(60000)
	allocator := NewAllocator(registry, lm)

	if allocator == nil {
		t.Fatal("expected non-nil allocator")
	}
	if allocator.registry != registry {
		t.Error("expected registry to be set")
	}
	if allocator.leaseManager != lm {
		t.Error("expected lease manager to be set")
	}
}

func TestAllocateAssignments_SingleWorkerFullCapacity(t *testing.T) {
	registry := NewRegistry()
	lm := NewLeaseManager(60000)
	allocator := NewAllocator(registry, lm)

	wid, _ := registry.RegisterWorker(
		types.HostInfo{Hostname: "host1"},
		types.WorkerCapacity{MaxVUs: 100},
	)

	assignments, _, err := allocator.AllocateAssignments("run1", "stage1", 100, []WorkerID{wid})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(assignments) != 1 {
		t.Fatalf("expected 1 assignment, got %d", len(assignments))
	}

	a := assignments[0]
	if a.RunID != "run1" {
		t.Errorf("expected RunID 'run1', got '%s'", a.RunID)
	}
	if a.StageID != "stage1" {
		t.Errorf("expected StageID 'stage1', got '%s'", a.StageID)
	}
	if a.VUIDRange.Start != 0 {
		t.Errorf("expected VUIDRange.Start 0, got %d", a.VUIDRange.Start)
	}
	if a.VUIDRange.End != 100 {
		t.Errorf("expected VUIDRange.End 100, got %d", a.VUIDRange.End)
	}
}

func TestAllocateAssignments_SingleWorkerPartialCapacity(t *testing.T) {
	registry := NewRegistry()
	lm := NewLeaseManager(60000)
	allocator := NewAllocator(registry, lm)

	wid, _ := registry.RegisterWorker(
		types.HostInfo{Hostname: "host1"},
		types.WorkerCapacity{MaxVUs: 100},
	)

	assignments, _, err := allocator.AllocateAssignments("run1", "stage1", 50, []WorkerID{wid})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(assignments) != 1 {
		t.Fatalf("expected 1 assignment, got %d", len(assignments))
	}

	a := assignments[0]
	if a.VUIDRange.Start != 0 || a.VUIDRange.End != 50 {
		t.Errorf("expected VUIDRange [0, 50), got [%d, %d)", a.VUIDRange.Start, a.VUIDRange.End)
	}
}

func TestAllocateAssignments_MultipleWorkersPackAlgorithm(t *testing.T) {
	registry := NewRegistry()
	lm := NewLeaseManager(60000)
	allocator := NewAllocator(registry, lm)

	wid1, _ := registry.RegisterWorker(types.HostInfo{Hostname: "host1"}, types.WorkerCapacity{MaxVUs: 100})
	wid2, _ := registry.RegisterWorker(types.HostInfo{Hostname: "host2"}, types.WorkerCapacity{MaxVUs: 100})
	wid3, _ := registry.RegisterWorker(types.HostInfo{Hostname: "host3"}, types.WorkerCapacity{MaxVUs: 100})

	assignments, _, err := allocator.AllocateAssignments("run1", "stage1", 250, []WorkerID{wid1, wid2, wid3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(assignments) != 3 {
		t.Fatalf("expected 3 assignments, got %d", len(assignments))
	}

	totalVUs := 0
	for _, a := range assignments {
		totalVUs += a.VUIDRange.End - a.VUIDRange.Start
	}
	if totalVUs != 250 {
		t.Errorf("expected total 250 VUs, got %d", totalVUs)
	}

	if assignments[0].VUIDRange.Start != 0 || assignments[0].VUIDRange.End != 100 {
		t.Errorf("expected first assignment [0, 100), got [%d, %d)", assignments[0].VUIDRange.Start, assignments[0].VUIDRange.End)
	}
	if assignments[1].VUIDRange.Start != 100 || assignments[1].VUIDRange.End != 200 {
		t.Errorf("expected second assignment [100, 200), got [%d, %d)", assignments[1].VUIDRange.Start, assignments[1].VUIDRange.End)
	}
	if assignments[2].VUIDRange.Start != 200 || assignments[2].VUIDRange.End != 250 {
		t.Errorf("expected third assignment [200, 250), got [%d, %d)", assignments[2].VUIDRange.Start, assignments[2].VUIDRange.End)
	}
}

func TestAllocateAssignments_ExactCapacityMatch(t *testing.T) {
	registry := NewRegistry()
	lm := NewLeaseManager(60000)
	allocator := NewAllocator(registry, lm)

	wid1, _ := registry.RegisterWorker(types.HostInfo{Hostname: "host1"}, types.WorkerCapacity{MaxVUs: 50})
	wid2, _ := registry.RegisterWorker(types.HostInfo{Hostname: "host2"}, types.WorkerCapacity{MaxVUs: 50})

	assignments, _, err := allocator.AllocateAssignments("run1", "stage1", 100, []WorkerID{wid1, wid2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(assignments) != 2 {
		t.Fatalf("expected 2 assignments, got %d", len(assignments))
	}

	totalVUs := 0
	for _, a := range assignments {
		totalVUs += a.VUIDRange.End - a.VUIDRange.Start
	}
	if totalVUs != 100 {
		t.Errorf("expected total 100 VUs, got %d", totalVUs)
	}
}

func TestAllocateAssignments_ExcessCapacity(t *testing.T) {
	registry := NewRegistry()
	lm := NewLeaseManager(60000)
	allocator := NewAllocator(registry, lm)

	wid1, _ := registry.RegisterWorker(types.HostInfo{Hostname: "host1"}, types.WorkerCapacity{MaxVUs: 100})
	wid2, _ := registry.RegisterWorker(types.HostInfo{Hostname: "host2"}, types.WorkerCapacity{MaxVUs: 100})
	wid3, _ := registry.RegisterWorker(types.HostInfo{Hostname: "host3"}, types.WorkerCapacity{MaxVUs: 100})

	assignments, _, err := allocator.AllocateAssignments("run1", "stage1", 150, []WorkerID{wid1, wid2, wid3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(assignments) != 2 {
		t.Fatalf("expected 2 assignments (excess capacity unused), got %d", len(assignments))
	}

	totalVUs := 0
	for _, a := range assignments {
		totalVUs += a.VUIDRange.End - a.VUIDRange.Start
	}
	if totalVUs != 150 {
		t.Errorf("expected total 150 VUs, got %d", totalVUs)
	}
}

func TestAllocateAssignments_InsufficientCapacity(t *testing.T) {
	registry := NewRegistry()
	lm := NewLeaseManager(60000)
	allocator := NewAllocator(registry, lm)

	wid1, _ := registry.RegisterWorker(types.HostInfo{Hostname: "host1"}, types.WorkerCapacity{MaxVUs: 50})
	wid2, _ := registry.RegisterWorker(types.HostInfo{Hostname: "host2"}, types.WorkerCapacity{MaxVUs: 50})

	_, _, err := allocator.AllocateAssignments("run1", "stage1", 150, []WorkerID{wid1, wid2})
	if err != ErrInsufficientCapacity {
		t.Errorf("expected ErrInsufficientCapacity, got %v", err)
	}
}

func TestAllocateAssignments_NoWorkers(t *testing.T) {
	registry := NewRegistry()
	lm := NewLeaseManager(60000)
	allocator := NewAllocator(registry, lm)

	_, _, err := allocator.AllocateAssignments("run1", "stage1", 100, []WorkerID{})
	if err != ErrNoWorkersAvailable {
		t.Errorf("expected ErrNoWorkersAvailable, got %v", err)
	}
}

func TestAllocateAssignments_TargetVUsZero(t *testing.T) {
	registry := NewRegistry()
	lm := NewLeaseManager(60000)
	allocator := NewAllocator(registry, lm)

	wid, _ := registry.RegisterWorker(types.HostInfo{Hostname: "host1"}, types.WorkerCapacity{MaxVUs: 100})

	_, _, err := allocator.AllocateAssignments("run1", "stage1", 0, []WorkerID{wid})
	if err != ErrInvalidTargetVUs {
		t.Errorf("expected ErrInvalidTargetVUs, got %v", err)
	}
}

func TestAllocateAssignments_TargetVUsNegative(t *testing.T) {
	registry := NewRegistry()
	lm := NewLeaseManager(60000)
	allocator := NewAllocator(registry, lm)

	wid, _ := registry.RegisterWorker(types.HostInfo{Hostname: "host1"}, types.WorkerCapacity{MaxVUs: 100})

	_, _, err := allocator.AllocateAssignments("run1", "stage1", -10, []WorkerID{wid})
	if err != ErrInvalidTargetVUs {
		t.Errorf("expected ErrInvalidTargetVUs, got %v", err)
	}
}

func TestAllocateAssignments_WorkerNotFound(t *testing.T) {
	registry := NewRegistry()
	lm := NewLeaseManager(60000)
	allocator := NewAllocator(registry, lm)

	_, _, err := allocator.AllocateAssignments("run1", "stage1", 100, []WorkerID{"nonexistent"})
	if err != ErrWorkerNotInRegistry {
		t.Errorf("expected ErrWorkerNotInRegistry, got %v", err)
	}
}

func TestAllocateAssignments_VURangeNonOverlapping(t *testing.T) {
	registry := NewRegistry()
	lm := NewLeaseManager(60000)
	allocator := NewAllocator(registry, lm)

	wid1, _ := registry.RegisterWorker(types.HostInfo{Hostname: "host1"}, types.WorkerCapacity{MaxVUs: 30})
	wid2, _ := registry.RegisterWorker(types.HostInfo{Hostname: "host2"}, types.WorkerCapacity{MaxVUs: 40})
	wid3, _ := registry.RegisterWorker(types.HostInfo{Hostname: "host3"}, types.WorkerCapacity{MaxVUs: 50})

	assignments, _, err := allocator.AllocateAssignments("run1", "stage1", 100, []WorkerID{wid1, wid2, wid3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for i := 0; i < len(assignments); i++ {
		for j := i + 1; j < len(assignments); j++ {
			if assignments[i].VUIDRange.Overlaps(assignments[j].VUIDRange) {
				t.Errorf("assignments %d and %d overlap: [%d, %d) and [%d, %d)",
					i, j,
					assignments[i].VUIDRange.Start, assignments[i].VUIDRange.End,
					assignments[j].VUIDRange.Start, assignments[j].VUIDRange.End)
			}
		}
	}
}

func TestAllocateAssignments_VURangeContiguous(t *testing.T) {
	registry := NewRegistry()
	lm := NewLeaseManager(60000)
	allocator := NewAllocator(registry, lm)

	wid1, _ := registry.RegisterWorker(types.HostInfo{Hostname: "host1"}, types.WorkerCapacity{MaxVUs: 30})
	wid2, _ := registry.RegisterWorker(types.HostInfo{Hostname: "host2"}, types.WorkerCapacity{MaxVUs: 40})
	wid3, _ := registry.RegisterWorker(types.HostInfo{Hostname: "host3"}, types.WorkerCapacity{MaxVUs: 50})

	assignments, _, err := allocator.AllocateAssignments("run1", "stage1", 100, []WorkerID{wid1, wid2, wid3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if assignments[0].VUIDRange.Start != 0 {
		t.Errorf("expected first assignment to start at 0, got %d", assignments[0].VUIDRange.Start)
	}

	for i := 1; i < len(assignments); i++ {
		if assignments[i].VUIDRange.Start != assignments[i-1].VUIDRange.End {
			t.Errorf("gap between assignments %d and %d: [%d, %d) and [%d, %d)",
				i-1, i,
				assignments[i-1].VUIDRange.Start, assignments[i-1].VUIDRange.End,
				assignments[i].VUIDRange.Start, assignments[i].VUIDRange.End)
		}
	}

	lastAssignment := assignments[len(assignments)-1]
	if lastAssignment.VUIDRange.End != 100 {
		t.Errorf("expected last assignment to end at 100, got %d", lastAssignment.VUIDRange.End)
	}
}

func TestAllocateAssignments_SortsByCapacityDescending(t *testing.T) {
	registry := NewRegistry()
	lm := NewLeaseManager(60000)
	allocator := NewAllocator(registry, lm)

	wid1, _ := registry.RegisterWorker(types.HostInfo{Hostname: "small"}, types.WorkerCapacity{MaxVUs: 20})
	wid2, _ := registry.RegisterWorker(types.HostInfo{Hostname: "large"}, types.WorkerCapacity{MaxVUs: 100})
	wid3, _ := registry.RegisterWorker(types.HostInfo{Hostname: "medium"}, types.WorkerCapacity{MaxVUs: 50})

	assignments, _, err := allocator.AllocateAssignments("run1", "stage1", 150, []WorkerID{wid1, wid2, wid3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(assignments) != 2 {
		t.Fatalf("expected 2 assignments (largest workers first), got %d", len(assignments))
	}

	if assignments[0].VUIDRange.End-assignments[0].VUIDRange.Start != 100 {
		t.Errorf("expected first assignment to use 100 VUs (largest worker), got %d",
			assignments[0].VUIDRange.End-assignments[0].VUIDRange.Start)
	}
	if assignments[1].VUIDRange.End-assignments[1].VUIDRange.Start != 50 {
		t.Errorf("expected second assignment to use 50 VUs (medium worker), got %d",
			assignments[1].VUIDRange.End-assignments[1].VUIDRange.Start)
	}
}

func TestAllocateAssignments_RegistryClosed(t *testing.T) {
	registry := NewRegistry()
	lm := NewLeaseManager(60000)
	allocator := NewAllocator(registry, lm)

	wid, _ := registry.RegisterWorker(types.HostInfo{Hostname: "host1"}, types.WorkerCapacity{MaxVUs: 100})
	registry.Close()

	_, _, err := allocator.AllocateAssignments("run1", "stage1", 50, []WorkerID{wid})
	if err != ErrRegistryClosed {
		t.Errorf("expected ErrRegistryClosed, got %v", err)
	}
}

func TestAllocateAssignments_MixedWorkerCapacities(t *testing.T) {
	registry := NewRegistry()
	lm := NewLeaseManager(60000)
	allocator := NewAllocator(registry, lm)

	wid1, _ := registry.RegisterWorker(types.HostInfo{Hostname: "tiny"}, types.WorkerCapacity{MaxVUs: 10})
	wid2, _ := registry.RegisterWorker(types.HostInfo{Hostname: "huge"}, types.WorkerCapacity{MaxVUs: 500})
	wid3, _ := registry.RegisterWorker(types.HostInfo{Hostname: "medium"}, types.WorkerCapacity{MaxVUs: 100})

	assignments, _, err := allocator.AllocateAssignments("run1", "stage1", 550, []WorkerID{wid1, wid2, wid3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	totalVUs := 0
	for _, a := range assignments {
		totalVUs += a.VUIDRange.End - a.VUIDRange.Start
	}
	if totalVUs != 550 {
		t.Errorf("expected total 550 VUs, got %d", totalVUs)
	}
}

func TestAllocateAssignments_SingleVU(t *testing.T) {
	registry := NewRegistry()
	lm := NewLeaseManager(60000)
	allocator := NewAllocator(registry, lm)

	wid, _ := registry.RegisterWorker(types.HostInfo{Hostname: "host1"}, types.WorkerCapacity{MaxVUs: 100})

	assignments, _, err := allocator.AllocateAssignments("run1", "stage1", 1, []WorkerID{wid})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(assignments) != 1 {
		t.Fatalf("expected 1 assignment, got %d", len(assignments))
	}

	if assignments[0].VUIDRange.Start != 0 || assignments[0].VUIDRange.End != 1 {
		t.Errorf("expected VUIDRange [0, 1), got [%d, %d)",
			assignments[0].VUIDRange.Start, assignments[0].VUIDRange.End)
	}
}

func TestReallocateAssignments_Success(t *testing.T) {
	registry := NewRegistry()
	lm := NewLeaseManager(60000)
	allocator := NewAllocator(registry, lm)

	wid1, _ := registry.RegisterWorker(types.HostInfo{Hostname: "host1"}, types.WorkerCapacity{MaxVUs: 50})
	wid2, _ := registry.RegisterWorker(types.HostInfo{Hostname: "host2"}, types.WorkerCapacity{MaxVUs: 50})
	wid3, _ := registry.RegisterWorker(types.HostInfo{Hostname: "host3"}, types.WorkerCapacity{MaxVUs: 50})

	assignments, workerMap, err := allocator.ReallocateAssignments("run1", "stage1", 100, []WorkerID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(assignments) != 2 {
		t.Fatalf("expected 2 assignments, got %d", len(assignments))
	}

	if len(workerMap) != 2 {
		t.Fatalf("expected 2 worker mappings, got %d", len(workerMap))
	}

	totalVUs := 0
	for _, a := range assignments {
		totalVUs += a.VUIDRange.End - a.VUIDRange.Start
	}
	if totalVUs != 100 {
		t.Errorf("expected total 100 VUs, got %d", totalVUs)
	}

	_ = wid1
	_ = wid2
	_ = wid3
}

func TestReallocateAssignments_ExcludeOneWorker(t *testing.T) {
	registry := NewRegistry()
	lm := NewLeaseManager(60000)
	allocator := NewAllocator(registry, lm)

	wid1, _ := registry.RegisterWorker(types.HostInfo{Hostname: "host1"}, types.WorkerCapacity{MaxVUs: 50})
	wid2, _ := registry.RegisterWorker(types.HostInfo{Hostname: "host2"}, types.WorkerCapacity{MaxVUs: 50})
	wid3, _ := registry.RegisterWorker(types.HostInfo{Hostname: "host3"}, types.WorkerCapacity{MaxVUs: 50})

	assignments, workerMap, err := allocator.ReallocateAssignments("run1", "stage1", 100, []WorkerID{wid1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(assignments) != 2 {
		t.Fatalf("expected 2 assignments, got %d", len(assignments))
	}

	if _, ok := workerMap[wid1]; ok {
		t.Error("excluded worker should not have assignment")
	}

	if _, ok := workerMap[wid2]; !ok {
		t.Error("wid2 should have assignment")
	}

	if _, ok := workerMap[wid3]; !ok {
		t.Error("wid3 should have assignment")
	}

	totalVUs := 0
	for _, a := range assignments {
		totalVUs += a.VUIDRange.End - a.VUIDRange.Start
	}
	if totalVUs != 100 {
		t.Errorf("expected total 100 VUs, got %d", totalVUs)
	}
}

func TestReallocateAssignments_ExcludeMultipleWorkers(t *testing.T) {
	registry := NewRegistry()
	lm := NewLeaseManager(60000)
	allocator := NewAllocator(registry, lm)

	wid1, _ := registry.RegisterWorker(types.HostInfo{Hostname: "host1"}, types.WorkerCapacity{MaxVUs: 50})
	wid2, _ := registry.RegisterWorker(types.HostInfo{Hostname: "host2"}, types.WorkerCapacity{MaxVUs: 50})
	wid3, _ := registry.RegisterWorker(types.HostInfo{Hostname: "host3"}, types.WorkerCapacity{MaxVUs: 100})

	assignments, workerMap, err := allocator.ReallocateAssignments("run1", "stage1", 100, []WorkerID{wid1, wid2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(assignments) != 1 {
		t.Fatalf("expected 1 assignment (only wid3 remains), got %d", len(assignments))
	}

	if _, ok := workerMap[wid1]; ok {
		t.Error("excluded worker wid1 should not have assignment")
	}

	if _, ok := workerMap[wid2]; ok {
		t.Error("excluded worker wid2 should not have assignment")
	}

	if _, ok := workerMap[wid3]; !ok {
		t.Error("wid3 should have assignment")
	}

	if assignments[0].VUIDRange.Start != 0 || assignments[0].VUIDRange.End != 100 {
		t.Errorf("expected VUIDRange [0, 100), got [%d, %d)",
			assignments[0].VUIDRange.Start, assignments[0].VUIDRange.End)
	}
}

func TestReallocateAssignments_InsufficientCapacity(t *testing.T) {
	registry := NewRegistry()
	lm := NewLeaseManager(60000)
	allocator := NewAllocator(registry, lm)

	wid1, _ := registry.RegisterWorker(types.HostInfo{Hostname: "host1"}, types.WorkerCapacity{MaxVUs: 50})
	registry.RegisterWorker(types.HostInfo{Hostname: "host2"}, types.WorkerCapacity{MaxVUs: 50})

	_, _, err := allocator.ReallocateAssignments("run1", "stage1", 100, []WorkerID{wid1})
	if err != ErrInsufficientCapacity {
		t.Errorf("expected ErrInsufficientCapacity, got %v", err)
	}
}

func TestReallocateAssignments_NoWorkersRemaining(t *testing.T) {
	registry := NewRegistry()
	lm := NewLeaseManager(60000)
	allocator := NewAllocator(registry, lm)

	wid1, _ := registry.RegisterWorker(types.HostInfo{Hostname: "host1"}, types.WorkerCapacity{MaxVUs: 50})

	_, _, err := allocator.ReallocateAssignments("run1", "stage1", 50, []WorkerID{wid1})
	if err != ErrNoWorkersAvailable {
		t.Errorf("expected ErrNoWorkersAvailable, got %v", err)
	}
}

func TestReallocateAssignments_PreservesVUIDRanges(t *testing.T) {
	registry := NewRegistry()
	lm := NewLeaseManager(60000)
	allocator := NewAllocator(registry, lm)

	wid1, _ := registry.RegisterWorker(types.HostInfo{Hostname: "host1"}, types.WorkerCapacity{MaxVUs: 30})
	registry.RegisterWorker(types.HostInfo{Hostname: "host2"}, types.WorkerCapacity{MaxVUs: 60})
	registry.RegisterWorker(types.HostInfo{Hostname: "host3"}, types.WorkerCapacity{MaxVUs: 70})

	assignments, _, err := allocator.ReallocateAssignments("run1", "stage1", 100, []WorkerID{wid1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if assignments[0].VUIDRange.Start != 0 {
		t.Errorf("expected first assignment to start at 0, got %d", assignments[0].VUIDRange.Start)
	}

	for i := 1; i < len(assignments); i++ {
		if assignments[i].VUIDRange.Start != assignments[i-1].VUIDRange.End {
			t.Errorf("gap between assignments %d and %d: [%d, %d) and [%d, %d)",
				i-1, i,
				assignments[i-1].VUIDRange.Start, assignments[i-1].VUIDRange.End,
				assignments[i].VUIDRange.Start, assignments[i].VUIDRange.End)
		}
	}

	lastAssignment := assignments[len(assignments)-1]
	if lastAssignment.VUIDRange.End != 100 {
		t.Errorf("expected last assignment to end at 100, got %d", lastAssignment.VUIDRange.End)
	}
}

func TestReallocateAssignments_InvalidTargetVUs(t *testing.T) {
	registry := NewRegistry()
	lm := NewLeaseManager(60000)
	allocator := NewAllocator(registry, lm)

	registry.RegisterWorker(types.HostInfo{Hostname: "host1"}, types.WorkerCapacity{MaxVUs: 50})

	_, _, err := allocator.ReallocateAssignments("run1", "stage1", 0, []WorkerID{})
	if err != ErrInvalidTargetVUs {
		t.Errorf("expected ErrInvalidTargetVUs for 0, got %v", err)
	}

	_, _, err = allocator.ReallocateAssignments("run1", "stage1", -10, []WorkerID{})
	if err != ErrInvalidTargetVUs {
		t.Errorf("expected ErrInvalidTargetVUs for -10, got %v", err)
	}
}

func TestReallocateAssignments_NoWorkersInRegistry(t *testing.T) {
	registry := NewRegistry()
	lm := NewLeaseManager(60000)
	allocator := NewAllocator(registry, lm)

	_, _, err := allocator.ReallocateAssignments("run1", "stage1", 50, []WorkerID{})
	if err != ErrNoWorkersAvailable {
		t.Errorf("expected ErrNoWorkersAvailable, got %v", err)
	}
}
