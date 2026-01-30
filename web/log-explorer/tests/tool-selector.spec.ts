import { test, expect } from '@playwright/test';
import { RunWizardPage, ToolSelectorPage, MockServerHelper } from './page-objects';

test.describe('Tool Selector Component', () => {
  async function navigateToToolSelector(page: import('@playwright/test').Page) {
    await MockServerHelper.interceptToolsList(page, MockServerHelper.getMockTools());
    
    const wizard = new RunWizardPage(page);
    await wizard.goto();
    await wizard.setTargetUrl('http://localhost:3000');
    await wizard.clickNext();
    await wizard.waitForStep('Stages');
    await wizard.clickNext();
    await wizard.waitForStep('Workload');
    
    const opSelect = page.locator('.operation-select').first();
    await opSelect.selectOption('tools/call');
    
    await page.locator('button:has-text("Browse")').first().click();
    await expect(page.locator('.tool-selector')).toBeVisible({ timeout: 5000 });
  }

  test('fetches and displays tools from target server', async ({ page }) => {
    await navigateToToolSelector(page);
    
    const toolSelector = new ToolSelectorPage(page);
    await toolSelector.fetchTools();
    
    const toolCount = await toolSelector.getToolCount();
    expect(toolCount).toBe(5);
    
    const toolNames = await toolSelector.getToolNames();
    expect(toolNames).toContain('echo');
    expect(toolNames).toContain('add_numbers');
    expect(toolNames).toContain('error_tool');
  });

  test('filters tools by search query', async ({ page }) => {
    await navigateToToolSelector(page);
    
    const toolSelector = new ToolSelectorPage(page);
    await toolSelector.fetchTools();
    
    await toolSelector.searchTools('echo');
    
    const toolNames = await toolSelector.getToolNames();
    expect(toolNames.length).toBe(1);
    expect(toolNames).toContain('echo');
    
    await toolSelector.searchTools('');
    const allToolNames = await toolSelector.getToolNames();
    expect(allToolNames.length).toBe(5);
  });

  test('filters tools by description', async ({ page }) => {
    await navigateToToolSelector(page);
    
    const toolSelector = new ToolSelectorPage(page);
    await toolSelector.fetchTools();
    
    await toolSelector.searchTools('numbers');
    
    const toolNames = await toolSelector.getToolNames();
    expect(toolNames).toContain('add_numbers');
  });

  test('shows no results message for unmatched search', async ({ page }) => {
    await navigateToToolSelector(page);
    
    const toolSelector = new ToolSelectorPage(page);
    await toolSelector.fetchTools();
    
    await toolSelector.searchTools('nonexistent_tool_xyz');
    
    await expect(page.locator('.tool-list-empty')).toBeVisible();
    await expect(page.locator('text=No tools match')).toBeVisible();
  });

  test('displays tool metadata correctly', async ({ page }) => {
    await navigateToToolSelector(page);
    
    const toolSelector = new ToolSelectorPage(page);
    await toolSelector.fetchTools();
    
    const echoItem = page.locator('.tool-item:has(.tool-name:text("echo"))');
    
    await expect(echoItem.locator('.tool-name')).toHaveText('echo');
    await expect(echoItem.locator('.tool-description')).toContainText('Returns the input message');
  });

  test('expands tool schema panel', async ({ page }) => {
    await navigateToToolSelector(page);
    
    const toolSelector = new ToolSelectorPage(page);
    await toolSelector.fetchTools();
    
    await toolSelector.expandToolSchema('echo');
    
    const schemaPanel = page.locator('.tool-item:has(.tool-name:text("echo")) .tool-schema-panel');
    await expect(schemaPanel).toBeVisible();
    await expect(schemaPanel.locator('text=Input Schema')).toBeVisible();
  });

  test('selects tool and shows validation indicator', async ({ page }) => {
    await navigateToToolSelector(page);
    
    const toolSelector = new ToolSelectorPage(page);
    await toolSelector.fetchTools();
    
    await toolSelector.selectTool('echo');
    
    const selectedItem = page.locator('.tool-item.selected');
    await expect(selectedItem).toBeVisible();
    await expect(selectedItem.locator('.tool-name')).toHaveText('echo');
    
    await expect(page.locator('.tool-validation.valid')).toBeVisible();
  });

  test('copies tool name to clipboard', async ({ page, context }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write']);
    
    await navigateToToolSelector(page);
    
    const toolSelector = new ToolSelectorPage(page);
    await toolSelector.fetchTools();
    
    await toolSelector.copyToolName('echo');
    
    await page.waitForTimeout(500);
    const copyIcon = page.locator('.tool-item:has(.tool-name:text("echo")) .tool-copy-btn');
    await expect(copyIcon).toBeVisible();
  });

  test('handles fetch tools loading state', async ({ page }) => {
    await page.route('**/tools/list', async route => {
      await new Promise(resolve => setTimeout(resolve, 1000));
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ jsonrpc: '2.0', id: 1, result: { tools: MockServerHelper.getMockTools() } }),
      });
    });
    
    const wizard = new RunWizardPage(page);
    await wizard.goto();
    await wizard.setTargetUrl('http://localhost:3000');
    await wizard.clickNext();
    await wizard.waitForStep('Stages');
    await wizard.clickNext();
    await wizard.waitForStep('Workload');
    
    const opSelect = page.locator('.operation-select').first();
    await opSelect.selectOption('tools/call');
    await page.locator('button:has-text("Browse")').first().click();
    
    const toolSelector = new ToolSelectorPage(page);
    toolSelector.fetchToolsButton.click();
    
    await expect(toolSelector.loadingIndicator).toBeVisible();
    await expect(toolSelector.toolList).toBeVisible({ timeout: 5000 });
  });

  test('handles fetch tools error', async ({ page }) => {
    await page.route('**/tools/list', async route => {
      await route.abort('failed');
    });
    
    const wizard = new RunWizardPage(page);
    await wizard.goto();
    await wizard.setTargetUrl('http://localhost:3000');
    await wizard.clickNext();
    await wizard.waitForStep('Stages');
    await wizard.clickNext();
    await wizard.waitForStep('Workload');
    
    const opSelect = page.locator('.operation-select').first();
    await opSelect.selectOption('tools/call');
    await page.locator('button:has-text("Browse")').first().click();
    
    const toolSelector = new ToolSelectorPage(page);
    await toolSelector.fetchToolsButton.click();
    
    await expect(toolSelector.errorMessage).toBeVisible({ timeout: 10000 });
  });

  test('shows empty state when no tools loaded', async ({ page }) => {
    await MockServerHelper.interceptToolsList(page, []);
    
    const wizard = new RunWizardPage(page);
    await wizard.goto();
    await wizard.setTargetUrl('http://localhost:3000');
    await wizard.clickNext();
    await wizard.waitForStep('Stages');
    await wizard.clickNext();
    await wizard.waitForStep('Workload');
    
    const opSelect = page.locator('.operation-select').first();
    await opSelect.selectOption('tools/call');
    await page.locator('button:has-text("Browse")').first().click();
    
    const emptyState = page.locator('.tool-selector-empty');
    await expect(emptyState).toBeVisible();
  });

  test('keyboard navigation works for tool selection', async ({ page }) => {
    await navigateToToolSelector(page);
    
    const toolSelector = new ToolSelectorPage(page);
    await toolSelector.fetchTools();
    
    const firstTool = toolSelector.toolItems.first();
    await firstTool.focus();
    await page.keyboard.press('Enter');
    
    await expect(firstTool).toHaveClass(/selected/);
  });

  test('visual regression: tool selector with tools loaded', async ({ page }) => {
    await navigateToToolSelector(page);
    
    const toolSelector = new ToolSelectorPage(page);
    await toolSelector.fetchTools();
    await page.waitForTimeout(300);
    
    await expect(toolSelector.container).toHaveScreenshot('tool-selector-loaded.png');
  });

  test('visual regression: tool selector with search active', async ({ page }) => {
    await navigateToToolSelector(page);
    
    const toolSelector = new ToolSelectorPage(page);
    await toolSelector.fetchTools();
    await toolSelector.searchTools('echo');
    await page.waitForTimeout(300);
    
    await expect(toolSelector.container).toHaveScreenshot('tool-selector-search.png');
  });

  test('visual regression: tool selector with expanded schema', async ({ page }) => {
    await navigateToToolSelector(page);
    
    const toolSelector = new ToolSelectorPage(page);
    await toolSelector.fetchTools();
    await toolSelector.expandToolSchema('nested_data');
    await page.waitForTimeout(300);
    
    await expect(toolSelector.container).toHaveScreenshot('tool-selector-expanded.png');
  });
});
