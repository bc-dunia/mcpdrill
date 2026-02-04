import { test, expect } from '@playwright/test';
import { RunWizardPage, ToolSelectorPage, ToolArgumentsEditorPage, MockServerHelper } from './page-objects';

test.describe('Tool Testing Workflow E2E', () => {
  test('complete tool discovery to configuration workflow', async ({ page }) => {
    await MockServerHelper.interceptToolsList(page, MockServerHelper.getMockTools());
    await MockServerHelper.interceptTestConnection(page, MockServerHelper.getMockTools());
    await MockServerHelper.interceptRunLogs(page, MockServerHelper.getMockLogs());
    await MockServerHelper.interceptRunMetrics(page, MockServerHelper.getMockMetrics());
    
    const wizard = new RunWizardPage(page);
    const toolSelector = new ToolSelectorPage(page);
    
    await wizard.goto();
    await wizard.setTargetUrl('http://localhost:3000');
    await wizard.testConnection();
    await wizard.clickNext();
    
    await wizard.waitForStep('Stages');
    await wizard.clickNext();
    
    await wizard.waitForStep('Workload');
    
    const opSelect = page.locator('.operation-select').first();
    await opSelect.selectOption('tools/call');
    
    await page.locator('button:has-text("Browse")').first().click();
    await expect(page.locator('.tool-selector')).toBeVisible({ timeout: 5000 });
    
    await toolSelector.fetchTools();
    
    const toolCount = await toolSelector.getToolCount();
    expect(toolCount).toBeGreaterThan(0);
    
    await toolSelector.selectTool('echo');
    
    const argsSection = page.locator('.tool-arguments-section');
    await expect(argsSection).toBeVisible({ timeout: 5000 });
    
    const messageInput = page.locator('input[id*="message"]').first();
    await messageInput.fill('Hello World');
  });

  test('workflow handles server errors gracefully', async ({ page }) => {
    await page.route('**/discover-tools', async route => {
      await route.fulfill({
        status: 500,
        contentType: 'application/json',
        body: JSON.stringify({ error: 'Internal Server Error' }),
      });
    });
    await MockServerHelper.interceptTestConnection(page, MockServerHelper.getMockTools());

    const wizard = new RunWizardPage(page);
    const toolSelector = new ToolSelectorPage(page);
    
    await wizard.goto();
    await wizard.setTargetUrl('http://localhost:3000');
    await wizard.testConnection();
    await wizard.clickNext();
    await wizard.waitForStep('Stages');
    await wizard.clickNext();
    await wizard.waitForStep('Workload');
    
    const opSelect = page.locator('.operation-select').first();
    await opSelect.selectOption('tools/call');
    await page.locator('button:has-text("Browse")').first().click();
    
    await toolSelector.fetchToolsButton.click();
    
    await expect(toolSelector.errorMessage).toBeVisible({ timeout: 5000 });
    
    const retryBtn = toolSelector.errorMessage.locator('button:has-text("Retry")');
    await expect(retryBtn).toBeVisible();
  });

  test('workflow preserves state across step navigation', async ({ page }) => {
    await MockServerHelper.interceptToolsList(page, MockServerHelper.getMockTools());
    await MockServerHelper.interceptTestConnection(page, MockServerHelper.getMockTools());
    
    const wizard = new RunWizardPage(page);
    
    await wizard.goto();
    
    const testUrl = 'http://localhost:3000';
    await wizard.setTargetUrl(testUrl);
    await wizard.testConnection();
    await wizard.clickNext();
    
    await wizard.waitForStep('Stages');
    await wizard.clickNext();
    
    await wizard.waitForStep('Workload');
    
    const opSelect = page.locator('.operation-select').first();
    await opSelect.selectOption('tools/call');
    
    await wizard.clickBack();
    await wizard.waitForStep('Stages');
    
    await wizard.clickNext();
    await wizard.waitForStep('Workload');
    
    await expect(opSelect).toHaveValue('tools/call');
  });

  test('visual regression: wizard workload step', async ({ page }) => {
    await MockServerHelper.interceptToolsList(page, MockServerHelper.getMockTools());
    await MockServerHelper.interceptTestConnection(page, MockServerHelper.getMockTools());
    
    const wizard = new RunWizardPage(page);
    
    await wizard.goto();
    await wizard.setTargetUrl('http://localhost:3000');
    await wizard.testConnection();
    await wizard.clickNext();
    await wizard.waitForStep('Stages');
    await wizard.clickNext();
    await wizard.waitForStep('Workload');
    
    await page.waitForTimeout(500);
    await expect(page.locator('.wizard-step').first()).toHaveScreenshot('workload-step.png');
  });

  test('selects tool and configures arguments', async ({ page }) => {
    await MockServerHelper.interceptToolsList(page, MockServerHelper.getMockTools());
    await MockServerHelper.interceptTestConnection(page, MockServerHelper.getMockTools());
    
    const wizard = new RunWizardPage(page);
    
    await wizard.goto();
    await wizard.setTargetUrl('http://localhost:3000');
    await wizard.testConnection();
    await wizard.clickNext();
    await wizard.waitForStep('Stages');
    await wizard.clickNext();
    await wizard.waitForStep('Workload');
    
    const opSelect = page.locator('.operation-select').first();
    await opSelect.selectOption('tools/call');
    
    await page.locator('button:has-text("Browse")').first().click();
    
    const toolSelector = new ToolSelectorPage(page);
    await toolSelector.fetchTools();
    await toolSelector.selectTool('add_numbers');
    
    const argsSection = page.locator('.tool-arguments-section');
    await expect(argsSection).toBeVisible({ timeout: 5000 });
    
    const aInput = page.locator('input[type="number"]').first();
    const bInput = page.locator('input[type="number"]').nth(1);
    await aInput.fill('5');
    await bInput.fill('10');
    
    await page.waitForTimeout(300);
    
    const toolNameInput = page.locator('input[id*="tool-name"]').first();
    const toolValue = await toolNameInput.inputValue();
    expect(toolValue).toBe('add_numbers');
  });
});
