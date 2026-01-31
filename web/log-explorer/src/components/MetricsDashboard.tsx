import { useState, useEffect, useCallback, useRef, useMemo } from 'react';
import type { RunInfo, LiveMetrics, MetricsDataPoint, MetricsSummary, StabilityMetrics, StopReason } from '../types';
import { fetchRun, stopRun, emergencyStopRun, subscribeToRunEvents, type StopMode, type RunEvent } from '../api';
import { LatencyChart } from './LatencyChart';
import { ThroughputChart } from './ThroughputChart';
import { ErrorRateChart } from './ErrorRateChart';
import { ConnectionStabilityChart } from './ConnectionStabilityChart';
import { SessionLifecycleTable } from './SessionLifecycleTable';
import { ServerResourcesSection } from './ServerResourcesSection';
import { StabilityEventsTimeline } from './StabilityEventsTimeline';
import { ToolMetricsDashboard } from './ToolMetricsDashboard';
import { Icon } from './Icon';

interface MetricsDashboardProps {
  runId: string;
  run?: RunInfo;
  onNavigateToWizard?: () => void;
}

const API_BASE = '';
const REFRESH_INTERVAL = 2000;
const MAX_DATA_POINTS = 60;
const MANUAL_REFRESH_DEBOUNCE_MS = 500;
const STORAGE_KEY_PREFIX = 'mcpdrill_metrics_';
const MAX_CACHED_RUNS = 10;

interface CachedMetricsData {
  dataPoints: MetricsDataPoint[];
  durationMs?: number;
  stability: StabilityMetrics | null;
  lastUpdated: number;
}

function saveMetricsToStorage(runId: string, data: CachedMetricsData): void {
  try {
    const keys = Object.keys(localStorage).filter(k => k.startsWith(STORAGE_KEY_PREFIX));
    if (keys.length >= MAX_CACHED_RUNS) {
      const oldestKey = keys
        .map(k => ({ key: k, time: JSON.parse(localStorage.getItem(k) || '{}').lastUpdated || 0 }))
        .sort((a, b) => a.time - b.time)[0]?.key;
      if (oldestKey) localStorage.removeItem(oldestKey);
    }
    localStorage.setItem(`${STORAGE_KEY_PREFIX}${runId}`, JSON.stringify(data));
  } catch (err) {
    console.warn('Failed to save metrics to localStorage:', err);
  }
}

function loadMetricsFromStorage(runId: string): CachedMetricsData | null {
  try {
    const stored = localStorage.getItem(`${STORAGE_KEY_PREFIX}${runId}`);
    if (stored) return JSON.parse(stored);
  } catch (err) {
    console.warn('Failed to load metrics from localStorage:', err);
  }
  return null;
}

async function fetchMetrics(runId: string, includeTimeSeries = false): Promise<LiveMetrics> {
  const url = includeTimeSeries 
    ? `${API_BASE}/runs/${runId}/metrics?include_time_series=true`
    : `${API_BASE}/runs/${runId}/metrics`;
  const response = await fetch(url);
  if (!response.ok) {
    throw new Error(`Failed to fetch metrics: ${response.statusText}`);
  }
  return response.json();
}

function convertTimeSeriestoDataPoints(timeSeries: LiveMetrics['time_series']): MetricsDataPoint[] {
  if (!timeSeries || timeSeries.length === 0) return [];
  return timeSeries.map(point => ({
    timestamp: point.timestamp,
    time: formatTime(point.timestamp),
    throughput: point.throughput,
    latency_p50_ms: point.latency_p50,
    latency_p95_ms: point.latency_p95,
    latency_p99_ms: point.latency_p99,
    latency_mean: point.latency_mean,
    error_rate: point.error_rate,
    success_ops: point.success_ops,
    failed_ops: point.failed_ops,
  }));
}

async function fetchStability(runId: string): Promise<StabilityMetrics> {
  const response = await fetch(`${API_BASE}/runs/${runId}/stability?include_events=true&include_time_series=true`);
  if (!response.ok) {
    throw new Error(`Failed to fetch stability: ${response.statusText}`);
  }
  return response.json();
}

function formatTime(timestamp: number | undefined | null): string {
  if (timestamp === undefined || timestamp === null || isNaN(timestamp)) return '—';
  const date = new Date(timestamp);
  if (isNaN(date.getTime())) return '—';
  return date.toLocaleTimeString('en-US', { 
    hour12: false, 
    hour: '2-digit', 
    minute: '2-digit', 
    second: '2-digit' 
  });
}

