# Streaming Scenario Templates

This document describes the pre-built streaming scenario templates included with MCPDrill. These templates are designed to test various aspects of Server-Sent Events (SSE) streaming workloads.

## Overview

MCPDrill includes three streaming-focused scenario templates:

1. **SSE Heavy Load** - High-throughput streaming with many concurrent connections
2. **SSE with Churn** - Streaming combined with aggressive connection churn
3. **Stream Stall Test** - Detection of stalled/frozen streams

All templates target the mock MCP server's `/notifications/subscribe` endpoint, which emits SSE events at configurable rates.

---

## Template 1: SSE Heavy Load

**File**: `examples/scenarios/sse-heavy-load.json`

### What It Tests

This scenario tests your server's ability to handle high-throughput streaming workloads with many concurrent SSE connections. It focuses on:

- Sustained high event rates across many sessions
- Session pool reuse under load
- Latency degradation under streaming load
- Stream error rates at scale

### Key Configuration

```json
{
  "scenario_id": "sse-heavy-load",
  "session_policy": {
    "mode": "pool",
    "pool_size": 100
  },
  "workload": {
    "in_flight_per_vu": 2,
    "think_time": {
      "mode": "fixed",
      "base_ms": 100
    }
  },
  "stages": [
    {
      "stage": "baseline",
      "load": { "target_vus": 50 }
    },
    {
      "stage": "ramp",
      "load": { "target_vus": 200 },
      "ramp": {
        "step_every_ms": 30000,
        "step_vus": 50
      }
    }
  ]
}
```

**Session Mode**: `pool` - Reuses sessions from a pool of 100 to maximize throughput

**Load Profile**:
- Baseline: 50 VUs
- Ramp: 50 → 200 VUs in 50 VU steps every 30s
- Each VU maintains 2 in-flight requests

**Tool Configuration**:
- Tool: `notifications_subscribe`
- Events per stream: 50
- Event interval: 100ms
- Expected stream duration: ~5 seconds

### Stop Conditions

The scenario will stop early if:

1. **High Error Rate**: Error rate > 5% sustained for 20 seconds
2. **Latency Degradation**: P99 latency > 10s sustained for 45 seconds
3. **Stream Errors**: Stream error rate > 10% sustained for 20 seconds

### How to Run

```bash
# Start the mock server
./server --addr :9000

# Start the control plane
./server --addr :8080

# Start a worker
./worker --control-plane http://localhost:8080

# Create and run the scenario
./mcpdrill create examples/scenarios/sse-heavy-load.json
./mcpdrill start <run-id>

# Monitor progress
./mcpdrill events <run-id> --follow
```

### Expected Results

**Success Indicators**:
- All stages complete without triggering stop conditions
- Error rate < 1%
- P99 latency < 5s
- Stream completion rate > 95%

**Typical Metrics**:
- Total operations: ~10,000-20,000
- Total events received: ~500,000-1,000,000
- Average throughput: 5,000-10,000 events/sec
- Session reuse rate: > 90%

### Customization

**Increase Load**:
```json
{
  "stages": [
    {
      "stage": "ramp",
      "load": { "target_vus": 500 },
      "ramp": { "step_vus": 100 }
    }
  ]
}
```

**Longer Streams**:
```json
{
  "tools": {
    "templates": [
      {
        "arguments": {
          "event_count": 200,
          "interval_ms": 50
        }
      }
    ]
  }
}
```

**Adjust Session Pool**:
```json
{
  "session_policy": {
    "mode": "pool",
    "pool_size": 200
  }
}
```

---

## Template 2: SSE with Churn

**File**: `examples/scenarios/sse-with-churn.json`

### What It Tests

This scenario tests your server's resilience to aggressive connection churn while maintaining streaming quality. It focuses on:

- Connection/disconnection cycles during active streams
- Session creation/teardown overhead
- Resource cleanup under churn
- Stream quality during connection instability

### Key Configuration

```json
{
  "scenario_id": "sse-with-churn",
  "session_policy": {
    "mode": "churn",
    "churn_interval_ops": 1
  },
  "workload": {
    "in_flight_per_vu": 1,
    "think_time": {
      "mode": "jitter",
      "base_ms": 500,
      "jitter_ms": 200
    }
  },
  "stages": [
    {
      "stage": "baseline",
      "load": { "target_vus": 20 }
    },
    {
      "stage": "soak",
      "duration_ms": 180000,
      "load": { "target_vus": 50 }
    }
  ]
}
```

