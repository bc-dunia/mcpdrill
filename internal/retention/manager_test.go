package retention

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type mockArtifactStore struct {
	mu         sync.Mutex
	baseDir    string
	deleteCalls []string
}

func (m *mockArtifactStore) BaseDir() string {
	return m.baseDir
}

func (m *mockArtifactStore) DeleteArtifacts(runID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleteCalls = append(m.deleteCalls, runID)
	return nil
}

func (m *mockArtifactStore) getDeleteCalls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.deleteCalls))
	copy(result, m.deleteCalls)
	return result
}

type mockTelemetryStore struct {
	mu         sync.Mutex
	runs       []RunRetentionInfo
	deleteCalls []string
}

func (m *mockTelemetryStore) ListRunsForRetention() []RunRetentionInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]RunRetentionInfo, len(m.runs))
	copy(result, m.runs)
	return result
}

func (m *mockTelemetryStore) DeleteRun(runID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleteCalls = append(m.deleteCalls, runID)
}

func (m *mockTelemetryStore) getDeleteCalls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.deleteCalls))
	copy(result, m.deleteCalls)
	return result
}

func TestConfig_DefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.ArtifactsTTLHours != 168 {
		t.Errorf("expected ArtifactsTTLHours=168, got %d", cfg.ArtifactsTTLHours)
	}
	if cfg.LogsTTLHours != 168 {
		t.Errorf("expected LogsTTLHours=168, got %d", cfg.LogsTTLHours)
	}
	if cfg.CleanupIntervalHours != 24 {
		t.Errorf("expected CleanupIntervalHours=24, got %d", cfg.CleanupIntervalHours)
	}
}

func TestConfig_WithDefaults(t *testing.T) {
	cfg := Config{}.WithDefaults()

	if cfg.ArtifactsTTLHours != 168 {
		t.Errorf("expected ArtifactsTTLHours=168, got %d", cfg.ArtifactsTTLHours)
	}
	if cfg.LogsTTLHours != 168 {
		t.Errorf("expected LogsTTLHours=168, got %d", cfg.LogsTTLHours)
	}
	if cfg.CleanupIntervalHours != 24 {
		t.Errorf("expected CleanupIntervalHours=24, got %d", cfg.CleanupIntervalHours)
	}

	cfg2 := Config{
		ArtifactsTTLHours:    48,
		LogsTTLHours:         72,
		CleanupIntervalHours: 12,
	}.WithDefaults()

	if cfg2.ArtifactsTTLHours != 48 {
		t.Errorf("expected ArtifactsTTLHours=48, got %d", cfg2.ArtifactsTTLHours)
	}
	if cfg2.LogsTTLHours != 72 {
		t.Errorf("expected LogsTTLHours=72, got %d", cfg2.LogsTTLHours)
	}
	if cfg2.CleanupIntervalHours != 12 {
		t.Errorf("expected CleanupIntervalHours=12, got %d", cfg2.CleanupIntervalHours)
	}
}

func TestManager_StartStop(t *testing.T) {
	cfg := Config{
		ArtifactsTTLHours:    1,
		LogsTTLHours:         1,
		CleanupIntervalHours: 1,
	}

	mgr := NewManager(cfg, nil, nil)

	mgr.Start()
	mgr.Start()

	done := make(chan struct{})
	go func() {
		mgr.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not return in time")
	}

	mgr.Stop()
}

