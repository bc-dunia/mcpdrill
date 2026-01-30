# MCP Drill Grafana Dashboards

This directory contains Grafana dashboard JSON files for visualizing MCP Drill metrics from Prometheus.

## Dashboards

### 1. Run Overview (`run-overview.json`)
High-level view of run execution and lifecycle.

**Panels:**
- Active Runs (gauge)
- Run State Distribution (pie chart)
- Total Runs Created (stat)
- Success Rate (stat)
- Run Duration Over Time (time series)
- Run State Timeline (time series)
- Run Creation Rate (time series)
- Run Completion Rate (time series)

**Template Variables:**
- `$scenario_id` - Filter by scenario

### 2. Worker Health (`worker-health.json`)
Worker pool health and resource utilization.

**Panels:**
- Total Workers (gauge)
- Average CPU Usage (stat)
- Average Memory Usage (stat)
- Saturated Workers (stat)
- CPU Usage by Worker (time series with alert)
- Memory Usage by Worker (time series)
- Active VUs by Worker (stacked time series)
- VU Utilization Rate (time series)
- Worker Saturation Rate (time series)

**Template Variables:**
- `$worker_id` - Filter by worker (multi-select, includes "All")

**Alerts:**
- Worker CPU High: Triggers when CPU > 90% for 5 minutes

### 3. Operation Performance (`operation-performance.json`)
MCP operation throughput, latency, and error rates.

**Panels:**
- Operation Throughput (time series)
- Total Operations (stat)
- Current Throughput (stat)
- Error Rate (stat)
- Total Errors (stat)
- Latency Percentiles (p50/p95/p99, time series)
- Average Latency by Operation (time series)
- Error Rate by Operation (time series with alert)
- Error Rate by Tool (time series)
- Operations by Tool (stacked time series)
- Latency Heatmap

**Template Variables:**
- `$operation` - Filter by operation type (multi-select, includes "All")
- `$tool_name` - Filter by tool name (multi-select, includes "All")

**Alerts:**
- Operation Error Rate High: Triggers when error rate > 5% for 5 minutes

### 4. System Health (`system-health.json`)
System-level metrics and stage progression.

**Panels:**
- System Overview (text/markdown)
- Stage Duration (time series)
- Stage VUs (time series)
- Control Plane CPU (placeholder)
- Control Plane Memory (placeholder)
- Goroutines (placeholder)
- Event Emission Rate (placeholder)
- Telemetry Ingestion Rate (placeholder)
- API Request Latency (placeholder)
- API Request Rate (placeholder)

**Template Variables:**
- `$run_id` - Filter by run ID
- `$stage_id` - Filter by stage (multi-select, includes "All")

**Note:** Many panels in this dashboard are placeholders for future instrumentation. Currently available metrics are stage duration and VU allocation.

## Prerequisites

1. **Prometheus** - Running and scraping MCP Drill `/metrics` endpoint
2. **Grafana** - Version 8.0 or higher
3. **MCP Drill Server** - Running with metrics endpoint exposed

## Setup Instructions

### Step 1: Configure Prometheus

Add MCP Drill server to your Prometheus scrape configuration:

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'mcpdrill'
    scrape_interval: 10s
    static_configs:
      - targets: ['localhost:8080']  # Adjust to your server address
```

Reload Prometheus configuration:
```bash
# Send SIGHUP to Prometheus process
kill -HUP <prometheus-pid>

# Or restart Prometheus
systemctl restart prometheus
```

Verify scraping:
```bash
# Check Prometheus targets page
open http://localhost:9090/targets

# Query a metric
curl 'http://localhost:9090/api/v1/query?query=mcpdrill_workers_total'
```

### Step 2: Add Prometheus Data Source to Grafana

1. Open Grafana (default: http://localhost:3000)
2. Navigate to **Configuration** → **Data Sources**
3. Click **Add data source**
4. Select **Prometheus**
5. Configure:
   - **Name:** `Prometheus` (must match dashboard data source references)
   - **URL:** `http://localhost:9090` (adjust to your Prometheus address)
   - **Access:** `Server` (default)
6. Click **Save & Test**

### Step 3: Import Dashboards

#### Option A: Import via UI (Recommended)

1. Navigate to **Dashboards** → **Import**
2. Click **Upload JSON file**
3. Select one of the dashboard JSON files:
   - `run-overview.json`
   - `worker-health.json`
   - `operation-performance.json`
   - `system-health.json`
