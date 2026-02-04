import { Page, Locator, expect } from '@playwright/test';

export class RunWizardPage {
  readonly page: Page;
  readonly targetUrlInput: Locator;
  readonly testConnectionButton: Locator;
  readonly nextButton: Locator;
  readonly backButton: Locator;
  readonly stepIndicators: Locator;
  readonly newRunButton: Locator;

  constructor(page: Page) {
    this.page = page;
    this.targetUrlInput = page.locator('#target-url');
    this.testConnectionButton = page.locator('.test-connection-btn');
    this.nextButton = page.locator('button:has-text("Next")');
    this.backButton = page.locator('button:has-text("Back")');
    this.stepIndicators = page.locator('.wizard-stepper button');
    this.newRunButton = page.locator('button:has-text("New Run")');
  }

  async goto() {
    await this.page.goto('/');
    await this.page.waitForLoadState('networkidle');
    await this.newRunButton.click();
    await expect(this.page.locator('h2:has-text("Configure Target")')).toBeVisible({ timeout: 10000 });
  }

  async setTargetUrl(url: string) {
    await this.targetUrlInput.fill(url);
  }

  async testConnection() {
    await this.testConnectionButton.click();
    await expect(this.page.locator('.connection-result.success')).toBeVisible({ timeout: 10000 });
  }

  async goToStep(stepName: 'Target' | 'Stages' | 'Workload' | 'Review') {
    await this.page.locator(`.wizard-stepper button:has-text("${stepName}")`).click();
  }

  async clickNext() {
    await this.nextButton.click();
  }

  async clickBack() {
    await this.backButton.click();
  }

  async waitForStep(stepName: string) {
    await expect(this.page.locator(`h2:has-text("${stepName}")`).first()).toBeVisible();
  }
}

export class ToolSelectorPage {
  readonly page: Page;
  readonly container: Locator;
  readonly fetchToolsButton: Locator;
  readonly searchInput: Locator;
  readonly toolList: Locator;
  readonly toolItems: Locator;
  readonly loadingIndicator: Locator;
  readonly errorMessage: Locator;
  readonly toolCount: Locator;

  constructor(page: Page) {
    this.page = page;
    this.container = page.locator('.tool-selector');
    this.fetchToolsButton = page.locator('button:has-text("Fetch Tools")');
    this.searchInput = page.locator('.tool-search input');
    this.toolList = page.locator('.tool-list');
    this.toolItems = page.locator('.tool-item');
    this.loadingIndicator = page.locator('button:has-text("Fetching...")');
    this.errorMessage = page.locator('.tool-selector-error');
    this.toolCount = page.locator('.tool-count');
  }

  async fetchTools() {
    await this.fetchToolsButton.click();
    await this.page.waitForResponse(resp => resp.url().includes('/discover-tools'), { timeout: 10000 }).catch(() => {});
    await expect(this.toolList.or(this.errorMessage)).toBeVisible({ timeout: 10000 });
  }

  async searchTools(query: string) {
    await this.searchInput.fill(query);
  }

  async selectTool(toolName: string) {
    const toolItem = this.page.locator(`.tool-item:has(.tool-name:text("${toolName}"))`);
    await toolItem.click();
    await expect(toolItem).toHaveClass(/selected/);
  }

  async expandToolSchema(toolName: string) {
    const expandBtn = this.page.locator(`.tool-item:has(.tool-name:text("${toolName}")) .tool-expand-btn`);
    await expandBtn.click();
    await expect(this.page.locator(`.tool-item:has(.tool-name:text("${toolName}")) .tool-schema-panel`)).toBeVisible();
  }

  async getToolNames(): Promise<string[]> {
    await this.toolItems.first().waitFor({ timeout: 5000 }).catch(() => {});
    return await this.page.locator('.tool-name').allInnerTexts();
  }

  async getToolCount(): Promise<number> {
    const text = await this.toolCount.textContent();
    const match = text?.match(/(\d+)/);
    return match ? parseInt(match[1]) : 0;
  }

  async copyToolName(toolName: string) {
    const copyBtn = this.page.locator(`.tool-item:has(.tool-name:text("${toolName}")) .tool-copy-btn`);
    await copyBtn.click();
  }
}

export class ToolArgumentsEditorPage {
  readonly page: Page;
  readonly container: Locator;
  readonly formView: Locator;
  readonly jsonEditor: Locator;
  readonly formModeButton: Locator;
  readonly jsonModeButton: Locator;
  readonly validationBadge: Locator;
  readonly validationSummary: Locator;
  readonly exportButton: Locator;
  readonly importButton: Locator;
  readonly savePresetButton: Locator;

