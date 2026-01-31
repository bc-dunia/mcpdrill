import { useState, useEffect, useCallback, useMemo, memo, useRef } from 'react';
import {
  ResponsiveContainer,
  PieChart,
  Pie,
  Cell,
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
} from 'recharts';
import type { ToolMetrics } from '../types';
import { Icon } from './Icon';
import { ErrorBoundary } from './ErrorBoundary';
import { exportChartAsPng } from '../utils/chartExport';
import { formatLatency } from '../utils/formatting';

interface ToolMetricsDashboardProps {
  runId: string;
  onToolClick?: (toolName: string) => void;
}

interface AggregatedMetricsResponse {
  run_id: string;
  by_tool?: Record<string, ToolMetrics>;
  timestamp?: number;
}

type SortKey = 'name' | 'total_ops' | 'success_ops' | 'failure_ops' | 'success_rate' | 'p50' | 'p95' | 'p99';
type SortDirection = 'asc' | 'desc';

const API_BASE = '';
const AUTO_REFRESH_INTERVAL = 2000;

const CHART_COLORS = [
  '#4ade80', '#22d3ee', '#a78bfa', '#fbbf24', '#f87171',
  '#34d399', '#60a5fa', '#c084fc', '#fb923c', '#fb7185',
  '#2dd4bf', '#818cf8', '#e879f9', '#facc15', '#f43f5e',
];

function getToolColor(index: number): string {
  return CHART_COLORS[index % CHART_COLORS.length];
}

// Debounce hook
function useDebounce<T>(value: T, delay: number): T {
  const [debouncedValue, setDebouncedValue] = useState<T>(value);

  useEffect(() => {
    const handler = setTimeout(() => {
      setDebouncedValue(value);
    }, delay);

    return () => {
      clearTimeout(handler);
    };
  }, [value, delay]);

  return debouncedValue;
}

// Custom Tooltip for Pie Chart
const PieTooltip = memo(({ active, payload }: { 
  active?: boolean; 
  payload?: Array<{ name: string; value: number; payload: { successRate: number; toolName: string } }> 
}) => {
  if (!active || !payload?.length) return null;
  const data = payload[0].payload;
  return (
    <div className="metrics-tooltip">
      <div className="tooltip-title">{data.toolName}</div>
      <div className="tooltip-row">
        <span>Success Rate</span>
        <span className="tooltip-value">{data.successRate.toFixed(1)}%</span>
      </div>
      <div className="tooltip-row">
        <span>Total Calls</span>
        <span className="tooltip-value">{payload[0].value.toLocaleString()}</span>
      </div>
    </div>
  );
});

// Custom Tooltip for Bar Chart
const BarTooltip = memo(({ active, payload, label }: { 
  active?: boolean; 
  payload?: Array<{ name: string; value: number; color: string }>; 
  label?: string 
}) => {
  if (!active || !payload?.length) return null;
  return (
    <div className="metrics-tooltip">
      <div className="tooltip-title">{label}</div>
      <div className="tooltip-values">
        {payload.map((entry, index) => (
          <div key={index} className="tooltip-row">
            <span className="tooltip-dot" style={{ background: entry.color }} />
            <span>{entry.name}</span>
            <span className="tooltip-value">{formatLatency(entry.value)}</span>
          </div>
        ))}
      </div>
    </div>
  );
});

