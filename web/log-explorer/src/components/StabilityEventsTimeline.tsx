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

/** Event types that indicate issues and should always be shown individually. */
const NOTABLE_TYPES = new Set(['dropped', 'reconnect']);

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

/** A display row: either a single notable event or a collapsed group. */
type TimelineRow =
  | { kind: 'single'; event: ConnectionEvent }
  | { kind: 'group'; eventType: string; count: number; firstTimestamp: string; lastTimestamp: string };

/**
 * Collapse consecutive runs of the same non-notable event type into groups
 * while keeping notable events (dropped, reconnect) as individual rows.
 */
function buildTimelineRows(sortedEvents: ConnectionEvent[]): TimelineRow[] {
  const rows: TimelineRow[] = [];
  let i = 0;

  while (i < sortedEvents.length) {
    const event = sortedEvents[i];

    // Notable events are always shown individually
    if (NOTABLE_TYPES.has(event.event_type)) {
      rows.push({ kind: 'single', event });
      i++;
      continue;
    }

    // Collect consecutive events of the same type
    let j = i + 1;
    while (
      j < sortedEvents.length &&
      sortedEvents[j].event_type === event.event_type &&
      !NOTABLE_TYPES.has(sortedEvents[j].event_type)
    ) {
      j++;
    }

    const count = j - i;
    if (count === 1) {
      rows.push({ kind: 'single', event });
    } else {
      // sortedEvents is newest-first, so first in array = latest timestamp
      rows.push({
        kind: 'group',
        eventType: event.event_type,
        count,
        firstTimestamp: sortedEvents[j - 1].timestamp, // oldest
        lastTimestamp: event.timestamp,                  // newest
      });
    }
    i = j;
  }

  return rows;
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

  const timelineRows = useMemo(() => buildTimelineRows(sortedEvents), [sortedEvents]);

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

  const hasNotableEvents = eventStats.dropped > 0 || eventStats.reconnect > 0;

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

      {/* When no issues exist, show a compact healthy summary */}
      {!hasNotableEvents && (
        <div className="timeline-healthy-summary">
          <Icon name="check-circle" size="sm" aria-hidden={true} />
          <span>
            {eventStats.created} sessions created
            {eventStats.terminated > 0 && <>, {eventStats.terminated} terminated</>}
            {eventStats.active > 0 && <>, {eventStats.active} active</>}
          </span>
          <span className="timeline-healthy-range">
            {sortedEvents.length > 0 && (
              <>
                {formatEventTime(sortedEvents[sortedEvents.length - 1].timestamp)}
                {' \u2013 '}
                {formatEventTime(sortedEvents[0].timestamp)}
              </>
            )}
          </span>
        </div>
      )}

      {/* When there are issues, show collapsed timeline */}
      {hasNotableEvents && (
        <div className="timeline-list">
          {timelineRows.slice(0, 80).map((row, index) => {
            if (row.kind === 'group') {
              const config = EVENT_CONFIG[row.eventType] || {
                icon: 'info' as IconName,
                color: 'var(--text-muted)',
                label: row.eventType,
              };
              return (
                <div key={index} className={`timeline-item timeline-item-group event-${row.eventType}`}>
                  <div className="timeline-marker" style={{ backgroundColor: config.color, opacity: 0.7 }}>
                    <Icon name={config.icon} size="xs" aria-hidden={true} />
                  </div>
                  <div className="timeline-content">
                    <div className="timeline-main">
                      <span className="timeline-label timeline-label-grouped">
                        {config.label} <span className="timeline-group-count">&times;{row.count}</span>
                      </span>
                      <span className="timeline-time">
                        {formatEventTime(row.firstTimestamp)} &ndash; {formatEventTime(row.lastTimestamp)}
                      </span>
                    </div>
                  </div>
                </div>
              );
            }

            const event = row.event;
            const config = EVENT_CONFIG[event.event_type] || {
              icon: 'info' as IconName,
              color: 'var(--text-muted)',
              label: event.event_type,
            };
            const isNotable = NOTABLE_TYPES.has(event.event_type);

            return (
              <div key={index} className={`timeline-item event-${event.event_type}${isNotable ? ' timeline-item-notable' : ''}`}>
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
                      {event.session_id}
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
          {timelineRows.length > 80 && (
            <div className="timeline-more">
              +{timelineRows.length - 80} more groups
            </div>
          )}
        </div>
      )}
    </div>
  );
}

export const StabilityEventsTimeline = memo(StabilityEventsTimelineComponent);
