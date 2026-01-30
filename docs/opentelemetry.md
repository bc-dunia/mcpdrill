# OpenTelemetry Integration

MCP Drill supports distributed tracing via OpenTelemetry.

## Enabling Tracing

```bash
# Export to OTLP collector (gRPC)
./mcpdrill-worker \
  --otel-enabled \
  --otel-exporter otlp-grpc \
  --otel-endpoint localhost:4317

# Export to OTLP collector (HTTP)
./mcpdrill-worker \
  --otel-enabled \
  --otel-exporter otlp-http \
  --otel-endpoint localhost:4318

# Export to stdout (debugging)
./mcpdrill-worker --otel-enabled --otel-exporter stdout
```

## Configuration Flags

| Flag | Description |
|------|-------------|
| `--otel-enabled` | Enable OpenTelemetry |
| `--otel-exporter` | Exporter type: `otlp-grpc`, `otlp-http`, `stdout` |
| `--otel-endpoint` | Collector endpoint |

## Span Attributes

Each span includes:

| Attribute | Description |
|-----------|-------------|
| `mcpdrill.run_id` | Run identifier |
| `mcpdrill.stage_id` | Current stage |
| `mcpdrill.worker_id` | Worker identifier |
| `mcpdrill.vu_id` | Virtual user identifier |
| `mcpdrill.operation` | Operation type |
| `mcpdrill.tool_name` | Tool name (for tools/call) |

## Example: Jaeger Setup

1. Start Jaeger:
```bash
docker run -d --name jaeger \
  -p 16686:16686 \
  -p 4317:4317 \
  jaegertracing/all-in-one:latest
```

2. Start worker with tracing:
```bash
./mcpdrill-worker \
  --control-plane http://localhost:8080 \
  --otel-enabled \
  --otel-exporter otlp-grpc \
  --otel-endpoint localhost:4317
```

3. View traces at `http://localhost:16686`

## Example: Grafana Tempo

1. Configure Tempo in `tempo.yaml`
2. Start worker pointing to Tempo endpoint
3. Query traces in Grafana
