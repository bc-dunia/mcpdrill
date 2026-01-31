import { useCallback, useState } from 'react';
import type { MetricsDataPoint, MetricsSummary, StabilityMetrics, StopReason, OperationLog } from '../types';
import { Icon } from './Icon';
import { formatDuration, formatTime } from '../utils/formatting';
import { StopRunDialog } from './StopRunDialog';
import { useToast } from './Toast';

const API_BASE = '';

interface LogsResponse {
  logs: OperationLog[];
  total: number;
}

interface MetricsControlsProps {
  runId: string;
  isAutoRefresh: boolean;
  setIsAutoRefresh: (value: boolean) => void;
  isRunActive: boolean;
  loading: boolean;
  onManualRefresh: () => void;
  summary: MetricsSummary;
  stability: StabilityMetrics | null;
  dataPoints: MetricsDataPoint[];
}



interface MetricsRunStatusProps {
  elapsedMs: number;
  isRunActive: boolean;
  currentRunState?: string;
  onNavigateToWizard?: () => void;
  runId: string;
  onStopped: () => void;
}

interface MetricsStopReasonProps {
  stopReason: StopReason;
  onDismiss: () => void;
}

export function MetricsControls({
  runId,
  isAutoRefresh,
  setIsAutoRefresh,
  isRunActive,
  loading,
  onManualRefresh,
  summary,
  stability,
  dataPoints,
}: MetricsControlsProps) {
  const { showToast } = useToast();

  const handleDownloadMetrics = useCallback(() => {
    const exportData = {
      run_id: runId,
      exported_at: new Date().toISOString(),
      summary,
      stability: stability ? {
        stability_score: stability.stability_score,
        total_sessions: stability.total_sessions,
        active_sessions: stability.active_sessions,
        dropped_sessions: stability.dropped_sessions,
        avg_session_lifetime_ms: stability.avg_session_lifetime_ms,
      } : null,
      time_series: dataPoints.map(p => ({
        timestamp: p.timestamp,
        time: p.time,
        latency_p50_ms: p.latency_p50_ms,
        latency_p95_ms: p.latency_p95_ms,
        latency_p99_ms: p.latency_p99_ms,
        latency_mean: p.latency_mean,
        throughput: p.throughput,
        error_rate: p.error_rate,
        success_ops: p.success_ops,
        failed_ops: p.failed_ops,
      })),
    };
    
    const blob = new Blob([JSON.stringify(exportData, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${runId}-metrics-${Date.now()}.json`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  }, [runId, summary, stability, dataPoints]);

  const handleDownloadLogs = useCallback(async () => {
    const BATCH_SIZE = 1000;
    const MAX_BATCHES = 20;
    
    try {
      const allLogs: OperationLog[] = [];
      let offset = 0;
      let hasMore = true;
      let batchCount = 0;

      while (hasMore && batchCount < MAX_BATCHES) {
        const response = await fetch(`${API_BASE}/runs/${runId}/logs?limit=${BATCH_SIZE}&offset=${offset}`);
        if (!response.ok) throw new Error('Failed to fetch logs');
        const data: LogsResponse = await response.json();
        const logs = data.logs || [];
        
        allLogs.push(...logs);
        
        hasMore = logs.length === BATCH_SIZE;
        offset += logs.length;
        batchCount++;
      }
      
      if (allLogs.length === 0) {
        showToast('No logs available to download', 'warning');
        return;
      }

      const headers = ['timestamp', 'operation', 'tool_name', 'latency_ms', 'ok', 'error_type', 'error_code', 'session_id', 'vu_id'];
      const rows = allLogs.map((log) => headers.map(h => {
        const key = h === 'timestamp' ? 'timestamp_ms' : h;
        const val = log[key as keyof OperationLog];
        if (h === 'timestamp' && typeof val === 'number') return new Date(val).toISOString();
        if (val === null || val === undefined) return '';
        return String(val).replace(/"/g, '""');
      }).map(v => `"${v}"`).join(','));
      
      const csv = [headers.join(','), ...rows].join('\n');
      const blob = new Blob([csv], { type: 'text/csv;charset=utf-8' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `${runId}-logs-${Date.now()}.csv`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);

      if (batchCount >= MAX_BATCHES) {
        showToast(`Downloaded ${allLogs.length} logs (truncated)`, 'info');
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to download logs';
      showToast(message, 'error');
    }
  }, [runId, showToast]);

  return (
    <div className="dashboard-controls">
      <div className="auto-refresh-toggle">
        <input
          type="checkbox"
          id="auto-refresh-toggle"
          checked={isAutoRefresh}
          onChange={(e) => setIsAutoRefresh(e.target.checked)}
          disabled={!isRunActive}
          aria-describedby="auto-refresh-hint"
        />
        <label htmlFor="auto-refresh-toggle">
          <span className="toggle-slider" aria-hidden="true" />
          <span className="toggle-label">Auto-refresh</span>
        </label>
        <span id="auto-refresh-hint" className="sr-only">
          {isRunActive ? 'Automatically refresh metrics every 2 seconds' : 'Auto-refresh disabled when run is not active'}
        </span>
      </div>
      <button 
        className="btn btn-secondary btn-sm" 
        onClick={onManualRefresh}
        disabled={loading}
        aria-label={loading ? 'Loading metrics' : 'Refresh metrics'}
      >
        <Icon name={loading ? 'loader' : 'refresh'} size="sm" aria-hidden={true} /> Refresh
      </button>
      <button 
        className="btn btn-secondary btn-sm" 
        onClick={handleDownloadMetrics}
        disabled={dataPoints.length === 0}
        aria-label="Download metrics as JSON"
        title="Download metrics as JSON"
      >
        <Icon name="download" size="sm" aria-hidden={true} /> Metrics
      </button>
      <button 
        className="btn btn-secondary btn-sm" 
        onClick={handleDownloadLogs}
        aria-label="Download logs as CSV"
        title="Download logs as CSV"
      >
        <Icon name="download" size="sm" aria-hidden={true} /> Logs
      </button>
    </div>
  );
}

export function MetricsRunStatus({
  elapsedMs,
  isRunActive,
  currentRunState,
  onNavigateToWizard,
  runId,
  onStopped,
}: MetricsRunStatusProps) {
  const [showStopDialog, setShowStopDialog] = useState(false);

  const handleStopped = useCallback(() => {
    setShowStopDialog(false);
    onStopped();
  }, [onStopped]);

  return (
    <>
      <div className="run-progress-bar" role="timer" aria-label="Run duration">
        <div className="progress-time-display">
          <Icon name="clock" size="sm" aria-hidden={true} />
          <span className="progress-elapsed">{formatDuration(elapsedMs)}</span>
        </div>
        <div className="progress-track">
          <div 
            className={`progress-fill ${isRunActive ? 'progress-fill-active' : ''}`}
            style={{ width: '100%' }}
          />
        </div>
        {isRunActive ? (
          <span className="progress-status">
            <span className="progress-dot" aria-hidden="true" />
            Running
            <button 
              className="btn btn-danger btn-stop-run btn-stop-inline" 
              onClick={() => setShowStopDialog(true)}
              aria-label="Stop this test run"
            >
              <Icon name="x-circle" size="sm" aria-hidden={true} /> 
              Stop Run
            </button>
          </span>
        ) : (
          <span className="progress-status progress-status-completed">
            <span className={`status-dot-static status-${currentRunState}`} aria-hidden="true" />
            {currentRunState?.replace(/_/g, ' ')}
            {onNavigateToWizard && (
              <button 
                className="btn btn-primary btn-sm btn-new-run" 
                onClick={onNavigateToWizard}
                aria-label="Start a new test run"
              >
                <Icon name="plus" size="sm" aria-hidden={true} /> New Run
              </button>
            )}
          </span>
        )}
      </div>

      <StopRunDialog
        isOpen={showStopDialog}
        runId={runId}
        onClose={() => setShowStopDialog(false)}
        onStopped={handleStopped}
      />
    </>
  );
}

function parseStopReason(stopReason: StopReason): { title: string; description: string; isError: boolean } | null {
  const reason = stopReason.reason;
  
  if (reason.startsWith('stop_condition_triggered:')) {
    const conditionPart = reason.replace('stop_condition_triggered:', '').trim();
    const match = conditionPart.match(/(\w+)\s*([><=]+)\s*([\d.]+)\s*\(observed\s+([\d.]+)\)/);
    
    if (match) {
      const [, metric, , threshold, observed] = match;
      const thresholdNum = parseFloat(threshold);
      const observedNum = parseFloat(observed);
      
      if (metric === 'error_rate') {
        const thresholdPct = (thresholdNum * 100).toFixed(0);
        const observedPct = (observedNum * 100).toFixed(1);
        return {
          title: 'Test stopped automatically',
          description: `Error rate exceeded ${thresholdPct}% threshold (observed: ${observedPct}%)`,
          isError: true,
        };
      }
      
      if (metric.includes('latency')) {
        const metricLabel = metric.replace(/_/g, ' ').replace('ms', '').trim();
        return {
          title: 'Test stopped automatically',
          description: `${metricLabel} exceeded ${threshold}ms threshold (observed: ${observed}ms)`,
          isError: false,
        };
      }
    }
    
    return {
      title: 'Test stopped automatically',
      description: conditionPart,
      isError: reason.includes('error_rate'),
    };
  }
  
  if (reason.includes('user_requested') || stopReason.actor === 'user') {
    return {
      title: 'Test stopped by user',
      description: `Stop mode: ${stopReason.mode}`,
      isError: false,
    };
  }
  
  return {
    title: 'Test stopped',
    description: reason,
    isError: false,
  };
}

export function MetricsStopReason({ stopReason, onDismiss }: MetricsStopReasonProps) {
  const parsed = parseStopReason(stopReason);
  if (!parsed) return null;

  return (
    <div className={`stop-reason-alert ${parsed.isError ? 'stop-reason-error' : ''}`} role="alert">
      <Icon name="alert-triangle" size="md" className="stop-reason-icon" aria-hidden={true} />
      <div className="stop-reason-content">
        <p className="stop-reason-title">
          {parsed.title}
        </p>
        <p className="stop-reason-description">{parsed.description}</p>
        <div className="stop-reason-meta">
          <span>
            <Icon name="zap" size="sm" aria-hidden={true} />
            {['system', 'autoramp', 'scheduler', 'stop_condition'].includes(stopReason.actor) ? 'Automatic' : 'Manual'}
          </span>
          <span>
            <Icon name="clock" size="sm" aria-hidden={true} />
            {formatTime(stopReason.at_ms)}
          </span>
        </div>
      </div>
      <button 
        type="button"
        className="stop-reason-dismiss"
        onClick={onDismiss}
        aria-label="Dismiss stop reason notification"
      >
        <Icon name="x" size="sm" aria-hidden={true} />
      </button>
    </div>
  );
}
