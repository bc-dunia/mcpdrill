# MCP Drill

A high-performance stress testing platform for MCP servers and gateways.

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)


**Simulate thousands of concurrent MCP clients, identify performance bottlenecks, and validate your infrastructure before production.**

> Built to battle-test [Peta](https://github.com/dunialabs/peta-core), an MCP control plane and runtime. If you're building MCP infrastructure, MCP Drill helps you break it before your users do.

## Why MCP Drill?

- **Self-Contained** - Run everything on your own infrastructure. No external services, no cloud dependencies, no data leaving your network
- **Realistic Load Simulation** - Simulate real MCP client behavior with configurable operation mixes
- **Multi-stage Testing** - Preflight, baseline, ramp-up, soak, and spike stages
- **Real-time Observability** - Monitor latency, throughput, and errors via Web UI and SSE
- **Distributed Architecture** - Scale horizontally with multiple workers
- **Safety First** - Built-in stop conditions prevent runaway tests

## Quick Start

### 1. Start Backend Services

```bash
git clone https://github.com/bc-dunia/mcpdrill.git
cd mcpdrill

# Build and start all backend services
make dev
```

This starts (loopback only, no authentication required):

- **Mock Server**: http://localhost:3000/mcp
- **Control Plane**: http://localhost:8080
- **Worker**: Auto-connected

To stop: `make dev-stop`

### 2. Start Web UI

```bash
cd web/log-explorer
npm install
npm run dev
```

Open **http://localhost:5173** — the dashboard is now ready.

### 3. Run Your First Test

Use the Web UI's **"New Run"** button to create and start a test, or via CLI:

```bash
# Create a run using the quick-start config
curl -X POST http://localhost:8080/runs \
  -H "Content-Type: application/json" \
  -d "{\"config\": $(cat examples/quick-start.json)}"

# Start the run (replace with your run_id from the response)
curl -X POST http://localhost:8080/runs/{run_id}/start
```

> **Note**: The quick-start config targets `http://127.0.0.1:3000/mcp` (IPv4 localhost). Using `localhost` may fail due to IPv6 resolution on some systems.

> **Optional: Server Resources (CPU/Memory)**  
> The Web UI can display server-side CPU/memory metrics, but this requires the optional `mcpdrill-agent`. The quick-start config has `server_telemetry.enabled: false` by default. To enable it, see [Server Telemetry Agent](#server-telemetry-agent-optional) section below.

### 4. View Results

Watch real-time metrics in the Web UI at **http://localhost:5173**, or query the API:

```bash
curl http://localhost:8080/runs/{run_id}
curl http://localhost:8080/runs/{run_id}/logs?limit=10
```

## Key Features

| Feature | Description |
|---------|-------------|
| **Virtual Users (VUs)** | Simulate concurrent MCP clients with weighted operation mix |
| **Session Modes** | `reuse`, `per_request`, `pool`, `churn` |
| **Stop Conditions** | Auto-stop on error rate, latency thresholds |
| **Web UI** | Real-time metrics dashboard, log explorer, run wizard |
| **Mock Server** | 27 built-in tools for isolated testing without external dependencies |
| **Telemetry Agent** | *(Optional)* Server-side CPU/memory monitoring |
| **OpenTelemetry** | *(Optional)* Distributed tracing with OTLP export |

## Screenshots

### Live Metrics Dashboard

Real-time performance monitoring with throughput, latency percentiles, error rates, and server resource utilization:

![Live Metrics Overview](docs/screenshot1.png)

### Tool-Level Analytics

Detailed per-tool breakdown with success rates, latency distribution, and error analysis:

![Tool Metrics](docs/screenshot2.png)

## Architecture

```
┌──────────────────────────────────────────────────┐
│                    Control Plane                 │
│  ┌────────────┐  ┌────────────┐  ┌────────────┐  │
│  │Run Manager │  │ Scheduler  │  │  HTTP API  │  │
│  └────────────┘  └────────────┘  └────────────┘  │
└─────────────────────────┬────────────────────────┘
                          │
            ┌─────────────┼─────────────┐
            │             │             │
      ┌─────┴─────┐ ┌─────┴─────┐ ┌─────┴─────┐
      │  Worker   │ │  Worker   │ │  Worker   │
      │ (VU Pool) │ │ (VU Pool) │ │ (VU Pool) │
      └─────┬─────┘ └─────┬─────┘ └─────┬─────┘
            │             │             │
            └─────────────┼─────────────┘
                          │
                   ┌──────┴──────┐
                   │ MCP Target  │
                   └─────────────┘
```

## Mock Server

MCP Drill includes a built-in mock MCP server with 27 tools for isolated testing without external dependencies. Use it to validate your test configurations before running against production systems.

### Quick Start

```bash
# Build and run the mock server
make mockserver
./mcpdrill-mockserver --addr :3000
```

The mock server exposes an MCP endpoint at `http://localhost:3000/mcp`.

### Verify It Works

```bash
# List available tools (JSON-RPC)
curl -X POST http://localhost:3000/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"tools/list","id":1}'

# Call a tool
curl -X POST http://localhost:3000/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"fast_echo","arguments":{"message":"hello"}},"id":2}'
```

> **Notes:**
> - `streaming_tool` requires `Accept: text/event-stream` header for SSE streaming
> - File tools (`read_file`, `write_file`, `list_directory`) are simulated and do not access the real filesystem
> - By default, `--addr :3000` binds to `127.0.0.1` only; use `--addr 0.0.0.0:3000` to expose externally

### Available Tools

| Category | Tools |
|----------|-------|
| **Basic** | `fast_echo`, `slow_echo`, `error_tool`, `timeout_tool`, `streaming_tool` |
| **Data Processing** | `json_transform`, `text_processor`, `list_operations`, `validate_email`, `calculate`, `hash_generator` |
| **API Simulation** | `weather_api`, `geocode`, `currency_convert` |
| **File Operations** | `read_file`, `write_file`, `list_directory` |
| **Stress Testing** | `large_payload`, `random_latency`, `conditional_error`, `degrading_performance`, `flaky_connection` |
| **Resilience** | `rate_limited`, `circuit_breaker`, `backpressure`, `stateful_counter`, `realistic_latency` |

### Example Test Configuration

> **Tip**: Use the **Web UI wizard** at http://localhost:5173 to create valid test configurations without manual JSON editing.

For a complete, validated example, see [`examples/quick-start.json`](examples/quick-start.json). Here's a simplified reference:

```json
{
  "schema_version": "run-config/v1",
  "scenario_id": "mock-server-test",
  "target": {
    "url": "http://127.0.0.1:3000/mcp",
    "transport": "streamable_http"
  },
  "workload": {
    "operation_mix": [
      { "operation": "tools_list", "weight": 1 },
      { "operation": "tools_call", "weight": 5 }
    ],
    "tools": {
      "selection": { "mode": "weighted" },
      "templates": [
        { "template_id": "echo", "tool_name": "fast_echo", "weight": 1, "arguments": { "message": "hello" } }
      ]
    }
  }
}
```

> **Note**: This is a simplified reference. The full config requires additional fields (`stages`, `safety`, `session_policy`, etc.). See `examples/quick-start.json` for a complete working example.

See [Tool Testing Guide](docs/tool-testing-guide.md) for detailed documentation on each tool.

## Server Telemetry Agent (Optional)

> **Note**: MCP Drill works perfectly without the telemetry agent. This is an optional add-on for when you want to correlate client-side load test metrics with server-side resource usage.

Monitor server-side metrics (CPU, memory, threads, file descriptors) from your MCP server during load tests.

```
┌─────────────────────────────┐          ┌─────────────────────────┐
│   Your MCP Server Host      │          │   Control Plane         │
│                             │          │                         │
│  MCP Server   mcpdrill-agent│─────────▶│  Correlates metrics     │
│  (port 3000)       │        │  HTTPS   │  with test runs         │
│       ▲            │        │          │                         │
│       └── monitors─┘        │          │                         │
└─────────────────────────────┘          └─────────────────────────┘
```

### Quick Setup

```bash
# Build the agent
make agent

# Enable on control plane
./mcpdrill-server --addr :8080 --enable-agent-ingest --agent-tokens "secret-token"

# Run agent on your MCP server host
./mcpdrill-agent \
  --control-plane-url http://localhost:8080 \
  --agent-token "secret-token" \
  --pair-key "my-test" \
  --listen-port 3000
```

### Link with Test Runs

```json
{
  "scenario_id": "capacity-test",
  "server_telemetry": {
    "enabled": true,
    "pair_key": "my-test"
  },
  "target": { "url": "http://localhost:3000/mcp" }
}
```

> **What is `pair_key`?**  
> The pair key links metrics from a specific server with your test runs. Use the **same value** for `--pair-key` when starting the agent and in your run config's `server_telemetry.pair_key` to correlate server-side resource metrics (CPU, memory) with client-side load test results.

See [Agent Telemetry Guide](docs/agent-telemetry.md) for full details.

## Documentation

| Topic | Description |
|-------|-------------|
| **[Getting Started](docs/getting-started.md)** | Installation, prerequisites, first test |
| **[CLI Reference](docs/cli.md)** | All commands and flags |
| **[Configuration](docs/configuration.md)** | Full config schema, stages, workloads |
| **[API Reference](docs/api.md)** | REST endpoints, authentication |
| **[Web UI](docs/web-ui.md)** | Dashboard, log explorer, run wizard |
| **[Tool Testing](docs/tool-testing-guide.md)** | Mock server, 27 built-in tools |
| **[Agent Telemetry](docs/agent-telemetry.md)** | Server-side metrics collection |
| **[OpenTelemetry](docs/opentelemetry.md)** | Distributed tracing setup |
| **[Multi-Node Deployment](docs/multi-node-deployment.md)** | Docker, Kubernetes, scaling |
| **[Plugins](docs/plugins.md)** | Custom operations |
| **[Troubleshooting](docs/troubleshooting.md)** | Common issues and solutions |
| **[Development](docs/development.md)** | Building, testing, contributing |

## API Quick Reference

```bash
# Create a test run
curl -X POST http://localhost:8080/runs -H "Content-Type: application/json" -d @config.json

# Start the run
curl -X POST http://localhost:8080/runs/{run_id}/start

# Check status
curl http://localhost:8080/runs/{run_id}

# Stream real-time events
curl -N http://localhost:8080/runs/{run_id}/events

# Graceful stop
curl -X POST http://localhost:8080/runs/{run_id}/stop -H "Content-Type: application/json" -d '{"mode":"drain"}'

# Emergency stop
curl -X POST http://localhost:8080/runs/{run_id}/emergency-stop
```

## Examples

Pre-built configurations in `examples/`:

- `examples/tool-testing/` - Tool validation scenarios
- `examples/scenarios/` - SSE and streaming test scenarios
- `examples/multi-node/` - Docker Compose and Kubernetes setup

## License

MIT License - see [LICENSE](LICENSE) for details.

---

<sub>MCP Drill was originally created to validate the production stability of [Peta](https://github.com/dunialabs/peta-core), an MCP control plane and runtime. If you're building MCP infrastructure that needs to scale, Peta handles the hard parts — routing, auth, observability — so you can focus on your agents.</sub>

