import { useState, useCallback, useMemo } from 'react';
import type { RunInfo, MetricsDataPoint, MetricsSummary } from '../types';
import { stopRun, emergencyStopRun, type StopMode } from '../api';
import { Icon } from './Icon';
import { useMetricsData } from '../hooks/useMetricsData';
import { MetricsCharts, MetricsTabs } from './MetricsCharts';
import { MetricsControls, MetricsRunStatus, MetricsStopConfirm, MetricsStopReason } from './MetricsControls';

interface MetricsDashboardProps {
  runId: string;
  run?: RunInfo;
  onNavigateToWizard?: () => void;
}

function calculateSummary(dataPoints: MetricsDataPoint[], durationMs?: number): MetricsSummary {
  if (dataPoints.length === 0) {
    return {
      total_ops: 0,
      failed_ops: 0,
      success_rate: 100,
      avg_latency: 0,
      peak_throughput: 0,
      avg_error_rate: 0,
      duration_seconds: 0,
    };
  }

  const totalOps = dataPoints.reduce((sum, d) => sum + d.success_ops + d.failed_ops, 0);
  const failedOps = dataPoints.reduce((sum, d) => sum + d.failed_ops, 0);
  const avgLatency = dataPoints.length > 0 
    ? dataPoints.reduce((sum, d) => sum + d.latency_mean, 0) / dataPoints.length 
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
  const [isStopping, setIsStopping] = useState(false);
  const [showStopConfirm, setShowStopConfirm] = useState(false);
  const [selectedStopMode, setSelectedStopMode] = useState<StopMode | 'emergency'>('drain');
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
    handleManualRefresh,
    loadMetrics,
    loadRunState,
  } = useMetricsData({ runId, run });

  const summary = useMemo(() => calculateSummary(dataPoints, durationMs), [dataPoints, durationMs]);

  const handleStopRun = useCallback(async () => {
    setIsStopping(true);
    try {
      if (selectedStopMode === 'emergency') {
        await emergencyStopRun(runId);
      } else {
        await stopRun(runId, selectedStopMode);
      }
      setShowStopConfirm(false);
      loadRunState();
    } catch (err) {
      console.error('Failed to stop run:', err);
      alert(err instanceof Error ? err.message : 'Failed to stop run');
    } finally {
      setIsStopping(false);
    }
  }, [runId, selectedStopMode, loadRunState]);

  return (
    <section className="metrics-dashboard" aria-labelledby="metrics-dashboard-heading">
      <div className="metrics-dashboard-header">
        <div className="dashboard-title">
          <h2 id="metrics-dashboard-heading"><Icon name="chart-bar" size="lg" aria-hidden={true} /> Live Metrics</h2>
          {run && (
            <span className={`run-state-badge state-${run.state}`}>
              {run.state.replace(/_/g, ' ')}
            </span>
          )}
          {isRunActive && (
            <span className={`sse-status ${sseConnected ? 'connected' : 'disconnected'}`} title={sseConnected ? 'Real-time streaming connected' : 'Using polling (SSE disconnected)'}>
              <Icon name={sseConnected ? 'wifi' : 'wifi-off'} size="sm" aria-hidden={true} />
              {sseConnected ? 'Live' : 'Polling'}
            </span>
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

      <MetricsStopConfirm
        show={showStopConfirm}
        selectedStopMode={selectedStopMode}
        setSelectedStopMode={setSelectedStopMode}
        isStopping={isStopping}
        onCancel={() => setShowStopConfirm(false)}
        onConfirm={handleStopRun}
      />

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
        isStopping={isStopping}
        onStopClick={() => setShowStopConfirm(true)}
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
      />
    </section>
  );
}
