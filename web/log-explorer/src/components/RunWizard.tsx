import { useState, useEffect, useCallback, useMemo } from 'react';
import type { RunConfig, WizardStep, TargetConfig as TargetConfigType, StageConfig as StageConfigType, WorkloadConfig as WorkloadConfigType, FetchedTool, ServerTelemetryConfig, AuthConfig } from '../types';
import { createRun, startRun, ConnectionTestResult } from '../api/index';
import { TargetConfig } from './TargetConfig';
import { StageConfig } from './StageConfig';
import { WorkloadConfig } from './WorkloadConfig';
import { RunReview } from './RunReview';
import { Icon } from './Icon';
import { useToast } from './Toast';
import { ConfirmDialog } from './ConfirmDialog';
import { formatRelativeTime } from '../utils/formatting';

type ConnectionStatus = 'idle' | 'testing' | 'success' | 'failed';

const WIZARD_STORAGE_KEY = 'mcpdrill-wizard-progress';

interface Props {
  onRunStarted: (runId: string) => void;
}

import type { IconName } from './Icon';

const STEPS: { id: WizardStep; label: string; icon: IconName }[] = [
  { id: 'target', label: 'Target', icon: 'target' },
  { id: 'stages', label: 'Stages', icon: 'zap' },
  { id: 'workload', label: 'Workload', icon: 'dice' },
  { id: 'review', label: 'Review', icon: 'rocket' },
];

// Generate hex stage IDs per spec: stg_<hex>
function generateStageID(): string {
  const timestamp = Date.now().toString(16);
  const random = Math.floor(Math.random() * 0xffff).toString(16).padStart(4, '0');
  return `stg_${timestamp}${random}`;
}

function createDefaultConfig(): RunConfig {
  return {
    scenario_id: `load-test-${Date.now()}`,
    target: {
      kind: 'server',
      url: 'http://127.0.0.1:3000/mcp',
      transport: 'streamable_http',
    },
    stages: [
      {
        stage_id: generateStageID(),
        stage: 'preflight',
        enabled: true,
        duration_ms: 10000,
        load: { target_vus: 1 },
      },
      {
        stage_id: generateStageID(),
        stage: 'baseline',
        enabled: true,
        duration_ms: 30000,
        load: { target_vus: 5 },
      },
      {
        stage_id: generateStageID(),
        stage: 'ramp',
        enabled: true,
        duration_ms: 60000,
        load: { target_vus: 20 },
      },
    ],
    workload: {
      op_mix: [
        { operation: 'tools/list', weight: 1 },
        { operation: 'tools/call', weight: 3, tool_name: 'fast_echo', arguments: { message: 'hello' } },
        { operation: 'tools/call', weight: 2, tool_name: 'calculate', arguments: { expression: '2 + 2' } },
        { operation: 'tools/call', weight: 1, tool_name: 'weather_api', arguments: { location: 'San Francisco' } },
      ],
    },
    session_policy: {
      mode: 'reuse',
    },
    server_telemetry: {
      enabled: false,
      pair_key: 'dev-test',
    },
    schema_version: 'run-config/v1',
  };
}

function normalizeTargetKind(kind: unknown): 'server' {
  if (kind === 'server') {
    return 'server';
  }
  return 'server';
}

function normalizeTargetTransport(transport: unknown): 'streamable_http' {
  if (transport === 'streamable_http') {
    return 'streamable_http';
  }
  return 'streamable_http';
}

function isValidUrl(urlString: string): boolean {
  const trimmed = urlString?.trim();
  if (!trimmed) return false;
  try {
    const url = new URL(trimmed);
    return ['http:', 'https:'].includes(url.protocol);
  } catch {
    return false;
  }
}

function loadWizardProgress(): { step: WizardStep; config: RunConfig; savedAt?: number } | null {
  try {
    const saved = localStorage.getItem(WIZARD_STORAGE_KEY);
    if (!saved) return null;
    const parsed = JSON.parse(saved);
    const hasValidStructure = parsed.config && parsed.step && parsed.config.target && parsed.config.stages;
    return hasValidStructure ? { step: parsed.step, config: parsed.config, savedAt: parsed.savedAt } : null;
  } catch {
    return null;
  }
}

function saveWizardProgress(step: WizardStep, config: RunConfig): void {
  try {
    localStorage.setItem(WIZARD_STORAGE_KEY, JSON.stringify({ step, config, savedAt: Date.now() }));
  } catch (err) {
    console.warn('Failed to save wizard progress to localStorage:', err);
  }
}

