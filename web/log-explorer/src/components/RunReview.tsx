import { useState, useEffect, useCallback } from 'react';
import type { RunConfig } from '../types';
import { validateRunConfig, type ValidationError, type ValidationWarning } from '../api';
import { ConfirmDialog } from './ConfirmDialog';
import { Icon } from './Icon';

interface Props {
  config: RunConfig;
  onStart: () => void;
  isStarting: boolean;
  error: string | null;
}

interface ValidationState {
  isValidating: boolean;
  isValid: boolean | null;
  errors: ValidationError[];
  warnings: ValidationWarning[];
  lastValidated: number | null;
}

const SENSITIVE_HEADERS = ['authorization', 'x-api-key', 'api-key', 'token', 'secret'];

function isSensitiveHeader(name: string): boolean {
  return SENSITIVE_HEADERS.some(h => name.toLowerCase().includes(h));
}

function maskSensitiveData(config: RunConfig): RunConfig {
  if (!config.target.headers || Object.keys(config.target.headers).length === 0) {
    return config;
  }
  
  return {
    ...config,
    target: {
      ...config.target,
      headers: Object.fromEntries(
        Object.entries(config.target.headers).map(([k, v]) => [
          k,
          isSensitiveHeader(k) ? '••••••••' : v
        ])
      ),
    },
  };
}

function formatDuration(ms: number): string {
  const seconds = Math.floor(ms / 1000);
  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = seconds % 60;
  if (minutes > 0) {
    return remainingSeconds > 0 ? `${minutes}m ${remainingSeconds}s` : `${minutes}m`;
  }
  return `${seconds}s`;
}

