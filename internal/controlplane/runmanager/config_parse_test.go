package runmanager

import "testing"

func TestParseRunConfigDefaultsProtocolPolicy(t *testing.T) {
	config := []byte(`{
		"target": {"url": "http://127.0.0.1:3000/mcp"},
		"workload": {"operation_mix": [{"operation": "tools_list", "weight": 1}]}
	}`)

	parsed, err := parseRunConfig(config)
	if err != nil {
		t.Fatalf("parseRunConfig() error = %v", err)
	}
	if parsed.Target.ProtocolVersion != "auto" {
		t.Fatalf("ProtocolVersion = %q, want auto", parsed.Target.ProtocolVersion)
	}
	if parsed.Target.ProtocolVersionPolicy != "supported" {
		t.Fatalf("ProtocolVersionPolicy = %q, want supported", parsed.Target.ProtocolVersionPolicy)
	}
}

func TestParseRunConfigPreservesTargetTimeoutsTLSAndChurn(t *testing.T) {
	config := []byte(`{
		"target": {
			"url": "http://127.0.0.1:3000/mcp",
			"timeouts": {
				"connect_timeout_ms": 1234,
				"request_timeout_ms": 5678,
				"stream_stall_timeout_ms": 9012
			},
			"tls": {"verify": false, "ca_bundle_ref": null}
		},
		"session_policy": {"mode": "churn", "churn_interval_ops": 7},
		"workload": {"operation_mix": [{"operation": "tools/list", "weight": 1}]}
	}`)

	parsed, err := parseRunConfig(config)
	if err != nil {
		t.Fatalf("parseRunConfig() error = %v", err)
	}
	if parsed.Target.Timeouts == nil {
		t.Fatal("target timeouts were not parsed")
	}
	if parsed.Target.Timeouts.ConnectTimeoutMs != 1234 || parsed.Target.Timeouts.RequestTimeoutMs != 5678 || parsed.Target.Timeouts.StreamStallTimeoutMs != 9012 {
		t.Fatalf("unexpected timeouts: %+v", parsed.Target.Timeouts)
	}
	if parsed.Target.TLS == nil || parsed.Target.TLS.Verify {
		t.Fatalf("unexpected TLS config: %+v", parsed.Target.TLS)
	}
	if parsed.SessionPolicy.ChurnIntervalOps != 7 {
		t.Fatalf("ChurnIntervalOps = %d, want 7", parsed.SessionPolicy.ChurnIntervalOps)
	}
	timeouts := buildTimeoutConfig(parsed.Target.Timeouts)
	if timeouts == nil || timeouts.ConnectTimeoutMs != 1234 || timeouts.RequestTimeoutMs != 5678 || timeouts.StreamStallTimeoutMs != 9012 {
		t.Fatalf("assignment target timeouts not preserved: %+v", timeouts)
	}
	tlsConfig := buildTLSConfig(parsed.Target.TLS)
	if tlsConfig == nil || tlsConfig.Verify {
		t.Fatalf("assignment target TLS not preserved: %+v", tlsConfig)
	}
}

func TestParseRunConfigPreservesAuthRefs(t *testing.T) {
	config := []byte(`{
		"target": {
			"url": "http://127.0.0.1:3000/mcp",
			"auth": {
				"type": "api_key_header",
				"api_key_header_name": "X-Custom-Key",
				"api_key_ref": "env://MCPDRILL_TEST_API_KEY"
			}
		},
		"workload": {"operation_mix": [{"operation": "tools/list", "weight": 1}]}
	}`)

	parsed, err := parseRunConfig(config)
	if err != nil {
		t.Fatalf("parseRunConfig() error = %v", err)
	}
	auth := buildAuthConfig(parsed.Target.Auth)
	if auth == nil {
		t.Fatal("auth was not preserved")
	}
	if auth.Type != "api_key_header" || auth.APIKeyHeaderName != "X-Custom-Key" || auth.APIKeyRef != "env://MCPDRILL_TEST_API_KEY" {
		t.Fatalf("unexpected auth config: %+v", auth)
	}
}

