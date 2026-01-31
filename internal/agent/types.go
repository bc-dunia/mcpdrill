// Package agent defines types for the telemetry agent that runs on MCP servers.
// The agent collects host and process metrics and reports them to the control plane.
package agent

// HostMetrics contains system-level metrics collected from the host.
type HostMetrics struct {
	// CPUPercent is the overall CPU usage percentage (0-100).
	CPUPercent float64 `json:"cpu_percent"`

	// MemTotal is the total system memory in bytes.
	MemTotal uint64 `json:"mem_total"`

	// MemUsed is the used system memory in bytes.
	MemUsed uint64 `json:"mem_used"`

	// MemAvailable is the available system memory in bytes.
	MemAvailable uint64 `json:"mem_available,omitempty"`

	// LoadAvg1 is the 1-minute load average.
	LoadAvg1 float64 `json:"load_avg_1,omitempty"`

	// LoadAvg5 is the 5-minute load average.
	LoadAvg5 float64 `json:"load_avg_5,omitempty"`

	// LoadAvg15 is the 15-minute load average.
	LoadAvg15 float64 `json:"load_avg_15,omitempty"`

	// DiskUsedPercent is the disk usage percentage for the primary partition (0-100).
	DiskUsedPercent float64 `json:"disk_used_percent,omitempty"`

	// NetworkBytesIn is the total bytes received since boot.
	NetworkBytesIn uint64 `json:"network_bytes_in,omitempty"`

	// NetworkBytesOut is the total bytes sent since boot.
	NetworkBytesOut uint64 `json:"network_bytes_out,omitempty"`
}

// ProcessMetrics contains metrics for the monitored MCP server process.
type ProcessMetrics struct {
	// PID is the process ID.
	PID int `json:"pid"`

	// CPUPercent is the process CPU usage percentage.
	CPUPercent float64 `json:"cpu_percent"`

	// MemRSS is the resident set size (physical memory) in bytes.
	MemRSS uint64 `json:"mem_rss"`

	// MemVMS is the virtual memory size in bytes.
	MemVMS uint64 `json:"mem_vms,omitempty"`

	// NumThreads is the number of threads in the process.
	NumThreads int `json:"num_threads,omitempty"`

	// NumFDs is the number of open file descriptors (Unix only).
	NumFDs int `json:"num_fds,omitempty"`

	// OpenConnections is the number of open network connections.
	OpenConnections int `json:"open_connections,omitempty"`
}
