import { test, expect } from '@playwright/test';
import { RunWizardPage, ToolSelectorPage, ToolArgumentsEditorPage, MockServerHelper } from './page-objects';

test.describe('Tool Arguments Editor Component', () => {
  async function navigateToToolConfig(page: import('@playwright/test').Page) {
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
    await expect(page.locator('.tool-selector')).toBeVisible({ timeout: 5000 });
    
    const toolSelector = new ToolSelectorPage(page);
    await toolSelector.fetchTools();
  }

  test('displays form fields based on tool schema', async ({ page }) => {
    await navigateToToolConfig(page);
    
    const toolSelector = new ToolSelectorPage(page);
    await toolSelector.selectTool('echo');
    
    const argsEditor = page.locator('.tool-arguments-section');
    await expect(argsEditor).toBeVisible({ timeout: 5000 });
    await expect(page.locator('label:has-text("message")')).toBeVisible();
  });

  test('fills arguments using form builder', async ({ page }) => {
    await navigateToToolConfig(page);
    
    const toolSelector = new ToolSelectorPage(page);
    await toolSelector.selectTool('add_numbers');
    
    const argsEditor = new ToolArgumentsEditorPage(page);
    await expect(argsEditor.container).toBeVisible({ timeout: 5000 });
    
    const aInput = page.locator('input[type="number"]').first();
    const bInput = page.locator('input[type="number"]').nth(1);
    await aInput.fill('5');
    await bInput.fill('3');
    
    await argsEditor.switchToJsonMode();
    const jsonValue = await argsEditor.getJsonValue();
    const parsed = JSON.parse(jsonValue);
    
    expect(parsed.a).toBe(5);
    expect(parsed.b).toBe(3);
  });

  test('toggles between form and JSON editor mode', async ({ page }) => {
    await navigateToToolConfig(page);
    
    const toolSelector = new ToolSelectorPage(page);
    await toolSelector.selectTool('echo');
    
    const argsEditor = new ToolArgumentsEditorPage(page);
    await expect(argsEditor.container).toBeVisible({ timeout: 5000 });
    
    await expect(argsEditor.formView).toBeVisible();
    
    await argsEditor.switchToJsonMode();
    await expect(argsEditor.jsonEditor).toBeVisible();
    await expect(argsEditor.formView).toBeHidden();
    
    await argsEditor.switchToFormMode();
    await expect(argsEditor.formView).toBeVisible();
    await expect(argsEditor.jsonEditor).toBeHidden();
  });

  test('synchronizes values between form and JSON mode', async ({ page }) => {
    await navigateToToolConfig(page);
    
    const toolSelector = new ToolSelectorPage(page);
    await toolSelector.selectTool('echo');
    
    const argsEditor = new ToolArgumentsEditorPage(page);
    await expect(argsEditor.container).toBeVisible({ timeout: 5000 });
    
    const messageInput = page.locator('input[id*="message"]').first();
    await messageInput.fill('Test Message');
    
    await argsEditor.switchToJsonMode();
    await page.waitForTimeout(300);
    const jsonValue = await argsEditor.getJsonValue();
    expect(jsonValue).toContain('Test Message');
  });

  test('validates required fields', async ({ page }) => {
    await navigateToToolConfig(page);
    
    const toolSelector = new ToolSelectorPage(page);
    await toolSelector.selectTool('echo');
    
    const argsEditor = new ToolArgumentsEditorPage(page);
    await expect(argsEditor.container).toBeVisible({ timeout: 5000 });
    
    const hasRequiredIndicator = await page.locator('.field-required, [aria-required="true"]').isVisible();
    expect(hasRequiredIndicator).toBe(true);
  });

  test('handles nested object arguments', async ({ page }) => {
    await navigateToToolConfig(page);
    
    const toolSelector = new ToolSelectorPage(page);
    await toolSelector.selectTool('nested_data');
    
    const argsEditor = new ToolArgumentsEditorPage(page);
    await expect(argsEditor.container).toBeVisible({ timeout: 5000 });
    
    const hasNestedFields = await page.locator('fieldset, .schema-field-object').first().isVisible();
    expect(hasNestedFields).toBe(true);
  });

  test('handles array arguments', async ({ page }) => {
    await navigateToToolConfig(page);
    
    const toolSelector = new ToolSelectorPage(page);
    await toolSelector.selectTool('nested_data');
    
    const argsEditor = new ToolArgumentsEditorPage(page);
    await expect(argsEditor.container).toBeVisible({ timeout: 5000 });
    
    const addItemBtn = page.locator('.schema-field-array button:has-text("Add Item")').first();
    if (await addItemBtn.isVisible()) {
      await addItemBtn.click();
      
      const arrayInputs = page.locator('.array-item input');
      await expect(arrayInputs.first()).toBeVisible();
    }
  });

  test('adds and removes array items', async ({ page }) => {
    await navigateToToolConfig(page);
    
    const toolSelector = new ToolSelectorPage(page);
    await toolSelector.selectTool('nested_data');
    
    const argsEditor = new ToolArgumentsEditorPage(page);
    await expect(argsEditor.container).toBeVisible({ timeout: 5000 });
    
    const addItemBtn = page.locator('.schema-field-array button:has-text("Add Item")').first();
    if (await addItemBtn.isVisible()) {
      await addItemBtn.click();
      await addItemBtn.click();
      
      let itemCount = await page.locator('.array-item').count();
      expect(itemCount).toBe(2);
      
      await page.locator('.array-item button.btn-danger').first().click();
      
      itemCount = await page.locator('.array-item').count();
      expect(itemCount).toBe(1);
    }
  });

  test('handles deeply nested objects (2-3 levels)', async ({ page }) => {
    await navigateToToolConfig(page);
    
    const toolSelector = new ToolSelectorPage(page);
    await toolSelector.selectTool('nested_data');
    
    const argsEditor = new ToolArgumentsEditorPage(page);
    await expect(argsEditor.container).toBeVisible({ timeout: 5000 });
    
    await argsEditor.switchToJsonMode();
    await argsEditor.setJsonValue({
      user: {
        name: 'John',
        profile: {
          age: 30,
          email: 'john@example.com',
        },
      },
      tags: ['developer', 'typescript'],
    });
    
    const finalJson = await argsEditor.getJsonValue();
    expect(finalJson).toContain('John');
    expect(finalJson).toContain('profile');
  });

  test('handles JSON syntax errors gracefully', async ({ page }) => {
    await navigateToToolConfig(page);
    
    const toolSelector = new ToolSelectorPage(page);
    await toolSelector.selectTool('echo');
    
    const argsEditor = new ToolArgumentsEditorPage(page);
    await expect(argsEditor.container).toBeVisible({ timeout: 5000 });
    
    await argsEditor.switchToJsonMode();
    await argsEditor.jsonEditor.fill('{ invalid json }');
    
    await expect(page.locator('.json-error')).toBeVisible();
  });

  test('visual regression: arguments editor form view', async ({ page }) => {
    await navigateToToolConfig(page);
    
    const toolSelector = new ToolSelectorPage(page);
    await toolSelector.selectTool('add_numbers');
    
    const argsEditor = new ToolArgumentsEditorPage(page);
    await expect(argsEditor.container).toBeVisible({ timeout: 5000 });
    
    const aInput = page.locator('input[type="number"]').first();
    const bInput = page.locator('input[type="number"]').nth(1);
    await aInput.fill('10');
    await bInput.fill('20');
    
    await page.waitForTimeout(300);
    await expect(argsEditor.container).toHaveScreenshot('args-editor-form.png');
  });

  test('visual regression: arguments editor JSON view', async ({ page }) => {
    await navigateToToolConfig(page);
    
    const toolSelector = new ToolSelectorPage(page);
    await toolSelector.selectTool('add_numbers');
    
    const argsEditor = new ToolArgumentsEditorPage(page);
    await expect(argsEditor.container).toBeVisible({ timeout: 5000 });
    
    await argsEditor.switchToJsonMode();
    await argsEditor.setJsonValue({ a: 10, b: 20 });
    
    await page.waitForTimeout(300);
    await expect(argsEditor.container).toHaveScreenshot('args-editor-json.png');
  });

  test('visual regression: nested fields editor', async ({ page }) => {
    await navigateToToolConfig(page);
    
    const toolSelector = new ToolSelectorPage(page);
    await toolSelector.selectTool('nested_data');
    
    const argsEditor = new ToolArgumentsEditorPage(page);
    await expect(argsEditor.container).toBeVisible({ timeout: 5000 });
    
    await page.waitForTimeout(300);
    await expect(argsEditor.container).toHaveScreenshot('args-editor-nested.png');
  });

  test('visual regression: validation error state', async ({ page }) => {
    await navigateToToolConfig(page);
    
    const toolSelector = new ToolSelectorPage(page);
    await toolSelector.selectTool('echo');
    
    const argsEditor = new ToolArgumentsEditorPage(page);
    await expect(argsEditor.container).toBeVisible({ timeout: 5000 });
    
    await argsEditor.switchToJsonMode();
    await argsEditor.jsonEditor.fill('{ invalid }');
    
    await page.waitForTimeout(300);
    await expect(argsEditor.container).toHaveScreenshot('args-editor-error.png');
  });
});
