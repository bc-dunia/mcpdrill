import type { RunEventHandler, SSEErrorHandler } from './types';

const API_BASE = '';

function createSSEHandler(
  eventSource: EventSource,
  eventName: string,
  onEvent: RunEventHandler,
  autoClose = false
) {
  eventSource.addEventListener(eventName, (event) => {
    try {
      const data = JSON.parse((event as MessageEvent).data);
      onEvent({
        event_id: (event as MessageEvent).lastEventId || data.event_id || '',
        type: data.type || eventName,
        timestamp: data.timestamp || Date.now(),
        data,
      });
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
      const data = JSON.parse(event.data);
      onEvent({
        event_id: event.lastEventId || '',
        type: data.type || 'message',
        timestamp: data.ts_ms || Date.now(),
        data,
      });
    } catch (err) {
      console.error('Failed to parse SSE message:', err);
    }
  };
  
  // Backend sends all events as 'run_event' with type in data.type
  createSSEHandler(eventSource, 'run_event', (event) => {
    onEvent(event);
    // Auto-close on terminal states
    if (event.type === 'STATE_TRANSITION' && 
        (event.data.to_state === 'completed' || event.data.to_state === 'failed' || event.data.to_state === 'stopped')) {
      eventSource.close();
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
