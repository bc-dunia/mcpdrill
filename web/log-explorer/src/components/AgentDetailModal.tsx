import { useState, useEffect, useCallback } from 'react';
import { Icon } from './Icon';
import { fetchAgentDetail, type AgentDetail } from '../api/index';

interface AgentDetailModalProps {
  agentId: string;
  onClose: () => void;
}

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

function formatPercent(value: number): string {
  return `${value.toFixed(1)}%`;
}

export function AgentDetailModal({ agentId, onClose }: AgentDetailModalProps) {
  const [agent, setAgent] = useState<AgentDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const loadAgentDetail = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await fetchAgentDetail(agentId);
      setAgent(data);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load agent details');
    } finally {
      setLoading(false);
    }
  }, [agentId]);

  useEffect(() => {
    loadAgentDetail();
  }, [loadAgentDetail]);

  useEffect(() => {
    const handleEscape = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    document.addEventListener('keydown', handleEscape);
    return () => document.removeEventListener('keydown', handleEscape);
  }, [onClose]);

  const handleBackdropClick = (e: React.MouseEvent) => {
    if (e.target === e.currentTarget) onClose();
  };

  return (
    <div 
      className="modal-overlay" 
      onClick={handleBackdropClick}
      role="dialog"
      aria-modal="true"
      aria-labelledby="agent-detail-title"
    >
      <div className="modal-content agent-detail-modal">
        <div className="modal-header">
          <h2 id="agent-detail-title">
            <Icon name="server" size="md" aria-hidden={true} />
            Agent Details
          </h2>
          <button 
            className="modal-close"
            onClick={onClose}
            aria-label="Close modal"
          >
            <Icon name="x" size="md" />
          </button>
        </div>

        <div className="modal-body">
          {loading && (
            <div className="loading-state">
              <Icon name="loader" size="lg" />
              <span>Loading agent details...</span>
            </div>
          )}

          {error && (
            <div className="error-state">
              <Icon name="alert-triangle" size="lg" />
              <span>{error}</span>
              <button className="btn btn-secondary btn-sm" onClick={loadAgentDetail}>
                <Icon name="refresh" size="sm" /> Retry
              </button>
            </div>
          )}

          {agent && !loading && !error && (
            <div className="agent-detail-content">
              <section className="detail-section">
                <h3>General Information</h3>
                <div className="detail-grid">
                  <div className="detail-item">
                    <span className="detail-label">Agent ID</span>
                    <span className="detail-value monospace">{agent.agent_id}</span>
                  </div>
                  <div className="detail-item">
                    <span className="detail-label">Status</span>
                    <span className={`detail-value status-badge ${agent.online ? 'online' : 'offline'}`}>
                      <span className={`status-dot ${agent.online ? 'online' : 'offline'}`} />
                      {agent.online ? 'Online' : 'Offline'}
                    </span>
                  </div>
                  <div className="detail-item">
                    <span className="detail-label">Pair Key</span>
                    <span className="detail-value">{agent.pair_key}</span>
                  </div>
                  <div className="detail-item">
                    <span className="detail-label">Hostname</span>
                    <span className="detail-value">{agent.hostname}</span>
                  </div>
                  <div className="detail-item">
                    <span className="detail-label">Platform</span>
                    <span className="detail-value">{agent.os}/{agent.arch}</span>
                  </div>
                  {agent.version && (
                    <div className="detail-item">
                      <span className="detail-label">Version</span>
                      <span className="detail-value">{agent.version}</span>
                    </div>
                  )}
                </div>
              </section>

              {agent.process_info && (
                <section className="detail-section">
                  <h3>Process Configuration</h3>
                  <div className="detail-grid">
                    {agent.process_info.pid && (
                      <div className="detail-item">
                        <span className="detail-label">PID</span>
                        <span className="detail-value monospace">{agent.process_info.pid}</span>
                      </div>
                    )}
                    {agent.process_info.listen_port && (
                      <div className="detail-item">
                        <span className="detail-label">Listen Port</span>
                        <span className="detail-value monospace">{agent.process_info.listen_port}</span>
                      </div>
                    )}
                    {agent.process_info.process_regex && (
                      <div className="detail-item full-width">
                        <span className="detail-label">Process Regex</span>
                        <code className="detail-value monospace">{agent.process_info.process_regex}</code>
                      </div>
                    )}
                  </div>
                </section>
              )}

              {agent.tags && Object.keys(agent.tags).length > 0 && (
                <section className="detail-section">
                  <h3>Tags</h3>
                  <div className="tags-list">
                    {Object.entries(agent.tags).map(([key, value]) => (
                      <span key={key} className="tag-item">
                        <span className="tag-key">{key}</span>
                        <span className="tag-value">{value}</span>
                      </span>
                    ))}
                  </div>
                </section>
              )}

              {agent.metrics_summary && (
                <section className="detail-section">
                  <h3>Metrics Summary</h3>
                  <div className="metrics-summary-grid">
                    <div className="metric-card">
                      <Icon name="activity" size="sm" />
                      <div className="metric-content">
                        <span className="metric-label">CPU (Avg / Max)</span>
                        <span className="metric-value">
                          {formatPercent(agent.metrics_summary.cpu_avg)} / {formatPercent(agent.metrics_summary.cpu_max)}
                        </span>
                      </div>
                    </div>
                    <div className="metric-card">
                      <Icon name="database" size="sm" />
                      <div className="metric-content">
                        <span className="metric-label">Memory (Avg / Max)</span>
                        <span className="metric-value">
                          {formatBytes(agent.metrics_summary.mem_avg)} / {formatBytes(agent.metrics_summary.mem_max)}
                        </span>
                      </div>
                    </div>
                    <div className="metric-card">
                      <Icon name="chart-bar" size="sm" />
                      <div className="metric-content">
                        <span className="metric-label">Total Samples</span>
                        <span className="metric-value">{agent.metrics_summary.total_samples.toLocaleString()}</span>
                      </div>
                    </div>
                  </div>
                </section>
              )}
            </div>
          )}
        </div>

        <div className="modal-footer">
          <button className="btn btn-secondary" onClick={onClose}>
            Close
          </button>
          {agent && (
            <button className="btn btn-secondary" onClick={loadAgentDetail} disabled={loading}>
              <Icon name="refresh" size="sm" /> Refresh
            </button>
          )}
        </div>
      </div>
    </div>
  );
}
