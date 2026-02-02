import { useState, useEffect, useCallback, useRef } from 'react';
import type { RunInfo, LiveMetrics, MetricsDataPoint, StabilityMetrics, StageMarker } from '../types';
import { fetchRun, subscribeToRunEvents, type RunEvent } from '../api/index';
import { formatTime } from '../utils/formatting';
import { saveWithEviction, loadFromLocalStorage } from '../utils/storage';
import { CONFIG, STORAGE_KEYS } from '../config';

const API_BASE = '';

function toSafeNumber(value: unknown, fallback: number): number {
  if (typeof value === 'number' && !isNaN(value) && isFinite(value)) {
    return value;
  }
  if (typeof value === 'string') {
    const parsed = parseFloat(value);
    if (!isNaN(parsed) && isFinite(parsed)) {
      return parsed;
    }
  }
  return fallback;
}

function getProperty(obj: object, key: string): unknown {
  return (obj as Record<string, unknown>)[key];
}

interface CachedTotals {
  total_ops: number;
  success_ops: number;
  failed_ops: number;
}

interface CachedMetricsData {
  dataPoints: MetricsDataPoint[];
  durationMs?: number;
  stability: StabilityMetrics | null;
  latestTotals?: CachedTotals;
  stageMarkers?: StageMarker[];
  lastUpdated: number;
}

function saveMetricsToStorage(runId: string, data: CachedMetricsData): void {
  saveWithEviction(STORAGE_KEYS.METRICS_PREFIX, runId, data, CONFIG.MAX_CACHED_RUNS);
}

