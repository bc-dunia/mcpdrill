import { useState, useCallback, useRef } from 'react';
import type { AuthConfig, AuthType } from '../types';
import { Icon } from './Icon';
import { parseTokenFile } from '../utils/tokenFileParser';

interface Props {
  authConfig?: AuthConfig;
  onChange: (config: AuthConfig | undefined) => void;
}

interface TokenEntry {
  value: string;
  id: string;
}

let tokenIdCounter = 0;
function generateTokenId(): string {
  return `token_${Date.now()}_${++tokenIdCounter}`;
}

function maskToken(token: string): string {
  if (token.length <= 6) return '\u2022\u2022\u2022\u2022\u2022\u2022';
  return '\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022' + token.slice(-6);
}

interface AuthConfigSectionProps extends Props {
  onTestConnection?: () => void;
  connectionStatus?: 'idle' | 'testing' | 'success' | 'failed';
  showTestButton?: boolean;
}

export function AuthConfigSection({ authConfig, onChange, onTestConnection, connectionStatus, showTestButton }: AuthConfigSectionProps) {
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [uploadError, setUploadError] = useState<string | null>(null);
  const [editingTokenId, setEditingTokenId] = useState<string | null>(null);
  const [editingValue, setEditingValue] = useState('');
  const [activeTokenIndex, setActiveTokenIndex] = useState<number>(
    authConfig?.activeTokenIndex ?? 0
  );

  const [tokenEntries, setTokenEntries] = useState<TokenEntry[]>(() => {
    if (!authConfig?.tokens) return [];
    return authConfig.tokens.map((token) => ({
      value: token,
      id: generateTokenId(),
    }));
  });
  
  const [quickTokenValue, setQuickTokenValue] = useState(() => {
    return authConfig?.tokens?.[0] || '';
  });

  const authType: AuthType = authConfig?.type || 'none';

  const syncTokensToConfig = useCallback(
    (entries: TokenEntry[], type: AuthType, tokenIndex?: number) => {
      if (type === 'none') {
        onChange(undefined);
      } else {
        const tokens = entries.map((e) => e.value).filter((v) => v.trim());
        const validIndex = tokenIndex ?? activeTokenIndex;
        const clampedIndex = tokens.length > 0 
          ? Math.min(Math.max(0, validIndex), tokens.length - 1) 
          : 0;
        onChange({
          type,
          tokens: tokens.length > 0 ? tokens : undefined,
          activeTokenIndex: tokens.length > 1 ? clampedIndex : undefined,
        });
      }
    },
    [onChange, activeTokenIndex]
  );

  const handleAuthTypeChange = useCallback(
    (newType: AuthType) => {
      if (newType === 'none') {
        onChange(undefined);
      } else {
        syncTokensToConfig(tokenEntries, newType);
      }
    },
    [onChange, syncTokensToConfig, tokenEntries]
  );

  const handleAddToken = useCallback(() => {
    const newEntry: TokenEntry = { value: '', id: generateTokenId() };
    const updated = [...tokenEntries, newEntry];
    setTokenEntries(updated);
    setEditingTokenId(newEntry.id);
    setEditingValue('');
  }, [tokenEntries]);

  const handleRemoveToken = useCallback(
    (id: string) => {
      const updated = tokenEntries.filter((entry) => entry.id !== id);
      setTokenEntries(updated);
      syncTokensToConfig(updated, authType);
      if (editingTokenId === id) {
        setEditingTokenId(null);
        setEditingValue('');
      }
    },
    [tokenEntries, syncTokensToConfig, authType, editingTokenId]
  );

  const handleEditToken = useCallback(
    (id: string) => {
      const entry = tokenEntries.find((e) => e.id === id);
      if (entry) {
        setEditingTokenId(id);
        setEditingValue(entry.value);
      }
    },
    [tokenEntries]
  );

  const handleSaveEdit = useCallback(() => {
    if (!editingTokenId) return;

    const trimmedValue = editingValue.trim();
    if (!trimmedValue) {
      // Remove empty tokens
      handleRemoveToken(editingTokenId);
      return;
    }

    const updated = tokenEntries.map((entry) =>
      entry.id === editingTokenId ? { ...entry, value: trimmedValue } : entry
    );
    setTokenEntries(updated);
    syncTokensToConfig(updated, authType);
    setEditingTokenId(null);
    setEditingValue('');
  }, [
    editingTokenId,
    editingValue,
    tokenEntries,
    syncTokensToConfig,
    authType,
    handleRemoveToken,
  ]);

  const handleCancelEdit = useCallback(() => {
    if (editingTokenId) {
      const entry = tokenEntries.find((e) => e.id === editingTokenId);
      if (entry && !entry.value.trim()) {
        // Remove if it was a new empty token
        handleRemoveToken(editingTokenId);
      }
    }
    setEditingTokenId(null);
    setEditingValue('');
  }, [editingTokenId, tokenEntries, handleRemoveToken]);

  const handleFileUpload = useCallback(
    async (event: React.ChangeEvent<HTMLInputElement>) => {
      const file = event.target.files?.[0];
      if (!file) return;

      setUploadError(null);

      try {
        const tokens = await parseTokenFile(file);
        const newEntries = tokens.map((token) => ({
          value: token,
          id: generateTokenId(),
        }));
        const updated = [...tokenEntries, ...newEntries];
        setTokenEntries(updated);
        syncTokensToConfig(updated, authType);
      } catch (err) {
        setUploadError(
          err instanceof Error ? err.message : 'Failed to parse token file'
        );
      }

      // Reset file input
      if (fileInputRef.current) {
        fileInputRef.current.value = '';
      }
    },
    [tokenEntries, syncTokensToConfig, authType]
  );

  const handleKeyDown = useCallback(
    (event: React.KeyboardEvent) => {
      if (event.key === 'Enter') {
        event.preventDefault();
        handleSaveEdit();
      } else if (event.key === 'Escape') {
        event.preventDefault();
        handleCancelEdit();
      }
    },
    [handleSaveEdit, handleCancelEdit]
  );

  const handleActiveTokenChange = useCallback(
    (event: React.ChangeEvent<HTMLSelectElement>) => {
      const newIndex = parseInt(event.target.value, 10);
      setActiveTokenIndex(newIndex);
      syncTokensToConfig(tokenEntries, authType, newIndex);
    },
    [syncTokensToConfig, tokenEntries, authType]
  );

  const handleQuickTokenChange = useCallback((value: string) => {
    setQuickTokenValue(value);
    const trimmedValue = value.trim();
    
    if (tokenEntries.length === 0 && trimmedValue) {
      const newEntry: TokenEntry = { value: trimmedValue, id: generateTokenId() };
      setTokenEntries([newEntry]);
      syncTokensToConfig([newEntry], authType);
    } else if (tokenEntries.length > 0) {
      const updated = tokenEntries.map((e, i) => i === 0 ? { ...e, value: trimmedValue } : e);
      setTokenEntries(updated);
      syncTokensToConfig(updated, authType);
    }
  }, [tokenEntries, syncTokensToConfig, authType]);

  const tokenCount = tokenEntries.filter((e) => e.value.trim()).length;
  const validTokens = tokenEntries.filter((e) => e.value.trim());
  const additionalTokens = tokenEntries.slice(1);

  return (
    <div className="form-section">
      <div className="section-header">
        <h3 id="auth-config-heading">
          <Icon name="shield" size="sm" aria-hidden={true} /> Authentication
        </h3>
      </div>

      <div
        className="form-field"
        role="radiogroup"
        aria-labelledby="auth-config-heading"
      >
        <label className="checkbox-label">
          <input
            type="radio"
            name="auth-type"
            value="none"
            checked={authType === 'none'}
            onChange={() => handleAuthTypeChange('none')}
            aria-describedby="auth-none-desc"
          />
          <span>None</span>
        </label>
        <span id="auth-none-desc" className="field-hint">
          No authentication required
        </span>

        <label className="checkbox-label" style={{ marginTop: '0.75rem' }}>
          <input
            type="radio"
            name="auth-type"
            value="bearer_token"
            checked={authType === 'bearer_token'}
            onChange={() => handleAuthTypeChange('bearer_token')}
            aria-describedby="auth-bearer-desc"
          />
          <span>Bearer Token</span>
        </label>
        <span id="auth-bearer-desc" className="field-hint">
          Use bearer token authentication
        </span>
      </div>

      {authType === 'bearer_token' && (
        <div className="form-field" style={{ marginTop: '1rem' }}>
          <label htmlFor="quick-token-input">Bearer Token</label>
          <div className="url-input-row">
            <input
              id="quick-token-input"
              type="password"
              value={quickTokenValue || (tokenEntries[0]?.value || '')}
              onChange={(e) => handleQuickTokenChange(e.target.value)}
              placeholder="Enter your bearer token"
              className="input"
              aria-describedby="quick-token-hint"
            />
            {showTestButton && onTestConnection && (
              <button
                type="button"
                onClick={onTestConnection}
                disabled={connectionStatus === 'testing'}
                className={`btn btn-secondary test-connection-btn ${connectionStatus || 'idle'}`}
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
            )}
          </div>
          <span id="quick-token-hint" className="field-hint">
            Token will be sent as: Authorization: Bearer &lt;token&gt;
          </span>

          {tokenCount > 1 && (
            <div style={{ marginTop: '1rem' }}>
              <div className="section-header">
                <label id="token-list-label">Additional Tokens</label>
                <div style={{ display: 'flex', gap: '0.5rem' }}>
                  <input
                    type="file"
                    accept=".txt,.csv,.json"
                    onChange={handleFileUpload}
                    style={{ display: 'none' }}
                    ref={fileInputRef}
                    aria-label="Upload tokens file"
                  />
                  <button
                    type="button"
                    onClick={() => fileInputRef.current?.click()}
                    className="btn btn-ghost btn-sm"
                    aria-label="Upload tokens from file"
                  >
                    <Icon name="upload" size="sm" aria-hidden={true} />
                    Upload
                  </button>
                  <button
                    type="button"
                    onClick={handleAddToken}
                    className="btn btn-ghost btn-sm"
                    aria-label="Add token manually"
                  >
                    <Icon name="plus" size="sm" aria-hidden={true} />
                    Add Token
                  </button>
                </div>
              </div>
            </div>
          )}

          {tokenCount <= 1 && (
            <div style={{ marginTop: '0.75rem' }}>
              <button
                type="button"
                onClick={handleAddToken}
                className="btn btn-ghost btn-sm"
                style={{ padding: '0.25rem 0' }}
              >
                <Icon name="plus" size="sm" aria-hidden={true} />
                Add additional tokens for load testing
              </button>
            </div>
          )}

          {uploadError && (
            <div className="agents-error" role="alert" style={{ marginTop: '0.5rem' }}>
              <Icon name="alert-triangle" size="sm" aria-hidden={true} />
              <span>{uploadError}</span>
            </div>
          )}

          {additionalTokens.length > 0 && (
            <p className="field-hint" style={{ marginTop: '0.5rem' }}>
              {tokenCount} token{tokenCount !== 1 ? 's' : ''} configured
            </p>
          )}

          {additionalTokens.length > 0 ? (
            <div
              className="headers-list"
              role="list"
              aria-labelledby="token-list-label"
              style={{ marginTop: '0.75rem' }}
            >
              {additionalTokens.map((entry, index) => (
                <div key={entry.id} className="header-row-wrapper" role="listitem">
                  <div className="header-row">
                    {editingTokenId === entry.id ? (
                      <>
                        <label
                          htmlFor={`token-edit-${entry.id}`}
                          className="sr-only"
                        >
                          Edit token {index + 2}
                        </label>
                        <input
                          id={`token-edit-${entry.id}`}
                          type="password"
                          value={editingValue}
                          onChange={(e) => setEditingValue(e.target.value)}
                          onKeyDown={handleKeyDown}
                          onBlur={handleSaveEdit}
                          placeholder="Enter token value"
                          className="input"
                          autoFocus
                          style={{ flex: 1 }}
                        />
                        <button
                          type="button"
                          onClick={handleSaveEdit}
                          className="btn btn-ghost btn-sm"
                          aria-label="Save token"
                        >
                          <Icon name="check" size="sm" aria-hidden={true} />
                        </button>
                        <button
                          type="button"
                          onClick={handleCancelEdit}
                          className="btn btn-ghost btn-sm"
                          aria-label="Cancel editing"
                        >
                          <Icon name="x" size="sm" aria-hidden={true} />
                        </button>
                      </>
                    ) : (
                      <>
                        <span
                          className="token-masked"
                          style={{
                            flex: 1,
                            fontFamily: 'monospace',
                            fontSize: '0.875rem',
                            color: 'var(--text-secondary)',
                            padding: '0.5rem 0.75rem',
                            background: 'var(--bg-tertiary)',
                            borderRadius: '4px',
                          }}
                          aria-label={`Token ${index + 2} (masked)`}
                        >
                          {entry.value.trim()
                            ? maskToken(entry.value)
                            : '(empty)'}
                        </span>
                        <button
                          type="button"
                          onClick={() => handleEditToken(entry.id)}
                          className="btn btn-ghost btn-sm"
                          aria-label={`Edit token ${index + 2}`}
                        >
                          <Icon name="edit" size="sm" aria-hidden={true} />
                        </button>
                        <button
                          type="button"
                          onClick={() => handleRemoveToken(entry.id)}
                          className="btn btn-ghost btn-sm btn-danger"
                          aria-label={`Delete token ${index + 2}`}
                        >
                          <Icon name="x" size="sm" aria-hidden={true} />
                        </button>
                      </>
                    )}
                  </div>
                </div>
              ))}
            </div>
          ) : null}

          {validTokens.length >= 2 && (
            <div className="form-field" style={{ marginTop: '1rem' }}>
              <label htmlFor="active-token-select">Active Token for Testing</label>
              <select
                id="active-token-select"
                className="select-input"
                value={activeTokenIndex}
                onChange={handleActiveTokenChange}
                aria-describedby="active-token-hint"
              >
                {validTokens.map((entry, index) => (
                  <option key={entry.id} value={index}>
                    Token {index + 1}: {maskToken(entry.value)}
                  </option>
                ))}
              </select>
              <span id="active-token-hint" className="field-hint">
                Select which token to use for test connections
              </span>
            </div>
          )}

          <p className="field-hint" style={{ marginTop: '0.75rem' }}>
            Supported file formats: .txt (line-separated), .csv (first column),
            .json (array or {`{"tokens": [...]}`})
          </p>
        </div>
      )}
    </div>
  );
}
