import { memo } from 'react';
import type { StopCondition, MetricsDataPoint } from '../types';

interface ThresholdStatusProps {
  thresholds: StopCondition[];
  currentMetrics: MetricsDataPoint | null;
}

type MetricFormatter = (value: number) => string;

const metricFormatters: Record<string, MetricFormatter> = {
  error_rate: (v) => `${(v * 100).toFixed(1)}%`,
  latency_p99_ms: (v) => `${v.toFixed(0)}ms`,
  latency_p95_ms: (v) => `${v.toFixed(0)}ms`,
  latency_p50_ms: (v) => `${v.toFixed(0)}ms`,
};

const metricLabels: Record<string, string> = {
  error_rate: 'Error Rate',
  latency_p99_ms: 'P99 Latency',
  latency_p95_ms: 'P95 Latency',
  latency_p50_ms: 'P50 Latency',
};

function getMetricValue(metrics: MetricsDataPoint | null, metric: string): number | null {
  if (!metrics) return null;
  
  switch (metric) {
    case 'error_rate':
      return metrics.error_rate;
    case 'latency_p99_ms':
      return metrics.latency_p99_ms;
    case 'latency_p95_ms':
      return metrics.latency_p95_ms;
    case 'latency_p50_ms':
      return metrics.latency_p50_ms;
    default:
      return null;
  }
}

function formatThreshold(metric: string, threshold: number): string {
  const formatter = metricFormatters[metric];
  return formatter ? formatter(threshold) : threshold.toString();
}

function isViolated(
  currentValue: number | null,
  threshold: number,
  comparator: string = '>'
): boolean {
  if (currentValue === null) return false;
  
  switch (comparator) {
    case '>':
      return currentValue > threshold;
    case '>=':
      return currentValue >= threshold;
    case '<':
      return currentValue < threshold;
    case '<=':
      return currentValue <= threshold;
    default:
      return currentValue > threshold;
  }
}

function ThresholdStatusComponent({ thresholds, currentMetrics }: ThresholdStatusProps) {
  if (thresholds.length === 0) {
    return (
      <div className="threshold-status threshold-status-empty">
        <span className="threshold-status-note">No stop conditions configured</span>
      </div>
    );
  }

  return (
    <div className="threshold-status" role="list" aria-label="Threshold status">
      {thresholds.map((threshold, index) => {
        const currentValue = getMetricValue(currentMetrics, threshold.metric);
        const violated = isViolated(currentValue, threshold.threshold, threshold.comparator);
        const formatter = metricFormatters[threshold.metric] || ((v: number) => v.toString());
        const label = metricLabels[threshold.metric] || threshold.metric;

        return (
          <div 
            key={threshold.id || index} 
            className={`threshold-item ${violated ? 'threshold-violated' : 'threshold-ok'}`}
            role="listitem"
          >
            <span className="threshold-icon" aria-hidden="true">
              {violated ? '\u26A0' : '\u2713'}
            </span>
            <span className="threshold-label">{label}</span>
            <span className="threshold-values">
              <span className="threshold-current">
                {currentValue !== null ? formatter(currentValue) : '—'}
              </span>
              <span className="threshold-comparator">
                {threshold.comparator || '>'}
              </span>
              <span className="threshold-limit">
                {formatThreshold(threshold.metric, threshold.threshold)}
              </span>
            </span>
          </div>
        );
      })}
    </div>
  );
}

export const ThresholdStatus = memo(ThresholdStatusComponent);
