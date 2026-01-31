package retention

import (
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ArtifactStore defines the interface for artifact storage operations needed by retention.
type ArtifactStore interface {
	BaseDir() string
	DeleteArtifacts(runID string) error
}

// RunRetentionInfo contains metadata about a run for retention purposes.
type RunRetentionInfo struct {
	RunID     string
	EndTimeMs int64
}

// TelemetryStore defines the interface for telemetry storage operations needed by retention.
type TelemetryStore interface {
	ListRunsForRetention() []RunRetentionInfo
	DeleteRun(runID string)
}

// Manager handles periodic cleanup of old artifacts and logs.
type Manager struct {
	config         Config
	artifactStore  ArtifactStore
	telemetryStore TelemetryStore
	stopCh         chan struct{}
	stoppedCh      chan struct{}
	mu             sync.Mutex
	running        bool
}

// NewManager creates a new retention Manager.
func NewManager(config Config, artifactStore ArtifactStore, telemetryStore TelemetryStore) *Manager {
	return &Manager{
		config:         config.WithDefaults(),
		artifactStore:  artifactStore,
		telemetryStore: telemetryStore,
		stopCh:         make(chan struct{}),
		stoppedCh:      make(chan struct{}),
	}
}

// Start begins the background cleanup goroutine.
func (m *Manager) Start() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return
	}
	m.running = true
	go m.run()
}

// Stop signals the background goroutine to stop and waits for it to exit.
func (m *Manager) Stop() {
	shouldStop := false
	func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if !m.running {
			return
		}
		m.running = false
		shouldStop = true
	}()

	if !shouldStop {
		return
	}

	close(m.stopCh)
	<-m.stoppedCh
}

func (m *Manager) run() {
	defer close(m.stoppedCh)

	interval := time.Duration(m.config.CleanupIntervalHours) * time.Hour
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.cleanup()
		case <-m.stopCh:
			return
		}
	}
}

func (m *Manager) cleanup() {
	artifactsDeleted := m.cleanupArtifacts()
	logsDeleted := m.cleanupLogs()

	if artifactsDeleted > 0 {
		log.Printf("Deleted %d artifacts older than %d hours", artifactsDeleted, m.config.ArtifactsTTLHours)
	}
	if logsDeleted > 0 {
		log.Printf("Deleted %d log entries older than %d hours", logsDeleted, m.config.LogsTTLHours)
	}
}

func (m *Manager) cleanupArtifacts() int {
	if m.artifactStore == nil {
		return 0
	}

	baseDir := m.artifactStore.BaseDir()
	if baseDir == "" {
		return 0
	}

	entries, err := os.ReadDir(baseDir)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("Failed to read artifacts directory: %v", err)
		}
		return 0
	}

	ttlMs := int64(m.config.ArtifactsTTLHours) * 60 * 60 * 1000
	now := time.Now().UnixMilli()
	deleted := 0

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		runID := entry.Name()
		runDir := filepath.Join(baseDir, runID)

		modTime, err := getDirectoryModTime(runDir)
		if err != nil {
			continue
		}

		age := now - modTime.UnixMilli()
		if age > ttlMs {
			if err := m.artifactStore.DeleteArtifacts(runID); err != nil {
				log.Printf("Failed to delete artifacts for run %s: %v", runID, err)
				continue
			}
			deleted++
		}
	}

	return deleted
}

func (m *Manager) cleanupLogs() int {
	if m.telemetryStore == nil {
		return 0
	}

	runs := m.telemetryStore.ListRunsForRetention()
	ttlMs := int64(m.config.LogsTTLHours) * 60 * 60 * 1000
	now := time.Now().UnixMilli()
	deleted := 0

	for _, runInfo := range runs {
		if runInfo.EndTimeMs == 0 {
			continue
		}

		age := now - runInfo.EndTimeMs
		if age > ttlMs {
			m.telemetryStore.DeleteRun(runInfo.RunID)
			deleted++
		}
	}

	return deleted
}

func getDirectoryModTime(dir string) (time.Time, error) {
	var latestModTime time.Time

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		if info.ModTime().After(latestModTime) {
			latestModTime = info.ModTime()
		}

		return nil
	})

	if err != nil {
		return time.Time{}, err
	}

	return latestModTime, nil
}

// RunCleanupNow triggers an immediate cleanup (useful for testing).
func (m *Manager) RunCleanupNow() {
	m.cleanup()
}
