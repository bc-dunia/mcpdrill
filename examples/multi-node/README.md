# Multi-Node Deployment Examples

This directory contains example configurations for deploying MCP Drill in distributed mode with a control plane and multiple workers.

## Quick Start

### Docker Compose

> **Note**: The example compose config runs with auth disabled (`--insecure`, `--insecure-worker-auth`) for quick testing. Don’t expose it beyond a trusted network without enabling auth.

**Start the stack** (1 control plane + 3 workers):
```bash
docker compose up -d
```

**Check worker registration**:
```bash
curl http://localhost:8080/workers
```

**Scale workers**:
```bash
docker compose up --scale worker=5 -d
```

**View logs**:
```bash
docker compose logs -f
```

**Stop the stack**:
```bash
docker compose down
```

### Kubernetes

**Apply manifests**:
```bash
kubectl apply -f kubernetes/
```

**Check status**:
```bash
kubectl get pods
kubectl get svc control-plane
kubectl get statefulset worker
```

**Scale workers**:
```bash
kubectl scale statefulset worker --replicas=5
```

**Port-forward control plane**:
```bash
kubectl port-forward svc/control-plane 8080:8080
```

**Check worker registration**:
```bash
curl http://localhost:8080/workers
```

**Delete resources**:
```bash
kubectl delete -f kubernetes/
```

## Configuration

### Docker Compose

**File**: `docker-compose.yml`

**Services**:
- `control-plane`: Server binary, port 8080 exposed
- `worker`: Worker binary, 3 replicas by default

**Customization**:

Change worker capacity:
```yaml
services:
  worker:
    command:
      - --control-plane=http://control-plane:8080
      - --max-vus=200
```

Change resource limits:
```yaml
services:
  worker:
    deploy:
      resources:
        limits:
          cpus: '2'
          memory: 2G
```

### Kubernetes

**Files**:
- `kubernetes/control-plane.yaml`: Deployment + Service
- `kubernetes/worker-statefulset.yaml`: StatefulSet

**Customization**:

Change worker capacity (edit `worker-statefulset.yaml`):
```yaml
args:
- --control-plane=http://control-plane:8080
- --max-vus=200
```

Change resource limits (edit `worker-statefulset.yaml`):
```yaml
resources:
  limits:
    cpu: 2000m
    memory: 2Gi
```

Expose control plane externally (create new file `control-plane-external.yaml`):
```yaml
apiVersion: v1
kind: Service
metadata:
  name: control-plane-external
spec:
  type: LoadBalancer
  selector:
    app: mcpdrill-control-plane
  ports:
  - port: 8080
    targetPort: 8080
```

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     Control Plane                            │
│                    (port 8080)                               │
└────────────────────────────┬─────────────────────────────────┘
                             │
                    ┌────────┴────────┐
                    │                 │
              ┌─────▼─────┐     ┌─────▼─────┐
              │  Worker 1 │     │  Worker 2 │  ...
              │  (100 VUs)│     │  (100 VUs)│
              └───────────┘     └───────────┘
```

**Control Plane**:
- Manages runs, schedules work, aggregates results
- Exposes HTTP API on port 8080
- Tracks worker health via heartbeats

**Workers**:
- Execute VUs according to assignments
- Send heartbeats every 10s
- Poll for assignments every 5s
- Send telemetry every 10s

## Testing the Deployment

### 1. Create a Run

Create `test-config.json`:
```json
{
  "scenario_id": "multi-node-test",
  "target": {
    "kind": "server",
    "url": "http://your-mcp-server:3000",
    "transport": "streamable_http"
  },
  "stages": [
    {
      "stage_id": "stg_0000000000000001",
      "stage": "preflight",
      "enabled": true,
      "duration_ms": 10000,
      "load": {
        "target_vus": 1
      }
    },
    {
      "stage_id": "stg_0000000000000002",
      "stage": "baseline",
      "enabled": true,
      "duration_ms": 30000,
      "load": {
        "target_vus": 10
      }
    }
  ],
  "workload": {
    "op_mix": [
      {
        "operation": "tools/list",
        "weight": 1
      }
    ]
  },
  "safety": {
    "hard_caps": {
      "max_vus": 100,
      "max_duration_ms": 300000
    },
    "worker_failure_policy": "replace_if_possible"
  },
  "session_policy": {
    "mode": "reuse"
  },
  "environment": {
    "allowlist": {
      "mode": "deny_by_default",
      "allowed_hosts": ["your-mcp-server"]
    }
  }
}
```

### 2. Submit the Run

```bash
curl -X POST http://localhost:8080/runs \
  -H "Content-Type: application/json" \
  -d @test-config.json
```

Response:
```json
{
  "run_id": "run_0000000000000001"
}
```

### 3. Start the Run

```bash
curl -X POST http://localhost:8080/runs/run_0000000000000001/start
```

### 4. Monitor Progress

**Check run status**:
```bash
curl http://localhost:8080/runs/run_0000000000000001
```

**Stream events**:
```bash
curl -N http://localhost:8080/runs/run_0000000000000001/events
```

**Check worker health**:
```bash
curl http://localhost:8080/workers
```

### 5. Stop the Run

```bash
curl -X POST http://localhost:8080/runs/run_0000000000000001/stop \
  -H "Content-Type: application/json" \
  -d '{"mode": "drain"}'
```

## Troubleshooting

### Workers Not Registering

**Check control plane logs**:
```bash
# Docker Compose
docker compose logs control-plane

# Kubernetes
kubectl logs -l app=mcpdrill-control-plane
```

**Check worker logs**:
```bash
# Docker Compose
docker compose logs worker

# Kubernetes
kubectl logs worker-0
```

**Verify network connectivity**:
```bash
# Docker Compose
docker compose exec worker ping control-plane

# Kubernetes
kubectl exec worker-0 -- wget -O- http://control-plane:8080/healthz
```

### Workers Timing Out

**Symptom**: Workers removed after 30s

**Check heartbeat logs**:
```bash
# Look for "Heartbeat sent successfully" in worker logs
docker compose logs worker | grep Heartbeat
```

**Increase heartbeat interval** (if high latency):
```yaml
services:
  worker:
    command:
      - --control-plane=http://control-plane:8080
      - --heartbeat-interval=20s
```

### Insufficient Capacity

**Symptom**: Run fails with "insufficient capacity"

**Check worker count**:
```bash
curl http://localhost:8080/workers | jq '.workers | length'
```

**Scale up workers**:
```bash
# Docker Compose
docker compose up --scale worker=5 -d

# Kubernetes
kubectl scale statefulset worker --replicas=5
```

## Next Steps

- Read the [Multi-Node Deployment Guide](../../docs/multi-node-deployment.md) for detailed information
- See the [Main README](../../README.md) for general usage
- Check the [API Reference](../../docs/api.md) for API details
