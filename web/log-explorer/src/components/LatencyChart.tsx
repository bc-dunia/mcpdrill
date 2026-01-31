import { memo, useMemo } from 'react';
import type { MetricsDataPoint, StageMarker } from '../types';
import { BaseChart, LineSeriesConfig, BrushRange } from './BaseChart';

interface LatencyChartProps {
  data: MetricsDataPoint[];
  loading?: boolean;
  brushRange?: BrushRange;
  stageMarkers?: StageMarker[];
}

const latencyColors = {
  p50: '#4ade80',
  p95: '#fbbf24',
  p99: '#f87171',
  mean: '#22d3ee',
};

const series: LineSeriesConfig[] = [
  { dataKey: 'latency_p50_ms', color: latencyColors.p50, name: 'P50' },
  { dataKey: 'latency_p95_ms', color: latencyColors.p95, name: 'P95' },
  { dataKey: 'latency_p99_ms', color: latencyColors.p99, name: 'P99' },
  { dataKey: 'latency_mean', color: latencyColors.mean, name: 'Mean', dashed: true },
];

function generateDataSummary(data: MetricsDataPoint[]): string {
  if (data.length === 0) return 'No data available';
  const latest = data[data.length - 1];
  return `Latest: P50 ${latest.latency_p50_ms.toFixed(0)}ms, P95 ${latest.latency_p95_ms.toFixed(0)}ms, P99 ${latest.latency_p99_ms.toFixed(0)}ms. ${data.length} data points.`;
}

interface TooltipEntry {
  name: string;
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
        {entries.map((entry, index) => (
          <div key={index} className="tooltip-row">
            <span className="tooltip-dot" style={{ background: entry.color }} aria-hidden="true" />
            <span className="tooltip-label">{entry.name}</span>
            <span className="tooltip-value">{entry.value.toFixed(1)} ms</span>
          </div>
        ))}
      </div>
    </div>
  );
};

const LatencyLegend = () => (
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
);

function LatencyChartComponent({ data, loading, brushRange, stageMarkers }: LatencyChartProps) {
  const filteredData = useMemo(() => {
    if (!brushRange || data.length === 0) return data;
    return data.slice(brushRange.startIndex, brushRange.endIndex + 1);
  }, [data, brushRange]);

  return (
    <BaseChart<MetricsDataPoint>
      data={filteredData}
      loading={loading}
      chartType="line"
      title="Latency Percentiles"
      titleTooltip="Response time percentiles (P50, P95, P99) over time. Lower is better."
      chartId="latency"
      emptyIcon="timer"
      emptyMessage="No latency data available"
      dataSummary={generateDataSummary(filteredData)}
      series={series}
      yAxisConfig={{ formatter: (value) => `${value}ms` }}
      customTooltip={CustomTooltip}
      showLegend={true}
      footer={<LatencyLegend />}
      stageMarkers={stageMarkers}
    />
  );
}

export const LatencyChart = memo(LatencyChartComponent);
