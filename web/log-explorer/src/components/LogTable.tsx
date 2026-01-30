import { memo, useState } from 'react';
import type { OperationLog, PaginationState } from '../types'
import { SkeletonTable } from './Skeleton'
import { Icon } from './Icon';

interface LogTableProps {
  logs: OperationLog[];
  loading: boolean;
  pagination: PaginationState;
  onPageChange: (offset: number) => void;
  onLimitChange?: (limit: number) => void;
}

function formatTimestamp(ms: number | undefined | null): string {
  if (ms === undefined || ms === null || isNaN(ms)) return '—';
  const date = new Date(ms);
  if (isNaN(date.getTime())) return '—';
  return date.toLocaleString('en-US', {
    month: 'short',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  });
}

function formatLatency(ms: number | undefined | null): string {
  if (ms === undefined || ms === null || isNaN(ms)) return '—';
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(2)}s`;
}

function LogTableComponent({ logs, loading, pagination, onPageChange, onLimitChange }: LogTableProps) {
  const { offset, limit, total } = pagination;
  const currentPage = Math.floor(offset / limit) + 1;
  const totalPages = Math.ceil(total / limit);
  const canGoPrev = offset > 0;
  const canGoNext = offset + limit < total;
  const [jumpPage, setJumpPage] = useState(String(currentPage));

  const handlePrev = () => {
    if (canGoPrev) {
      onPageChange(Math.max(0, offset - limit));
    }
  };

  const handleNext = () => {
    if (canGoNext) {
      onPageChange(offset + limit);
    }
  };

  if (loading) {
    return <SkeletonTable rows={8} columns={6} />;
  }

  if (logs.length === 0) {
    return (
      <div className="log-table-empty" role="status">
        <div className="empty-icon" aria-hidden="true"><Icon name="search" size="xl" /></div>
        <p>No logs found matching your filters.</p>
      </div>
    );
  }

  return (
    <div className="log-table-wrapper">
      <div className="log-table-scroll">
        <table className="log-table">
          <caption className="sr-only">
            Operation logs showing timestamp, operation type, tool name, latency, status, and error information
          </caption>
          <thead>
            <tr>
              <th scope="col">Timestamp</th>
              <th scope="col">Operation</th>
              <th scope="col">Tool</th>
              <th scope="col">Latency</th>
              <th scope="col">Status</th>
              <th scope="col">Error</th>
            </tr>
          </thead>
          <tbody>
            {logs.map((log, idx) => (
              <tr 
                key={`${log.timestamp_ms}-${idx}`} 
                className={log.ok ? '' : 'row-error'}
                tabIndex={0}
                aria-label={`${log.operation} at ${formatTimestamp(log.timestamp_ms)}, ${log.ok ? 'successful' : 'failed'}, latency ${formatLatency(log.latency_ms)}`}
              >
                <td className="cell-timestamp">
                  <span className="timestamp-value">{formatTimestamp(log.timestamp_ms)}</span>
                </td>
                <td className="cell-operation">
                  <code>{log.operation}</code>
                </td>
                <td className="cell-tool">
                  {log.tool_name ? <code>{log.tool_name}</code> : <span className="muted">—</span>}
                </td>
                <td className="cell-latency">
                  <span className={`latency-badge ${log.latency_ms > 1000 ? 'latency-slow' : ''}`}>
                    {formatLatency(log.latency_ms)}
                  </span>
                </td>
                <td className="cell-status">
                  <span className={`status-badge ${log.ok ? 'status-ok' : 'status-error'}`}>
                    {log.ok ? 'OK' : 'Error'}
                  </span>
                </td>
                <td className="cell-error">
                  {log.error_type ? (
                    <span className="error-info">
                      <code className="error-type">{log.error_type}</code>
                      {log.error_code && <code className="error-code">{log.error_code}</code>}
                    </span>
                  ) : (
                    <span className="muted">—</span>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <div className="pagination">
        <div className="pagination-info">
          Showing {offset + 1}–{Math.min(offset + logs.length, total)} of {total.toLocaleString()} logs
        </div>
        <div className="pagination-controls">
          {onLimitChange && (
            <div className="pagination-rows-per-page">
              <label htmlFor="rows-per-page" className="pagination-label">Rows:</label>
              <select
                id="rows-per-page"
                value={limit}
                onChange={e => {
                  const newLimit = parseInt(e.target.value);
                  onLimitChange(newLimit);
                  onPageChange(0);
                }}
                className="select-input select-small"
              >
                <option value="25">25</option>
                <option value="50">50</option>
                <option value="100">100</option>
                <option value="200">200</option>
              </select>
            </div>
          )}
          <button
            onClick={handlePrev}
            disabled={!canGoPrev}
            className="btn btn-pagination"
            aria-label="Previous page"
          >
            ← Prev
          </button>
          <div className="pagination-jump">
            <span className="pagination-label">Page</span>
            <input
              type="number"
              min={1}
              max={totalPages}
              value={jumpPage}
              onChange={e => setJumpPage(e.target.value)}
              onKeyDown={e => {
                if (e.key === 'Enter') {
                  const page = parseInt(jumpPage);
                  if (page >= 1 && page <= totalPages) {
                    onPageChange((page - 1) * limit);
                  }
                }
              }}
              onBlur={() => {
                const page = parseInt(jumpPage);
                if (page >= 1 && page <= totalPages) {
                  onPageChange((page - 1) * limit);
                } else {
                  setJumpPage(String(currentPage));
                }
              }}
              className="input input-small page-jump"
              aria-label="Jump to page"
            />
            <span className="pagination-label">of {totalPages}</span>
          </div>
          <button
            onClick={handleNext}
            disabled={!canGoNext}
            className="btn btn-pagination"
            aria-label="Next page"
          >
            Next →
          </button>
        </div>
      </div>
    </div>
  )
}

export const LogTable = memo(LogTableComponent);
