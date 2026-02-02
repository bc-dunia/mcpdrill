import { useState, useEffect, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import type { RunInfo, MetricsDataPoint } from '../types';
import { fetchRun } from '../api/runs';
import { useMetricsData } from '../hooks/useMetricsData';
import { Icon } from './Icon';
import { PassFailBadge, determinePassFailStatus } from './PassFailBadge';
import { ThresholdStatus } from './ThresholdStatus';
import { StopRunDialog } from './StopRunDialog';
import { useToast } from './Toast';
import { ServerResourcesSection } from './ServerResourcesSection';

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  const seconds = Math.floor(ms / 1000);
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = seconds % 60;
  if (minutes < 60) return `${minutes}m ${remainingSeconds}s`;
  const hours = Math.floor(minutes / 60);
  const remainingMinutes = minutes % 60;
  return `${hours}h ${remainingMinutes}m`;
}

function formatTimestamp(isoString?: string): string {
  if (!isoString) return '—';
  return new Date(isoString).toLocaleString();
}

interface KpiCardProps {
  label: string;
  value: string | number;
  unit?: string;
  trend?: 'up' | 'down' | 'neutral';
  highlight?: 'success' | 'warning' | 'danger';
}

function KpiCard({ label, value, unit, highlight }: KpiCardProps) {
  const highlightClass = highlight ? `kpi-card-${highlight}` : '';
  return (
    <div className={`kpi-card ${highlightClass}`}>
      <span className="kpi-value">
        {value}
        {unit && <span className="kpi-unit">{unit}</span>}
      </span>
      <span className="kpi-label">{label}</span>
    </div>
  );
}

interface StageTimelineProps {
  stages: Array<{ name: string; rawName?: string; duration_ms: number; status: 'completed' | 'running' | 'pending' }>;
  currentStage?: string | null;
}

function sanitizeClassName(name: string): string {
  return name.toLowerCase().replace(/[^a-z0-9-]/g, '-').replace(/-+/g, '-').replace(/^-|-$/g, '');
}

function StageTimeline({ stages, currentStage }: StageTimelineProps) {
  const totalDuration = stages.reduce((sum, s) => sum + s.duration_ms, 0);
  
  if (stages.length === 0) {
    return (
      <div className="stage-timeline stage-timeline-empty">
        <span className="stage-timeline-note">Stage information unavailable</span>
      </div>
    );
  }

  return (
    <div className="stage-timeline">
      <div className="stage-timeline-bar">
        {stages.map((stage) => {
          const widthPercent = totalDuration > 0 ? (stage.duration_ms / totalDuration) * 100 : 100 / stages.length;
          const clampedWidth = Math.min(Math.max(widthPercent, 10), 100);
          const isActive = currentStage === stage.rawName || currentStage === stage.name;
          const stageClass = sanitizeClassName(stage.name);
          
          return (
            <div
              key={`${stage.name}-${stage.duration_ms}`}
              className={`stage-timeline-segment timeline-${stageClass} ${isActive ? 'stage-active' : ''} stage-${stage.status}`}
              style={{ width: `${clampedWidth}%` }}
              title={`${stage.name}: ${formatDuration(stage.duration_ms)}`}
            >
              <span className="stage-timeline-label">{stage.name}</span>
            </div>
          );
        })}
      </div>
      <div className="stage-timeline-legend">
        {stages.map((stage) => (
          <span key={`legend-${stage.name}`} className="stage-timeline-item">
            <span className={`stage-dot timeline-${sanitizeClassName(stage.name)}`} />
            {stage.name}: {formatDuration(stage.duration_ms)}
          </span>
        ))}
        <span className="stage-timeline-total">Total: {formatDuration(totalDuration)}</span>
      </div>
    </div>
  );
}

