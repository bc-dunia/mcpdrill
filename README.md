# MCP Drill

A high-performance stress testing platform for MCP servers and gateways.

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**Simulate thousands of concurrent MCP clients, identify performance bottlenecks, and validate your infrastructure before production.**

> Built to battle-test [Peta](https://github.com/dunialabs/peta-core), an MCP control plane and runtime. If you're building MCP infrastructure, MCP Drill helps you break it before your users do.

## Why MCP Drill?

- **Self-Contained** - Run everything on your own infrastructure. No external services, no cloud dependencies
- **Realistic Load Simulation** - Configurable operation mixes with weighted tool selection
- **Multi-stage Testing** - Preflight, baseline, ramp-up, and soak stages
- **Real-time Observability** - Live metrics dashboard with SSE streaming
- **Distributed** - Scale horizontally with multiple workers
- **Safety First** - Built-in stop conditions prevent runaway tests

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

| Component | Role |
|-----------|------|
| **Control Plane** | Manages test runs, schedules work, serves the API and Web UI |
| **Worker** | Spawns virtual users (VUs) that execute MCP operations against the target |
| **Mock Server** | Built-in MCP server with 27 tools for isolated testing — [tool guide](docs/tool-testing-guide.md) |
| **Telemetry Agent** | *(Optional)* Server-side CPU/memory monitoring — [setup guide](docs/agent-telemetry.md) |

## Screenshots

Real-time performance monitoring with throughput, latency percentiles, error rates, and server resource utilization:

![Live Metrics Overview](docs/screenshot1.png)

Per-tool breakdown with success rates, latency distribution, and error analysis:

![Tool Metrics](docs/screenshot2.png)

## Quick Start

**Prerequisites**: Go 1.22+, Node.js 18+ (for Web UI)

### 1. Start Backend

```bash
git clone https://github.com/bc-dunia/mcpdrill.git
cd mcpdrill
make dev
```

This starts all services on loopback (no auth required):

| Service | URL |
|---------|-----|
| Mock Server | http://localhost:3000/mcp |
| Control Plane | http://localhost:8080 |
| Worker | Auto-connected |

Stop with `make dev-stop`. View logs with `make dev-logs`.

### 2. Start Web UI

```bash
cd web/log-explorer
npm install
npm run dev
```

Open **http://localhost:5173**.

### 3. Run a Test

Click **"New Run"** in the Web UI to create and start a test using the built-in wizard.

Or via CLI:

```bash
curl -X POST http://localhost:8080/runs \
  -H "Content-Type: application/json" \
  -d "{\"config\": $(cat examples/quick-start.json)}"

# Start the run (replace with your run_id from the response)
curl -X POST http://localhost:8080/runs/{run_id}/start
```

Results appear in real time in the Web UI dashboard.

> **Note**: The quick-start config targets `http://127.0.0.1:3000/mcp` (IPv4). Using `localhost` may fail due to IPv6 resolution on some systems.

## Key Features

| Feature | Description |
|---------|-------------|
| **Virtual Users (VUs)** | Simulate concurrent MCP clients with weighted operation mixes |
| **Session Modes** | `reuse`, `per_request`, `pool`, `churn` |
| **Stop Conditions** | Auto-stop on error rate, latency thresholds |
| **Mock Server** | 27 built-in tools for isolated testing |
| **Web UI** | Real-time dashboard, log explorer, run wizard |
| **OpenTelemetry** | *(Optional)* Distributed tracing with OTLP export |

## API Quick Reference

```bash
# Create a run
curl -X POST http://localhost:8080/runs -H "Content-Type: application/json" -d @config.json

# Start / Stop / Emergency stop
curl -X POST http://localhost:8080/runs/{run_id}/start
curl -X POST http://localhost:8080/runs/{run_id}/stop -d '{"mode":"drain"}'
curl -X POST http://localhost:8080/runs/{run_id}/emergency-stop

# Status and events
curl http://localhost:8080/runs/{run_id}
curl -N http://localhost:8080/runs/{run_id}/events
```

## Documentation

| Topic | Link |
|-------|------|
| Getting Started | [docs/getting-started.md](docs/getting-started.md) |
| CLI Reference | [docs/cli.md](docs/cli.md) |
| Configuration | [docs/configuration.md](docs/configuration.md) |
| API Reference | [docs/api.md](docs/api.md) |
| Web UI | [docs/web-ui.md](docs/web-ui.md) |
| Mock Server & Tools | [docs/tool-testing-guide.md](docs/tool-testing-guide.md) |
| Server Telemetry Agent | [docs/agent-telemetry.md](docs/agent-telemetry.md) |
| OpenTelemetry | [docs/opentelemetry.md](docs/opentelemetry.md) |
| Multi-Node Deployment | [docs/multi-node-deployment.md](docs/multi-node-deployment.md) |
| Plugins | [docs/plugins.md](docs/plugins.md) |
| Troubleshooting | [docs/troubleshooting.md](docs/troubleshooting.md) |
| Development | [docs/development.md](docs/development.md) |

## Examples

Pre-built configs in [`examples/`](examples/):

- [`quick-start.json`](examples/quick-start.json) - Minimal test against mock server
- [`tool-testing/`](examples/tool-testing/) - Tool validation scenarios
- [`scenarios/`](examples/scenarios/) - SSE and streaming tests
- [`multi-node/`](examples/multi-node/) - Docker Compose and Kubernetes

## License

MIT License - see [LICENSE](LICENSE) for details.

---

<sub>MCP Drill was originally created to validate the production stability of [Peta](https://github.com/dunialabs/peta-core), an MCP control plane and runtime. If you're building MCP infrastructure that needs to scale, Peta handles the hard parts — routing, auth, observability — so you can focus on your agents.</sub>

