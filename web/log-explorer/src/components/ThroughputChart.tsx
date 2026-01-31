import { memo } from 'react';
import type { MetricsDataPoint, StageMarker } from '../types';
import { BaseChart, AreaSeriesConfig, GradientDef, BrushRange } from './BaseChart';

interface ThroughputChartProps {
  data: MetricsDataPoint[];
  loading?: boolean;
  enableBrush?: boolean;
  brushRange?: BrushRange;
  onBrushChange?: (range: BrushRange) => void;
  stageMarkers?: StageMarker[];
}

const throughputColors = {
  success: '#4ade80',
  failed: '#f87171',
};

const gradients: GradientDef[] = [
  { id: 'successGradient', color: throughputColors.success },
  { id: 'failedGradient', color: throughputColors.failed },
];

const series: AreaSeriesConfig[] = [
  { dataKey: 'success_ops', color: throughputColors.success, name: 'Success', gradientId: 'successGradient', stackId: '1' },
  { dataKey: 'failed_ops', color: throughputColors.failed, name: 'Failed', gradientId: 'failedGradient', stackId: '1' },
];

function generateDataSummary(data: MetricsDataPoint[]): string {
  if (data.length === 0) return 'No data available';
  const latest = data[data.length - 1];
  const totalOps = latest.success_ops + latest.failed_ops;
  return `Latest: ${totalOps.toFixed(0)} ops/sec (${latest.success_ops.toFixed(0)} success, ${latest.failed_ops.toFixed(0)} failed). ${data.length} data points.`;
}

interface TooltipEntry {
  name: string;
  value: number;
  color: string;
}

const CustomTooltip = ({ active, payload, label }: { active?: boolean; payload?: unknown[]; label?: string }) => {
  if (!active || !payload?.length) return null;
  const entries = payload as TooltipEntry[];
  const total = entries.reduce((sum, entry) => sum + (entry.value || 0), 0);
  return (
    <div className="metrics-tooltip" role="tooltip" aria-live="polite">
      <div className="tooltip-time">{label}</div>
      <div className="tooltip-values">
        {entries.map((entry, index) => (
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
};

const ThroughputStats = ({ data }: { data: MetricsDataPoint[] }) => {
  const latest = data.length > 0 ? data[data.length - 1] : null;
  
  return (
    <div className="throughput-stats">
      <div className="stat-item stat-success">
        <span className="stat-dot" style={{ background: throughputColors.success }} />
        <span className="stat-label">Success</span>
        <span className="stat-value" title="Latest successful ops/sec">{latest ? latest.success_ops.toLocaleString() : 0}</span>
      </div>
      <div className="stat-item stat-failed">
        <span className="stat-dot" style={{ background: throughputColors.failed }} />
        <span className="stat-label">Failed</span>
        <span className="stat-value" title="Latest failed ops/sec">{latest ? latest.failed_ops.toLocaleString() : 0}</span>
      </div>
    </div>
  );
};

function ThroughputChartComponent({ data, loading, enableBrush, brushRange, onBrushChange, stageMarkers }: ThroughputChartProps) {
  return (
    <BaseChart<MetricsDataPoint>
      data={data}
      loading={loading}
      chartType="area"
      title="Throughput"
      titleTooltip="Operations per second over time. Shows actual load being generated."
      chartId="throughput"
      emptyIcon="trending-up"
      emptyMessage="No throughput data available"
      dataSummary={generateDataSummary(data)}
      series={series}
      gradients={gradients}
      customTooltip={CustomTooltip}
      showLegend={true}
      headerActions={<span className="chart-unit">ops/sec</span>}
      footer={<ThroughputStats data={data} />}
      enableBrush={enableBrush}
      brushRange={brushRange}
      onBrushChange={onBrushChange}
      stageMarkers={stageMarkers}
    />
  );
}

export const ThroughputChart = memo(ThroughputChartComponent);
