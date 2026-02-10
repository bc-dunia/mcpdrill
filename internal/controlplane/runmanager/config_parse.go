package runmanager

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/types"
)

type parsedRunConfig struct {
	Target        parsedTarget        `json:"target"`
	Stages        []parsedStage       `json:"stages"`
	Workload      parsedWorkload      `json:"workload"`
	SessionPolicy parsedSessionPolicy `json:"session_policy"`
	Safety        parsedSafety        `json:"safety"`
}

type parsedRedirectPolicy struct {
	Mode         string   `json:"mode"`
	MaxRedirects int      `json:"max_redirects,omitempty"`
	Allowlist    []string `json:"allowlist,omitempty"`
}

type parsedAuth struct {
	Type   string   `json:"type"`
	Tokens []string `json:"tokens,omitempty"`
}

type parsedTarget struct {
	URL                   string                `json:"url"`
	Transport             string                `json:"transport"`
	Headers               map[string]string     `json:"headers,omitempty"`
	Auth                  *parsedAuth           `json:"auth,omitempty"`
	Identification        *parsedIdentification `json:"identification,omitempty"`
	RedirectPolicy        *parsedRedirectPolicy `json:"redirect_policy,omitempty"`
	ProtocolVersion       string                `json:"protocol_version,omitempty"`
	ProtocolVersionPolicy string                `json:"protocol_version_policy,omitempty"`
}

type parsedIdentification struct {
	RunIDHeader *parsedRunIDHeader `json:"run_id_header,omitempty"`
	UserAgent   *parsedUserAgent   `json:"user_agent,omitempty"`
}

type parsedRunIDHeader struct {
	Name          string `json:"name"`
	ValueTemplate string `json:"value_template"`
}

type parsedUserAgent struct {
	Value string `json:"value"`
}

type parsedStreamingConfig struct {
	StreamStallSeconds int     `json:"stream_stall_seconds,omitempty"`
	MinEventsPerSecond float64 `json:"min_events_per_second,omitempty"`
}

type parsedStage struct {
	StageID             string                 `json:"stage_id"`
	Stage               string                 `json:"stage"`
	Enabled             bool                   `json:"enabled"`
	DurationMs          int64                  `json:"duration_ms"`
	MaxDurationMs       int64                  `json:"max_duration_ms,omitempty"`
	Load                parsedLoad             `json:"load"`
	StopConditions      []parsedStopCondition  `json:"stop_conditions"`
	StreamingStopConfig *parsedStreamingConfig `json:"streaming_stop_conditions,omitempty"`
}

type parsedStopCondition struct {
	ID             string            `json:"id"`
	Metric         string            `json:"metric"`
	Comparator     string            `json:"comparator"`
	Threshold      float64           `json:"threshold"`
	WindowMs       int64             `json:"window_ms"`
	SustainWindows int               `json:"sustain_windows"`
	Scope          map[string]string `json:"scope"`
}

type parsedLoad struct {
	TargetVUs  int `json:"target_vus"`
	StartVUs   int `json:"start_vus,omitempty"`    // Starting VUs for ramp (default: 10% of target)
	RampSteps  int `json:"ramp_steps,omitempty"`   // Number of steps to reach target (default: 5)
	StepHoldMs int `json:"step_hold_ms,omitempty"` // How long to hold each step (default: duration/steps)
}

type parsedWorkload struct {
	OpMix        []parsedOpMixEntry `json:"op_mix"`
	OperationMix []parsedOpMixEntry `json:"operation_mix"`
	Tools        *parsedToolsConfig `json:"tools,omitempty"`
}

type parsedToolsConfig struct {
	Selection parsedToolSelection  `json:"selection"`
	Templates []parsedToolTemplate `json:"templates"`
}

type parsedToolSelection struct {
	Mode string `json:"mode"`
}

type parsedToolTemplate struct {
	TemplateID string                 `json:"template_id"`
	ToolName   string                 `json:"tool_name"`
	Weight     int                    `json:"weight"`
	Arguments  map[string]interface{} `json:"arguments,omitempty"`
}

type parsedOpMixEntry struct {
	Operation  string                 `json:"operation"`
	Weight     int                    `json:"weight"`
	ToolName   string                 `json:"tool_name,omitempty"`
	Arguments  map[string]interface{} `json:"arguments,omitempty"`
	URI        string                 `json:"uri,omitempty"`
	PromptName string                 `json:"prompt_name,omitempty"`
}

type parsedSessionPolicy struct {
	Mode      string `json:"mode"`
	PoolSize  int    `json:"pool_size,omitempty"`
	TTLMs     int64  `json:"ttl_ms,omitempty"`
	MaxIdleMs int64  `json:"max_idle_ms,omitempty"`
}

type parsedSafety struct {
	HardCaps          parsedHardCaps   `json:"hard_caps"`
	StopPolicy        parsedStopPolicy `json:"stop_policy"`
	AnalysisTimeoutMs int64            `json:"analysis_timeout_ms"`
}

type parsedStopPolicy struct {
	Mode           string `json:"mode"`
	DrainTimeoutMs int64  `json:"drain_timeout_ms"`
}

