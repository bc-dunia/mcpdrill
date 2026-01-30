package types

// HostInfo contains information about a worker's host.
type HostInfo struct {
	Hostname string `json:"hostname"`
	IPAddr   string `json:"ip_addr"`
	Platform string `json:"platform"`
}

// WorkerCapacity describes the capacity limits of a worker.
type WorkerCapacity struct {
	MaxVUs           int     `json:"max_vus"`
	MaxConcurrentOps int     `json:"max_concurrent_ops"`
	MaxRPS           float64 `json:"max_rps"`
}

// WorkerHealth contains the current health metrics of a worker.
type WorkerHealth struct {
	CPUPercent     float64 `json:"cpu_percent"`
	MemBytes       int64   `json:"mem_bytes"`
	ActiveVUs      int     `json:"active_vus"`
	ActiveSessions int     `json:"active_sessions"`
	InFlightOps    int     `json:"in_flight_ops"`
	QueueDepth     int     `json:"queue_depth"`
}
