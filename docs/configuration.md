# Configuration Reference

Complete reference for MCP Drill configuration.

## Server Configuration

The control plane server accepts the following command-line flags:

### Telemetry Storage Limits

| Flag | Default | Recommended Max | Description |
|------|---------|-----------------|-------------|
| `--max-ops-per-run` | 20,000,000 | 50,000,000 | Maximum operations stored per run (0=unlimited) |
| `--max-logs-per-run` | 20,000,000 | 50,000,000 | Maximum logs stored per run (0=unlimited) |
| `--max-total-runs` | 100 | - | Maximum runs kept in memory before eviction (0=unlimited) |

> **Note**: Values exceeding 50M will trigger a warning due to high memory usage (25GB+). Values up to 100M are supported for enterprise deployments.

**Memory Considerations**:

| Limit | Estimated RAM | @ 1K ops/sec | @ 5K ops/sec | @ 10K ops/sec |
|-------|---------------|--------------|--------------|---------------|
| 20M (default) | 6-10 GB | ~5.5 hours | ~67 min | ~33 min |
| 50M (recommended max) | 15-25 GB | ~14 hours | ~2.8 hours | ~1.4 hours |
| 100M (enterprise) | 30-50 GB | ~28 hours | ~5.5 hours | ~2.8 hours |

> **Tip**: Check your actual throughput in the Web UI metrics. High VU counts with fast tools can easily reach 5,000-10,000+ ops/sec.

When limits are exceeded, new data is dropped and the UI displays a truncation warning. Metrics remain accurate for the stored data.

### Examples

```bash
# Default (20M operations, ~33 min at 10K ops/sec)
./mcpdrill-server --addr :8080

# High capacity for longer soak tests (50M, requires 16+ GB RAM)
./mcpdrill-server --addr :8080 \
  --max-ops-per-run 50000000 \
  --max-logs-per-run 50000000 \
  --max-total-runs 20

# Enterprise: very long soak tests (100M, requires 50+ GB RAM)
./mcpdrill-server --addr :8080 \
  --max-ops-per-run 100000000 \
  --max-logs-per-run 100000000 \
  --max-total-runs 10
```

---

## Test Run Configuration

Full schema for test run configuration files.

## Full Schema

```json
{
  "scenario_id": "string (required)",
  "target": {
    "kind": "server | gateway",
    "url": "string (required)",
    "transport": "streamable_http",
    "headers": { "key": "value" },
    "timeout_ms": 30000
  },
  "stages": [
    {
      "stage_id": "stg_<hex> (required)",
      "stage": "preflight | baseline | ramp | soak | spike | custom",
      "enabled": true,
      "duration_ms": 60000,
      "load": {
        "target_vus": 10,
        "target_rps": 100
      },
      "stop_conditions": [
        {
          "metric": "error_rate | latency_p50_ms | latency_p95_ms | latency_p99_ms | stream_stall_seconds | min_events_per_second",
          "threshold": 0.1,
          "window_ms": 10000
        }
      ]
    }
  ],
  "workload": {
    "op_mix": [
      { "operation": "tools/list", "weight": 1 },
      { "operation": "tools/call", "weight": 5, "tool_name": "echo", "arguments": {} },
      { "operation": "ping", "weight": 1 }
    ],
    "think_time": {
      "base_ms": 100,
      "jitter_ms": 50
    }
  },
  "session_policy": {
    "mode": "reuse | per_request | pool | churn",
    "pool_size": 10,
    "churn_interval_ms": 30000
  },
  "safety": {
    "hard_caps": {
      "max_vus": 1000,
      "max_duration_ms": 3600000,
      "max_errors": 10000
    }
  },
  "environment": {
    "allowlist": {
      "mode": "deny_by_default | allow_by_default",
      "allowed_hosts": ["localhost", "*.example.com"]
    }
  },
  "server_telemetry": {
    "enabled": true,
    "pair_key": "my-test-key"
  }
}
```

## Target Configuration

| Field | Type | Description |
|-------|------|-------------|
| `kind` | string | `server` or `gateway` |
| `url` | string | Target MCP server URL (required) |
| `transport` | string | Transport type (`streamable_http`) |
| `headers` | object | Custom HTTP headers |
| `timeout_ms` | number | Request timeout in milliseconds |

## Stage Types

