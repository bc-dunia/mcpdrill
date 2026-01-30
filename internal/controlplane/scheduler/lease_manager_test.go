package scheduler

import (
	"sync"
	"testing"
	"time"
)

func TestVUIDRangeOverlaps(t *testing.T) {
	tests := []struct {
		name     string
		r1       VUIDRange
		r2       VUIDRange
		expected bool
	}{
		{"no overlap - r1 before r2", VUIDRange{0, 10}, VUIDRange{10, 20}, false},
		{"no overlap - r2 before r1", VUIDRange{10, 20}, VUIDRange{0, 10}, false},
		{"overlap - r1 contains r2", VUIDRange{0, 20}, VUIDRange{5, 15}, true},
		{"overlap - r2 contains r1", VUIDRange{5, 15}, VUIDRange{0, 20}, true},
		{"overlap - partial", VUIDRange{0, 15}, VUIDRange{10, 20}, true},
		{"overlap - same range", VUIDRange{0, 10}, VUIDRange{0, 10}, true},
		{"no overlap - adjacent", VUIDRange{0, 10}, VUIDRange{10, 20}, false},
		{"overlap - single point", VUIDRange{0, 11}, VUIDRange{10, 20}, true},
		{"empty range r1 - no overlap", VUIDRange{5, 5}, VUIDRange{0, 10}, false},
		{"empty range r2 - no overlap", VUIDRange{0, 10}, VUIDRange{5, 5}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.r1.Overlaps(tt.r2)
			if result != tt.expected {
				t.Errorf("VUIDRange%v.Overlaps(%v) = %v, want %v", tt.r1, tt.r2, result, tt.expected)
			}
		})
	}
}

func TestNewLeaseManager(t *testing.T) {
	t.Run("with custom TTL", func(t *testing.T) {
		lm := NewLeaseManager(30000)
		if lm == nil {
			t.Fatal("NewLeaseManager returned nil")
		}
		if lm.ttlMs != 30000 {
			t.Errorf("ttlMs = %d, want 30000", lm.ttlMs)
		}
	})

	t.Run("with zero TTL uses default", func(t *testing.T) {
		lm := NewLeaseManager(0)
		if lm.ttlMs != DefaultLeaseTTLMs {
			t.Errorf("ttlMs = %d, want %d", lm.ttlMs, DefaultLeaseTTLMs)
		}
	})

	t.Run("with negative TTL uses default", func(t *testing.T) {
		lm := NewLeaseManager(-1000)
		if lm.ttlMs != DefaultLeaseTTLMs {
			t.Errorf("ttlMs = %d, want %d", lm.ttlMs, DefaultLeaseTTLMs)
		}
	})
}

