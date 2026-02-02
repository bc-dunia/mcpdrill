# Web UI

MCP Drill includes a React-based Web UI for visual test management and monitoring.

## Development Setup

Start the UI development server:

```bash
cd web/log-explorer
npm install
npm run dev
```

This starts the Vite development server with hot reloading.

## Access

Open **http://localhost:5173** in your browser.

> **Note**: The UI connects to the control plane at `http://localhost:8080`. Make sure the backend is running (`make dev`).

## Features

### Metrics Dashboard

Real-time monitoring of your test runs:

| Feature | Description |
|---------|-------------|
| **Throughput Chart** | Real-time operations per second |
| **Latency Chart** | P50/P95/P99 percentiles over time |
| **Error Rate Chart** | Error percentage with threshold line |
| **Connection Stability** | Active sessions, drops, reconnects |
| **Stop Control** | Inline stop button with mode selection |
| **Auto-refresh** | Toggle for live updates |
| **Tool Metrics** | Per-tool performance breakdown |

### Log Explorer

Browse and filter operation logs:

- Full-text search
- Filter by operation, tool, status
- Filter by stage, session, VU
- Export to CSV

### Run Wizard

Create test configurations visually:

| Step | Features |
|------|----------|
| **Target Config** | URL input, transport selection, custom headers, **Test Connection** button |
| **Workload Config** | Operation mix, tool selector with schema view |
| **Session Policy** | Session mode selection, pool size |
| **Safety Config** | Hard caps, stop conditions |
| **Review** | JSON preview, validation, create run |

### Run Comparison

Side-by-side comparison of two runs:

- Metric deltas with improvement indicators
- Latency distribution comparison
- Error rate trends

### Tool Selector

Browse and select tools for testing:

- Filter: All / No params / With params
- Schema view with required field indicators
- Random tool selection
- Copy tool name to clipboard

## API Dependencies

| Feature | Endpoints Used |
|---------|----------------|
| Run Wizard | `/test-connection`, `/discover-tools`, `/runs` |
| Metrics Dashboard | `/runs/{id}`, `/runs/{id}/metrics`, `/runs/{id}/stability` |
| Log Explorer | `/runs/{id}/logs` |
| Run Comparison | `/runs/{a}/compare/{b}` |

## Authentication

If Control Plane authentication is enabled, the Web UI requires valid credentials. Configure auth appropriately for your deployment.
