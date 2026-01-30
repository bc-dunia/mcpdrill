import { useState, useEffect, useCallback, useMemo, memo, useRef } from 'react';
import {
  ResponsiveContainer,
  PieChart,
  Pie,
  Cell,
  BarChart,
  Bar,
  LineChart,
  Line,
  ScatterChart,
  Scatter,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
} from 'recharts';
import type { ToolMetrics, OperationLog, ToolVolumeDataPoint } from '../types';
import { Icon } from './Icon';
import { exportChartAsPng } from '../utils/chartExport';

interface ToolMetricsDashboardProps {
  runId: string;
  onToolClick?: (toolName: string) => void;
}

interface AggregatedMetricsResponse {
  run_id: string;
  by_tool?: Record<string, ToolMetrics>;
  timestamp?: number;
}

const API_BASE = '';

const CHART_COLORS = [
  '#4ade80', '#22d3ee', '#a78bfa', '#fbbf24', '#f87171',
  '#34d399', '#60a5fa', '#c084fc', '#fb923c', '#fb7185',
  '#2dd4bf', '#818cf8', '#e879f9', '#facc15', '#f43f5e',
];

function getToolColor(index: number): string {
  return CHART_COLORS[index % CHART_COLORS.length];
}

function formatLatency(ms: number): string {
  if (ms < 1000) return `${ms.toFixed(0)}ms`;
  return `${(ms / 1000).toFixed(2)}s`;
}

const PieTooltip = ({ active, payload }: { active?: boolean; payload?: Array<{ name: string; value: number; payload: { successRate: number; toolName: string } }> }) => {
  if (!active || !payload?.length) return null;
  const data = payload[0].payload;
  return (
    <div className="metrics-tooltip">
      <div className="tooltip-title">{data.toolName}</div>
      <div className="tooltip-row">
        <span>Success Rate</span>
        <span className="tooltip-value">{data.successRate.toFixed(1)}%</span>
      </div>
      <div className="tooltip-row">
        <span>Total Calls</span>
        <span className="tooltip-value">{payload[0].value}</span>
      </div>
    </div>
  );
};

const BarTooltip = ({ active, payload, label }: { active?: boolean; payload?: Array<{ name: string; value: number; color: string }>; label?: string }) => {
  if (!active || !payload?.length) return null;
  return (
    <div className="metrics-tooltip">
      <div className="tooltip-title">{label}</div>
      <div className="tooltip-values">
        {payload.map((entry, index) => (
          <div key={index} className="tooltip-row">
            <span className="tooltip-dot" style={{ background: entry.color }} />
            <span>{entry.name}</span>
            <span className="tooltip-value">{formatLatency(entry.value)}</span>
          </div>
        ))}
      </div>
    </div>
  );
};

const LineTooltip = ({ active, payload, label }: { active?: boolean; payload?: Array<{ name: string; value: number; color: string }>; label?: string }) => {
  if (!active || !payload?.length) return null;
  return (
    <div className="metrics-tooltip">
      <div className="tooltip-title">{label}</div>
      <div className="tooltip-values">
        {payload.filter(p => p.value > 0).map((entry, index) => (
          <div key={index} className="tooltip-row">
            <span className="tooltip-dot" style={{ background: entry.color }} />
            <span>{entry.name}</span>
            <span className="tooltip-value">{entry.value} calls</span>
          </div>
        ))}
      </div>
    </div>
  );
};

const ScatterTooltip = ({ active, payload }: { active?: boolean; payload?: Array<{ payload: { tool: string; size: number; latency: number } }> }) => {
  if (!active || !payload?.length) return null;
  const data = payload[0].payload;
  return (
    <div className="metrics-tooltip">
      <div className="tooltip-title">{data.tool}</div>
      <div className="tooltip-row">
        <span>Argument Size</span>
        <span className="tooltip-value">{data.size} bytes</span>
      </div>
      <div className="tooltip-row">
        <span>Latency</span>
        <span className="tooltip-value">{formatLatency(data.latency)}</span>
      </div>
    </div>
  );
};