// Memoized Pie Chart Component
const CallDistributionChart = memo(({ 
  data, 
  onToolClick, 
  chartRef,
  runId 
}: { 
  data: Array<{ toolName: string; value: number; successRate: number }>;
  onToolClick?: (toolName: string) => void;
  chartRef: React.RefObject<HTMLDivElement>;
  runId: string;
}) => (
  <div 
    className="tool-metrics-chart-card" 
    ref={chartRef}
    role="region" 
    aria-labelledby="pie-chart-title"
  >
    <div className="tool-metrics-chart-header">
      <h3 id="pie-chart-title">Call Distribution</h3>
      <button
        type="button"
        onClick={() => exportChartAsPng(chartRef.current, `${runId}-call-distribution`)}
        className="btn btn-ghost btn-sm"
        aria-label="Export chart as PNG"
      >
        <Icon name="download" size="sm" aria-hidden={true} />
      </button>
    </div>
    <div className="tool-metrics-chart-body">
      <ResponsiveContainer width="100%" height={220}>
        <PieChart>
          <Pie
            data={data}
            dataKey="value"
            nameKey="toolName"
            cx="50%"
            cy="50%"
            innerRadius={50}
            outerRadius={80}
            paddingAngle={2}
            onClick={(d) => onToolClick?.(d.toolName)}
            style={{ cursor: onToolClick ? 'pointer' : 'default' }}
          >
            {data.map((entry, index) => (
              <Cell 
                key={entry.toolName}
                fill={getToolColor(index)}
                stroke="var(--bg-primary)"
                strokeWidth={2}
              />
            ))}
          </Pie>
          <Tooltip content={<PieTooltip />} />
          <Legend 
            layout="vertical" 
            align="right" 
            verticalAlign="middle"
            wrapperStyle={{ fontSize: '11px', maxWidth: '120px' }}
            formatter={(value: string) => value.length > 14 ? value.slice(0, 14) + '...' : value}
          />
        </PieChart>
      </ResponsiveContainer>
    </div>
  </div>
));

// Memoized Bar Chart Component for Slowest Tools
const SlowestToolsChart = memo(({ 
  data, 
  chartRef,
  runId 
}: { 
  data: Array<{ name: string; fullName: string; p95: number }>;
  chartRef: React.RefObject<HTMLDivElement>;
  runId: string;
}) => (
  <div 
    className="tool-metrics-chart-card" 
    ref={chartRef}
    role="region" 
    aria-labelledby="slowest-chart-title"
  >
    <div className="tool-metrics-chart-header">
      <h3 id="slowest-chart-title">Slowest Tools (P95)</h3>
      <button
        type="button"
        onClick={() => exportChartAsPng(chartRef.current, `${runId}-slowest-tools`)}
        className="btn btn-ghost btn-sm"
        aria-label="Export chart as PNG"
      >
        <Icon name="download" size="sm" aria-hidden={true} />
      </button>
    </div>
    <div className="tool-metrics-chart-body">
      {data.length > 0 ? (
        <ResponsiveContainer width="100%" height={220}>
          <BarChart 
            data={data} 
            layout="vertical" 
            margin={{ top: 8, right: 16, left: 80, bottom: 8 }}
          >
            <CartesianGrid strokeDasharray="3 3" stroke="var(--border-subtle)" horizontal={false} />
            <XAxis 
              type="number" 
              stroke="var(--text-muted)" 
              fontSize={10}
              tickFormatter={(v) => `${v}ms`}
            />
            <YAxis 
              type="category" 
              dataKey="name" 
              stroke="var(--text-muted)" 
              fontSize={10}
              width={75}
              tickFormatter={(v) => v.length > 12 ? v.slice(0, 12) + '...' : v}
            />
            <Tooltip content={<BarTooltip />} />
            <Bar 
              dataKey="p95" 
              name="P95 Latency" 
              fill="#fbbf24" 
              radius={[0, 4, 4, 0]}
              barSize={16}
            />
          </BarChart>
        </ResponsiveContainer>
      ) : (
        <div className="tool-metrics-chart-empty">
          <Icon name="chart-bar" size="lg" aria-hidden={true} />
          <p>No latency data available</p>
        </div>
      )}
    </div>
  </div>
));

