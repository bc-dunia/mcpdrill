import { useRef, useCallback, memo, useMemo } from 'react';
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
import type { StabilityTimePoint } from '../types';
import { Icon } from './Icon';
import { exportChartAsPng } from '../utils/chartExport';

interface ConnectionStabilityChartProps {
  data: StabilityTimePoint[];
  loading?: boolean;
}

interface ChartDataPoint {
  time: string;
  timestamp: number;
  active: number;
  created: number;
  dropped: number;
  reconnects: number;
}

function formatTime(timestamp: number): string {
  const date = new Date(timestamp);
  return date.toLocaleTimeString('en-US', {
    hour12: false,
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  });
}

function transformData(data: StabilityTimePoint[]): ChartDataPoint[] {
  return data.map((point) => ({
    time: formatTime(point.timestamp),
    timestamp: point.timestamp,
    active: point.active_sessions,
    created: point.new_sessions,
    dropped: point.dropped_sessions,
    reconnects: point.reconnects,
  }));
}

const stabilityColors = {
  active: '#4ade80',
  created: '#22d3ee',
  dropped: '#f87171',
  reconnects: '#fbbf24',
};

const CustomTooltip = ({
  active,
  payload,
  label,
}: {
  active?: boolean;
  payload?: Array<{ name: string; value: number; color: string }>;
  label?: string;
}) => {
  if (!active || !payload?.length) return null;

  return (
    <div className="metrics-tooltip" role="tooltip">
      <div className="tooltip-time">{label}</div>
      <div className="tooltip-values">
        {payload.map((entry, index) => (
          <div key={index} className="tooltip-row">
            <span className="tooltip-dot" style={{ background: entry.color }} aria-hidden="true" />
            <span className="tooltip-label">{entry.name}</span>
            <span className="tooltip-value">{entry.value}</span>
          </div>
        ))}
      </div>
    </div>
  );
};

function ConnectionStabilityChartComponent({ data, loading }: ConnectionStabilityChartProps) {
  const chartRef = useRef<HTMLDivElement>(null);
  const chartData = useMemo(() => transformData(data), [data]);

  const exportAsPng = useCallback(() => {
    exportChartAsPng(chartRef.current, `connection-stability-${Date.now()}`);
  }, []);

  if (loading) {
    return (
      <div className="metrics-chart-container" role="region" aria-label="Connection Stability chart">
        <div className="metrics-chart-header">
          <h3>Connection Stability</h3>
        </div>
        <div className="metrics-chart-loading" role="status">
          <div className="spinner" aria-hidden="true" />
          <span>Loading stability data...</span>
        </div>
      </div>
    );
  }

  if (!data.length) {
    return (
      <div className="metrics-chart-container" role="region" aria-label="Connection Stability chart">
        <div className="metrics-chart-header">
          <h3>Connection Stability</h3>
        </div>
        <div className="metrics-chart-empty" role="status">
          <span className="empty-icon" aria-hidden="true">
            <Icon name="activity" size="xl" />
          </span>
          <span>No stability data available</span>
        </div>
      </div>
    );
  }

  return (
    <div
      className="metrics-chart-container"
      ref={chartRef}
      role="region"
      aria-labelledby="stability-chart-title"
    >
      <div className="metrics-chart-header">
        <h3 id="stability-chart-title" title="Session lifecycle: active, new, dropped, and reconnected sessions over time.">Connection Stability</h3>
        <button
          className="btn btn-ghost btn-sm"
          onClick={exportAsPng}
          aria-label="Export stability chart as PNG"
        >
          <Icon name="download" size="sm" aria-hidden={true} />
        </button>
      </div>
      <div className="metrics-chart-body">
        <ResponsiveContainer width="100%" height={280}>
          <AreaChart data={chartData} margin={{ top: 8, right: 16, left: 0, bottom: 0 }}>
            <CartesianGrid strokeDasharray="3 3" stroke="var(--border-subtle)" />
            <XAxis dataKey="time" stroke="var(--text-muted)" fontSize={11} tickLine={false} />
            <YAxis stroke="var(--text-muted)" fontSize={11} tickLine={false} axisLine={false} />
            <Tooltip content={<CustomTooltip />} isAnimationActive={false} />
            <Legend wrapperStyle={{ fontSize: '11px', paddingTop: '8px' }} />
            <Area
              type="monotone"
              dataKey="active"
              stackId="1"
              stroke={stabilityColors.active}
              fill={stabilityColors.active}
              fillOpacity={0.6}
              name="Active Sessions"
              isAnimationActive={false}
            />
            <Area
              type="monotone"
              dataKey="created"
              stackId="2"
              stroke={stabilityColors.created}
              fill={stabilityColors.created}
              fillOpacity={0.6}
              name="New Sessions"
              isAnimationActive={false}
            />
            <Area
              type="monotone"
              dataKey="dropped"
              stackId="3"
              stroke={stabilityColors.dropped}
              fill={stabilityColors.dropped}
              fillOpacity={0.6}
              name="Dropped"
              isAnimationActive={false}
            />
            <Area
              type="monotone"
              dataKey="reconnects"
              stackId="4"
              stroke={stabilityColors.reconnects}
              fill={stabilityColors.reconnects}
              fillOpacity={0.6}
              name="Reconnects"
              isAnimationActive={false}
            />
          </AreaChart>
        </ResponsiveContainer>
      </div>
      {chartData.length > 0 && (
        <div className="stability-stats">
          <div className="stat-item">
            <span className="stat-dot" style={{ background: stabilityColors.active }} />
            <span className="stat-label">Active</span>
            <span className="stat-value">{chartData[chartData.length - 1].active}</span>
          </div>
          <div className="stat-item">
            <span className="stat-dot" style={{ background: stabilityColors.created }} />
            <span className="stat-label">New</span>
            <span className="stat-value">{chartData[chartData.length - 1].created}</span>
          </div>
          <div className="stat-item">
            <span className="stat-dot" style={{ background: stabilityColors.dropped }} />
            <span className="stat-label">Dropped</span>
            <span className="stat-value">{chartData[chartData.length - 1].dropped}</span>
          </div>
          <div className="stat-item">
            <span className="stat-dot" style={{ background: stabilityColors.reconnects }} />
            <span className="stat-label">Reconnects</span>
            <span className="stat-value">{chartData[chartData.length - 1].reconnects}</span>
          </div>
        </div>
      )}
    </div>
  );
}

export const ConnectionStabilityChart = memo(ConnectionStabilityChartComponent);
