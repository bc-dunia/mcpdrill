import { useState, useEffect, useCallback, useRef, memo } from 'react';
import type { ServerMetricsResponse, ServerMetricsDataPoint, ServerMetricsAggregated } from '../types';
import { CpuUsageChart } from './CpuUsageChart';
import { MemoryUsageChart } from './MemoryUsageChart';
import { LoadAverageChart } from './LoadAverageChart';
import { Icon } from './Icon';
import { formatTime } from '../utils/formatting';
import { saveToLocalStorage, loadFromLocalStorage } from '../utils/storage';
import { CONFIG, STORAGE_KEYS } from '../config';

interface ServerResourcesSectionProps {
  runId: string;
  isRunActive: boolean;
}

const API_BASE = '';

interface CachedServerMetrics {
  dataPoints: ServerMetricsDataPoint[];
  aggregated: ServerMetricsAggregated | null;
  lastUpdated: number;
}

function saveToStorage(runId: string, data: CachedServerMetrics): void {
  saveToLocalStorage(`${STORAGE_KEYS.SERVER_METRICS_PREFIX}${runId}`, data);
}

function loadFromStorage(runId: string): CachedServerMetrics | null {
  return loadFromLocalStorage<CachedServerMetrics>(`${STORAGE_KEYS.SERVER_METRICS_PREFIX}${runId}`);
}

function convertSamplesToDataPoints(samples: ServerMetricsResponse['samples']): ServerMetricsDataPoint[] {
  return samples
    .filter(sample => sample.host != null)
    .map(sample => {
      const host = sample.host!;
      const memTotalGb = host.mem_total / (1024 * 1024 * 1024);
      const memUsedGb = host.mem_used / (1024 * 1024 * 1024);
      const memPercent = host.mem_total > 0 ? (host.mem_used / host.mem_total) * 100 : 0;

      return {
        timestamp: sample.timestamp,
        time: formatTime(sample.timestamp),
        cpu_percent: host.cpu_percent ?? 0,
        memory_percent: memPercent,
        memory_used_gb: memUsedGb,
        memory_total_gb: memTotalGb,
        load_avg_1: host.load_avg_1 ?? 0,
        load_avg_5: host.load_avg_5 ?? 0,
        load_avg_15: host.load_avg_15 ?? 0,
      };
    });
}

async function fetchServerMetrics(runId: string): Promise<ServerMetricsResponse> {
  const response = await fetch(`${API_BASE}/runs/${runId}/server-metrics`);
  if (!response.ok) {
    throw new Error(`Failed to fetch server metrics: ${response.statusText}`);
  }
  return response.json();
}

