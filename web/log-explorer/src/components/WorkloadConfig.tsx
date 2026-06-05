import { useMemo, useCallback, useState, useEffect } from 'react';
import type { WorkloadConfig as WorkloadConfigType, OpMixEntry, FetchedTool, JSONSchema } from '../types';
import { HelpTooltip } from './HelpTooltip';
import { Icon, type IconName } from './Icon';
import { ToolSelector } from './ToolSelector';
import { ToolArgumentsEditor } from './ToolArgumentsEditor';

interface Props {
  config: WorkloadConfigType;
  onChange: (config: WorkloadConfigType) => void;
  targetUrl?: string;
  headers?: Record<string, string>;
  protocolVersion?: string;
  protocolVersionPolicy?: string;
  fetchedTools?: FetchedTool[];
  onToolsFetched?: (tools: FetchedTool[]) => void;
}

interface OperationInfo {
  value: OpMixEntry['operation'];
  label: string;
  description: string;
  icon: IconName;
  help: string;
  disabled?: boolean;
  disabledReason?: string;
}

const OPERATIONS: OperationInfo[] = [
  { 
    value: 'tools/list', 
    label: 'List Tools', 
    description: 'Enumerate available tools', 
    icon: 'wrench',
    help: 'Requests the list of all available tools from the MCP server. Useful for testing discovery performance.',
  },
  { 
    value: 'tools/call', 
    label: 'Call Tool', 
    description: 'Execute a specific tool', 
    icon: 'cog',
    help: 'Invokes a specific tool by name. You\'ll need to specify the tool name to call during the test.',
  },
  { 
    value: 'ping', 
    label: 'Ping', 
    description: 'Heartbeat check', 
    icon: 'activity',
    help: 'Sends a ping request to verify server responsiveness. Useful for connection health monitoring.',
  },
  { 
    value: 'resources/list', 
    label: 'List Resources', 
    description: 'Enumerate available resources', 
    icon: 'folder',
    help: 'Requests the list of all resources exposed by the MCP server.',
  },
  { 
    value: 'resources/read', 
    label: 'Read Resource', 
    description: 'Read a specific resource', 
    icon: 'file-text',
    help: 'Reads the content of a specific resource. Tests resource retrieval performance.',
  },
  { 
    value: 'prompts/list', 
    label: 'List Prompts', 
    description: 'Enumerate available prompts', 
    icon: 'message-square',
    help: 'Requests the list of available prompt templates from the MCP server.',
  },
  { 
    value: 'prompts/get', 
    label: 'Get Prompt', 
    description: 'Retrieve a specific prompt', 
    icon: 'edit',
    help: 'Fetches a specific prompt template by name with arguments.',
  },
  {
    value: 'subscriptions/listen',
    label: 'Listen Subscriptions',
    description: 'Open notification stream',
    icon: 'activity',
    help: 'Opens a 2026 MCP subscriptions/listen stream for list/resource change notifications.',
  },
  {
    value: 'tasks/get',
    label: 'Get Task',
    description: 'Poll task status',
    icon: 'activity',
    help: 'Polls a 2026 MCP Tasks extension task by task ID.',
  },
  {
    value: 'tasks/update',
    label: 'Update Task',
    description: 'Submit task input',
    icon: 'edit',
    help: 'Submits input responses to a 2026 MCP Tasks extension task.',
  },
  {
    value: 'tasks/cancel',
    label: 'Cancel Task',
    description: 'Cancel task',
    icon: 'activity',
    help: 'Requests cancellation of a 2026 MCP Tasks extension task.',
  },
];

