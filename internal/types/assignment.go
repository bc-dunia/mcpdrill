// Package types provides shared type definitions used across multiple packages.
package types

import (
	"os"
	"strings"
)

// RedirectPolicyConfig holds redirect policy configuration for assignments.
type RedirectPolicyConfig struct {
	Mode         string   `json:"mode"`
	MaxRedirects int      `json:"max_redirects,omitempty"`
	Allowlist    []string `json:"allowlist,omitempty"`
}

// TimeoutConfig contains target transport timeout settings in milliseconds.
type TimeoutConfig struct {
	ConnectTimeoutMs     int64 `json:"connect_timeout_ms,omitempty"`
	RequestTimeoutMs     int64 `json:"request_timeout_ms,omitempty"`
	StreamStallTimeoutMs int64 `json:"stream_stall_timeout_ms,omitempty"`
}

// TLSConfig contains target TLS settings.
type TLSConfig struct {
	Verify      bool   `json:"verify"`
	CABundleRef string `json:"ca_bundle_ref,omitempty"`
}

// AuthConfig contains authentication configuration for the target.
type AuthConfig struct {
	Type             string   `json:"type"`
	Tokens           []string `json:"tokens,omitempty"`
	BearerTokenRef   string   `json:"bearer_token_ref,omitempty"`
	APIKeyHeaderName string   `json:"api_key_header_name,omitempty"`
	APIKeyRef        string   `json:"api_key_ref,omitempty"`
}

// TargetConfig contains the target configuration for an assignment.
type TargetConfig struct {
	URL                   string                `json:"url"`
	Transport             string                `json:"transport"`
	Headers               map[string]string     `json:"headers,omitempty"`
	RedirectPolicy        *RedirectPolicyConfig `json:"redirect_policy,omitempty"`
	Timeouts              *TimeoutConfig        `json:"timeouts,omitempty"`
	TLS                   *TLSConfig            `json:"tls,omitempty"`
	Auth                  *AuthConfig           `json:"auth,omitempty"`
	ProtocolVersion       string                `json:"protocol_version,omitempty"`
	ProtocolVersionPolicy string                `json:"protocol_version_policy,omitempty"`
}

// WorkloadConfig contains the workload configuration for an assignment.
type WorkloadConfig struct {
	OpMix         []OpMixEntry       `json:"op_mix"`
	InFlightPerVU int                `json:"in_flight_per_vu,omitempty"`
	ThinkTime     ThinkTimeConfig    `json:"think_time,omitempty"`
	UserJourney   *UserJourneyConfig `json:"user_journey,omitempty"`
}

type ThinkTimeConfig struct {
	Mode     string `json:"mode,omitempty"`
	BaseMs   int64  `json:"base_ms,omitempty"`
	JitterMs int64  `json:"jitter_ms,omitempty"`
}

type UserJourneyConfig struct {
	StartupSequence *StartupSequenceConfig `json:"startup_sequence,omitempty"`
	PeriodicOps     *PeriodicOpsConfig     `json:"periodic_ops,omitempty"`
	ReconnectPolicy *ReconnectPolicyConfig `json:"reconnect_policy,omitempty"`
}

type StartupSequenceConfig struct {
	RunToolsListOnStart bool `json:"run_tools_list_on_start"`
}

type PeriodicOpsConfig struct {
	ToolsListIntervalMs  int64 `json:"tools_list_interval_ms,omitempty"`
	ToolsListAfterErrors int   `json:"tools_list_after_errors,omitempty"`
}

type ReconnectPolicyConfig struct {
	Enabled        bool    `json:"enabled"`
	InitialDelayMs int64   `json:"initial_delay_ms,omitempty"`
	MaxDelayMs     int64   `json:"max_delay_ms,omitempty"`
	Multiplier     float64 `json:"multiplier,omitempty"`
	JitterFraction float64 `json:"jitter_fraction,omitempty"`
	MaxRetries     int     `json:"max_retries,omitempty"`
}

type LoadConfig struct {
	TargetRPS float64 `json:"target_rps,omitempty"`
}

// OpMixEntry represents a single operation in the mix.
type OpMixEntry struct {
	Operation      string                 `json:"operation"`
	Weight         int                    `json:"weight"`
	ToolName       string                 `json:"tool_name,omitempty"`
	Arguments      map[string]interface{} `json:"arguments,omitempty"`
	URI            string                 `json:"uri,omitempty"`
	PromptName     string                 `json:"prompt_name,omitempty"`
	TaskID         string                 `json:"task_id,omitempty"`
	InputResponses map[string]interface{} `json:"input_responses,omitempty"`
	RequestState   string                 `json:"request_state,omitempty"`
	Notifications  map[string]interface{} `json:"notifications,omitempty"`
}

// SessionPolicyConfig contains session policy for an assignment.
type SessionPolicyConfig struct {
	Mode             string `json:"mode"`
	PoolSize         int    `json:"pool_size,omitempty"`
	TTLMs            int64  `json:"ttl_ms,omitempty"`
	MaxIdleMs        int64  `json:"max_idle_ms,omitempty"`
	ChurnIntervalOps int64  `json:"churn_interval_ops,omitempty"`
}

// GetHeadersWithAuth returns the target headers with auth token injected if configured.
// If auth is configured with bearer_token type and has tokens, the first token is used
// as the Authorization header value.
func (t *TargetConfig) GetHeadersWithAuth() map[string]string {
	headers := make(map[string]string)
	for k, v := range t.Headers {
		headers[k] = v
	}
	if t.Auth == nil {
		return headers
	}
	switch t.Auth.Type {
	case "bearer_token":
		if token := t.Auth.firstToken(); token != "" {
			headers["Authorization"] = "Bearer " + token
		}
	case "api_key_header":
		headerName := t.Auth.APIKeyHeaderName
		if headerName == "" {
			headerName = "X-API-Key"
		}
		if token := resolveSecretRef(t.Auth.APIKeyRef); token != "" {
			headers[headerName] = token
		}
	}
	return headers
}

func (a *AuthConfig) firstToken() string {
	if len(a.Tokens) > 0 {
		return a.Tokens[0]
	}
	return resolveSecretRef(a.BearerTokenRef)
}

func resolveSecretRef(ref string) string {
	switch {
	case strings.HasPrefix(ref, "env://"):
		return os.Getenv(strings.TrimPrefix(ref, "env://"))
	case strings.HasPrefix(ref, "file://"):
		data, err := os.ReadFile(strings.TrimPrefix(ref, "file://"))
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(data))
	default:
		return ""
	}
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
	Load          LoadConfig          `json:"load,omitempty"`
	Target        TargetConfig        `json:"target"`
	Workload      WorkloadConfig      `json:"workload"`
	SessionPolicy SessionPolicyConfig `json:"session_policy"`
}
