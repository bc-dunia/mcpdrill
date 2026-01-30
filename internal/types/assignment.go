// Package types provides shared type definitions used across multiple packages.
package types

// RedirectPolicyConfig holds redirect policy configuration for assignments.
type RedirectPolicyConfig struct {
	Mode         string   `json:"mode"`
	MaxRedirects int      `json:"max_redirects,omitempty"`
	Allowlist    []string `json:"allowlist,omitempty"`
}

// TargetConfig contains the target configuration for an assignment.
type TargetConfig struct {
	URL            string                `json:"url"`
	Transport      string                `json:"transport"`
	Headers        map[string]string     `json:"headers,omitempty"`
	RedirectPolicy *RedirectPolicyConfig `json:"redirect_policy,omitempty"`
}

// WorkloadConfig contains the workload configuration for an assignment.
type WorkloadConfig struct {
	OpMix []OpMixEntry `json:"op_mix"`
}

// OpMixEntry represents a single operation in the mix.
type OpMixEntry struct {
	Operation  string                 `json:"operation"`
	Weight     int                    `json:"weight"`
	ToolName   string                 `json:"tool_name,omitempty"`
	Arguments  map[string]interface{} `json:"arguments,omitempty"`
	URI        string                 `json:"uri,omitempty"`
	PromptName string                 `json:"prompt_name,omitempty"`
}

// SessionPolicyConfig contains session policy for an assignment.
type SessionPolicyConfig struct {
	Mode      string `json:"mode"`
	PoolSize  int    `json:"pool_size,omitempty"`
	TTLMs     int64  `json:"ttl_ms,omitempty"`
	MaxIdleMs int64  `json:"max_idle_ms,omitempty"`
}

// WorkerAssignment represents a work assignment for a worker.
type WorkerAssignment struct {
	RunID         string              `json:"run_id"`
	ExecutionID   string              `json:"execution_id"`
	Stage         string              `json:"stage"`
	StageID       string              `json:"stage_id"`
	LeaseID       string              `json:"lease_id"`
	VUIDStart     int                 `json:"vu_id_start"`
	VUIDEnd       int                 `json:"vu_id_end"`
	DurationMs    int64               `json:"duration_ms"`
	Target        TargetConfig        `json:"target"`
	Workload      WorkloadConfig      `json:"workload"`
	SessionPolicy SessionPolicyConfig `json:"session_policy"`
}
