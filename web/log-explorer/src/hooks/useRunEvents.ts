import { useState, useEffect, useCallback, useRef } from 'react';
import { subscribeToRunEvents, type RunEvent } from '../api/index';

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

  const handleEvent = useCallback((event: RunEvent) => {
    setLastEventId(event.event_id || null);
    setEventCount(prev => prev + 1);
    
    onEvent?.(event);
    
    switch (event.type) {
      case 'stage_started':
        onStageStarted?.(
          event.data.stage_id as string,
          event.data.stage as string
        );
        break;
      case 'stage_completed':
        onStageCompleted?.(
          event.data.stage_id as string,
          event.data.result as string
        );
        break;
      case 'run_completed':
        onRunCompleted?.(event.data.state as string);
        setConnected(false);
        break;
      case 'metrics':
        onMetrics?.(event.data);
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
      if (enabled && runId) {
        connect();
      }
    }, 3000);
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
