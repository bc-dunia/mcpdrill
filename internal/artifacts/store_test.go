package artifacts

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestNewFilesystemStore(t *testing.T) {
	t.Run("creates base directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		baseDir := filepath.Join(tmpDir, "artifacts")

		store, err := NewFilesystemStore(baseDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if store == nil {
			t.Fatal("expected non-nil store")
		}

		if _, err := os.Stat(baseDir); os.IsNotExist(err) {
			t.Error("expected base directory to be created")
		}
	})

	t.Run("empty base directory error", func(t *testing.T) {
		_, err := NewFilesystemStore("")
		if err == nil {
			t.Error("expected error for empty base directory")
		}
	})

	t.Run("existing directory works", func(t *testing.T) {
		tmpDir := t.TempDir()

		store, err := NewFilesystemStore(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if store.BaseDir() != tmpDir {
			t.Errorf("expected base dir %s, got %s", tmpDir, store.BaseDir())
		}
	})
}

func TestSaveArtifact(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		store, _ := NewFilesystemStore(t.TempDir())
		data := []byte("test report content")

		info, err := store.SaveArtifact("run_0000000000000123", ArtifactTypeReport, "report.html", data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if info.RunID != "run_0000000000000123" {
			t.Errorf("expected run ID 'run_0000000000000123', got %s", info.RunID)
		}
		if info.ArtifactType != ArtifactTypeReport {
			t.Errorf("expected artifact type 'reports', got %s", info.ArtifactType)
		}
		if info.Filename != "report.html" {
			t.Errorf("expected filename 'report.html', got %s", info.Filename)
		}
		if info.SizeBytes != int64(len(data)) {
			t.Errorf("expected size %d, got %d", len(data), info.SizeBytes)
		}

		savedData, err := os.ReadFile(info.Path)
		if err != nil {
			t.Fatalf("failed to read saved file: %v", err)
		}
		if string(savedData) != string(data) {
			t.Error("saved data doesn't match original")
		}
	})

	t.Run("creates nested directories", func(t *testing.T) {
		store, _ := NewFilesystemStore(t.TempDir())

		info, err := store.SaveArtifact("run_0000000000000456", ArtifactTypeTelemetry, "ops.jsonl", []byte("{}"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expectedDir := filepath.Join(store.BaseDir(), "run_0000000000000456", "telemetry")
		if _, err := os.Stat(expectedDir); os.IsNotExist(err) {
			t.Error("expected nested directories to be created")
		}
		if info.Path != filepath.Join(expectedDir, "ops.jsonl") {
			t.Errorf("unexpected path: %s", info.Path)
		}
	})

	t.Run("empty run ID error", func(t *testing.T) {
		store, _ := NewFilesystemStore(t.TempDir())
		_, err := store.SaveArtifact("", ArtifactTypeReport, "report.html", []byte("data"))
		if err == nil {
			t.Error("expected error for empty run ID")
		}
	})

	t.Run("empty artifact type error", func(t *testing.T) {
		store, _ := NewFilesystemStore(t.TempDir())
		_, err := store.SaveArtifact("run_0000000000000123", "", "report.html", []byte("data"))
		if err == nil {
			t.Error("expected error for empty artifact type")
		}
	})

	t.Run("empty filename error", func(t *testing.T) {
		store, _ := NewFilesystemStore(t.TempDir())
		_, err := store.SaveArtifact("run_0000000000000123", ArtifactTypeReport, "", []byte("data"))
		if err == nil {
			t.Error("expected error for empty filename")
		}
	})

	t.Run("filename with path separator error", func(t *testing.T) {
		store, _ := NewFilesystemStore(t.TempDir())
		_, err := store.SaveArtifact("run_0000000000000123", ArtifactTypeReport, "sub/report.html", []byte("data"))
		if err == nil {
			t.Error("expected error for filename with path separator")
		}
	})

	t.Run("overwrite existing file", func(t *testing.T) {
		store, _ := NewFilesystemStore(t.TempDir())

		_, err := store.SaveArtifact("run_0000000000000123", ArtifactTypeReport, "report.html", []byte("original"))
		if err != nil {
			t.Fatalf("first save failed: %v", err)
		}

		info, err := store.SaveArtifact("run_0000000000000123", ArtifactTypeReport, "report.html", []byte("updated"))
		if err != nil {
			t.Fatalf("second save failed: %v", err)
		}

		data, _ := os.ReadFile(info.Path)
		if string(data) != "updated" {
			t.Error("expected file to be overwritten")
		}
	})
}

func TestGetArtifact(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		store, _ := NewFilesystemStore(t.TempDir())
		originalData := []byte("test content")
		store.SaveArtifact("run_0000000000000123", ArtifactTypeReport, "report.html", originalData)

		data, err := store.GetArtifact("run_0000000000000123", ArtifactTypeReport, "report.html")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(data) != string(originalData) {
			t.Error("retrieved data doesn't match original")
		}
	})

	t.Run("not found", func(t *testing.T) {
		store, _ := NewFilesystemStore(t.TempDir())

		_, err := store.GetArtifact("run_0000000000000123", ArtifactTypeReport, "nonexistent.html")
		if err == nil {
			t.Error("expected error for nonexistent artifact")
		}
	})

	t.Run("empty run ID error", func(t *testing.T) {
		store, _ := NewFilesystemStore(t.TempDir())
		_, err := store.GetArtifact("", ArtifactTypeReport, "report.html")
		if err == nil {
			t.Error("expected error for empty run ID")
		}
	})

	t.Run("empty artifact type error", func(t *testing.T) {
		store, _ := NewFilesystemStore(t.TempDir())
		_, err := store.GetArtifact("run_0000000000000123", "", "report.html")
		if err == nil {
			t.Error("expected error for empty artifact type")
		}
	})

	t.Run("empty filename error", func(t *testing.T) {
		store, _ := NewFilesystemStore(t.TempDir())
		_, err := store.GetArtifact("run_0000000000000123", ArtifactTypeReport, "")
		if err == nil {
			t.Error("expected error for empty filename")
		}
	})
}