export function RunReview({ config, onStart, isStarting, error }: Props) {
  const [showConfirm, setShowConfirm] = useState(false);
  const [validation, setValidation] = useState<ValidationState>({
    isValidating: false,
    isValid: null,
    errors: [],
    warnings: [],
    lastValidated: null,
  });
  
  const enabledStages = config.stages.filter(s => s.enabled);
  const totalDuration = enabledStages.reduce((sum, s) => sum + s.duration_ms, 0);
  const maxVUs = Math.max(...enabledStages.map(s => s.load.target_vus));
  const totalWeight = config.workload.op_mix.reduce((sum, op) => sum + op.weight, 0);

  const runValidation = useCallback(async () => {
    setValidation(prev => ({ ...prev, isValidating: true }));
    try {
      const result = await validateRunConfig(config);
      setValidation({
        isValidating: false,
        isValid: result.ok,
        errors: result.errors || [],
        warnings: result.warnings || [],
        lastValidated: Date.now(),
      });
    } catch (err) {
      setValidation(prev => ({
        ...prev,
        isValidating: false,
        isValid: null,
        errors: [{
          field: 'general',
          code: 'VALIDATION_FAILED',
          message: err instanceof Error ? err.message : 'Validation request failed',
        }],
        lastValidated: Date.now(),
      }));
    }
  }, [config]);

  useEffect(() => {
    runValidation();
  }, [runValidation]);

  return (
    <div className="wizard-step">
      <div className="step-header">
        <div className="step-icon" aria-hidden="true"><Icon name="rocket" size="lg" /></div>
        <div className="step-title">
          <h2>Review & Start</h2>
          <p>Verify your configuration and launch the load test</p>
        </div>
      </div>

      <div className="review-grid">
        <div className="review-card">
          <h3>Target</h3>
          <div className="review-content">
            <div className="review-row">
              <span className="review-label">Type</span>
              <span className="review-value">{config.target.kind}</span>
            </div>
            <div className="review-row">
              <span className="review-label">URL</span>
              <span className="review-value review-url">{config.target.url || '—'}</span>
            </div>
            <div className="review-row">
              <span className="review-label">Transport</span>
              <span className="review-value">{config.target.transport}</span>
            </div>
            {config.target.headers && Object.keys(config.target.headers).length > 0 && (
              <div className="review-row">
                <span className="review-label">Headers</span>
                <span className="review-value">{Object.keys(config.target.headers).length} custom</span>
              </div>
            )}
          </div>
        </div>

        <div className="review-card">
          <h3>Test Plan</h3>
          <div className="review-content">
            <div className="review-row">
              <span className="review-label">Scenario</span>
              <span className="review-value">{config.scenario_id}</span>
            </div>
            <div className="review-row">
              <span className="review-label">Stages</span>
              <span className="review-value">{enabledStages.length} enabled</span>
            </div>
            <div className="review-row">
              <span className="review-label">Duration</span>
              <span className="review-value">{formatDuration(totalDuration)}</span>
            </div>
            <div className="review-row">
              <span className="review-label">Max VUs</span>
              <span className="review-value">{maxVUs}</span>
            </div>
          </div>
        </div>

        <div className="review-card">
          <h3>Stages</h3>
          <div className="review-stages">
            {enabledStages.map(stage => (
              <div key={stage.stage_id} className="review-stage">
                <span className="stage-badge">{stage.stage}</span>
                <span className="stage-detail">{stage.load.target_vus} VUs</span>
                <span className="stage-detail">{formatDuration(stage.duration_ms)}</span>
              </div>
            ))}
            {enabledStages.length === 0 && (
              <p className="review-empty">No stages enabled</p>
            )}
          </div>
        </div>

        <div className="review-card">
          <h3>Workload</h3>
          <div className="review-operations">
            {config.workload.op_mix.map((op, index) => (
              <div key={index} className="review-operation">
                <span className="operation-name">{op.operation}</span>
                <span className="operation-weight">
                  {totalWeight > 0 ? Math.round((op.weight / totalWeight) * 100) : 0}%
                </span>
              </div>
            ))}
          </div>
        </div>
      </div>

      <div className="review-validation" role="region" aria-label="Configuration validation">
        <div className="validation-header">
          <h3>
            <Icon name="check-circle" size="md" aria-hidden={true} /> Validation
          </h3>
          <button
            type="button"
            className="btn btn-ghost btn-sm"
            onClick={runValidation}
            disabled={validation.isValidating}
          >
            <Icon name={validation.isValidating ? 'loader' : 'refresh'} size="sm" aria-hidden={true} />
            {validation.isValidating ? 'Validating...' : 'Re-validate'}
          </button>
        </div>
        
        {validation.isValidating && (
          <div className="validation-status validating">
            <Icon name="loader" size="sm" aria-hidden={true} />
            Validating configuration...
          </div>
        )}
        
        {!validation.isValidating && validation.isValid === true && validation.errors.length === 0 && (
          <div className="validation-status valid">
            <Icon name="check" size="sm" aria-hidden={true} />
            Configuration is valid
            {validation.warnings.length > 0 && ` (${validation.warnings.length} warning${validation.warnings.length !== 1 ? 's' : ''})`}
          </div>
        )}
        
        {!validation.isValidating && validation.errors.length > 0 && (
          <div className="validation-errors">
            <div className="validation-status invalid">
              <Icon name="x-circle" size="sm" aria-hidden={true} />
              {validation.errors.length} error{validation.errors.length !== 1 ? 's' : ''} found
            </div>
            <ul className="validation-list">
              {validation.errors.map((err, idx) => (
                <li key={idx} className="validation-item error">
                  <Icon name="x" size="xs" aria-hidden={true} />
                  <span className="validation-field">{err.field}:</span>
                  <span className="validation-message">{err.message}</span>
                  {err.code && <code className="validation-code">{err.code}</code>}
                </li>
              ))}
            </ul>
          </div>
        )}
        
        {!validation.isValidating && validation.warnings.length > 0 && (
          <div className="validation-warnings">
            <div className="validation-status warning">
              <Icon name="alert-triangle" size="sm" aria-hidden={true} />
              {validation.warnings.length} warning{validation.warnings.length !== 1 ? 's' : ''}
            </div>
            <ul className="validation-list">
              {validation.warnings.map((warn, idx) => (
                <li key={idx} className="validation-item warning">
                  <Icon name="alert-triangle" size="xs" aria-hidden={true} />
                  <span className="validation-field">{warn.field}:</span>
                  <span className="validation-message">{warn.message}</span>
                  {warn.code && <code className="validation-code">{warn.code}</code>}
                </li>
              ))}
            </ul>
          </div>
        )}
      </div>

      <div className="review-json">
        <h3>Configuration JSON</h3>
        <pre className="json-preview">{JSON.stringify(maskSensitiveData(config), null, 2)}</pre>
      </div>

      {error && (
        <div className="review-error" role="alert">
          <span className="error-icon" aria-hidden="true"><Icon name="alert-triangle" size="sm" /></span>
          <span>{error}</span>
        </div>
      )}

      <div className="review-actions">
        <button
          type="button"
          onClick={() => setShowConfirm(true)}
          disabled={isStarting || !config.target.url || enabledStages.length === 0 || (validation.errors.length > 0 && !validation.isValidating)}
          className="btn btn-primary btn-lg"
        >
          {isStarting ? (
            <>
              <Icon name="loader" size="sm" aria-hidden={true} />
              Starting...
            </>
          ) : (
            <>
              <Icon name="rocket" size="sm" aria-hidden={true} />
              Start Load Test
            </>
          )}
        </button>
        {(!config.target.url || enabledStages.length === 0) && (
          <p className="review-hint">
            {!config.target.url && 'Please configure a target URL. '}
            {enabledStages.length === 0 && 'Please enable at least one stage.'}
          </p>
        )}
      </div>

      <ConfirmDialog
        isOpen={showConfirm}
        title="Start Load Test?"
        message={`This will start a load test against ${config.target.url} with up to ${maxVUs} virtual users for ${formatDuration(totalDuration)}. Continue?`}
        confirmLabel="Start Test"
        cancelLabel="Cancel"
        variant="warning"
        onConfirm={() => {
          setShowConfirm(false);
          onStart();
        }}
        onCancel={() => setShowConfirm(false)}
      />
    </div>
  );
}
