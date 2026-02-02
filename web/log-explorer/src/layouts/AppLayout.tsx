import { Suspense } from 'react';
import { Outlet, NavLink, useLocation } from 'react-router-dom';
import { ErrorBoundary } from '../components/ErrorBoundary';
import { Icon } from '../components/Icon';
import { useTheme } from '../contexts/ThemeContext';
import { ToastProvider } from '../components/Toast';

function LoadingFallback() {
  return (
    <div className="lazy-loading-fallback" role="status" aria-label="Loading">
      <div className="spinner" aria-hidden="true" />
      <span>Loading...</span>
    </div>
  );
}

function getBreadcrumbText(pathname: string): string {
  if (pathname === '/' || pathname.startsWith('/runs/')) {
    return 'Log Explorer';
  }
  if (pathname === '/wizard') {
    return 'New Run';
  }
  if (pathname === '/compare') {
    return 'Compare Runs';
  }
  return 'Log Explorer';
}

export function AppLayout() {
  const { theme, toggleTheme } = useTheme();
  const location = useLocation();
  const breadcrumbText = getBreadcrumbText(location.pathname);

  return (
    <ToastProvider>
      <div className="app">
        <a href="#main-content" className="skip-link">
          Skip to main content
        </a>
        <div className="promo-banner promo-banner-top">
          <span>Looking to run MCP infrastructure reliably in production?</span>
          {' '}
          <a href="https://github.com/dunialabs/peta-core" target="_blank" rel="noopener noreferrer">
            Check out Peta →
          </a>
        </div>
        <header className="app-header">
          <div className="logo">
            <span className="logo-icon" aria-hidden="true">
              <Icon name="microscope" size="lg" />
            </span>
            <h1 className="logo-text">MCP Drill</h1>
          </div>
          <div className="breadcrumb-context" aria-label="Current location">
            <span className="breadcrumb-separator" aria-hidden="true">/</span>
            <span className="breadcrumb-current">{breadcrumbText}</span>
          </div>
          <nav className="app-nav" aria-label="Main navigation">
            <NavLink
              to="/"
              end
              className={({ isActive }) => `nav-tab ${isActive || location.pathname.startsWith('/runs/') ? 'active' : ''}`}
              aria-current={location.pathname === '/' || location.pathname.startsWith('/runs/') ? 'page' : undefined}
            >
              <span className="nav-icon" aria-hidden="true">
                <Icon name="chart-bar" size="md" />
              </span>
              Log Explorer
            </NavLink>
            <NavLink
              to="/wizard"
              className={({ isActive }) => `nav-tab ${isActive ? 'active' : ''}`}
            >
              <span className="nav-icon" aria-hidden="true">
                <Icon name="rocket" size="md" />
              </span>
              New Run…
            </NavLink>
            <NavLink
              to="/compare"
              className={({ isActive }) => `nav-tab ${isActive ? 'active' : ''}`}
            >
              <span className="nav-icon" aria-hidden="true">
                <Icon name="scale" size="md" />
              </span>
              Compare
            </NavLink>
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
        </header>
        <main className="app-main" id="main-content" tabIndex={-1}>
          <ErrorBoundary>
            <Suspense fallback={<LoadingFallback />}>
              <Outlet />
            </Suspense>
          </ErrorBoundary>
        </main>
        <footer className="promo-banner promo-banner-bottom">
          <span>MCP Drill was originally created to validate the production stability of</span>
          {' '}
          <a href="https://github.com/dunialabs/peta-core" target="_blank" rel="noopener noreferrer">
            Peta
          </a>
          <span>, an MCP control plane and runtime.</span>
        </footer>
      </div>
    </ToastProvider>
  );
}
