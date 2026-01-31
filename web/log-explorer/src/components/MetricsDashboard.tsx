import { useState, useMemo } from 'react';
import type { RunInfo, MetricsDataPoint, MetricsSummary } from '../types';
import { Icon } from './Icon';
import { useMetricsData, type LatestTotals } from '../hooks/useMetricsData';
import { MetricsCharts, MetricsTabs } from './MetricsCharts';
import { MetricsControls, MetricsRunStatus, MetricsStopReason } from './MetricsControls';
import { ConnectionStatus } from './ConnectionStatus';

interface MetricsDashboardProps {
  runId: string;
  run?: RunInfo;
  onNavigateToWizard?: () => void;
}

function calculateSummary(
  dataPoints: MetricsDataPoint[],
  latestTotals: LatestTotals,
  durationMs?: number
): MetricsSummary {
  const totalOps = latestTotals.total_ops;
  const failedOps = latestTotals.failed_ops;
  
  const avgLatency = dataPoints.length > 0 
    ? dataPoints[dataPoints.length - 1].latency_mean
    : 0;
  const peakThroughput = dataPoints.length > 0 
    ? Math.max(...dataPoints.map(d => d.throughput)) 
    : 0;
  const avgErrorRate = dataPoints.length > 0 
    ? dataPoints.reduce((sum, d) => sum + d.error_rate, 0) / dataPoints.length 
    : 0;
  const durationSeconds = durationMs !== undefined && durationMs > 0
    ? durationMs / 1000
    : dataPoints.length > 1 
      ? (dataPoints[dataPoints.length - 1].timestamp - dataPoints[0].timestamp) / 1000 
      : 0;

  return {
    total_ops: totalOps,
    failed_ops: failedOps,
    success_rate: totalOps > 0 ? ((totalOps - failedOps) / totalOps) * 100 : 100,
    avg_latency: avgLatency,
    peak_throughput: peakThroughput,
    avg_error_rate: avgErrorRate,
    duration_seconds: Math.max(0, durationSeconds),
  };
}

export function MetricsDashboard({ runId, run, onNavigateToWizard }: MetricsDashboardProps) {
  const [activeMetricsTab, setActiveMetricsTab] = useState<'overview' | 'tools'>('overview');
  const [stopReasonDismissed, setStopReasonDismissed] = useState(false);

  const {
    dataPoints,
    loading,
    error,
    isAutoRefresh,
    setIsAutoRefresh,
    durationMs,
    stability,
    stabilityLoading,
    currentRunState,
    currentRunInfo,
    sseConnected,
    currentStage,
    isRunActive,
    elapsedMs,
    latestTotals,
    stageMarkers,
    handleManualRefresh,
    loadMetrics,
    loadRunState,
  } = useMetricsData({ runId, run });

  const summary = useMemo(
    () => calculateSummary(dataPoints, latestTotals, durationMs),
    [dataPoints, latestTotals, durationMs]
  );

  return (
    <section className="metrics-dashboard" aria-labelledby="metrics-dashboard-heading">
      <div className="metrics-dashboard-header">
        <div className="dashboard-title">
          <h2 id="metrics-dashboard-heading"><Icon name="chart-bar" size="lg" aria-hidden={true} /> Live Metrics</h2>
          {currentRunState && (
            <span className={`run-state-badge state-${currentRunState}`}>
              {currentRunState.replace(/_/g, ' ')}
            </span>
          )}
          {isRunActive && (
            <ConnectionStatus
              connected={sseConnected}
              lastUpdated={dataPoints.length > 0 ? dataPoints[dataPoints.length - 1].timestamp : null}
            />
          )}
          {currentStage && isRunActive && (
            <span className="current-stage-badge">
              <Icon name="zap" size="sm" aria-hidden={true} /> {currentStage}
            </span>
          )}
        </div>
        <MetricsTabs activeTab={activeMetricsTab} onChange={setActiveMetricsTab} />
        <MetricsControls
          runId={runId}
          isAutoRefresh={isAutoRefresh}
          setIsAutoRefresh={setIsAutoRefresh}
          isRunActive={isRunActive}
          loading={loading}
          onManualRefresh={handleManualRefresh}
          summary={summary}
          stability={stability}
          dataPoints={dataPoints}
        />
      </div>

      {error && (
        <div className="error-state" role="alert">
          <div className="error-state-icon" aria-hidden="true"><Icon name="chart-bar" size="xl" /></div>
          <h3>Metrics Unavailable</h3>
          <p className="error-state-message">
            Unable to load performance metrics for this run.
          </p>
          <code className="error-state-details">{error}</code>
          <button 
            type="button" 
            className="btn-retry" 
            onClick={() => loadMetrics()}
            disabled={loading}
          >
            <Icon name="refresh" size="sm" aria-hidden={true} />
            {loading ? 'Retrying...' : 'Try Again'}
          </button>
        </div>
      )}

      <MetricsRunStatus
        elapsedMs={elapsedMs}
        isRunActive={isRunActive}
        currentRunState={currentRunState}
        onNavigateToWizard={onNavigateToWizard}
        runId={runId}
        onStopped={loadRunState}
      />

      {!isRunActive && currentRunInfo?.stop_reason && !stopReasonDismissed && (() => {
        return (
          <MetricsStopReason
            stopReason={currentRunInfo.stop_reason}
            onDismiss={() => setStopReasonDismissed(true)}
          />
        );
      })()}

      <MetricsCharts
        activeTab={activeMetricsTab}
        runId={runId}
        isRunActive={isRunActive}
        dataPoints={dataPoints}
        loading={loading}
        stability={stability}
        stabilityLoading={stabilityLoading}
        summary={summary}
        stageMarkers={stageMarkers}
      />
    </section>
  );
}