func TestIssueLease(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		lm := NewLeaseManager(60000)
		assignment := Assignment{
			RunID:     "run_0000000000000001",
			StageID:   "stage_1",
			VUIDRange: VUIDRange{0, 100},
		}

		leaseID, err := lm.IssueLease("worker_1", assignment)
		if err != nil {
			t.Fatalf("IssueLease failed: %v", err)
		}

		if leaseID == "" {
			t.Error("leaseID is empty")
		}

		lease, err := lm.GetLease(leaseID)
		if err != nil {
			t.Fatalf("GetLease failed: %v", err)
		}

		if lease.State != LeaseStateActive {
			t.Errorf("State = %s, want %s", lease.State, LeaseStateActive)
		}
		if lease.WorkerID != "worker_1" {
			t.Errorf("WorkerID = %s, want worker_1", lease.WorkerID)
		}
		if lease.Assignment.RunID != "run_0000000000000001" {
			t.Errorf("Assignment.RunID = %s, want run_0000000000000001", lease.Assignment.RunID)
		}
	})

	t.Run("overlap detection - same run", func(t *testing.T) {
		lm := NewLeaseManager(60000)

		_, err := lm.IssueLease("worker_1", Assignment{
			RunID:     "run_0000000000000001",
			StageID:   "stage_1",
			VUIDRange: VUIDRange{0, 100},
		})
		if err != nil {
			t.Fatalf("First IssueLease failed: %v", err)
		}

		_, err = lm.IssueLease("worker_2", Assignment{
			RunID:     "run_0000000000000001",
			StageID:   "stage_1",
			VUIDRange: VUIDRange{50, 150},
		})
		if err != ErrVUIDRangeOverlap {
			t.Errorf("Expected ErrVUIDRangeOverlap, got %v", err)
		}
	})

	t.Run("no overlap - different runs", func(t *testing.T) {
		lm := NewLeaseManager(60000)

		_, err := lm.IssueLease("worker_1", Assignment{
			RunID:     "run_0000000000000001",
			StageID:   "stage_1",
			VUIDRange: VUIDRange{0, 100},
		})
		if err != nil {
			t.Fatalf("First IssueLease failed: %v", err)
		}

		_, err = lm.IssueLease("worker_2", Assignment{
			RunID:     "run_0000000000000002",
			StageID:   "stage_1",
			VUIDRange: VUIDRange{0, 100},
		})
		if err != nil {
			t.Errorf("Second IssueLease should succeed for different run: %v", err)
		}
	})

	t.Run("no overlap - adjacent ranges", func(t *testing.T) {
		lm := NewLeaseManager(60000)

		_, err := lm.IssueLease("worker_1", Assignment{
			RunID:     "run_0000000000000001",
			StageID:   "stage_1",
			VUIDRange: VUIDRange{0, 100},
		})
		if err != nil {
			t.Fatalf("First IssueLease failed: %v", err)
		}

		_, err = lm.IssueLease("worker_2", Assignment{
			RunID:     "run_0000000000000001",
			StageID:   "stage_1",
			VUIDRange: VUIDRange{100, 200},
		})
		if err != nil {
			t.Errorf("Adjacent ranges should not overlap: %v", err)
		}
	})

	t.Run("overlap check ignores revoked leases", func(t *testing.T) {
		lm := NewLeaseManager(60000)

		leaseID, _ := lm.IssueLease("worker_1", Assignment{
			RunID:     "run_0000000000000001",
			StageID:   "stage_1",
			VUIDRange: VUIDRange{0, 100},
		})

		lm.RevokeLease(leaseID)

		_, err := lm.IssueLease("worker_2", Assignment{
			RunID:     "run_0000000000000001",
			StageID:   "stage_1",
			VUIDRange: VUIDRange{0, 100},
		})
		if err != nil {
			t.Errorf("Should allow overlapping range after revocation: %v", err)
		}
	})

	t.Run("closed manager", func(t *testing.T) {
		lm := NewLeaseManager(60000)
		lm.Close()

		_, err := lm.IssueLease("worker_1", Assignment{
			RunID:     "run_0000000000000001",
			StageID:   "stage_1",
			VUIDRange: VUIDRange{0, 100},
		})
		if err != ErrLeaseManagerClosed {
			t.Errorf("Expected ErrLeaseManagerClosed, got %v", err)
		}
	})
}

