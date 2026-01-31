import type { MetricsSummary as MetricsSummaryType, StabilityMetrics } from '../types';

interface MetricsSummaryProps {
  summary: MetricsSummaryType;
  isRunActive: boolean;
  stabilityLoading: boolean;
  stability: StabilityMetrics | null;
}

export function MetricsSummary({ summary, isRunActive, stabilityLoading, stability }: MetricsSummaryProps) {
  return (
    <div className="metrics-summary-grid-compact" role="region" aria-label="Metrics summary">
      <div className="summary-card-mini" title="Total MCP operations executed">
        <span className="summary-value">
          {summary.total_ops.toLocaleString()}
          {isRunActive && summary.total_ops > 0 && (
            <span className="trend-arrow trend-up" aria-label="increasing">↑</span>
          )}
        </span>
        <span className="summary-label">Total Ops</span>
      </div>
      <div className="summary-card-mini" title="Percentage of successful operations">
        <span className="summary-value">
          {summary.success_rate.toFixed(1)}%
        </span>
        <span className="summary-label">Success</span>
      </div>
      <div className={`summary-card-mini ${summary.failed_ops > 0 ? 'card-error' : ''}`} title="Operations that failed">
        <span className={`summary-value ${summary.failed_ops > 0 ? 'text-danger' : ''}`}>
          {summary.failed_ops.toLocaleString()}
        </span>
        <span className="summary-label">Failed</span>
      </div>
      <div className="summary-card-mini" title="Highest throughput achieved">
        <span className="summary-value">{summary.peak_throughput.toFixed(1)}</span>
        <span className="summary-label">Peak/s</span>
      </div>
      <div className="summary-card-mini" title="Average response time">
        <span className={`summary-value ${summary.avg_latency > 500 ? 'text-warning' : ''}`}>
          {summary.avg_latency.toFixed(1)}ms
        </span>
        <span className="summary-label">Latency</span>
      </div>
      <div className="summary-card-mini" title="Active sessions">
        <span className="summary-value">
          {stabilityLoading ? '—' : (stability?.active_sessions ?? 0)}
        </span>
        <span className="summary-label">Sessions</span>
      </div>
    </div>
  );
}