**Session Mode**: `churn` - Creates a new session for every operation (interval=1)

**Load Profile**:
- Baseline: 20 VUs for 30s
- Soak: 50 VUs for 3 minutes
- Each VU maintains 1 in-flight request

**Tool Configuration**:
- Tool: `notifications_subscribe`
- Events per stream: 20
- Event interval: 200ms
- Expected stream duration: ~4 seconds

### Stop Conditions

The scenario will stop early if:

1. **Connection Failures**: Connection error rate > 10% sustained for 20 seconds
2. **Sustained Errors**: Error rate > 5% sustained for 90 seconds
3. **Session Exhaustion**: > 10 session creation failures in 10 seconds

### How to Run

```bash
# Start the mock server
./server --addr :9000

# Start the control plane
./server --addr :8080

# Start a worker
./worker --control-plane http://localhost:8080

# Create and run the scenario
./mcpdrill create examples/scenarios/sse-with-churn.json
./mcpdrill start <run-id>

# Monitor progress
./mcpdrill events <run-id> --follow
```

### Expected Results

**Success Indicators**:
- Soak stage completes without triggering stop conditions
- Connection error rate < 5%
- No session exhaustion
- Stream completion rate > 90%

**Typical Metrics**:
- Total operations: ~5,000-10,000
- Total sessions created: ~5,000-10,000 (1:1 with operations)
- Session reuse rate: 0% (by design)
- Average connection setup time: < 100ms

### Customization

**More Aggressive Churn**:
```json
{
  "workload": {
    "think_time": {
      "mode": "fixed",
      "base_ms": 100
    }
  }
}
```

**Longer Soak**:
```json
{
  "stages": [
    {
      "stage": "soak",
      "duration_ms": 600000
    }
  ]
}
```

**Reduce Churn Frequency**:
```json
{
  "session_policy": {
    "mode": "churn",
    "churn_interval_ops": 5
  }
}
```

---

## Template 3: Stream Stall Test

**File**: `examples/scenarios/stream-stall-test.json`

### What It Tests

This scenario tests MCPDrill's ability to detect stalled/frozen streams. It uses aggressive timeouts to trigger stall detection when streams stop producing events. It focuses on:

- Stream stall detection accuracy
- Timeout handling
- Recovery from stalled streams
- Telemetry for stream health

### Key Configuration

```json
{
  "scenario_id": "stream-stall-test",
  "target": {
    "timeouts": {
      "stream_stall_timeout_ms": 10000
    }
  },
  "workload": {
    "tools": {
      "templates": [
        {
          "arguments": {
            "event_count": 5,
            "interval_ms": 15000
          }
        }
      ]
    }
  },
  "stages": [
    {
      "stage": "baseline",
      "load": { "target_vus": 10 },
      "stop_conditions": [
        {
          "metric": "stream_stall_count",
          "comparator": ">=",
          "threshold": 5
        }
      ]
    }
  ]
}
```

**Session Mode**: `reuse` - Standard session reuse

**Timeout Configuration**:
- Stream stall timeout: 10 seconds
- Event interval: 15 seconds (deliberately exceeds timeout)

**Load Profile**:
- Baseline: 10 VUs for up to 60s
- Expected to trigger stop condition early

**Tool Configuration**:
- Tool: `notifications_subscribe`
- Events per stream: 5
- Event interval: 15s (exceeds 10s stall timeout)
- Expected behavior: Stall detection after first event

### Stop Conditions

The scenario is **designed to trigger** this stop condition:

1. **Stream Stalls Detected**: >= 5 stream stalls detected in 30 seconds

This is the expected behavior - the test validates that stall detection works correctly.

### How to Run

```bash
# Start the mock server
./server --addr :9000

# Start the control plane
./server --addr :8080

# Start a worker
./worker --control-plane http://localhost:8080

# Create and run the scenario
./mcpdrill create examples/scenarios/stream-stall-test.json
./mcpdrill start <run-id>

# Monitor progress
./mcpdrill events <run-id> --follow
```

### Expected Results

**Success Indicators**:
- Stop condition triggers within 30-60 seconds
- Stream stall count >= 5
- Stalls are properly logged in telemetry
- No crashes or hangs