func TestRenewLease(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		lm := NewLeaseManager(60000)
		leaseID, _ := lm.IssueLease("worker_1", Assignment{
			RunID:     "run_0000000000000001",
			StageID:   "stage_1",
			VUIDRange: VUIDRange{0, 100},
		})

		leaseBefore, _ := lm.GetLease(leaseID)
		expiresAtBefore := leaseBefore.ExpiresAt

		time.Sleep(10 * time.Millisecond)

		err := lm.RenewLease(leaseID)
		if err != nil {
			t.Fatalf("RenewLease failed: %v", err)
		}

		leaseAfter, _ := lm.GetLease(leaseID)
		if leaseAfter.ExpiresAt <= expiresAtBefore {
			t.Errorf("ExpiresAt should be extended: before=%d, after=%d", expiresAtBefore, leaseAfter.ExpiresAt)
		}
	})

	t.Run("not found", func(t *testing.T) {
		lm := NewLeaseManager(60000)
		err := lm.RenewLease("nonexistent")
		if err != ErrLeaseNotFound {
			t.Errorf("Expected ErrLeaseNotFound, got %v", err)
		}
	})

	t.Run("revoked lease", func(t *testing.T) {
		lm := NewLeaseManager(60000)
		leaseID, _ := lm.IssueLease("worker_1", Assignment{
			RunID:     "run_0000000000000001",
			StageID:   "stage_1",
			VUIDRange: VUIDRange{0, 100},
		})

		lm.RevokeLease(leaseID)

		err := lm.RenewLease(leaseID)
		if err != ErrLeaseRevoked {
			t.Errorf("Expected ErrLeaseRevoked, got %v", err)
		}
	})

	t.Run("expired lease", func(t *testing.T) {
		lm := NewLeaseManager(1)
		leaseID, _ := lm.IssueLease("worker_1", Assignment{
			RunID:     "run_0000000000000001",
			StageID:   "stage_1",
			VUIDRange: VUIDRange{0, 100},
		})

		time.Sleep(10 * time.Millisecond)
		lm.ExpireLeases()

		err := lm.RenewLease(leaseID)
		if err != ErrLeaseExpired {
			t.Errorf("Expected ErrLeaseExpired, got %v", err)
		}
	})

	t.Run("closed manager", func(t *testing.T) {
		lm := NewLeaseManager(60000)
		leaseID, _ := lm.IssueLease("worker_1", Assignment{
			RunID:     "run_0000000000000001",
			StageID:   "stage_1",
			VUIDRange: VUIDRange{0, 100},
		})

		lm.Close()

		err := lm.RenewLease(leaseID)
		if err != ErrLeaseManagerClosed {
			t.Errorf("Expected ErrLeaseManagerClosed, got %v", err)
		}
	})
}

func TestRevokeLease(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		lm := NewLeaseManager(60000)
		leaseID, _ := lm.IssueLease("worker_1", Assignment{
			RunID:     "run_0000000000000001",
			StageID:   "stage_1",
			VUIDRange: VUIDRange{0, 100},
		})

		err := lm.RevokeLease(leaseID)
		if err != nil {
			t.Fatalf("RevokeLease failed: %v", err)
		}

		lease, _ := lm.GetLease(leaseID)
		if lease.State != LeaseStateRevoked {
			t.Errorf("State = %s, want %s", lease.State, LeaseStateRevoked)
		}
		if lease.RevokedAt == nil {
			t.Error("RevokedAt should be set")
		}
	})

	t.Run("already revoked - idempotent", func(t *testing.T) {
		lm := NewLeaseManager(60000)
		leaseID, _ := lm.IssueLease("worker_1", Assignment{
			RunID:     "run_0000000000000001",
			StageID:   "stage_1",
			VUIDRange: VUIDRange{0, 100},
		})

		lm.RevokeLease(leaseID)
		err := lm.RevokeLease(leaseID)
		if err != nil {
			t.Errorf("Second RevokeLease should be idempotent: %v", err)
		}
	})

	t.Run("not found", func(t *testing.T) {
		lm := NewLeaseManager(60000)
		err := lm.RevokeLease("nonexistent")
		if err != ErrLeaseNotFound {
			t.Errorf("Expected ErrLeaseNotFound, got %v", err)
		}
	})

	t.Run("closed manager", func(t *testing.T) {
		lm := NewLeaseManager(60000)
		leaseID, _ := lm.IssueLease("worker_1", Assignment{
			RunID:     "run_0000000000000001",
			StageID:   "stage_1",
			VUIDRange: VUIDRange{0, 100},
		})

		lm.Close()

		err := lm.RevokeLease(leaseID)
		if err != ErrLeaseManagerClosed {
			t.Errorf("Expected ErrLeaseManagerClosed, got %v", err)
		}
	})
}

