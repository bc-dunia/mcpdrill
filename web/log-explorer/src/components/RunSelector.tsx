import { memo, useMemo } from 'react';
import type { RunInfo } from '../types';
import { SearchableSelect } from './SearchableSelect';

interface RunSelectorProps {
  label: string;
  runs: RunInfo[];
  selectedRunId: string;
  onChange: (runId: string) => void;
  excludeRunId?: string;
  disabled?: boolean;
}

function RunSelectorComponent({
  label,
  runs,
  selectedRunId,
  onChange,
  excludeRunId,
  disabled = false,
}: RunSelectorProps) {
  const filteredRuns = useMemo(
    () => excludeRunId ? runs.filter(run => run.id !== excludeRunId) : runs,
    [runs, excludeRunId]
  );

  const runOptions = useMemo(
    () => filteredRuns.map(run => ({
      value: run.id,
      label: run.id,
      sublabel: `${run.scenario_id} • ${run.state}`,
    })),
    [filteredRuns]
  );

  const selectedRun = useMemo(
    () => runs.find(r => r.id === selectedRunId),
    [runs, selectedRunId]
  );
  
  const isCompleted = selectedRun?.state === 'completed';
  const isRunning = selectedRun?.state?.startsWith('running');

  const selectId = `run-selector-${label.toLowerCase().replace(/\s+/g, '-')}`;

  return (
    <div className="comparison-run-selector">
      <label htmlFor={selectId} className="selector-label">{label}</label>
      <SearchableSelect
        id={selectId}
        options={runOptions}
        value={selectedRunId}
        onChange={onChange}
        placeholder="Select a run..."
        disabled={disabled}
      />
      {selectedRunId && (
        <div className="selector-status">
          {isCompleted && (
            <span className="status-indicator status-completed">
              <span className="status-dot" />
              Completed
            </span>
          )}
          {isRunning && (
            <span className="status-indicator status-running">
              <span className="status-dot status-dot-pulse" />
              Running
            </span>
          )}
          {!isCompleted && !isRunning && selectedRun && (
            <span className="status-indicator status-other">
              <span className="status-dot" />
              {selectedRun.state}
            </span>
          )}
        </div>
      )}
    </div>
  );
}

export const RunSelector = memo(RunSelectorComponent);
