# MCP Drill

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

A high-performance stress testing platform for MCP servers and gateways.

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

### 1. Install

```bash
git clone https://github.com/bc-dunia/mcpdrill.git
cd mcpdrill

# Build binaries
go build -o mcpdrill ./cmd/mcpdrill
go build -o mcpdrill-server ./cmd/server
go build -o mcpdrill-worker ./cmd/worker
```

### 2. Start Services

```bash
# Terminal 1: Start control plane
./mcpdrill-server --addr :8080

# Terminal 2: Start worker
./mcpdrill-worker --control-plane http://localhost:8080
```

### 3. Run Your First Test

Create `test.json`:

```json
{
  "scenario_id": "quick-test",
  "target": {
    "url": "http://localhost:3000/mcp",
    "transport": "streamable_http"
  },
  "stages": [
    {
      "stage_id": "stg_0000000000000001",
      "stage": "ramp",
      "duration_ms": 30000,
      "load": { "target_vus": 10 }
    }
  ],
  "workload": {
    "op_mix": [
      { "operation": "tools/list", "weight": 1 },
      { "operation": "tools/call", "weight": 5, "tool_name": "echo", "arguments": {"message": "test"} }
    ]
  }
}
```

Run the test:

```bash
./mcpdrill create test.json
./mcpdrill start run_0000000000000001
./mcpdrill events run_0000000000000001 --follow
```

### 4. View Results

- **CLI**: `./mcpdrill status run_0000000000000001`
- **Web UI**: Build and access at `http://localhost:8080/ui/logs/`

```bash
cd web/log-explorer && npm install && npm run build
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

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    Control Plane                         │
│  ┌────────────┐  ┌────────────┐  ┌────────────┐        │
│  │Run Manager │  │ Scheduler  │  │  HTTP API  │        │
│  └────────────┘  └────────────┘  └────────────┘        │
└─────────────────────────┬───────────────────────────────┘
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
go build -o mcpdrill-agent ./cmd/agent

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

The `pair_key` links agent metrics with your test runs. See [Agent Telemetry Guide](docs/agent-telemetry.md) for full details.

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

## CLI Quick Reference

```bash
mcpdrill create <config.json>     # Create a test run
mcpdrill start <run_id>           # Start the run
mcpdrill status <run_id>          # Check status
mcpdrill events <run_id> --follow # Stream real-time events
mcpdrill stop <run_id>            # Graceful stop
mcpdrill compare <run_a> <run_b>  # Compare two runs
```

## Examples

Pre-built configurations in `examples/`:

- `examples/tool-testing/` - Tool validation scenarios
- `examples/multi-node/` - Docker Compose setup

## Contributing

Contributions are welcome! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

1. Fork the repository
2. Create a feature branch
3. Make your changes with tests
4. Submit a pull request

## License

MIT License - see [LICENSE](LICENSE) for details.

---

**Resources**: [Issues](https://github.com/bc-dunia/mcpdrill/issues) · [Discussions](https://github.com/bc-dunia/mcpdrill/discussions)
