import { useState, useEffect, useCallback, useRef, useMemo } from 'react'
import { useParams, useNavigate, useLocation, useSearchParams } from 'react-router-dom'
import type { RunInfo, OperationLog, LogFilters, PaginationState } from '../types'
import { fetchRuns, fetchLogs, exportAsJSON, exportAsCSV } from '../api/index'
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

export function LogExplorer() {
  const { runId: urlRunId } = useParams<{ runId?: string }>();
  const navigate = useNavigate();
  const location = useLocation();
  const [searchParams, setSearchParams] = useSearchParams();
  
  const [runs, setRuns] = useState<RunInfo[]>([]);
  const [selectedRunId, setSelectedRunId] = useState<string>(urlRunId || '');
  const [selectedRun, setSelectedRun] = useState<RunInfo | undefined>();
  
  const getActiveTabFromPath = (): ViewTab => {
    if (location.pathname.endsWith('/logs')) return 'logs';
    return 'metrics';
  };
  const [activeTab, setActiveTab] = useState<ViewTab>(getActiveTabFromPath);
  const [logs, setLogs] = useState<OperationLog[]>([]);
  
  const getFiltersFromParams = useCallback((params: URLSearchParams): LogFilters => ({
    stage: params.get('stage') || '',
    stage_id: params.get('stage_id') || '',
    worker_id: params.get('worker_id') || '',
    session_id: params.get('session_id') || '',
    vu_id: params.get('vu_id') || '',
    operation: params.get('operation') || '',
    tool_name: params.get('tool_name') || '',
    error_type: params.get('error_type') || '',
    error_code: params.get('error_code') || '',
  }), []);

  const [filters, setFilters] = useState<LogFilters>(() => getFiltersFromParams(searchParams));
  const prevSearchParamsRef = useRef(searchParams.toString());

  useEffect(() => {
    const currentParamsString = searchParams.toString();
    if (currentParamsString !== prevSearchParamsRef.current) {
      prevSearchParamsRef.current = currentParamsString;
      setFilters(getFiltersFromParams(searchParams));
    }
  }, [searchParams, getFiltersFromParams]);
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
    if (urlRunId) {
      setSelectedRunId(urlRunId);
      const run = runs.find(r => r.id === urlRunId);
      if (run) setSelectedRun(run);
    } else if (runs.length > 0 && !selectedRunId) {
      const mostRecentRun = runs[0];
      setSelectedRunId(mostRecentRun.id);
      setSelectedRun(mostRecentRun);
    }
  }, [runs, selectedRunId, urlRunId]);

  useEffect(() => {
    setActiveTab(getActiveTabFromPath());
  }, [location.pathname]);

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
    prevSearchParamsRef.current = '';
    setSearchParams(new URLSearchParams(), { replace: true });
    const tab = activeTab === 'logs' ? '/logs' : '/metrics';
    navigate(`/runs/${runId}${tab}`);
  }, [runs, activeTab, navigate, setSearchParams]);

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
    
    const params = new URLSearchParams();
    Object.entries(newFilters).forEach(([key, value]) => {
      if (value) params.set(key, value);
    });
    prevSearchParamsRef.current = params.toString();
    setSearchParams(params, { replace: true });
  }, [setSearchParams]);

  const handlePageChange = useCallback((newOffset: number) => {
    loadLogs(selectedRunId, filters, newOffset, logsPerPage);
  }, [loadLogs, selectedRunId, filters, logsPerPage]);

  const handleLimitChange = useCallback((newLimit: number) => {
    setLogsPerPage(newLimit);
  }, []);

  const handleLogFilterClick = useCallback((key: keyof LogFilters, value: string) => {
    const newFilters = { ...filters, [key]: value };
    handleFilterChange(newFilters);
  }, [filters, handleFilterChange]);

  const handleNavigateToLogs = useCallback((key: keyof LogFilters, value: string) => {
    setActiveTab('logs');
    
    const params = new URLSearchParams();
    params.set(key, value);
    prevSearchParamsRef.current = params.toString();
    setSearchParams(params, { replace: true });
    navigate(`/runs/${selectedRunId}/logs?${params.toString()}`);
    
    setFilters({ ...emptyFilters, [key]: value });
    setPagination(prev => ({ ...prev, offset: 0 }));
  }, [selectedRunId, navigate, setSearchParams]);

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

  const handleTabChange = useCallback((tab: ViewTab) => {
    setActiveTab(tab);
    if (selectedRunId) {
      const tabPath = tab === 'logs' ? '/logs' : '/metrics';
      const params = searchParams.toString();
      navigate(`/runs/${selectedRunId}${tabPath}${params ? `?${params}` : ''}`);
    }
  }, [selectedRunId, navigate, searchParams]);

  const handleNavigateToWizard = useCallback(() => {
    navigate('/wizard');
  }, [navigate]);

  const handleErrorClick = useCallback((errorType: string) => {
    handleNavigateToLogs('error_type', errorType);
  }, [handleNavigateToLogs]);

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
                onClick={() => handleTabChange('logs')}
                aria-pressed={activeTab === 'logs'}
              >
                <span className="tab-icon" aria-hidden="true"><Icon name="clipboard" size="sm" /></span>
                Logs
              </button>
              <button
                className={`view-tab ${activeTab === 'metrics' ? 'active' : ''}`}
                onClick={() => handleTabChange('metrics')}
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
                  onFilterClick={handleLogFilterClick}
                />
              )}
            </div>

            <aside className="error-signatures-sidebar">
              <ErrorSignatures runId={selectedRunId} onErrorClick={handleErrorClick} />
            </aside>
          </div>
        </>
      )}

      {selectedRunId && activeTab === 'metrics' && (
        <MetricsDashboard runId={selectedRunId} run={selectedRun} onNavigateToWizard={handleNavigateToWizard} onNavigateToLogs={handleNavigateToLogs} />
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
                  onClick={handleNavigateToWizard}
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