4. Configure:
   - **Name:** (auto-filled from JSON)
   - **Folder:** Select or create a folder (e.g., "MCP Drill")
   - **UID:** (auto-filled, must be unique)
5. Click **Import**
6. Repeat for all 4 dashboards

#### Option B: Import via API

```bash
# Set Grafana credentials
GRAFANA_URL="http://localhost:3000"
GRAFANA_USER="admin"
GRAFANA_PASS="admin"

# Import all dashboards
for dashboard in run-overview worker-health operation-performance system-health; do
  curl -X POST \
    -H "Content-Type: application/json" \
    -u "$GRAFANA_USER:$GRAFANA_PASS" \
    -d @"${dashboard}.json" \
    "$GRAFANA_URL/api/dashboards/db"
done
```

#### Option C: Provision Dashboards (Persistent)

For production deployments, use Grafana provisioning:

1. Create provisioning directory:
```bash
mkdir -p /etc/grafana/provisioning/dashboards
```

2. Create dashboard provider config:
```yaml
# /etc/grafana/provisioning/dashboards/mcpdrill.yaml
apiVersion: 1

providers:
  - name: 'MCP Drill'
    orgId: 1
    folder: 'MCP Drill'
    type: file
    disableDeletion: false
    updateIntervalSeconds: 10
    allowUiUpdates: true
    options:
      path: /var/lib/grafana/dashboards/mcpdrill
```

3. Copy dashboard files:
```bash
mkdir -p /var/lib/grafana/dashboards/mcpdrill
cp *.json /var/lib/grafana/dashboards/mcpdrill/
```

4. Restart Grafana:
```bash
systemctl restart grafana-server
```

### Step 4: Verify Dashboards

1. Navigate to **Dashboards** → **Browse**
2. Open each dashboard:
   - MCP Drill - Run Overview
   - MCP Drill - Worker Health
   - MCP Drill - Operation Performance
   - MCP Drill - System Health
3. Verify:
   - Template variables populate correctly
   - Panels display data (if MCP Drill is running)
   - No "No data" errors (except for placeholder panels)

## Usage

### Monitoring a Load Test

1. **Start MCP Drill components:**
   ```bash
   # Terminal 1: Start server
   ./server --addr :8080
   
   # Terminal 2: Start worker(s)
   ./worker --control-plane http://localhost:8080
   
   # Terminal 3: Create and start a run
   ./mcpdrill create config.json
   ./mcpdrill start <run-id>
   ```

2. **Open dashboards:**
   - **Run Overview**: Monitor run lifecycle and success rate
   - **Worker Health**: Watch worker CPU/memory and VU allocation
   - **Operation Performance**: Track operation latency and error rates
   - **System Health**: View stage progression and system metrics

3. **Use template variables:**
   - Select specific scenarios, workers, operations, or tools
   - Use "All" to see aggregate metrics
   - Combine filters for detailed analysis

4. **Set time range:**
   - Use Grafana time picker (top-right)
   - Common ranges: Last 15m, Last 1h, Last 6h
   - Set custom range for historical analysis

5. **Enable auto-refresh:**
   - Click refresh dropdown (top-right)
   - Select interval: 5s, 10s, 30s, 1m
   - Dashboards will update automatically

### Alert Configuration

Two dashboards include pre-configured alerts:

1. **Worker Health → Worker CPU High**
   - Condition: CPU > 90% for 5 minutes
   - Action: Configure notification channels in Grafana

2. **Operation Performance → Operation Error Rate High**
   - Condition: Error rate > 5% for 5 minutes
   - Action: Configure notification channels in Grafana

To configure notifications:
1. Navigate to **Alerting** → **Notification channels**
2. Add channels (email, Slack, PagerDuty, etc.)
3. Edit alert rules to use your channels

## Troubleshooting

### No Data in Panels

**Symptom:** Panels show "No data" or empty graphs

**Possible Causes:**
1. Prometheus not scraping MCP Drill
   - Check Prometheus targets: http://localhost:9090/targets
   - Verify MCP Drill `/metrics` endpoint: `curl http://localhost:8080/metrics`
   - Check Prometheus logs for scrape errors

2. Grafana data source misconfigured
   - Test data source: Configuration → Data Sources → Prometheus → Save & Test
   - Verify Prometheus URL is correct
   - Check network connectivity between Grafana and Prometheus

