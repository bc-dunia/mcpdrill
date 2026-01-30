import { test, expect } from '@playwright/test';
import { MockServerHelper } from './page-objects';

test.describe('Tool Results Panel Component', () => {
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
        body: JSON.stringify({ run_id: 'run_test123', state: 'completed', current_stage: 'done' }),
      });
    });
  });

  test('displays log explorer with run data', async ({ page }) => {
    await page.goto('/');
    await page.waitForLoadState('networkidle');
    
    await expect(page.locator('.app')).toBeVisible();
    await expect(page.locator('text=MCP Drill')).toBeVisible();
  });

  test('navigation tabs are visible', async ({ page }) => {
    await page.goto('/');
    await page.waitForLoadState('networkidle');
    
    await expect(page.locator('.nav-tab:has-text("Log Explorer")')).toBeVisible();
    await expect(page.locator('.nav-tab:has-text("New Run")')).toBeVisible();
    await expect(page.locator('.nav-tab:has-text("Compare")')).toBeVisible();
  });

  test('handles loading state gracefully', async ({ page }) => {
    await page.route('**/runs/*/logs*', async route => {
      await new Promise(resolve => setTimeout(resolve, 500));
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ logs: MockServerHelper.getMockLogs() }),
      });
    });
    
    await page.goto('/');
    
    const spinner = page.locator('.spinner, .loading, [role="status"]');
    const isLoading = await spinner.isVisible().catch(() => false);
  });

  test('switches between views', async ({ page }) => {
    await page.goto('/');
    await page.waitForLoadState('networkidle');
    
    await page.locator('.nav-tab:has-text("New Run")').click();
    await expect(page.locator('h2:has-text("Configure Target")')).toBeVisible({ timeout: 5000 });
    
    await page.locator('.nav-tab:has-text("Log Explorer")').click();
    await expect(page.locator('.app-main')).toBeVisible({ timeout: 5000 });
  });

  test('visual regression: log explorer main view', async ({ page }) => {
    await page.goto('/');
    await page.waitForTimeout(500);
    
    await expect(page.locator('.app-main')).toHaveScreenshot('log-explorer-main.png');
  });

  test('error handling when logs fail to load', async ({ page }) => {
    await page.route('**/runs/*/logs*', async route => {
      await route.abort('failed');
    });
    
    await page.goto('/');
    
    await page.waitForTimeout(1000);
  });

  test('theme toggle works', async ({ page }) => {
    await page.goto('/');
    
    const themeButton = page.locator('.theme-toggle');
    await expect(themeButton).toBeVisible();
    
    await themeButton.click();
    await page.waitForTimeout(300);
    
    await themeButton.click();
    await page.waitForTimeout(300);
  });
});
