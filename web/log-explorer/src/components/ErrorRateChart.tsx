import { useRef, useCallback, memo } from 'react';
import {
  ResponsiveContainer,
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ReferenceLine,
} from 'recharts';
import type { MetricsDataPoint } from '../types';
import { Icon } from './Icon';
import { exportChartAsPng } from '../utils/chartExport';

interface ErrorRateChartProps {
  data: MetricsDataPoint[];
  loading?: boolean;
  threshold?: number;
}

function generateDataSummary(data: MetricsDataPoint[], threshold?: number): string {
  if (data.length === 0) return 'No data available';
  const latest = data[data.length - 1];
  const thresholdText = threshold !== undefined ? ` Threshold: ${(threshold * 100).toFixed(0)}%.` : '';
  return `Current error rate: ${(latest.error_rate * 100).toFixed(2)}%.${thresholdText} ${data.length} data points.`;
}

const errorColors = {
  rate: '#f87171',
  threshold: '#fbbf24',
};

const CustomTooltip = ({ active, payload, label }: { active?: boolean; payload?: Array<{ name: string; value: number; color: string }>; label?: string }) => {
  if (!active || !payload?.length) return null;

  return (
    <div className="metrics-tooltip" role="tooltip" aria-live="polite">
      <div className="tooltip-time">{label}</div>
      <div className="tooltip-values">
        {payload.map((entry, index) => (
          <div key={index} className="tooltip-row">
            <span className="tooltip-dot" style={{ background: entry.color }} aria-hidden="true" />
            <span className="tooltip-label">{entry.name}</span>
            <span className="tooltip-value">{(entry.value * 100).toFixed(2)}%</span>
          </div>
        ))}
      </div>
    </div>
  );
};

function ErrorRateChartComponent({ data, loading, threshold }: ErrorRateChartProps) {
  const chartRef = useRef<HTMLDivElement>(null);

  const exportAsPng = useCallback(() => {
    exportChartAsPng(chartRef.current, `error-rate-${Date.now()}`);
  }, []);

  const currentErrorRate = data.length > 0 ? data[data.length - 1].error_rate : 0;
  const isAboveThreshold = threshold !== undefined && currentErrorRate > threshold;

  if (loading) {
    return (
      <div className="metrics-chart-container" role="region" aria-label="Error Rate chart">
        <div className="metrics-chart-header">
          <h3>Error Rate</h3>
        </div>
        <div className="metrics-chart-loading" role="status" aria-label="Loading chart data">
          <div className="spinner" aria-hidden="true" />
          <span>Loading metrics...</span>
        </div>
      </div>
    );
  }

  if (!data.length) {
    return (
      <div className="metrics-chart-container" role="region" aria-label="Error Rate chart">
        <div className="metrics-chart-header">
          <h3>Error Rate</h3>
        </div>
        <div className="metrics-chart-empty" role="status">
          <span className="empty-icon" aria-hidden="true"><Icon name="alert-triangle" size="xl" /></span>
          <span>No error data available</span>
        </div>
      </div>
    );
  }

  return (
    <div 
      className="metrics-chart-container" 
      ref={chartRef}
      role="region" 
      aria-labelledby="error-rate-chart-title"
      aria-describedby="error-rate-chart-summary"
    >
      <div className="metrics-chart-header">
        <h3 id="error-rate-chart-title" title="Percentage of failed operations over time. Red line shows threshold.">Error Rate</h3>
        <div className="chart-header-actions">
          <span 
            className={`error-rate-badge ${isAboveThreshold ? 'error-rate-critical' : 'error-rate-ok'}`}
            role="status"
            aria-label={`Current error rate: ${(currentErrorRate * 100).toFixed(2)} percent${isAboveThreshold ? ', above threshold' : ''}`}
          >
            {(currentErrorRate * 100).toFixed(2)}%
          </span>
          <button 
            className="btn btn-ghost btn-sm" 
            onClick={exportAsPng} 
            aria-label="Export error rate chart as PNG"
          >
            <Icon name="download" size="sm" aria-hidden={true} />
          </button>
        </div>
      </div>
      <p id="error-rate-chart-summary" className="sr-only">{generateDataSummary(data, threshold)}</p>
      <div className="metrics-chart-body">
        <ResponsiveContainer width="100%" height={280}>
          <AreaChart data={data} margin={{ top: 8, right: 16, left: 0, bottom: 0 }}>
            <defs>
              <linearGradient id="errorGradient" x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%" stopColor={errorColors.rate} stopOpacity={0.4}/>
                <stop offset="95%" stopColor={errorColors.rate} stopOpacity={0.05}/>
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
              tickFormatter={(value) => `${(value * 100).toFixed(0)}%`}
              domain={[0, 'auto']}
            />
            <Tooltip content={<CustomTooltip />} />
            {threshold !== undefined && (
              <ReferenceLine 
                y={threshold} 
                stroke={errorColors.threshold}
                strokeDasharray="5 5"
                strokeWidth={2}
                label={{
                  value: `Threshold: ${(threshold * 100).toFixed(0)}%`,
                  position: 'right',
                  fill: errorColors.threshold,
                  fontSize: 11,
                }}
              />
            )}
            <Area 
              type="monotone" 
              dataKey="error_rate" 
              stroke={errorColors.rate}
              strokeWidth={2}
              fill="url(#errorGradient)"
              dot={data.length <= 2 ? { r: 4, fill: errorColors.rate } : false}
              name="Error Rate"
            />
          </AreaChart>
        </ResponsiveContainer>
      </div>
      {threshold !== undefined && (
        <div className="error-threshold-info">
          <span className="threshold-label">Stop Condition Threshold:</span>
          <span className="threshold-value">{(threshold * 100).toFixed(1)}%</span>
          {isAboveThreshold && (
            <span className="threshold-warning" role="alert">
              <Icon name="alert-triangle" size="sm" aria-hidden={true} /> Above threshold
            </span>
          )}
        </div>
      )}
    </div>
  );
}

export const ErrorRateChart = memo(ErrorRateChartComponent);