func TestGetLease(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		lm := NewLeaseManager(60000)
		leaseID, _ := lm.IssueLease("worker_1", Assignment{
			RunID:     "run_0000000000000001",
			StageID:   "stage_1",
			VUIDRange: VUIDRange{0, 100},
		})

		lease, err := lm.GetLease(leaseID)
		if err != nil {
			t.Fatalf("GetLease failed: %v", err)
		}

		if lease.LeaseID != leaseID {
			t.Errorf("LeaseID = %s, want %s", lease.LeaseID, leaseID)
		}
	})

	t.Run("not found", func(t *testing.T) {
		lm := NewLeaseManager(60000)
		_, err := lm.GetLease("nonexistent")
		if err != ErrLeaseNotFound {
			t.Errorf("Expected ErrLeaseNotFound, got %v", err)
		}
	})

	t.Run("returns copy", func(t *testing.T) {
		lm := NewLeaseManager(60000)
		leaseID, _ := lm.IssueLease("worker_1", Assignment{
			RunID:     "run_0000000000000001",
			StageID:   "stage_1",
			VUIDRange: VUIDRange{0, 100},
		})

		lease1, _ := lm.GetLease(leaseID)
		lease1.State = LeaseStateRevoked

		lease2, _ := lm.GetLease(leaseID)
		if lease2.State != LeaseStateActive {
			t.Error("GetLease should return a copy, not the original")
		}
	})

	t.Run("closed manager", func(t *testing.T) {
		lm := NewLeaseManager(60000)
		leaseID, _ := lm.IssueLease("worker_1", Assignment{
			RunID:     "run_0000000000000001",
			StageID:   "stage_1",
			VUIDRange: VUIDRange{0, 100},
		})

		lm.Close()

		_, err := lm.GetLease(leaseID)
		if err != ErrLeaseManagerClosed {
			t.Errorf("Expected ErrLeaseManagerClosed, got %v", err)
		}
	})
}

func TestListLeases(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		lm := NewLeaseManager(60000)
		leases := lm.ListLeases("run_0000000000000001")
		if len(leases) != 0 {
			t.Errorf("Expected empty list, got %d leases", len(leases))
		}
	})

	t.Run("filter by run", func(t *testing.T) {
		lm := NewLeaseManager(60000)

		lm.IssueLease("worker_1", Assignment{
			RunID:     "run_0000000000000001",
			StageID:   "stage_1",
			VUIDRange: VUIDRange{0, 100},
		})
		lm.IssueLease("worker_2", Assignment{
			RunID:     "run_0000000000000001",
			StageID:   "stage_1",
			VUIDRange: VUIDRange{100, 200},
		})
		lm.IssueLease("worker_3", Assignment{
			RunID:     "run_0000000000000002",
			StageID:   "stage_1",
			VUIDRange: VUIDRange{0, 100},
		})

		run1Leases := lm.ListLeases("run_0000000000000001")
		if len(run1Leases) != 2 {
			t.Errorf("Expected 2 leases for run_1, got %d", len(run1Leases))
		}

		run2Leases := lm.ListLeases("run_0000000000000002")
		if len(run2Leases) != 1 {
			t.Errorf("Expected 1 lease for run_2, got %d", len(run2Leases))
		}
	})

	t.Run("includes all states", func(t *testing.T) {
		lm := NewLeaseManager(60000)

		leaseID1, _ := lm.IssueLease("worker_1", Assignment{
			RunID:     "run_0000000000000001",
			StageID:   "stage_1",
			VUIDRange: VUIDRange{0, 100},
		})
		lm.IssueLease("worker_2", Assignment{
			RunID:     "run_0000000000000001",
			StageID:   "stage_1",
			VUIDRange: VUIDRange{100, 200},
		})

		lm.RevokeLease(leaseID1)

		leases := lm.ListLeases("run_0000000000000001")
		if len(leases) != 2 {
			t.Errorf("Expected 2 leases (including revoked), got %d", len(leases))
		}
	})

	t.Run("returns copies", func(t *testing.T) {
		lm := NewLeaseManager(60000)
		lm.IssueLease("worker_1", Assignment{
			RunID:     "run_0000000000000001",
			StageID:   "stage_1",
			VUIDRange: VUIDRange{0, 100},
		})

		leases := lm.ListLeases("run_0000000000000001")
		leases[0].State = LeaseStateRevoked

		leasesAgain := lm.ListLeases("run_0000000000000001")
		if leasesAgain[0].State != LeaseStateActive {
			t.Error("ListLeases should return copies")
		}
	})
}