export function WorkloadConfig({ config, onChange, targetUrl, headers, protocolVersion, protocolVersionPolicy, fetchedTools, onToolsFetched }: Props) {
  const [expandedToolConfig, setExpandedToolConfig] = useState<number | null>(null);
  const [localTools, setLocalTools] = useState<FetchedTool[]>(fetchedTools || []);

  useEffect(() => {
    if (fetchedTools !== undefined) {
      setLocalTools(fetchedTools);
    }
  }, [fetchedTools]);
  
  const totalWeight = useMemo(
    () => config.op_mix.reduce((sum, op) => sum + op.weight, 0),
    [config.op_mix]
  );

  const handleToolsFetched = useCallback((tools: FetchedTool[]) => {
    setLocalTools(tools);
    onToolsFetched?.(tools);
  }, [onToolsFetched]);

  const getToolSchema = useCallback((toolName: string): JSONSchema | undefined => {
    const tool = localTools.find(t => t.name === toolName);
    return tool?.inputSchema;
  }, [localTools]);

  const updateOperation = useCallback((index: number, updates: Partial<OpMixEntry>) => {
    const newOpMix = [...config.op_mix];
    newOpMix[index] = { ...newOpMix[index], ...updates };
    onChange({ ...config, op_mix: newOpMix });
  }, [config, onChange]);

  const updateObjectFromJSON = useCallback((index: number, field: 'input_responses' | 'notifications', value: string) => {
    if (!value.trim()) {
      updateOperation(index, { [field]: undefined });
      return;
    }
    try {
      const parsed = JSON.parse(value);
      if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
        updateOperation(index, { [field]: parsed });
      }
    } catch {
    }
  }, [updateOperation]);

  const addOperation = useCallback(() => {
    const usedOps = new Set(config.op_mix.map(op => op.operation));
    const toolsCallOp = OPERATIONS.find(op => op.value === 'tools/call');
    const availableOp = OPERATIONS.find(op => !usedOps.has(op.value) && !op.disabled);
    
    if (availableOp) {
      onChange({
        ...config,
        op_mix: [...config.op_mix, { operation: availableOp.value, weight: 1 }],
      });
    } else if (toolsCallOp && !toolsCallOp.disabled) {
      onChange({
        ...config,
        op_mix: [...config.op_mix, { operation: 'tools/call', weight: 1 }],
      });
    }
  }, [config, onChange]);

  const removeOperation = useCallback((index: number) => {
    const newOpMix = config.op_mix.filter((_, i) => i !== index);
    onChange({ ...config, op_mix: newOpMix });
  }, [config, onChange]);

  const getPercentage = useCallback((weight: number): string => {
    if (totalWeight === 0) return '0%';
    return `${Math.round((weight / totalWeight) * 100)}%`;
  }, [totalWeight]);

  return (
    <div className="wizard-step">
      <div className="step-header">
        <div className="step-icon" aria-hidden={true}><Icon name="dice" size="lg" /></div>
        <div className="step-title">
          <h2>
            Set Workload
            <HelpTooltip text="Configure which MCP operations to run and their relative frequency. Higher weights mean more frequent execution." />
          </h2>
          <p>Define the mix of MCP operations to execute during the test</p>
        </div>
      </div>

      <div className="workload-operations">
        {config.op_mix.map((op, index) => {
          const opMeta = OPERATIONS.find(o => o.value === op.operation);
          const opId = `operation-${index}`;
          return (
            <div key={index} className="operation-card">
              <div className="operation-header">
                <div className="operation-select-wrapper">
                  <label htmlFor={`${opId}-select`} className="sr-only">
                    Operation type
                  </label>
                  <select
                    id={`${opId}-select`}
                    value={op.operation}
                    onChange={e => updateOperation(index, { operation: e.target.value as OpMixEntry['operation'] })}
                    className="select-input operation-select"
                    aria-describedby={opMeta ? `${opId}-desc` : undefined}
                  >
                    {OPERATIONS.map(opOption => (
                      <option 
                        key={opOption.value} 
                        value={opOption.value}
                        disabled={opOption.disabled}
                      >
                        {opOption.label}{opOption.disabled ? ` (${opOption.disabledReason})` : ''}
                      </option>
                    ))}
                  </select>
                  {opMeta && (
                    <span className="operation-icon" aria-hidden={true}><Icon name={opMeta.icon} size="sm" /></span>
                  )}
                </div>
                <button
                  type="button"
                  onClick={() => removeOperation(index)}
                  className="btn btn-ghost btn-sm btn-danger"
                  disabled={config.op_mix.length <= 1}
                  aria-label={`Remove ${opMeta?.label || 'operation'}`}
                >
                  <span aria-hidden={true}>×</span>
                </button>
              </div>

              {opMeta && (
                <p id={`${opId}-desc`} className="operation-description">
                  {opMeta.description}
                  <HelpTooltip text={opMeta.help} />
                </p>
              )}

              <div className="operation-weight">
                <label htmlFor={`${opId}-weight`}>Weight</label>
                <div className="weight-control">
                  <input
                    id={`${opId}-weight`}
                    type="range"
                    min="1"
                    max="100"
                    value={op.weight}
                    onChange={e => updateOperation(index, { weight: parseInt(e.target.value) })}
                    className="weight-slider"
                    aria-valuemin={1}
                    aria-valuemax={100}
                    aria-valuenow={op.weight}
                    aria-valuetext={`${op.weight} (${getPercentage(op.weight)} of total)`}
                  />
                  <input
                    type="number"
                    min="1"
                    max="100"
                    value={op.weight}
                    onChange={e => {
                      const val = parseInt(e.target.value) || 1;
                      updateOperation(index, { weight: Math.max(1, Math.min(100, val)) });
                    }}
                    className="weight-input"
                    aria-label={`Weight value for ${op.operation}`}
                  />
                  <span className="weight-percentage" aria-hidden={true}>{getPercentage(op.weight)}</span>
                </div>
              </div>

              {op.operation === 'tools/call' && (
                <div className="operation-extra operation-tool-config">
                  <div className="tool-name-row">
                    <label htmlFor={`${opId}-tool-name`}>Tool Name</label>
                    <div className="tool-name-input-group">
                      <input
                        id={`${opId}-tool-name`}
                        type="text"
                        value={op.tool_name || ''}
                        onChange={e => updateOperation(index, { tool_name: e.target.value })}
                        placeholder="e.g., echo, add, get_time"
                        className="input"
                        aria-describedby={`${opId}-tool-name-hint`}
                      />
                      {localTools.length > 0 && op.tool_name && localTools.some(t => t.name === op.tool_name) && (
                        <span className="tool-valid-indicator" title="Tool exists on server">
                          <Icon name="check-circle" size="sm" aria-hidden={true} />
                        </span>
                      )}
                      <button
                        type="button"
                        onClick={() => setExpandedToolConfig(expandedToolConfig === index ? null : index)}
                        className="btn btn-secondary btn-sm"
                        aria-expanded={expandedToolConfig === index}
                        aria-label={expandedToolConfig === index ? 'Hide tool browser' : 'Browse available tools'}
                      >
                        <Icon name="search" size="sm" aria-hidden={true} />
                        {expandedToolConfig === index ? 'Hide' : 'Browse'}
                      </button>
                    </div>
                    <span id={`${opId}-tool-name-hint`} className="sr-only">
                      Enter the name of the tool to call or browse available tools
                    </span>
                  </div>

                  {expandedToolConfig === index && targetUrl && (
                    <div className="tool-browser-panel">
                      <ToolSelector
                        targetUrl={targetUrl}
                        selectedTool={op.tool_name || null}
                        onSelect={(toolName) => {
                          updateOperation(index, { 
                            tool_name: toolName,
                            arguments: op.arguments || {}
                          });
                        }}
                        tools={localTools}
                        onToolsFetched={handleToolsFetched}
                        headers={headers}
                        protocolVersion={protocolVersion}
                        protocolVersionPolicy={protocolVersionPolicy}
                      />
                    </div>
                  )}

                  {op.tool_name && (
                    <div className="tool-arguments-section">
                      <ToolArgumentsEditor
                        toolName={op.tool_name}
                        schema={getToolSchema(op.tool_name)}
                        value={op.arguments || {}}
                        onChange={(args) => updateOperation(index, { arguments: args })}
                        targetUrl={targetUrl}
                        headers={headers}
                        protocolVersion={protocolVersion}
                        protocolVersionPolicy={protocolVersionPolicy}
                      />
                    </div>
                  )}

                  <div className="field-row">
                    <label htmlFor={`${opId}-tool-input-responses`}>Input Responses JSON</label>
                    <input
                      id={`${opId}-tool-input-responses`}
                      type="text"
                      value={op.input_responses ? JSON.stringify(op.input_responses) : ''}
                      onChange={e => updateObjectFromJSON(index, 'input_responses', e.target.value)}
                      placeholder='{"request-1":{"type":"text","text":"answer"}}'
                      className="input"
                    />
                    <span className="field-hint">Optional MCP 2026 MRTR responses for retrying a tool call after input_required</span>
                  </div>
                  <div className="field-row">
                    <label htmlFor={`${opId}-tool-request-state`}>Request State</label>
                    <input
                      id={`${opId}-tool-request-state`}
                      type="text"
                      value={op.request_state || ''}
                      onChange={e => updateOperation(index, { request_state: e.target.value || undefined })}
                      placeholder="Opaque requestState returned by the server"
                      className="input"
                    />
                    <span className="field-hint">Echo the opaque requestState from an input_required result when retrying</span>
                  </div>
                </div>
              )}

              {op.operation === 'resources/read' && (
                <div className="operation-extra">
                  <div className="field-row">
                    <label htmlFor={`${opId}-uri`}>Resource URI</label>
                    <input
                      id={`${opId}-uri`}
                      type="text"
                      value={op.uri || ''}
                      onChange={e => updateOperation(index, { uri: e.target.value })}
                      placeholder="e.g., file:///docs/readme.md"
                      className="input"
                    />
                    <span className="field-hint">The URI of the resource to read</span>
                  </div>
                  <div className="field-row">
                    <label htmlFor={`${opId}-resource-input-responses`}>Input Responses JSON</label>
                    <input
                      id={`${opId}-resource-input-responses`}
                      type="text"
                      value={op.input_responses ? JSON.stringify(op.input_responses) : ''}
                      onChange={e => updateObjectFromJSON(index, 'input_responses', e.target.value)}
                      placeholder='{"request-1":{"type":"text","text":"answer"}}'
                      className="input"
                    />
                    <span className="field-hint">Optional MCP 2026 MRTR responses for retrying a resource read</span>
                  </div>
                  <div className="field-row">
                    <label htmlFor={`${opId}-resource-request-state`}>Request State</label>
                    <input
                      id={`${opId}-resource-request-state`}
                      type="text"
                      value={op.request_state || ''}
                      onChange={e => updateOperation(index, { request_state: e.target.value || undefined })}
                      placeholder="Opaque requestState returned by the server"
                      className="input"
                    />
                    <span className="field-hint">Echo the opaque requestState from an input_required result when retrying</span>
                  </div>
                </div>
              )}

              {op.operation === 'prompts/get' && (
                <div className="operation-extra">
                  <div className="field-row">
                    <label htmlFor={`${opId}-prompt-name`}>Prompt Name</label>
                    <input
                      id={`${opId}-prompt-name`}
                      type="text"
                      value={op.prompt_name || ''}
                      onChange={e => updateOperation(index, { prompt_name: e.target.value })}
                      placeholder="e.g., summarize, translate"
                      className="input"
                    />
                    <span className="field-hint">The name of the prompt template to retrieve</span>
                  </div>
                  <div className="field-row">
                    <label htmlFor={`${opId}-prompt-input-responses`}>Input Responses JSON</label>
                    <input
                      id={`${opId}-prompt-input-responses`}
                      type="text"
                      value={op.input_responses ? JSON.stringify(op.input_responses) : ''}
                      onChange={e => updateObjectFromJSON(index, 'input_responses', e.target.value)}
                      placeholder='{"request-1":{"type":"text","text":"answer"}}'
                      className="input"
                    />
                    <span className="field-hint">Optional MCP 2026 MRTR responses for retrying a prompt get</span>
                  </div>
                  <div className="field-row">
                    <label htmlFor={`${opId}-prompt-request-state`}>Request State</label>
                    <input
                      id={`${opId}-prompt-request-state`}
                      type="text"
                      value={op.request_state || ''}
                      onChange={e => updateOperation(index, { request_state: e.target.value || undefined })}
                      placeholder="Opaque requestState returned by the server"
                      className="input"
                    />
                    <span className="field-hint">Echo the opaque requestState from an input_required result when retrying</span>
                  </div>
                </div>
              )}

              {(op.operation === 'tasks/get' || op.operation === 'tasks/update' || op.operation === 'tasks/cancel') && (
                <div className="operation-extra">
                  <div className="field-row">
                    <label htmlFor={`${opId}-task-id`}>Task ID</label>
                    <input
                      id={`${opId}-task-id`}
                      type="text"
                      value={op.task_id || ''}
                      onChange={e => updateOperation(index, { task_id: e.target.value })}
                      placeholder="e.g., task-123"
                      className="input"
                    />
                    <span className="field-hint">The 2026 MCP task ID to poll, update, or cancel</span>
                  </div>
                  {op.operation === 'tasks/update' && (
                    <div className="field-row">
                      <label htmlFor={`${opId}-input-responses`}>Input Responses JSON</label>
                      <input
                        id={`${opId}-input-responses`}
                        type="text"
                        value={op.input_responses ? JSON.stringify(op.input_responses) : ''}
                        onChange={e => updateObjectFromJSON(index, 'input_responses', e.target.value)}
                        placeholder='{"request-1":{"type":"text","text":"answer"}}'
                        className="input"
                      />
                      <span className="field-hint">Responses keyed by the server-provided input request ID</span>
                    </div>
                  )}
                </div>
              )}

              {op.operation === 'subscriptions/listen' && (
                <div className="operation-extra">
                  <div className="field-row">
                    <label htmlFor={`${opId}-notifications`}>Notifications JSON</label>
                    <input
                      id={`${opId}-notifications`}
                      type="text"
                      value={op.notifications ? JSON.stringify(op.notifications) : ''}
                      onChange={e => updateObjectFromJSON(index, 'notifications', e.target.value)}
                      placeholder='{"toolsListChanged":true}'
                      className="input"
                    />
                    <span className="field-hint">Subscription filters, for example toolsListChanged or taskIds</span>
                  </div>
                </div>
              )}
            </div>
          );
        })}
      </div>

      {config.op_mix.length < 20 && (
        <button type="button" onClick={addOperation} className="btn btn-secondary add-operation-btn">
          + Add Operation
        </button>
      )}

      <div className="workload-distribution">
        <h3>Distribution Preview</h3>
        <div className="distribution-bar">
          {config.op_mix.map((op, index) => {
            const percentage = totalWeight > 0 ? (op.weight / totalWeight) * 100 : 0;
            const opMeta = OPERATIONS.find(o => o.value === op.operation);
            return (
              <div
                key={index}
                className={`distribution-segment distribution-op-${index}`}
                style={{ width: `${percentage}%` }}
                title={`${opMeta?.label}: ${getPercentage(op.weight)}`}
              >
                {percentage > 15 && opMeta && <Icon name={opMeta.icon} size="sm" aria-hidden={true} />}
              </div>
            );
          })}
        </div>
        <div className="distribution-legend">
          {config.op_mix.map((op, index) => {
            const opMeta = OPERATIONS.find(o => o.value === op.operation);
            return (
              <div key={index} className="legend-item">
                <span className={`legend-color legend-op-${index}`}>
                  {opMeta && <Icon name={opMeta.icon} size="sm" aria-hidden={true} />}
                </span>
                <span className="legend-label">{opMeta?.label}</span>
                <span className="legend-value">{getPercentage(op.weight)}</span>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
