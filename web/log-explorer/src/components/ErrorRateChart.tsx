import { memo } from 'react';
import type { MetricsDataPoint } from '../types';
import { BaseChart, AreaSeriesConfig, GradientDef, ReferenceLineConfig } from './BaseChart';
import { Icon } from './Icon';

interface ErrorRateChartProps {
  data: MetricsDataPoint[];
  loading?: boolean;
  threshold?: number;
}

const errorColors = {
  rate: '#f87171',
  threshold: '#fbbf24',
};

const gradients: GradientDef[] = [
  { id: 'errorGradient', color: errorColors.rate },
];

const series: AreaSeriesConfig[] = [
  { dataKey: 'error_rate', color: errorColors.rate, name: 'Error Rate', gradientId: 'errorGradient' },
];

function generateDataSummary(data: MetricsDataPoint[], threshold?: number): string {
  if (data.length === 0) return 'No data available';
  const latest = data[data.length - 1];
  const thresholdText = threshold !== undefined ? ` Threshold: ${(threshold * 100).toFixed(0)}%.` : '';
  return `Current error rate: ${(latest.error_rate * 100).toFixed(2)}%.${thresholdText} ${data.length} data points.`;
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
            <span className="tooltip-value">{(entry.value * 100).toFixed(2)}%</span>
          </div>
        ))}
      </div>
    </div>
  );
};

interface ErrorRateBadgeProps {
  rate: number;
  isAboveThreshold: boolean;
}

const ErrorRateBadge = ({ rate, isAboveThreshold }: ErrorRateBadgeProps) => (
  <span
    className={`error-rate-badge ${isAboveThreshold ? 'error-rate-critical' : 'error-rate-ok'}`}
    role="status"
    aria-label={`Current error rate: ${(rate * 100).toFixed(2)} percent${isAboveThreshold ? ', above threshold' : ''}`}
  >
    {(rate * 100).toFixed(2)}%
  </span>
);

interface ThresholdInfoProps {
  threshold: number;
  isAboveThreshold: boolean;
}

const ThresholdInfo = ({ threshold, isAboveThreshold }: ThresholdInfoProps) => (
  <div className="error-threshold-info">
    <span className="threshold-label">Stop Condition Threshold:</span>
    <span className="threshold-value">{(threshold * 100).toFixed(1)}%</span>
    {isAboveThreshold && (
      <span className="threshold-warning" role="alert">
        <Icon name="alert-triangle" size="sm" aria-hidden={true} /> Above threshold
      </span>
    )}
  </div>
);

function ErrorRateChartComponent({ data, loading, threshold }: ErrorRateChartProps) {
  const currentErrorRate = data.length > 0 ? data[data.length - 1].error_rate : 0;
  const isAboveThreshold = threshold !== undefined && currentErrorRate > threshold;

  const referenceLine: ReferenceLineConfig | undefined = threshold !== undefined
    ? {
        y: threshold,
        stroke: errorColors.threshold,
        label: `Threshold: ${(threshold * 100).toFixed(0)}%`,
        dashed: true,
      }
    : undefined;

  return (
    <BaseChart<MetricsDataPoint>
      data={data}
      loading={loading}
      chartType="area"
      title="Error Rate"
      titleTooltip="Percentage of failed operations over time. Red line shows threshold."
      chartId="error-rate"
      emptyIcon="alert-triangle"
      emptyMessage="No error data available"
      dataSummary={generateDataSummary(data, threshold)}
      series={series}
      gradients={gradients}
      yAxisConfig={{
        formatter: (value) => `${(value * 100).toFixed(0)}%`,
        domain: [0, 'auto'],
      }}
      customTooltip={CustomTooltip}
      referenceLine={referenceLine}
      headerActions={<ErrorRateBadge rate={currentErrorRate} isAboveThreshold={isAboveThreshold} />}
      footer={threshold !== undefined ? <ThresholdInfo threshold={threshold} isAboveThreshold={isAboveThreshold} /> : undefined}
    />
  );
}

export const ErrorRateChart = memo(ErrorRateChartComponent);
