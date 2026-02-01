import { useState, useCallback, useEffect, useRef, useMemo } from 'react';
import type { MetricsDataPoint, MetricsSummary, StabilityMetrics, StageMarker, LogFilters } from '../types';
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
import type { BrushRange } from './BaseChart';

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
  stageMarkers?: StageMarker[];
  onNavigateToLogs?: (key: keyof LogFilters, value: string) => void;
}

export function MetricsTabs({ activeTab, onChange }: MetricsTabsProps) {
  return (
    <div className="metrics-tabs" role="tablist" aria-label="Metrics view">
      <button
        type="button"
        id="metrics-overview-tab"
        role="tab"
        aria-selected={activeTab === 'overview'}
        aria-controls="metrics-overview-panel"
        tabIndex={activeTab === 'overview' ? 0 : -1}
        className={`metrics-tab ${activeTab === 'overview' ? 'active' : ''}`}
        onClick={() => onChange('overview')}
      >
        <Icon name="chart-bar" size="sm" aria-hidden={true} /> Overview
      </button>
      <button
        type="button"
        id="metrics-tools-tab"
        role="tab"
        aria-selected={activeTab === 'tools'}
        aria-controls="metrics-tools-panel"
        tabIndex={activeTab === 'tools' ? 0 : -1}
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
  stageMarkers,
  onNavigateToLogs,
}: MetricsChartsProps) {
  const [brushRange, setBrushRange] = useState<BrushRange>({
    startIndex: 0,
    endIndex: Math.max(0, dataPoints.length - 1),
  });
  const [hasUserZoomed, setHasUserZoomed] = useState(false);
  const [selectedStage, setSelectedStage] = useState<string>('all');
  const prevRunIdRef = useRef(runId);

  const availableStages = useMemo(() => {
    if (!stageMarkers || stageMarkers.length === 0) return [];
    // Get unique stages in order
    const seen = new Set<string>();
    return stageMarkers.filter(m => {
      if (seen.has(m.stage)) return false;
      seen.add(m.stage);
      return true;
    });
  }, [stageMarkers]);

  const filteredDataPoints = useMemo(() => {
    if (selectedStage === 'all' || !stageMarkers || stageMarkers.length === 0) {
      return dataPoints;
    }
    
    // Find the stage marker(s) for selected stage
    const stageIndices = stageMarkers
      .map((m, i) => ({ marker: m, index: i }))
      .filter(({ marker }) => marker.stage === selectedStage);
    
    if (stageIndices.length === 0) return dataPoints;
    
    // Get time range: from first occurrence of stage to next different stage (or end)
    const startTs = stageIndices[0].marker.timestamp;
    const nextStageIndex = stageIndices[stageIndices.length - 1].index + 1;
    const endTs = nextStageIndex < stageMarkers.length 
      ? stageMarkers[nextStageIndex].timestamp 
      : Infinity;
    
    return dataPoints.filter(d => d.timestamp >= startTs && d.timestamp < endTs);
  }, [dataPoints, stageMarkers, selectedStage]);

  useEffect(() => {
    if (runId !== prevRunIdRef.current) {
      prevRunIdRef.current = runId;
      setHasUserZoomed(false);
      setBrushRange({ startIndex: 0, endIndex: 0 });
      setSelectedStage('all');
    }
  }, [runId]);

  useEffect(() => {
    if (!hasUserZoomed) {
      setBrushRange({
        startIndex: 0,
        endIndex: Math.max(0, filteredDataPoints.length - 1),
      });
    } else {
      setBrushRange(prev => ({
        startIndex: prev.startIndex,
        endIndex: Math.max(prev.endIndex, filteredDataPoints.length - 1),
      }));
    }
  }, [filteredDataPoints.length, hasUserZoomed]);

  const handleBrushChange = useCallback((range: BrushRange) => {
    setHasUserZoomed(true);
    setBrushRange(range);
  }, []);

  const handleResetZoom = useCallback(() => {
    setHasUserZoomed(false);
    setBrushRange({
      startIndex: 0,
      endIndex: Math.max(0, filteredDataPoints.length - 1),
    });
  }, [filteredDataPoints.length]);

  const handleToolClick = useCallback((toolName: string) => {
    onNavigateToLogs?.('tool_name', toolName);
  }, [onNavigateToLogs]);

  const handleSessionClick = useCallback((sessionId: string) => {
    onNavigateToLogs?.('session_id', sessionId);
  }, [onNavigateToLogs]);

  const isZoomed = filteredDataPoints.length > 0 && (brushRange.startIndex > 0 || brushRange.endIndex < filteredDataPoints.length - 1);

  return (
    <>
      {activeTab === 'overview' && (
        <div id="metrics-overview-panel" role="tabpanel" aria-labelledby="metrics-overview-tab">
          {availableStages.length > 1 && (
            <div className="stage-filter-bar">
              <label htmlFor="stage-filter" className="stage-filter-label">
                <Icon name="list" size="sm" aria-hidden={true} />
                Stage:
              </label>
              <select
                id="stage-filter"
                value={selectedStage}
                onChange={(e) => setSelectedStage(e.target.value)}
                className="select-input select-small"
              >
                <option value="all">All Stages</option>
                {availableStages.map((m) => (
                  <option key={m.stage} value={m.stage}>
                    {m.label}
                  </option>
                ))}
              </select>
              {selectedStage !== 'all' && (
                <span className="stage-filter-info">
                  {filteredDataPoints.length} of {dataPoints.length} points
                </span>
              )}
            </div>
          )}

          <MetricsSummarySection
            summary={summary}
            isRunActive={isRunActive}
            stabilityLoading={stabilityLoading}
            stability={stability}
          />

          {isZoomed && (
            <div className="brush-controls">
              <button type="button" className="btn btn-ghost btn-sm" onClick={handleResetZoom}>
                <Icon name="refresh" size="sm" aria-hidden={true} />
                Reset Zoom
              </button>
              <span className="brush-range-info">
                Showing {brushRange.endIndex - brushRange.startIndex + 1} of {dataPoints.length} data points
              </span>
            </div>
          )}

          <div className="metrics-charts-grid-hierarchical">
            <div className="chart-cell chart-primary">
              <ThroughputChart 
                data={filteredDataPoints} 
                loading={loading && dataPoints.length === 0}
                enableBrush={true}
                brushRange={brushRange}
                onBrushChange={handleBrushChange}
                stageMarkers={stageMarkers}
              />
            </div>
            <div className="chart-cell chart-secondary">
              <ConnectionStabilityChart
                data={stability?.time_series ?? []}
                loading={stabilityLoading && !stability?.time_series?.length}
              />
            </div>
            <div className="chart-cell chart-primary">
              <LatencyChart 
                data={filteredDataPoints} 
                loading={loading && dataPoints.length === 0}
                brushRange={brushRange}
                stageMarkers={stageMarkers}
              />
            </div>
            <div className="chart-cell chart-secondary">
              <ErrorRateChart 
                data={filteredDataPoints} 
                loading={loading && dataPoints.length === 0}
                threshold={0.1}
                brushRange={brushRange}
                stageMarkers={stageMarkers}
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
                onSessionClick={handleSessionClick}
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
          <ToolMetricsDashboard runId={runId} onToolClick={handleToolClick} />
        </div>
      )}
    </>
  );
}
