# UI/UX Issues Found - E2E Telemetry Test

## Screenshot Review: `05-test-running.png`
**Review Date:** 2026-02-01
**Screenshot Path:** `screenshots/e2e-telemetry-test-20260201-003535/05-test-running.png`

---

## Critical Issues

### Issue 1: Raw JSON Dump with No Page Structure
**Severity:** HIGH  
**Location:** Run detail view  
**Description:** The page displays raw JSON with no UI chrome - no page header, navigation, breadcrumbs, or context labels. Users must infer what the data represents.

### Issue 2: JSON Horizontal Overflow / Truncation
**Severity:** HIGH  
**Location:** JSON content area  
**Description:** The JSON is rendered as a single long line that overflows the viewport horizontally. The word "activ" is visibly truncated at the edge. No scroll indicators are visible, making it impossible to read the full content.

### Issue 3: Pretty-Print Checkbox State Mismatch
**Severity:** MEDIUM  
**Location:** Pretty-print control (top-left)  
**Description:** The "Pretty-print" checkbox exists but is unchecked, and the JSON remains in compact single-line format. The control's presence suggests the feature exists, but the current state shows unformatted JSON - potentially confusing UX where users may not realize they need to check the box.

---

## Layout & Spacing Issues

### Issue 4: No Page Margins/Padding
**Severity:** MEDIUM  
**Location:** Page container  
**Description:** Content is pinned to the extreme top-left corner with no page-level padding or margins. This creates an unpolished, cramped appearance.

### Issue 5: Massive Unused Whitespace
**Severity:** MEDIUM  
**Location:** Below JSON content  
**Description:** The JSON occupies only a thin strip at the top (~60-80px), leaving the entire rest of the viewport (90%+) as blank white space. Poor use of available screen real estate.

### Issue 6: Detached Control Element
**Severity:** LOW  
**Location:** Pretty-print checkbox  
**Description:** The "Pretty-print" control appears isolated with no panel, header, or container grouping. It lacks visual connection to the content it controls.

---

## Visual Consistency Issues

### Issue 7: Raw Debug Output Styling
**Severity:** MEDIUM  
**Location:** Entire page  
**Description:** The page looks like raw debug output rather than a designed application view. The checkbox uses default browser styling that doesn't match an "app" presentation. No visual design system is apparent.

### Issue 8: Monospace Font Without Syntax Highlighting
**Severity:** LOW  
**Location:** JSON content  
**Description:** JSON is displayed in plain monospace text with no syntax highlighting (keys, values, strings, numbers all same color). This makes the data harder to scan and understand.

---

## UX Pattern Issues

### Issue 9: No Structured Data Rendering
**Severity:** MEDIUM  
**Location:** Data display  
**Description:** Key fields like `state`, `scenario_id`, timestamps are buried in raw JSON. A well-designed view would extract and display these prominently with proper labels.

### Issue 10: Missing Status/Progress Indicators
**Severity:** LOW  
**Location:** Missing from page  
**Description:** For a "test running" view, there's no visible progress indicator, timeline, status badge, or stage visualization. Users must parse the JSON `state` field manually.

### Issue 11: No Copy/Action Affordances
**Severity:** LOW  
**Location:** IDs and hashes  
**Description:** Raw IDs like `run_188fdf20e270b1300002` and long config hashes are displayed without copy buttons, tooltips, or any interactive affordances.

---

## Summary for Task 13

**Total Issues Found:** 11
- Critical/High: 2
- Medium: 5
- Low: 4

**Recommended Fix Priority:**
1. Fix JSON overflow/truncation (Issue 2) - Content is unreadable
2. Add page structure with proper padding/margins (Issues 1, 4)
3. Ensure pretty-print works and defaults to ON for readability (Issue 3)
4. Add basic visual styling to match app design system (Issue 7)

**Note:** This appears to be a raw detail view endpoint (`/runs/{id}`) rendered directly. May need a proper detail page component with structured display of run information.

---

## Resolution (Task 13)

**Date:** 2026-02-01

### Root Cause Analysis

All 11 issues identified above were caused by **navigating to the wrong URL path** during the E2E test screenshot capture.

**The Problem:**
- Screenshot was taken at: `http://localhost:5173/runs/{run_id}`
- Correct URL should be: `http://localhost:5173/ui/logs/runs/{run_id}`

**Why This Happened:**
1. Vite dev server proxies `/runs` to backend API at `http://localhost:8080`
2. Browser navigation to `/runs/xxx` triggers the proxy
3. Proxy returns raw JSON from the backend API
4. The "Pretty-print" checkbox was NOT from the React app - it was browser dev tools or a JSON viewer extension

**Evidence:**
- When accessed via correct URL, `RunOverview.tsx` renders properly with:
  - Full page structure, header, navigation
  - Status badges and timestamps
  - Key Metrics cards
  - Stage Timeline
  - Action buttons
- `MetricsDashboard.tsx` shows properly styled charts and data

### Issues Status

| Issue # | Status | Reason |
|---------|--------|--------|
| 1 | NOT A BUG | Was viewing raw API response, not React UI |
| 2 | NOT A BUG | Was viewing raw API response, not React UI |
| 3 | NOT A BUG | "Pretty-print" checkbox was browser/extension, not app |
| 4 | NOT A BUG | Was viewing raw API response, not React UI |
| 5 | NOT A BUG | Was viewing raw API response, not React UI |
| 6 | NOT A BUG | Was viewing raw API response, not React UI |
| 7 | NOT A BUG | Was viewing raw API response, not React UI |
| 8 | NOT A BUG | Was viewing raw API response, not React UI |
| 9 | NOT A BUG | Was viewing raw API response, not React UI |
| 10 | NOT A BUG | Was viewing raw API response, not React UI |
| 11 | NOT A BUG | Was viewing raw API response, not React UI |

### Verification Screenshots

Two verification screenshots were captured showing the properly styled UI:
- `screenshots/e2e-telemetry-test-20260201-003535/06-run-overview-fixed.png` - RunOverview component
- `screenshots/e2e-telemetry-test-20260201-003535/07-metrics-view-fixed.png` - MetricsDashboard component

### Recommendation

For future E2E tests, always use the `/ui/logs/` prefix for frontend routes:
- ❌ `http://localhost:5173/runs/{id}` → returns raw JSON
- ✅ `http://localhost:5173/ui/logs/runs/{id}` → returns React UI
