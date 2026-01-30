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

interface MemoryUsageChartProps {
  data: ServerMetricsDataPoint[];
  loading?: boolean;
}

function generateDataSummary(data: ServerMetricsDataPoint[]): string {
  if (data.length === 0) return 'No data available';
  const latest = data[data.length - 1];
  const max = Math.max(...data.map(d => d.memory_percent));
  return `Latest: ${latest.memory_percent.toFixed(1)}% (${latest.memory_used_gb.toFixed(1)} / ${latest.memory_total_gb.toFixed(1)} GiB). Peak: ${max.toFixed(1)}%. ${data.length} data points.`;
}

const memoryColor = '#a78bfa';

interface TooltipPayload {
  value: number;
  color: string;
  payload: ServerMetricsDataPoint;
}

const CustomTooltip = ({ active, payload, label }: { active?: boolean; payload?: TooltipPayload[]; label?: string }) => {
  if (!active || !payload?.length) return null;

  const dataPoint = payload[0].payload;

  return (
    <div className="metrics-tooltip" role="tooltip" aria-live="polite">
      <div className="tooltip-time">{label}</div>
      <div className="tooltip-values">
        <div className="tooltip-row">
          <span className="tooltip-dot" style={{ background: memoryColor }} aria-hidden="true" />
          <span className="tooltip-label">Memory</span>
          <span className="tooltip-value">{payload[0].value.toFixed(1)}%</span>
        </div>
        <div className="tooltip-row">
          <span className="tooltip-label" style={{ marginLeft: '16px' }}>Used</span>
          <span className="tooltip-value">{dataPoint.memory_used_gb.toFixed(1)} / {dataPoint.memory_total_gb.toFixed(1)} GiB</span>
        </div>
      </div>
    </div>
  );
};

function MemoryUsageChartComponent({ data, loading }: MemoryUsageChartProps) {
  const chartRef = useRef<HTMLDivElement>(null);

  const exportAsPng = useCallback(() => {
    exportChartAsPng(chartRef.current, `memory-usage-${Date.now()}`);
  }, []);

  if (loading) {
    return (
      <div className="metrics-chart-container" role="region" aria-label="Memory Usage chart">
        <div className="metrics-chart-header">
          <h3>Memory Usage</h3>
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
      <div className="metrics-chart-container" role="region" aria-label="Memory Usage chart">
        <div className="metrics-chart-header">
          <h3>Memory Usage</h3>
        </div>
        <div className="metrics-chart-empty" role="status">
          <span className="empty-icon" aria-hidden="true"><Icon name="database" size="xl" /></span>
          <span>No memory data available</span>
        </div>
      </div>
    );
  }

  return (
    <div 
      className="metrics-chart-container" 
      ref={chartRef}
      role="region" 
      aria-labelledby="memory-chart-title"
      aria-describedby="memory-chart-summary"
    >
      <div className="metrics-chart-header">
        <h3 id="memory-chart-title" title="Server memory utilization over time">Memory Usage</h3>
        <div className="chart-header-actions">
          <span className="chart-unit">%</span>
          <button 
            className="btn btn-ghost btn-sm" 
            onClick={exportAsPng} 
            aria-label="Export memory chart as PNG"
          >
            <Icon name="download" size="sm" aria-hidden={true} />
          </button>
        </div>
      </div>
      <p id="memory-chart-summary" className="sr-only">{generateDataSummary(data)}</p>
      <div className="metrics-chart-body">
        <ResponsiveContainer width="100%" height={200}>
          <AreaChart data={data} margin={{ top: 8, right: 16, left: 0, bottom: 0 }}>
            <defs>
              <linearGradient id="memoryGradient" x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%" stopColor={memoryColor} stopOpacity={0.4}/>
                <stop offset="95%" stopColor={memoryColor} stopOpacity={0.05}/>
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
              dataKey="memory_percent" 
              stroke={memoryColor}
              strokeWidth={2}
              fill="url(#memoryGradient)"
              dot={data.length <= 2 ? { r: 4, fill: memoryColor } : false}
              name="Memory"
            />
          </AreaChart>
        </ResponsiveContainer>
      </div>
    </div>
  );
}

export const MemoryUsageChart = memo(MemoryUsageChartComponent);