  constructor(page: Page) {
    this.page = page;
    this.container = page.locator('.tool-args-editor');
    this.formView = page.locator('.schema-form');
    this.jsonEditor = page.locator('.json-editor');
    this.formModeButton = page.locator('button[role="tab"]:has-text("Form")');
    this.jsonModeButton = page.locator('button[role="tab"]:has-text("JSON")');
    this.validationBadge = page.locator('.validation-badge');
    this.validationSummary = page.locator('.validation-summary');
    this.exportButton = page.locator('button[aria-label*="Export"]');
    this.importButton = page.locator('button[aria-label*="Import"]');
    this.savePresetButton = page.locator('button:has-text("Save Preset"), button:has-text("Save as Preset")');
  }

  async switchToJsonMode() {
    await this.jsonModeButton.click();
    await expect(this.jsonEditor).toBeVisible();
  }

  async switchToFormMode() {
    await this.formModeButton.click();
    await expect(this.formView).toBeVisible();
  }

  async fillField(fieldName: string, value: string) {
    const field = this.page.locator(`[id^="field-"][id$="${fieldName}"], label:has-text("${fieldName}") + input, label:has-text("${fieldName}") + textarea`).first();
    await field.fill(value);
  }

  async fillNumberField(fieldName: string, value: number) {
    const field = this.page.locator(`input[type="number"][id*="${fieldName}"]`).first();
    await field.fill(value.toString());
  }

  async toggleCheckbox(fieldName: string) {
    const checkbox = this.page.locator(`input[type="checkbox"][id*="${fieldName}"]`).first();
    await checkbox.click();
  }

  async setJsonValue(json: object) {
    await this.switchToJsonMode();
    await this.jsonEditor.fill(JSON.stringify(json, null, 2));
  }

  async getJsonValue(): Promise<string> {
    return await this.jsonEditor.inputValue();
  }

  async addArrayItem(fieldName: string) {
    const addBtn = this.page.locator(`.schema-field-array:has(label:has-text("${fieldName}")) button:has-text("Add Item")`);
    await addBtn.click();
  }

  async hasValidationError(): Promise<boolean> {
    const badge = this.validationBadge;
    return await badge.locator('.invalid').isVisible().catch(() => false);
  }

  async saveAsPreset(presetName: string) {
    await this.savePresetButton.click();
    await this.page.locator('.preset-dialog input').fill(presetName);
    await this.page.locator('.preset-dialog button:has-text("Save")').click();
  }

  async loadPreset(presetName: string) {
    await this.page.locator(`.preset-load-btn:has-text("${presetName}")`).click();
  }
}

export class ToolResultsPanelPage {
  readonly page: Page;
  readonly container: Locator;
  readonly resultsList: Locator;
  readonly resultCards: Locator;
  readonly listViewButton: Locator;
  readonly groupedViewButton: Locator;
  readonly toolFilter: Locator;
  readonly statusFilter: Locator;
  readonly exportJsonButton: Locator;
  readonly exportCsvButton: Locator;
  readonly refreshButton: Locator;
  readonly summaryStats: Locator;
  readonly loadingIndicator: Locator;
  readonly emptyState: Locator;

  constructor(page: Page) {
    this.page = page;
    this.container = page.locator('.tool-results-panel');
    this.resultsList = page.locator('.results-list');
    this.resultCards = page.locator('.result-card');
    this.listViewButton = page.locator('button[role="tab"]:has-text("List")');
    this.groupedViewButton = page.locator('button[role="tab"]:has-text("Grouped")');
    this.toolFilter = page.locator('#filter-tool');
    this.statusFilter = page.locator('#filter-status');
    this.exportJsonButton = page.locator('button[aria-label*="JSON"]');
    this.exportCsvButton = page.locator('button[aria-label*="CSV"]');
    this.refreshButton = page.locator('button[aria-label="Refresh results"]');
    this.summaryStats = page.locator('.results-summary');
    this.loadingIndicator = page.locator('.results-loading');
    this.emptyState = page.locator('.results-empty');
  }

  async switchToListView() {
    await this.listViewButton.click();
  }

  async switchToGroupedView() {
    await this.groupedViewButton.click();
  }

  async filterByTool(toolName: string) {
    await this.toolFilter.selectOption(toolName);
  }

  async filterByStatus(status: 'all' | 'success' | 'error') {
    const value = status === 'all' ? '' : status;
    await this.statusFilter.selectOption(value);
  }

  async expandResultCard(index: number) {
    await this.resultCards.nth(index).locator('.result-card-header').click();
  }

