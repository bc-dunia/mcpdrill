import { memo, useState, useEffect } from 'react';
import { Icon } from './Icon';
import { fetchErrorSignatures, type ErrorSignature } from '../api/index';
import { CONFIG } from '../config';

interface ErrorSignaturesProps {
  runId: string;
}

function formatTime(ms: number): string {
  const date = new Date(ms);
  return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
}

function ErrorSignaturesComponent({ runId }: ErrorSignaturesProps) {
  const [signatures, setSignatures] = useState<ErrorSignature[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!runId) {
      setLoading(false);
      return;
    }

    const loadSignatures = async () => {
      setLoading(true);
      setError(null);
      try {
        const data = await fetchErrorSignatures(runId);
        setSignatures(data.signatures || []);
      } catch (err) {
        if (err instanceof Error && err.message.includes('404')) {
          setSignatures([]);
        } else {
          setError(err instanceof Error ? err.message : 'Unknown error');
          setSignatures([]);
        }
      } finally {
        setLoading(false);
      }
    };

    loadSignatures();
    const interval = setInterval(loadSignatures, CONFIG.REFRESH_INTERVALS.ERROR_SIGNATURES);
    return () => clearInterval(interval);
  }, [runId]);

  if (loading) {
    return (
      <div className="error-signatures" role="region" aria-label="Error signatures">
        <h3>Top Errors</h3>
        <div className="error-signatures-loading" role="status">
          <div className="spinner" aria-hidden="true" />
          <span>Loading error signatures...</span>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="error-signatures" role="region" aria-label="Error signatures">
        <h3>Top Errors</h3>
        <div className="error-signatures-error" role="alert">
          <Icon name="alert-triangle" size="lg" />
          <span>{error}</span>
        </div>
      </div>
    );
  }

  if (signatures.length === 0) {
    return (
      <div className="error-signatures" role="region" aria-label="Error signatures">
        <h3>Top Errors</h3>
        <div className="error-signatures-empty" role="status">
          <Icon name="check-circle" size="xl" className="success-icon" />
          <p>No errors detected</p>
          <p className="empty-hint">All operations completed successfully.</p>
        </div>
      </div>
    );
  }

  return (
    <div className="error-signatures" role="region" aria-label="Error signatures">
      <h3>Top Errors <span className="error-count-badge">{signatures.length}</span></h3>
      <div className="error-signatures-list">
        {signatures.map((sig, index) => (
          <div key={index} className="error-signature-item">
            <div className="error-signature-header">
              <span className="error-count">{sig.count}x</span>
              <span className="error-pattern" title={sig.sample_error}>{sig.pattern}</span>
            </div>
            <div className="error-signature-details">
              <div className="error-detail">
                <Icon name="clock" size="xs" />
                <span>First: {formatTime(sig.first_seen_ms)}</span>
                <span className="detail-separator">|</span>
                <span>Last: {formatTime(sig.last_seen_ms)}</span>
              </div>
              {sig.affected_operations.length > 0 && (
                <div className="error-detail">
                  <Icon name="zap" size="xs" />
                  <span>Operations: {sig.affected_operations.join(', ')}</span>
                </div>
              )}
              {sig.affected_tools.length > 0 && (
                <div className="error-detail">
                  <Icon name="wrench" size="xs" />
                  <span>Tools: {sig.affected_tools.join(', ')}</span>
                </div>
              )}
            </div>
            {sig.sample_error !== sig.pattern && (
              <div className="error-sample">
                <details>
                  <summary>Sample error</summary>
                  <pre>{sig.sample_error}</pre>
                </details>
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  )
}

export const ErrorSignatures = memo(ErrorSignaturesComponent);