func TestExpireLeases(t *testing.T) {
	t.Run("no leases", func(t *testing.T) {
		lm := NewLeaseManager(60000)
		expired := lm.ExpireLeases()
		if len(expired) != 0 {
			t.Errorf("Expected no expired leases, got %d", len(expired))
		}
	})

	t.Run("all active - none expired", func(t *testing.T) {
		lm := NewLeaseManager(60000)
		lm.IssueLease("worker_1", Assignment{
			RunID:     "run_0000000000000001",
			StageID:   "stage_1",
			VUIDRange: VUIDRange{0, 100},
		})

		expired := lm.ExpireLeases()
		if len(expired) != 0 {
			t.Errorf("Expected no expired leases, got %d", len(expired))
		}
	})

	t.Run("some expired", func(t *testing.T) {
		lm := NewLeaseManager(1)

		leaseID1, _ := lm.IssueLease("worker_1", Assignment{
			RunID:     "run_0000000000000001",
			StageID:   "stage_1",
			VUIDRange: VUIDRange{0, 100},
		})

		time.Sleep(10 * time.Millisecond)

		expired := lm.ExpireLeases()
		if len(expired) != 1 {
			t.Errorf("Expected 1 expired lease, got %d", len(expired))
		}
		if expired[0] != leaseID1 {
			t.Errorf("Expected %s to be expired, got %s", leaseID1, expired[0])
		}

		lease, _ := lm.GetLease(leaseID1)
		if lease.State != LeaseStateExpired {
			t.Errorf("State = %s, want %s", lease.State, LeaseStateExpired)
		}
	})

	t.Run("skips revoked leases", func(t *testing.T) {
		lm := NewLeaseManager(1)

		leaseID, _ := lm.IssueLease("worker_1", Assignment{
			RunID:     "run_0000000000000001",
			StageID:   "stage_1",
			VUIDRange: VUIDRange{0, 100},
		})

		lm.RevokeLease(leaseID)
		time.Sleep(10 * time.Millisecond)

		expired := lm.ExpireLeases()
		if len(expired) != 0 {
			t.Errorf("Revoked leases should not be expired, got %d", len(expired))
		}
	})

	t.Run("skips already expired leases", func(t *testing.T) {
		lm := NewLeaseManager(1)

		lm.IssueLease("worker_1", Assignment{
			RunID:     "run_0000000000000001",
			StageID:   "stage_1",
			VUIDRange: VUIDRange{0, 100},
		})

		time.Sleep(10 * time.Millisecond)

		expired1 := lm.ExpireLeases()
		if len(expired1) != 1 {
			t.Errorf("First call: expected 1 expired, got %d", len(expired1))
		}

		expired2 := lm.ExpireLeases()
		if len(expired2) != 0 {
			t.Errorf("Second call: expected 0 expired (already expired), got %d", len(expired2))
		}
	})
}

func TestLeaseCount(t *testing.T) {
	lm := NewLeaseManager(60000)

	if lm.LeaseCount() != 0 {
		t.Errorf("Initial count should be 0, got %d", lm.LeaseCount())
	}

	lm.IssueLease("worker_1", Assignment{
		RunID:     "run_0000000000000001",
		StageID:   "stage_1",
		VUIDRange: VUIDRange{0, 100},
	})

	if lm.LeaseCount() != 1 {
		t.Errorf("Count should be 1, got %d", lm.LeaseCount())
	}
}

func TestActiveLeaseCount(t *testing.T) {
	lm := NewLeaseManager(60000)

	leaseID1, _ := lm.IssueLease("worker_1", Assignment{
		RunID:     "run_0000000000000001",
		StageID:   "stage_1",
		VUIDRange: VUIDRange{0, 100},
	})
	lm.IssueLease("worker_2", Assignment{
		RunID:     "run_0000000000000001",
		StageID:   "stage_1",
		VUIDRange: VUIDRange{100, 200},
	})

	if lm.ActiveLeaseCount() != 2 {
		t.Errorf("Active count should be 2, got %d", lm.ActiveLeaseCount())
	}

	lm.RevokeLease(leaseID1)

	if lm.ActiveLeaseCount() != 1 {
		t.Errorf("Active count should be 1 after revocation, got %d", lm.ActiveLeaseCount())
	}
}

