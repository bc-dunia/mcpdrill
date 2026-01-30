import { memo } from 'react';
import type { StageConfig as StageConfigType } from '../types';
import { GlossaryTerm } from './GlossaryTerm';
import { Icon, type IconName } from './Icon';

interface HelpTooltipProps {
  text: string;
}

const HelpTooltip = memo(function HelpTooltip({ text }: HelpTooltipProps) {
  const tooltipId = `help-${Math.random().toString(36).substring(2, 9)}`;
  return (
    <button 
      type="button"
      className="help-tooltip" 
      aria-describedby={tooltipId}
      aria-label="Help"
    >
      ?
      <span id={tooltipId} className="help-tooltip-content" role="tooltip">
        {text}
      </span>
    </button>
  );
});

interface Props {
  stages: StageConfigType[];
  onChange: (stages: StageConfigType[]) => void;
}

interface StageInfo {
  name: string;
  description: string;
  icon: IconName;
  help: string;
}

const STAGE_LABELS: Record<string, StageInfo> = {
  preflight: {
    name: 'Preflight',
    description: 'Initial connectivity check with minimal load',
    icon: 'plane-takeoff',
    help: 'Verifies your target is reachable before running the full test. Uses 1 VU to check basic connectivity and response.',
  },
  baseline: {
    name: 'Baseline',
    description: 'Establish performance baseline under steady load',
    icon: 'chart-bar',
    help: 'Runs a steady, low load to establish normal performance metrics. Use this as your reference point for comparison.',
  },
  ramp: {
    name: 'Ramp',
    description: 'Gradually increase load to find limits',
    icon: 'trending-up',
    help: 'Progressively increases virtual users to find your system\'s breaking point. Watch for latency spikes or error rate increases.',
  },
};

function formatDuration(ms: number): string {
  const seconds = Math.floor(ms / 1000);
  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = seconds % 60;
  if (minutes > 0) {
    return remainingSeconds > 0 ? `${minutes}m ${remainingSeconds}s` : `${minutes}m`;
  }
  return `${seconds}s`;
}

export function StageConfig({ stages, onChange }: Props) {
  const updateStage = (index: number, updates: Partial<StageConfigType>) => {
    const newStages = [...stages];
    newStages[index] = { ...newStages[index], ...updates };
    onChange(newStages);
  };

  const updateLoad = (index: number, targetVUs: number) => {
    const newStages = [...stages];
    newStages[index] = {
      ...newStages[index],
      load: { target_vus: targetVUs },
    };
    onChange(newStages);
  };

  return (
    <div className="wizard-step">
      <div className="step-header">
        <div className="step-icon" aria-hidden="true"><Icon name="zap" size="lg" /></div>
        <div className="step-title">
          <h2>Define Stages</h2>
          <p>Configure the test progression through preflight, baseline, and ramp phases</p>
        </div>
      </div>

      <div className="stages-grid">
        {stages.map((stage, index) => {
          const meta = STAGE_LABELS[stage.stage];
          return (
            <div
              key={stage.stage_id}
              className={`stage-card ${stage.enabled ? 'stage-enabled' : 'stage-disabled'}`}
            >
              <div className="stage-card-header">
                <div className="stage-card-title">
                  <span className="stage-icon" aria-hidden="true"><Icon name={meta.icon} size="sm" /></span>
                  <span className="stage-name">{meta.name}</span>
                  <HelpTooltip text={meta.help} />
                </div>
                <label className="toggle">
                  <input
                    type="checkbox"
                    checked={stage.enabled}
                    onChange={e => updateStage(index, { enabled: e.target.checked })}
                    aria-label={`Enable ${meta.name} stage`}
                  />
                  <span className="toggle-slider" />
                </label>
              </div>

              <p className="stage-description">{meta.description}</p>

              <div className="stage-fields">
                <div className="stage-field">
                  <label htmlFor={`stage-duration-${stage.stage_id}`}>Duration</label>
                  <div className="duration-input">
                    <input
                      id={`stage-duration-${stage.stage_id}`}
                      type="number"
                      min="1"
                      value={Math.floor(stage.duration_ms / 1000)}
                      onChange={e => {
                        const value = parseInt(e.target.value);
                        if (!isNaN(value) && value > 0) {
                          updateStage(index, { duration_ms: value * 1000 });
                        }
                      }}
                      className="input input-number"
                      disabled={!stage.enabled}
                      aria-describedby={`stage-duration-${stage.stage_id}-hint`}
                    />
                    <span className="input-suffix" id={`stage-duration-${stage.stage_id}-hint`}>seconds</span>
                  </div>
                </div>

                <div className="stage-field">
                  <label htmlFor={`stage-vus-${stage.stage_id}`}>
                    <GlossaryTerm term="Virtual Users" definition="Simulated clients making requests to your target server">
                      Virtual Users
                    </GlossaryTerm>
                  </label>
                  <div className="vu-input">
                    <input
                      id={`stage-vus-${stage.stage_id}`}
                      type="number"
                      min="1"
                      max="1000"
                      value={stage.load.target_vus}
                      onChange={e => {
                        const value = parseInt(e.target.value);
                        if (!isNaN(value) && value >= 1 && value <= 1000) {
                          updateLoad(index, value);
                        }
                      }}
                      className="input input-number"
                      disabled={!stage.enabled}
                      aria-describedby={`stage-vus-${stage.stage_id}-hint`}
                    />
                    <span className="input-suffix" id={`stage-vus-${stage.stage_id}-hint`}>VUs</span>
                  </div>
                </div>
              </div>

              {stage.enabled && (
                <div className="stage-summary">
                  <span className="summary-badge">
                    {stage.load.target_vus} VU{stage.load.target_vus !== 1 ? 's' : ''} for {formatDuration(stage.duration_ms)}
                  </span>
                </div>
              )}
            </div>
          );
        })}
      </div>

      <div className="stages-timeline">
        <div className="timeline-label">Test Timeline</div>
        <div className="timeline-bar">
          {stages.filter(s => s.enabled).map((stage) => {
            const totalDuration = stages.filter(s => s.enabled).reduce((sum, s) => sum + s.duration_ms, 0);
            const width = totalDuration > 0 ? (stage.duration_ms / totalDuration) * 100 : 0;
            const meta = STAGE_LABELS[stage.stage];
            return (
              <div
                key={stage.stage_id}
                className={`timeline-segment timeline-${stage.stage}`}
                style={{ width: `${width}%` }}
                title={`${meta.name}: ${formatDuration(stage.duration_ms)}`}
              >
                <span className="timeline-segment-label">{meta.name}</span>
              </div>
            );
          })}
        </div>
        <div className="timeline-total">
          Total: {formatDuration(stages.filter(s => s.enabled).reduce((sum, s) => sum + s.duration_ms, 0))}
        </div>
      </div>
    </div>
  );
}
