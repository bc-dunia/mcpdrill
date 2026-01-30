import { useState, useMemo, memo } from 'react';
import type { ConnectionMetrics } from '../types';
import { Icon } from './Icon';

interface SessionLifecycleTableProps {
  sessions: ConnectionMetrics[];
  loading?: boolean;
}

type SortField = 'session_id' | 'state' | 'request_count' | 'error_count' | 'avg_latency_ms' | 'reconnect_count';
type SortDirection = 'asc' | 'desc';

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms.toFixed(0)}ms`;
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
  return `${(ms / 60000).toFixed(1)}m`;
}

function getStateColor(state: string): string {
  switch (state) {
    case 'active':
      return '#4ade80';
    case 'terminated':
      return '#94a3b8';
    case 'dropped':
      return '#f87171';
    default:
      return '#94a3b8';
  }
}

function SessionLifecycleTableComponent({ sessions, loading }: SessionLifecycleTableProps) {
  const [sortField, setSortField] = useState<SortField>('request_count');
  const [sortDirection, setSortDirection] = useState<SortDirection>('desc');
  const [filter, setFilter] = useState<'all' | 'active' | 'dropped' | 'terminated'>('all');

  const handleSort = (field: SortField) => {
    if (sortField === field) {
      setSortDirection(sortDirection === 'asc' ? 'desc' : 'asc');
    } else {
      setSortField(field);
      setSortDirection('desc');
    }
  };

  const filteredAndSortedSessions = useMemo(() => {
    let filtered = sessions;
    if (filter !== 'all') {
      filtered = sessions.filter((s) => s.state === filter);
    }

    return [...filtered].sort((a, b) => {
      let aVal: number | string;
      let bVal: number | string;

      switch (sortField) {
        case 'session_id':
          aVal = a.session_id;
          bVal = b.session_id;
          break;
        case 'state':
          aVal = a.state;
          bVal = b.state;
          break;
        case 'request_count':
          aVal = a.request_count;
          bVal = b.request_count;
          break;
        case 'error_count':
          aVal = a.error_count;
          bVal = b.error_count;
          break;
        case 'avg_latency_ms':
          aVal = a.avg_latency_ms;
          bVal = b.avg_latency_ms;
          break;
        case 'reconnect_count':
          aVal = a.reconnect_count;
          bVal = b.reconnect_count;
          break;
        default:
          return 0;
      }

      if (typeof aVal === 'string' && typeof bVal === 'string') {
        return sortDirection === 'asc' ? aVal.localeCompare(bVal) : bVal.localeCompare(aVal);
      }
      return sortDirection === 'asc' ? (aVal as number) - (bVal as number) : (bVal as number) - (aVal as number);
    });
  }, [sessions, sortField, sortDirection, filter]);

  const stats = useMemo(() => {
    const active = sessions.filter((s) => s.state === 'active').length;
    const dropped = sessions.filter((s) => s.state === 'dropped').length;
    const terminated = sessions.filter((s) => s.state === 'terminated').length;
    return { total: sessions.length, active, dropped, terminated };
  }, [sessions]);

  if (loading) {
    return (
      <div className="session-lifecycle-container" role="region" aria-label="Session Lifecycle">
        <div className="session-lifecycle-header">
          <h3><Icon name="users" size="lg" aria-hidden={true} /> Session Lifecycle</h3>
        </div>
        <div className="session-lifecycle-loading">
          <div className="spinner" aria-hidden="true" />
          <span>Loading sessions...</span>
        </div>
      </div>
    );
  }

  if (!sessions.length) {
    return (
      <div className="session-lifecycle-container" role="region" aria-label="Session Lifecycle">
        <div className="session-lifecycle-header">
          <h3><Icon name="users" size="lg" aria-hidden={true} /> Session Lifecycle</h3>
        </div>
        <div className="session-lifecycle-empty">
          <Icon name="users" size="xl" aria-hidden={true} />
          <span>No session data available</span>
        </div>
      </div>
    );
  }

  return (
    <div className="session-lifecycle-container" role="region" aria-label="Session Lifecycle">
      <div className="session-lifecycle-header">
        <h3><Icon name="users" size="lg" aria-hidden={true} /> Session Lifecycle</h3>
        <div className="session-stats">
          <span className="stat-badge stat-total">{stats.total} total</span>
          <span className="stat-badge stat-active">{stats.active} active</span>
          <span className="stat-badge stat-dropped">{stats.dropped} dropped</span>
          <span className="stat-badge stat-terminated">{stats.terminated} ended</span>
        </div>
      </div>
      <div className="session-lifecycle-filters">
        <button
          className={`filter-btn ${filter === 'all' ? 'active' : ''}`}
          onClick={() => setFilter('all')}
        >
          All
        </button>
        <button
          className={`filter-btn ${filter === 'active' ? 'active' : ''}`}
          onClick={() => setFilter('active')}
        >
          Active
        </button>
        <button
          className={`filter-btn ${filter === 'dropped' ? 'active' : ''}`}
          onClick={() => setFilter('dropped')}
        >
          Dropped
        </button>
        <button
          className={`filter-btn ${filter === 'terminated' ? 'active' : ''}`}
          onClick={() => setFilter('terminated')}
        >
          Ended
        </button>
      </div>
      <div className="session-lifecycle-table-wrapper">
        <table className="session-lifecycle-table">
          <thead>
            <tr>
              <th onClick={() => handleSort('session_id')} className="sortable" title="Unique identifier for this MCP session">
                Session ID {sortField === 'session_id' && (sortDirection === 'asc' ? '↑' : '↓')}
              </th>
              <th onClick={() => handleSort('state')} className="sortable" title="Current state: active, dropped, or terminated">
                State {sortField === 'state' && (sortDirection === 'asc' ? '↑' : '↓')}
              </th>
              <th onClick={() => handleSort('request_count')} className="sortable" title="Total number of requests made in this session">
                Requests {sortField === 'request_count' && (sortDirection === 'asc' ? '↑' : '↓')}
              </th>
              <th onClick={() => handleSort('error_count')} className="sortable" title="Number of failed requests in this session">
                Errors {sortField === 'error_count' && (sortDirection === 'asc' ? '↑' : '↓')}
              </th>
              <th onClick={() => handleSort('avg_latency_ms')} className="sortable" title="Average response time for this session">
                Avg Latency {sortField === 'avg_latency_ms' && (sortDirection === 'asc' ? '↑' : '↓')}
              </th>
              <th onClick={() => handleSort('reconnect_count')} className="sortable" title="Number of times this session reconnected">
                Reconnects {sortField === 'reconnect_count' && (sortDirection === 'asc' ? '↑' : '↓')}
              </th>
            </tr>
          </thead>
          <tbody>
            {filteredAndSortedSessions.slice(0, 50).map((session) => (
              <tr key={session.session_id} className={`session-row state-${session.state}`}>
                <td className="session-id" title={session.session_id}>
                  {session.session_id.slice(0, 12)}...
                </td>
                <td>
                  <span className="state-badge" style={{ color: getStateColor(session.state) }}>
                    {session.state}
                  </span>
                </td>
                <td>{session.request_count.toLocaleString()}</td>
                <td className={session.error_count > 0 ? 'error-cell' : ''}>
                  {session.error_count.toLocaleString()}
                </td>
                <td>{formatDuration(session.avg_latency_ms)}</td>
                <td className={session.reconnect_count > 0 ? 'warn-cell' : ''}>
                  {session.reconnect_count}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {filteredAndSortedSessions.length > 50 && (
          <div className="table-truncated-notice">
            Showing 50 of {filteredAndSortedSessions.length} sessions
          </div>
        )}
      </div>
    </div>
  );
}

export const SessionLifecycleTable = memo(SessionLifecycleTableComponent);