func TestManager_CleanupArtifacts(t *testing.T) {
	tmpDir := t.TempDir()

	oldRunDir := filepath.Join(tmpDir, "run_0000000000000001")
	if err := os.MkdirAll(oldRunDir, 0755); err != nil {
		t.Fatal(err)
	}
	oldFile := filepath.Join(oldRunDir, "report.html")
	if err := os.WriteFile(oldFile, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(oldFile, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(oldRunDir, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	newRunDir := filepath.Join(tmpDir, "run_0000000000000002")
	if err := os.MkdirAll(newRunDir, 0755); err != nil {
		t.Fatal(err)
	}
	newFile := filepath.Join(newRunDir, "report.html")
	if err := os.WriteFile(newFile, []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}

	artifactStore := &mockArtifactStore{baseDir: tmpDir}

	cfg := Config{
		ArtifactsTTLHours:    24,
		LogsTTLHours:         24,
		CleanupIntervalHours: 1,
	}

	mgr := NewManager(cfg, artifactStore, nil)
	mgr.RunCleanupNow()

	deleteCalls := artifactStore.getDeleteCalls()
	if len(deleteCalls) != 1 {
		t.Fatalf("expected 1 delete call, got %d", len(deleteCalls))
	}
	if deleteCalls[0] != "run_0000000000000001" {
		t.Errorf("expected delete call for run_old, got %s", deleteCalls[0])
	}
}

func TestManager_CleanupLogs(t *testing.T) {
	now := time.Now().UnixMilli()
	oldEndTime := now - (48 * 60 * 60 * 1000)
	newEndTime := now - (1 * 60 * 60 * 1000)

	telemetryStore := &mockTelemetryStore{
		runs: []RunRetentionInfo{
			{RunID: "run_0000000000000001", EndTimeMs: oldEndTime},
			{RunID: "run_0000000000000002", EndTimeMs: newEndTime},
			{RunID: "run_0000000000000003", EndTimeMs: 0},
		},
	}

	cfg := Config{
		ArtifactsTTLHours:    24,
		LogsTTLHours:         24,
		CleanupIntervalHours: 1,
	}

	mgr := NewManager(cfg, nil, telemetryStore)
	mgr.RunCleanupNow()

	deleteCalls := telemetryStore.getDeleteCalls()
	if len(deleteCalls) != 1 {
		t.Fatalf("expected 1 delete call, got %d", len(deleteCalls))
	}
	if deleteCalls[0] != "run_0000000000000001" {
		t.Errorf("expected delete call for run_old, got %s", deleteCalls[0])
	}
}

func TestManager_CleanupBoth(t *testing.T) {
	tmpDir := t.TempDir()

	oldRunDir := filepath.Join(tmpDir, "run_0000000000000001")
	if err := os.MkdirAll(oldRunDir, 0755); err != nil {
		t.Fatal(err)
	}
	oldFile := filepath.Join(oldRunDir, "report.html")
	if err := os.WriteFile(oldFile, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(oldFile, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(oldRunDir, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	artifactStore := &mockArtifactStore{baseDir: tmpDir}

	now := time.Now().UnixMilli()
	oldEndTime := now - (48 * 60 * 60 * 1000)

	telemetryStore := &mockTelemetryStore{
		runs: []RunRetentionInfo{
			{RunID: "run_0000000000000004", EndTimeMs: oldEndTime},
		},
	}

	cfg := Config{
		ArtifactsTTLHours:    24,
		LogsTTLHours:         24,
		CleanupIntervalHours: 1,
	}

	mgr := NewManager(cfg, artifactStore, telemetryStore)
	mgr.RunCleanupNow()

	artifactDeletes := artifactStore.getDeleteCalls()
	if len(artifactDeletes) != 1 {
		t.Fatalf("expected 1 artifact delete call, got %d", len(artifactDeletes))
	}

	logDeletes := telemetryStore.getDeleteCalls()
	if len(logDeletes) != 1 {
		t.Fatalf("expected 1 log delete call, got %d", len(logDeletes))
	}
}

func TestManager_NilStores(t *testing.T) {
	cfg := Config{
		ArtifactsTTLHours:    24,
		LogsTTLHours:         24,
		CleanupIntervalHours: 1,
	}

	mgr := NewManager(cfg, nil, nil)
	mgr.RunCleanupNow()
}

func TestManager_EmptyBaseDir(t *testing.T) {
	artifactStore := &mockArtifactStore{baseDir: ""}

	cfg := Config{
		ArtifactsTTLHours:    24,
		LogsTTLHours:         24,
		CleanupIntervalHours: 1,
	}

	mgr := NewManager(cfg, artifactStore, nil)
	mgr.RunCleanupNow()

	deleteCalls := artifactStore.getDeleteCalls()
	if len(deleteCalls) != 0 {
		t.Errorf("expected 0 delete calls, got %d", len(deleteCalls))
	}
}

func TestManager_NonExistentBaseDir(t *testing.T) {
	artifactStore := &mockArtifactStore{baseDir: "/nonexistent/path/that/does/not/exist"}

	cfg := Config{
		ArtifactsTTLHours:    24,
		LogsTTLHours:         24,
		CleanupIntervalHours: 1,
	}

	mgr := NewManager(cfg, artifactStore, nil)
	mgr.RunCleanupNow()

	deleteCalls := artifactStore.getDeleteCalls()
	if len(deleteCalls) != 0 {
		t.Errorf("expected 0 delete calls, got %d", len(deleteCalls))
	}
}

func TestManager_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()

	for i := 0; i < 5; i++ {
		runDir := filepath.Join(tmpDir, "run_"+string(rune('a'+i)))
		if err := os.MkdirAll(runDir, 0755); err != nil {
			t.Fatal(err)
		}
		file := filepath.Join(runDir, "report.html")
		if err := os.WriteFile(file, []byte("data"), 0644); err != nil {
			t.Fatal(err)
		}
		oldTime := time.Now().Add(-48 * time.Hour)
		if err := os.Chtimes(file, oldTime, oldTime); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(runDir, oldTime, oldTime); err != nil {
			t.Fatal(err)
		}
	}

	artifactStore := &mockArtifactStore{baseDir: tmpDir}

	now := time.Now().UnixMilli()
	oldEndTime := now - (48 * 60 * 60 * 1000)

	telemetryStore := &mockTelemetryStore{
		runs: []RunRetentionInfo{
			{RunID: "run_0000000000000001", EndTimeMs: oldEndTime},
			{RunID: "run_0000000000000002", EndTimeMs: oldEndTime},
			{RunID: "run_0000000000000003", EndTimeMs: oldEndTime},
		},
	}

	cfg := Config{
		ArtifactsTTLHours:    24,
		LogsTTLHours:         24,
		CleanupIntervalHours: 1,
	}

	mgr := NewManager(cfg, artifactStore, telemetryStore)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mgr.RunCleanupNow()
		}()
	}
	wg.Wait()
}

func TestManager_GracefulShutdown(t *testing.T) {
	cfg := Config{
		ArtifactsTTLHours:    1,
		LogsTTLHours:         1,
		CleanupIntervalHours: 1,
	}

	mgr := NewManager(cfg, nil, nil)
	mgr.Start()

	time.Sleep(10 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		mgr.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("graceful shutdown did not complete in time")
	}
}

func TestManager_TTLCalculation(t *testing.T) {
	now := time.Now().UnixMilli()

	tests := []struct {
		name       string
		ttlHours   int
		endTimeMs  int64
		shouldDelete bool
	}{
		{
			name:       "just inside TTL boundary",
			ttlHours:   24,
			// Use 1 second buffer inside TTL to avoid race with time.Now() during cleanup
			endTimeMs:  now - (24*60*60*1000 - 1000),
			shouldDelete: false,
		},
		{
			name:       "just past TTL",
			ttlHours:   24,
			// 1 second past TTL - should always be deleted
			endTimeMs:  now - (24*60*60*1000 + 1000),
			shouldDelete: true,
		},
		{
			name:       "well within TTL",
			ttlHours:   24,
			endTimeMs:  now - (12 * 60 * 60 * 1000),
			shouldDelete: false,
		},
		{
			name:       "zero end time",
			ttlHours:   24,
			endTimeMs:  0,
			shouldDelete: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			telemetryStore := &mockTelemetryStore{
				runs: []RunRetentionInfo{
					{RunID: "test_run", EndTimeMs: tt.endTimeMs},
				},
			}

			cfg := Config{
				ArtifactsTTLHours:    tt.ttlHours,
				LogsTTLHours:         tt.ttlHours,
				CleanupIntervalHours: 1,
			}

			mgr := NewManager(cfg, nil, telemetryStore)
			mgr.RunCleanupNow()

			deleteCalls := telemetryStore.getDeleteCalls()
			if tt.shouldDelete && len(deleteCalls) == 0 {
				t.Error("expected run to be deleted, but it wasn't")
			}
			if !tt.shouldDelete && len(deleteCalls) > 0 {
				t.Error("expected run to NOT be deleted, but it was")
			}
		})
	}
}
