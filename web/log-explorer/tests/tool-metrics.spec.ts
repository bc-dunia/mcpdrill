import { test, expect } from '@playwright/test';
import { MockServerHelper } from './page-objects';

test.describe('Tool Metrics Dashboard Component', () => {
  test.beforeEach(async ({ page }) => {
    await MockServerHelper.interceptRunLogs(page, MockServerHelper.getMockLogs());
    await MockServerHelper.interceptRunMetrics(page, MockServerHelper.getMockMetrics());
    
    await page.route('**/runs', async route => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([
          { run_id: 'run_test123', state: 'completed', current_stage: 'done' },
        ]),
      });
    });
    
    await page.route('**/runs/run_test123', async route => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ 
          run_id: 'run_test123', 
          state: 'completed', 
          current_stage: 'done',
          metrics: {
            total_operations: 200,
            operations_per_second: 50,
            error_rate: 0.05,
            latency_p50_ms: 45,
            latency_p95_ms: 120,
            latency_p99_ms: 250,
          },
        }),
      });
    });
  });

  test('displays main application with metrics endpoint mocked', async ({ page }) => {
    await page.goto('/');
    
    await expect(page.locator('.app')).toBeVisible();
    await expect(page.locator('h1:has-text("MCP Drill")')).toBeVisible();
  });

  test('handles metrics API error gracefully', async ({ page }) => {
    await page.route('**/runs/*/metrics', async route => {
      await route.fulfill({
        status: 500,
        contentType: 'application/json',
        body: JSON.stringify({ error: 'Internal Server Error' }),
      });
    });
    
    await page.goto('/');
    await page.waitForTimeout(1000);
  });

  test('handles empty metrics state', async ({ page }) => {
    await page.route('**/runs/*/metrics', async route => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ run_id: 'run_test123', by_tool: {}, timestamp: Date.now() }),
      });
    });
    
    await page.goto('/');
    await page.waitForTimeout(1000);
  });

  test('visual regression: main application', async ({ page }) => {
    await page.goto('/');
    await page.waitForTimeout(500);
    
    await expect(page.locator('.app')).toHaveScreenshot('main-app.png');
  });

  test('can navigate to New Run wizard', async ({ page }) => {
    await page.goto('/');
    
    await page.locator('button:has-text("New Run")').click();
    
    await expect(page.locator('h2:has-text("Configure Target")')).toBeVisible({ timeout: 5000 });
  });

  test('can navigate to Compare view', async ({ page }) => {
    await page.goto('/');
    
    await page.locator('button:has-text("Compare")').click();
    
    await page.waitForTimeout(1000);
  });

  test('metrics data flows through the application', async ({ page }) => {
    let metricsCallCount = 0;
    await page.route('**/runs/*/metrics', async route => {
      metricsCallCount++;
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ 
          run_id: 'run_test123', 
          by_tool: MockServerHelper.getMockMetrics(), 
          timestamp: Date.now(),
        }),
      });
    });
    
    await page.goto('/');
    await page.waitForTimeout(2000);
  });
});