function ToolMetricsDashboardComponent({ runId, onToolClick }: ToolMetricsDashboardProps) {
  const [toolMetrics, setToolMetrics] = useState<Record<string, ToolMetrics>>({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [autoRefresh, setAutoRefresh] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');
  const [errorsOnly, setErrorsOnly] = useState(false);
  const [sortKey, setSortKey] = useState<SortKey>('total_ops');
  const [sortDirection, setSortDirection] = useState<SortDirection>('desc');
  
  const pieChartRef = useRef<HTMLDivElement>(null);
  const barChartRef = useRef<HTMLDivElement>(null);
  
  const debouncedSearch = useDebounce(searchQuery, 300);
  const prevRunIdRef = useRef(runId);

  useEffect(() => {
    if (prevRunIdRef.current !== runId) {
      setToolMetrics({});
      setError(null);
      setLoading(true);
      setSearchQuery('');
      setErrorsOnly(false);
      prevRunIdRef.current = runId;
    }
  }, [runId]);

  const fetchData = useCallback(async () => {
    if (!runId) return;
    
    const thisRunId = runId;

    try {
      const response = await fetch(`${API_BASE}/runs/${thisRunId}/metrics`);
      if (!response.ok) {
        throw new Error(`Failed to fetch metrics (${response.status})`);
      }
      
      if (prevRunIdRef.current !== thisRunId) return;
      
      const data: AggregatedMetricsResponse = await response.json();
      setToolMetrics(data.by_tool || {});
      setError(null);
    } catch (err) {
      if (prevRunIdRef.current === thisRunId) {
        console.error('Error fetching tool metrics:', err);
        setError(err instanceof Error ? err.message : 'Failed to fetch data');
      }
    } finally {
      if (prevRunIdRef.current === thisRunId) {
        setLoading(false);
      }
    }
  }, [runId]);

  // Initial fetch
  useEffect(() => {
    fetchData();
  }, [fetchData]);

  // Auto-refresh polling
  useEffect(() => {
    if (!autoRefresh) return;
    
    const interval = setInterval(fetchData, AUTO_REFRESH_INTERVAL);
    return () => clearInterval(interval);
  }, [autoRefresh, fetchData]);

  // Compute KPI metrics
  const kpiMetrics = useMemo(() => {
    const tools = Object.entries(toolMetrics);
    if (tools.length === 0) {
      return { totalCalls: 0, errorRate: 0, avgP95: 0, worstTool: null };
    }

    let totalCalls = 0;
    let totalErrors = 0;
    let p95Sum = 0;
    let worstTool: { name: string; errorRate: number; p95: number } | null = null;

    for (const [name, metrics] of tools) {
      totalCalls += metrics.total_ops;
      totalErrors += metrics.failure_ops;
      p95Sum += metrics.latency_p95;

      const toolErrorRate = metrics.total_ops > 0 
        ? (metrics.failure_ops / metrics.total_ops) * 100 
        : 0;

      if (!worstTool || toolErrorRate > worstTool.errorRate || 
          (toolErrorRate === worstTool.errorRate && metrics.latency_p95 > worstTool.p95)) {
        worstTool = { name, errorRate: toolErrorRate, p95: metrics.latency_p95 };
      }
    }

    return {
      totalCalls,
      errorRate: totalCalls > 0 ? (totalErrors / totalCalls) * 100 : 0,
      avgP95: tools.length > 0 ? p95Sum / tools.length : 0,
      worstTool,
    };
  }, [toolMetrics]);

  // Filter and sort tools
  const filteredAndSortedTools = useMemo(() => {
    let tools = Object.entries(toolMetrics).map(([name, metrics]) => {
      const successRate = metrics.total_ops > 0 
        ? (metrics.success_ops / metrics.total_ops) * 100 
        : 100;
      return { name, metrics, successRate };
    });

    // Apply search filter
    if (debouncedSearch) {
      const searchLower = debouncedSearch.toLowerCase();
      tools = tools.filter(t => t.name.toLowerCase().includes(searchLower));
    }

    // Apply errors-only filter
    if (errorsOnly) {
      tools = tools.filter(t => t.metrics.failure_ops > 0);
    }

    // Sort
    tools.sort((a, b) => {
      let aVal: number | string;
      let bVal: number | string;

      switch (sortKey) {
        case 'name':
          aVal = a.name.toLowerCase();
          bVal = b.name.toLowerCase();
          break;
        case 'total_ops':
          aVal = a.metrics.total_ops;
          bVal = b.metrics.total_ops;
          break;
        case 'success_ops':
          aVal = a.metrics.success_ops;
          bVal = b.metrics.success_ops;
          break;
        case 'failure_ops':
          aVal = a.metrics.failure_ops;
          bVal = b.metrics.failure_ops;
          break;
        case 'success_rate':
          aVal = a.successRate;
          bVal = b.successRate;
          break;
        case 'p50':
          aVal = a.metrics.latency_p50;
          bVal = b.metrics.latency_p50;
          break;
        case 'p95':
          aVal = a.metrics.latency_p95;
          bVal = b.metrics.latency_p95;
          break;
        case 'p99':
          aVal = a.metrics.latency_p99;
          bVal = b.metrics.latency_p99;
          break;
        default:
          aVal = a.metrics.total_ops;
          bVal = b.metrics.total_ops;
      }

      if (typeof aVal === 'string') {
        return sortDirection === 'asc' 
          ? aVal.localeCompare(bVal as string) 
          : (bVal as string).localeCompare(aVal);
      }
      return sortDirection === 'asc' ? aVal - (bVal as number) : (bVal as number) - aVal;
    });

    return tools;
  }, [toolMetrics, debouncedSearch, errorsOnly, sortKey, sortDirection]);

  // Chart data
  const pieData = useMemo(() => {
    return Object.entries(toolMetrics)
      .map(([name, metrics]) => ({
        toolName: name,
        value: metrics.total_ops,
        successRate: metrics.total_ops > 0 
          ? (metrics.success_ops / metrics.total_ops) * 100 
          : 100,
      }))
      .sort((a, b) => b.value - a.value)
      .slice(0, 8);
  }, [toolMetrics]);

  const slowestToolsData = useMemo(() => {
    return Object.entries(toolMetrics)
      .map(([name, metrics]) => ({
        name: name.length > 14 ? name.slice(0, 14) + '...' : name,
        fullName: name,
        p95: metrics.latency_p95,
      }))
      .sort((a, b) => b.p95 - a.p95)
      .slice(0, 6);
  }, [toolMetrics]);

  // Handlers
  const handleSort = useCallback((key: SortKey) => {
    if (sortKey === key) {
      setSortDirection(d => d === 'asc' ? 'desc' : 'asc');
    } else {
      setSortKey(key);
      setSortDirection('desc');
    }
  }, [sortKey]);

  const handleRefresh = useCallback(() => {
    setLoading(true);
    fetchData();
  }, [fetchData]);

  const getErrorRateClass = (rate: number): string => {
    if (rate < 1) return 'tool-metrics-rate-good';
    if (rate <= 5) return 'tool-metrics-rate-warn';
    return 'tool-metrics-rate-bad';
  };

  const getSuccessRateClass = (rate: number): string => {
    if (rate >= 99) return 'tool-metrics-rate-good';
    if (rate >= 95) return 'tool-metrics-rate-warn';
    return 'tool-metrics-rate-bad';
  };

  const renderSortIcon = (key: SortKey) => {
    if (sortKey !== key) return null;
    return (
      <span className="tool-metrics-sort-icon" aria-hidden="true">
        {sortDirection === 'asc' ? '↑' : '↓'}
      </span>
    );
  };

  // Loading state
  if (loading && Object.keys(toolMetrics).length === 0) {
    return (
      <div className="tool-metrics-dashboard" role="region" aria-label="Tool Metrics Dashboard">
        <div className="tool-metrics-loading" role="status" aria-live="polite">
          <div className="spinner" aria-hidden={true} />
          <span>Loading tool metrics...</span>
        </div>
      </div>
    );
  }

  // Error state
  if (error && Object.keys(toolMetrics).length === 0) {
    return (
      <div className="tool-metrics-dashboard" role="region" aria-label="Tool Metrics Dashboard">
        <div className="tool-metrics-error" role="alert">
          <Icon name="alert-triangle" size="lg" aria-hidden={true} />
          <h3>Failed to Load Metrics</h3>
          <p>{error}</p>
          <button type="button" onClick={handleRefresh} className="btn btn-secondary">
            <Icon name="refresh" size="sm" aria-hidden={true} />
            Retry
          </button>
        </div>
      </div>
    );
  }

  // Empty state
  if (Object.keys(toolMetrics).length === 0) {
    return (
      <div className="tool-metrics-dashboard" role="region" aria-label="Tool Metrics Dashboard">
        <div className="tool-metrics-empty">
          <Icon name="chart-bar" size="xl" aria-hidden={true} />
          <h3>No Tool Metrics Available</h3>
          <p className="muted">Run a test with tool operations to see metrics</p>
        </div>
      </div>
    );
  }

  return (
    <div className="tool-metrics-dashboard" role="region" aria-label="Tool Metrics Dashboard">
      {/* Header */}
      <div className="tool-metrics-header">
        <div className="tool-metrics-title">
          <Icon name="chart-bar" size="md" aria-hidden={true} />
          <h2>Tool Performance</h2>
          <span className="tool-metrics-count">{Object.keys(toolMetrics).length} tools</span>
        </div>
        <div className="tool-metrics-controls">
          <label className="tool-metrics-auto-refresh">
            <input
              type="checkbox"
              checked={autoRefresh}
              onChange={(e) => setAutoRefresh(e.target.checked)}
              aria-describedby="auto-refresh-desc"
            />
            <span className="tool-metrics-toggle-slider" aria-hidden="true" />
            <span className="tool-metrics-toggle-label">Auto-refresh</span>
          </label>
          <span id="auto-refresh-desc" className="sr-only">
            Refresh metrics every 2 seconds
          </span>
          <button
            type="button"
            onClick={handleRefresh}
            disabled={loading}
            className="btn btn-secondary btn-sm"
            aria-label="Refresh metrics"
          >
            <Icon name={loading ? 'loader' : 'refresh'} size="sm" aria-hidden={true} />
            Refresh
          </button>
        </div>
      </div>

      {/* KPI Cards */}
      <div className="tool-metrics-kpi-grid" role="region" aria-label="Key performance indicators">
        <div className="tool-metrics-kpi-card" title="Sum of all tool operations">
          <span className="tool-metrics-kpi-value">
            {kpiMetrics.totalCalls.toLocaleString()}
          </span>
          <span className="tool-metrics-kpi-label">Total Calls</span>
        </div>
        <div 
          className={`tool-metrics-kpi-card ${kpiMetrics.errorRate > 5 ? 'kpi-error' : kpiMetrics.errorRate > 1 ? 'kpi-warn' : ''}`} 
          title="Overall error percentage"
        >
          <span className={`tool-metrics-kpi-value ${getErrorRateClass(kpiMetrics.errorRate)}`}>
            {kpiMetrics.errorRate.toFixed(2)}%
          </span>
          <span className="tool-metrics-kpi-label">Error Rate</span>
        </div>
        <div className="tool-metrics-kpi-card" title="Average P95 latency across all tools">
          <span className="tool-metrics-kpi-value">
            {formatLatency(kpiMetrics.avgP95)}
          </span>
          <span className="tool-metrics-kpi-label">Avg P95</span>
        </div>
        <div 
          className="tool-metrics-kpi-card" 
          title={kpiMetrics.worstTool ? `Tool with highest error rate` : 'No data'}
        >
          <span className="tool-metrics-kpi-value tool-metrics-kpi-worst" title={kpiMetrics.worstTool?.name}>
            {kpiMetrics.worstTool 
              ? (kpiMetrics.worstTool.name.length > 12 
                  ? kpiMetrics.worstTool.name.slice(0, 12) + '...' 
                  : kpiMetrics.worstTool.name)
              : '—'}
          </span>
          <span className="tool-metrics-kpi-sublabel">
            {kpiMetrics.worstTool 
              ? `${kpiMetrics.worstTool.errorRate.toFixed(1)}% err · ${formatLatency(kpiMetrics.worstTool.p95)}`
              : ''}
          </span>
          <span className="tool-metrics-kpi-label">Top Error Rate</span>
        </div>
      </div>

      {/* Search and Filters */}
      <div className="tool-metrics-filters">
        <div className="tool-metrics-search">
          <Icon name="search" size="sm" aria-hidden={true} />
          <input
            type="text"
            placeholder="Filter by tool name..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="tool-metrics-search-input"
            aria-label="Filter tools by name"
          />
          {searchQuery && (
            <button
              type="button"
              onClick={() => setSearchQuery('')}
              className="tool-metrics-search-clear"
              aria-label="Clear search"
            >
              <Icon name="x" size="sm" aria-hidden={true} />
            </button>
          )}
        </div>
        <label className="tool-metrics-checkbox">
          <input
            type="checkbox"
            checked={errorsOnly}
            onChange={(e) => setErrorsOnly(e.target.checked)}
          />
          <span>Show errors only</span>
        </label>
        {(debouncedSearch || errorsOnly) && (
          <span className="tool-metrics-filter-count">
            {filteredAndSortedTools.length} of {Object.keys(toolMetrics).length}
          </span>
        )}
      </div>

      {/* Main Table */}
      <div className="tool-metrics-table-container" role="region" aria-label="Tool metrics table">
        <div className="tool-metrics-table-scroll">
          <table className="tool-metrics-table" role="grid">
            <caption className="sr-only">
              Detailed performance metrics for each tool. Click column headers to sort.
            </caption>
            <thead>
              <tr>
                <th 
                  scope="col" 
                  onClick={() => handleSort('name')}
                  className="tool-metrics-th-sortable"
                  aria-sort={sortKey === 'name' ? (sortDirection === 'asc' ? 'ascending' : 'descending') : 'none'}
                >
                  Tool {renderSortIcon('name')}
                </th>
                <th 
                  scope="col" 
                  onClick={() => handleSort('total_ops')}
                  className="tool-metrics-th-sortable tool-metrics-th-numeric"
                  aria-sort={sortKey === 'total_ops' ? (sortDirection === 'asc' ? 'ascending' : 'descending') : 'none'}
                >
                  Total {renderSortIcon('total_ops')}
                </th>
                <th 
                  scope="col" 
                  onClick={() => handleSort('success_ops')}
                  className="tool-metrics-th-sortable tool-metrics-th-numeric"
                  aria-sort={sortKey === 'success_ops' ? (sortDirection === 'asc' ? 'ascending' : 'descending') : 'none'}
                >
                  Success {renderSortIcon('success_ops')}
                </th>
                <th 
                  scope="col" 
                  onClick={() => handleSort('failure_ops')}
                  className="tool-metrics-th-sortable tool-metrics-th-numeric"
                  aria-sort={sortKey === 'failure_ops' ? (sortDirection === 'asc' ? 'ascending' : 'descending') : 'none'}
                >
                  Errors {renderSortIcon('failure_ops')}
                </th>
                <th 
                  scope="col" 
                  onClick={() => handleSort('success_rate')}
                  className="tool-metrics-th-sortable tool-metrics-th-numeric"
                  aria-sort={sortKey === 'success_rate' ? (sortDirection === 'asc' ? 'ascending' : 'descending') : 'none'}
                >
                  Success% {renderSortIcon('success_rate')}
                </th>
                <th 
                  scope="col" 
                  onClick={() => handleSort('p50')}
                  className="tool-metrics-th-sortable tool-metrics-th-numeric"
                  aria-sort={sortKey === 'p50' ? (sortDirection === 'asc' ? 'ascending' : 'descending') : 'none'}
                >
                  P50 {renderSortIcon('p50')}
                </th>
                <th 
                  scope="col" 
                  onClick={() => handleSort('p95')}
                  className="tool-metrics-th-sortable tool-metrics-th-numeric"
                  aria-sort={sortKey === 'p95' ? (sortDirection === 'asc' ? 'ascending' : 'descending') : 'none'}
                >
                  P95 {renderSortIcon('p95')}
                </th>
                <th 
                  scope="col" 
                  onClick={() => handleSort('p99')}
                  className="tool-metrics-th-sortable tool-metrics-th-numeric"
                  aria-sort={sortKey === 'p99' ? (sortDirection === 'asc' ? 'ascending' : 'descending') : 'none'}
                >
                  P99 {renderSortIcon('p99')}
                </th>
              </tr>
            </thead>
            <tbody>
              {filteredAndSortedTools.length === 0 ? (
                <tr>
                  <td colSpan={8} className="tool-metrics-table-empty-row">
                    <Icon name="search" size="md" aria-hidden={true} />
                    <span>No tools match your filters</span>
                  </td>
                </tr>
              ) : (
                filteredAndSortedTools.map(({ name, metrics, successRate }) => (
                  <tr 
                    key={name}
                    onClick={() => onToolClick?.(name)}
                    className={`tool-metrics-row ${onToolClick ? 'tool-metrics-row-clickable' : ''} ${metrics.failure_ops > 0 ? 'tool-metrics-row-error' : ''}`}
                    tabIndex={onToolClick ? 0 : undefined}
                    onKeyDown={(e) => {
                      if (onToolClick && (e.key === 'Enter' || e.key === ' ')) {
                        e.preventDefault();
                        onToolClick(name);
                      }
                    }}
                    role={onToolClick ? 'button' : undefined}
                  >
                    <td 
                      className="tool-metrics-cell-name" 
                      title={name}
                    >
                      {name.length > 28 ? name.slice(0, 28) + '...' : name}
                    </td>
                    <td className="tool-metrics-cell-numeric">
                      {metrics.total_ops.toLocaleString()}
                    </td>
                    <td className="tool-metrics-cell-numeric tool-metrics-cell-success">
                      {metrics.success_ops.toLocaleString()}
                    </td>
                    <td className={`tool-metrics-cell-numeric ${metrics.failure_ops > 0 ? 'tool-metrics-cell-error' : ''}`}>
                      {metrics.failure_ops.toLocaleString()}
                    </td>
                    <td className="tool-metrics-cell-numeric">
                      <span className={`tool-metrics-rate-badge ${getSuccessRateClass(successRate)}`}>
                        {successRate.toFixed(1)}%
                      </span>
                    </td>
                    <td className="tool-metrics-cell-numeric tool-metrics-cell-latency">
                      {formatLatency(metrics.latency_p50)}
                    </td>
                    <td className="tool-metrics-cell-numeric tool-metrics-cell-latency">
                      {formatLatency(metrics.latency_p95)}
                    </td>
                    <td className="tool-metrics-cell-numeric tool-metrics-cell-latency">
                      {formatLatency(metrics.latency_p99)}
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* Charts Grid */}
      <ErrorBoundary fallback={
        <div className="tool-metrics-chart-error">
          <Icon name="alert-triangle" size="lg" />
          <p>Charts unavailable</p>
        </div>
      }>
        <div className="tool-metrics-charts-grid">
          <CallDistributionChart 
            data={pieData} 
            onToolClick={onToolClick} 
            chartRef={pieChartRef}
            runId={runId}
          />
          <SlowestToolsChart 
            data={slowestToolsData} 
            chartRef={barChartRef}
            runId={runId}
          />
        </div>
      </ErrorBoundary>
    </div>
  );
}

export const ToolMetricsDashboard = memo(ToolMetricsDashboardComponent);
