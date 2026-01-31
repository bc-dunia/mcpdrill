import type { RunEvent, RunEventHandler, SSEErrorHandler } from './types';

const API_BASE = '';

function normalizeRunEvent(raw: Record<string, unknown>, fallbackEventId?: string): RunEvent {
  const eventType = (raw.type as string)?.toUpperCase() || 'UNKNOWN';
  const payload = (raw.payload as Record<string, unknown>) || {};
  const correlation = (raw.correlation as Record<string, unknown>) || {};
  
  return {
    schema_version: raw.schema_version as string,
    event_id: (raw.event_id as string) || fallbackEventId || '',
    ts_ms: raw.ts_ms as number,
    run_id: raw.run_id as string,
    execution_id: raw.execution_id as string,
    type: eventType,
    actor: raw.actor as string,
    correlation: {
      stage: correlation.stage as string | undefined,
      stage_id: correlation.stage_id as string | undefined,
      worker_id: correlation.worker_id as string | undefined,
      vu_id: correlation.vu_id as string | undefined,
      session_id: correlation.session_id as string | undefined,
    },
    payload: payload,
    evidence: raw.evidence as Array<{ kind: string; ref: string; note?: string }>,
    timestamp: (raw.ts_ms as number) || Date.now(),
    data: { ...payload, ...correlation },
  };
}

function createSSEHandler(
  eventSource: EventSource,
  eventName: string,
  onEvent: RunEventHandler,
  autoClose = false
) {
  eventSource.addEventListener(eventName, (event) => {
    try {
      const raw = JSON.parse((event as MessageEvent).data);
      const normalized = normalizeRunEvent(raw, (event as MessageEvent).lastEventId);
      onEvent(normalized);
      if (autoClose) eventSource.close();
    } catch (err) {
      console.error(`Failed to parse ${eventName}:`, err);
    }
  });
}

/**
 * Subscribe to run events via Server-Sent Events
 * @param runId - The run ID to subscribe to
 * @param onEvent - Callback for each event
 * @param onError - Callback for errors (optional)
 * @param lastEventId - Resume from this event ID (optional)
 * @returns Cleanup function to close the connection
 */
export function subscribeToRunEvents(
  runId: string,
  onEvent: RunEventHandler,
  onError?: SSEErrorHandler,
  lastEventId?: string
): () => void {
  const params = new URLSearchParams();
  if (lastEventId) {
    params.set('cursor', lastEventId);
  }
  
  const url = params.toString() 
    ? `${API_BASE}/runs/${runId}/events?${params}` 
    : `${API_BASE}/runs/${runId}/events`;
  
  const eventSource = new EventSource(url);
  
  eventSource.onmessage = (event) => {
    try {
      const raw = JSON.parse(event.data);
      const normalized = normalizeRunEvent(raw, event.lastEventId);
      onEvent(normalized);
    } catch (err) {
      console.error('Failed to parse SSE message:', err);
    }
  };
  
  createSSEHandler(eventSource, 'run_event', (event) => {
    onEvent(event);
    if (event.type === 'STATE_TRANSITION') {
      const toState = event.payload?.to_state || event.data.to_state;
      if (toState === 'completed' || toState === 'failed' || toState === 'stopped') {
        eventSource.close();
      }
    }
  });
  
  eventSource.onerror = (error) => {
    if (onError) {
      onError(error);
    }
  };
  
  // Return cleanup function
  return () => {
    eventSource.close();
  };
}
