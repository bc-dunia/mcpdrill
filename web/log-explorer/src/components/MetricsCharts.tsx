import type { MetricsDataPoint, MetricsSummary, StabilityMetrics } from '../types';
import { LatencyChart } from './LatencyChart';
import { ThroughputChart } from './ThroughputChart';
import { ErrorRateChart } from './ErrorRateChart';
import { ConnectionStabilityChart } from './ConnectionStabilityChart';
import { SessionLifecycleTable } from './SessionLifecycleTable';
import { ServerResourcesSection } from './ServerResourcesSection';
import { StabilityEventsTimeline } from './StabilityEventsTimeline';
import { ToolMetricsDashboard } from './ToolMetricsDashboard';
import { Icon } from './Icon';
import { MetricsSummary as MetricsSummarySection } from './MetricsSummary';

type MetricsTab = 'overview' | 'tools';

interface MetricsTabsProps {
  activeTab: MetricsTab;
  onChange: (tab: MetricsTab) => void;
}

interface MetricsChartsProps {
  activeTab: MetricsTab;
  runId: string;
  isRunActive: boolean;
  dataPoints: MetricsDataPoint[];
  loading: boolean;
  stability: StabilityMetrics | null;
  stabilityLoading: boolean;
  summary: MetricsSummary;
}

export function MetricsTabs({ activeTab, onChange }: MetricsTabsProps) {
  return (
    <div className="metrics-tabs" role="tablist" aria-label="Metrics view">
      <button
        role="tab"
        aria-selected={activeTab === 'overview'}
        aria-controls="metrics-overview-panel"
        className={`metrics-tab ${activeTab === 'overview' ? 'active' : ''}`}
        onClick={() => onChange('overview')}
      >
        <Icon name="chart-bar" size="sm" aria-hidden={true} /> Overview
      </button>
      <button
        role="tab"
        aria-selected={activeTab === 'tools'}
        aria-controls="metrics-tools-panel"
        className={`metrics-tab ${activeTab === 'tools' ? 'active' : ''}`}
        onClick={() => onChange('tools')}
      >
        <Icon name="tool" size="sm" aria-hidden={true} /> By Tool
      </button>
    </div>
  );
}

export function MetricsCharts({
  activeTab,
  runId,
  isRunActive,
  dataPoints,
  loading,
  stability,
  stabilityLoading,
  summary,
}: MetricsChartsProps) {
  return (
    <>
      {activeTab === 'overview' && (
        <div id="metrics-overview-panel" role="tabpanel" aria-labelledby="metrics-overview-tab">
          <MetricsSummarySection
            summary={summary}
            isRunActive={isRunActive}
            stabilityLoading={stabilityLoading}
            stability={stability}
          />

          <div className="metrics-charts-grid-hierarchical">
            <div className="chart-cell chart-primary">
              <ThroughputChart data={dataPoints} loading={loading && dataPoints.length === 0} />
            </div>
            <div className="chart-cell chart-secondary">
              <ConnectionStabilityChart
                data={stability?.time_series ?? []}
                loading={stabilityLoading && !stability?.time_series?.length}
              />
            </div>
            <div className="chart-cell chart-primary">
              <LatencyChart data={dataPoints} loading={loading && dataPoints.length === 0} />
            </div>
            <div className="chart-cell chart-secondary">
              <ErrorRateChart 
                data={dataPoints} 
                loading={loading && dataPoints.length === 0}
                threshold={0.1}
              />
            </div>
          </div>

          <ServerResourcesSection 
            runId={runId} 
            isRunActive={isRunActive ?? false}
          />

          {(stability?.events?.length ?? 0) > 0 && (
            <div className="stability-events-section">
              <StabilityEventsTimeline
                events={stability?.events ?? []}
                loading={stabilityLoading}
              />
            </div>
          )}

          {(stability?.session_metrics?.length ?? 0) > 0 && (
            <div className="session-lifecycle-section">
              <SessionLifecycleTable
                sessions={stability?.session_metrics ?? []}
                loading={stabilityLoading}
              />
            </div>
          )}

          {!isRunActive && dataPoints.length > 0 && (
            <div className="metrics-complete-notice" role="status">
              <Icon name="check" size="sm" aria-hidden={true} />
              <span>Run completed. Showing final metrics snapshot.</span>
            </div>
          )}
        </div>
      )}

      {activeTab === 'tools' && (
        <div id="metrics-tools-panel" role="tabpanel" aria-labelledby="metrics-tools-tab">
          <ToolMetricsDashboard runId={runId} />
        </div>
      )}
    </>
  );
}