func TestParseRunConfigPreservesLoadAndWorkloadControls(t *testing.T) {
	config := []byte(`{
		"target": {"url": "http://127.0.0.1:3000/mcp"},
		"stages": [{
			"stage_id": "baseline",
			"stage": "baseline",
			"enabled": true,
			"duration_ms": 1000,
			"load": {"target_vus": 4, "target_rps": 12.5},
			"stop_conditions": []
		}],
		"workload": {
			"in_flight_per_vu": 3,
			"think_time": {"mode": "jitter", "base_ms": 25, "jitter_ms": 5},
			"user_journey": {
				"startup_sequence": {"run_tools_list_on_start": false},
				"periodic_ops": {"tools_list_interval_ms": 1000, "tools_list_after_errors": 2},
				"reconnect_policy": {"enabled": false, "initial_delay_ms": 10, "max_delay_ms": 20, "multiplier": 1.5, "jitter_fraction": 0.1, "max_retries": 4}
			},
			"operation_mix": [{"operation": "tools/list", "weight": 1}]
		}
	}`)

	parsed, err := parseRunConfig(config)
	if err != nil {
		t.Fatalf("parseRunConfig() error = %v", err)
	}
	load := buildLoadConfig(parsed.Stages[0].Load)
	if load.TargetRPS != 12.5 {
		t.Fatalf("TargetRPS = %v, want 12.5", load.TargetRPS)
	}
	workload := buildWorkloadConfig(parsed.Workload)
	if workload.InFlightPerVU != 3 {
		t.Fatalf("InFlightPerVU = %d, want 3", workload.InFlightPerVU)
	}
	if workload.ThinkTime.BaseMs != 25 || workload.ThinkTime.JitterMs != 5 {
		t.Fatalf("ThinkTime = %+v", workload.ThinkTime)
	}
	if workload.UserJourney == nil || workload.UserJourney.ReconnectPolicy == nil {
		t.Fatalf("UserJourney not preserved: %+v", workload.UserJourney)
	}
	if workload.UserJourney.ReconnectPolicy.Enabled {
		t.Fatal("ReconnectPolicy.Enabled = true, want false")
	}
}

func TestParseRunConfigPreservesProtocolPolicy(t *testing.T) {
	config := []byte(`{
		"target": {
			"url": "http://127.0.0.1:3000/mcp",
			"protocol_version": "2026-07-28",
			"protocol_version_policy": "supported"
		},
		"workload": {"operation_mix": [{"operation": "tasks/get", "weight": 1, "task_id": "task-1"}]}
	}`)

	parsed, err := parseRunConfig(config)
	if err != nil {
		t.Fatalf("parseRunConfig() error = %v", err)
	}
	if parsed.Target.ProtocolVersion != "2026-07-28" {
		t.Fatalf("ProtocolVersion = %q, want 2026-07-28", parsed.Target.ProtocolVersion)
	}
	if parsed.Target.ProtocolVersionPolicy != "supported" {
		t.Fatalf("ProtocolVersionPolicy = %q, want supported", parsed.Target.ProtocolVersionPolicy)
	}
	if parsed.Workload.OpMix[0].Operation != "tasks/get" {
		t.Fatalf("Operation = %q, want tasks/get", parsed.Workload.OpMix[0].Operation)
	}
}

