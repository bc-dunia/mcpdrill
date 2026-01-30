import { useRef, useCallback, memo } from 'react';
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
import type { ServerMetricsDataPoint } from '../types';
import { Icon } from './Icon';
import { exportChartAsPng } from '../utils/chartExport';

interface LoadAverageChartProps {
  data: ServerMetricsDataPoint[];
  loading?: boolean;
}

function generateDataSummary(data: ServerMetricsDataPoint[]): string {
  if (data.length === 0) return 'No data available';
  const latest = data[data.length - 1];
  return `Latest: 1m ${latest.load_avg_1.toFixed(2)}, 5m ${latest.load_avg_5.toFixed(2)}, 15m ${latest.load_avg_15.toFixed(2)}. ${data.length} data points.`;
}

const loadColors = {
  load1: '#4ade80',
  load5: '#fbbf24',
  load15: '#f87171',
};

interface TooltipPayload {
  name: string;
  value: number;
  color: string;
  dataKey: string;
}

const CustomTooltip = ({ active, payload, label }: { active?: boolean; payload?: TooltipPayload[]; label?: string }) => {
  if (!active || !payload?.length) return null;

  return (
    <div className="metrics-tooltip" role="tooltip" aria-live="polite">
      <div className="tooltip-time">{label}</div>
      <div className="tooltip-values">
        {payload.map((entry, index) => (
          <div key={index} className="tooltip-row">
            <span className="tooltip-dot" style={{ background: entry.color }} aria-hidden="true" />
            <span className="tooltip-label">{entry.name}</span>
            <span className="tooltip-value">{entry.value.toFixed(2)}</span>
          </div>
        ))}
      </div>
    </div>
  );
};

function LoadAverageChartComponent({ data, loading }: LoadAverageChartProps) {
  const chartRef = useRef<HTMLDivElement>(null);

  const exportAsPng = useCallback(() => {
    exportChartAsPng(chartRef.current, `load-average-${Date.now()}`);
  }, []);

  if (loading) {
    return (
      <div className="metrics-chart-container" role="region" aria-label="Load Average chart">
        <div className="metrics-chart-header">
          <h3>Load Average</h3>
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
      <div className="metrics-chart-container" role="region" aria-label="Load Average chart">
        <div className="metrics-chart-header">
          <h3>Load Average</h3>
        </div>
        <div className="metrics-chart-empty" role="status">
          <span className="empty-icon" aria-hidden="true"><Icon name="activity" size="xl" /></span>
          <span>No load average data available</span>
        </div>
      </div>
    );
  }

  return (
    <div 
      className="metrics-chart-container" 
      ref={chartRef}
      role="region" 
      aria-labelledby="load-chart-title"
      aria-describedby="load-chart-summary"
    >
      <div className="metrics-chart-header">
        <h3 id="load-chart-title" title="System load average (1, 5, 15 minute windows)">Load Average</h3>
        <button 
          className="btn btn-ghost btn-sm" 
          onClick={exportAsPng} 
          aria-label="Export load average chart as PNG"
        >
          <Icon name="download" size="sm" aria-hidden={true} />
        </button>
      </div>
      <p id="load-chart-summary" className="sr-only">{generateDataSummary(data)}</p>
      <div className="metrics-chart-body">
        <ResponsiveContainer width="100%" height={200}>
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
              dataKey="load_avg_1" 
              stroke={loadColors.load1}
              strokeWidth={2}
              dot={data.length <= 2 ? { r: 4, fill: loadColors.load1 } : false}
              activeDot={{ r: 4, fill: loadColors.load1 }}
              name="1 min"
            />
            <Line 
              type="monotone" 
              dataKey="load_avg_5" 
              stroke={loadColors.load5}
              strokeWidth={2}
              dot={data.length <= 2 ? { r: 4, fill: loadColors.load5 } : false}
              activeDot={{ r: 4, fill: loadColors.load5 }}
              name="5 min"
            />
            <Line 
              type="monotone" 
              dataKey="load_avg_15" 
              stroke={loadColors.load15}
              strokeWidth={2}
              dot={data.length <= 2 ? { r: 4, fill: loadColors.load15 } : false}
              activeDot={{ r: 4, fill: loadColors.load15 }}
              name="15 min"
            />
          </LineChart>
        </ResponsiveContainer>
      </div>
      <div className="latency-legend">
        <div className="latency-legend-item">
          <span className="legend-marker" style={{ background: loadColors.load1 }} />
          <span className="legend-text">1 min - Short-term load</span>
        </div>
        <div className="latency-legend-item">
          <span className="legend-marker" style={{ background: loadColors.load5 }} />
          <span className="legend-text">5 min - Medium-term load</span>
        </div>
        <div className="latency-legend-item">
          <span className="legend-marker" style={{ background: loadColors.load15 }} />
          <span className="legend-text">15 min - Long-term load</span>
        </div>
      </div>
    </div>
  );
}

export const LoadAverageChart = memo(LoadAverageChartComponent);