function ToolMetricsDashboardComponent({ runId, onToolClick }: ToolMetricsDashboardProps) {
  const [toolMetrics, setToolMetrics] = useState<Record<string, ToolMetrics>>({});
  const [logs, setLogs] = useState<OperationLog[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  
  const pieChartRef = useRef<HTMLDivElement>(null);
  const barChartRef = useRef<HTMLDivElement>(null);
  const lineChartRef = useRef<HTMLDivElement>(null);
  const scatterChartRef = useRef<HTMLDivElement>(null);

  const fetchData = useCallback(async () => {
    if (!runId) return;

    setLoading(true);
    setError(null);

    try {
      const [metricsRes, logsRes] = await Promise.all([
        fetch(`${API_BASE}/runs/${runId}/metrics`),
        fetch(`${API_BASE}/runs/${runId}/logs?limit=1000&operation=tools/call`),
      ]);

      if (!metricsRes.ok) {
        throw new Error(`Failed to fetch metrics: ${metricsRes.statusText}`);
      }

      const metricsData: AggregatedMetricsResponse = await metricsRes.json();
      setToolMetrics(metricsData.by_tool || {});

      if (logsRes.ok) {
        const logsData = await logsRes.json();
        setLogs(logsData.logs || []);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch data');
    } finally {
      setLoading(false);
    }
  }, [runId]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  const pieData = useMemo(() => {
    return Object.entries(toolMetrics).map(([name, metrics]) => ({
      toolName: name,
      value: metrics.total_ops,
      successRate: metrics.total_ops > 0 
        ? (metrics.success_ops / metrics.total_ops) * 100 
        : 100,
    }));
  }, [toolMetrics]);

  const barData = useMemo(() => {
    return Object.entries(toolMetrics).map(([name, metrics]) => ({
      name: name.length > 12 ? name.slice(0, 12) + '...' : name,
      fullName: name,
      p50: metrics.latency_p50,
      p95: metrics.latency_p95,
      p99: metrics.latency_p99,
    })).sort((a, b) => b.p50 - a.p50).slice(0, 10);
  }, [toolMetrics]);

  const lineData = useMemo<ToolVolumeDataPoint[]>(() => {
    if (logs.length === 0) return [];

    const toolNames = [...new Set(logs.map(l => l.tool_name).filter(Boolean))];
    const bucketSize = 60000;
    const buckets = new Map<number, Record<string, number>>();

    for (const log of logs) {
      const bucket = Math.floor(log.timestamp_ms / bucketSize) * bucketSize;
      if (!buckets.has(bucket)) {
        const entry: Record<string, number> = {};
        toolNames.forEach(t => entry[t] = 0);
        buckets.set(bucket, entry);
      }
      if (log.tool_name) {
        buckets.get(bucket)![log.tool_name]++;
      }
    }

    return Array.from(buckets.entries())
      .sort((a, b) => a[0] - b[0])
      .map(([timestamp, counts]) => ({
        timestamp,
        time: new Date(timestamp).toLocaleTimeString('en-US', {
          hour: '2-digit',
          minute: '2-digit',
          hour12: false,
        }),
        ...counts,
      }));
  }, [logs]);

  const toolNamesForLine = useMemo(() => {
    if (lineData.length === 0) return [];
    const firstPoint = lineData[0];
    return Object.keys(firstPoint).filter(k => k !== 'timestamp' && k !== 'time');
  }, [lineData]);

  const scatterData = useMemo(() => {
    const results: Array<{ tool: string; size: number; latency: number; color: string }> = [];
    const toolColorMap = new Map<string, string>();
    let colorIndex = 0;

    for (const log of logs) {
      if (!log.tool_name) continue;
      
      if (!toolColorMap.has(log.tool_name)) {
        toolColorMap.set(log.tool_name, getToolColor(colorIndex++));
      }

      results.push({
        tool: log.tool_name,
        size: 0,
        latency: log.latency_ms,
        color: toolColorMap.get(log.tool_name)!,
      });
    }

    return results.slice(0, 200);
  }, [logs]);

  const toolsForScatter = useMemo(() => {
    const unique = new Map<string, string>();
    scatterData.forEach(d => unique.set(d.tool, d.color));
    return Array.from(unique.entries());
  }, [scatterData]);

  if (loading) {
    return (
      <div className="tool-metrics-dashboard" role="region" aria-label="Tool Metrics Dashboard">
        <div className="dashboard-loading" role="status" aria-live="polite">
          <div className="spinner" aria-hidden={true} />
          <span>Loading tool metrics...</span>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="tool-metrics-dashboard" role="region" aria-label="Tool Metrics Dashboard">
        <div className="dashboard-error" role="alert">
          <Icon name="alert-triangle" size="lg" aria-hidden={true} />
          <h3>Failed to Load Metrics</h3>
          <p>{error}</p>
          <button type="button" onClick={fetchData} className="btn btn-secondary">
            <Icon name="refresh" size="sm" aria-hidden={true} />
            Retry
          </button>
        </div>
      </div>
    );
  }

  if (Object.keys(toolMetrics).length === 0) {
    return (
      <div className="tool-metrics-dashboard" role="region" aria-label="Tool Metrics Dashboard">
        <div className="dashboard-empty">
          <Icon name="chart-bar" size="xl" aria-hidden={true} />
          <h3>No Tool Metrics Available</h3>
          <p className="muted">Run a test with tool operations to see metrics</p>
        </div>
      </div>
    );
  }

  return (
    <div className="tool-metrics-dashboard" role="region" aria-label="Tool Metrics Dashboard">
      <div className="dashboard-header">
        <div className="dashboard-title">
          <Icon name="chart-bar" size="md" aria-hidden={true} />
          <h2>Tool Performance Metrics</h2>
          <span className="tool-count">{Object.keys(toolMetrics).length} tools</span>
        </div>
        <button
          type="button"
          onClick={fetchData}
          disabled={loading}
          className="btn btn-secondary btn-sm"
          aria-label="Refresh metrics"
        >
          <Icon name={loading ? 'loader' : 'refresh'} size="sm" aria-hidden={true} />
          Refresh
        </button>
      </div>

      <div className="tool-charts-grid">
        <div 
          className="chart-card chart-pie" 
          ref={pieChartRef}
          role="region" 
          aria-labelledby="pie-chart-title"
        >
          <div className="chart-header">
            <h3 id="pie-chart-title">Success Rate by Tool</h3>
            <button
              type="button"
              onClick={() => exportChartAsPng(pieChartRef.current, `${runId}-success-rate`)}
              className="btn btn-ghost btn-sm"
              aria-label="Export chart as PNG"
            >
              <Icon name="download" size="sm" aria-hidden={true} />
            </button>
          </div>
          <div className="chart-body">
            <ResponsiveContainer width="100%" height={280}>
              <PieChart>
                <Pie
                  data={pieData}
                  dataKey="value"
                  nameKey="toolName"
                  cx="50%"
                  cy="50%"
                  innerRadius={60}
                  outerRadius={100}
                  paddingAngle={2}
                  onClick={(data) => onToolClick?.(data.toolName)}
                  style={{ cursor: onToolClick ? 'pointer' : 'default' }}
                >
                  {pieData.map((entry, index) => (
                    <Cell 
                      key={entry.toolName}
                      fill={getToolColor(index)}
                      stroke="var(--bg-primary)"
                      strokeWidth={2}
                    />
                  ))}
                </Pie>
                <Tooltip content={<PieTooltip />} />
                <Legend 
                  layout="vertical" 
                  align="right" 
                  verticalAlign="middle"
                  wrapperStyle={{ fontSize: '11px' }}
                />
              </PieChart>
            </ResponsiveContainer>
          </div>
        </div>

        <div 
          className="chart-card chart-bar" 
          ref={barChartRef}
          role="region" 
          aria-labelledby="bar-chart-title"
        >
          <div className="chart-header">
            <h3 id="bar-chart-title">Latency Comparison</h3>
            <button
              type="button"
              onClick={() => exportChartAsPng(barChartRef.current, `${runId}-latency-comparison`)}
              className="btn btn-ghost btn-sm"
              aria-label="Export chart as PNG"
            >
              <Icon name="download" size="sm" aria-hidden={true} />
            </button>
          </div>
          <div className="chart-body">
            <ResponsiveContainer width="100%" height={280}>
              <BarChart data={barData} margin={{ top: 8, right: 16, left: 0, bottom: 40 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--border-subtle)" />
                <XAxis 
                  dataKey="name" 
                  stroke="var(--text-muted)" 
                  fontSize={10}
                  angle={-45}
                  textAnchor="end"
                  height={60}
                />
                <YAxis 
                  stroke="var(--text-muted)" 
                  fontSize={11}
                  tickFormatter={(v) => `${v}ms`}
                />
                <Tooltip content={<BarTooltip />} />
                <Legend wrapperStyle={{ fontSize: '11px', paddingTop: '16px' }} />
                <Bar dataKey="p50" name="P50" fill="#4ade80" radius={[2, 2, 0, 0]} />
                <Bar dataKey="p95" name="P95" fill="#fbbf24" radius={[2, 2, 0, 0]} />
                <Bar dataKey="p99" name="P99" fill="#f87171" radius={[2, 2, 0, 0]} />
              </BarChart>
            </ResponsiveContainer>
          </div>
        </div>

        <div 
          className="chart-card chart-line" 
          ref={lineChartRef}
          role="region" 
          aria-labelledby="line-chart-title"
        >
          <div className="chart-header">
            <h3 id="line-chart-title">Call Volume Over Time</h3>
            <button
              type="button"
              onClick={() => exportChartAsPng(lineChartRef.current, `${runId}-call-volume`)}
              className="btn btn-ghost btn-sm"
              aria-label="Export chart as PNG"
            >
              <Icon name="download" size="sm" aria-hidden={true} />
            </button>
          </div>
          <div className="chart-body">
            {lineData.length > 0 ? (
              <ResponsiveContainer width="100%" height={280}>
                <LineChart data={lineData} margin={{ top: 8, right: 16, left: 0, bottom: 0 }}>
                  <CartesianGrid strokeDasharray="3 3" stroke="var(--border-subtle)" />
                  <XAxis 
                    dataKey="time" 
                    stroke="var(--text-muted)" 
                    fontSize={11}
                  />
                  <YAxis 
                    stroke="var(--text-muted)" 
                    fontSize={11}
                  />
                  <Tooltip content={<LineTooltip />} />
                  <Legend wrapperStyle={{ fontSize: '11px' }} />
                  {toolNamesForLine.map((toolName, index) => (
                    <Line
                      key={toolName}
                      type="monotone"
                      dataKey={toolName}
                      name={toolName.length > 15 ? toolName.slice(0, 15) + '...' : toolName}
                      stroke={getToolColor(index)}
                      strokeWidth={2}
                      dot={false}
                      activeDot={{ r: 4 }}
                    />
                  ))}
                </LineChart>
              </ResponsiveContainer>
            ) : (
              <div className="chart-empty">
                <Icon name="trending-up" size="lg" aria-hidden={true} />
                <p>No time series data available</p>
              </div>
            )}
          </div>
        </div>

        <div 
          className="chart-card chart-scatter" 
          ref={scatterChartRef}
          role="region" 
          aria-labelledby="scatter-chart-title"
        >
          <div className="chart-header">
            <h3 id="scatter-chart-title">Argument Size vs Latency (data unavailable)</h3>
            <button
              type="button"
              onClick={() => exportChartAsPng(scatterChartRef.current, `${runId}-size-latency`)}
              className="btn btn-ghost btn-sm"
              aria-label="Export chart as PNG"
            >
              <Icon name="download" size="sm" aria-hidden={true} />
            </button>
          </div>
          <div className="chart-body">
            {scatterData.length > 0 ? (
              <ResponsiveContainer width="100%" height={280}>
                <ScatterChart margin={{ top: 8, right: 16, left: 0, bottom: 0 }}>
                  <CartesianGrid strokeDasharray="3 3" stroke="var(--border-subtle)" />
                  <XAxis 
                    dataKey="size" 
                    name="Size" 
                    unit=" bytes"
                    stroke="var(--text-muted)" 
                    fontSize={11}
                  />
                  <YAxis 
                    dataKey="latency" 
                    name="Latency" 
                    unit="ms"
                    stroke="var(--text-muted)" 
                    fontSize={11}
                  />
                  <Tooltip content={<ScatterTooltip />} />
                  <Legend wrapperStyle={{ fontSize: '11px' }} />
                  {toolsForScatter.map(([toolName, color]) => (
                    <Scatter
                      key={toolName}
                      name={toolName.length > 15 ? toolName.slice(0, 15) + '...' : toolName}
                      data={scatterData.filter(d => d.tool === toolName)}
                      fill={color}
                    />
                  ))}
                </ScatterChart>
              </ResponsiveContainer>
            ) : (
              <div className="chart-empty">
                <Icon name="scatter" size="lg" aria-hidden={true} />
                <p>No data available for scatter plot</p>
              </div>
            )}
          </div>
        </div>
      </div>

      <div className="tool-metrics-table" role="region" aria-label="Tool metrics table">
        <h3>Detailed Metrics</h3>
        <div className="table-scroll">
          <table className="metrics-table">
            <caption className="sr-only">
              Detailed performance metrics for each tool
            </caption>
            <thead>
              <tr>
                <th scope="col">Tool</th>
                <th scope="col">Total Calls</th>
                <th scope="col">Success</th>
                <th scope="col">Errors</th>
                <th scope="col">Success Rate</th>
                <th scope="col">P50</th>
                <th scope="col">P95</th>
                <th scope="col">P99</th>
              </tr>
            </thead>
            <tbody>
              {Object.entries(toolMetrics)
                .sort((a, b) => b[1].total_ops - a[1].total_ops)
                .map(([name, metrics]) => {
                  const successRate = metrics.total_ops > 0 
                    ? (metrics.success_ops / metrics.total_ops) * 100 
                    : 100;
                  return (
                    <tr 
                      key={name}
                      onClick={() => onToolClick?.(name)}
                      className={onToolClick ? 'clickable' : ''}
                      tabIndex={onToolClick ? 0 : undefined}
                      onKeyDown={(e) => {
                        if (onToolClick && (e.key === 'Enter' || e.key === ' ')) {
                          e.preventDefault();
                          onToolClick(name);
                        }
                      }}
                    >
                      <td className="cell-tool">{name}</td>
                      <td>{metrics.total_ops.toLocaleString()}</td>
                      <td className="cell-success">{metrics.success_ops.toLocaleString()}</td>
                      <td className="cell-error">{metrics.failure_ops.toLocaleString()}</td>
                      <td>
                        <span className={`rate-badge ${successRate >= 99 ? 'rate-good' : successRate >= 95 ? 'rate-warn' : 'rate-bad'}`}>
                          {successRate.toFixed(1)}%
                        </span>
                      </td>
                      <td>{formatLatency(metrics.latency_p50)}</td>
                      <td>{formatLatency(metrics.latency_p95)}</td>
                      <td>{formatLatency(metrics.latency_p99)}</td>
                    </tr>
                  );
                })}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}

export const ToolMetricsDashboard = memo(ToolMetricsDashboardComponent);