func TestParseRunConfigPreservesToolCallMRTRDuringTemplateExpansion(t *testing.T) {
	config := []byte(`{
		"target": {"url": "http://127.0.0.1:3000/mcp"},
		"workload": {
			"operation_mix": [{
				"operation": "tools_call",
				"weight": 1,
				"request_state": "opaque-state",
				"input_responses": {"request-1": {"type": "text", "text": "answer"}}
			}],
			"tools": {
				"selection": {"mode": "round_robin"},
				"templates": [{"template_id": "tmpl-1", "tool_name": "fast_echo", "weight": 7, "arguments": {"message": "hello"}}]
			}
		}
	}`)

	parsed, err := parseRunConfig(config)
	if err != nil {
		t.Fatalf("parseRunConfig() error = %v", err)
	}
	if len(parsed.Workload.OpMix) != 1 {
		t.Fatalf("len(OpMix) = %d, want 1", len(parsed.Workload.OpMix))
	}
	op := parsed.Workload.OpMix[0]
	if op.ToolName != "fast_echo" {
		t.Fatalf("ToolName = %q, want fast_echo", op.ToolName)
	}
	if op.RequestState != "opaque-state" {
		t.Fatalf("RequestState = %q, want opaque-state", op.RequestState)
	}
	if _, ok := op.InputResponses["request-1"]; !ok {
		t.Fatalf("InputResponses missing request-1: %+v", op.InputResponses)
	}
}

func TestParseRunConfigKeepsNamedToolCallRowsSeparate(t *testing.T) {
	config := []byte(`{
		"target": {"url": "http://127.0.0.1:3000/mcp"},
		"workload": {
			"operation_mix": [
				{"operation": "tools_call", "weight": 2, "tool_name": "fast_echo", "arguments": {"message": "hello"}},
				{"operation": "tools_call", "weight": 3, "tool_name": "calculate", "arguments": {"expression": "2+2"}}
			],
			"tools": {
				"selection": {"mode": "round_robin"},
				"templates": [
					{"template_id": "tmpl-1", "tool_name": "fast_echo", "weight": 2, "arguments": {"message": "hello"}},
					{"template_id": "tmpl-2", "tool_name": "calculate", "weight": 3, "arguments": {"expression": "2+2"}}
				]
			}
		}
	}`)

	parsed, err := parseRunConfig(config)
	if err != nil {
		t.Fatalf("parseRunConfig() error = %v", err)
	}
	if len(parsed.Workload.OpMix) != 2 {
		t.Fatalf("len(OpMix) = %d, want 2", len(parsed.Workload.OpMix))
	}
	if parsed.Workload.OpMix[0].ToolName != "fast_echo" || parsed.Workload.OpMix[1].ToolName != "calculate" {
		t.Fatalf("unexpected op mix: %+v", parsed.Workload.OpMix)
	}
}

func TestParseRunConfigNormalizesOperationAliases(t *testing.T) {
	config := []byte(`{
		"target": {"url": "http://127.0.0.1:3000/mcp", "protocol_version": "2026-07-28"},
		"workload": {
			"operation_mix": [
				{"operation": "tools_list", "weight": 1},
				{"operation": "tools_call", "weight": 1, "tool_name": "fast_echo"},
				{"operation": "resources_list", "weight": 1},
				{"operation": "resources_read", "weight": 1, "uri": "mock://resource"},
				{"operation": "prompts_list", "weight": 1},
				{"operation": "prompts_get", "weight": 1, "prompt_name": "prompt"},
				{"operation": "subscriptions_listen", "weight": 1},
				{"operation": "tasks_get", "weight": 1, "task_id": "task-1"},
				{"operation": "tasks_update", "weight": 1, "task_id": "task-1"},
				{"operation": "tasks_cancel", "weight": 1, "task_id": "task-1"}
			]
		}
	}`)

	parsed, err := parseRunConfig(config)
	if err != nil {
		t.Fatalf("parseRunConfig() error = %v", err)
	}
	want := []string{
		"tools/list",
		"tools/call",
		"resources/list",
		"resources/read",
		"prompts/list",
		"prompts/get",
		"subscriptions/listen",
		"tasks/get",
		"tasks/update",
		"tasks/cancel",
	}
	if len(parsed.Workload.OpMix) != len(want) {
		t.Fatalf("len(OpMix) = %d, want %d", len(parsed.Workload.OpMix), len(want))
	}
	for i, op := range parsed.Workload.OpMix {
		if op.Operation != want[i] {
			t.Fatalf("OpMix[%d].Operation = %q, want %q", i, op.Operation, want[i])
		}
	}
}
