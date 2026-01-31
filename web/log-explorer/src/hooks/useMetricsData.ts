import { useState, useEffect, useCallback, useRef } from 'react';
import type { RunInfo, LiveMetrics, MetricsDataPoint, StabilityMetrics } from '../types';
import { fetchRun, subscribeToRunEvents, type RunEvent } from '../api/index';
import { formatTime } from '../utils/formatting';
import { saveWithEviction, loadFromLocalStorage } from '../utils/storage';
import { CONFIG, STORAGE_KEYS } from '../config';

const API_BASE = '';

interface CachedMetricsData {
  dataPoints: MetricsDataPoint[];
  durationMs?: number;
  stability: StabilityMetrics | null;
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
  handleManualRefresh: () => void;
  loadMetrics: () => Promise<void>;
  loadStability: () => Promise<void>;
  loadRunState: () => Promise<void>;
}

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
      const includeTimeSeries = !isRunActive;
      const metrics = await fetchMetrics(runId, includeTimeSeries);
      
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
          latency_mean: metrics.latency_mean || (metrics.latency_p50_ms + metrics.latency_p95_ms) / 2,
          error_rate: metrics.error_rate,
          success_ops: metrics.success_ops || (metrics.total_ops - metrics.failed_ops),
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
    }, CONFIG.DEBOUNCE_MS);
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
      intervalRef.current = window.setInterval(() => loadMetrics(), CONFIG.REFRESH_INTERVALS.METRICS);
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
  }, [isRunActive, currentRunInfo?.started_at, run?.started_at, durationMs]);

  const handleSseEvent = useCallback((event: RunEvent) => {
    const eventType = event.type?.toUpperCase();
    
    switch (eventType) {
      case 'STAGE_STARTED':
        setCurrentStage(event.correlation?.stage || event.data.stage as string || null);
        break;
      case 'STAGE_COMPLETED':
        loadMetrics();
        loadStability();
        break;
      case 'STATE_TRANSITION': {
        const toState = event.payload?.to_state || event.data.to_state;
        if (toState === 'completed' || toState === 'failed' || toState === 'stopped') {
          setCurrentRunState(toState as string);
          loadMetrics();
          loadStability();
        }
        break;
      }
      case 'WORKER_HEARTBEAT': {
        const metrics = event.payload?.metrics || event.data.metrics;
        if (metrics && typeof metrics === 'object') {
          const m = metrics as Record<string, unknown>;
          const timestamp = (m.timestamp as number) || Date.now();
          const newPoint: MetricsDataPoint = {
            timestamp,
            time: formatTime(timestamp),
            throughput: (m.throughput as number) || 0,
            latency_p50_ms: (m.latency_p50_ms as number) || 0,
            latency_p95_ms: (m.latency_p95_ms as number) || 0,
            latency_p99_ms: (m.latency_p99_ms as number) || 0,
            latency_mean: (m.latency_mean as number) || 0,
            error_rate: (m.error_rate as number) || 0,
            success_ops: (m.success_ops as number) || 0,
            failed_ops: (m.failed_ops as number) || 0,
          };
          setDataPoints(prev => {
            const updated = [...prev, newPoint];
            return updated.slice(-CONFIG.MAX_DATA_POINTS);
          });
          if (m.duration_ms !== undefined) {
            setDurationMs(m.duration_ms as number);
          }
        }
        break;
      }
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
    handleManualRefresh,
    loadMetrics,
    loadStability,
    loadRunState,
  };
}
