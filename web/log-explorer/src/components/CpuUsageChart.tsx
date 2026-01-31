import { memo } from 'react';
import type { ServerMetricsDataPoint } from '../types';
import { BaseChart, AreaSeriesConfig, GradientDef } from './BaseChart';

interface CpuUsageChartProps {
  data: ServerMetricsDataPoint[];
  loading?: boolean;
}

const cpuColor = '#22d3ee';

const gradients: GradientDef[] = [
  { id: 'cpuGradient', color: cpuColor },
];

const series: AreaSeriesConfig[] = [
  { dataKey: 'cpu_percent', color: cpuColor, name: 'CPU', gradientId: 'cpuGradient' },
];

function generateDataSummary(data: ServerMetricsDataPoint[]): string {
  if (data.length === 0) return 'No data available';
  const latest = data[data.length - 1];
  const max = Math.max(...data.map(d => d.cpu_percent));
  return `Latest: ${latest.cpu_percent.toFixed(1)}% CPU. Peak: ${max.toFixed(1)}%. ${data.length} data points.`;
}

interface TooltipEntry {
  value: number;
  color: string;
}

const CustomTooltip = ({ active, payload, label }: { active?: boolean; payload?: unknown[]; label?: string }) => {
  if (!active || !payload?.length) return null;
  const entries = payload as TooltipEntry[];
  return (
    <div className="metrics-tooltip" role="tooltip" aria-live="polite">
      <div className="tooltip-time">{label}</div>
      <div className="tooltip-values">
        <div className="tooltip-row">
          <span className="tooltip-dot" style={{ background: cpuColor }} aria-hidden="true" />
          <span className="tooltip-label">CPU</span>
          <span className="tooltip-value">{entries[0].value.toFixed(1)}%</span>
        </div>
      </div>
    </div>
  );
};

function CpuUsageChartComponent({ data, loading }: CpuUsageChartProps) {
  return (
    <BaseChart<ServerMetricsDataPoint>
      data={data}
      loading={loading}
      chartType="area"
      title="CPU Usage"
      titleTooltip="Server CPU utilization over time"
      chartId="cpu"
      emptyIcon="cpu"
      emptyMessage="No CPU data available"
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

export const CpuUsageChart = memo(CpuUsageChartComponent);
