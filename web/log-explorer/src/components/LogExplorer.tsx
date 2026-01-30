import { useState, useEffect, useCallback, useRef, useMemo } from 'react'
import type { RunInfo, OperationLog, LogFilters, PaginationState } from '../types'
import { fetchRuns, fetchLogs, exportAsJSON, exportAsCSV } from '../api'
import { FilterPanel } from './FilterPanel'
import { LogTable } from './LogTable'
import { ErrorSignatures } from './ErrorSignatures'
import { MetricsDashboard } from './MetricsDashboard'
import { SearchableSelect } from './SearchableSelect'
import { Icon } from './Icon'
import { useToast } from './Toast'

const DEFAULT_LOGS_PER_PAGE = 50;

type ViewTab = 'logs' | 'metrics';

const emptyFilters: LogFilters = {
  stage: '',
  stage_id: '',
  worker_id: '',
  session_id: '',
  vu_id: '',
  operation: '',
  tool_name: '',
  error_type: '',
  error_code: '',
};

interface LogExplorerProps {
  onNavigateToWizard?: () => void;
}

export function LogExplorer({ onNavigateToWizard }: LogExplorerProps) {
  const [runs, setRuns] = useState<RunInfo[]>([]);
  const [selectedRunId, setSelectedRunId] = useState<string>('');
  const [selectedRun, setSelectedRun] = useState<RunInfo | undefined>();
  const [activeTab, setActiveTab] = useState<ViewTab>('metrics');
  const [logs, setLogs] = useState<OperationLog[]>([]);
  const [filters, setFilters] = useState<LogFilters>(emptyFilters);
  const [logsPerPage, setLogsPerPage] = useState(DEFAULT_LOGS_PER_PAGE);
  const [pagination, setPagination] = useState<PaginationState>({
    offset: 0,
    limit: DEFAULT_LOGS_PER_PAGE,
    total: 0,
  });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [runsError, setRunsError] = useState<string | null>(null);
  const [runsLoading, setRunsLoading] = useState(true);
  const abortControllerRef = useRef<AbortController | null>(null);
  const { showToast } = useToast();

  const loadRuns = useCallback(async () => {
    setRunsLoading(true);
    setRunsError(null);
    try {
      const data = await fetchRuns();
      setRuns(data);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to load runs';
      setRunsError(message);
    } finally {
      setRunsLoading(false);
    }
  }, []);

  useEffect(() => {
    loadRuns();
  }, [loadRuns]);

  useEffect(() => {
    if (runs.length > 0 && !selectedRunId) {
      const mostRecentRun = runs[0];
      setSelectedRunId(mostRecentRun.id);
      setSelectedRun(mostRecentRun);
    }
  }, [runs, selectedRunId]);

  const loadLogs = useCallback(async (runId: string, currentFilters: LogFilters, offset: number, limit: number) => {
    if (!runId) return;
    
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
    }
    abortControllerRef.current = new AbortController();
    const signal = abortControllerRef.current.signal;
    
    setLoading(true);
    setError(null);
    
    try {
      const response = await fetchLogs(runId, currentFilters, offset, limit, signal);
      setLogs(response.logs);
      setPagination({
        offset: response.offset,
        limit: response.limit,
        total: response.total,
      });
    } catch (err) {
      if (typeof err === 'object' && err !== null && 'name' in err && (err as { name: string }).name === 'AbortError') {
        return;
      }
      setError(err instanceof Error ? err.message : 'Failed to load logs');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (selectedRunId && activeTab === 'logs') {
      loadLogs(selectedRunId, filters, 0, logsPerPage);
    }
    
    return () => {
      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
      }
    };
  }, [selectedRunId, filters, loadLogs, activeTab, logsPerPage]);

  const handleRunChange = useCallback((runId: string) => {
    setSelectedRunId(runId);
    setSelectedRun(runs.find(r => r.id === runId));
    setFilters(emptyFilters);
    setPagination(prev => ({ ...prev, offset: 0 }));
  }, [runs]);

  const runOptions = useMemo(() => 
    runs.map(run => ({
      value: run.id,
      label: run.id,
      sublabel: `${run.scenario_id} • ${run.state}`,
    })),
    [runs]
  );

  const handleFilterChange = useCallback((newFilters: LogFilters) => {
    setFilters(newFilters);
    setPagination(prev => ({ ...prev, offset: 0 }));
  }, []);

  const handlePageChange = useCallback((newOffset: number) => {
    loadLogs(selectedRunId, filters, newOffset, logsPerPage);
  }, [loadLogs, selectedRunId, filters, logsPerPage]);

  const handleLimitChange = useCallback((newLimit: number) => {
    setLogsPerPage(newLimit);
  }, []);

  const handleExportJSON = useCallback(() => {
    const filename = `logs-${selectedRunId}-${Date.now()}.json`;
    exportAsJSON(logs, filename);
    showToast(`Exported ${logs.length} logs to JSON`, 'success');
  }, [selectedRunId, logs, showToast]);

  const handleExportCSV = useCallback(() => {
    const filename = `logs-${selectedRunId}-${Date.now()}.csv`;
    exportAsCSV(logs, filename);
    showToast(`Exported ${logs.length} logs to CSV`, 'success');
  }, [selectedRunId, logs, showToast]);

  const handleRetryLogs = useCallback(() => {
    if (selectedRunId) {
      loadLogs(selectedRunId, filters, pagination.offset, logsPerPage);
    }
  }, [selectedRunId, filters, pagination.offset, loadLogs, logsPerPage]);

  return (
    <div className="log-explorer">
      <div className="log-explorer-controls">
        <div className="run-selector">
          <label htmlFor="run-select">Run</label>
          <SearchableSelect
            id="run-select"
            options={runOptions}
            value={selectedRunId}
            onChange={handleRunChange}
            placeholder="Select a run..."
            disabled={runsLoading}
          />
        </div>

        {selectedRunId && (
          <>
            <div className="view-tabs" role="group" aria-label="View options">
              <button
                className={`view-tab ${activeTab === 'logs' ? 'active' : ''}`}
                onClick={() => setActiveTab('logs')}
                aria-pressed={activeTab === 'logs'}
              >
                <span className="tab-icon" aria-hidden="true"><Icon name="clipboard" size="sm" /></span>
                Logs
              </button>
              <button
                className={`view-tab ${activeTab === 'metrics' ? 'active' : ''}`}
                onClick={() => setActiveTab('metrics')}
                aria-pressed={activeTab === 'metrics'}
              >
                <span className="tab-icon" aria-hidden="true"><Icon name="chart-bar" size="sm" /></span>
                Metrics
              </button>
            </div>

            {activeTab === 'logs' && (
              <div className="export-buttons">
                <button onClick={handleExportJSON} className="btn btn-secondary" disabled={logs.length === 0}>
                  Export JSON
                </button>
                <button onClick={handleExportCSV} className="btn btn-secondary" disabled={logs.length === 0}>
                  Export CSV
                </button>
              </div>
            )}
          </>
        )}
      </div>

      {selectedRunId && activeTab === 'logs' && (
        <>
          <FilterPanel filters={filters} onChange={handleFilterChange} />
          
          <div className="log-explorer-content">
            <div className="log-table-container">
              {error && (
                <div className="error-state" role="alert">
                  <div className="error-state-icon" aria-hidden="true"><Icon name="alert-triangle" size="xl" /></div>
                  <h3>Failed to Load Logs</h3>
                  <p className="error-state-message">
                    We couldn't retrieve the log data. This might be a temporary issue with the server.
                  </p>
                  <code className="error-state-details">{error}</code>
                  <button 
                    type="button" 
                    className="btn-retry" 
                    onClick={handleRetryLogs}
                    disabled={loading}
                  >
                    <span className="retry-icon" aria-hidden="true"><Icon name="refresh" size="md" /></span>
                    {loading ? 'Retrying...' : 'Try Again'}
                  </button>
                </div>
              )}
              
              {!error && (
                <LogTable
                  logs={logs}
                  loading={loading}
                  pagination={pagination}
                  onPageChange={handlePageChange}
                  onLimitChange={handleLimitChange}
                />
              )}
            </div>

            <aside className="error-signatures-sidebar">
              <ErrorSignatures runId={selectedRunId} />
            </aside>
          </div>
        </>
      )}

      {selectedRunId && activeTab === 'metrics' && (
        <MetricsDashboard runId={selectedRunId} run={selectedRun} />
      )}

      {!selectedRunId && (
        <div className="empty-state" role="status">
          {runsError ? (
            <>
              <div className="error-state-icon" aria-hidden="true"><Icon name="alert-triangle" size="xl" /></div>
              <h2>Unable to Load Runs</h2>
              <p className="error-state-message">
                We couldn't connect to the server to retrieve your runs.
              </p>
              <code className="error-state-details">{runsError}</code>
              <button 
                type="button" 
                className="btn-retry" 
                onClick={loadRuns}
                disabled={runsLoading}
              >
                <span className="retry-icon" aria-hidden="true"><Icon name="refresh" size="md" /></span>
                {runsLoading ? 'Retrying...' : 'Try Again'}
              </button>
            </>
          ) : runsLoading ? (
            <>
              <div className="spinner" aria-hidden="true" />
              <p>Loading runs...</p>
            </>
          ) : runs.length === 0 ? (
            <>
              <div className="empty-state-icon" aria-hidden="true"><Icon name="rocket" size="xl" /></div>
              <h2>No Runs Yet</h2>
              <p>Start by creating your first load test to see results here.</p>
              <div className="empty-state-cta">
                <button 
                  type="button"
                  className="btn btn-primary" 
                  onClick={onNavigateToWizard}
                >
                  <Icon name="plus" size="sm" aria-hidden={true} /> Create New Run
                </button>
              </div>
            </>
          ) : (
            <>
              <div className="empty-state-icon" aria-hidden="true"><Icon name="clipboard" size="xl" /></div>
              <h2>Select a Run</h2>
              <p>Choose a run from the dropdown above to explore its logs and metrics.</p>
              <p className="empty-state-hint">
                You have {runs.length} run{runs.length !== 1 ? 's' : ''} available
              </p>
            </>
          )}
        </div>
      )}
    </div>
  )
}
