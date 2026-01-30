import { useRef, useCallback } from 'react';
import {
  ResponsiveContainer,
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
} from 'recharts';
import type { MetricsDataPoint } from '../types';
import { Icon } from './Icon';
import { exportChartAsPng } from '../utils/chartExport';

interface MetricsChartProps {
  data: MetricsDataPoint[];
  title: string;
  loading?: boolean;
}

const chartColors = {
  p50: '#4ade80',
  p95: '#fbbf24',
  p99: '#f87171',
  mean: '#22d3ee',
  throughput: '#22d3ee',
  errorRate: '#f87171',
  success: '#4ade80',
  failed: '#f87171',
};

const CustomTooltip = ({ active, payload, label }: { active?: boolean; payload?: Array<{ name: string; value: number; color: string }>; label?: string }) => {
  if (!active || !payload?.length) return null;

  return (
    <div className="metrics-tooltip">
      <div className="tooltip-time">{label}</div>
      <div className="tooltip-values">
        {payload.map((entry, index) => (
          <div key={index} className="tooltip-row">
            <span className="tooltip-dot" style={{ background: entry.color }} />
            <span className="tooltip-label">{entry.name}</span>
            <span className="tooltip-value">{typeof entry.value === 'number' ? entry.value.toFixed(2) : entry.value}</span>
          </div>
        ))}
      </div>
    </div>
  );
};

export function MetricsChart({ data, title, loading }: MetricsChartProps) {
  const chartRef = useRef<HTMLDivElement>(null);

  const exportAsPng = useCallback(() => {
    const filename = `${title.toLowerCase().replace(/\s+/g, '-')}-${Date.now()}`;
    exportChartAsPng(chartRef.current, filename);
  }, [title]);

  if (loading) {
    return (
      <div className="metrics-chart-container">
        <div className="metrics-chart-header">
          <h3>{title}</h3>
        </div>
        <div className="metrics-chart-loading">
          <div className="spinner" />
          <span>Loading metrics...</span>
        </div>
      </div>
    );
  }

  if (!data.length) {
    return (
      <div className="metrics-chart-container">
        <div className="metrics-chart-header">
          <h3>{title}</h3>
        </div>
        <div className="metrics-chart-empty">
          <span className="empty-icon"><Icon name="chart-bar" size="xl" /></span>
          <span>No data available</span>
        </div>
      </div>
    );
  }

  return (
    <div className="metrics-chart-container" ref={chartRef}>
      <div className="metrics-chart-header">
        <h3>{title}</h3>
        <button className="btn btn-ghost btn-sm" onClick={exportAsPng} title="Export as PNG">
          <Icon name="download" size="sm" aria-hidden={true} />
        </button>
      </div>
      <div className="metrics-chart-body">
        <ResponsiveContainer width="100%" height={240}>
          <LineChart data={data} margin={{ top: 8, right: 16, left: 0, bottom: 0 }}>
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
            <Line 
              type="monotone" 
              dataKey="throughput" 
              stroke={chartColors.throughput}
              strokeWidth={2}
              dot={false}
              activeDot={{ r: 4, fill: chartColors.throughput }}
              name="Throughput"
            />
          </LineChart>
        </ResponsiveContainer>
      </div>
    </div>
  );
}

export { chartColors };
