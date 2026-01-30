package e2e

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/controlplane/api"
	"github.com/bc-dunia/mcpdrill/internal/retention"
	"github.com/bc-dunia/mcpdrill/internal/types"
)

func TestRetentionCleanup(t *testing.T) {
	artifactDir := t.TempDir()

	oldRunID := "run_00000000000012345"
	oldRunBaseDir := filepath.Join(artifactDir, oldRunID)
	oldRunDir := filepath.Join(oldRunBaseDir, "reports")
	if err := os.MkdirAll(oldRunDir, 0755); err != nil {
		t.Fatalf("Failed to create old run dir: %v", err)
	}
	oldFile := filepath.Join(oldRunDir, "report.json")
	if err := os.WriteFile(oldFile, []byte(`{"test": "old"}`), 0644); err != nil {
		t.Fatalf("Failed to create old file: %v", err)
	}

	oldTime := time.Now().Add(-8 * 24 * time.Hour)
	if err := os.Chtimes(oldFile, oldTime, oldTime); err != nil {
		t.Fatalf("Failed to set old file time: %v", err)
	}
	if err := os.Chtimes(oldRunDir, oldTime, oldTime); err != nil {
		t.Fatalf("Failed to set old dir time: %v", err)
	}
	if err := os.Chtimes(oldRunBaseDir, oldTime, oldTime); err != nil {
		t.Fatalf("Failed to set old base dir time: %v", err)
	}

	recentRunID := "run_00000000000067890"
	recentRunDir := filepath.Join(artifactDir, recentRunID, "reports")
	if err := os.MkdirAll(recentRunDir, 0755); err != nil {
		t.Fatalf("Failed to create recent run dir: %v", err)
	}
	recentFile := filepath.Join(recentRunDir, "report.json")
	if err := os.WriteFile(recentFile, []byte(`{"test": "recent"}`), 0644); err != nil {
		t.Fatalf("Failed to create recent file: %v", err)
	}

	artifactStore := &testArtifactStore{baseDir: artifactDir}

	config := retention.Config{
		ArtifactsTTLHours:    1,
		LogsTTLHours:         1,
		CleanupIntervalHours: 24,
	}

	manager := retention.NewManager(config, artifactStore, nil)

	manager.RunCleanupNow()

	if _, err := os.Stat(filepath.Join(artifactDir, oldRunID)); !os.IsNotExist(err) {
		t.Error("Expected old run directory to be deleted")
	}

	if _, err := os.Stat(filepath.Join(artifactDir, recentRunID)); os.IsNotExist(err) {
		t.Error("Expected recent run directory to be preserved")
	}

	t.Logf("Retention cleanup test passed")
}

func TestRetentionLogsCleanup(t *testing.T) {
	telemetryStore := api.NewTelemetryStore()

	now := time.Now().UnixMilli()
	oldEndTime := now - (8 * 24 * 60 * 60 * 1000)
	recentEndTime := now - (1 * 60 * 60 * 1000)

	oldRunID := "run_0000000000000a1b2"
	telemetryStore.AddTelemetryBatch(oldRunID, api.TelemetryBatchRequest{
		RunID: oldRunID,
		Operations: []types.OperationOutcome{
			{Operation: "test", LatencyMs: 100, OK: true, ExecutionID: "exec_001", Stage: "preflight", StageID: "stage_001", TimestampMs: oldEndTime - 1000},
			{Operation: "test", LatencyMs: 100, OK: true, ExecutionID: "exec_001", Stage: "preflight", StageID: "stage_001", TimestampMs: oldEndTime},
		},
	})

	recentRunID := "run_0000000000000c3d4"
	telemetryStore.AddTelemetryBatch(recentRunID, api.TelemetryBatchRequest{
		RunID: recentRunID,
		Operations: []types.OperationOutcome{
			{Operation: "test", LatencyMs: 100, OK: true, ExecutionID: "exec_001", Stage: "preflight", StageID: "stage_001", TimestampMs: recentEndTime - 1000},
			{Operation: "test", LatencyMs: 100, OK: true, ExecutionID: "exec_001", Stage: "preflight", StageID: "stage_001", TimestampMs: recentEndTime},
		},
	})

	runningRunID := "run_0000000000000e5f6"
	telemetryStore.AddTelemetryBatch(runningRunID, api.TelemetryBatchRequest{
		RunID: runningRunID,
		Operations: []types.OperationOutcome{
			{Operation: "test", LatencyMs: 100, OK: true, ExecutionID: "exec_001", Stage: "preflight", StageID: "stage_001", TimestampMs: now},
		},
	})

	adapter := &telemetryStoreAdapter{store: telemetryStore}

	config := retention.Config{
		ArtifactsTTLHours:    1,
		LogsTTLHours:         1,
		CleanupIntervalHours: 24,
	}

	manager := retention.NewManager(config, nil, adapter)

	manager.RunCleanupNow()

	if telemetryStore.HasRun(oldRunID) {
		t.Error("Expected old run to be deleted")
	}

	if !telemetryStore.HasRun(recentRunID) {
		t.Error("Expected recent run to be preserved")
	}

	if !telemetryStore.HasRun(runningRunID) {
		t.Error("Expected running run to be preserved")
	}

	t.Logf("Retention logs cleanup test passed")
}

