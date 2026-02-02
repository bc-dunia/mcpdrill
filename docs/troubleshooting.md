# Troubleshooting

Common issues and solutions for MCP Drill.

## Worker Issues

### Worker Not Connecting

```bash
# Check control plane is running
curl http://localhost:8080/healthz

# Verify worker can reach control plane
./mcpdrill-worker --control-plane http://localhost:8080 --debug
```

### Worker Disconnecting

- Check network stability
- Verify heartbeat timeout isn't too aggressive
- Check control plane logs for errors

## Test Issues

### High Error Rate

- Check target server is running
- Verify URL in config is correct
- Check network connectivity
- Review stop conditions thresholds
- Check target server logs for errors

### Tests Not Progressing

- Ensure at least one worker is connected
- Check run status for errors: `./mcpdrill status <run_id> --json`
- Verify target server is responding

### Slow Performance

- Start with fewer VUs and scale up
- Check target server resource usage
- Use `session_policy.mode: "reuse"` for efficiency
- Monitor worker CPU (if >80%, add more workers)

## Configuration Issues

### Validation Errors

```bash
# Validate config before creating run
./mcpdrill validate config.json
```

Common validation errors:
- Missing required fields (`scenario_id`, `target.url`)
- Invalid stage IDs (must be `stg_` + hex)
- Invalid session mode
- Negative duration values

### SSRF Protection

If URL validation fails:
- Check `environment.allowlist` settings
- Ensure target is in allowed hosts
- Use explicit IP addresses if needed

## Data Truncation

### "Data Truncated" Warning in UI

This warning appears when the test exceeds storage limits. By default, the server stores up to 20 million operations per run.

**Symptoms:**
- Yellow banner: "Data truncated: Some operations exceeded storage limits"
- Metrics may be incomplete for long-running tests

**Solutions:**

1. **Increase limits** when starting the server:
   ```bash
   # Extended capacity (20M, ~5.5 hours at 1000 ops/sec)
   ./mcpdrill-server --max-ops-per-run 20000000 --max-logs-per-run 20000000
   
   # High capacity (50M, requires 16+ GB RAM)
   ./mcpdrill-server --max-ops-per-run 50000000 --max-logs-per-run 50000000
   
   # Enterprise (100M, requires 50+ GB RAM)
   ./mcpdrill-server --max-ops-per-run 100000000 --max-logs-per-run 100000000
   ```

2. **Run shorter stages** - break very long soak tests into multiple shorter runs

**Memory usage estimate:**

| Limit | RAM Required | @ 1K ops/sec | @ 10K ops/sec |
|-------|--------------|--------------|---------------|
| 20M (default) | 6-10 GB | ~5.5 hours | ~33 min |
| 50M (recommended max) | 15-25 GB | ~14 hours | ~1.4 hours |
| 100M (enterprise) | 30-50 GB | ~28 hours | ~2.8 hours |

> **Note**: Throughput varies significantly by VU count and tool complexity. Check your actual ops/sec in the metrics dashboard.

## Web UI Issues

### UI Not Loading

- Ensure web UI is built: `cd web/log-explorer && npm run build`
- Check that `./web/log-explorer/dist/` exists
- Verify server is running on correct port

### Real-time Updates Not Working

- Check SSE connection in browser DevTools
- Verify no proxy is blocking SSE
- Try refreshing the page

## Agent Issues

### Agent Not Connecting

```bash
# Verify Control Plane is reachable
curl https://your-control-plane:8080/healthz

# Check agent logs
./mcpdrill-agent --control-plane-url ... 2>&1 | grep -i error
```

### Metrics Not Appearing

- Verify `pair_key` matches exactly between agent and run config
- Check agent is registered: `curl http://localhost:8080/agents`
- Ensure `server_telemetry.enabled: true` in run config

### TLS Errors

```bash
# Use custom CA
--tls-ca-file /path/to/ca.pem

# Dev only: skip verification
--tls-insecure-skip-verify
```

## Getting Help

- Check logs with `--debug` flag
- Review [GitHub Issues](https://github.com/bc-dunia/mcpdrill/issues)
- Open a new issue with:
  - MCP Drill version
  - Configuration (redacted)
  - Error messages
  - Steps to reproduce
