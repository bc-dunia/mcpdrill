# Configuration Reference

Complete reference for MCP Drill test configuration.

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

| Operation | Description |
|-----------|-------------|
| `tools/list` | List available tools from server |
| `tools/call` | Call a specific tool |
| `ping` | Simple connectivity check |

### tools/call Example

```json
{
  "operation": "tools/call",
  "weight": 5,
  "tool_name": "calculate",
  "arguments": {
    "expression": "2 + 2 * 3"
  }
}
```

## Safety Configuration

| Field | Description |
|-------|-------------|
| `max_vus` | Maximum virtual users allowed |
| `max_duration_ms` | Maximum test duration |
| `max_errors` | Stop after this many errors |

## Example Configurations

### Basic Load Test

```json
{
  "scenario_id": "basic-test",
  "target": {
    "url": "http://localhost:3000/mcp",
    "transport": "streamable_http"
  },
  "stages": [
    {
      "stage_id": "stg_0000000000000001",
      "stage": "ramp",
      "duration_ms": 60000,
      "load": { "target_vus": 50 }
    }
  ],
  "workload": {
    "op_mix": [
      { "operation": "tools/list", "weight": 1 }
    ]
  }
}
```

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
