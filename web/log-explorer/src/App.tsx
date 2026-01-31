/**
 * App.tsx - Legacy entry point
 * 
 * Note: This file is kept for backwards compatibility but is no longer used.
 * The app now uses React Router with AppLayout as the root layout component.
 * See: src/router.tsx and src/layouts/AppLayout.tsx
 * 
 * The main entry point is now:
 * - main.tsx -> RouterProvider -> router.tsx -> AppLayout
 */

import './App.css'

// Legacy export - not currently used
// App routing is handled by RouterProvider in main.tsx
export default function App() {
  return null
}