function clearWizardProgress(): void {
  try {
    localStorage.removeItem(WIZARD_STORAGE_KEY);
  } catch (err) {
    console.warn('Failed to clear wizard progress from localStorage:', err);
  }
}

export function RunWizard({ onRunStarted }: Props) {
  const [currentStep, setCurrentStep] = useState<WizardStep>(() => {
    const saved = loadWizardProgress();
    return saved?.step ?? 'target';
  });
  const [config, setConfig] = useState<RunConfig>(() => {
    const saved = loadWizardProgress();
    if (saved?.config) {
      const defaults = createDefaultConfig();
      return {
        ...defaults,
        ...saved.config,
        target: {
          ...defaults.target,
          ...(saved.config.target ?? {}),
          kind: normalizeTargetKind(saved.config.target?.kind),
          transport: normalizeTargetTransport(saved.config.target?.transport),
        },
        workload: { ...defaults.workload, ...(saved.config.workload ?? {}) },
        session_policy: saved.config.session_policy 
          ? { ...defaults.session_policy, ...saved.config.session_policy }
          : defaults.session_policy,
        server_telemetry: saved.config.server_telemetry
          ? { ...defaults.server_telemetry, ...saved.config.server_telemetry }
          : defaults.server_telemetry,
      };
    }
    return createDefaultConfig();
  });
  const [isStarting, setIsStarting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [hasRestoredProgress, setHasRestoredProgress] = useState(() => loadWizardProgress() !== null);
  const [connectionStatus, setConnectionStatus] = useState<ConnectionStatus>('idle');
  const [discoveredTools, setDiscoveredTools] = useState<FetchedTool[]>([]);
  const [authConfig, setAuthConfig] = useState<AuthConfig | undefined>(undefined);
  const { showToast } = useToast();
  
  const [lastSaved, setLastSaved] = useState<number | null>(() => {
    const saved = loadWizardProgress();
    return saved?.savedAt ?? null;
  });
  const [showResetConfirm, setShowResetConfirm] = useState(false);

  const handleConnectionStatusChange = useCallback((status: ConnectionStatus, result?: ConnectionTestResult) => {
    setConnectionStatus(status);
    if (status === 'success' && result?.tools) {
      const tools: FetchedTool[] = result.tools.map(t => ({
        name: t.name,
        description: t.description,
        inputSchema: t.inputSchema as FetchedTool['inputSchema'],
        annotations: t.annotations,
      }));
      setDiscoveredTools(tools);
      showToast(`Connected! Found ${result.tool_count} tools.`, 'success');
    } else if (status === 'failed') {
      setDiscoveredTools([]);
    }
  }, [showToast]);

  useEffect(() => {
    saveWizardProgress(currentStep, config);
    setLastSaved(Date.now());
  }, [currentStep, config]);

  useEffect(() => {
    if (hasRestoredProgress) {
      showToast('Your previous progress has been restored', 'info');
      setHasRestoredProgress(false);
    }
  }, [hasRestoredProgress, showToast]);

  const currentStepIndex = STEPS.findIndex(s => s.id === currentStep);

  const isStepValid = useCallback((step: WizardStep): boolean => {
    switch (step) {
      case 'target':
        return isValidUrl(config.target.url) && connectionStatus === 'success';
      case 'stages':
        return config.stages.some(s => s.enabled);
      case 'workload':
        return config.workload.op_mix.length > 0;
      case 'review':
        return true;
      default:
        return false;
    }
  }, [config.target.url, config.stages, config.workload.op_mix.length, connectionStatus]);

  const canNavigateToStep = useCallback((targetStepIndex: number): boolean => {
    if (targetStepIndex <= currentStepIndex) return true;
    for (let i = 0; i < targetStepIndex; i++) {
      if (!isStepValid(STEPS[i].id)) return false;
    }
    return true;
  }, [currentStepIndex, isStepValid]);

  const hasEnabledStages = useMemo(() => isStepValid('stages'), [isStepValid]);
  const hasOperations = useMemo(() => isStepValid('workload'), [isStepValid]);

  const canProceed = useCallback(() => {
    return isStepValid(currentStep);
  }, [currentStep, isStepValid]);

  const handleTargetChange = useCallback((target: TargetConfigType) => {
    setConfig(prev => ({ ...prev, target }));
  }, []);

  const handleStagesChange = useCallback((stages: StageConfigType[]) => {
    setConfig(prev => ({ ...prev, stages }));
  }, []);

  const handleWorkloadChange = useCallback((workload: WorkloadConfigType) => {
    setConfig(prev => ({ ...prev, workload }));
  }, []);

  const handleServerTelemetryChange = useCallback((serverTelemetry: ServerTelemetryConfig | undefined) => {
    setConfig(prev => ({ ...prev, server_telemetry: serverTelemetry }));
  }, []);

  const handleAuthConfigChange = useCallback((config: AuthConfig | undefined) => {
    setAuthConfig(config);
  }, []);

  const handleNext = useCallback(() => {
    const nextIndex = currentStepIndex + 1;
    if (nextIndex < STEPS.length) {
      setCurrentStep(STEPS[nextIndex].id);
    }
  }, [currentStepIndex]);

  const handleBack = useCallback(() => {
    const prevIndex = currentStepIndex - 1;
    if (prevIndex >= 0) {
      setCurrentStep(STEPS[prevIndex].id);
    }
  }, [currentStepIndex]);

  const handleStepClick = useCallback((step: WizardStep) => {
    const targetIndex = STEPS.findIndex(s => s.id === step);
    if (canNavigateToStep(targetIndex)) {
      setCurrentStep(step);
    }
  }, [canNavigateToStep]);

  const handleReset = useCallback(() => {
    clearWizardProgress();
    setConfig(createDefaultConfig());
    setCurrentStep('target');
    setLastSaved(null);
    setShowResetConfirm(false);
    setConnectionStatus('idle');
    setDiscoveredTools([]);
    showToast('Configuration reset to defaults', 'info');
  }, [showToast]);

  const validateAllSteps = useCallback((): string | null => {
    if (!isStepValid('target')) {
      return 'Please enter a valid target URL';
    }
    if (config.server_telemetry?.enabled && !config.server_telemetry.pair_key?.trim()) {
      return 'Pair key is required when server telemetry is enabled';
    }
    if (!isStepValid('stages')) {
      return 'Please enable at least one test stage';
    }
    if (!isStepValid('workload')) {
      return 'Please define at least one operation';
    }
    for (const op of config.workload.op_mix) {
      if (op.operation === 'tools/call' && !op.tool_name?.trim()) {
        return 'Tool name is required for tools/call operations';
      }
      if (op.operation === 'resources/read' && !op.uri?.trim()) {
        return 'Resource URI is required for resources/read operations';
      }
      if (op.operation === 'prompts/get' && !op.prompt_name?.trim()) {
        return 'Prompt name is required for prompts/get operations';
      }
    }
    return null;
  }, [isStepValid, config.server_telemetry, config.workload.op_mix]);

  const handleStart = async () => {
    const validationError = validateAllSteps();
    if (validationError) {
      setError(validationError);
      showToast(validationError, 'error');
      return;
    }

    setIsStarting(true);
    setError(null);

    try {
      const configWithAuth: RunConfig = {
        ...config,
        target: {
          ...config.target,
          auth: authConfig,
        },
      };
      const runId = await createRun(configWithAuth);
      await startRun(runId);
      clearWizardProgress();
      showToast(`Run ${runId} started successfully!`, 'success');
      onRunStarted(runId);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to start run';
      setError(message);
      showToast(message, 'error');
    } finally {
      setIsStarting(false);
    }
  };

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      const target = e.target as HTMLElement;
      const isInInput = target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.tagName === 'SELECT';
      
      if (e.key === 'Escape' && currentStepIndex > 0 && !isInInput) {
        e.preventDefault();
        handleBack();
      }
      
      if ((e.ctrlKey || e.metaKey) && e.key === 'Enter' && currentStep !== 'review') {
        if (canProceed()) {
          e.preventDefault();
          handleNext();
        }
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [currentStep, currentStepIndex, canProceed, handleBack, handleNext]);

  return (
    <div className="run-wizard">
      <nav className="wizard-progress" aria-label="Wizard progress">
        {STEPS.map((step, index) => {
          const isCompleted = index < currentStepIndex;
          const isCurrent = currentStep === step.id;
          const canNavigate = canNavigateToStep(index);
          const isDisabled = !canNavigate && !isCurrent;
          return (
            <button
              key={step.id}
              type="button"
              className={`wizard-step-indicator ${isCurrent ? 'active' : ''} ${isCompleted ? 'completed' : ''} ${isDisabled ? 'disabled' : ''}`}
              onClick={() => handleStepClick(step.id)}
              disabled={isDisabled}
              aria-current={isCurrent ? 'step' : undefined}
              aria-label={`${step.label}${isCompleted ? ' (completed)' : isCurrent ? ' (current)' : isDisabled ? ' (complete previous steps first)' : ''}`}
            >
              <span className="step-number" aria-hidden="true">
                {isCompleted ? <Icon name="check" size="sm" /> : index + 1}
              </span>
              <span className="step-label">{step.label}</span>
            </button>
          );
        })}
        <div 
          className="wizard-progress-bar" 
          role="progressbar" 
          aria-valuenow={currentStepIndex + 1} 
          aria-valuemin={1} 
          aria-valuemax={STEPS.length}
          aria-label={`Step ${currentStepIndex + 1} of ${STEPS.length}`}
        >
          <div
            className="wizard-progress-fill"
            style={{ width: `${(currentStepIndex / (STEPS.length - 1)) * 100}%` }}
          />
        </div>
      </nav>

      <div className="wizard-header">
        <div className="wizard-draft-status">
          {lastSaved && (
            <span className="draft-saved">
              Draft saved {formatRelativeTime(lastSaved)}
            </span>
          )}
          
        </div>
        <button
          type="button"
          className="btn btn-ghost btn-sm"
          onClick={() => setShowResetConfirm(true)}
        >
          <Icon name="rotate-ccw" size="sm" />
          Reset to Defaults
        </button>
      </div>

      <div className="wizard-content">
        {currentStep === 'target' && (
          <>
            <TargetConfig 
              config={config.target} 
              onChange={handleTargetChange}
              onConnectionStatusChange={handleConnectionStatusChange}
              serverTelemetry={config.server_telemetry}
              onServerTelemetryChange={handleServerTelemetryChange}
              authConfig={authConfig}
              onAuthConfigChange={handleAuthConfigChange}
            />
            {isValidUrl(config.target.url) && connectionStatus !== 'success' && (
              <div className="validation-feedback validation-info" role="status">
                <span className="validation-icon" aria-hidden="true"><Icon name="info" size="md" /></span>
                <span>Please test the connection before proceeding.</span>
              </div>
            )}
          </>
        )}
        {currentStep === 'stages' && (
          <>
            <StageConfig stages={config.stages} onChange={handleStagesChange} />
            {!hasEnabledStages && (
              <div className="validation-feedback" role="status">
                <span className="validation-icon" aria-hidden="true"><Icon name="alert-triangle" size="md" /></span>
                <span>At least one stage must be enabled to proceed.</span>
              </div>
            )}
          </>
        )}
        {currentStep === 'workload' && (
          <>
            <WorkloadConfig 
              config={config.workload} 
              onChange={handleWorkloadChange} 
              targetUrl={config.target.url}
              headers={(() => {
                const h = { ...config.target.headers };
                if (authConfig?.type === 'bearer_token' && authConfig.tokens && authConfig.tokens.length > 0) {
                  h['Authorization'] = `Bearer ${authConfig.tokens[0]}`;
                }
                return h;
              })()}
              fetchedTools={discoveredTools}
              onToolsFetched={setDiscoveredTools}
            />
            {!hasOperations && (
              <div className="validation-feedback" role="status">
                <span className="validation-icon" aria-hidden="true"><Icon name="alert-triangle" size="md" /></span>
                <span>At least one operation must be defined to proceed.</span>
              </div>
            )}
          </>
        )}
        {currentStep === 'review' && (
          <RunReview config={config} onStart={handleStart} isStarting={isStarting} error={error} />
        )}
      </div>

      <div className="wizard-navigation">
        <button
          type="button"
          onClick={handleBack}
          disabled={currentStepIndex === 0}
          className="btn btn-secondary"
          aria-label="Go to previous step (Escape)"
        >
          <Icon name="arrow-left" size="sm" aria-hidden={true} /> Back
          {currentStepIndex > 0 && (
            <span className="keyboard-shortcut" aria-hidden="true">
              <kbd>Esc</kbd>
            </span>
          )}
        </button>
        {currentStep !== 'review' && (
          <button
            type="button"
            onClick={handleNext}
            disabled={!canProceed()}
            className="btn btn-primary"
            aria-label="Go to next step (Ctrl+Enter)"
          >
            Next <Icon name="arrow-right" size="sm" aria-hidden={true} />
            <span className="keyboard-shortcut" aria-hidden="true">
              <kbd>Ctrl</kbd>+<kbd>Enter</kbd>
            </span>
          </button>
        )}
      </div>

      <ConfirmDialog
        isOpen={showResetConfirm}
        title="Reset Configuration?"
        message="This will discard all changes and restore default settings."
        confirmLabel="Reset"
        cancelLabel="Cancel"
        variant="warning"
        onConfirm={handleReset}
        onCancel={() => setShowResetConfirm(false)}
      />
    </div>
  );
}