func TestRetentionManagerStartStop(t *testing.T) {
	config := retention.Config{
		ArtifactsTTLHours:    168,
		LogsTTLHours:         168,
		CleanupIntervalHours: 24,
	}

	manager := retention.NewManager(config, nil, nil)

	manager.Start()
	manager.Start()

	manager.Stop()
	manager.Stop()

	t.Logf("Retention manager start/stop test passed")
}

func TestRetentionCombinedCleanup(t *testing.T) {
	artifactDir := t.TempDir()

	oldRunID := "run_0000000000000a0b0"
	oldRunBaseDir := filepath.Join(artifactDir, oldRunID)
	oldRunDir := filepath.Join(oldRunBaseDir, "reports")
	if err := os.MkdirAll(oldRunDir, 0755); err != nil {
		t.Fatalf("Failed to create old run dir: %v", err)
	}
	oldFile := filepath.Join(oldRunDir, "report.json")
	if err := os.WriteFile(oldFile, []byte(`{"test": "old"}`), 0644); err != nil {
		t.Fatalf("Failed to create old file: %v", err)
	}
	oldTime := time.Now().Add(-8 * 24 * time.Hour)
	if err := os.Chtimes(oldFile, oldTime, oldTime); err != nil {
		t.Fatalf("Failed to set old file time: %v", err)
	}
	if err := os.Chtimes(oldRunDir, oldTime, oldTime); err != nil {
		t.Fatalf("Failed to set old dir time: %v", err)
	}
	if err := os.Chtimes(oldRunBaseDir, oldTime, oldTime); err != nil {
		t.Fatalf("Failed to set old base dir time: %v", err)
	}

	telemetryStore := api.NewTelemetryStore()
	now := time.Now().UnixMilli()
	oldEndTime := now - (8 * 24 * 60 * 60 * 1000)

	telemetryStore.AddTelemetryBatch(oldRunID, api.TelemetryBatchRequest{
		RunID: oldRunID,
		Operations: []types.OperationOutcome{
			{Operation: "test", LatencyMs: 100, OK: true, ExecutionID: "exec_001", Stage: "preflight", StageID: "stage_001", TimestampMs: oldEndTime},
		},
	})

	artifactStore := &testArtifactStore{baseDir: artifactDir}
	adapter := &telemetryStoreAdapter{store: telemetryStore}

	config := retention.Config{
		ArtifactsTTLHours:    1,
		LogsTTLHours:         1,
		CleanupIntervalHours: 24,
	}

	manager := retention.NewManager(config, artifactStore, adapter)

	manager.RunCleanupNow()

	if _, err := os.Stat(filepath.Join(artifactDir, oldRunID)); !os.IsNotExist(err) {
		t.Error("Expected old run artifacts to be deleted")
	}

	if telemetryStore.HasRun(oldRunID) {
		t.Error("Expected old run logs to be deleted")
	}

	t.Logf("Retention combined cleanup test passed")
}

type testArtifactStore struct {
	baseDir string
}

func (s *testArtifactStore) BaseDir() string {
	return s.baseDir
}

func (s *testArtifactStore) DeleteArtifacts(runID string) error {
	return os.RemoveAll(filepath.Join(s.baseDir, runID))
}

type telemetryStoreAdapter struct {
	store *api.TelemetryStore
}

func (a *telemetryStoreAdapter) ListRunsForRetention() []retention.RunRetentionInfo {
	apiRuns := a.store.ListRunsForRetention()
	result := make([]retention.RunRetentionInfo, len(apiRuns))
	for i, r := range apiRuns {
		result[i] = retention.RunRetentionInfo{
			RunID:     r.RunID,
			EndTimeMs: r.EndTimeMs,
		}
	}
	return result
}

func (a *telemetryStoreAdapter) DeleteRun(runID string) {
	a.store.DeleteRun(runID)
}