export function RunOverview() {
  const { runId } = useParams<{ runId: string }>();
  const navigate = useNavigate();
  const { showToast } = useToast();
  
  const [run, setRun] = useState<RunInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showStopDialog, setShowStopDialog] = useState(false);

  const metricsRunId = runId || '';
  const {
    dataPoints,
    isRunActive,
    currentRunState,
    currentStage,
    elapsedMs,
    latestTotals,
    stageMarkers,
  } = useMetricsData({ runId: metricsRunId, run: run || undefined });

  const latestMetrics: MetricsDataPoint | null = dataPoints.length > 0 
    ? dataPoints[dataPoints.length - 1] 
    : null;

  const loadRun = useCallback(async () => {
    if (!runId) return;
    
    setLoading(true);
    setError(null);
    
    try {
      const runData = await fetchRun(runId);
      setRun(runData);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load run');
    } finally {
      setLoading(false);
    }
  }, [runId]);

  useEffect(() => {
    loadRun();
  }, [loadRun]);

  const handleRunStopped = useCallback(() => {
    showToast('Run stop initiated', 'success');
    setShowStopDialog(false);
    loadRun();
  }, [showToast, loadRun]);

  const passFailResult = determinePassFailStatus(
    currentRunState || run?.state || '',
    run?.stop_reason,
    latestMetrics?.error_rate
  );

  const runState = currentRunState || run?.state || 'unknown';

  // Derive stages from stageMarkers (real data from SSE events)
  const sortedMarkers = [...stageMarkers].sort((a, b) => a.timestamp - b.timestamp);
  const derivedStages = sortedMarkers.map((marker, index, arr) => {
    const nextMarker = arr[index + 1];
    const isLast = index === arr.length - 1;
    const startedAtMs = run?.started_at ? new Date(run.started_at).getTime() : 0;
    const duration = nextMarker 
      ? nextMarker.timestamp - marker.timestamp
      : isRunActive ? elapsedMs - (marker.timestamp - startedAtMs) : 0;
    
    return {
      name: marker.label,
      rawName: marker.stage,
      duration_ms: Math.max(0, duration),
      status: isLast && isRunActive ? 'running' as const : 'completed' as const,
    };
  });

  // Use derived stages if available, otherwise show empty state
  const stages = derivedStages.length > 0 ? derivedStages : [];

  if (loading) {
    return (
      <div className="run-overview run-overview-loading">
        <div className="spinner" />
        <span>Loading run details...</span>
      </div>
    );
  }

  if (error || !run) {
    return (
      <div className="run-overview run-overview-error">
        <Icon name="alert-triangle" size="lg" />
        <span>{error || 'Run not found'}</span>
        <button className="btn btn-secondary" onClick={() => navigate('/')}>
          Back to Runs
        </button>
      </div>
    );
  }

  return (
    <div className="run-overview">
      <section className="run-overview-header">
        <div className="run-overview-header-main">
          <div className="run-overview-id">
            <span className="run-id-label">Run</span>
            <code className="run-id-value">{run.id}</code>
          </div>
          <div className="run-overview-badges">
            <span className={`run-state-badge state-${runState}`}>
              {runState}
            </span>
            <PassFailBadge status={passFailResult.status} reason={passFailResult.reason} />
          </div>
        </div>
        <div className="run-overview-timestamps">
          <div className="timestamp-item">
            <Icon name="clock" size="sm" />
            <span className="timestamp-label">Created</span>
            <span className="timestamp-value">{formatTimestamp(run.created_at)}</span>
          </div>
          {run.started_at && (
            <div className="timestamp-item">
              <Icon name="play" size="sm" />
              <span className="timestamp-label">Started</span>
              <span className="timestamp-value">{formatTimestamp(run.started_at)}</span>
            </div>
          )}
          <div className="timestamp-item">
            <Icon name="timer" size="sm" />
            <span className="timestamp-label">Duration</span>
            <span className="timestamp-value">{formatDuration(elapsedMs)}</span>
          </div>
        </div>
      </section>

      <section className="run-overview-summary">
        <div className="summary-row">
          <div className="summary-item">
            <span className="summary-label">Scenario</span>
            <code className="summary-value">{run.scenario_id || '—'}</code>
          </div>
          <div className="summary-item">
            <span className="summary-label">Current Stage</span>
            <span className="summary-value">{currentStage || '—'}</span>
          </div>
        </div>
      </section>

      <section className="run-overview-kpis">
        <h3 className="section-title">Key Metrics</h3>
        <div className="kpi-grid">
          <KpiCard 
            label="Throughput" 
            value={latestMetrics?.throughput?.toFixed(1) || '0'} 
            unit="ops/s"
          />
          <KpiCard 
            label="P95 Latency" 
            value={latestMetrics?.latency_p95_ms?.toFixed(0) || '0'} 
            unit="ms"
            highlight={latestMetrics && latestMetrics.latency_p95_ms > 500 ? 'warning' : undefined}
          />
          <KpiCard 
            label="Error Rate" 
            value={latestMetrics ? (latestMetrics.error_rate * 100).toFixed(1) : '0'} 
            unit="%"
            highlight={latestMetrics && latestMetrics.error_rate > 0.05 ? 'danger' : undefined}
          />
          <KpiCard 
            label="Total Operations" 
            value={latestTotals.total_ops.toLocaleString()} 
          />
        </div>
      </section>

      <section className="run-overview-thresholds">
        <h3 className="section-title">Stop Conditions</h3>
        <ThresholdStatus thresholds={[]} currentMetrics={latestMetrics} />
        <p className="section-note">Stop conditions are configured in the run wizard.</p>
      </section>

      <section className="run-overview-stages">
        <h3 className="section-title">Stage Timeline</h3>
        <StageTimeline stages={stages} currentStage={currentStage} />
      </section>

      {runId && (
        <ServerResourcesSection runId={runId} isRunActive={isRunActive} />
      )}

      <section className="run-overview-actions">
        <button 
          className="btn btn-secondary"
          onClick={() => navigate(`/runs/${runId}/logs`)}
        >
          <Icon name="list" size="sm" />
          View Logs
        </button>
        <button 
          className="btn btn-secondary"
          onClick={() => navigate(`/runs/${runId}/metrics`)}
        >
          <Icon name="chart-bar" size="sm" />
          View Metrics
        </button>
        <button 
          className="btn btn-secondary"
          onClick={() => navigate(`/compare?runA=${runId}`)}
        >
          <Icon name="scale" size="sm" />
          Compare
        </button>
        {isRunActive && (
          <button 
            className="btn btn-danger"
            onClick={() => setShowStopDialog(true)}
          >
            <Icon name="x" size="sm" />
            Stop Run
          </button>
        )}
      </section>

      {runId && (
        <StopRunDialog
          isOpen={showStopDialog}
          runId={runId}
          onClose={() => setShowStopDialog(false)}
          onStopped={handleRunStopped}
        />
      )}
    </div>
  );
}
