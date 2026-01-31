import { lazy, Suspense, useCallback } from 'react';
import { useSearchParams } from 'react-router-dom';

const RunComparison = lazy(() => import('../components/RunComparison').then(m => ({ default: m.RunComparison })));

function LoadingFallback() {
  return (
    <div className="lazy-loading-fallback" role="status" aria-label="Loading">
      <div className="spinner" aria-hidden="true" />
      <span>Loading...</span>
    </div>
  );
}

export function ComparePage() {
  const [searchParams, setSearchParams] = useSearchParams();

  const runA = searchParams.get('runA') || undefined;
  const runB = searchParams.get('runB') || undefined;

  const handleUrlChange = useCallback((newRunA: string, newRunB: string) => {
    const params = new URLSearchParams();
    if (newRunA) params.set('runA', newRunA);
    if (newRunB) params.set('runB', newRunB);
    setSearchParams(params, { replace: true });
  }, [setSearchParams]);

  return (
    <Suspense fallback={<LoadingFallback />}>
      <RunComparison
        initialRunA={runA}
        initialRunB={runB}
        onUrlChange={handleUrlChange}
      />
    </Suspense>
  );
}
