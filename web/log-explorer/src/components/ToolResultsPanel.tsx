import { useState, useEffect, useCallback, useMemo, memo } from 'react';
import type { OperationLog } from '../types';
import { Icon } from './Icon';
import { formatTimestamp, formatLatency } from '../utils/formatting';

interface ToolResultsPanelProps {
  runId: string;
  toolFilter?: string;
  logs?: OperationLog[];
}

interface ResultFilters {
  toolName: string;
  status: 'all' | 'success' | 'error';
  minLatency: number;
  maxLatency: number;
}

interface GroupedResults {
  toolName: string;
  count: number;
  successCount: number;
  errorCount: number;
  avgLatency: number;
  logs: OperationLog[];
}

const API_BASE = '';

function exportAsJson(data: unknown, filename: string) {
  const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

function escapeCsvCell(value: string | number | boolean): string {
  const str = String(value);
  const needsQuoting = str.includes(',') || str.includes('"') || str.includes('\n');
  const formulaChars = ['=', '+', '-', '@', '\t', '\r'];
  const startsWithFormula = formulaChars.some(c => str.startsWith(c));
  
  if (startsWithFormula) {
    return `"'${str.replace(/"/g, '""')}"`;
  }
  if (needsQuoting) {
    return `"${str.replace(/"/g, '""')}"`;
  }
  return str;
}

function exportAsCsv(logs: OperationLog[], filename: string) {
  const headers = ['timestamp', 'operation', 'tool_name', 'latency_ms', 'ok', 'error_type', 'error_code'];
  const rows = logs.map(log => [
    escapeCsvCell(new Date(log.timestamp_ms).toISOString()),
    escapeCsvCell(log.operation),
    escapeCsvCell(log.tool_name || ''),
    escapeCsvCell(log.latency_ms),
    escapeCsvCell(log.ok ? 'true' : 'false'),
    escapeCsvCell(log.error_type || ''),
    escapeCsvCell(log.error_code || ''),
  ]);
  
  const csv = [headers.join(','), ...rows.map(row => row.join(','))].join('\n');
  const blob = new Blob([csv], { type: 'text/csv' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

function ResultCard({ log, onCopy }: { log: OperationLog; onCopy: () => void }) {
  const [expanded, setExpanded] = useState(false);

  return (
    <div className={`result-card ${log.ok ? 'result-success' : 'result-error'}`}>
      <div 
        className="result-card-header"
        onClick={() => setExpanded(!expanded)}
        role="button"
        tabIndex={0}
        onKeyDown={e => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            setExpanded(!expanded);
          }
        }}
        aria-expanded={expanded}
      >
        <div className="result-card-left">
          <span className={`result-badge ${log.ok ? 'badge-success' : 'badge-error'}`}>
            <Icon name={log.ok ? 'check' : 'x'} size="xs" aria-hidden={true} />
            {log.ok ? 'Success' : 'Error'}
          </span>
          <span className="result-tool-name">{log.tool_name || log.operation}</span>
        </div>
        <div className="result-card-right">
          <span className={`result-latency ${log.latency_ms > 1000 ? 'latency-slow' : ''}`}>
            {formatLatency(log.latency_ms)}
          </span>
          <span className="result-timestamp">{formatTimestamp(log.timestamp_ms)}</span>
          <Icon 
            name={expanded ? 'chevron-up' : 'chevron-down'} 
            size="sm" 
            aria-hidden={true}
          />
        </div>
      </div>

      {expanded && (
        <div className="result-card-body">
          <div className="result-details">
            <div className="result-detail-row">
              <span className="result-detail-label">Operation</span>
              <code className="result-detail-value">{log.operation}</code>
            </div>
            <div className="result-detail-row">
              <span className="result-detail-label">Worker</span>
              <code className="result-detail-value">{log.worker_id}</code>
            </div>
            <div className="result-detail-row">
              <span className="result-detail-label">VU ID</span>
              <code className="result-detail-value">{log.vu_id}</code>
            </div>
            <div className="result-detail-row">
              <span className="result-detail-label">Session</span>
              <code className="result-detail-value">{log.session_id}</code>
            </div>
            {log.stage && (
              <div className="result-detail-row">
                <span className="result-detail-label">Stage</span>
                <code className="result-detail-value">{log.stage}</code>
              </div>
            )}
            {!log.ok && (
              <>
                <div className="result-detail-row result-error-row">
                  <span className="result-detail-label">Error Type</span>
                  <code className="result-detail-value error-value">{log.error_type}</code>
                </div>
                {log.error_code && (
                  <div className="result-detail-row result-error-row">
                    <span className="result-detail-label">Error Code</span>
                    <code className="result-detail-value error-value">{log.error_code}</code>
                  </div>
                )}
              </>
            )}
          </div>
          <div className="result-card-actions">
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                onCopy();
              }}
              className="btn btn-ghost btn-sm"
              aria-label="Copy result to clipboard"
            >
              <Icon name="copy" size="sm" aria-hidden={true} />
              Copy
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

function ToolResultsPanelComponent({ runId, toolFilter, logs: externalLogs }: ToolResultsPanelProps) {
  const [logs, setLogs] = useState<OperationLog[]>(externalLogs || []);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [copyFeedback, setCopyFeedback] = useState(false);
  const [viewMode, setViewMode] = useState<'list' | 'grouped'>('list');
  const [filters, setFilters] = useState<ResultFilters>({
    toolName: toolFilter || '',
    status: 'all',
    minLatency: 0,
    maxLatency: 10000,
  });

  const fetchLogs = useCallback(async () => {
    if (!runId) return;

    setLoading(true);
    setError(null);

    try {
      const params = new URLSearchParams({
        limit: '500',
        operation: 'tools/call',
      });
      
      if (filters.toolName) {
        params.set('tool_name', filters.toolName);
      }

      const response = await fetch(`${API_BASE}/runs/${runId}/logs?${params}`);
      if (!response.ok) {
        throw new Error(`HTTP ${response.status}: ${response.statusText}`);
      }

      const data = await response.json();
      setLogs(data.logs || []);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch logs');
    } finally {
      setLoading(false);
    }
  }, [runId, filters.toolName]);

  useEffect(() => {
    if (!externalLogs) {
      fetchLogs();
    }
  }, [fetchLogs, externalLogs]);

  useEffect(() => {
    if (externalLogs) {
      setLogs(externalLogs);
    }
  }, [externalLogs]);

  useEffect(() => {
    if (toolFilter) {
      setFilters(prev => ({ ...prev, toolName: toolFilter }));
    }
  }, [toolFilter]);

  const filteredLogs = useMemo(() => {
    return logs.filter(log => {
      if (filters.toolName && log.tool_name !== filters.toolName) return false;
      if (filters.status === 'success' && !log.ok) return false;
      if (filters.status === 'error' && log.ok) return false;
      if (log.latency_ms < filters.minLatency) return false;
      if (log.latency_ms > filters.maxLatency) return false;
      return true;
    });
  }, [logs, filters]);

  const groupedResults = useMemo<GroupedResults[]>(() => {
    const groups = new Map<string, OperationLog[]>();
    
    for (const log of filteredLogs) {
      const key = log.tool_name || log.operation;
      if (!groups.has(key)) {
        groups.set(key, []);
      }
      groups.get(key)!.push(log);
    }

    return Array.from(groups.entries()).map(([toolName, toolLogs]) => {
      const successCount = toolLogs.filter(l => l.ok).length;
      const avgLatency = toolLogs.reduce((sum, l) => sum + l.latency_ms, 0) / toolLogs.length;
      return {
        toolName,
        count: toolLogs.length,
        successCount,
        errorCount: toolLogs.length - successCount,
        avgLatency,
        logs: toolLogs,
      };
    }).sort((a, b) => b.count - a.count);
  }, [filteredLogs]);

  const uniqueTools = useMemo(() => {
    const tools = new Set<string>();
    logs.forEach(log => {
      if (log.tool_name) tools.add(log.tool_name);
    });
    return Array.from(tools).sort();
  }, [logs]);

  const latencyRange = useMemo(() => {
    if (logs.length === 0) return { min: 0, max: 10000 };
    const latencies = logs.map(l => l.latency_ms);
    return {
      min: Math.floor(Math.min(...latencies)),
      max: Math.ceil(Math.max(...latencies)),
    };
  }, [logs]);

  const handleCopyResult = useCallback(async (log: OperationLog) => {
    try {
      await navigator.clipboard.writeText(JSON.stringify(log, null, 2));
      setCopyFeedback(true);
      setTimeout(() => setCopyFeedback(false), 2000);
    } catch {
      console.error('Failed to copy to clipboard');
    }
  }, []);

  const summary = useMemo(() => {
    const total = filteredLogs.length;
    const success = filteredLogs.filter(l => l.ok).length;
    const errors = total - success;
    const avgLatency = total > 0 
      ? filteredLogs.reduce((sum, l) => sum + l.latency_ms, 0) / total 
      : 0;
    return { total, success, errors, avgLatency };
  }, [filteredLogs]);

  return (
    <div className="tool-results-panel" role="region" aria-label="Tool Execution Results">
      <div className="results-header">
        <div className="results-title">
          <Icon name="list" size="md" aria-hidden={true} />
          <h3>Tool Execution Results</h3>
          {filteredLogs.length > 0 && (
            <span className="results-count">{filteredLogs.length} results</span>
          )}
        </div>
        <div className="results-actions">
          <div className="view-toggle" role="tablist">
            <button
              type="button"
              role="tab"
              aria-selected={viewMode === 'list'}
              onClick={() => setViewMode('list')}
              className={`btn btn-sm ${viewMode === 'list' ? 'btn-primary' : 'btn-ghost'}`}
            >
              <Icon name="list" size="sm" aria-hidden={true} />
              List
            </button>
            <button
              type="button"
              role="tab"
              aria-selected={viewMode === 'grouped'}
              onClick={() => setViewMode('grouped')}
              className={`btn btn-sm ${viewMode === 'grouped' ? 'btn-primary' : 'btn-ghost'}`}
            >
              <Icon name="folder" size="sm" aria-hidden={true} />
              Grouped
            </button>
          </div>
          <button
            type="button"
            onClick={() => exportAsJson(filteredLogs, `${runId}-results.json`)}
            disabled={filteredLogs.length === 0}
            className="btn btn-ghost btn-sm"
            aria-label="Export as JSON"
          >
            <Icon name="download" size="sm" aria-hidden={true} />
            JSON
          </button>
          <button
            type="button"
            onClick={() => exportAsCsv(filteredLogs, `${runId}-results.csv`)}
            disabled={filteredLogs.length === 0}
            className="btn btn-ghost btn-sm"
            aria-label="Export as CSV"
          >
            <Icon name="download" size="sm" aria-hidden={true} />
            CSV
          </button>
          <button
            type="button"
            onClick={fetchLogs}
            disabled={loading}
            className="btn btn-secondary btn-sm"
            aria-label="Refresh results"
          >
            <Icon name={loading ? 'loader' : 'refresh'} size="sm" aria-hidden={true} />
          </button>
        </div>
      </div>

      {copyFeedback && (
        <div className="copy-feedback" role="status" aria-live="polite">
          <Icon name="check" size="sm" aria-hidden={true} />
          Copied to clipboard
        </div>
      )}

      {error && (
        <div className="results-error" role="alert">
          <Icon name="alert-triangle" size="sm" aria-hidden={true} />
          <span>{error}</span>
          <button type="button" onClick={fetchLogs} className="btn btn-ghost btn-xs">
            Retry
          </button>
        </div>
      )}

      <div className="results-filters">
        <div className="filter-group">
          <label htmlFor="filter-tool">Tool</label>
          <select
            id="filter-tool"
            value={filters.toolName}
            onChange={e => setFilters(prev => ({ ...prev, toolName: e.target.value }))}
            className="select-input"
          >
            <option value="">All Tools</option>
            {uniqueTools.map(tool => (
              <option key={tool} value={tool}>{tool}</option>
            ))}
          </select>
        </div>
        <div className="filter-group">
          <label htmlFor="filter-status">Status</label>
          <select
            id="filter-status"
            value={filters.status}
            onChange={e => setFilters(prev => ({ ...prev, status: e.target.value as ResultFilters['status'] }))}
            className="select-input"
          >
            <option value="all">All</option>
            <option value="success">Success Only</option>
            <option value="error">Errors Only</option>
          </select>
        </div>
        <div className="filter-group filter-latency">
          <label>Latency Range (ms)</label>
          <div className="latency-range-inputs">
            <input
              type="number"
              value={filters.minLatency}
              onChange={e => setFilters(prev => ({ ...prev, minLatency: parseInt(e.target.value) || 0 }))}
              min={0}
              max={filters.maxLatency}
              className="input input-small"
              aria-label="Minimum latency"
            />
            <span>—</span>
            <input
              type="number"
              value={filters.maxLatency}
              onChange={e => setFilters(prev => ({ ...prev, maxLatency: parseInt(e.target.value) || latencyRange.max }))}
              min={filters.minLatency}
              className="input input-small"
              aria-label="Maximum latency"
            />
          </div>
        </div>
      </div>

      <div className="results-summary">
        <div className="summary-stat">
          <span className="summary-value">{summary.total}</span>
          <span className="summary-label">Total</span>
        </div>
        <div className="summary-stat stat-success">
          <span className="summary-value">{summary.success}</span>
          <span className="summary-label">Success</span>
        </div>
        <div className="summary-stat stat-error">
          <span className="summary-value">{summary.errors}</span>
          <span className="summary-label">Errors</span>
        </div>
        <div className="summary-stat">
          <span className="summary-value">{formatLatency(summary.avgLatency)}</span>
          <span className="summary-label">Avg Latency</span>
        </div>
      </div>

      {loading && filteredLogs.length === 0 && (
        <div className="results-loading" role="status" aria-live="polite">
          <div className="spinner" aria-hidden={true} />
          <span>Loading results...</span>
        </div>
      )}

      {!loading && filteredLogs.length === 0 && (
        <div className="results-empty">
          <Icon name="inbox" size="xl" aria-hidden={true} />
          <p>No results found</p>
          <p className="muted">
            {logs.length > 0 
              ? 'Try adjusting your filters' 
              : 'Run a test to see execution results'}
          </p>
        </div>
      )}

      {viewMode === 'list' && filteredLogs.length > 0 && (
        <div className="results-list" role="list">
          {filteredLogs.slice(0, 100).map((log, idx) => (
            <ResultCard 
              key={`${log.timestamp_ms}-${idx}`} 
              log={log} 
              onCopy={() => handleCopyResult(log)}
            />
          ))}
          {filteredLogs.length > 100 && (
            <div className="results-truncated" role="status">
              Showing first 100 of {filteredLogs.length} results
            </div>
          )}
        </div>
      )}

      {viewMode === 'grouped' && groupedResults.length > 0 && (
        <div className="results-grouped">
          {groupedResults.map(group => (
            <details key={group.toolName} className="result-group">
              <summary className="result-group-header">
                <div className="result-group-info">
                  <span className="result-group-name">{group.toolName}</span>
                  <span className="result-group-count">{group.count} calls</span>
                </div>
                <div className="result-group-stats">
                  <span className="stat-success">{group.successCount} ok</span>
                  <span className="stat-error">{group.errorCount} err</span>
                  <span className="stat-latency">{formatLatency(group.avgLatency)} avg</span>
                </div>
              </summary>
              <div className="result-group-logs">
                {group.logs.slice(0, 20).map((log, idx) => (
                  <ResultCard 
                    key={`${log.timestamp_ms}-${idx}`} 
                    log={log} 
                    onCopy={() => handleCopyResult(log)}
                  />
                ))}
                {group.logs.length > 20 && (
                  <div className="results-truncated">
                    Showing first 20 of {group.logs.length} results for this tool
                  </div>
                )}
              </div>
            </details>
          ))}
        </div>
      )}
    </div>
  );
}

export const ToolResultsPanel = memo(ToolResultsPanelComponent);
