import { useState, useEffect, useCallback, useRef } from 'react';
import { subscribeToRunEvents, type RunEvent } from '../api/index';
import { CONFIG } from '../config';

interface UseRunEventsOptions {
  enabled?: boolean;
  onEvent?: (event: RunEvent) => void;
  onStageStarted?: (stageId: string, stageName: string) => void;
  onStageCompleted?: (stageId: string, result: string) => void;
  onRunCompleted?: (state: string) => void;
  onMetrics?: (metrics: Record<string, unknown>) => void;
  fallbackToPolling?: boolean;
  pollingInterval?: number;
}

interface UseRunEventsResult {
  connected: boolean;
  error: Error | null;
  lastEventId: string | null;
  eventCount: number;
  reconnect: () => void;
}

export function useRunEvents(
  runId: string | null,
  options: UseRunEventsOptions = {}
): UseRunEventsResult {
  const {
    enabled = true,
    onEvent,
    onStageStarted,
    onStageCompleted,
    onRunCompleted,
    onMetrics,
  } = options;

  const [connected, setConnected] = useState(false);
  const [error, setError] = useState<Error | null>(null);
  const [lastEventId, setLastEventId] = useState<string | null>(null);
  const [eventCount, setEventCount] = useState(0);
  
  const cleanupRef = useRef<(() => void) | null>(null);
  const reconnectTimeoutRef = useRef<number | null>(null);
  const connectRef = useRef<(() => void) | null>(null);

  const handleEvent = useCallback((event: RunEvent) => {
    setLastEventId(event.event_id || null);
    setEventCount(prev => prev + 1);
    
    onEvent?.(event);
    
    // Normalize event type to uppercase (backend sends UPPERCASE, handle both cases)
    const eventType = event.type?.toUpperCase();
    
    switch (eventType) {
      case 'STAGE_STARTED':
        onStageStarted?.(
          (event.data.stage_id ?? event.correlation?.stage_id) as string,
          (event.data.stage ?? event.correlation?.stage) as string
        );
        break;
      case 'STAGE_COMPLETED':
        onStageCompleted?.(
          (event.data.stage_id ?? event.correlation?.stage_id) as string,
          event.data.result as string
        );
        break;
      case 'STATE_TRANSITION':
        // Handle run completion via state transition
        if (event.data.to_state === 'completed' || event.data.to_state === 'failed' || event.data.to_state === 'aborted') {
          onRunCompleted?.(event.data.to_state as string);
          setConnected(false);
        }
        break;
      case 'WORKER_HEARTBEAT':
        // Worker heartbeats contain metrics data
        if (event.data.metrics) {
          onMetrics?.(event.data.metrics as Record<string, unknown>);
        }
        break;
    }
  }, [onEvent, onStageStarted, onStageCompleted, onRunCompleted, onMetrics]);

  const handleError = useCallback((err: Event) => {
    console.error('SSE connection error:', err);
    setConnected(false);
    setError(new Error('Connection lost'));
    
    if (reconnectTimeoutRef.current) {
      clearTimeout(reconnectTimeoutRef.current);
    }
    reconnectTimeoutRef.current = window.setTimeout(() => {
      if (enabled && runId && connectRef.current) {
        connectRef.current();
      }
    }, CONFIG.RECONNECT_DELAY_MS);
  }, [enabled, runId]);

  const connect = useCallback(() => {
    if (!runId || !enabled) return;
    
    if (cleanupRef.current) {
      cleanupRef.current();
      cleanupRef.current = null;
    }
    
    setError(null);
    setConnected(true);
    
    cleanupRef.current = subscribeToRunEvents(
      runId,
      handleEvent,
      handleError,
      lastEventId || undefined
    );
  }, [runId, enabled, handleEvent, handleError, lastEventId]);

  connectRef.current = connect;

  const reconnect = useCallback(() => {
    if (cleanupRef.current) {
      cleanupRef.current();
      cleanupRef.current = null;
    }
    setEventCount(0);
    setLastEventId(null);
    connect();
  }, [connect]);

  useEffect(() => {
    if (enabled && runId) {
      connect();
    }
    
    return () => {
      if (cleanupRef.current) {
        cleanupRef.current();
        cleanupRef.current = null;
      }
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current);
        reconnectTimeoutRef.current = null;
      }
    };
  }, [runId, enabled, connect]);

  return {
    connected,
    error,
    lastEventId,
    eventCount,
    reconnect,
  };
}
