# Server Telemetry Agent

The optional `mcpdrill-agent` collects server-side metrics (CPU, memory, file descriptors, threads) from the machine running your MCP server.

## Architecture

```
┌─────────────────────────────────────┐     ┌─────────────────────────────────┐
│     Remote Server (your MCP host)   │     │      Control Plane Server       │
│                                     │     │                                 │
│  ┌─────────────┐  ┌───────────────┐ │     │  ┌───────────────────────────┐  │
│  │ MCP Server  │  │ mcpdrill-agent│─┼────▶│  │  /agents/v1/register      │  │
│  │ (port 3000) │  │               │ │     │  │  /agents/v1/metrics       │  │
│  └─────────────┘  └───────────────┘ │     │  └───────────────────────────┘  │
│        ▲                │           │     │                                 │
│        └── monitors ────┘           │     │   Correlates via pair_key       │
└─────────────────────────────────────┘     └─────────────────────────────────┘
            Outbound HTTPS only ─────────────────▶
```

Agents initiate outbound connections to the Control Plane. This works behind NAT/firewalls.

## Building

```bash
go build -o mcpdrill-agent ./cmd/agent
```

## Enabling on Control Plane

```bash
./mcpdrill-server \
  --addr :8080 \
  --enable-agent-ingest \
  --agent-tokens "your-secret-token"
```

| Flag | Description |
|------|-------------|
| `--enable-agent-ingest` | Enable agent telemetry endpoints |
| `--agent-tokens` | Comma-separated list of valid tokens |

## Running the Agent

```bash
./mcpdrill-agent \
  --control-plane-url https://your-control-plane:8080 \
  --agent-token "your-secret-token" \
  --pair-key "my-load-test" \
  --listen-port 3000
```

### Required Flags

| Flag | Description |
|------|-------------|
| `--control-plane-url` | Control Plane URL |
| `--agent-token` | Authentication token |
| `--pair-key` | Links agent metrics with runs |

### Process Selection (choose one)

| Flag | Description |
|------|-------------|
| `--pid <pid>` | Monitor by explicit PID |
| `--listen-port <port>` | Find process listening on port |
| `--process-regex "<regex>"` | Match by command line |

### Optional Tuning

| Flag | Default | Description |
|------|---------|-------------|
| `--sample-interval-ms` | 1000 | Collection frequency |
| `--push-interval-ms` | 5000 | Push batch frequency |
| `--buffer-seconds` | 60 | Local buffer size |
| `--tags` | - | Custom metadata |
| `--tls-ca-file` | - | Custom CA certificate |
| `--tls-insecure-skip-verify` | false | Skip TLS verification |

## Linking with Runs

1. **Agent side:** Configure `--pair-key "my-load-test"`
2. **Run config:** Include matching key:

```json
{
  "scenario_id": "capacity-test",
  "server_telemetry": {
    "enabled": true,
    "pair_key": "my-load-test"
  },
  "target": { ... }
}
```

## Metrics Collected

### Host Metrics

| Metric | Description |
|--------|-------------|
| `cpu_percent` | Total CPU usage % |
| `load_avg_1/5/15` | Load averages |
| `mem_total/used/free` | Memory usage (bytes) |
| `swap_used` | Swap usage (bytes) |

### Process Metrics

| Metric | Description |
|--------|-------------|
| `cpu_percent` | Process CPU % |
| `rss_bytes` | Resident memory |
| `virtual_bytes` | Virtual memory |
| `thread_count` | Thread count |
| `fd_count` | Open file descriptors |

## API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/agents/v1/register` | Agent registration |
| `POST` | `/agents/v1/metrics` | Metrics ingestion |
| `GET` | `/agents` | List connected agents |
| `GET` | `/agents/{id}` | Get agent details |
| `GET` | `/runs/{id}/server-metrics` | Query server metrics for run |

## Troubleshooting

**Agent not connecting:**
```bash
curl https://your-control-plane:8080/healthz
./mcpdrill-agent --control-plane-url ... 2>&1 | grep -i error
```

**Metrics not appearing:**
- Verify `pair_key` matches exactly
- Check agent is registered: `curl http://localhost:8080/agents`
- Ensure `server_telemetry.enabled: true` in run config

**TLS errors:**
```bash
--tls-ca-file /path/to/ca.pem
# Dev only:
--tls-insecure-skip-verify
```
