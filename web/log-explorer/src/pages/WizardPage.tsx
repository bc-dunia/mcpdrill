import { lazy, Suspense, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';

const RunWizard = lazy(() => import('../components/RunWizard').then(m => ({ default: m.RunWizard })));

function LoadingFallback() {
  return (
    <div className="lazy-loading-fallback" role="status" aria-label="Loading">
      <div className="spinner" aria-hidden="true" />
      <span>Loading...</span>
    </div>
  );
}

export function WizardPage() {
  const navigate = useNavigate();

  const handleRunStarted = useCallback((runId: string) => {
    navigate(`/runs/${runId}`);
  }, [navigate]);

  return (
    <Suspense fallback={<LoadingFallback />}>
      <RunWizard onRunStarted={handleRunStarted} />
    </Suspense>
  );
}
