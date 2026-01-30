import { useState, useEffect, useCallback, lazy, Suspense } from 'react'
import { ErrorBoundary } from './components/ErrorBoundary'
import { LogExplorer } from './components/LogExplorer'
import { ToastProvider } from './components/Toast'
import { Icon } from './components/Icon'
import { useTheme } from './contexts/ThemeContext'
import './App.css'

// Lazy load heavy components for better initial bundle size
const RunWizard = lazy(() => import('./components/RunWizard').then(m => ({ default: m.RunWizard })));
const RunComparison = lazy(() => import('./components/RunComparison').then(m => ({ default: m.RunComparison })));

function LoadingFallback() {
  return (
    <div className="lazy-loading-fallback" role="status" aria-label="Loading">
      <div className="spinner" aria-hidden="true" />
      <span>Loading...</span>
    </div>
  );
}

type AppView = 'explorer' | 'wizard' | 'compare';

function getInitialView(): { view: AppView; runA?: string; runB?: string } {
  const params = new URLSearchParams(window.location.search);
  const runA = params.get('runA') || undefined;
  const runB = params.get('runB') || undefined;
  if (runA || runB) {
    return { view: 'compare', runA, runB };
  }
  return { view: 'explorer' };
}

export default function App() {
  const { theme, toggleTheme } = useTheme();
  const [initialState] = useState(getInitialView);
  const [view, setView] = useState<AppView>(initialState.view);
  const [lastStartedRunId, setLastStartedRunId] = useState<string | null>(null);
  const [compareRunA, setCompareRunA] = useState(initialState.runA || '');
  const [compareRunB, setCompareRunB] = useState(initialState.runB || '');

  const handleRunStarted = (runId: string) => {
    setLastStartedRunId(runId);
    setView('explorer');
  };

  const handleCompareUrlChange = useCallback((runA: string, runB: string) => {
    setCompareRunA(runA);
    setCompareRunB(runB);
    const params = new URLSearchParams();
    if (runA) params.set('runA', runA);
    if (runB) params.set('runB', runB);
    const newUrl = params.toString() ? `?${params.toString()}` : window.location.pathname;
    window.history.replaceState({}, '', newUrl);
  }, []);

  useEffect(() => {
    if (view !== 'compare') {
      window.history.replaceState({}, '', window.location.pathname);
    }
  }, [view]);

  return (
    <ToastProvider>
      <div className="app">
        <a href="#main-content" className="skip-link">
          Skip to main content
        </a>
        <header className="app-header">
          <div className="logo">
            <span className="logo-icon" aria-hidden="true"><Icon name="microscope" size="lg" /></span>
            <h1 className="logo-text">MCP Drill</h1>
          </div>
          <div className="breadcrumb-context" aria-label="Current location">
            <span className="breadcrumb-separator" aria-hidden="true">/</span>
            <span className="breadcrumb-current">
              {view === 'explorer' && 'Log Explorer'}
              {view === 'wizard' && 'New Run'}
              {view === 'compare' && 'Compare Runs'}
            </span>
          </div>
          <nav className="app-nav" aria-label="Main navigation">
            <button
              type="button"
              className={`nav-tab ${view === 'explorer' ? 'active' : ''}`}
              onClick={() => setView('explorer')}
              aria-current={view === 'explorer' ? 'page' : undefined}
            >
              <span className="nav-icon" aria-hidden="true"><Icon name="chart-bar" size="md" /></span>
              Log Explorer
            </button>
            <button
              type="button"
              className={`nav-tab ${view === 'wizard' ? 'active' : ''}`}
              onClick={() => setView('wizard')}
              aria-current={view === 'wizard' ? 'page' : undefined}
            >
              <span className="nav-icon" aria-hidden="true"><Icon name="rocket" size="md" /></span>
              New Run
            </button>
            <button
              type="button"
              className={`nav-tab ${view === 'compare' ? 'active' : ''}`}
              onClick={() => setView('compare')}
              aria-current={view === 'compare' ? 'page' : undefined}
            >
              <span className="nav-icon" aria-hidden="true"><Icon name="scale" size="md" /></span>
              Compare
            </button>
          </nav>
          <button
            type="button"
            className="theme-toggle"
            onClick={toggleTheme}
            aria-label={`Switch to ${theme === 'light' ? 'dark' : 'light'} mode`}
            title={`Switch to ${theme === 'light' ? 'dark' : 'light'} mode`}
          >
            <Icon name={theme === 'light' ? 'moon' : 'sun'} size="md" />
          </button>
          {lastStartedRunId && view === 'explorer' && (
            <div className="run-started-badge" role="status" aria-live="polite">
              <span className="badge-icon" aria-hidden="true"><Icon name="check" size="sm" /></span>
              Run started: {lastStartedRunId}
            </div>
          )}
        </header>
        <main className="app-main" id="main-content" tabIndex={-1}>
          <ErrorBoundary>
            {view === 'explorer' && <LogExplorer onNavigateToWizard={() => setView('wizard')} />}
            {view === 'wizard' && (
              <Suspense fallback={<LoadingFallback />}>
                <RunWizard onRunStarted={handleRunStarted} />
              </Suspense>
            )}
            {view === 'compare' && (
              <Suspense fallback={<LoadingFallback />}>
                <RunComparison
                  initialRunA={compareRunA}
                  initialRunB={compareRunB}
                  onUrlChange={handleCompareUrlChange}
                />
              </Suspense>
            )}
          </ErrorBoundary>
        </main>
      </div>
    </ToastProvider>
  )
}