func TestListArtifacts(t *testing.T) {
	t.Run("success with multiple artifacts", func(t *testing.T) {
		store, _ := NewFilesystemStore(t.TempDir())

		store.SaveArtifact("run_0000000000000123", ArtifactTypeReport, "report.html", []byte("html"))
		store.SaveArtifact("run_0000000000000123", ArtifactTypeReport, "report.json", []byte("json"))
		store.SaveArtifact("run_0000000000000123", ArtifactTypeTelemetry, "ops.jsonl", []byte("ops"))

		artifacts, err := store.ListArtifacts("run_0000000000000123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(artifacts) != 3 {
			t.Errorf("expected 3 artifacts, got %d", len(artifacts))
		}

		foundReport := false
		foundTelemetry := false
		for _, a := range artifacts {
			if a.ArtifactType == ArtifactTypeReport && a.Filename == "report.html" {
				foundReport = true
			}
			if a.ArtifactType == ArtifactTypeTelemetry && a.Filename == "ops.jsonl" {
				foundTelemetry = true
			}
		}
		if !foundReport {
			t.Error("expected to find report.html")
		}
		if !foundTelemetry {
			t.Error("expected to find ops.jsonl")
		}
	})

	t.Run("empty for nonexistent run", func(t *testing.T) {
		store, _ := NewFilesystemStore(t.TempDir())

		artifacts, err := store.ListArtifacts("nonexistent_run")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(artifacts) != 0 {
			t.Errorf("expected 0 artifacts, got %d", len(artifacts))
		}
	})

	t.Run("empty run ID error", func(t *testing.T) {
		store, _ := NewFilesystemStore(t.TempDir())
		_, err := store.ListArtifacts("")
		if err == nil {
			t.Error("expected error for empty run ID")
		}
	})

	t.Run("includes size information", func(t *testing.T) {
		store, _ := NewFilesystemStore(t.TempDir())
		data := []byte("test content with known size")
		store.SaveArtifact("run_0000000000000123", ArtifactTypeReport, "report.html", data)

		artifacts, _ := store.ListArtifacts("run_0000000000000123")
		if len(artifacts) != 1 {
			t.Fatalf("expected 1 artifact, got %d", len(artifacts))
		}
		if artifacts[0].SizeBytes != int64(len(data)) {
			t.Errorf("expected size %d, got %d", len(data), artifacts[0].SizeBytes)
		}
	})
}

