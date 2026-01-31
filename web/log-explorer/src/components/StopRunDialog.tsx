import { useState, useEffect, useRef, useCallback } from 'react';
import { Icon, type IconName } from './Icon';
import { stopRun, emergencyStopRun } from '../api';

type StopMode = 'drain' | 'immediate' | 'emergency';

interface StopRunDialogProps {
  isOpen: boolean;
  runId: string;
  onClose: () => void;
  onStopped: () => void;
}

interface StopOption {
  mode: StopMode;
  title: string;
  description: string;
  outcome: string;
  icon: IconName;
  variant: 'default' | 'warning' | 'danger';
}

const STOP_OPTIONS: StopOption[] = [
  {
    mode: 'drain',
    title: 'Drain Workers',
    description: 'Wait for in-flight operations to complete.',
    outcome: 'Clean shutdown, ~5-10 seconds',
    icon: 'clock',
    variant: 'default',
  },
  {
    mode: 'immediate',
    title: 'Stop Immediately',
    description: 'Cancel pending operations, wait for active ones.',
    outcome: 'Fast stop, some operations may be interrupted',
    icon: 'x-circle',
    variant: 'warning',
  },
  {
    mode: 'emergency',
    title: 'Emergency Stop',
    description: 'Forcefully terminate all operations now.',
    outcome: 'Immediate termination, data may be lost',
    icon: 'alert-triangle',
    variant: 'danger',
  },
];

export function StopRunDialog({ isOpen, runId, onClose, onStopped }: StopRunDialogProps) {
  const [selectedMode, setSelectedMode] = useState<StopMode>('drain');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const dialogRef = useRef<HTMLDivElement>(null);
  const firstOptionRef = useRef<HTMLInputElement>(null);
  const previousFocusRef = useRef<HTMLElement | null>(null);
  const previousOverflowRef = useRef<string>('');
  const selectedModeRef = useRef<StopMode>(selectedMode);
  const onCloseRef = useRef(onClose);

  selectedModeRef.current = selectedMode;
  onCloseRef.current = onClose;

  const handleStop = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      if (selectedMode === 'emergency') {
        await emergencyStopRun(runId);
      } else {
        await stopRun(runId, selectedMode);
      }
      onStopped();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to stop run');
    } finally {
      setLoading(false);
    }
  }, [runId, selectedMode, onStopped]);

  useEffect(() => {
    if (!isOpen) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.stopPropagation();
        e.preventDefault();
        onCloseRef.current();
        return;
      }

      if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
        e.preventDefault();
        const currentIndex = STOP_OPTIONS.findIndex((opt) => opt.mode === selectedModeRef.current);
        let newIndex: number;
        if (e.key === 'ArrowDown') {
          newIndex = (currentIndex + 1) % STOP_OPTIONS.length;
        } else {
          newIndex = (currentIndex - 1 + STOP_OPTIONS.length) % STOP_OPTIONS.length;
        }
        setSelectedMode(STOP_OPTIONS[newIndex].mode);
        return;
      }

      if (e.key === 'Tab' && dialogRef.current) {
        const focusableElements = dialogRef.current.querySelectorAll<HTMLElement>(
          'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'
        );
        const firstElement = focusableElements[0];
        const lastElement = focusableElements[focusableElements.length - 1];

        if (e.shiftKey && document.activeElement === firstElement) {
          e.preventDefault();
          lastElement?.focus();
        } else if (!e.shiftKey && document.activeElement === lastElement) {
          e.preventDefault();
          firstElement?.focus();
        }
      }
    };

    previousFocusRef.current = document.activeElement as HTMLElement;
    previousOverflowRef.current = document.body.style.overflow;
    document.addEventListener('keydown', handleKeyDown);
    requestAnimationFrame(() => {
      firstOptionRef.current?.focus();
    });
    document.body.style.overflow = 'hidden';
    setSelectedMode('drain');
    setError(null);

    return () => {
      document.removeEventListener('keydown', handleKeyDown);
      document.body.style.overflow = previousOverflowRef.current;
      previousFocusRef.current?.focus();
    };
  }, [isOpen]);

  const handleBackdropClick = useCallback(
    (e: React.MouseEvent) => {
      if (e.target === dialogRef.current) {
        onClose();
      }
    },
    [onClose]
  );

  if (!isOpen) return null;

  const selectedOption = STOP_OPTIONS.find((opt) => opt.mode === selectedMode);
  const buttonVariant = selectedOption?.variant === 'danger' 
    ? 'btn-danger-solid' 
    : selectedOption?.variant === 'warning' 
      ? 'btn-warning' 
      : 'btn-primary';

  return (
    <div
      ref={dialogRef}
      className="stop-dialog-backdrop"
      onClick={handleBackdropClick}
      role="dialog"
      aria-modal="true"
      aria-labelledby="stop-dialog-title"
    >
      <div className="stop-dialog">
        <div className="stop-dialog-header">
          <Icon name="alert-triangle" size="lg" className="stop-dialog-icon" aria-hidden={true} />
          <h2 id="stop-dialog-title" className="stop-dialog-title">
            Stop Test Run
          </h2>
        </div>

        <p className="stop-dialog-subtitle">Choose how to stop this test run:</p>

        {error && (
          <div className="stop-dialog-error" role="alert">
            <Icon name="alert-circle" size="sm" aria-hidden={true} />
            {error}
          </div>
        )}

        <div className="stop-dialog-options" role="radiogroup" aria-label="Stop mode options">
          {STOP_OPTIONS.map((option, index) => (
            <label
              key={option.mode}
              className={`stop-option ${selectedMode === option.mode ? 'selected' : ''} variant-${option.variant}`}
            >
              <input
                ref={index === 0 ? firstOptionRef : undefined}
                type="radio"
                name="stopMode"
                value={option.mode}
                checked={selectedMode === option.mode}
                onChange={() => setSelectedMode(option.mode)}
                disabled={loading}
                className="stop-option-radio"
              />
              <span className={`stop-option-icon variant-${option.variant}`} aria-hidden="true">
                <Icon name={option.icon} size="sm" />
              </span>
              <div className="stop-option-content">
                <h4>{option.title}</h4>
                <p>{option.description}</p>
                <span className="stop-option-outcome">{option.outcome}</span>
              </div>
            </label>
          ))}
        </div>

        <div className="stop-dialog-actions">
          <button
            type="button"
            className="btn btn-secondary"
            onClick={onClose}
            disabled={loading}
          >
            Cancel
          </button>
          <button
            type="button"
            className={`btn ${buttonVariant}`}
            onClick={handleStop}
            disabled={loading}
          >
            {loading ? (
              <>
                <Icon name="loader" size="sm" aria-hidden={true} />
                Stopping...
              </>
            ) : (
              <>
                <Icon name="x-circle" size="sm" aria-hidden={true} />
                Stop Run
              </>
            )}
          </button>
        </div>
      </div>
    </div>
  );
}
