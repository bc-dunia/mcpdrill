import { useState, useEffect } from 'react';
import { formatRelativeTime } from '../utils/formatting';

interface ConnectionStatusProps {
  connected: boolean;
  lastUpdated: number | null;
}

export function ConnectionStatus({ connected, lastUpdated }: ConnectionStatusProps) {
  const [, setTick] = useState(0);

  useEffect(() => {
    if (lastUpdated === null) return;

    const interval = setInterval(() => {
      setTick(t => t + 1);
    }, 1000);

    return () => clearInterval(interval);
  }, [lastUpdated]);

  return (
    <div className="connection-status">
      <span
        className={`connection-dot ${connected ? 'connected' : 'disconnected'}`}
        aria-hidden="true"
      />
      <span className="connection-label" aria-live="polite">
        {connected ? 'Live' : 'Polling'}
      </span>
      {lastUpdated !== null && (
        <>
          <span className="connection-separator" aria-hidden="true">·</span>
          <span className="last-updated" aria-label={`Last updated ${formatRelativeTime(lastUpdated)}`}>
            {formatRelativeTime(lastUpdated)}
          </span>
        </>
      )}
    </div>
  );
}