function formatDuration(ms: number | undefined | null): string {
  if (ms === undefined || ms === null || isNaN(ms)) return '—';
  const seconds = Math.floor(ms / 1000);
  const minutes = Math.floor(seconds / 60);
  const hours = Math.floor(minutes / 60);
  
  if (hours > 0) {
    return `${hours}h ${minutes % 60}m ${seconds % 60}s`;
  }
  if (minutes > 0) {
    return `${minutes}m ${seconds % 60}s`;
  }
  return `${seconds}s`;
}

function parseStopReason(stopReason: StopReason | undefined): { title: string; description: string; isError: boolean } | null {
  if (!stopReason) return null;
  
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
  const [dataPoints, setDataPoints] = useState<MetricsDataPoint[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [isAutoRefresh, setIsAutoRefresh] = useState(true);
  const [durationMs, setDurationMs] = useState<number | undefined>(undefined);
  const [stability, setStability] = useState<StabilityMetrics | null>(null);
  const [stabilityLoading, setStabilityLoading] = useState(true);
  const [isStopping, setIsStopping] = useState(false);
  const [showStopConfirm, setShowStopConfirm] = useState(false);
  const [selectedStopMode, setSelectedStopMode] = useState<StopMode | 'emergency'>('drain');
  const [currentRunState, setCurrentRunState] = useState<string | undefined>(run?.state);
  const [currentRunInfo, setCurrentRunInfo] = useState<RunInfo | undefined>(run);
  const [sseConnected, setSseConnected] = useState(false);
  const [currentStage, setCurrentStage] = useState<string | null>(null);
  const [activeMetricsTab, setActiveMetricsTab] = useState<'overview' | 'tools'>('overview');
  const [stopReasonDismissed, setStopReasonDismissed] = useState(false);
  const [elapsedMs, setElapsedMs] = useState<number>(0);
  const intervalRef = useRef<number | null>(null);
  const elapsedIntervalRef = useRef<number | null>(null);
  const isLoadingRef = useRef(false);
  const debounceTimeoutRef = useRef<number | null>(null);
  const sseCleanupRef = useRef<(() => void) | null>(null);

  const isRunActive = currentRunState != null && !['completed', 'failed', 'aborted', 'stopping'].includes(currentRunState);

  useEffect(() => {
    setCurrentRunState(run?.state);
    if (run) setCurrentRunInfo(run);
  }, [run]);

  const loadMetrics = useCallback(async () => {
    if (isLoadingRef.current) return;
    isLoadingRef.current = true;
    setLoading(true);
    
    try {
      // For completed runs, request full time series; for active runs, just current snapshot
      const includeTimeSeries = !isRunActive;
      const metrics = await fetchMetrics(runId, includeTimeSeries);
      
      // If we got time series data, use it directly (for completed runs)
      if (metrics.time_series && metrics.time_series.length > 0) {
        setDataPoints(convertTimeSeriestoDataPoints(metrics.time_series));
      } else {
        // Polling mode for active runs - append single point
        const timestamp = metrics.timestamp || Date.now();
        
        const newPoint: MetricsDataPoint = {
          timestamp,
          time: formatTime(timestamp),
          throughput: metrics.throughput,
          latency_p50_ms: metrics.latency_p50_ms,
          latency_p95_ms: metrics.latency_p95_ms,
          latency_p99_ms: metrics.latency_p99_ms,
          latency_mean: metrics.latency_mean || (metrics.latency_p50_ms + metrics.latency_p95_ms) / 2,
          error_rate: metrics.error_rate,
          success_ops: metrics.success_ops || (metrics.total_ops - metrics.failed_ops),
          failed_ops: metrics.failed_ops,
        };

        setDataPoints(prev => {
          const updated = [...prev, newPoint];
          return updated.slice(-MAX_DATA_POINTS);
        });
      }
      
      if (metrics.duration_ms !== undefined) {
        setDurationMs(metrics.duration_ms);
      }
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load metrics');
    } finally {
      setLoading(false);
      isLoadingRef.current = false;
    }
  }, [runId, isRunActive]);

  const loadRunState = useCallback(async () => {
    try {
      const runInfo = await fetchRun(runId);
      setCurrentRunState(runInfo.state);
      setCurrentRunInfo(runInfo);
    } catch (err) {
      console.warn('Failed to load run state:', err);
    }
  }, [runId]);

  const loadStability = useCallback(async () => {
    setStabilityLoading(true);
    try {
      const data = await fetchStability(runId);
      setStability(data);
    } catch {
      setStability(null);
    } finally {
      setStabilityLoading(false);
    }
  }, [runId]);

  const handleManualRefresh = useCallback(() => {
    if (debounceTimeoutRef.current) {
      clearTimeout(debounceTimeoutRef.current);
    }
    debounceTimeoutRef.current = window.setTimeout(() => {
      loadMetrics();
      loadStability();
      debounceTimeoutRef.current = null;
    }, MANUAL_REFRESH_DEBOUNCE_MS);
  }, [loadMetrics, loadStability]);

  useEffect(() => {
    const cached = loadMetricsFromStorage(runId);
    if (cached && cached.dataPoints.length > 0) {
      setDataPoints(cached.dataPoints);
      setDurationMs(cached.durationMs);
      setStability(cached.stability);
      setLoading(false);
      setStabilityLoading(false);
    } else {
      setDataPoints([]);
      setDurationMs(undefined);
      setLoading(true);
    }
    isLoadingRef.current = false;
    loadMetrics();
  }, [runId, loadMetrics]);

  useEffect(() => {
    if (intervalRef.current) {
      clearInterval(intervalRef.current);
      intervalRef.current = null;
    }

    if (isAutoRefresh && isRunActive) {
      intervalRef.current = window.setInterval(() => loadMetrics(), REFRESH_INTERVAL);
    }

    return () => {
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
        intervalRef.current = null;
      }
      if (debounceTimeoutRef.current) {
        clearTimeout(debounceTimeoutRef.current);
        debounceTimeoutRef.current = null;
      }
    };
  }, [isAutoRefresh, isRunActive, loadMetrics]);

  useEffect(() => {
    loadStability();
  }, [runId, loadStability]);

  useEffect(() => {
    if (isAutoRefresh && isRunActive) {
      const stabilityInterval = window.setInterval(() => loadStability(), REFRESH_INTERVAL);
      return () => clearInterval(stabilityInterval);
    }
  }, [isAutoRefresh, isRunActive, loadStability]);

  useEffect(() => {
    if (isAutoRefresh && isRunActive) {
      const runStateInterval = window.setInterval(() => loadRunState(), REFRESH_INTERVAL);
      return () => clearInterval(runStateInterval);
    }
  }, [isAutoRefresh, isRunActive, loadRunState]);

  useEffect(() => {
    if (elapsedIntervalRef.current) {
      clearInterval(elapsedIntervalRef.current);
      elapsedIntervalRef.current = null;
    }

    const startedAt = currentRunInfo?.started_at || run?.started_at;
    if (isRunActive && startedAt) {
      const startTime = new Date(startedAt).getTime();
      const updateElapsed = () => {
        setElapsedMs(Date.now() - startTime);
      };
      updateElapsed();
      elapsedIntervalRef.current = window.setInterval(updateElapsed, 1000);
    } else if (!isRunActive && durationMs !== undefined) {
      setElapsedMs(durationMs);
    }

    return () => {
      if (elapsedIntervalRef.current) {
        clearInterval(elapsedIntervalRef.current);
        elapsedIntervalRef.current = null;
      }
    };
  }, [isRunActive, currentRunInfo?.started_at, run?.started_at, durationMs]);

  const handleSseEvent = useCallback((event: RunEvent) => {
    switch (event.type) {
      case 'stage_started':
        setCurrentStage(event.data.stage as string || null);
        break;
      case 'stage_completed':
        loadMetrics();
        loadStability();
        break;
      case 'run_completed':
        setCurrentRunState(event.data.state as string || 'completed');
        loadMetrics();
        loadStability();
        break;
      case 'metrics':
        if (event.data) {
          const timestamp = (event.data.timestamp as number) || Date.now();
          const newPoint: MetricsDataPoint = {
            timestamp,
            time: formatTime(timestamp),
            throughput: (event.data.throughput as number) || 0,
            latency_p50_ms: (event.data.latency_p50_ms as number) || 0,
            latency_p95_ms: (event.data.latency_p95_ms as number) || 0,
            latency_p99_ms: (event.data.latency_p99_ms as number) || 0,
            latency_mean: (event.data.latency_mean as number) || 0,
            error_rate: (event.data.error_rate as number) || 0,
            success_ops: (event.data.success_ops as number) || 0,
            failed_ops: (event.data.failed_ops as number) || 0,
          };
          setDataPoints(prev => {
            const updated = [...prev, newPoint];
            return updated.slice(-MAX_DATA_POINTS);
          });
          if (event.data.duration_ms !== undefined) {
            setDurationMs(event.data.duration_ms as number);
          }
        }
        break;
    }
  }, [loadMetrics, loadStability]);

  const handleSseError = useCallback(() => {
    setSseConnected(false);
  }, []);

  useEffect(() => {
    if (!isRunActive || !isAutoRefresh) {
      if (sseCleanupRef.current) {
        sseCleanupRef.current();
        sseCleanupRef.current = null;
      }
      setSseConnected(false);
      return;
    }

    try {
      sseCleanupRef.current = subscribeToRunEvents(
        runId,
        handleSseEvent,
        handleSseError
      );
      setSseConnected(true);
    } catch {
      setSseConnected(false);
    }

    return () => {
      if (sseCleanupRef.current) {
        sseCleanupRef.current();
        sseCleanupRef.current = null;
      }
    };
  }, [runId, isRunActive, isAutoRefresh, handleSseEvent, handleSseError]);

  useEffect(() => {
    if (dataPoints.length > 0) {
      saveMetricsToStorage(runId, {
        dataPoints,
        durationMs,
        stability,
        lastUpdated: Date.now(),
      });
    }
  }, [runId, dataPoints, durationMs, stability]);

  const summary = useMemo(() => calculateSummary(dataPoints, durationMs), [dataPoints, durationMs]);

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
      const allLogs: Record<string, unknown>[] = [];
      let offset = 0;
      let hasMore = true;
      let batchCount = 0;

      while (hasMore && batchCount < MAX_BATCHES) {
        const response = await fetch(`${API_BASE}/runs/${runId}/logs?limit=${BATCH_SIZE}&offset=${offset}`);
        if (!response.ok) throw new Error('Failed to fetch logs');
        const data = await response.json();
        const logs = data.logs || [];
        
        allLogs.push(...logs);
        
        hasMore = logs.length === BATCH_SIZE;
        offset += logs.length;
        batchCount++;
      }
      
      if (allLogs.length === 0) {
        alert('No logs available to download');
        return;
      }

      const headers = ['timestamp', 'operation', 'tool_name', 'latency_ms', 'ok', 'error_type', 'error_code', 'session_id', 'vu_id'];
      const rows = allLogs.map((log: Record<string, unknown>) => headers.map(h => {
        const val = log[h === 'timestamp' ? 'timestamp_ms' : h];
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
    } catch (err) {
      console.error('Failed to download logs:', err);
      alert('Failed to download logs');
    }
  }, [runId]);

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
        <div className="metrics-tabs" role="tablist" aria-label="Metrics view">
          <button
            role="tab"
            aria-selected={activeMetricsTab === 'overview'}
            aria-controls="metrics-overview-panel"
            className={`metrics-tab ${activeMetricsTab === 'overview' ? 'active' : ''}`}
            onClick={() => setActiveMetricsTab('overview')}
          >
            <Icon name="chart-bar" size="sm" aria-hidden={true} /> Overview
          </button>
          <button
            role="tab"
            aria-selected={activeMetricsTab === 'tools'}
            aria-controls="metrics-tools-panel"
            className={`metrics-tab ${activeMetricsTab === 'tools' ? 'active' : ''}`}
            onClick={() => setActiveMetricsTab('tools')}
          >
            <Icon name="tool" size="sm" aria-hidden={true} /> By Tool
          </button>
        </div>
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
            onClick={handleManualRefresh}
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
      </div>

      {showStopConfirm && (
        <div className="stop-confirm-overlay" role="dialog" aria-modal="true" aria-labelledby="stop-confirm-title">
          <div className="stop-confirm-dialog">
            <h3 id="stop-confirm-title">
              <Icon name="alert-triangle" size="lg" aria-hidden={true} /> Stop Test Run?
            </h3>
            <p>Choose how to stop this test run:</p>
            
            <div className="stop-mode-options" role="radiogroup" aria-label="Stop mode">
              <label className={`stop-mode-option ${selectedStopMode === 'drain' ? 'selected' : ''}`}>
                <input
                  type="radio"
                  name="stopMode"
                  value="drain"
                  checked={selectedStopMode === 'drain'}
                  onChange={() => setSelectedStopMode('drain')}
                  disabled={isStopping}
                />
                <div className="stop-mode-content">
                  <span className="stop-mode-title">
                    <Icon name="clock" size="sm" aria-hidden={true} /> Graceful Stop (Drain)
                  </span>
                  <span className="stop-mode-description">
                    Wait for in-flight operations to complete. Recommended for accurate results.
                  </span>
                </div>
              </label>
              
              <label className={`stop-mode-option ${selectedStopMode === 'immediate' ? 'selected' : ''}`}>
                <input
                  type="radio"
                  name="stopMode"
                  value="immediate"
                  checked={selectedStopMode === 'immediate'}
                  onChange={() => setSelectedStopMode('immediate')}
                  disabled={isStopping}
                />
                <div className="stop-mode-content">
                  <span className="stop-mode-title">
                    <Icon name="x" size="sm" aria-hidden={true} /> Immediate Stop
                  </span>
                  <span className="stop-mode-description">
                    Cancel all operations immediately. In-flight requests will be aborted.
                  </span>
                </div>
              </label>
              
              <label className={`stop-mode-option stop-mode-emergency ${selectedStopMode === 'emergency' ? 'selected' : ''}`}>
                <input
                  type="radio"
                  name="stopMode"
                  value="emergency"
                  checked={selectedStopMode === 'emergency'}
                  onChange={() => setSelectedStopMode('emergency')}
                  disabled={isStopping}
                />
                <div className="stop-mode-content">
                  <span className="stop-mode-title">
                    <Icon name="alert-triangle" size="sm" aria-hidden={true} /> Emergency Stop
                  </span>
                  <span className="stop-mode-description">
                    Force terminate immediately. Use only if other methods fail.
                  </span>
                </div>
              </label>
            </div>
            
            <div className="stop-confirm-actions">
              <button 
                className="btn btn-secondary" 
                onClick={() => setShowStopConfirm(false)}
                disabled={isStopping}
              >
                Cancel
              </button>
              <button 
                className={`btn ${selectedStopMode === 'emergency' ? 'btn-danger' : 'btn-warning'}`}
                onClick={handleStopRun}
                disabled={isStopping}
              >
                {isStopping ? (
                  <><Icon name="loader" size="sm" aria-hidden={true} /> Stopping...</>
                ) : (
                  <><Icon name="x-circle" size="sm" aria-hidden={true} /> {
                    selectedStopMode === 'drain' ? 'Stop (Drain)' :
                    selectedStopMode === 'immediate' ? 'Stop (Immediate)' :
                    'Emergency Stop'
                  }</>
                )}
              </button>
            </div>
          </div>
        </div>
      )}

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
              onClick={() => setShowStopConfirm(true)}
              disabled={isStopping}
              aria-label="Stop this test run"
            >
              <Icon name={isStopping ? 'loader' : 'x'} size="sm" aria-hidden={true} /> 
              {isStopping ? 'Stopping...' : 'Stop'}
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

      {!isRunActive && currentRunInfo?.stop_reason && !stopReasonDismissed && (() => {
        const parsed = parseStopReason(currentRunInfo.stop_reason);
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
                  {currentRunInfo.stop_reason.actor === 'system' ? 'Automatic' : 'Manual'}
                </span>
                <span>
                  <Icon name="clock" size="sm" aria-hidden={true} />
                  {formatTime(currentRunInfo.stop_reason.at_ms)}
                </span>
              </div>
            </div>
            <button 
              type="button"
              className="stop-reason-dismiss"
              onClick={() => setStopReasonDismissed(true)}
              aria-label="Dismiss stop reason notification"
            >
              <Icon name="x" size="sm" aria-hidden={true} />
            </button>
          </div>
        );
      })()}

      {activeMetricsTab === 'overview' && (
        <div id="metrics-overview-panel" role="tabpanel" aria-labelledby="metrics-overview-tab">
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

      {activeMetricsTab === 'tools' && (
        <div id="metrics-tools-panel" role="tabpanel" aria-labelledby="metrics-tools-tab">
          <ToolMetricsDashboard runId={runId} />
        </div>
      )}
    </section>
  );
}
