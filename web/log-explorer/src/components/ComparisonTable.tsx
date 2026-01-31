import { memo } from 'react';
import type { ComparisonResult, MetricConfig, MetricKey } from '../types';
import { Icon, type IconName } from './Icon';

interface ComparisonTableProps {
  comparison: ComparisonResult;
}

const METRIC_CONFIGS: MetricConfig[] = [
  {
    key: 'throughput',
    label: 'Throughput',
    unit: '/s',
    direction: 'higher_better',
    format: (v) => v.toFixed(1),
  },
  {
    key: 'latency_p50_ms',
    label: 'P50 Latency',
    unit: 'ms',
    direction: 'lower_better',
    format: (v) => v.toFixed(1),
  },
  {
    key: 'latency_p95_ms',
    label: 'P95 Latency',
    unit: 'ms',
    direction: 'lower_better',
    format: (v) => v.toFixed(1),
  },
  {
    key: 'latency_p99_ms',
    label: 'P99 Latency',
    unit: 'ms',
    direction: 'lower_better',
    format: (v) => v.toFixed(1),
  },
  {
    key: 'error_rate',
    label: 'Error Rate',
    unit: '%',
    direction: 'lower_better',
    format: (v) => (v * 100).toFixed(2),
  },
  {
    key: 'total_ops',
    label: 'Total Operations',
    unit: '',
    direction: 'higher_better',
    format: (v) => v.toLocaleString(),
  },
  {
    key: 'failed_ops',
    label: 'Failed Operations',
    unit: '',
    direction: 'lower_better',
    format: (v) => v.toLocaleString(),
  },
  {
    key: 'duration_ms',
    label: 'Duration',
    unit: 's',
    direction: 'lower_better',
    format: (v) => (v / 1000).toFixed(1),
  },
];

function calculateDiffPct(valueA: number, valueB: number): number {
  if (valueA === 0) return valueB === 0 ? 0 : 100;
  return ((valueB - valueA) / valueA) * 100;
}

function ComparisonTableComponent({ comparison }: ComparisonTableProps) {
  const { run_a, run_b } = comparison;

  const getIndicator = (diffPct: number, direction: 'higher_better' | 'lower_better'): { icon: IconName; className: string; label: string } => {
    const threshold = 1;
    if (Math.abs(diffPct) < threshold) {
      return { icon: 'minus', className: 'neutral', label: 'No change' };
    }
    const isImprovement = direction === 'lower_better' ? diffPct < 0 : diffPct > 0;
    return isImprovement
      ? { icon: 'check-circle', className: 'improved', label: 'Improved' }
      : { icon: 'x-circle', className: 'regressed', label: 'Regressed' };
  };

  const formatDiff = (valueA: number, valueB: number, config: MetricConfig) => {
    const rawDiff = valueB - valueA;
    if (config.key === 'error_rate') {
      return `${rawDiff >= 0 ? '+' : ''}${(rawDiff * 100).toFixed(2)}%`;
    }
    if (config.key === 'duration_ms') {
      return `${rawDiff >= 0 ? '+' : ''}${(rawDiff / 1000).toFixed(1)}s`;
    }
    return `${rawDiff >= 0 ? '+' : ''}${config.format(rawDiff)}${config.unit}`;
  };

  const formatPctChange = (pct: number) => {
    if (Math.abs(pct) < 0.1) return '0%';
    return `${pct >= 0 ? '+' : ''}${pct.toFixed(1)}%`;
  };

  return (
    <div className="comparison-table-wrapper">
      <table className="comparison-table" aria-label="Run comparison metrics">
        <thead>
          <tr>
            <th scope="col" className="col-metric">Metric</th>
            <th scope="col" className="col-value">
              <span className="run-label run-a">Run A</span>
              <span className="run-id">{run_a.run_id.slice(0, 16)}...</span>
            </th>
            <th scope="col" className="col-value">
              <span className="run-label run-b">Run B</span>
              <span className="run-id">{run_b.run_id.slice(0, 16)}...</span>
            </th>
            <th scope="col" className="col-diff">Diff</th>
            <th scope="col" className="col-pct">% Change</th>
            <th scope="col" className="col-status">Status</th>
          </tr>
        </thead>
        <tbody>
          {METRIC_CONFIGS.map((config) => {
            const valueA = run_a[config.key as MetricKey];
            const valueB = run_b[config.key as MetricKey];
            const diffPct = calculateDiffPct(valueA, valueB);
            const indicator = getIndicator(diffPct, config.direction);

            return (
              <tr key={config.key} className={`row-${indicator.className}`}>
                <td className="col-metric">
                  <span className="metric-name">{config.label}</span>
                </td>
                <td className="col-value">
                  <span className="value-display">
                    {config.format(valueA)}
                    <span className="value-unit">{config.unit}</span>
                  </span>
                </td>
                <td className="col-value">
                  <span className="value-display">
                    {config.format(valueB)}
                    <span className="value-unit">{config.unit}</span>
                  </span>
                </td>
                <td className="col-diff">
                  <span className={`diff-value ${indicator.className}`}>
                    {formatDiff(valueA, valueB, config)}
                  </span>
                </td>
                <td className="col-pct">
                  <span className={`pct-value ${indicator.className}`}>
                    {formatPctChange(diffPct)}
                  </span>
                </td>
                <td className="col-status">
                  <span 
                    className={`status-indicator ${indicator.className}`} 
                    title={indicator.label}
                    aria-label={indicator.label}
                  >
                    <Icon name={indicator.icon} size="sm" aria-hidden={true} />
                  </span>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

export const ComparisonTable = memo(ComparisonTableComponent);
