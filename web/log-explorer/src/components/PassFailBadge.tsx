import { memo } from 'react';

export type PassFailStatus = 'pass' | 'fail' | 'running' | 'aborted' | 'unknown';

interface PassFailBadgeProps {
  status: PassFailStatus;
  reason?: string;
}

function PassFailBadgeComponent({ status, reason }: PassFailBadgeProps) {
  const labels: Record<PassFailStatus, string> = {
    pass: 'PASS',
    fail: 'FAIL',
    running: 'RUNNING',
    aborted: 'ABORTED',
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
  stopReason?: { mode: string; reason?: string; actor?: string },
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

  // Handle 'aborted' state (emergency stop)
  if (runState === 'aborted') {
    return { 
      status: 'aborted', 
      reason: stopReason?.reason || 'Emergency stop' 
    };
  }

  // Handle 'stopped' state - distinguish between normal completion and failures
  if (runState === 'stopped') {
    // Automatic stops by system actors (autoramp, scheduler) are normal completions
    const automaticActors = ['autoramp', 'scheduler', 'system'];
    const isAutomaticCompletion = stopReason?.actor && 
      automaticActors.includes(stopReason.actor) &&
      stopReason.reason === 'stop_requested';
    
    if (isAutomaticCompletion) {
      // Check error rate even for automatic completions
      if (errorRate !== undefined && errorRate > errorThreshold) {
        return { 
          status: 'fail', 
          reason: `Error rate ${(errorRate * 100).toFixed(1)}% exceeded threshold ${(errorThreshold * 100).toFixed(0)}%` 
        };
      }
      return { status: 'pass' };
    }
    
    // User-initiated stops (actor: 'user' or 'ui') - not a failure, just stopped
    const userActors = ['user', 'ui'];
    if (stopReason?.actor && userActors.includes(stopReason.actor)) {
      return { 
        status: 'pass', 
        reason: 'Stopped by user' 
      };
    }
    
    // Other stops are considered failures
    return { 
      status: 'fail', 
      reason: stopReason?.reason || 'Run stopped' 
    };
  }

  if (runState === 'failed') {
    return { 
      status: 'fail', 
      reason: stopReason?.reason || 'Run failed' 
    };
  }

  if (runState === 'stopping') {
    return { status: 'running' };
  }

  if (runState === 'pending' || runState === 'created') {
    return { status: 'unknown' };
  }

  return { status: 'unknown' };
}