**Typical Metrics**:
- Total operations: ~10-20
- Stream stalls detected: 5-10
- Average time to stall detection: ~10 seconds
- Error attribution: "stream_stall" origin

### Customization

**Adjust Stall Sensitivity**:
```json
{
  "target": {
    "timeouts": {
      "stream_stall_timeout_ms": 5000
    }
  }
}
```

**Test Recovery**:
```json
{
  "workload": {
    "tools": {
      "templates": [
        {
          "arguments": {
            "event_count": 10,
            "interval_ms": 8000
          }
        }
      ]
    }
  }
}
```

**Increase Detection Threshold**:
```json
{
  "stages": [
    {
      "stop_conditions": [
        {
          "metric": "stream_stall_count",
          "threshold": 20
        }
      ]
    }
  ]
}
```

---

## Common Patterns

### Targeting Your Own Server

Replace the mock server URL with your target:

```json
{
  "target": {
    "url": "https://your-server.example.com",
    "transport": "streamable_http"
  },
  "environment": {
    "allowlist": {
      "allowed_targets": [
        {
          "kind": "suffix",
          "value": ".example.com"
        }
      ]
    }
  }
}
```

### Adding Authentication

```json
{
  "target": {
    "auth": {
      "type": "bearer_token",
      "bearer_token_ref": "env:API_TOKEN"
    }
  }
}
```

### Custom Tool Configuration

```json
{
  "workload": {
    "tools": {
      "templates": [
        {
          "template_id": "your-tool",
          "tool_name": "your_streaming_tool",
          "weight": 1,
          "arguments": {
            "your_param": "value"
          },
          "expects_streaming": true
        }
      ]
    }
  }
}
```

### Adjusting Stop Conditions

```json
{
  "stages": [
    {
      "stop_conditions": [
        {
          "id": "custom-condition",
          "metric": "your_metric",
          "comparator": ">",
          "threshold": 100,
          "window_ms": 30000,
          "sustain_windows": 2,
          "scope": {}
        }
      ]
    }
  ]
}
```

---

## Metrics Reference

### Streaming-Specific Metrics

- `stream_stall_count` - Number of detected stream stalls
- `stream_error_rate` - Percentage of streams that failed
- `stream_completion_rate` - Percentage of streams that completed successfully
- `events_received_total` - Total SSE events received
- `events_per_second` - Event throughput rate

### General Metrics

- `error_rate` - Overall operation error rate
- `latency_p99_ms` - 99th percentile latency
- `connection_error_rate` - Connection establishment failures
- `session_creation_failures` - Failed session creations

---

## Troubleshooting

### Scenario Fails Immediately

**Symptom**: Stop conditions trigger in preflight stage

**Solutions**:
- Check target server is running and accessible
- Verify allowlist configuration
- Check authentication credentials
- Review server logs for errors

### High Stream Error Rates

**Symptom**: Stream error rate > 10%

**Possible Causes**:
- Server cannot handle event rate
- Network instability
- Timeout configuration too aggressive
- Server resource exhaustion

**Solutions**:
- Reduce VU count
- Increase timeouts
- Reduce event rate in tool arguments
- Scale server resources

### Stream Stalls Not Detected

**Symptom**: Expected stalls don't trigger stop condition

**Possible Causes**:
- Stall timeout too long
- Event interval shorter than timeout
- Stop condition threshold too high

**Solutions**:
- Reduce `stream_stall_timeout_ms`
- Increase event `interval_ms`
- Lower stop condition threshold

### Session Exhaustion

**Symptom**: Session creation failures in churn mode

**Possible Causes**:
- Server connection limits
- OS file descriptor limits
- Network port exhaustion

**Solutions**:
- Reduce VU count
- Increase think time
- Check server connection limits
- Verify OS ulimits

---

## Next Steps

1. **Run the templates** against the mock server to understand baseline behavior
2. **Customize for your target** by adjusting URLs, auth, and tool configurations
3. **Tune stop conditions** based on your SLOs
4. **Combine with other scenarios** to create comprehensive test suites
5. **Integrate into CI/CD** for regression testing

For more information, see:
- [Run Configuration Schema](../schemas/run-config/v1.json)
- [Session Policies Documentation](./session-policies.md)
- [Stop Conditions Guide](./stop-conditions.md)
