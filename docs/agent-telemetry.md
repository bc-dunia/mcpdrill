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
| `--pid <pid>` | Monitor by explicit PID (mutually exclusive with `--listen-port`) |
| `--listen-port <port>` | Find process listening on port (mutually exclusive with `--pid`) |

### Optional Tuning

| Flag | Default | Description |
|------|---------|-------------|
| `--tls-ca-file` | - | Custom CA certificate |
| `--tls-insecure-skip-verify` | false | Skip TLS verification |

**Note:** The following flags are deprecated and will be removed in a future version:
- `--sample-interval-ms` (collection frequency is now fixed)
- `--push-interval-ms` (push frequency is now fixed)
- `--buffer-seconds` (buffer size is now fixed)
- `--tags` (custom metadata not currently supported)
- `--process-regex` (use `--pid` or `--listen-port` instead)

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
| `cpu_percent` | Total CPU usage % (0-100) |
| `load_avg_1` | 1-minute load average |
| `load_avg_5` | 5-minute load average |
| `load_avg_15` | 15-minute load average |
| `mem_total` | Total system memory (bytes) |
| `mem_used` | Used system memory (bytes) |
| `mem_available` | Available system memory (bytes) |
| `disk_used_percent` | Disk usage % for primary partition (0-100) |
| `network_bytes_in` | Total bytes received since boot |
| `network_bytes_out` | Total bytes sent since boot |
| `swap_used` | Swap usage (bytes) - *coming soon* |

### Process Metrics

| Metric | Description |
|--------|-------------|
| `pid` | Process ID |
| `cpu_percent` | Process CPU usage % |
| `mem_rss` | Resident set size (physical memory, bytes) |
| `mem_vms` | Virtual memory size (bytes) |
| `num_threads` | Number of threads in the process |
| `num_fds` | Number of open file descriptors (Unix only) |
| `open_connections` | Number of open network connections |

## API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/agents/v1/register` | Agent registration |
| `POST` | `/agents/v1/metrics` | Metrics ingestion |
| `GET` | `/agents` | List connected agents |
| `GET` | `/agents/{id}` | Get agent details |
| `GET` | `/runs/{id}/server-metrics` | Query server metrics for run |

### Example Metrics Response

```json
{
  "host_metrics": {
    "cpu_percent": 45.2,
    "mem_total": 17179869184,
    "mem_used": 12884901888,
    "mem_available": 4294967296,
    "load_avg_1": 2.5,
    "load_avg_5": 2.1,
    "load_avg_15": 1.8,
    "disk_used_percent": 68.5,
    "network_bytes_in": 1234567890,
    "network_bytes_out": 987654321
  },
  "process_metrics": {
    "pid": 12345,
    "cpu_percent": 23.4,
    "mem_rss": 536870912,
    "mem_vms": 1073741824,
    "num_threads": 8,
    "num_fds": 42,
    "open_connections": 15
  }
}
```

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
