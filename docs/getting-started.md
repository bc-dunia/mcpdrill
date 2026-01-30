# Getting Started

This guide walks you through installing MCP Drill and running your first stress test.

## Prerequisites

- **Go 1.22+** - Required for building from source
- **Node.js 18+** - Optional, for building the Web UI
- **Target MCP Server** - The server you want to test

## Supported Platforms

| OS | Architecture | Status |
|----|--------------|--------|
| Linux | amd64, arm64 | Fully supported |
| macOS | amd64, arm64 (Apple Silicon) | Fully supported |
| Windows | amd64 | Experimental (use WSL2 for production) |

## Installation

### Build from Source

```bash
git clone https://github.com/bc-dunia/mcpdrill.git
cd mcpdrill

# Build all binaries
go build -o mcpdrill ./cmd/mcpdrill
go build -o mcpdrill-server ./cmd/server
go build -o mcpdrill-worker ./cmd/worker

# Optional components
go build -o mcpdrill-agent ./cmd/agent        # Server telemetry
go build -o mcpdrill-mockserver ./cmd/mockserver  # Mock MCP server
```

### Verify Installation

```bash
./mcpdrill --help
```

### Build Web UI (Optional)

```bash
cd web/log-explorer
npm install
npm run build
cd ../..
```

## Quick Start

### 1. Start the Control Plane

```bash
./mcpdrill-server --addr :8080
```

### 2. Start Worker(s)

In a new terminal:

```bash
./mcpdrill-worker --control-plane http://localhost:8080
```

### 3. Create Test Configuration

Create `test.json`:

```json
{
  "scenario_id": "basic-load-test",
  "target": {
    "kind": "server",
    "url": "http://localhost:3000/mcp",
    "transport": "streamable_http"
  },
  "stages": [
    {
      "stage_id": "stg_0000000000000001",
      "stage": "preflight",
      "duration_ms": 10000,
      "load": { "target_vus": 1 }
    },
    {
      "stage_id": "stg_0000000000000002",
      "stage": "ramp",
      "duration_ms": 60000,
      "load": { "target_vus": 50 },
      "stop_conditions": [
        { "metric": "error_rate", "threshold": 0.1, "window_ms": 10000 }
      ]
    }
  ],
  "workload": {
    "op_mix": [
      { "operation": "tools/list", "weight": 1 },
      { "operation": "tools/call", "weight": 5, "tool_name": "echo", "arguments": {"message": "test"} }
    ]
  },
  "session_policy": { "mode": "reuse" },
  "safety": {
    "hard_caps": { "max_vus": 100, "max_duration_ms": 300000 }
  }
}
```

### 4. Run the Test

```bash
# Create the run
./mcpdrill create test.json
# Output: Created run: run_0000000000000001

# Start the run
./mcpdrill start run_0000000000000001

# Monitor in real-time
./mcpdrill events run_0000000000000001 --follow

# Check status
./mcpdrill status run_0000000000000001
```

## Using the Mock Server

For testing without a real MCP server:

```bash
# Start mock server
./mcpdrill-mockserver --addr :3000

# The mock server provides 27 built-in tools at http://localhost:3000/mcp
```

See [Tool Testing Guide](tool-testing-guide.md) for details.

## Docker Quick Start

```bash
cd examples/multi-node

# Start control plane + 3 workers
docker-compose up -d

# Verify workers
curl http://localhost:8080/workers

# Scale workers
docker-compose up --scale worker=5 -d
```

See [Multi-Node Deployment](multi-node-deployment.md) for details.

## Next Steps

- [CLI Reference](cli.md) - Learn all commands
- [Configuration](configuration.md) - Understand config options
- [Web UI](web-ui.md) - Use the visual dashboard
