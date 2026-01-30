import { useState, useCallback, useMemo, useEffect } from 'react';
import type { TargetConfig as TargetConfigType, ServerTelemetryConfig, AgentInfo } from '../types';
import { Icon } from './Icon';
import { AgentDetailModal } from './AgentDetailModal';
import { testConnection, ConnectionTestResult, fetchAgents } from '../api';

type ConnectionStatus = 'idle' | 'testing' | 'success' | 'failed';

interface Props {
  config: TargetConfigType;
  onChange: (config: TargetConfigType) => void;
  onConnectionStatusChange?: (status: ConnectionStatus, result?: ConnectionTestResult) => void;
  serverTelemetry?: ServerTelemetryConfig;
  onServerTelemetryChange?: (config: ServerTelemetryConfig | undefined) => void;
}

interface HeaderEntry {
  key: string;
  value: string;
  id: string;
}

const SENSITIVE_HEADERS = ['authorization', 'x-api-key', 'api-key', 'token', 'secret'];

function isSensitiveHeader(name: string): boolean {
  return SENSITIVE_HEADERS.some(h => name.toLowerCase().includes(h));
}

function isValidUrl(urlString: string): boolean {
  const trimmed = urlString?.trim();
  if (!trimmed) return false;
  try {
    const url = new URL(trimmed);
    return ['http:', 'https:'].includes(url.protocol);
  } catch {
    return false;
  }
}

function normalizeHeaderKey(key: string): string {
  return key.toLowerCase().trim();
}

let headerIdCounter = 0;
function generateHeaderId(): string {
  return `header_${Date.now()}_${++headerIdCounter}`;
}