function loadMetricsFromStorage(runId: string): CachedMetricsData | null {
  return loadFromLocalStorage<CachedMetricsData>(`${STORAGE_KEYS.METRICS_PREFIX}${runId}`);
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

interface UseMetricsDataOptions {
  runId: string;
  run?: RunInfo;
}

export interface LatestTotals {
  total_ops: number;
  success_ops: number;
  failed_ops: number;
}

interface UseMetricsDataResult {
  dataPoints: MetricsDataPoint[];
  loading: boolean;
  error: string | null;
  isAutoRefresh: boolean;
  setIsAutoRefresh: (value: boolean) => void;
  durationMs: number | undefined;
  stability: StabilityMetrics | null;
  stabilityLoading: boolean;
  currentRunState: string | undefined;
  currentRunInfo: RunInfo | undefined;
  sseConnected: boolean;
  currentStage: string | null;
  isRunActive: boolean;
  elapsedMs: number;
  latestTotals: LatestTotals;
  stageMarkers: StageMarker[];
  handleManualRefresh: () => void;
  loadMetrics: () => Promise<void>;
  loadStability: () => Promise<void>;
  loadRunState: () => Promise<void>;
}

const TERMINAL_STATES = ['completed', 'failed', 'stopped', 'aborted'];

export function useMetricsData({ runId, run }: UseMetricsDataOptions): UseMetricsDataResult {
  const [dataPoints, setDataPoints] = useState<MetricsDataPoint[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [isAutoRefresh, setIsAutoRefresh] = useState(true);
  const [durationMs, setDurationMs] = useState<number | undefined>(undefined);
  const [stability, setStability] = useState<StabilityMetrics | null>(null);
  const [stabilityLoading, setStabilityLoading] = useState(true);
  const [currentRunState, setCurrentRunState] = useState<string | undefined>(run?.state);
  const [currentRunInfo, setCurrentRunInfo] = useState<RunInfo | undefined>(run);
  const [sseConnected, setSseConnected] = useState(false);
  const [currentStage, setCurrentStage] = useState<string | null>(null);
  const [elapsedMs, setElapsedMs] = useState<number>(0);
  const [latestTotals, setLatestTotals] = useState<LatestTotals>({ total_ops: 0, success_ops: 0, failed_ops: 0 });
  const [stageMarkers, setStageMarkers] = useState<StageMarker[]>([]);

  const intervalRef = useRef<number | null>(null);
  const fallbackIntervalRef = useRef<number | null>(null);
  const elapsedIntervalRef = useRef<number | null>(null);
  const isLoadingRef = useRef(false);
  const debounceTimeoutRef = useRef<number | null>(null);
  const sseCleanupRef = useRef<(() => void) | null>(null);
  const requestIdRef = useRef(0);
  const stabilityRequestIdRef = useRef(0);
  const runStateRequestIdRef = useRef(0);
  const currentRunIdRef = useRef(runId);
  const isFirstRenderAfterRunChangeRef = useRef(false);
  const prevRunStateRef = useRef<string | undefined>(currentRunState);
  const lastSseEventRef = useRef<number>(Date.now());

  const isRunActive = currentRunState != null && !['completed', 'failed', 'aborted', 'stopping', 'stopped'].includes(currentRunState);

  useEffect(() => {
    setCurrentRunState(run?.state);
    if (run) setCurrentRunInfo(run);
  }, [run]);

  const loadMetricsWithTimeSeries = useCallback(async (forceTimeSeries: boolean) => {
    if (isLoadingRef.current) return;
    isLoadingRef.current = true;
    setLoading(true);
    
    const thisRequestId = ++requestIdRef.current;
    const thisRunId = runId;
    
    try {
      const metrics = await fetchMetrics(thisRunId, forceTimeSeries);
      
      if (currentRunIdRef.current !== thisRunId || requestIdRef.current !== thisRequestId) {
        return;
      }
      
      const successOps = metrics.success_ops ?? (metrics.total_ops - metrics.failed_ops);
      setLatestTotals({
        total_ops: metrics.total_ops,
        success_ops: successOps,
        failed_ops: metrics.failed_ops,
      });
      
      if (metrics.time_series && metrics.time_series.length > 0) {
        setDataPoints(convertTimeSeriestoDataPoints(metrics.time_series));
      } else {
        const timestamp = metrics.timestamp || Date.now();
        
        const newPoint: MetricsDataPoint = {
          timestamp,
          time: formatTime(timestamp),
          throughput: metrics.throughput,
          latency_p50_ms: metrics.latency_p50_ms,
          latency_p95_ms: metrics.latency_p95_ms,
          latency_p99_ms: metrics.latency_p99_ms,
          latency_mean: metrics.latency_mean ?? (metrics.latency_p50_ms + metrics.latency_p95_ms) / 2,
          error_rate: metrics.error_rate,
          success_ops: successOps,
          failed_ops: metrics.failed_ops,
        };

        setDataPoints(prev => {
          const updated = [...prev, newPoint];
          return updated.slice(-CONFIG.MAX_DATA_POINTS);
        });
      }
      
      if (metrics.duration_ms !== undefined) {
        setDurationMs(metrics.duration_ms);
      }
      setError(null);
    } catch (err) {
      if (currentRunIdRef.current === thisRunId) {
        setError(err instanceof Error ? err.message : 'Failed to load metrics');
      }
    } finally {
      if (currentRunIdRef.current === thisRunId && requestIdRef.current === thisRequestId) {
        setLoading(false);
        isLoadingRef.current = false;
      }
    }
  }, [runId]);

  const loadMetrics = useCallback(async () => {
    if (isLoadingRef.current) return;
    isLoadingRef.current = true;
    setLoading(true);
    
    const thisRequestId = ++requestIdRef.current;
    const thisRunId = runId;
    
    try {
      const includeTimeSeries = !isRunActive;
      const metrics = await fetchMetrics(thisRunId, includeTimeSeries);
      
      if (currentRunIdRef.current !== thisRunId || requestIdRef.current !== thisRequestId) {
        return;
      }
      
      const successOps = metrics.success_ops ?? (metrics.total_ops - metrics.failed_ops);
      setLatestTotals({
        total_ops: metrics.total_ops,
        success_ops: successOps,
        failed_ops: metrics.failed_ops,
      });
      
      if (metrics.time_series && metrics.time_series.length > 0) {
        setDataPoints(convertTimeSeriestoDataPoints(metrics.time_series));
      } else {
        const timestamp = metrics.timestamp || Date.now();
        
        const newPoint: MetricsDataPoint = {
          timestamp,
          time: formatTime(timestamp),
          throughput: metrics.throughput,
          latency_p50_ms: metrics.latency_p50_ms,
          latency_p95_ms: metrics.latency_p95_ms,
          latency_p99_ms: metrics.latency_p99_ms,
          latency_mean: metrics.latency_mean ?? (metrics.latency_p50_ms + metrics.latency_p95_ms) / 2,
          error_rate: metrics.error_rate,
          success_ops: successOps,
          failed_ops: metrics.failed_ops,
        };

        setDataPoints(prev => {
          const updated = [...prev, newPoint];
          return updated.slice(-CONFIG.MAX_DATA_POINTS);
        });
      }
      
      if (metrics.duration_ms !== undefined) {
        setDurationMs(metrics.duration_ms);
      }
      setError(null);
    } catch (err) {
      if (currentRunIdRef.current === thisRunId) {
        setError(err instanceof Error ? err.message : 'Failed to load metrics');
      }
    } finally {
      if (currentRunIdRef.current === thisRunId && requestIdRef.current === thisRequestId) {
        setLoading(false);
        isLoadingRef.current = false;
      }
    }
  }, [runId, isRunActive]);

  const loadRunState = useCallback(async () => {
    const thisRunId = runId;
    const thisRequestId = ++runStateRequestIdRef.current;
    try {
      const runInfo = await fetchRun(thisRunId);
      if (currentRunIdRef.current !== thisRunId || runStateRequestIdRef.current !== thisRequestId) return;
      setCurrentRunState(runInfo.state);
      setCurrentRunInfo(runInfo);
    } catch (err) {
      if (currentRunIdRef.current === thisRunId && runStateRequestIdRef.current === thisRequestId) {
        console.warn('Failed to load run state:', err);
      }
    }
  }, [runId]);

  const loadStability = useCallback(async () => {
    const thisRunId = runId;
    const thisRequestId = ++stabilityRequestIdRef.current;
    setStabilityLoading(true);
    try {
      const data = await fetchStability(thisRunId);
      if (currentRunIdRef.current !== thisRunId || stabilityRequestIdRef.current !== thisRequestId) return;
      setStability(data);
    } catch {
      if (currentRunIdRef.current === thisRunId && stabilityRequestIdRef.current === thisRequestId) {
        setStability(null);
      }
    } finally {
      if (currentRunIdRef.current === thisRunId && stabilityRequestIdRef.current === thisRequestId) {
        setStabilityLoading(false);
      }
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
    }, CONFIG.DEBOUNCE_MS);
  }, [loadMetrics, loadStability]);

  useEffect(() => {
    currentRunIdRef.current = runId;
    isFirstRenderAfterRunChangeRef.current = true;
    
    setError(null);
    setCurrentStage(null);
    setStageMarkers([]);
    
    const cached = loadMetricsFromStorage(runId);
    if (cached && cached.dataPoints.length > 0) {
      setDataPoints(cached.dataPoints);
      setDurationMs(cached.durationMs);
      setStability(cached.stability);
      if (cached.latestTotals) {
        setLatestTotals(cached.latestTotals);
      } else if (cached.dataPoints.length > 0) {
        const lastPoint = cached.dataPoints[cached.dataPoints.length - 1];
        setLatestTotals({
          total_ops: lastPoint.success_ops + lastPoint.failed_ops,
          success_ops: lastPoint.success_ops,
          failed_ops: lastPoint.failed_ops,
        });
      }
      if (cached.stageMarkers) {
        setStageMarkers(cached.stageMarkers);
      }
      setLoading(false);
      setStabilityLoading(false);
    } else {
      setDataPoints([]);
      setDurationMs(undefined);
      setLatestTotals({ total_ops: 0, success_ops: 0, failed_ops: 0 });
      setStability(null);
      setLoading(true);
      setStabilityLoading(true);
    }
    isLoadingRef.current = false;
    loadMetrics();
  }, [runId, loadMetrics]);

  useEffect(() => {
    if (intervalRef.current) {
      clearInterval(intervalRef.current);
      intervalRef.current = null;
    }
    if (fallbackIntervalRef.current) {
      clearInterval(fallbackIntervalRef.current);
      fallbackIntervalRef.current = null;
    }

    if (isAutoRefresh && isRunActive) {
      if (!sseConnected) {
        intervalRef.current = window.setInterval(() => loadMetrics(), CONFIG.REFRESH_INTERVALS.METRICS);
      } else {
        const FALLBACK_INTERVAL = 5000;
        const SSE_STALE_THRESHOLD = 5000;
        fallbackIntervalRef.current = window.setInterval(() => {
          const timeSinceLastEvent = Date.now() - lastSseEventRef.current;
          if (timeSinceLastEvent > SSE_STALE_THRESHOLD) {
            loadMetrics();
          }
        }, FALLBACK_INTERVAL);
      }
    }

    return () => {
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
        intervalRef.current = null;
      }
      if (fallbackIntervalRef.current) {
        clearInterval(fallbackIntervalRef.current);
        fallbackIntervalRef.current = null;
      }
      if (debounceTimeoutRef.current) {
        clearTimeout(debounceTimeoutRef.current);
        debounceTimeoutRef.current = null;
      }
    };
  }, [isAutoRefresh, isRunActive, sseConnected, loadMetrics]);

  useEffect(() => {
    loadStability();
  }, [runId, loadStability]);

  useEffect(() => {
    loadRunState();
  }, [runId, loadRunState]);

  useEffect(() => {
    if (isAutoRefresh && isRunActive) {
      const stabilityInterval = window.setInterval(() => loadStability(), CONFIG.REFRESH_INTERVALS.METRICS);
      return () => clearInterval(stabilityInterval);
    }
  }, [isAutoRefresh, isRunActive, loadStability]);

  useEffect(() => {
    if (isAutoRefresh && isRunActive) {
      const runStateInterval = window.setInterval(() => loadRunState(), CONFIG.REFRESH_INTERVALS.METRICS);
      return () => clearInterval(runStateInterval);
    }
  }, [isAutoRefresh, isRunActive, loadRunState]);

  useEffect(() => {
    const prevState = prevRunStateRef.current;
    const wasActive = prevState != null && !TERMINAL_STATES.includes(prevState);
    const isNowTerminal = currentRunState != null && TERMINAL_STATES.includes(currentRunState);
    
    prevRunStateRef.current = currentRunState;
    
    if (wasActive && isNowTerminal) {
      loadMetricsWithTimeSeries(true);
      loadStability();
    }
  }, [currentRunState, loadMetricsWithTimeSeries, loadStability]);

  useEffect(() => {
    if (elapsedIntervalRef.current) {
      clearInterval(elapsedIntervalRef.current);
      elapsedIntervalRef.current = null;
    }

    const startedAt = currentRunInfo?.started_at || run?.started_at || currentRunInfo?.created_at || run?.created_at;
    if (isRunActive && startedAt) {
      const startTime = new Date(startedAt).getTime();
      const updateElapsed = () => {
        setElapsedMs(Date.now() - startTime);
      };
      updateElapsed();
      elapsedIntervalRef.current = window.setInterval(updateElapsed, CONFIG.REFRESH_INTERVALS.ELAPSED_TIMER);
    } else if (!isRunActive && durationMs !== undefined) {
      setElapsedMs(durationMs);
    }

    return () => {
      if (elapsedIntervalRef.current) {
        clearInterval(elapsedIntervalRef.current);
        elapsedIntervalRef.current = null;
      }
    };
  }, [isRunActive, currentRunInfo?.started_at, run?.started_at, currentRunInfo?.created_at, run?.created_at, durationMs]);

  const handleSseEvent = useCallback((event: RunEvent) => {
    if (currentRunIdRef.current !== runId) return;
    
    lastSseEventRef.current = Date.now();
    
    const eventType = event.type?.toUpperCase();
    
    switch (eventType) {
      case 'STAGE_STARTED': {
        const rawStage = event.correlation?.stage || event.data.stage;
        const stageName = typeof rawStage === 'string' ? rawStage : 'unknown';
        const timestamp = event.ts_ms || Date.now();
        const marker: StageMarker = {
          timestamp,
          time: formatTime(timestamp),
          stage: stageName,
          label: stageName.toUpperCase(),
        };
        setStageMarkers(prev => {
          const exists = prev.some(m => m.stage === stageName && Math.abs(m.timestamp - timestamp) < 1000);
          if (exists) return prev;
          return [...prev, marker];
        });
        setCurrentStage(stageName);
        break;
      }
      case 'STAGE_COMPLETED':
        loadMetrics();
        loadStability();
        break;
      case 'STATE_TRANSITION': {
        const rawState = event.payload?.to_state || event.data.to_state;
        const toState = typeof rawState === 'string' ? rawState : '';
        if (toState) {
          setCurrentRunState(toState);
        }
        break;
      }
      case 'WORKER_HEARTBEAT': {
        const metrics = event.payload?.metrics || event.data.metrics;
        if (metrics && typeof metrics === 'object' && metrics !== null) {
          const timestamp = toSafeNumber(getProperty(metrics, 'timestamp'), Date.now());
          const totalOps = toSafeNumber(getProperty(metrics, 'total_ops'), 0);
          const failedOps = toSafeNumber(getProperty(metrics, 'failed_ops'), 0);
          const successOps = toSafeNumber(getProperty(metrics, 'success_ops'), totalOps - failedOps);
          
          setLatestTotals({
            total_ops: totalOps,
            success_ops: successOps,
            failed_ops: failedOps,
          });
          
          const newPoint: MetricsDataPoint = {
            timestamp,
            time: formatTime(timestamp),
            throughput: toSafeNumber(getProperty(metrics, 'throughput'), 0),
            latency_p50_ms: toSafeNumber(getProperty(metrics, 'latency_p50_ms'), 0),
            latency_p95_ms: toSafeNumber(getProperty(metrics, 'latency_p95_ms'), 0),
            latency_p99_ms: toSafeNumber(getProperty(metrics, 'latency_p99_ms'), 0),
            latency_mean: toSafeNumber(getProperty(metrics, 'latency_mean'), 0),
            error_rate: toSafeNumber(getProperty(metrics, 'error_rate'), 0),
            success_ops: successOps,
            failed_ops: failedOps,
          };
          setDataPoints(prev => {
            const updated = [...prev, newPoint];
            return updated.slice(-CONFIG.MAX_DATA_POINTS);
          });
          const durationValue = getProperty(metrics, 'duration_ms');
          if (durationValue !== undefined) {
            setDurationMs(toSafeNumber(durationValue, 0));
          }
        }
        break;
      }
    }
  }, [runId, loadMetrics, loadStability]);

  const handleSseError = useCallback(() => {
    if (currentRunIdRef.current !== runId) return;
    setSseConnected(false);
  }, [runId]);

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
    if (isFirstRenderAfterRunChangeRef.current) {
      isFirstRenderAfterRunChangeRef.current = false;
      return;
    }
    if (dataPoints.length > 0 || latestTotals.total_ops > 0) {
      saveMetricsToStorage(runId, {
        dataPoints,
        durationMs,
        stability,
        latestTotals,
        stageMarkers,
        lastUpdated: Date.now(),
      });
    }
  }, [runId, dataPoints, durationMs, stability, latestTotals, stageMarkers]);

  return {
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
    loadStability,
    loadRunState,
  };
}