func TestLeaseManagerClose(t *testing.T) {
	t.Run("clears leases", func(t *testing.T) {
		lm := NewLeaseManager(60000)
		lm.IssueLease("worker_1", Assignment{
			RunID:     "run_0000000000000001",
			StageID:   "stage_1",
			VUIDRange: VUIDRange{0, 100},
		})

		lm.Close()

		if lm.LeaseCount() != 0 {
			t.Errorf("Leases should be cleared after close, got %d", lm.LeaseCount())
		}
	})

	t.Run("idempotent", func(t *testing.T) {
		lm := NewLeaseManager(60000)
		err1 := lm.Close()
		err2 := lm.Close()

		if err1 != nil || err2 != nil {
			t.Errorf("Close should be idempotent: err1=%v, err2=%v", err1, err2)
		}
	})
}

func TestLeaseCopy(t *testing.T) {
	t.Run("nil lease", func(t *testing.T) {
		var l *Lease
		copy := l.Copy()
		if copy != nil {
			t.Error("Copy of nil should be nil")
		}
	})

	t.Run("with RevokedAt", func(t *testing.T) {
		revokedAt := int64(12345)
		l := &Lease{
			LeaseID:   "lease_1",
			WorkerID:  "worker_1",
			State:     LeaseStateRevoked,
			RevokedAt: &revokedAt,
		}

		copy := l.Copy()
		if copy.RevokedAt == nil {
			t.Error("RevokedAt should be copied")
		}
		if *copy.RevokedAt != revokedAt {
			t.Errorf("RevokedAt = %d, want %d", *copy.RevokedAt, revokedAt)
		}

		*copy.RevokedAt = 99999
		if *l.RevokedAt != revokedAt {
			t.Error("Modifying copy should not affect original")
		}
	})
}

func TestConcurrentIssue(t *testing.T) {
	lm := NewLeaseManager(60000)
	var wg sync.WaitGroup
	numGoroutines := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			lm.IssueLease(WorkerID("worker_"+string(rune('0'+idx%10))), Assignment{
				RunID:     "run_" + string(rune('0'+idx%5)),
				StageID:   "stage_1",
				VUIDRange: VUIDRange{idx * 10, idx*10 + 10},
			})
		}(i)
	}

	wg.Wait()

	if lm.LeaseCount() != numGoroutines {
		t.Errorf("Expected %d leases, got %d", numGoroutines, lm.LeaseCount())
	}
}

func TestConcurrentRenew(t *testing.T) {
	lm := NewLeaseManager(60000)
	leaseID, _ := lm.IssueLease("worker_1", Assignment{
		RunID:     "run_0000000000000001",
		StageID:   "stage_1",
		VUIDRange: VUIDRange{0, 100},
	})

	var wg sync.WaitGroup
	numGoroutines := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lm.RenewLease(leaseID)
		}()
	}

	wg.Wait()

	lease, _ := lm.GetLease(leaseID)
	if lease.State != LeaseStateActive {
		t.Errorf("Lease should still be active after concurrent renewals")
	}
}

func TestConcurrentMixedOperations(t *testing.T) {
	lm := NewLeaseManager(60000)
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			lm.IssueLease(WorkerID("worker_"+string(rune('0'+idx))), Assignment{
				RunID:     "run_0000000000000001",
				StageID:   "stage_1",
				VUIDRange: VUIDRange{idx * 100, idx*100 + 100},
			})
		}(i)
	}

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lm.ListLeases("run_0000000000000001")
		}()
	}

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lm.ExpireLeases()
		}()
	}

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lm.LeaseCount()
			lm.ActiveLeaseCount()
		}()
	}

	wg.Wait()
}

func TestLeaseIDGeneration(t *testing.T) {
	lm := NewLeaseManager(60000)

	ids := make(map[LeaseID]bool)
	for i := 0; i < 100; i++ {
		leaseID, _ := lm.IssueLease("worker_1", Assignment{
			RunID:     "run_0000000000000001",
			StageID:   "stage_1",
			VUIDRange: VUIDRange{i * 10, i*10 + 10},
		})
		if ids[leaseID] {
			t.Errorf("Duplicate lease ID generated: %s", leaseID)
		}
		ids[leaseID] = true
	}
}