export function TargetConfig({ config, onChange, onConnectionStatusChange, serverTelemetry, onServerTelemetryChange }: Props) {
  const [urlTouched, setUrlTouched] = useState(false);
  const urlError = urlTouched && !isValidUrl(config.url) ? 'Please enter a valid HTTP or HTTPS URL' : null;
  
  const [connectionStatus, setConnectionStatus] = useState<ConnectionStatus>('idle');
  const [connectionResult, setConnectionResult] = useState<ConnectionTestResult | null>(null);
  
  const [agents, setAgents] = useState<AgentInfo[]>([]);
  const [agentsLoading, setAgentsLoading] = useState(false);
  const [agentsError, setAgentsError] = useState<string | null>(null);
  const [selectedAgentId, setSelectedAgentId] = useState<string | null>(null);
  
  const [headerEntries, setHeaderEntries] = useState<HeaderEntry[]>(() => {
    if (!config.headers) return [];
    return Object.entries(config.headers).map(([key, value]) => ({
      key,
      value,
      id: generateHeaderId(),
    }));
  });

  useEffect(() => {
    setConnectionStatus('idle');
    setConnectionResult(null);
    onConnectionStatusChange?.('idle');
  }, [config.url, config.transport, config.headers, onConnectionStatusChange]);

  const handleTestConnection = useCallback(async () => {
    if (!isValidUrl(config.url)) {
      setUrlTouched(true);
      return;
    }
    
    setConnectionStatus('testing');
    setConnectionResult(null);
    onConnectionStatusChange?.('testing');
    
    try {
      const result = await testConnection(config.url, config.headers);
      setConnectionResult(result);
      
      if (result.success) {
        setConnectionStatus('success');
        onConnectionStatusChange?.('success', result);
      } else {
        setConnectionStatus('failed');
        onConnectionStatusChange?.('failed', result);
      }
    } catch (err) {
      const errorResult: ConnectionTestResult = {
        success: false,
        error: err instanceof Error ? err.message : 'Connection test failed',
        error_code: 'NETWORK_ERROR',
      };
      setConnectionResult(errorResult);
      setConnectionStatus('failed');
      onConnectionStatusChange?.('failed', errorResult);
    }
  }, [config.url, config.headers, onConnectionStatusChange]);

  const headerCollisions = useMemo(() => {
    const collisions = new Set<string>();
    const seen = new Map<string, string>();
    
    for (const entry of headerEntries) {
      if (!entry.key.trim()) continue;
      const normalized = normalizeHeaderKey(entry.key);
      const existingId = seen.get(normalized);
      if (existingId) {
        collisions.add(existingId);
        collisions.add(entry.id);
      } else {
        seen.set(normalized, entry.id);
      }
    }
    return collisions;
  }, [headerEntries]);

  const syncHeadersToConfig = useCallback((entries: HeaderEntry[]) => {
    const headers: Record<string, string> = {};
    const seen = new Set<string>();
    
    for (const entry of entries) {
      const trimmedKey = entry.key.trim();
      const normalized = normalizeHeaderKey(trimmedKey);
      if (trimmedKey && !seen.has(normalized)) {
        headers[trimmedKey] = entry.value;
        seen.add(normalized);
      }
    }
    onChange({ ...config, headers });
  }, [config, onChange]);

  const handleChange = (field: keyof TargetConfigType, value: string) => {
    onChange({ ...config, [field]: value });
  };

  const handleUrlBlur = useCallback(() => {
    setUrlTouched(true);
  }, []);

  const handleHeaderKeyChange = (id: string, newKey: string) => {
    const updated = headerEntries.map(entry =>
      entry.id === id ? { ...entry, key: newKey } : entry
    );
    setHeaderEntries(updated);
    syncHeadersToConfig(updated);
  };

  const handleHeaderValueChange = (id: string, newValue: string) => {
    const updated = headerEntries.map(entry =>
      entry.id === id ? { ...entry, value: newValue } : entry
    );
    setHeaderEntries(updated);
    syncHeadersToConfig(updated);
  };

  const handleRemoveHeader = (id: string) => {
    const updated = headerEntries.filter(entry => entry.id !== id);
    setHeaderEntries(updated);
    syncHeadersToConfig(updated);
  };

  const handleAddHeader = () => {
    const updated = [...headerEntries, { key: '', value: '', id: generateHeaderId() }];
    setHeaderEntries(updated);
  };

  const loadAgents = useCallback(async () => {
    setAgentsLoading(true);
    setAgentsError(null);
    try {
      const data = await fetchAgents();
      setAgents(data);
    } catch (err) {
      setAgentsError(err instanceof Error ? err.message : 'Failed to load agents');
      setAgents([]);
    } finally {
      setAgentsLoading(false);
    }
  }, []);

  useEffect(() => {
    if (serverTelemetry?.enabled) {
      loadAgents();
    }
  }, [serverTelemetry?.enabled, loadAgents]);

  const handleTelemetryEnabledChange = useCallback((enabled: boolean) => {
    if (enabled) {
      onServerTelemetryChange?.({ enabled: true, pair_key: serverTelemetry?.pair_key || '' });
    } else {
      onServerTelemetryChange?.(undefined);
    }
  }, [serverTelemetry?.pair_key, onServerTelemetryChange]);

  const handlePairKeyChange = useCallback((pair_key: string) => {
    onServerTelemetryChange?.({ enabled: true, pair_key });
  }, [onServerTelemetryChange]);

  const matchingAgents = useMemo(() => {
    if (!serverTelemetry?.pair_key) return [];
    return agents.filter(a => a.pair_key === serverTelemetry.pair_key);
  }, [agents, serverTelemetry?.pair_key]);

  return (
    <div className="wizard-step">
      <div className="step-header">
        <div className="step-icon" aria-hidden="true"><Icon name="target" size="lg" /></div>
        <div className="step-title">
          <h2>Configure Target</h2>
          <p>Define the MCP server or gateway you want to test</p>
        </div>
      </div>

      <div className="form-section">
        <div className="form-row">
          <div className="form-field">
            <label htmlFor="target-kind">Target Type</label>
            <select
              id="target-kind"
              value={config.kind}
              onChange={e => handleChange('kind', e.target.value)}
              className="select-input"
            >
              <option value="server">Server</option>
              <option value="gateway">Gateway</option>
            </select>
          </div>

          <div className="form-field">
            <label htmlFor="target-transport">Transport</label>
            <select
              id="target-transport"
              value={config.transport}
              onChange={e => handleChange('transport', e.target.value)}
              className="select-input"
            >
              <option value="streamable_http">Streamable HTTP</option>
              <option value="stdio">STDIO</option>
            </select>
          </div>
        </div>

        <div className="form-field form-field-full">
          <label htmlFor="target-url">Target URL</label>
          <div className="url-input-row">
            <input
              id="target-url"
              type="url"
              value={config.url}
              onChange={e => handleChange('url', e.target.value)}
              onBlur={handleUrlBlur}
              placeholder="http://localhost:3000/mcp"
              className={`input ${urlError ? 'input-error' : ''}`}
              aria-invalid={urlError ? 'true' : undefined}
              aria-describedby={urlError ? 'target-url-error' : 'target-url-hint'}
              required
            />
            <button
              type="button"
              onClick={handleTestConnection}
              disabled={connectionStatus === 'testing' || !isValidUrl(config.url)}
              className={`btn btn-secondary test-connection-btn ${connectionStatus}`}
              aria-busy={connectionStatus === 'testing'}
            >
              {connectionStatus === 'testing' ? (
                <>
                  <span className="spinner-sm" aria-hidden="true" />
                  Testing...
                </>
              ) : (
                <>
                  <Icon name="activity" size="sm" aria-hidden={true} />
                  Test Connection
                </>
              )}
            </button>
          </div>
          {urlError ? (
            <span id="target-url-error" className="field-error" role="alert">{urlError}</span>
          ) : (
            <span id="target-url-hint" className="field-hint">The endpoint URL of your MCP server</span>
          )}
        </div>

        {connectionResult && (
          <div className={`connection-result ${connectionStatus}`} role="status" aria-live="polite">
            {connectionStatus === 'success' ? (
              <>
                <div className="connection-result-header">
                  <Icon name="check-circle" size="md" aria-hidden={true} />
                  <span className="connection-result-title">Connection Successful</span>
                </div>
                <div className="connection-result-details">
                  <div className="connection-stat">
                    <span className="stat-label">Tools Found</span>
                    <span className="stat-value">{connectionResult.tool_count ?? 0}</span>
                  </div>
                  <div className="connection-stat">
                    <span className="stat-label">Connect</span>
                    <span className="stat-value">{connectionResult.connect_latency_ms ?? 0}ms</span>
                  </div>
                  <div className="connection-stat">
                    <span className="stat-label">Tools List</span>
                    <span className="stat-value">{connectionResult.tools_latency_ms ?? 0}ms</span>
                  </div>
                  <div className="connection-stat">
                    <span className="stat-label">Total</span>
                    <span className="stat-value">{connectionResult.total_latency_ms ?? 0}ms</span>
                  </div>
                </div>
                {connectionResult.tools && connectionResult.tools.length > 0 && (
                  <div className="connection-tools-preview">
                    <span className="tools-preview-label">Available Tools:</span>
                    <div className="tools-preview-list">
                      {connectionResult.tools.slice(0, 8).map((tool, i) => (
                        <span key={i} className="tool-chip" title={tool.description || tool.name}>
                          {tool.name}
                        </span>
                      ))}
                      {connectionResult.tools.length > 8 && (
                        <span className="tool-chip tool-chip-more">
                          +{connectionResult.tools.length - 8} more
                        </span>
                      )}
                    </div>
                  </div>
                )}
              </>
            ) : (
              <>
                <div className="connection-result-header">
                  <Icon name="alert-triangle" size="md" aria-hidden={true} />
                  <span className="connection-result-title">Connection Failed</span>
                </div>
                <div className="connection-result-error">
                  {connectionResult.error || 'Unknown error'}
                </div>
              </>
            )}
          </div>
        )}
      </div>

      <div className="form-section">
        <div className="section-header">
          <h3 id="custom-headers-heading">Custom Headers</h3>
          <button 
            type="button" 
            onClick={handleAddHeader} 
            className="btn btn-ghost btn-sm"
            aria-label="Add custom header"
          >
            + Add Header
          </button>
        </div>

        {headerEntries.length > 0 ? (
          <div className="headers-list" role="list" aria-label="Custom headers">
            {headerEntries.map((entry, index) => {
              const hasCollision = headerCollisions.has(entry.id);
              return (
                <div key={entry.id} className="header-row-wrapper" role="listitem">
                  <div className="header-row">
                    <div className="header-name-field">
                      <label htmlFor={`header-name-${entry.id}`} className="sr-only">
                        Header name {index + 1}
                      </label>
                      <input
                        id={`header-name-${entry.id}`}
                        type="text"
                        value={entry.key}
                        onChange={e => handleHeaderKeyChange(entry.id, e.target.value)}
                        placeholder="Header name"
                        className={`input ${hasCollision ? 'input-error' : ''}`}
                        aria-invalid={hasCollision ? 'true' : undefined}
                        aria-describedby={hasCollision ? `header-error-${entry.id}` : undefined}
                      />
                    </div>
                    <label htmlFor={`header-value-${entry.id}`} className="sr-only">
                      Header value {index + 1}
                    </label>
                    <input
                      id={`header-value-${entry.id}`}
                      type={isSensitiveHeader(entry.key) ? 'password' : 'text'}
                      value={entry.value}
                      onChange={e => handleHeaderValueChange(entry.id, e.target.value)}
                      placeholder="Header value"
                      className="input"
                    />
                    <button
                      type="button"
                      onClick={() => handleRemoveHeader(entry.id)}
                      className="btn btn-ghost btn-sm btn-danger"
                      aria-label={`Remove header ${entry.key || `row ${index + 1}`}`}
                    >
                      ×
                    </button>
                  </div>
                  {hasCollision && (
                    <span id={`header-error-${entry.id}`} className="field-error header-collision-error" role="alert">
                      Duplicate header name (case-insensitive)
                    </span>
                  )}
                </div>
              );
            })}
          </div>
        ) : (
          <p className="empty-hint">No custom headers configured</p>
        )}
      </div>

      {onServerTelemetryChange && (
        <div className="form-section">
          <div className="section-header">
            <h3>Server Telemetry</h3>
            <span className="section-badge optional">Optional</span>
          </div>
          <p className="section-description">
            Collect server-side metrics (CPU, memory, load) during the test. Requires mcpdrill-agent running on the target server.
          </p>

          <div className="form-field">
            <label className="checkbox-label">
              <input
                type="checkbox"
                checked={serverTelemetry?.enabled || false}
                onChange={e => handleTelemetryEnabledChange(e.target.checked)}
              />
              <span>Enable Server Telemetry</span>
            </label>
          </div>

          {serverTelemetry?.enabled && (
            <>
              <div className="form-field">
                <label htmlFor="pair-key">Pair Key</label>
                <input
                  id="pair-key"
                  type="text"
                  value={serverTelemetry.pair_key}
                  onChange={e => handlePairKeyChange(e.target.value)}
                  placeholder="e.g., my-load-test"
                  className={`input ${serverTelemetry.pair_key.trim() === '' ? 'input-error' : ''}`}
                  aria-invalid={serverTelemetry.pair_key.trim() === '' ? 'true' : undefined}
                  aria-describedby={serverTelemetry.pair_key.trim() === '' ? 'pair-key-error' : 'pair-key-hint'}
                />
                {serverTelemetry.pair_key.trim() === '' ? (
                  <span id="pair-key-error" className="field-error" role="alert">
                    Pair key is required when server telemetry is enabled
                  </span>
                ) : (
                  <span id="pair-key-hint" className="field-hint">
                    Must match the --pair-key flag used when starting mcpdrill-agent
                  </span>
                )}
              </div>

              <div className="form-field">
                <div className="agents-header">
                  <label>Available Agents</label>
                  <button
                    type="button"
                    onClick={loadAgents}
                    disabled={agentsLoading}
                    className="btn btn-ghost btn-sm"
                  >
                    {agentsLoading ? 'Loading...' : 'Refresh'}
                  </button>
                </div>

                {agentsError && (
                  <div className="agents-error">
                    <Icon name="alert-triangle" size="sm" />
                    <span>{agentsError}</span>
                  </div>
                )}

                {agents.length === 0 && !agentsLoading && !agentsError && (
                  <p className="empty-hint">
                    No agents registered. Start mcpdrill-agent with matching --pair-key to enable server telemetry.
                  </p>
                )}

                {agents.length > 0 && (
                  <div className="agents-list">
                    {agents.map(agent => {
                      const isMatching = agent.pair_key === serverTelemetry.pair_key;
                      return (
                        <button
                          type="button"
                          key={agent.agent_id}
                          className={`agent-item ${isMatching ? 'matching' : ''} ${agent.online ? 'online' : 'offline'}`}
                          onClick={() => setSelectedAgentId(agent.agent_id)}
                          title="Click to view agent details"
                        >
                          <div className="agent-status">
                            <span className={`status-dot ${agent.online ? 'online' : 'offline'}`} />
                            <span className="status-text">{agent.online ? 'Online' : 'Offline'}</span>
                          </div>
                          <div className="agent-info">
                            <span className="agent-pair-key">{agent.pair_key}</span>
                            <span className="agent-details">
                              {agent.hostname} • {agent.os}/{agent.arch}
                            </span>
                          </div>
                          {isMatching && (
                            <div className="agent-match-badge">
                              <Icon name="check" size="sm" />
                              Match
                            </div>
                          )}
                          <Icon name="chevron-right" size="sm" className="agent-detail-arrow" />
                        </button>
                      );
                    })}
                  </div>
                )}

                {serverTelemetry.pair_key && matchingAgents.length === 0 && agents.length > 0 && (
                  <div className="agents-warning">
                    <Icon name="alert-triangle" size="sm" />
                    <span>No agents found with pair key "{serverTelemetry.pair_key}"</span>
                  </div>
                )}

                {matchingAgents.length > 0 && matchingAgents.every(a => a.online) && (
                  <div className="agents-success">
                    <Icon name="check-circle" size="sm" />
                    <span>{matchingAgents.length} matching agent{matchingAgents.length > 1 ? 's' : ''} online</span>
                  </div>
                )}
              </div>
            </>
          )}
        </div>
      )}

      {selectedAgentId && (
        <AgentDetailModal
          agentId={selectedAgentId}
          onClose={() => setSelectedAgentId(null)}
        />
      )}
    </div>
  );
}