  async getResultCount(): Promise<number> {
    const countText = await this.page.locator('.results-count').textContent();
    const match = countText?.match(/(\d+)/);
    return match ? parseInt(match[1]) : 0;
  }

  async getSummaryStats(): Promise<{ total: number; success: number; errors: number }> {
    const total = await this.summaryStats.locator('.summary-stat:first-child .summary-value').textContent();
    const success = await this.summaryStats.locator('.stat-success .summary-value').textContent();
    const errors = await this.summaryStats.locator('.stat-error .summary-value').textContent();
    return {
      total: parseInt(total || '0'),
      success: parseInt(success || '0'),
      errors: parseInt(errors || '0'),
    };
  }

  async hasSuccessBadge(index: number): Promise<boolean> {
    return await this.resultCards.nth(index).locator('.badge-success').isVisible();
  }

  async hasErrorBadge(index: number): Promise<boolean> {
    return await this.resultCards.nth(index).locator('.badge-error').isVisible();
  }
}

export class ToolMetricsDashboardPage {
  readonly page: Page;
  readonly container: Locator;
  readonly pieChart: Locator;
  readonly barChart: Locator;
  readonly lineChart: Locator;
  readonly scatterChart: Locator;
  readonly metricsTable: Locator;
  readonly refreshButton: Locator;
  readonly loadingIndicator: Locator;
  readonly emptyState: Locator;
  readonly errorState: Locator;
  readonly toolCount: Locator;

  constructor(page: Page) {
    this.page = page;
    this.container = page.locator('.tool-metrics-dashboard');
    this.pieChart = page.locator('.chart-pie');
    this.barChart = page.locator('.chart-bar');
    this.lineChart = page.locator('.chart-line');
    this.scatterChart = page.locator('.chart-scatter');
    this.metricsTable = page.locator('.tool-metrics-table');
    this.refreshButton = page.locator('.dashboard-header button:has-text("Refresh")');
    this.loadingIndicator = page.locator('.dashboard-loading');
    this.emptyState = page.locator('.dashboard-empty');
    this.errorState = page.locator('.dashboard-error');
    this.toolCount = page.locator('.tool-count');
  }

  async waitForChartsToLoad() {
    await expect(this.loadingIndicator).toBeHidden({ timeout: 15000 });
    await expect(this.pieChart.or(this.emptyState).or(this.errorState)).toBeVisible();
  }

  async exportChart(chartType: 'pie' | 'bar' | 'line' | 'scatter') {
    const chartMap = {
      pie: this.pieChart,
      bar: this.barChart,
      line: this.lineChart,
      scatter: this.scatterChart,
    };
    const exportBtn = chartMap[chartType].locator('button[aria-label*="Export"]');
    await exportBtn.click();
  }

  async getToolCountFromDashboard(): Promise<number> {
    const text = await this.toolCount.textContent();
    const match = text?.match(/(\d+)/);
    return match ? parseInt(match[1]) : 0;
  }

  async clickToolInTable(toolName: string) {
    await this.metricsTable.locator(`tr:has-text("${toolName}")`).click();
  }

  async getTableRowCount(): Promise<number> {
    return await this.metricsTable.locator('tbody tr').count();
  }

  async getToolMetricsFromTable(toolName: string): Promise<{
    totalCalls: number;
    success: number;
    errors: number;
    successRate: string;
    p50: string;
    p95: string;
    p99: string;
  }> {
    const row = this.metricsTable.locator(`tr:has-text("${toolName}")`);
    const cells = await row.locator('td').allTextContents();
    return {
      totalCalls: parseInt(cells[1]?.replace(/,/g, '') || '0'),
      success: parseInt(cells[2]?.replace(/,/g, '') || '0'),
      errors: parseInt(cells[3]?.replace(/,/g, '') || '0'),
      successRate: cells[4] || '0%',
      p50: cells[5] || '0ms',
      p95: cells[6] || '0ms',
      p99: cells[7] || '0ms',
    };
  }
}

