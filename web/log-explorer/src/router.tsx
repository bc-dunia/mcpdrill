import { createBrowserRouter, Navigate } from 'react-router-dom';
import { AppLayout } from './layouts/AppLayout';
import { LogExplorer } from './components/LogExplorer';
import { RunOverview } from './components/RunOverview';
import { WizardPage } from './pages/WizardPage';
import { ComparePage } from './pages/ComparePage';

export const router = createBrowserRouter(
  [
    {
      path: '/',
      element: <AppLayout />,
      children: [
        {
          index: true,
          element: <LogExplorer />,
        },
        {
          path: 'wizard',
          element: <WizardPage />,
        },
        {
          path: 'runs/:runId',
          element: <RunOverview />,
        },
        {
          path: 'runs/:runId/overview',
          element: <RunOverview />,
        },
        {
          path: 'runs/:runId/logs',
          element: <LogExplorer />,
        },
        {
          path: 'runs/:runId/metrics',
          element: <LogExplorer />,
        },
        {
          path: 'compare',
          element: <ComparePage />,
        },
        {
          path: '*',
          element: <Navigate to="/" replace />,
        },
      ],
    },
  ],
  {
    basename: import.meta.env.BASE_URL,
  }
);
