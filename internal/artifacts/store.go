// Package artifacts provides artifact storage for run reports and other generated files.
package artifacts

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// ArtifactType represents the type of artifact being stored.
type ArtifactType string

const (
	// ArtifactTypeReport represents report artifacts (HTML, JSON).
	ArtifactTypeReport ArtifactType = "reports"

	// ArtifactTypeTelemetry represents telemetry artifacts.
	ArtifactTypeTelemetry ArtifactType = "telemetry"

	// ArtifactTypeConfig represents configuration snapshots.
	ArtifactTypeConfig ArtifactType = "config"
)

// ArtifactInfo contains metadata about a stored artifact.
type ArtifactInfo struct {
	RunID        string       `json:"run_id"`
	ArtifactType ArtifactType `json:"artifact_type"`
	Filename     string       `json:"filename"`
	Path         string       `json:"path"`
	SizeBytes    int64        `json:"size_bytes"`
}

// Store defines the interface for artifact storage.
type Store interface {
	// SaveArtifact stores an artifact for a run.
	// Creates directories as needed.
	SaveArtifact(runID string, artifactType ArtifactType, filename string, data []byte) (*ArtifactInfo, error)

	// GetArtifact retrieves an artifact for a run.
	GetArtifact(runID string, artifactType ArtifactType, filename string) ([]byte, error)

	// ListArtifacts lists all artifacts for a run.
	ListArtifacts(runID string) ([]ArtifactInfo, error)

	// DeleteArtifacts deletes all artifacts for a run.
	DeleteArtifacts(runID string) error
}

// FilesystemStore implements Store using the local filesystem.
// Artifacts are stored in {baseDir}/{runID}/{artifactType}/{filename}.
type FilesystemStore struct {
	baseDir string
	mu      sync.RWMutex
}

// NewFilesystemStore creates a new FilesystemStore with the given base directory.
// The base directory will be created if it doesn't exist.
func NewFilesystemStore(baseDir string) (*FilesystemStore, error) {
	if baseDir == "" {
		return nil, fmt.Errorf("base directory cannot be empty")
	}

	// Create base directory if it doesn't exist
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base directory: %w", err)
	}

	return &FilesystemStore{
		baseDir: baseDir,
	}, nil
}

// SaveArtifact stores an artifact for a run.
// Thread-safe for concurrent writes.
func (fs *FilesystemStore) SaveArtifact(runID string, artifactType ArtifactType, filename string, data []byte) (*ArtifactInfo, error) {
	if runID == "" {
		return nil, fmt.Errorf("run ID cannot be empty")
	}
	if artifactType == "" {
		return nil, fmt.Errorf("artifact type cannot be empty")
	}
	if filename == "" {
		return nil, fmt.Errorf("filename cannot be empty")
	}

	// Validate filename doesn't contain path separators
	if filepath.Base(filename) != filename {
		return nil, fmt.Errorf("filename cannot contain path separators")
	}

	fs.mu.Lock()
	defer fs.mu.Unlock()

	// Build directory path: {baseDir}/{runID}/{artifactType}/
	dir := filepath.Join(fs.baseDir, runID, string(artifactType))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create artifact directory: %w", err)
	}

	// Build file path
	filePath := filepath.Join(dir, filename)

	// Write file
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return nil, fmt.Errorf("failed to write artifact: %w", err)
	}

	return &ArtifactInfo{
		RunID:        runID,
		ArtifactType: artifactType,
		Filename:     filename,
		Path:         filePath,
		SizeBytes:    int64(len(data)),
	}, nil
}

// GetArtifact retrieves an artifact for a run.
func (fs *FilesystemStore) GetArtifact(runID string, artifactType ArtifactType, filename string) ([]byte, error) {
	if runID == "" {
		return nil, fmt.Errorf("run ID cannot be empty")
	}
	if artifactType == "" {
		return nil, fmt.Errorf("artifact type cannot be empty")
	}
	if filename == "" {
		return nil, fmt.Errorf("filename cannot be empty")
	}

	fs.mu.RLock()
	defer fs.mu.RUnlock()

	filePath := filepath.Join(fs.baseDir, runID, string(artifactType), filename)
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("artifact not found: %s/%s/%s", runID, artifactType, filename)
		}
		return nil, fmt.Errorf("failed to read artifact: %w", err)
	}

	return data, nil
}

// ListArtifacts lists all artifacts for a run.
func (fs *FilesystemStore) ListArtifacts(runID string) ([]ArtifactInfo, error) {
	if runID == "" {
		return nil, fmt.Errorf("run ID cannot be empty")
	}

	fs.mu.RLock()
	defer fs.mu.RUnlock()

	runDir := filepath.Join(fs.baseDir, runID)
	if _, err := os.Stat(runDir); os.IsNotExist(err) {
		return []ArtifactInfo{}, nil
	}

	var artifacts []ArtifactInfo

	// Walk through artifact type directories
	artifactTypes := []ArtifactType{ArtifactTypeReport, ArtifactTypeTelemetry, ArtifactTypeConfig}
	for _, artifactType := range artifactTypes {
		typeDir := filepath.Join(runDir, string(artifactType))
		entries, err := os.ReadDir(typeDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("failed to read artifact directory: %w", err)
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}

			info, err := entry.Info()
			if err != nil {
				continue
			}

			artifacts = append(artifacts, ArtifactInfo{
				RunID:        runID,
				ArtifactType: artifactType,
				Filename:     entry.Name(),
				Path:         filepath.Join(typeDir, entry.Name()),
				SizeBytes:    info.Size(),
			})
		}
	}

	return artifacts, nil
}

// DeleteArtifacts deletes all artifacts for a run.
func (fs *FilesystemStore) DeleteArtifacts(runID string) error {
	if runID == "" {
		return fmt.Errorf("run ID cannot be empty")
	}

	fs.mu.Lock()
	defer fs.mu.Unlock()

	runDir := filepath.Join(fs.baseDir, runID)
	if _, err := os.Stat(runDir); os.IsNotExist(err) {
		return nil // Nothing to delete
	}

	if err := os.RemoveAll(runDir); err != nil {
		return fmt.Errorf("failed to delete artifacts: %w", err)
	}

	return nil
}

// BaseDir returns the base directory of the store.
func (fs *FilesystemStore) BaseDir() string {
	return fs.baseDir
}