3. No active runs or workers
   - Start MCP Drill server and workers
   - Create and start a run
   - Wait 10-30 seconds for metrics to appear

4. Time range too narrow
   - Expand time range in Grafana time picker
   - Check "Last 1 hour" or "Last 6 hours"

### Template Variables Not Populating

**Symptom:** Template variable dropdowns are empty

**Possible Causes:**
1. No metrics with required labels
   - Run a load test to generate metrics
   - Check Prometheus for label values: `curl 'http://localhost:9090/api/v1/label/scenario_id/values'`

2. Data source name mismatch
   - Ensure Prometheus data source is named "Prometheus"
   - Or edit dashboard JSON to match your data source name

### Incorrect Metric Values

**Symptom:** Metrics show unexpected values or spikes

**Possible Causes:**
1. Prometheus scrape interval too long
   - Reduce scrape interval in prometheus.yml (recommended: 10s)
   - Reload Prometheus configuration

2. Grafana refresh interval too long
   - Reduce dashboard refresh interval (top-right dropdown)
   - Recommended: 10s for active monitoring

3. PromQL query issues
   - Check panel query in Edit mode
   - Test query in Prometheus UI: http://localhost:9090/graph
   - Verify label filters match your data

### Placeholder Panels

**Symptom:** Some panels in System Health dashboard show placeholder text

**Expected Behavior:** These metrics are not yet instrumented in MCP Drill:
- Control plane CPU/memory
- Goroutine count
- Event emission rate
- Telemetry ingestion rate
- API request latency/rate

**Future Work:** These will be added in Phase 2 M2.3 Task 3 (future iteration).

## Customization

### Modifying Dashboards

1. Open dashboard in Grafana
2. Click **Dashboard settings** (gear icon, top-right)
3. Edit panels:
   - Click panel title → **Edit**
   - Modify query, visualization, thresholds, etc.
   - Click **Apply**
4. Save dashboard:
   - Click **Save dashboard** (disk icon, top-right)
   - Add change description
   - Click **Save**

### Exporting Modified Dashboards

1. Open dashboard
2. Click **Dashboard settings** → **JSON Model**
3. Copy JSON
4. Save to file: `<dashboard-name>.json`
5. Commit to version control

### Adding New Panels

1. Click **Add panel** (top-right)
2. Select visualization type
3. Configure query:
   - Data source: Prometheus
   - Metric: Select from available metrics
   - Labels: Add filters
4. Configure visualization options
5. Click **Apply**

## Metrics Reference

See Task 1 implementation for full metrics documentation:
- `.sisyphus/notepads/phase2-m2.3-dashboards/learnings.md`

### Available Metrics

**Run Metrics:**
- `mcpdrill_runs_total{scenario_id}` - Total runs created
- `mcpdrill_run_duration_seconds{scenario_id}` - Run duration histogram
- `mcpdrill_run_state{scenario_id,state}` - Run state gauge

**Worker Metrics:**
- `mcpdrill_workers_total` - Total registered workers
- `mcpdrill_worker_health_cpu_percent{worker_id}` - Worker CPU %
- `mcpdrill_worker_health_memory_mb{worker_id}` - Worker memory MB
- `mcpdrill_worker_health_active_vus{worker_id}` - Active VUs

**Operation Metrics:**
- `mcpdrill_operations_total{operation,tool_name}` - Total operations
- `mcpdrill_operation_duration_seconds{operation,tool_name}` - Operation latency histogram
- `mcpdrill_operation_errors_total{operation,tool_name}` - Total errors

**Stage Metrics:**
- `mcpdrill_stage_duration_seconds{run_id,stage_id}` - Stage duration
- `mcpdrill_stage_vus{run_id,stage_id}` - Stage VU allocation

## Support

For issues or questions:
1. Check MCP Drill logs: `./server --log-level debug`
2. Check Prometheus scrape status: http://localhost:9090/targets
3. Check Grafana logs: `journalctl -u grafana-server -f`
4. Review implementation notes: `.sisyphus/notepads/phase2-m2.3-dashboards/learnings.md`

## Version

- **Dashboard Version:** 1.0
- **Grafana Compatibility:** 8.0+
- **Prometheus Compatibility:** 2.0+
- **MCP Drill Phase:** Phase 2 M2.3
