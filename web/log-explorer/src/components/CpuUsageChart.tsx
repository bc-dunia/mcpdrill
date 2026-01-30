import { useRef, useCallback, memo } from 'react';
import {
  ResponsiveContainer,
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
} from 'recharts';
import type { ServerMetricsDataPoint } from '../types';
import { Icon } from './Icon';
import { exportChartAsPng } from '../utils/chartExport';

interface CpuUsageChartProps {
  data: ServerMetricsDataPoint[];
  loading?: boolean;
}

function generateDataSummary(data: ServerMetricsDataPoint[]): string {
  if (data.length === 0) return 'No data available';
  const latest = data[data.length - 1];
  const max = Math.max(...data.map(d => d.cpu_percent));
  return `Latest: ${latest.cpu_percent.toFixed(1)}% CPU. Peak: ${max.toFixed(1)}%. ${data.length} data points.`;
}

const cpuColor = '#22d3ee';

const CustomTooltip = ({ active, payload, label }: { active?: boolean; payload?: Array<{ value: number; color: string }>; label?: string }) => {
  if (!active || !payload?.length) return null;

  return (
    <div className="metrics-tooltip" role="tooltip" aria-live="polite">
      <div className="tooltip-time">{label}</div>
      <div className="tooltip-values">
        <div className="tooltip-row">
          <span className="tooltip-dot" style={{ background: cpuColor }} aria-hidden="true" />
          <span className="tooltip-label">CPU</span>
          <span className="tooltip-value">{payload[0].value.toFixed(1)}%</span>
        </div>
      </div>
    </div>
  );
};

function CpuUsageChartComponent({ data, loading }: CpuUsageChartProps) {
  const chartRef = useRef<HTMLDivElement>(null);

  const exportAsPng = useCallback(() => {
    exportChartAsPng(chartRef.current, `cpu-usage-${Date.now()}`);
  }, []);

  if (loading) {
    return (
      <div className="metrics-chart-container" role="region" aria-label="CPU Usage chart">
        <div className="metrics-chart-header">
          <h3>CPU Usage</h3>
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
      <div className="metrics-chart-container" role="region" aria-label="CPU Usage chart">
        <div className="metrics-chart-header">
          <h3>CPU Usage</h3>
        </div>
        <div className="metrics-chart-empty" role="status">
          <span className="empty-icon" aria-hidden="true"><Icon name="cpu" size="xl" /></span>
          <span>No CPU data available</span>
        </div>
      </div>
    );
  }

  return (
    <div 
      className="metrics-chart-container" 
      ref={chartRef}
      role="region" 
      aria-labelledby="cpu-chart-title"
      aria-describedby="cpu-chart-summary"
    >
      <div className="metrics-chart-header">
        <h3 id="cpu-chart-title" title="Server CPU utilization over time">CPU Usage</h3>
        <div className="chart-header-actions">
          <span className="chart-unit">%</span>
          <button 
            className="btn btn-ghost btn-sm" 
            onClick={exportAsPng} 
            aria-label="Export CPU chart as PNG"
          >
            <Icon name="download" size="sm" aria-hidden={true} />
          </button>
        </div>
      </div>
      <p id="cpu-chart-summary" className="sr-only">{generateDataSummary(data)}</p>
      <div className="metrics-chart-body">
        <ResponsiveContainer width="100%" height={200}>
          <AreaChart data={data} margin={{ top: 8, right: 16, left: 0, bottom: 0 }}>
            <defs>
              <linearGradient id="cpuGradient" x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%" stopColor={cpuColor} stopOpacity={0.4}/>
                <stop offset="95%" stopColor={cpuColor} stopOpacity={0.05}/>
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
              domain={[0, 100]}
              tickFormatter={(value) => `${value}%`}
            />
            <Tooltip content={<CustomTooltip />} />
            <Area 
              type="monotone" 
              dataKey="cpu_percent" 
              stroke={cpuColor}
              strokeWidth={2}
              fill="url(#cpuGradient)"
              dot={data.length <= 2 ? { r: 4, fill: cpuColor } : false}
              name="CPU"
            />
          </AreaChart>
        </ResponsiveContainer>
      </div>
    </div>
  );
}

export const CpuUsageChart = memo(CpuUsageChartComponent);
