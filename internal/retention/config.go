// Package retention provides artifact and log retention management.
package retention

// Config holds retention policy configuration.
type Config struct {
	// ArtifactsTTLHours is the time-to-live for artifacts in hours.
	// Artifacts older than this will be deleted during cleanup.
	// Default: 168 (7 days)
	ArtifactsTTLHours int

	// LogsTTLHours is the time-to-live for telemetry logs in hours.
	// Logs older than this will be deleted during cleanup.
	// Default: 168 (7 days)
	LogsTTLHours int

	// CleanupIntervalHours is the interval between cleanup runs in hours.
	// Default: 24 (once per day)
	CleanupIntervalHours int
}

// DefaultConfig returns a Config with default values.
func DefaultConfig() Config {
	return Config{
		ArtifactsTTLHours:    168, // 7 days
		LogsTTLHours:         168, // 7 days
		CleanupIntervalHours: 24,  // once per day
	}
}

// WithDefaults returns a copy of the config with zero values replaced by defaults.
func (c Config) WithDefaults() Config {
	result := c
	if result.ArtifactsTTLHours <= 0 {
		result.ArtifactsTTLHours = 168
	}
	if result.LogsTTLHours <= 0 {
		result.LogsTTLHours = 168
	}
	if result.CleanupIntervalHours <= 0 {
		result.CleanupIntervalHours = 24
	}
	return result
}