| Stage | Purpose |
|-------|---------|
| `preflight` | Verify target is reachable with minimal load |
| `baseline` | Establish performance baseline at low load |
| `ramp` | Gradually increase load to find limits |
| `soak` | Sustain load to detect memory leaks and stability issues |
| `spike` | Sudden load increase to test burst handling |
| `custom` | User-defined stage behavior |

## Session Modes

| Mode | Description |
|------|-------------|
| `reuse` | Single session per VU (default, most efficient) |
| `per_request` | New session per request (highest isolation) |
| `pool` | Shared session pool across VUs |
| `churn` | Periodic session recreation (simulates real traffic) |

## Stop Conditions

| Metric | Description |
|--------|-------------|
| `error_rate` | Percentage of failed operations (e.g., >10%) |
| `latency_p50_ms` | 50th percentile latency |
| `latency_p95_ms` | 95th percentile latency |
| `latency_p99_ms` | 99th percentile latency |
| `stream_stall_seconds` | Streaming: seconds without SSE events |
| `min_events_per_second` | Streaming: minimum SSE event rate |

## Operations

| Operation | Description | Required Fields |
|-----------|-------------|-----------------|
| `tools_list` | List available tools from server | - |
| `tools_call` | Call a specific tool | Uses `workload.tools.templates` |
| `resources_list` | List available resources | - |
| `resources_read` | Read a specific resource | `uri` |
| `prompts_list` | List available prompts | - |
| `prompts_get` | Get a specific prompt | `prompt_name` |
| `ping` | Simple connectivity check | - |

### Operation Examples

**tools_call** - Uses tool templates defined in `workload.tools.templates`:
```json
{
  "operation": "tools_call",
  "weight": 5
}
```

**resources_read** - Requires `uri` field:
```json
{
  "operation": "resources_read",
  "weight": 2,
  "uri": "file:///docs/readme.md"
}
```

**prompts_get** - Requires `prompt_name` field:
```json
{
  "operation": "prompts_get",
  "weight": 1,
  "prompt_name": "summarize",
  "arguments": { "text": "Hello world" }
}
```

## Safety Configuration

| Field | Description |
|-------|-------------|
| `max_vus` | Maximum virtual users allowed |
| `max_duration_ms` | Maximum test duration |
| `max_errors` | Stop after this many errors |

## Example Configurations

> **Tip**: Use the Web UI wizard at http://localhost:5173 to generate valid run configurations. The wizard handles all required fields and schema compliance automatically.

For complete, validated examples, see the `examples/` directory, particularly `examples/quick-start.json`.

### Minimal Configuration Reference

The run config requires `schema_version: "run-config/v1"` and uses underscore-style operation names (`tools_list`, not `tools/list`). Here's a simplified reference:

```json
{
  "schema_version": "run-config/v1",
  "scenario_id": "basic-test",
  "target": {
    "url": "http://localhost:3000/mcp",
    "transport": "streamable_http"
  },
  "workload": {
    "operation_mix": [
      { "operation": "tools_list", "weight": 1 },
      { "operation": "tools_call", "weight": 3 }
    ],
    "tools": {
      "selection": { "mode": "weighted" },
      "templates": [
        { "template_id": "echo", "tool_name": "fast_echo", "weight": 1, "arguments": { "message": "test" } }
      ]
    }
  }
}
```

> **Note**: This is a simplified reference. Production configs require additional fields like `stages`, `safety`, `session_policy`, etc. See `examples/quick-start.json` for a complete valid config.

### Soak Test with Stop Conditions

```json
{
  "scenario_id": "soak-test",
  "target": {
    "url": "http://localhost:3000/mcp",
    "transport": "streamable_http"
  },
  "stages": [
    {
      "stage_id": "stg_0000000000000001",
      "stage": "soak",
      "duration_ms": 3600000,
      "load": { "target_vus": 100 },
      "stop_conditions": [
        { "metric": "error_rate", "threshold": 0.05, "window_ms": 30000 },
        { "metric": "latency_p95_ms", "threshold": 500, "window_ms": 30000 }
      ]
    }
  ],
  "workload": {
    "op_mix": [
      { "operation": "tools/call", "weight": 1, "tool_name": "echo", "arguments": {"message": "soak"} }
    ]
  },
  "session_policy": { "mode": "reuse" },
  "safety": {
    "hard_caps": { "max_vus": 200, "max_duration_ms": 7200000 }
  }
}
```