function ServerResourcesSectionComponent({ runId, isRunActive }: ServerResourcesSectionProps) {
   const [dataPoints, setDataPoints] = useState<ServerMetricsDataPoint[]>([]);
   const [aggregated, setAggregated] = useState<ServerMetricsAggregated | null>(null);
   const [loading, setLoading] = useState(true);
   const [isCollapsed, setIsCollapsed] = useState(false);
   const [hasAgent, setHasAgent] = useState<boolean | null>(null);
   const intervalRef = useRef<number | null>(null);
   const isLoadingRef = useRef(false);
   const abortControllerRef = useRef<AbortController | null>(null);

   const loadMetrics = useCallback(async (signal?: AbortSignal) => {
     if (isLoadingRef.current) return;
     isLoadingRef.current = true;

     try {
       const response = await fetchServerMetrics(runId);
       
       if (!signal?.aborted) {
         if (response.samples && response.samples.length > 0) {
           const points = convertSamplesToDataPoints(response.samples);
           setDataPoints(points);
           setAggregated(response.aggregated ?? null);
           setHasAgent(true);
         } else {
           setHasAgent(false);
         }
       }
     } catch {
       if (!signal?.aborted) {
         setHasAgent(false);
       }
     } finally {
       if (!signal?.aborted) {
         setLoading(false);
       }
       isLoadingRef.current = false;
     }
   }, [runId]);

   useEffect(() => {
     abortControllerRef.current = new AbortController();
     const signal = abortControllerRef.current.signal;

     const cached = loadFromStorage(runId);
     if (cached && cached.dataPoints.length > 0) {
       setDataPoints(cached.dataPoints);
       setAggregated(cached.aggregated);
       setHasAgent(true);
       setLoading(false);
     }
     loadMetrics(signal);

     return () => {
       abortControllerRef.current?.abort();
     };
   }, [runId, loadMetrics]);

   useEffect(() => {
     if (intervalRef.current) {
       clearInterval(intervalRef.current);
       intervalRef.current = null;
     }

     if (isRunActive && hasAgent) {
       intervalRef.current = window.setInterval(() => {
         if (!abortControllerRef.current?.signal.aborted) {
           loadMetrics(abortControllerRef.current?.signal);
         }
       }, CONFIG.REFRESH_INTERVALS.SERVER_RESOURCES);
     }

     return () => {
       if (intervalRef.current) {
         clearInterval(intervalRef.current);
         intervalRef.current = null;
       }
     };
   }, [isRunActive, hasAgent, loadMetrics]);

  useEffect(() => {
    if (dataPoints.length > 0) {
      saveToStorage(runId, {
        dataPoints,
        aggregated,
        lastUpdated: Date.now(),
      });
    }
  }, [runId, dataPoints, aggregated]);

  const formatMemory = (gb: number): string => {
    if (gb >= 1) return `${gb.toFixed(1)} GB`;
    return `${(gb * 1024).toFixed(0)} MB`;
  };

  // Don't render anything if:
  // - Still loading with no cached data
  // - No agent data available (agent not configured/connected)
  // This keeps the dashboard clean for users who don't use the optional agent feature
  if (loading && !dataPoints.length) {
    return null;
  }

  if (hasAgent === false || dataPoints.length === 0) {
    return null;
  }

  const peakCpu = aggregated?.cpu_max ?? Math.max(...dataPoints.map(d => d.cpu_percent), 0);
  const avgCpu = aggregated?.cpu_avg ?? (dataPoints.length > 0 
    ? dataPoints.reduce((sum, d) => sum + d.cpu_percent, 0) / dataPoints.length 
    : 0);
  const peakMem = aggregated?.mem_max ?? Math.max(...dataPoints.map(d => d.memory_percent), 0);
  const avgMem = aggregated?.mem_avg ?? (dataPoints.length > 0 
    ? dataPoints.reduce((sum, d) => sum + d.memory_percent, 0) / dataPoints.length 
    : 0);
  const latestMemGb = dataPoints.length > 0 ? dataPoints[dataPoints.length - 1].memory_used_gb : 0;

  return (
    <section className="server-resources-section" aria-labelledby="server-resources-heading">
      <div className="server-resources-header">
        <button 
          className="server-resources-toggle" 
          onClick={() => setIsCollapsed(!isCollapsed)}
          aria-expanded={!isCollapsed}
          aria-controls="server-resources-content"
        >
          <Icon name={isCollapsed ? 'chevron-right' : 'chevron-down'} size="sm" aria-hidden={true} />
          <h3 id="server-resources-heading">
            <Icon name="server" size="md" aria-hidden={true} /> Server Resources
          </h3>
        </button>
        <div className="server-resources-summary">
          <span className="summary-badge-compact" title="Peak CPU utilization">
            <Icon name="cpu" size="xs" aria-hidden={true} />
            Peak: {peakCpu.toFixed(1)}%
          </span>
          <span className="summary-badge-compact" title="Average CPU utilization">
            Avg: {avgCpu.toFixed(1)}%
          </span>
          <span className="summary-badge-compact summary-badge-memory" title="Peak memory utilization">
            <Icon name="database" size="xs" aria-hidden={true} />
            Peak: {peakMem.toFixed(1)}%
          </span>
          <span className="summary-badge-compact summary-badge-memory" title={`Current: ${formatMemory(latestMemGb)}`}>
            Avg: {avgMem.toFixed(1)}%
          </span>
        </div>
      </div>

      {!isCollapsed && (
        <div id="server-resources-content" className="server-resources-charts">
          <div className="server-charts-row">
            <div className="server-chart-cell">
              <CpuUsageChart data={dataPoints} loading={loading && dataPoints.length === 0} />
            </div>
            <div className="server-chart-cell">
              <MemoryUsageChart data={dataPoints} loading={loading && dataPoints.length === 0} />
            </div>
          </div>
          <div className="server-charts-row server-charts-full">
            <LoadAverageChart data={dataPoints} loading={loading && dataPoints.length === 0} />
          </div>
        </div>
      )}
    </section>
  );
}

export const ServerResourcesSection = memo(ServerResourcesSectionComponent);
