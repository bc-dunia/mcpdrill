import { memo, useMemo } from 'react';
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer,
  Cell,
  ReferenceLine,
} from 'recharts';
import type { ComparisonResult } from '../types';
import { Icon } from './Icon';

interface ComparisonChartProps {
  comparison: ComparisonResult;
}

interface LatencyDataPoint {
  name: string;
  runA: number;
  runB: number;
  diff: number;
  diffPct: number;
}

function ComparisonChartComponent({ comparison }: ComparisonChartProps) {
  const { run_a, run_b, diff } = comparison;

  const latencyData = useMemo<LatencyDataPoint[]>(() => [
    {
      name: 'P50',
      runA: run_a.latency_p50_ms,
      runB: run_b.latency_p50_ms,
      diff: diff.latency_p50_ms,
      diffPct: diff.latency_p50_pct,
    },
    {
      name: 'P95',
      runA: run_a.latency_p95_ms,
      runB: run_b.latency_p95_ms,
      diff: diff.latency_p95_ms,
      diffPct: diff.latency_p95_pct,
    },
    {
      name: 'P99',
      runA: run_a.latency_p99_ms,
      runB: run_b.latency_p99_ms,
      diff: diff.latency_p99_ms,
      diffPct: diff.latency_p99_pct,
    },
  ], [run_a, run_b, diff]);

  const throughputData = useMemo(() => [
    {
      name: 'Throughput',
      runA: run_a.throughput,
      runB: run_b.throughput,
      diff: diff.throughput,
      diffPct: diff.throughput_pct,
    },
  ], [run_a.throughput, run_b.throughput, diff.throughput, diff.throughput_pct]);

  const errorData = useMemo(() => [
    {
      name: 'Error Rate',
      runA: run_a.error_rate * 100,
      runB: run_b.error_rate * 100,
      diff: diff.error_rate * 100,
      diffPct: diff.error_rate_pct,
    },
  ], [run_a.error_rate, run_b.error_rate, diff.error_rate, diff.error_rate_pct]);

  const getBarColor = (diffPct: number, isLowerBetter: boolean) => {
    const threshold = 1;
    if (Math.abs(diffPct) < threshold) return 'var(--text-muted)';
    const isImprovement = isLowerBetter ? diffPct < 0 : diffPct > 0;
    return isImprovement ? 'var(--accent-green)' : 'var(--accent-red)';
  };

  const formatDiffText = (diffPct: number) => {
    const threshold = 1;
    if (Math.abs(diffPct) < threshold) return '';
    const sign = diffPct > 0 ? '+' : '';
    return `${sign}${diffPct.toFixed(1)}%`;
  };

  const getDiffIndicator = (diffPct: number, isLowerBetter: boolean): { icon: 'minus' | 'check-circle' | 'x-circle'; className: string } => {
    const threshold = 1;
    if (Math.abs(diffPct) < threshold) return { icon: 'minus', className: 'neutral' };
    const isImprovement = isLowerBetter ? diffPct < 0 : diffPct > 0;
    return isImprovement 
      ? { icon: 'check-circle', className: 'improved' }
      : { icon: 'x-circle', className: 'regressed' };
  };

  const CustomTooltip = ({ active, payload, label }: { active?: boolean; payload?: Array<{ value: number; dataKey: string }>; label?: string }) => {
    if (!active || !payload?.length) return null;
    const data = latencyData.find(d => d.name === label) || 
                 throughputData.find(d => d.name === label) ||
                 errorData.find(d => d.name === label);
    if (!data) return null;

    const isLatency = ['P50', 'P95', 'P99'].includes(label || '');
    const isError = label === 'Error Rate';
    const unit = isLatency ? 'ms' : isError ? '%' : '/s';
    const isLowerBetter = isLatency || isError;

    const indicator = getDiffIndicator(data.diffPct, isLowerBetter);
    return (
      <div className="chart-tooltip">
        <div className="tooltip-title">{label}</div>
        <div className="tooltip-row">
          <span className="tooltip-label run-a-label">Run A:</span>
          <span className="tooltip-value">{data.runA.toFixed(1)}{unit}</span>
        </div>
        <div className="tooltip-row">
          <span className="tooltip-label run-b-label">Run B:</span>
          <span className="tooltip-value">{data.runB.toFixed(1)}{unit}</span>
        </div>
        <div className={`tooltip-diff ${indicator.className}`}>
          {formatDiffText(data.diffPct)} <Icon name={indicator.icon} size="sm" aria-hidden={true} />
        </div>
      </div>
    );
  };

  // Generate screen reader summaries
  const latencySummary = useMemo(() => {
    const changes = latencyData.map(d => {
      const change = d.diffPct < -1 ? 'improved' : d.diffPct > 1 ? 'regressed' : 'unchanged';
      return `${d.name}: ${change} by ${Math.abs(d.diffPct).toFixed(1)}%`;
    });
    return `Latency comparison between runs. ${changes.join('. ')}.`;
  }, [latencyData]);

  const throughputSummary = useMemo(() => {
    const d = throughputData[0];
    const change = d.diffPct > 1 ? 'improved' : d.diffPct < -1 ? 'regressed' : 'unchanged';
    return `Throughput comparison: Run A ${d.runA.toFixed(1)} per second, Run B ${d.runB.toFixed(1)} per second. Performance ${change} by ${Math.abs(d.diffPct).toFixed(1)}%.`;
  }, [throughputData]);

  const errorSummary = useMemo(() => {
    const d = errorData[0];
    const change = d.diffPct < -1 ? 'improved' : d.diffPct > 1 ? 'regressed' : 'unchanged';
    return `Error rate comparison: Run A ${d.runA.toFixed(2)}%, Run B ${d.runB.toFixed(2)}%. Error rate ${change} by ${Math.abs(d.diffPct).toFixed(1)}%.`;
  }, [errorData]);

  return (
    <div className="comparison-charts">
      <div className="chart-section" role="region" aria-labelledby="latency-chart-title" aria-describedby="latency-chart-desc">
        <h3 id="latency-chart-title" className="chart-title">
          <span className="chart-icon" aria-hidden="true"><Icon name="timer" size="md" /></span>
          Latency Comparison
        </h3>
        <p id="latency-chart-desc" className="sr-only">{latencySummary}</p>
        <div className="chart-container">
          <ResponsiveContainer width="100%" height={280}>
            <BarChart data={latencyData} barGap={8}>
              <CartesianGrid strokeDasharray="3 3" stroke="var(--border-subtle)" vertical={false} />
              <XAxis 
                dataKey="name" 
                tick={{ fill: 'var(--text-muted)', fontSize: 12 }}
                axisLine={{ stroke: 'var(--border-subtle)' }}
                tickLine={false}
              />
              <YAxis 
                tick={{ fill: 'var(--text-muted)', fontSize: 11 }}
                axisLine={false}
                tickLine={false}
                tickFormatter={(v) => `${v}ms`}
              />
              <Tooltip content={<CustomTooltip />} cursor={{ fill: 'var(--bg-hover)', opacity: 0.5 }} />
              <Legend 
                wrapperStyle={{ paddingTop: 16 }}
                formatter={(value) => <span style={{ color: 'var(--text-secondary)', fontSize: 12 }}>{value}</span>}
              />
              <Bar dataKey="runA" name="Run A" radius={[4, 4, 0, 0]} maxBarSize={48}>
                {latencyData.map((_, index) => (
                  <Cell key={`cell-a-${index}`} fill="var(--accent-cyan)" opacity={0.85} />
                ))}
              </Bar>
              <Bar dataKey="runB" name="Run B" radius={[4, 4, 0, 0]} maxBarSize={48}>
                {latencyData.map((entry, index) => (
                  <Cell 
                    key={`cell-b-${index}`} 
                    fill={getBarColor(entry.diffPct, true)}
                    opacity={0.85}
                  />
                ))}
              </Bar>
            </BarChart>
          </ResponsiveContainer>
        </div>
        <div className="chart-legend-diff">
          {latencyData.map((d) => {
            const indicator = getDiffIndicator(d.diffPct, true);
            return (
              <div key={d.name} className="legend-diff-item">
                <span className="legend-diff-label">{d.name}:</span>
                <span className={`legend-diff-value ${indicator.className}`}>
                  {formatDiffText(d.diffPct)} <Icon name={indicator.icon} size="sm" aria-hidden={true} />
                </span>
              </div>
            );
          })}
        </div>
      </div>

      <div className="chart-row">
        <div className="chart-section chart-section-half" role="region" aria-labelledby="throughput-chart-title" aria-describedby="throughput-chart-desc">
          <h3 id="throughput-chart-title" className="chart-title">
            <span className="chart-icon" aria-hidden="true"><Icon name="rocket" size="md" /></span>
            Throughput
          </h3>
          <p id="throughput-chart-desc" className="sr-only">{throughputSummary}</p>
          <div className="chart-container chart-container-small">
            <ResponsiveContainer width="100%" height={200}>
              <BarChart data={throughputData} barGap={16}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--border-subtle)" vertical={false} />
                <XAxis 
                  dataKey="name" 
                  tick={{ fill: 'var(--text-muted)', fontSize: 12 }}
                  axisLine={{ stroke: 'var(--border-subtle)' }}
                  tickLine={false}
                />
                <YAxis 
                  tick={{ fill: 'var(--text-muted)', fontSize: 11 }}
                  axisLine={false}
                  tickLine={false}
                  tickFormatter={(v) => `${v}/s`}
                />
                <Tooltip content={<CustomTooltip />} cursor={{ fill: 'var(--bg-hover)', opacity: 0.5 }} />
                <Bar dataKey="runA" name="Run A" fill="var(--accent-cyan)" radius={[4, 4, 0, 0]} maxBarSize={56} opacity={0.85} />
                <Bar dataKey="runB" name="Run B" radius={[4, 4, 0, 0]} maxBarSize={56} opacity={0.85}>
                  {throughputData.map((entry, index) => (
                    <Cell 
                      key={`cell-t-${index}`} 
                      fill={getBarColor(entry.diffPct, false)}
                    />
                  ))}
                </Bar>
              </BarChart>
            </ResponsiveContainer>
          </div>
          <div className="chart-metric-summary">
            <span className={`metric-diff ${getDiffIndicator(diff.throughput_pct, false).className}`}>
              {formatDiffText(diff.throughput_pct)} <Icon name={getDiffIndicator(diff.throughput_pct, false).icon} size="sm" aria-hidden={true} />
            </span>
          </div>
        </div>

        <div className="chart-section chart-section-half" role="region" aria-labelledby="error-chart-title" aria-describedby="error-chart-desc">
          <h3 id="error-chart-title" className="chart-title">
            <span className="chart-icon" aria-hidden="true"><Icon name="alert-triangle" size="md" /></span>
            Error Rate
          </h3>
          <p id="error-chart-desc" className="sr-only">{errorSummary}</p>
          <div className="chart-container chart-container-small">
            <ResponsiveContainer width="100%" height={200}>
              <BarChart data={errorData} barGap={16}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--border-subtle)" vertical={false} />
                <XAxis 
                  dataKey="name" 
                  tick={{ fill: 'var(--text-muted)', fontSize: 12 }}
                  axisLine={{ stroke: 'var(--border-subtle)' }}
                  tickLine={false}
                />
                <YAxis 
                  tick={{ fill: 'var(--text-muted)', fontSize: 11 }}
                  axisLine={false}
                  tickLine={false}
                  tickFormatter={(v) => `${v}%`}
                  domain={[0, 'auto']}
                />
                <Tooltip content={<CustomTooltip />} cursor={{ fill: 'var(--bg-hover)', opacity: 0.5 }} />
                <ReferenceLine y={5} stroke="var(--accent-amber)" strokeDasharray="4 4" opacity={0.6} />
                <Bar dataKey="runA" name="Run A" fill="var(--accent-cyan)" radius={[4, 4, 0, 0]} maxBarSize={56} opacity={0.85} />
                <Bar dataKey="runB" name="Run B" radius={[4, 4, 0, 0]} maxBarSize={56} opacity={0.85}>
                  {errorData.map((entry, index) => (
                    <Cell 
                      key={`cell-e-${index}`} 
                      fill={getBarColor(entry.diffPct, true)}
                    />
                  ))}
                </Bar>
              </BarChart>
            </ResponsiveContainer>
          </div>
          <div className="chart-metric-summary">
            <span className={`metric-diff ${getDiffIndicator(diff.error_rate_pct, true).className}`}>
              {formatDiffText(diff.error_rate_pct)} <Icon name={getDiffIndicator(diff.error_rate_pct, true).icon} size="sm" aria-hidden={true} />
            </span>
          </div>
        </div>
      </div>
    </div>
  );
}

export const ComparisonChart = memo(ComparisonChartComponent);