func TestDeleteArtifacts(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		store, _ := NewFilesystemStore(t.TempDir())
		store.SaveArtifact("run_0000000000000123", ArtifactTypeReport, "report.html", []byte("html"))
		store.SaveArtifact("run_0000000000000123", ArtifactTypeTelemetry, "ops.jsonl", []byte("ops"))

		err := store.DeleteArtifacts("run_0000000000000123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		artifacts, _ := store.ListArtifacts("run_0000000000000123")
		if len(artifacts) != 0 {
			t.Errorf("expected 0 artifacts after delete, got %d", len(artifacts))
		}

		runDir := filepath.Join(store.BaseDir(), "run_0000000000000123")
		if _, err := os.Stat(runDir); !os.IsNotExist(err) {
			t.Error("expected run directory to be deleted")
		}
	})

	t.Run("nonexistent run is no-op", func(t *testing.T) {
		store, _ := NewFilesystemStore(t.TempDir())

		err := store.DeleteArtifacts("nonexistent_run")
		if err != nil {
			t.Errorf("expected no error for nonexistent run, got: %v", err)
		}
	})

	t.Run("empty run ID error", func(t *testing.T) {
		store, _ := NewFilesystemStore(t.TempDir())
		err := store.DeleteArtifacts("")
		if err == nil {
			t.Error("expected error for empty run ID")
		}
	})
}

func TestConcurrentOperations(t *testing.T) {
	t.Run("concurrent saves", func(t *testing.T) {
		store, _ := NewFilesystemStore(t.TempDir())
		var wg sync.WaitGroup

		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				filename := filepath.Base(t.Name()) + "_" + string(rune('a'+idx%26)) + ".html"
				_, err := store.SaveArtifact("run_00000000000000cc", ArtifactTypeReport, filename, []byte("data"))
				if err != nil {
					t.Errorf("concurrent save failed: %v", err)
				}
			}(i)
		}

		wg.Wait()
	})

	t.Run("concurrent reads and writes", func(t *testing.T) {
		store, _ := NewFilesystemStore(t.TempDir())
		store.SaveArtifact("run_0000000000000001", ArtifactTypeReport, "report.html", []byte("initial"))

		var wg sync.WaitGroup

		for i := 0; i < 25; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, _ = store.GetArtifact("run_0000000000000001", ArtifactTypeReport, "report.html")
			}()
		}

		for i := 0; i < 25; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				_, _ = store.SaveArtifact("run_0000000000000001", ArtifactTypeReport, "report.html", []byte("updated"))
			}(i)
		}

		wg.Wait()
	})

	t.Run("concurrent list operations", func(t *testing.T) {
		store, _ := NewFilesystemStore(t.TempDir())
		store.SaveArtifact("run_0000000000000002", ArtifactTypeReport, "report.html", []byte("data"))

		var wg sync.WaitGroup

		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, _ = store.ListArtifacts("run_0000000000000002")
			}()
		}

		wg.Wait()
	})
}

func TestArtifactTypes(t *testing.T) {
	store, _ := NewFilesystemStore(t.TempDir())

	types := []ArtifactType{ArtifactTypeReport, ArtifactTypeTelemetry, ArtifactTypeConfig}
	for _, at := range types {
		t.Run(string(at), func(t *testing.T) {
			_, err := store.SaveArtifact("run_0000000000000003", at, "test.txt", []byte("data"))
			if err != nil {
				t.Errorf("failed to save artifact of type %s: %v", at, err)
			}
		})
	}

	artifacts, _ := store.ListArtifacts("run_0000000000000003")
	if len(artifacts) != 3 {
		t.Errorf("expected 3 artifacts, got %d", len(artifacts))
	}
}
