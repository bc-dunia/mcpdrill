import { memo } from 'react';

export type PassFailStatus = 'pass' | 'fail' | 'running' | 'unknown';

interface PassFailBadgeProps {
  status: PassFailStatus;
  reason?: string;
}

function PassFailBadgeComponent({ status, reason }: PassFailBadgeProps) {
  const labels: Record<PassFailStatus, string> = {
    pass: 'PASS',
    fail: 'FAIL',
    running: 'RUNNING',
    unknown: 'UNKNOWN',
  };

  return (
    <span 
      className={`pass-fail-badge pass-fail-${status}`}
      title={status === 'fail' && reason ? reason : undefined}
      role="status"
      aria-label={`Run status: ${labels[status]}${status === 'fail' && reason ? `. Reason: ${reason}` : ''}`}
    >
      {labels[status]}
    </span>
  );
}

export const PassFailBadge = memo(PassFailBadgeComponent);

export function determinePassFailStatus(
  runState: string,
  stopReason?: { mode: string; reason?: string },
  errorRate?: number,
  errorThreshold: number = 0.1
): { status: PassFailStatus; reason?: string } {
  if (runState === 'running' || runState === 'scheduling') {
    return { status: 'running' };
  }

  if (runState === 'completed') {
    if (stopReason?.mode === 'condition_met') {
      return { 
        status: 'fail', 
        reason: stopReason.reason || 'Stop condition triggered' 
      };
    }
    
    if (errorRate !== undefined && errorRate > errorThreshold) {
      return { 
        status: 'fail', 
        reason: `Error rate ${(errorRate * 100).toFixed(1)}% exceeded threshold ${(errorThreshold * 100).toFixed(0)}%` 
      };
    }
    
    return { status: 'pass' };
  }

  if (runState === 'failed' || runState === 'stopped') {
    return { 
      status: 'fail', 
      reason: stopReason?.reason || `Run ${runState}` 
    };
  }

  if (runState === 'pending' || runState === 'created') {
    return { status: 'unknown' };
  }

  return { status: 'unknown' };
}
