import { memo } from 'react';
import type { ServerMetricsDataPoint } from '../types';
import { BaseChart, AreaSeriesConfig, GradientDef } from './BaseChart';

interface MemoryUsageChartProps {
  data: ServerMetricsDataPoint[];
  loading?: boolean;
}

const memoryColor = '#a78bfa';

const gradients: GradientDef[] = [
  { id: 'memoryGradient', color: memoryColor },
];

const series: AreaSeriesConfig[] = [
  { dataKey: 'memory_percent', color: memoryColor, name: 'Memory', gradientId: 'memoryGradient' },
];

function generateDataSummary(data: ServerMetricsDataPoint[]): string {
  if (data.length === 0) return 'No data available';
  const latest = data[data.length - 1];
  const max = Math.max(...data.map(d => d.memory_percent));
  return `Latest: ${latest.memory_percent.toFixed(1)}% (${latest.memory_used_gb.toFixed(1)} / ${latest.memory_total_gb.toFixed(1)} GiB). Peak: ${max.toFixed(1)}%. ${data.length} data points.`;
}

interface TooltipPayload {
  value: number;
  color: string;
  payload: ServerMetricsDataPoint;
}

const CustomTooltip = ({ active, payload, label }: { active?: boolean; payload?: unknown[]; label?: string }) => {
  if (!active || !payload?.length) return null;
  const entries = payload as TooltipPayload[];
  const dataPoint = entries[0].payload;
  return (
    <div className="metrics-tooltip" role="tooltip" aria-live="polite">
      <div className="tooltip-time">{label}</div>
      <div className="tooltip-values">
        <div className="tooltip-row">
          <span className="tooltip-dot" style={{ background: memoryColor }} aria-hidden="true" />
          <span className="tooltip-label">Memory</span>
          <span className="tooltip-value">{entries[0].value.toFixed(1)}%</span>
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
  return (
    <BaseChart<ServerMetricsDataPoint>
      data={data}
      loading={loading}
      chartType="area"
      title="Memory Usage"
      titleTooltip="Server memory utilization over time"
      chartId="memory"
      emptyIcon="database"
      emptyMessage="No memory data available"
      dataSummary={generateDataSummary(data)}
      height={200}
      series={series}
      gradients={gradients}
      yAxisConfig={{
        formatter: (value) => `${value}%`,
        domain: [0, 100],
      }}
      customTooltip={CustomTooltip}
      headerActions={<span className="chart-unit">%</span>}
    />
  );
}

export const MemoryUsageChart = memo(MemoryUsageChartComponent);
