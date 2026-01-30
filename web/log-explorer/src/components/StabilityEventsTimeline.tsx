import { memo, useMemo } from 'react';
import type { ConnectionEvent } from '../types';
import { Icon, type IconName } from './Icon';

interface StabilityEventsTimelineProps {
  events: ConnectionEvent[];
  loading?: boolean;
}

const EVENT_CONFIG: Record<string, { icon: IconName; color: string; label: string }> = {
  created: { icon: 'plus', color: 'var(--accent-green)', label: 'Session Created' },
  active: { icon: 'check', color: 'var(--accent-cyan)', label: 'Session Active' },
  dropped: { icon: 'x-circle', color: 'var(--accent-red)', label: 'Session Dropped' },
  terminated: { icon: 'x', color: 'var(--accent-amber)', label: 'Session Terminated' },
  reconnect: { icon: 'refresh', color: 'var(--accent-purple)', label: 'Reconnected' },
};

function formatEventTime(timestamp: string): string {
  const date = new Date(timestamp);
  return date.toLocaleTimeString('en-US', {
    hour12: false,
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  });
}

function formatDuration(ms: number | undefined): string {
  if (!ms) return '';
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
  return `${Math.floor(ms / 60000)}m ${Math.floor((ms % 60000) / 1000)}s`;
}

function StabilityEventsTimelineComponent({ events, loading }: StabilityEventsTimelineProps) {
  const sortedEvents = useMemo(() => {
    return [...events].sort((a, b) => 
      new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime()
    );
  }, [events]);

  const eventStats = useMemo(() => {
    const stats = { created: 0, dropped: 0, reconnect: 0, terminated: 0, active: 0 };
    events.forEach(e => {
      if (e.event_type in stats) {
        stats[e.event_type as keyof typeof stats]++;
      }
    });
    return stats;
  }, [events]);

  if (loading) {
    return (
      <div className="stability-events-timeline">
        <h4>Connection Events</h4>
        <div className="timeline-loading">
          <div className="spinner" aria-hidden="true" />
          <span>Loading events...</span>
        </div>
      </div>
    );
  }

  if (events.length === 0) {
    return (
      <div className="stability-events-timeline">
        <h4>Connection Events</h4>
        <div className="timeline-empty">
          <Icon name="check-circle" size="lg" aria-hidden={true} />
          <span>No connection events recorded</span>
        </div>
      </div>
    );
  }

  return (
    <div className="stability-events-timeline">
      <div className="timeline-header">
        <h4>Connection Events</h4>
        <div className="timeline-stats">
          {eventStats.dropped > 0 && (
            <span className="stat-badge stat-dropped">
              <Icon name="x-circle" size="xs" aria-hidden={true} />
              {eventStats.dropped} dropped
            </span>
          )}
          {eventStats.reconnect > 0 && (
            <span className="stat-badge stat-reconnect">
              <Icon name="refresh" size="xs" aria-hidden={true} />
              {eventStats.reconnect} reconnects
            </span>
          )}
          <span className="stat-badge stat-total">
            {events.length} events
          </span>
        </div>
      </div>
      
      <div className="timeline-list">
        {sortedEvents.slice(0, 50).map((event, index) => {
          const config = EVENT_CONFIG[event.event_type] || {
            icon: 'info' as IconName,
            color: 'var(--text-muted)',
            label: event.event_type,
          };
          
          return (
            <div key={index} className={`timeline-item event-${event.event_type}`}>
              <div className="timeline-marker" style={{ backgroundColor: config.color }}>
                <Icon name={config.icon} size="xs" aria-hidden={true} />
              </div>
              <div className="timeline-content">
                <div className="timeline-main">
                  <span className="timeline-label">{config.label}</span>
                  <span className="timeline-time">{formatEventTime(event.timestamp)}</span>
                </div>
                <div className="timeline-details">
                  <span className="timeline-session" title={event.session_id}>
                    {event.session_id.slice(0, 16)}...
                  </span>
                  {event.duration_ms !== undefined && event.duration_ms > 0 && (
                    <span className="timeline-duration">
                      {formatDuration(event.duration_ms)}
                    </span>
                  )}
                  {event.reason && (
                    <span className="timeline-reason" title={event.reason}>
                      {event.reason}
                    </span>
                  )}
                </div>
              </div>
            </div>
          );
        })}
        {sortedEvents.length > 50 && (
          <div className="timeline-more">
            +{sortedEvents.length - 50} more events
          </div>
        )}
      </div>
    </div>
  );
}

export const StabilityEventsTimeline = memo(StabilityEventsTimelineComponent);
