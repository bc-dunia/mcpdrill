# Multi-Node Deployment Guide

This guide explains how to deploy MCP Drill in distributed mode with a single control plane and multiple workers across different nodes.

## Table of Contents

- [Overview](#overview)
- [Architecture](#architecture)
- [Network Requirements](#network-requirements)
- [Docker Compose Deployment](#docker-compose-deployment)
- [Kubernetes Deployment](#kubernetes-deployment)
- [Configuration](#configuration)
- [Monitoring](#monitoring)
- [Troubleshooting](#troubleshooting)
- [Best Practices](#best-practices)

## Overview

MCP Drill supports distributed deployment where:

- **Control Plane** (server) manages runs, schedules work, and aggregates results
- **Workers** execute virtual users (VUs) and send telemetry back to the control plane
- **Communication** happens over HTTP (default port 8080)

This architecture allows you to:

- Scale horizontally by adding more workers
- Distribute load across multiple machines
- Isolate control plane from data plane for better resource management
- Run high-scale tests (1000+ VUs) across multiple nodes

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     Control Plane                            │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ Run Manager  │  │  Scheduler   │  │  HTTP API    │      │
│  │ (State Mgmt) │  │ (Worker Mgmt)│  │ (REST + SSE) │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
│         │                  │                  │              │
│         └──────────────────┴──────────────────┘              │
│                            │                                 │
└────────────────────────────┼─────────────────────────────────┘
                             │
                    ┌────────┴────────┐
                    │                 │
              ┌─────▼─────┐     ┌─────▼─────┐
              │  Worker 1 │     │  Worker 2 │  ...
              │  (100 VUs)│     │  (100 VUs)│
              └───────────┘     └───────────┘
                    │                 │
                    └────────┬────────┘
                             │
                    ┌────────▼────────┐
                    │   MCP Target    │
                    │ (Gateway/Server)│
                    └─────────────────┘
```

### Control Plane Responsibilities

- **Run Lifecycle Management**: Create, start, stop runs
- **Worker Registry**: Track registered workers and their health
- **Scheduling**: Allocate VUs to workers based on capacity
- **Telemetry Aggregation**: Collect operation results from workers
- **Analysis & Reporting**: Generate reports when runs complete

### Worker Responsibilities

- **VU Execution**: Run virtual users according to assignments
- **Session Management**: Maintain MCP sessions (reuse, pool, per-request, churn)
- **Telemetry Emission**: Send operation results to control plane
- **Health Reporting**: Report CPU, memory, active VUs, sessions

### Communication Flow

1. **Registration**: Worker registers with control plane on startup
2. **Heartbeat**: Worker sends heartbeat every 10s (configurable)
3. **Assignment Polling**: Worker polls for assignments every 5s (configurable)
4. **VU Execution**: Worker executes VUs when assignment received
5. **Telemetry**: Worker sends operation results every 10s (configurable)

## Network Requirements

### Connectivity

- **Control plane must be reachable from all workers**
  - Workers need outbound HTTP access to control plane
  - Control plane must accept inbound connections on configured port (default 8080)

- **Workers need outbound access to MCP target**
  - Workers make HTTP requests to the target MCP server
  - Ensure firewall rules allow outbound traffic

### Firewall Rules

**Control Plane**:
- Allow inbound TCP on port 8080 (or configured port) from worker nodes
- Allow outbound TCP for responses

**Workers**:
- Allow outbound TCP to control plane port 8080
- Allow outbound TCP to MCP target (typically port 3000 or 443)

### DNS/Service Discovery

- **Docker Compose**: Uses Docker's built-in DNS (service names resolve automatically)
- **Kubernetes**: Uses Kubernetes DNS (service names resolve to ClusterIP)
- **Bare Metal**: Use static IPs or configure DNS entries

### Latency Considerations

- **Heartbeat timeout**: 30s (3x heartbeat interval)
- **Network latency**: Should be < 1s between control plane and workers
- **High latency**: May cause heartbeat timeouts and worker removal

## Docker Compose Deployment

### Prerequisites

- Docker 20.10+ and Docker Compose 1.29+
- Network connectivity between containers
- Sufficient resources (CPU, memory) for control plane + workers

### Quick Start

1. **Clone the repository**:
   ```bash
   git clone https://github.com/bc-dunia/mcpdrill.git
   cd mcpdrill/examples/multi-node
   ```

2. **Start the stack**:
   ```bash
   docker-compose up -d
   ```

   This starts:
   - 1 control plane (port 8080 exposed)
   - 3 workers (100 VUs each)

3. **Check worker registration**:
   ```bash
   curl http://localhost:8080/workers
   ```

   You should see 3 registered workers with their capacity and health.

4. **Scale workers**:
   ```bash
   docker-compose up --scale worker=5 -d
   ```

   This adds 2 more workers (total 5).

5. **View logs**:
   ```bash
   # Control plane logs
   docker-compose logs -f control-plane

   # Worker logs
   docker-compose logs -f worker

   # All logs
   docker-compose logs -f
   ```

6. **Stop the stack**:
   ```bash
   docker-compose down
   ```

### Configuration

See `examples/multi-node/docker-compose.yml` for full configuration.

**Key settings**:
- Control plane address: `--addr :8080`
- Worker control plane URL: `--control-plane http://control-plane:8080`
- Worker max VUs: `--max-vus 100` (default)
- Heartbeat interval: `--heartbeat-interval 10s` (default)

**Environment variables** (optional):
```yaml
environment:
  - MCPDRILL_CONTROL_PLANE=http://control-plane:8080
  - MCPDRILL_MAX_VUS=100
```

### Customization

**Change worker capacity**:
```yaml
services:
  worker:
    command: ["--control-plane", "http://control-plane:8080", "--max-vus", "200"]
```

**Change heartbeat interval**:
```yaml
services:
  worker:
    command: ["--control-plane", "http://control-plane:8080", "--heartbeat-interval", "5s"]
```

**Add resource limits**:
```yaml
services:
  worker:
    deploy:
      resources:
        limits:
          cpus: '2'
          memory: 2G
        reservations:
          cpus: '1'
          memory: 1G
```

## Kubernetes Deployment

### Prerequisites

- Kubernetes cluster 1.20+
- kubectl configured to access cluster
- Sufficient cluster resources (CPU, memory)

### Quick Start

1. **Apply manifests**:
   ```bash
   kubectl apply -f examples/multi-node/kubernetes/
   ```

   This creates:
   - 1 control plane Deployment (1 replica)
   - 1 control plane Service (ClusterIP)
   - 1 worker StatefulSet (3 replicas)

2. **Check control plane**:
   ```bash
   kubectl get pods -l app=mcpdrill-control-plane
   kubectl logs -l app=mcpdrill-control-plane
   ```

3. **Check workers**:
   ```bash
   kubectl get statefulset worker
   kubectl get pods -l app=mcpdrill-worker
   kubectl logs worker-0
   ```

4. **Check worker registration**:
   ```bash
   # Port-forward control plane
   kubectl port-forward svc/control-plane 8080:8080

   # In another terminal
   curl http://localhost:8080/workers
   ```

5. **Scale workers**:
   ```bash
   kubectl scale statefulset worker --replicas=5
   ```

6. **Delete resources**:
   ```bash
   kubectl delete -f examples/multi-node/kubernetes/
   ```

### Configuration

See `examples/multi-node/kubernetes/` for full manifests.

**Control Plane** (`control-plane.yaml`):
- Deployment with 1 replica
- Service (ClusterIP) on port 8080
- Resource limits: 500m CPU, 512Mi memory
- Liveness/readiness probes on `/healthz` and `/readyz`

**Workers** (`worker-statefulset.yaml`):
- StatefulSet with 3 replicas
- Resource limits: 1000m CPU, 1Gi memory
- Connects to `http://control-plane:8080`
- Max VUs: 100 per worker

### Customization

**Change worker capacity**:
```yaml
spec:
  template:
    spec:
      containers:
      - name: worker
        args:
        - --control-plane=http://control-plane:8080
        - --max-vus=200
```

**Change resource limits**:
```yaml
resources:
  limits:
    cpu: 2000m
    memory: 2Gi
  requests:
    cpu: 1000m
    memory: 1Gi
```

**Expose control plane externally** (LoadBalancer):
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

**Use NodePort**:
```yaml
spec:
  type: NodePort
  ports:
  - port: 8080
    targetPort: 8080
    nodePort: 30080
```

## Configuration

### Server Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--addr` | `:8080` | HTTP server address (host:port) |

**Example**:
```bash
./mcpdrill-server --addr :9090
```

### Worker Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--control-plane` | `http://localhost:8080` | Control plane URL |
| `--worker-id` | (auto-assigned) | Worker ID (optional) |
| `--heartbeat-interval` | `10s` | Heartbeat interval |
| `--assignment-poll-interval` | `5s` | Assignment poll interval |
| `--telemetry-interval` | `10s` | Telemetry send interval |

**Example**:
```bash
./mcpdrill-worker \
  --control-plane http://control-plane:8080 \
  --heartbeat-interval 5s \
  --telemetry-interval 5s
```

### Worker Capacity

Workers report capacity during registration:

- **MaxVUs**: Maximum virtual users (default: 100)
- **MaxConcurrentOps**: Maximum concurrent operations (default: 50)
- **MaxRPS**: Maximum requests per second (default: 1000)

These are currently hardcoded in `cmd/worker/main.go` but can be made configurable in future versions.

### Worker Failure Policies

Configure in run config JSON:

```json
{
  "safety": {
    "worker_failure_policy": "fail_fast"
  }
}
```

**Policies**:
- `fail_fast` (default): Stop run immediately if worker fails
- `replace_if_possible`: Try to reallocate VUs to other workers, stop if impossible
- `best_effort`: Continue with reduced capacity (risky)

### Heartbeat Timeout

Workers are removed if they don't send heartbeat within 30s (3x heartbeat interval).

This is currently hardcoded in `internal/controlplane/scheduler/heartbeat_monitor.go` but can be made configurable in future versions.

## Monitoring

### Worker Health

**Check registered workers**:
```bash
curl http://control-plane:8080/workers
```

**Response**:
```json
{
  "workers": [
    {
      "worker_id": "wkr_0000000000000001",
      "host_info": {
        "hostname": "worker-0",
        "ip_addr": "",
        "platform": "linux/amd64"
      },
      "capacity": {
        "max_vus": 100,
        "max_concurrent_ops": 50,
        "max_rps": 1000
      },
      "health": {
        "cpu_percent": 45.2,
        "mem_bytes": 524288000,
        "active_vus": 20,
        "active_sessions": 5,
        "in_flight_ops": 10,
        "queue_depth": 0
      },
      "saturated": false,
      "last_heartbeat": "2026-01-27T10:30:45Z"
    }
  ]
}
```

**Key metrics**:
- `active_vus`: Current VUs running on worker
- `cpu_percent`: CPU utilization (saturated if > 90%)
- `mem_bytes`: Memory usage
- `saturated`: True if worker is overloaded (CPU > 90% or VUs at max)
- `last_heartbeat`: Last heartbeat timestamp

### Run Status

**Check run status**:
```bash
curl http://control-plane:8080/runs/{run_id}
```

**Response**:
```json
{
  "run_id": "run_0000000000000001",
  "state": "BASELINE_RUNNING",
  "created_at": "2026-01-27T10:30:00Z",
  "started_at": "2026-01-27T10:30:05Z",
  "active_stage": {
    "stage_id": "stg_0000000000000001",
    "started_at": "2026-01-27T10:30:10Z"
  }
}
```

### Event Stream

**Stream run events** (SSE):
```bash
curl -N http://control-plane:8080/runs/{run_id}/events
```

**Example events**:
```
id: 0
data: {"event_type":"RUN_CREATED","timestamp":"2026-01-27T10:30:00Z","run_id":"run_0000000000000001"}

id: 1
data: {"event_type":"STATE_TRANSITION","timestamp":"2026-01-27T10:30:05Z","run_id":"run_0000000000000001","from_state":"CREATED","to_state":"PREFLIGHT_RUNNING"}

id: 2
data: {"event_type":"WORKER_CAPACITY_LOST","timestamp":"2026-01-27T10:30:15Z","run_id":"run_0000000000000001","worker_id":"wkr_0000000000000001"}
```

### Health Checks

**Control plane health**:
```bash
curl http://control-plane:8080/healthz
# Response: {"status":"ok"}

curl http://control-plane:8080/readyz
# Response: {"status":"ready","ready":true}
```

### Metrics to Monitor

**Control Plane**:
- Worker count (should match expected)
- Heartbeat latency (should be < 1s)
- Event log size (grows over time)

**Workers**:
- CPU utilization (should be < 90%)
- Memory usage (should not grow unbounded)
- Active VUs (should match assignments)
- Active sessions (should match session policy)

## Troubleshooting

### Worker Registration Failures

**Symptom**: Workers not appearing in `/workers` endpoint

**Possible Causes**:
- Network connectivity issues
- Wrong control plane URL
- Firewall blocking port 8080
- Control plane not running

**Solutions**:

1. **Check network connectivity**:
   ```bash
   # From worker node
   ping control-plane
   curl http://control-plane:8080/healthz
   ```

2. **Verify control plane URL**:
   ```bash
   # Check worker logs
   docker-compose logs worker
   # Look for: "Registering with Control Plane at http://control-plane:8080"
   ```

3. **Check firewall rules**:
   ```bash
   # On control plane node
   sudo iptables -L -n | grep 8080
   ```

4. **Check control plane status**:
   ```bash
   curl http://control-plane:8080/healthz
   ```

### Worker Heartbeat Timeouts

**Symptom**: Workers removed after 30s, events show `WORKER_CAPACITY_LOST`

**Possible Causes**:
- Network partition
- Worker crash
- High network latency (> 1s)
- Worker overloaded (CPU > 100%)

**Solutions**:

1. **Check worker logs**:
   ```bash
   docker-compose logs worker
   kubectl logs worker-0
   # Look for errors or crashes
   ```

2. **Verify network stability**:
   ```bash
   # From worker node
   ping -c 100 control-plane
   # Check for packet loss
   ```

3. **Check worker health**:
   ```bash
   curl http://control-plane:8080/workers
   # Look for high CPU or memory usage
   ```

4. **Increase heartbeat interval** (if high latency):
   ```bash
   ./mcpdrill-worker --control-plane http://control-plane:8080 --heartbeat-interval 20s
   ```

### Insufficient Capacity

**Symptom**: Run fails with "insufficient capacity" error

**Possible Causes**:
- Not enough workers
- Workers saturated (CPU > 90%)
- Target VUs exceeds total worker capacity

**Solutions**:

1. **Scale up workers**:
   ```bash
   # Docker Compose
   docker-compose up --scale worker=5 -d

   # Kubernetes
   kubectl scale statefulset worker --replicas=5
   ```

2. **Check worker saturation**:
   ```bash
   curl http://control-plane:8080/workers
   # Look for "saturated": true
   ```

3. **Reduce target VUs**:
   ```json
   {
     "stages": [
       {
         "stage_id": "stg_0000000000000001",
         "load": {
           "target_vus": 50  // Reduce from 100
         }
       }
     ]
   }
   ```

4. **Wait for workers to recover**:
   - Workers unsaturate when CPU < 80% and VUs < max
   - May take 10-30s after load decreases

### Worker Saturation

**Symptom**: Workers show `"saturated": true`, new assignments not issued

**Possible Causes**:
- CPU > 90%
- Active VUs >= max VUs
- Insufficient worker resources

**Solutions**:

1. **Check worker health**:
   ```bash
   curl http://control-plane:8080/workers
   # Look for high CPU or active_vus >= max_vus
   ```

2. **Increase worker resources**:
   ```yaml
   # Docker Compose
   deploy:
     resources:
       limits:
         cpus: '2'
         memory: 2G

   # Kubernetes
   resources:
     limits:
       cpu: 2000m
       memory: 2Gi
   ```

3. **Reduce VU load**:
   - Lower target VUs in run config
   - Increase think time between operations

4. **Add more workers**:
   - Scale horizontally instead of vertically

### Assignment Not Received

**Symptom**: Run started but workers not executing VUs

**Possible Causes**:
- Worker not polling assignments
- Assignment poll interval too long
- Worker already has active assignment

**Solutions**:

1. **Check worker logs**:
   ```bash
   docker-compose logs worker
   # Look for: "Received assignment: run=..."
   ```

2. **Reduce poll interval**:
   ```bash
   ./mcpdrill-worker --control-plane http://control-plane:8080 --assignment-poll-interval 2s
   ```

3. **Check run state**:
   ```bash
   curl http://control-plane:8080/runs/{run_id}
   # Should be in PREFLIGHT_RUNNING, BASELINE_RUNNING, or RAMP_RUNNING
   ```

### Telemetry Not Received

**Symptom**: Run completes but no results in report

**Possible Causes**:
- Worker not sending telemetry
- Telemetry interval too long
- Network issues

**Solutions**:

1. **Check worker logs**:
   ```bash
   docker-compose logs worker
   # Look for: "Telemetry sent: X operations accepted"
   ```

2. **Reduce telemetry interval**:
   ```bash
   ./mcpdrill-worker --control-plane http://control-plane:8080 --telemetry-interval 5s
   ```

3. **Check control plane logs**:
   ```bash
   docker-compose logs control-plane
   # Look for telemetry ingestion errors
   ```

## Best Practices

### Production Deployment

1. **Use at least 3 workers for redundancy**
   - Allows for worker failure without stopping runs (with `replace_if_possible` policy)
   - Distributes load more evenly

2. **Size workers based on target VUs**
   - Rule of thumb: 100 VUs per worker
   - Adjust based on operation complexity and target latency

3. **Monitor worker health metrics**
   - Set up alerts for CPU > 80%, memory > 80%
   - Monitor heartbeat latency (should be < 1s)

4. **Use low-latency network**
   - Control plane <-> worker latency should be < 100ms
   - Workers <-> target latency depends on test scenario

5. **Restrict control plane access**
   - Use firewall rules to allow only worker nodes
   - Consider TLS for production (not yet implemented)

6. **Set resource limits**
   - Prevent workers from consuming all node resources
   - Use Docker/Kubernetes resource limits

7. **Use StatefulSet for workers** (Kubernetes)
   - Provides stable network identity
   - Easier to track individual workers

8. **Configure worker failure policy**
   - `fail_fast`: Safest, stops run on worker failure
   - `replace_if_possible`: Resilient, tries to continue
   - `best_effort`: Risky, only for advanced users

### Capacity Planning

**Calculate total capacity**:
```
Total VUs = (Number of Workers) × (Max VUs per Worker)
```

**Example**:
- 5 workers × 100 VUs = 500 total VUs
- Leave 20% headroom for saturation: 400 usable VUs

**Sizing guidelines**:
- **Small tests** (< 50 VUs): 1 worker
- **Medium tests** (50-200 VUs): 2-3 workers
- **Large tests** (200-1000 VUs): 5-10 workers
- **Very large tests** (1000+ VUs): 10+ workers

### Network Optimization

1. **Colocate workers with target**
   - Reduces network latency
   - More realistic load testing

2. **Use dedicated network**
   - Avoid shared networks with other services
   - Prevents network contention

3. **Monitor network bandwidth**
   - High VU counts can saturate network
   - Consider network limits when sizing

### Monitoring and Alerting

**Key metrics to monitor**:
- Worker count (should match expected)
- Worker saturation rate (should be < 20%)
- Heartbeat timeout rate (should be 0%)
- Run failure rate (should be < 5%)

**Recommended alerts**:
- Worker count drops below expected
- Worker saturation > 50% for > 5 minutes
- Heartbeat timeout rate > 10%
- Control plane CPU > 80%

### Security Considerations

1. **Network isolation**
   - Use private networks for control plane <-> worker communication
   - Restrict control plane access to worker nodes only

2. **TLS encryption** (future)
   - Not yet implemented
   - Planned for future versions

3. **Authentication** (future)
   - Not yet implemented
   - Planned for future versions

4. **Resource limits**
   - Prevent DoS via excessive VUs
   - Use `max_vus` in run config safety section

## References

- [Main README](../README.md)
- [Quick Start Guide](../README.md#quick-start)
- [Docker Compose Example](../examples/multi-node/docker-compose.yml)
- [Kubernetes Examples](../examples/multi-node/kubernetes/)
- [API Reference](../ref/03-api-reference.md)
- [Configuration Schema](../schemas/run_config.schema.json)
