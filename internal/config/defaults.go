package config

// Default configuration constants for session management and telemetry
const (
	DefaultEventBufferSize   = 10000
	DefaultChannelBufferSize = 10000
	DefaultSessionTTLMs      = 900000 // 15 minutes
	DefaultSessionIdleMs     = 60000  // 1 minute
	MinSessionTimeoutMs      = 1000
)
