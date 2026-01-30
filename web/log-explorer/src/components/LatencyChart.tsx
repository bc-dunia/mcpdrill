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
import type { MetricsDataPoint } from '../types';
import { Icon } from './Icon';
import { exportChartAsPng } from '../utils/chartExport';

interface LatencyChartProps {
  data: MetricsDataPoint[];
  loading?: boolean;
}

function generateDataSummary(data: MetricsDataPoint[]): string {
  if (data.length === 0) return 'No data available';
  const latest = data[data.length - 1];
  return `Latest: P50 ${latest.latency_p50_ms.toFixed(0)}ms, P95 ${latest.latency_p95_ms.toFixed(0)}ms, P99 ${latest.latency_p99_ms.toFixed(0)}ms. ${data.length} data points.`;
}

const latencyColors = {
  p50: '#4ade80',
  p95: '#fbbf24',
  p99: '#f87171',
  mean: '#22d3ee',
};

const CustomTooltip = ({ active, payload, label }: { active?: boolean; payload?: Array<{ name: string; value: number; color: string; dataKey: string }>; label?: string }) => {
  if (!active || !payload?.length) return null;

  return (
    <div className="metrics-tooltip" role="tooltip" aria-live="polite">
      <div className="tooltip-time">{label}</div>
      <div className="tooltip-values">
        {payload.map((entry, index) => (
          <div key={index} className="tooltip-row">
            <span className="tooltip-dot" style={{ background: entry.color }} aria-hidden="true" />
            <span className="tooltip-label">{entry.name}</span>
            <span className="tooltip-value">{entry.value.toFixed(1)} ms</span>
          </div>
        ))}
      </div>
    </div>
  );
}

function LatencyChartComponent({ data, loading }: LatencyChartProps) {
  const chartRef = useRef<HTMLDivElement>(null);

  const exportAsPng = useCallback(() => {
    exportChartAsPng(chartRef.current, `latency-${Date.now()}`);
  }, []);

  if (loading) {
    return (
      <div className="metrics-chart-container" role="region" aria-label="Latency Percentiles chart">
        <div className="metrics-chart-header">
          <h3>Latency Percentiles</h3>
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
      <div className="metrics-chart-container" role="region" aria-label="Latency Percentiles chart">
        <div className="metrics-chart-header">
          <h3>Latency Percentiles</h3>
        </div>
        <div className="metrics-chart-empty" role="status">
          <span className="empty-icon" aria-hidden="true"><Icon name="timer" size="xl" /></span>
          <span>No latency data available</span>
        </div>
      </div>
    );
  }

  return (
    <div 
      className="metrics-chart-container" 
      ref={chartRef}
      role="region" 
      aria-labelledby="latency-chart-title"
      aria-describedby="latency-chart-summary"
    >
      <div className="metrics-chart-header">
        <h3 id="latency-chart-title" title="Response time percentiles (P50, P95, P99) over time. Lower is better.">Latency Percentiles</h3>
        <button 
          className="btn btn-ghost btn-sm" 
          onClick={exportAsPng} 
          aria-label="Export latency chart as PNG"
        >
          <Icon name="download" size="sm" aria-hidden={true} />
        </button>
      </div>
      <p id="latency-chart-summary" className="sr-only">{generateDataSummary(data)}</p>
      <div className="metrics-chart-body">
        <ResponsiveContainer width="100%" height={280}>
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
              tickFormatter={(value) => `${value}ms`}
            />
            <Tooltip content={<CustomTooltip />} />
            <Legend 
              wrapperStyle={{ fontSize: '11px', paddingTop: '8px' }}
            />
             <Line 
               type="monotone" 
               dataKey="latency_p50_ms" 
               stroke={latencyColors.p50}
               strokeWidth={2}
               dot={data.length <= 2 ? { r: 4, fill: latencyColors.p50 } : false}
               activeDot={{ r: 4, fill: latencyColors.p50 }}
               name="P50"
             />
             <Line 
               type="monotone" 
               dataKey="latency_p95_ms" 
               stroke={latencyColors.p95}
               strokeWidth={2}
               dot={data.length <= 2 ? { r: 4, fill: latencyColors.p95 } : false}
               activeDot={{ r: 4, fill: latencyColors.p95 }}
               name="P95"
             />
             <Line 
               type="monotone" 
               dataKey="latency_p99_ms" 
               stroke={latencyColors.p99}
               strokeWidth={2}
               dot={data.length <= 2 ? { r: 4, fill: latencyColors.p99 } : false}
               activeDot={{ r: 4, fill: latencyColors.p99 }}
               name="P99"
             />
            <Line 
              type="monotone" 
              dataKey="latency_mean" 
              stroke={latencyColors.mean}
              strokeWidth={2}
              strokeDasharray="5 5"
              dot={data.length <= 2 ? { r: 4, fill: latencyColors.mean } : false}
              activeDot={{ r: 4, fill: latencyColors.mean }}
              name="Mean"
            />
          </LineChart>
        </ResponsiveContainer>
      </div>
      <div className="latency-legend">
        <div className="latency-legend-item">
          <span className="legend-marker" style={{ background: latencyColors.p50 }} />
          <span className="legend-text">P50 - Median latency</span>
        </div>
        <div className="latency-legend-item">
          <span className="legend-marker" style={{ background: latencyColors.p95 }} />
          <span className="legend-text">P95 - 95th percentile</span>
        </div>
        <div className="latency-legend-item">
          <span className="legend-marker" style={{ background: latencyColors.p99 }} />
          <span className="legend-text">P99 - 99th percentile</span>
        </div>
        <div className="latency-legend-item">
          <span className="legend-marker legend-dashed" style={{ background: latencyColors.mean }} />
          <span className="legend-text">Mean - Average latency</span>
        </div>
      </div>
    </div>
  );
}

export const LatencyChart = memo(LatencyChartComponent);
