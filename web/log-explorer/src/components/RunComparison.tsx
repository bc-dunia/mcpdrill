import { useState, useEffect, useCallback } from 'react';
import type { RunInfo, ComparisonResult } from '../types';
import { fetchRuns, fetchComparison } from '../api/index';
import { RunSelector } from './RunSelector';
import { ComparisonChart } from './ComparisonChart';
import { ComparisonTable } from './ComparisonTable';
import { Icon } from './Icon';

interface RunComparisonProps {
  initialRunA?: string;
  initialRunB?: string;
  onUrlChange?: (runA: string, runB: string) => void;
}

export function RunComparison({ initialRunA, initialRunB, onUrlChange }: RunComparisonProps) {
  const [runs, setRuns] = useState<RunInfo[]>([]);
  const [runIdA, setRunIdA] = useState(initialRunA || '');
  const [runIdB, setRunIdB] = useState(initialRunB || '');
  const [comparison, setComparison] = useState<ComparisonResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [runsLoading, setRunsLoading] = useState(true);

  useEffect(() => {
    setRunsLoading(true);
    fetchRuns()
      .then(setRuns)
      .catch(err => setError(err.message))
      .finally(() => setRunsLoading(false));
  }, []);

  const loadComparison = useCallback(async (idA: string, idB: string) => {
    if (!idA || !idB) {
      setComparison(null);
      return;
    }

    setLoading(true);
    setError(null);

    try {
      const result = await fetchComparison(idA, idB);
      setComparison(result);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load comparison');
      setComparison(null);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (runIdA && runIdB) {
      loadComparison(runIdA, runIdB);
      onUrlChange?.(runIdA, runIdB);
    }
  }, [runIdA, runIdB, loadComparison, onUrlChange]);

  const handleSwapRuns = useCallback(() => {
    setRunIdA(runIdB);
    setRunIdB(runIdA);
  }, [runIdA, runIdB]);

   const handleClear = useCallback(() => {
     setRunIdA('');
     setRunIdB('');
     setComparison(null);
     setError(null);
     onUrlChange?.('', '');
   }, [onUrlChange]);

  const runA = runs.find(r => r.id === runIdA);
  const runB = runs.find(r => r.id === runIdB);
  const bothSelected = runIdA && runIdB;
  const configMismatch = runA && runB && runA.scenario_id !== runB.scenario_id;

  return (
    <section className="run-comparison" aria-labelledby="comparison-heading">
      <div className="comparison-header">
        <div className="comparison-title">
          <span className="title-icon" aria-hidden="true"><Icon name="scale" size="lg" /></span>
          <div className="title-text">
            <h2 id="comparison-heading">A/B Comparison</h2>
            <p>Compare metrics between two runs to identify performance changes</p>
          </div>
        </div>
      </div>

      <div className="comparison-selectors">
        <RunSelector
          label="Run A (Baseline)"
          runs={runs}
          selectedRunId={runIdA}
          onChange={setRunIdA}
          excludeRunId={runIdB}
          disabled={runsLoading}
        />

        <button
          type="button"
          className="swap-button"
          onClick={handleSwapRuns}
          disabled={!bothSelected}
          aria-label="Swap run A and run B"
        >
          <Icon name="swap" size="sm" aria-hidden={true} />
        </button>

        <RunSelector
          label="Run B (Comparison)"
          runs={runs}
          selectedRunId={runIdB}
          onChange={setRunIdB}
          excludeRunId={runIdA}
          disabled={runsLoading}
        />

        {bothSelected && (
          <button type="button" className="btn btn-ghost clear-button" onClick={handleClear}>
            Clear
          </button>
        )}
      </div>

      {configMismatch && (
        <div className="comparison-warning" role="alert">
          <span className="warning-icon" aria-hidden="true"><Icon name="alert-triangle" size="md" /></span>
          <div className="warning-content">
            <strong>Different configurations detected</strong>
            <p>
              Run A uses <code>{runA.scenario_id}</code> while Run B uses <code>{runB.scenario_id}</code>.
              Results may not be directly comparable.
            </p>
          </div>
        </div>
      )}



      {error && (
        <div className="comparison-error" role="alert">
          <span className="error-icon" aria-hidden="true"><Icon name="x-circle" size="sm" /></span>
          <span>{error}</span>
        </div>
      )}

      {loading && (
        <div className="comparison-loading" role="status" aria-label="Loading comparison">
          <div className="loading-spinner" aria-hidden="true" />
          <span>Loading comparison data...</span>
        </div>
      )}

      {!bothSelected && !loading && (
        <div className="comparison-empty">
          <div className="empty-visual">
            <div className="empty-box empty-box-a">A</div>
            <span className="empty-vs">vs</span>
            <div className="empty-box empty-box-b">B</div>
          </div>
          <h3>Select Two Runs to Compare</h3>
          <p>Choose a baseline run (A) and a comparison run (B) to see detailed metrics differences.</p>
        </div>
      )}

      {comparison && !loading && (
        <div className="comparison-results">
          <div className="results-summary">
            <div className="summary-card summary-card-a">
              <div className="summary-label">Run A</div>
              <div className="summary-id">{comparison.run_a.run_id}</div>
              <div className="summary-scenario">{runA?.scenario_id || ''}</div>
            </div>
            <div className="summary-vs">
              <span>VS</span>
            </div>
            <div className="summary-card summary-card-b">
              <div className="summary-label">Run B</div>
              <div className="summary-id">{comparison.run_b.run_id}</div>
              <div className="summary-scenario">{runB?.scenario_id || ''}</div>
            </div>
          </div>

          <section className="comparison-section">
            <ComparisonChart comparison={comparison} />
          </section>

          <section className="comparison-section" aria-labelledby="detailed-metrics-title">
            <h3 id="detailed-metrics-title" className="section-title">
              <span className="section-icon" aria-hidden="true"><Icon name="chart-bar" size="md" /></span>
              Detailed Metrics
            </h3>
            <ComparisonTable comparison={comparison} />
          </section>
        </div>
      )}
    </section>
  );
}
