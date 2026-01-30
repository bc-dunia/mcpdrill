import { useRef, useCallback, memo } from 'react';
import {
  ResponsiveContainer,
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
} from 'recharts';
import type { MetricsDataPoint } from '../types';
import { Icon } from './Icon';
import { exportChartAsPng } from '../utils/chartExport';

interface ThroughputChartProps {
  data: MetricsDataPoint[];
  loading?: boolean;
}

function generateDataSummary(data: MetricsDataPoint[]): string {
  if (data.length === 0) return 'No data available';
  const latest = data[data.length - 1];
  const totalOps = latest.success_ops + latest.failed_ops;
  return `Latest: ${totalOps.toFixed(0)} ops/sec (${latest.success_ops.toFixed(0)} success, ${latest.failed_ops.toFixed(0)} failed). ${data.length} data points.`;
}

const throughputColors = {
  success: '#4ade80',
  failed: '#f87171',
  total: '#22d3ee',
};

const CustomTooltip = ({ active, payload, label }: { active?: boolean; payload?: Array<{ name: string; value: number; color: string }>; label?: string }) => {
  if (!active || !payload?.length) return null;

  const total = payload.reduce((sum, entry) => sum + (entry.value || 0), 0);

  return (
    <div className="metrics-tooltip" role="tooltip" aria-live="polite">
      <div className="tooltip-time">{label}</div>
      <div className="tooltip-values">
        {payload.map((entry, index) => (
          <div key={index} className="tooltip-row">
            <span className="tooltip-dot" style={{ background: entry.color }} aria-hidden="true" />
            <span className="tooltip-label">{entry.name}</span>
            <span className="tooltip-value">{entry.value.toFixed(0)} ops</span>
          </div>
        ))}
        <div className="tooltip-row tooltip-total">
          <span className="tooltip-label">Total</span>
          <span className="tooltip-value">{total.toFixed(0)} ops/s</span>
        </div>
      </div>
    </div>
  );
}

function ThroughputChartComponent({ data, loading }: ThroughputChartProps) {
  const chartRef = useRef<HTMLDivElement>(null);

  const exportAsPng = useCallback(() => {
    exportChartAsPng(chartRef.current, `throughput-${Date.now()}`);
  }, []);

  if (loading) {
    return (
      <div className="metrics-chart-container" role="region" aria-label="Throughput chart">
        <div className="metrics-chart-header">
          <h3>Throughput</h3>
        </div>
        <div className="metrics-chart-loading" role="status" aria-live="polite">
          <div className="spinner" aria-hidden="true" />
          <span>Loading metrics...</span>
        </div>
      </div>
    );
  }

  if (!data.length) {
    return (
      <div className="metrics-chart-container" role="region" aria-label="Throughput chart">
        <div className="metrics-chart-header">
          <h3>Throughput</h3>
        </div>
        <div className="metrics-chart-empty" role="status">
          <span className="empty-icon" aria-hidden="true"><Icon name="trending-up" size="xl" /></span>
          <span>No throughput data available</span>
        </div>
      </div>
    );
  }

  return (
    <div 
      className="metrics-chart-container" 
      ref={chartRef}
      role="region" 
      aria-labelledby="throughput-chart-title"
      aria-describedby="throughput-chart-summary"
    >
      <div className="metrics-chart-header">
        <h3 id="throughput-chart-title" title="Operations per second over time. Shows actual load being generated.">Throughput</h3>
        <div className="chart-header-actions">
          <span className="chart-unit">ops/sec</span>
          <button 
            className="btn btn-ghost btn-sm" 
            onClick={exportAsPng} 
            aria-label="Export throughput chart as PNG"
          >
            <Icon name="download" size="sm" aria-hidden={true} />
          </button>
        </div>
      </div>
      <p id="throughput-chart-summary" className="sr-only">{generateDataSummary(data)}</p>
      <div className="metrics-chart-body">
        <ResponsiveContainer width="100%" height={280}>
          <AreaChart data={data} margin={{ top: 8, right: 16, left: 0, bottom: 0 }}>
            <defs>
              <linearGradient id="successGradient" x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%" stopColor={throughputColors.success} stopOpacity={0.4}/>
                <stop offset="95%" stopColor={throughputColors.success} stopOpacity={0.05}/>
              </linearGradient>
              <linearGradient id="failedGradient" x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%" stopColor={throughputColors.failed} stopOpacity={0.4}/>
                <stop offset="95%" stopColor={throughputColors.failed} stopOpacity={0.05}/>
              </linearGradient>
            </defs>
            <CartesianGrid strokeDasharray="3 3" stroke="var(--border-subtle)" />
            <XAxis 
              dataKey="time" 
              stroke="var(--text-muted)" 
              fontSize={11}
              tickLine={false}
            />
            <YAxis 
              stroke="var(--text-muted)" 
              fontSize={11}
              tickLine={false}
              axisLine={false}
            />
            <Tooltip content={<CustomTooltip />} />
            <Legend 
              wrapperStyle={{ fontSize: '11px', paddingTop: '8px' }}
            />
            <Area 
              type="monotone" 
              dataKey="success_ops" 
              stackId="1"
              stroke={throughputColors.success}
              strokeWidth={2}
              fill="url(#successGradient)"
              dot={data.length <= 2 ? { r: 4, fill: throughputColors.success } : false}
              name="Success"
            />
            <Area 
              type="monotone" 
              dataKey="failed_ops" 
              stackId="1"
              stroke={throughputColors.failed}
              strokeWidth={2}
              fill="url(#failedGradient)"
              dot={data.length <= 2 ? { r: 4, fill: throughputColors.failed } : false}
              name="Failed"
            />
          </AreaChart>
        </ResponsiveContainer>
      </div>
      <div className="throughput-stats">
        <div className="stat-item stat-success">
          <span className="stat-dot" style={{ background: throughputColors.success }} />
          <span className="stat-label">Success</span>
          <span className="stat-value">{data.length > 0 ? data[data.length - 1].success_ops.toFixed(0) : 0}</span>
        </div>
        <div className="stat-item stat-failed">
          <span className="stat-dot" style={{ background: throughputColors.failed }} />
          <span className="stat-label">Failed</span>
          <span className="stat-value">{data.length > 0 ? data[data.length - 1].failed_ops.toFixed(0) : 0}</span>
        </div>
      </div>
    </div>
  );
}

export const ThroughputChart = memo(ThroughputChartComponent);