export class MockServerHelper {
  static async interceptToolsList(page: Page, tools: Array<{ name: string; description: string; inputSchema?: object }>) {
    // UI uses the control plane discovery endpoint, not the MCP JSON-RPC method directly.
    await page.route('**/discover-tools', async route => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ tools }),
      });
    });
  }

  static async interceptTestConnection(
    page: Page,
    tools: Array<{ name: string; description: string; inputSchema?: object }> = MockServerHelper.getMockTools()
  ) {
    await page.route('**/test-connection', async route => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          success: true,
          tool_count: tools.length,
          connect_latency_ms: 10,
          tools_latency_ms: 20,
          total_latency_ms: 30,
        }),
      });
    });
  }

  static async interceptRunLogs(page: Page, logs: Array<{
    timestamp_ms: number;
    operation: string;
    tool_name: string;
    latency_ms: number;
    ok: boolean;
    worker_id: string;
    vu_id: string;
    session_id: string;
    error_type?: string;
    error_code?: string;
  }>) {
    await page.route('**/runs/*/logs*', async route => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ logs }),
      });
    });
  }

  static async interceptRunMetrics(page: Page, byTool: Record<string, {
    total_calls: number;
    success_count: number;
    error_count: number;
    latency_p50_ms: number;
    latency_p95_ms: number;
    latency_p99_ms: number;
  }>) {
    await page.route('**/runs/*/metrics', async route => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          run_id: 'run_test123',
          by_tool: byTool,
          timestamp: Date.now(),
        }),
      });
    });
  }

  static getMockTools() {
    return [
      {
        name: 'echo',
        description: 'Returns the input message',
        inputSchema: {
          type: 'object',
          properties: {
            message: { type: 'string', description: 'Message to echo' },
          },
          required: ['message'],
        },
      },
      {
        name: 'add_numbers',
        description: 'Adds two numbers together',
        inputSchema: {
          type: 'object',
          properties: {
            a: { type: 'number', description: 'First number' },
            b: { type: 'number', description: 'Second number' },
          },
          required: ['a', 'b'],
        },
      },
      {
        name: 'error_tool',
        description: 'Always returns an error',
        inputSchema: {
          type: 'object',
          properties: {
            error_message: { type: 'string' },
          },
        },
      },
      {
        name: 'nested_data',
        description: 'Accepts nested data structures',
        inputSchema: {
          type: 'object',
          properties: {
            user: {
              type: 'object',
              properties: {
                name: { type: 'string' },
                profile: {
                  type: 'object',
                  properties: {
                    age: { type: 'number' },
                    email: { type: 'string', format: 'email' },
                  },
                },
              },
            },
            tags: { type: 'array', items: { type: 'string' } },
          },
        },
      },
      {
        name: 'slow_tool',
        description: 'Simulates slow response',
        inputSchema: {
          type: 'object',
          properties: {
            delay_ms: { type: 'integer', minimum: 0, maximum: 5000 },
          },
        },
      },
    ];
  }

  static getMockLogs() {
    const baseTime = Date.now();
    return [
      { timestamp_ms: baseTime, operation: 'tools/call', tool_name: 'echo', latency_ms: 45, ok: true, worker_id: 'w1', vu_id: 'vu1', session_id: 's1' },
      { timestamp_ms: baseTime + 100, operation: 'tools/call', tool_name: 'echo', latency_ms: 38, ok: true, worker_id: 'w1', vu_id: 'vu2', session_id: 's2' },
      { timestamp_ms: baseTime + 200, operation: 'tools/call', tool_name: 'add_numbers', latency_ms: 52, ok: true, worker_id: 'w1', vu_id: 'vu1', session_id: 's1' },
      { timestamp_ms: baseTime + 300, operation: 'tools/call', tool_name: 'error_tool', latency_ms: 25, ok: false, worker_id: 'w1', vu_id: 'vu3', session_id: 's3', error_type: 'tool_error', error_code: 'ERR_FORCED' },
      { timestamp_ms: baseTime + 400, operation: 'tools/call', tool_name: 'nested_data', latency_ms: 78, ok: true, worker_id: 'w2', vu_id: 'vu4', session_id: 's4' },
      { timestamp_ms: baseTime + 500, operation: 'tools/call', tool_name: 'slow_tool', latency_ms: 1250, ok: true, worker_id: 'w2', vu_id: 'vu5', session_id: 's5' },
    ];
  }

  static getMockMetrics() {
    return {
      echo: { total_calls: 100, success_count: 98, error_count: 2, latency_p50_ms: 42, latency_p95_ms: 85, latency_p99_ms: 120 },
      add_numbers: { total_calls: 50, success_count: 50, error_count: 0, latency_p50_ms: 35, latency_p95_ms: 65, latency_p99_ms: 90 },
      error_tool: { total_calls: 20, success_count: 0, error_count: 20, latency_p50_ms: 15, latency_p95_ms: 25, latency_p99_ms: 30 },
      nested_data: { total_calls: 30, success_count: 28, error_count: 2, latency_p50_ms: 75, latency_p95_ms: 150, latency_p99_ms: 200 },
      slow_tool: { total_calls: 10, success_count: 10, error_count: 0, latency_p50_ms: 1200, latency_p95_ms: 1500, latency_p99_ms: 1800 },
    };
  }
}
