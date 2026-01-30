# API Reference

Complete REST API reference for MCP Drill Control Plane.

## Base URL

Default: `http://localhost:8080`

## Endpoints

### Run Management

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/runs` | List all runs |
| `POST` | `/runs` | Create run from config |
| `GET` | `/runs/{id}` | Get run status |
| `POST` | `/runs/{id}/start` | Start run |
| `POST` | `/runs/{id}/stop` | Graceful stop |
| `POST` | `/runs/{id}/emergency-stop` | Immediate stop |
| `GET` | `/runs/{id}/events` | Stream events (SSE) |
| `GET` | `/runs/{id}/metrics` | Get aggregated metrics |
| `GET` | `/runs/{id}/stability` | Get connection stability metrics |
| `GET` | `/runs/{id}/logs` | Query operation logs |
| `POST` | `/runs/{id}/validate` | Validate run configuration |
| `GET` | `/runs/{a}/compare/{b}` | Compare two runs |

### Target Discovery

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/test-connection` | Test MCP server connectivity |
| `POST` | `/discover-tools` | Discover available tools |

### System

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/healthz` | Health check |
| `GET` | `/readyz` | Readiness check |
| `GET` | `/workers` | List registered workers |
| `GET` | `/metrics` | Prometheus metrics |
| `GET` | `/agents` | List connected telemetry agents |
| `GET` | `/agents/{id}` | Get agent details |

## Examples

### Create a Run

```bash
curl -X POST http://localhost:8080/runs \
  -H "Content-Type: application/json" \
  -d '{"config": '"$(cat config.json)"', "actor": "api-user"}'

# Response: {"run_id": "run_0000000000000001"}
```

### Start a Run

```bash
curl -X POST http://localhost:8080/runs/run_0000000000000001/start

# Response: {"status": "started"}
```

### Get Run Status

```bash
curl http://localhost:8080/runs/run_0000000000000001

# Response:
# {
#   "run_id": "run_0000000000000001",
#   "state": "running",
#   "current_stage": "ramp",
#   "metrics": {
#     "total_operations": 15420,
#     "operations_per_second": 125.3,
#     "error_rate": 0.02,
#     "latency_p50_ms": 45,
#     "latency_p95_ms": 120,
#     "latency_p99_ms": 250
#   }
# }
```

### Stream Events (SSE)

```bash
curl -N http://localhost:8080/runs/run_0000000000000001/events

# Response (Server-Sent Events):
# event: stage_started
# data: {"stage_id":"stg_0000000000000001","stage":"preflight"}
#
# event: metrics
# data: {"operations":100,"errors":0,"latency_p50":42}
```

### Stop a Run

```bash
curl -X POST http://localhost:8080/runs/run_0000000000000001/stop \
  -H "Content-Type: application/json" \
  -d '{"mode": "drain"}'

# Response: {"status": "stopping", "mode": "drain"}
```

### Test Connection

```bash
curl -X POST http://localhost:8080/test-connection \
  -H "Content-Type: application/json" \
  -d '{"target_url": "http://localhost:3000/mcp"}'

# Response:
# {
#   "success": true,
#   "message": "Connected successfully. Found 27 tools.",
#   "tool_count": 27,
#   "connect_latency_ms": 12,
#   "total_latency_ms": 57
# }
```

### Discover Tools

```bash
curl -X POST http://localhost:8080/discover-tools \
  -H "Content-Type: application/json" \
  -d '{"target_url": "http://localhost:3000/mcp"}'

# Response:
# {
#   "tools": [
#     {"name": "echo", "description": "Echo back the input", "inputSchema": {...}},
#     ...
#   ]
# }
```

## Authentication

### Modes

| Mode | Use Case |
|------|----------|
| `none` | Development/testing (default) |
| `api_key` | Simple deployments |
| `jwt` | Production with RBAC |

### API Key Authentication

```bash
# Start server with API keys
./mcpdrill-server --auth-mode api_key --api-keys "key1,key2"

# Use in requests
curl -H "X-API-Key: key1" http://localhost:8080/runs
```

### JWT Authentication

```bash
# Start server with JWT
./mcpdrill-server --auth-mode jwt --jwt-secret "your-256-bit-secret"

# Use in requests
curl -H "Authorization: Bearer <token>" http://localhost:8080/runs
```

JWT tokens should include:
- `sub`: User identifier
- `iss`: Issuer
- `exp`: Expiration timestamp
- `roles`: Array of roles (`admin`, `operator`, `viewer`)

### Role-Based Access Control

| Role | Permissions |
|------|-------------|
| `admin` | Full access |
| `operator` | Create, start, stop runs |
| `viewer` | Read-only access |
