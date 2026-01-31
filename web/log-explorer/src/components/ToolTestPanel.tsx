import { createContext, useCallback, useContext, useMemo, useState } from 'react';
import type { ToolTestResult } from '../api/index';
import { testTool } from '../api/index';
import { Icon } from './Icon';

interface ToolTestPanelProps {
  targetUrl?: string;
  toolName: string;
  value: Record<string, unknown>;
  headers?: Record<string, string>;
  isValid: boolean;
  children: React.ReactNode;
}

interface ToolTestPanelContextValue {
  targetUrl?: string;
  toolName: string;
  headers?: Record<string, string>;
  isValid: boolean;
  isTesting: boolean;
  testResult: ToolTestResult | null;
  handleTestTool: () => void;
  dismissResult: () => void;
}

const ToolTestPanelContext = createContext<ToolTestPanelContextValue | null>(null);

function useToolTestPanelContext() {
  return useContext(ToolTestPanelContext);
}

export function ToolTestPanel({
  targetUrl,
  toolName,
  value,
  headers,
  isValid,
  children,
}: ToolTestPanelProps) {
  const [testResult, setTestResult] = useState<ToolTestResult | null>(null);
  const [isTesting, setIsTesting] = useState(false);

  const handleTestTool = useCallback(async () => {
    if (!targetUrl || !toolName) return;

    setIsTesting(true);
    setTestResult(null);

    try {
      const result = await testTool(targetUrl, toolName, value, headers);
      setTestResult(result);
    } catch (err) {
      setTestResult({
        success: false,
        error: err instanceof Error ? err.message : 'Test failed',
        latency_ms: 0,
      });
    } finally {
      setIsTesting(false);
    }
  }, [targetUrl, toolName, value, headers]);

  const dismissResult = useCallback(() => setTestResult(null), []);

  const contextValue = useMemo(() => ({
    targetUrl,
    toolName,
    headers,
    isValid,
    isTesting,
    testResult,
    handleTestTool,
    dismissResult,
  }), [
    targetUrl,
    toolName,
    headers,
    isValid,
    isTesting,
    testResult,
    handleTestTool,
    dismissResult,
  ]);

  return (
    <ToolTestPanelContext.Provider value={contextValue}>
      {children}
    </ToolTestPanelContext.Provider>
  );
}

export function ToolTestPanelButton() {
  const context = useToolTestPanelContext();

  if (!context || !context.targetUrl) return null;

  const { handleTestTool, isTesting, isValid, headers } = context;
  const hasHeaders = headers && Object.keys(headers).length > 0;

  return (
    <button
      type="button"
      onClick={handleTestTool}
      disabled={isTesting || !isValid}
      className="btn btn-secondary btn-sm test-tool-btn"
      aria-label={`Test tool with current arguments${hasHeaders ? ' (with custom headers)' : ''}`}
      title={hasHeaders ? `Using ${Object.keys(headers).length} custom header(s)` : undefined}
    >
      {isTesting ? (
        <>
          <Icon name="loader" size="sm" aria-hidden={true} />
          Testing...
        </>
      ) : (
        <>
          <Icon name="play" size="sm" aria-hidden={true} />
          Test Tool
          {hasHeaders && (
            <span className="header-indicator" aria-hidden={true}>+H</span>
          )}
        </>
      )}
    </button>
  );
}

export function ToolTestPanelResult() {
  const context = useToolTestPanelContext();

  if (!context || !context.testResult) return null;

  const { testResult, dismissResult } = context;

  return (
    <div className={`test-result ${testResult.success ? 'success' : 'error'}`} role="status">
      <div className="test-result-header">
        <Icon
          name={testResult.success ? 'check-circle' : 'x-circle'}
          size="sm"
          aria-hidden={true}
        />
        <span className="test-result-status">
          {testResult.success ? 'Success' : 'Failed'}
        </span>
        <span className="test-result-latency">{testResult.latency_ms}ms</span>
        <button
          type="button"
          onClick={dismissResult}
          className="btn btn-ghost btn-xs"
          aria-label="Dismiss test result"
        >
          <Icon name="x" size="xs" aria-hidden={true} />
        </button>
      </div>
      {testResult.error && (
        <div className="test-result-error">{testResult.error}</div>
      )}
      {testResult.success && testResult.result !== undefined && (
        <div className="test-result-output">
          <pre>{JSON.stringify(testResult.result, null, 2)}</pre>
        </div>
      )}
    </div>
  );
}