type parsedHardCaps struct {
	MaxVUs        int   `json:"max_vus"`
	MaxDurationMs int64 `json:"max_duration_ms"`
	MaxErrors     int   `json:"max_errors"`
}

func parseRunConfig(config []byte) (*parsedRunConfig, error) {
	var parsed parsedRunConfig
	if err := json.Unmarshal(config, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse run config: %w", err)
	}

	if len(parsed.Workload.OpMix) == 0 && len(parsed.Workload.OperationMix) > 0 {
		parsed.Workload.OpMix = parsed.Workload.OperationMix
	}

	for i := range parsed.Workload.OpMix {
		parsed.Workload.OpMix[i].Operation = normalizeOperationName(parsed.Workload.OpMix[i].Operation)
	}

	parsed.Workload.OpMix = expandToolsTemplates(parsed.Workload.OpMix, parsed.Workload.Tools)

	return &parsed, nil
}

func expandToolsTemplates(opMix []parsedOpMixEntry, tools *parsedToolsConfig) []parsedOpMixEntry {
	if tools == nil || len(tools.Templates) == 0 {
		return opMix
	}

	var expanded []parsedOpMixEntry

	for _, op := range opMix {
		if op.Operation == "tools/call" && op.ToolName == "" {
			for _, tmpl := range tools.Templates {
				expanded = append(expanded, parsedOpMixEntry{
					Operation: "tools/call",
					Weight:    tmpl.Weight,
					ToolName:  tmpl.ToolName,
					Arguments: tmpl.Arguments,
				})
			}
		} else {
			expanded = append(expanded, op)
		}
	}

	return expanded
}

func normalizeOperationName(op string) string {
	switch op {
	case "tools_list":
		return "tools/list"
	case "tools_call":
		return "tools/call"
	case "resources_list":
		return "resources/list"
	case "resources_read":
		return "resources/read"
	case "prompts_list":
		return "prompts/list"
	case "prompts_get":
		return "prompts/get"
	case "initialize":
		return "initialize"
	case "ping":
		return "ping"
	default:
		return op
	}
}

func buildTargetHeaders(runID string, target *parsedTarget) map[string]string {
	headers := make(map[string]string)

	for k, v := range target.Headers {
		headers[k] = v
	}

	if target.Identification != nil {
		if target.Identification.RunIDHeader != nil {
			name := target.Identification.RunIDHeader.Name
			if name == "" {
				name = "X-Test-Run-Id"
			}
			value := target.Identification.RunIDHeader.ValueTemplate
			value = strings.ReplaceAll(value, "${run_id}", runID)
			headers[name] = value
		}

		if target.Identification.UserAgent != nil {
			value := target.Identification.UserAgent.Value
			value = strings.ReplaceAll(value, "${run_id}", runID)
			headers["User-Agent"] = value
		}
	}

	return headers
}

func buildRedirectPolicy(policy *parsedRedirectPolicy) *types.RedirectPolicyConfig {
	if policy == nil {
		return nil
	}
	return &types.RedirectPolicyConfig{
		Mode:         policy.Mode,
		MaxRedirects: policy.MaxRedirects,
		Allowlist:    policy.Allowlist,
	}
}

func buildAuthConfig(auth *parsedAuth) *types.AuthConfig {
	if auth == nil || auth.Type == "" || auth.Type == "none" {
		return nil
	}
	return &types.AuthConfig{
		Type:   auth.Type,
		Tokens: auth.Tokens,
	}
}

func findStageByName(config *parsedRunConfig, stageName StageName) *parsedStage {
	for i := range config.Stages {
		if config.Stages[i].Stage == string(stageName) && config.Stages[i].Enabled {
			return &config.Stages[i]
		}
	}
	return nil
}

const (
	DefaultDrainTimeoutMs    = 30000
	DefaultAnalysisTimeoutMs = 1800000
)

func getDrainTimeout(config []byte) time.Duration {
	parsed, err := parseRunConfig(config)
	if err != nil || parsed.Safety.StopPolicy.DrainTimeoutMs <= 0 {
		return time.Duration(DefaultDrainTimeoutMs) * time.Millisecond
	}
	return time.Duration(parsed.Safety.StopPolicy.DrainTimeoutMs) * time.Millisecond
}

func getAnalysisTimeout(config []byte) time.Duration {
	parsed, err := parseRunConfig(config)
	if err != nil || parsed.Safety.AnalysisTimeoutMs <= 0 {
		return time.Duration(DefaultAnalysisTimeoutMs) * time.Millisecond
	}
	return time.Duration(parsed.Safety.AnalysisTimeoutMs) * time.Millisecond
}

func convertOpMix(entries []parsedOpMixEntry) []types.OpMixEntry {
	result := make([]types.OpMixEntry, len(entries))
	for i, e := range entries {
		result[i] = types.OpMixEntry{
			Operation:  e.Operation,
			Weight:     e.Weight,
			ToolName:   e.ToolName,
			Arguments:  e.Arguments,
			URI:        e.URI,
			PromptName: e.PromptName,
		}
	}
	return result
}
