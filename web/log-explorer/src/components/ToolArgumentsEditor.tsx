import { useState, useCallback, useMemo, memo, useEffect } from 'react';
import type { JSONSchema, ArgumentPreset } from '../types';
import { Icon } from './Icon';
import { testTool, ToolTestResult } from '../api';

interface ToolArgumentsEditorProps {
  toolName: string;
  schema?: JSONSchema;
  value: Record<string, unknown>;
  onChange: (args: Record<string, unknown>) => void;
  presets?: ArgumentPreset[];
  onSavePreset?: (preset: Omit<ArgumentPreset, 'id' | 'createdAt'>) => void;
  targetUrl?: string;
  headers?: Record<string, string>;
}

interface ValidationError {
  path: string;
  message: string;
}

interface SchemaFieldProps {
  name: string;
  schema: JSONSchema;
  value: unknown;
  onChange: (value: unknown) => void;
  required?: boolean;
  errors: ValidationError[];
  path: string;
}

const STORAGE_KEY = 'mcpdrill-arg-presets';

function loadPresets(): ArgumentPreset[] {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    return stored ? JSON.parse(stored) : [];
  } catch {
    return [];
  }
}

function savePresetsToStorage(presets: ArgumentPreset[]) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(presets));
  } catch {
    console.error('Failed to save presets');
  }
}

function validateValue(schema: JSONSchema, value: unknown, path: string): ValidationError[] {
  const errors: ValidationError[] = [];
  
  if (value === undefined || value === null || value === '') {
    return errors;
  }

  if (schema.type === 'string' && typeof value === 'string') {
    if (schema.minLength !== undefined && value.length < schema.minLength) {
      errors.push({ path, message: `Minimum length is ${schema.minLength}` });
    }
    if (schema.maxLength !== undefined && value.length > schema.maxLength) {
      errors.push({ path, message: `Maximum length is ${schema.maxLength}` });
    }
    if (schema.pattern) {
      try {
        const regex = new RegExp(schema.pattern);
        if (!regex.test(value)) {
          errors.push({ path, message: `Must match pattern: ${schema.pattern}` });
        }
      } catch {
        errors.push({ path, message: `Invalid pattern in schema: ${schema.pattern}` });
      }
    }
  }

  if ((schema.type === 'number' || schema.type === 'integer') && typeof value === 'number') {
    if (schema.minimum !== undefined && value < schema.minimum) {
      errors.push({ path, message: `Minimum value is ${schema.minimum}` });
    }
    if (schema.maximum !== undefined && value > schema.maximum) {
      errors.push({ path, message: `Maximum value is ${schema.maximum}` });
    }
  }

  if (schema.enum && !schema.enum.includes(value)) {
    errors.push({ path, message: `Must be one of: ${schema.enum.join(', ')}` });
  }

  return errors;
}

function validateObject(schema: JSONSchema, value: Record<string, unknown>): ValidationError[] {
  const errors: ValidationError[] = [];
  
  if (!schema.properties) return errors;

  const required = new Set(schema.required || []);
  
  for (const key of required) {
    const val = value[key];
    if (val === undefined || val === null || val === '') {
      errors.push({ path: key, message: 'This field is required' });
    }
  }

  for (const [key, propSchema] of Object.entries(schema.properties)) {
    const val = value[key];
    errors.push(...validateValue(propSchema, val, key));
    
    if (propSchema.type === 'object' && propSchema.properties && typeof val === 'object' && val !== null) {
      const nestedErrors = validateObject(propSchema, val as Record<string, unknown>);
      errors.push(...nestedErrors.map(e => ({ ...e, path: `${key}.${e.path}` })));
    }
  }

  return errors;
}

const SchemaField = memo(function SchemaField({ 
  name, 
  schema, 
  value, 
  onChange, 
  required,
  errors,
  path 
}: SchemaFieldProps) {
  const fieldId = `field-${path.replace(/\./g, '-')}`;
  const fieldErrors = errors.filter(e => e.path === path);
  const hasError = fieldErrors.length > 0;

  const handleChange = useCallback((newValue: unknown) => {
    onChange(newValue);
  }, [onChange]);

  if (schema.enum) {
    return (
      <div className={`schema-field ${hasError ? 'has-error' : ''}`}>
        <div className="field-label-row">
          <label htmlFor={fieldId}>
            {schema.title || name}
          </label>
          {required ? (
            <span className="field-badge field-required-badge">Required</span>
          ) : (
            <span className="field-badge field-optional-badge">Optional</span>
          )}
        </div>
        {schema.description && <p className="field-description">{schema.description}</p>}
        <select
          id={fieldId}
          value={String(value ?? '')}
          onChange={e => handleChange(e.target.value)}
          className="select-input"
          aria-invalid={hasError}
          aria-describedby={hasError ? `${fieldId}-error` : undefined}
        >
          <option value="">Select...</option>
          {schema.enum.map((opt, i) => (
            <option key={i} value={String(opt)}>{String(opt)}</option>
          ))}
        </select>
        {hasError && (
          <span id={`${fieldId}-error`} className="field-error" role="alert">
            {fieldErrors[0].message}
          </span>
        )}
      </div>
    );
  }

  switch (schema.type) {
    case 'string':
      return (
        <div className={`schema-field ${hasError ? 'has-error' : ''}`}>
          <div className="field-label-row">
            <label htmlFor={fieldId}>
              {schema.title || name}
            </label>
            {required ? (
              <span className="field-badge field-required-badge">Required</span>
            ) : (
              <span className="field-badge field-optional-badge">Optional</span>
            )}
          </div>
          {schema.description && <p className="field-description">{schema.description}</p>}
          {schema.maxLength && schema.maxLength > 200 ? (
            <textarea
              id={fieldId}
              value={String(value ?? '')}
              onChange={e => handleChange(e.target.value)}
              className="input textarea"
              rows={4}
              placeholder={schema.default ? `Default: ${schema.default}` : undefined}
              aria-invalid={hasError}
              aria-describedby={hasError ? `${fieldId}-error` : undefined}
            />
          ) : (
            <input
              id={fieldId}
              type={schema.format === 'email' ? 'email' : schema.format === 'uri' ? 'url' : 'text'}
              value={String(value ?? '')}
              onChange={e => handleChange(e.target.value)}
              className="input"
              placeholder={schema.default ? `Default: ${schema.default}` : undefined}
              minLength={schema.minLength}
              maxLength={schema.maxLength}
              pattern={schema.pattern}
              aria-invalid={hasError}
              aria-describedby={hasError ? `${fieldId}-error` : undefined}
            />
          )}
          {hasError && (
            <span id={`${fieldId}-error`} className="field-error" role="alert">
              {fieldErrors[0].message}
            </span>
          )}
        </div>
      );

    case 'number':
    case 'integer':
      return (
        <div className={`schema-field ${hasError ? 'has-error' : ''}`}>
          <div className="field-label-row">
            <label htmlFor={fieldId}>
              {schema.title || name}
            </label>
            {required ? (
              <span className="field-badge field-required-badge">Required</span>
            ) : (
              <span className="field-badge field-optional-badge">Optional</span>
            )}
          </div>
          {schema.description && <p className="field-description">{schema.description}</p>}
          <input
            id={fieldId}
            type="number"
            value={value !== undefined && value !== null ? String(value) : ''}
            onChange={e => {
              const val = e.target.value;
              if (val === '') {
                handleChange(undefined);
              } else {
                handleChange(schema.type === 'integer' ? parseInt(val) : parseFloat(val));
              }
            }}
            className="input"
            min={schema.minimum}
            max={schema.maximum}
            step={schema.type === 'integer' ? 1 : 'any'}
            placeholder={schema.default !== undefined ? `Default: ${schema.default}` : undefined}
            aria-invalid={hasError}
            aria-describedby={hasError ? `${fieldId}-error` : undefined}
          />
          {hasError && (
            <span id={`${fieldId}-error`} className="field-error" role="alert">
              {fieldErrors[0].message}
            </span>
          )}
        </div>
      );

    case 'boolean':
      return (
        <div className={`schema-field schema-field-checkbox ${hasError ? 'has-error' : ''}`}>
          <div className="field-label-row">
            <label className="checkbox-label">
              <input
                id={fieldId}
                type="checkbox"
                checked={Boolean(value)}
                onChange={e => handleChange(e.target.checked)}
                aria-describedby={schema.description ? `${fieldId}-desc` : undefined}
              />
              <span className="checkbox-text">
                {schema.title || name}
              </span>
            </label>
            {required ? (
              <span className="field-badge field-required-badge">Required</span>
            ) : (
              <span className="field-badge field-optional-badge">Optional</span>
            )}
          </div>
          {schema.description && (
            <p id={`${fieldId}-desc`} className="field-description">{schema.description}</p>
          )}
        </div>
      );

    case 'array':
      const arrayValue = Array.isArray(value) ? value : [];
      return (
        <div className={`schema-field schema-field-array ${hasError ? 'has-error' : ''}`}>
          <div className="field-label-row">
            <label>
              {schema.title || name}
            </label>
            {required ? (
              <span className="field-badge field-required-badge">Required</span>
            ) : (
              <span className="field-badge field-optional-badge">Optional</span>
            )}
          </div>
          {schema.description && <p className="field-description">{schema.description}</p>}
          <div className="array-items">
            {arrayValue.map((item, index) => (
              <div key={index} className="array-item">
                <input
                  type="text"
                  value={String(item ?? '')}
                  onChange={e => {
                    const newArray = [...arrayValue];
                    newArray[index] = e.target.value;
                    handleChange(newArray);
                  }}
                  className="input"
                  aria-label={`${name} item ${index + 1}`}
                />
                <button
                  type="button"
                  onClick={() => {
                    const newArray = arrayValue.filter((_, i) => i !== index);
                    handleChange(newArray);
                  }}
                  className="btn btn-ghost btn-sm btn-danger"
                  aria-label={`Remove ${name} item ${index + 1}`}
                >
                  <Icon name="x" size="sm" aria-hidden={true} />
                </button>
              </div>
            ))}
            <button
              type="button"
              onClick={() => handleChange([...arrayValue, ''])}
              className="btn btn-secondary btn-sm"
            >
              <Icon name="plus" size="sm" aria-hidden={true} />
              Add Item
            </button>
          </div>
        </div>
      );

    case 'object':
      if (!schema.properties) {
        return (
          <div className="schema-field">
            <label>{schema.title || name}</label>
            <p className="field-description muted">Object without defined schema</p>
          </div>
        );
      }
      const objectValue = (typeof value === 'object' && value !== null) 
        ? value as Record<string, unknown> 
        : {};
      const nestedRequired = new Set(schema.required || []);
      
      return (
        <fieldset className="schema-field schema-field-object">
          <div className="field-label-row">
            <legend>
              {schema.title || name}
            </legend>
            {required ? (
              <span className="field-badge field-required-badge">Required</span>
            ) : (
              <span className="field-badge field-optional-badge">Optional</span>
            )}
          </div>
          {schema.description && <p className="field-description">{schema.description}</p>}
          <div className="nested-fields">
            {Object.entries(schema.properties).map(([key, propSchema]) => (
              <SchemaField
                key={key}
                name={key}
                schema={propSchema}
                value={objectValue[key]}
                onChange={newVal => {
                  handleChange({ ...objectValue, [key]: newVal });
                }}
                required={nestedRequired.has(key)}
                errors={errors}
                path={`${path}.${key}`}
              />
            ))}
          </div>
        </fieldset>
      );

    default:
      return (
        <div className="schema-field">
          <label htmlFor={fieldId}>{schema.title || name}</label>
          <input
            id={fieldId}
            type="text"
            value={String(value ?? '')}
            onChange={e => handleChange(e.target.value)}
            className="input"
          />
        </div>
      );
  }
});

function ToolArgumentsEditorComponent({
  toolName,
  schema,
  value,
  onChange,
  presets: externalPresets,
  onSavePreset,
  targetUrl,
  headers,
}: ToolArgumentsEditorProps) {
  const [mode, setMode] = useState<'form' | 'json'>('form');
  const [jsonText, setJsonText] = useState('');
  const [jsonError, setJsonError] = useState<string | null>(null);
  const [presets, setPresets] = useState<ArgumentPreset[]>([]);
  const [presetName, setPresetName] = useState('');
  const [showPresetDialog, setShowPresetDialog] = useState(false);
  const [testResult, setTestResult] = useState<ToolTestResult | null>(null);
  const [isTesting, setIsTesting] = useState(false);

  useEffect(() => {
    const stored = loadPresets();
    setPresets(externalPresets || stored);
  }, [externalPresets]);

  useEffect(() => {
    if (mode === 'json') {
      setJsonText(JSON.stringify(value, null, 2));
      setJsonError(null);
    }
  }, [mode, value]);

  const validationErrors = useMemo(() => {
    if (!schema) return [];
    return validateObject(schema, value);
  }, [schema, value]);

  const isValid = validationErrors.length === 0;

  const toolPresets = useMemo(() => 
    presets.filter(p => p.toolName === toolName),
    [presets, toolName]
  );

  const handleJsonChange = useCallback((text: string) => {
    setJsonText(text);
    try {
      const parsed = JSON.parse(text);
      if (typeof parsed === 'object' && parsed !== null && !Array.isArray(parsed)) {
        onChange(parsed);
        setJsonError(null);
      } else {
        setJsonError('Value must be a JSON object');
      }
    } catch {
      setJsonError('Invalid JSON syntax');
    }
  }, [onChange]);

  const handleFieldChange = useCallback((key: string, fieldValue: unknown) => {
    onChange({ ...value, [key]: fieldValue });
  }, [value, onChange]);

  const handleSavePreset = useCallback(() => {
    if (!presetName.trim()) return;

    const newPreset: ArgumentPreset = {
      id: `preset-${Date.now()}`,
      name: presetName.trim(),
      toolName,
      arguments: value,
      createdAt: Date.now(),
    };

    const updated = [...presets, newPreset];
    setPresets(updated);
    savePresetsToStorage(updated);
    onSavePreset?.({ name: newPreset.name, toolName, arguments: value });
    setPresetName('');
    setShowPresetDialog(false);
  }, [presetName, toolName, value, presets, onSavePreset]);

  const handleLoadPreset = useCallback((preset: ArgumentPreset) => {
    onChange(preset.arguments);
    if (mode === 'json') {
      setJsonText(JSON.stringify(preset.arguments, null, 2));
    }
  }, [onChange, mode]);

  const handleDeletePreset = useCallback((presetId: string) => {
    const updated = presets.filter(p => p.id !== presetId);
    setPresets(updated);
    savePresetsToStorage(updated);
  }, [presets]);

  const handleExportArgs = useCallback(() => {
    const blob = new Blob([JSON.stringify(value, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${toolName}-args.json`;
    a.click();
    URL.revokeObjectURL(url);
  }, [value, toolName]);

  const handleImportArgs = useCallback(() => {
    const input = document.createElement('input');
    input.type = 'file';
    input.accept = '.json';
    input.onchange = async (e) => {
      const file = (e.target as HTMLInputElement).files?.[0];
      if (!file) return;
      
      try {
        const text = await file.text();
        const parsed = JSON.parse(text);
        if (typeof parsed === 'object' && parsed !== null && !Array.isArray(parsed)) {
          onChange(parsed);
          if (mode === 'json') {
            setJsonText(JSON.stringify(parsed, null, 2));
          }
        }
      } catch {
        setJsonError('Failed to parse imported file');
      }
    };
    input.click();
  }, [onChange, mode]);

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

  if (!schema || !schema.properties) {
    return (
      <div className="tool-args-editor tool-args-empty">
        <Icon name="code" size="lg" aria-hidden={true} />
        <p>No arguments schema available for this tool</p>
        <p className="muted">You can still provide arguments in JSON mode</p>
        <button
          type="button"
          onClick={() => setMode('json')}
          className="btn btn-secondary btn-sm"
        >
          Switch to JSON Mode
        </button>
        {mode === 'json' && (
          <div className="json-editor-wrapper">
            <textarea
              value={jsonText}
              onChange={e => handleJsonChange(e.target.value)}
              className="json-editor"
              rows={10}
              spellCheck={false}
              aria-label="JSON arguments editor"
              aria-invalid={!!jsonError}
            />
            {jsonError && (
              <div className="json-error" role="alert">
                <Icon name="alert-triangle" size="sm" aria-hidden={true} />
                {jsonError}
              </div>
            )}
          </div>
        )}
      </div>
    );
  }

  const requiredFields = new Set(schema.required || []);

  return (
    <div className="tool-args-editor" role="region" aria-label="Tool Arguments Configuration">
      <div className="tool-args-header">
        <div className="tool-args-title">
          <Icon name="edit" size="md" aria-hidden={true} />
          <h3>Arguments for {toolName}</h3>
          {!isValid && (
            <span className="validation-badge invalid" aria-label="Has validation errors">
              {validationErrors.length} error{validationErrors.length > 1 ? 's' : ''}
            </span>
          )}
          {isValid && Object.keys(value).length > 0 && (
            <span className="validation-badge valid" aria-label="Valid">
              <Icon name="check" size="xs" aria-hidden={true} />
            </span>
          )}
        </div>
        <div className="tool-args-actions">
          {targetUrl && (
            <button
              type="button"
              onClick={handleTestTool}
              disabled={isTesting || !isValid}
              className="btn btn-secondary btn-sm test-tool-btn"
              aria-label={`Test tool with current arguments${headers && Object.keys(headers).length > 0 ? ' (with custom headers)' : ''}`}
              title={headers && Object.keys(headers).length > 0 ? `Using ${Object.keys(headers).length} custom header(s)` : undefined}
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
                  {headers && Object.keys(headers).length > 0 && (
                    <span className="header-indicator" aria-hidden={true}>+H</span>
                  )}
                </>
              )}
            </button>
          )}
        </div>
      </div>

      <p className="tool-args-description">
        Configure the arguments that will be passed to this tool during the load test. 
        Fields marked with <span className="required-indicator">*</span> are required.
      </p>

      <div className="tool-args-mode-section">
        <div className="mode-toggle-container">
          <div className="mode-toggle-group" role="tablist" aria-label="Editor mode">
            <button
              type="button"
              role="tab"
              aria-selected={mode === 'form'}
              onClick={() => setMode('form')}
              className={`mode-toggle-btn ${mode === 'form' ? 'active' : ''}`}
            >
              <Icon name="layout" size="sm" aria-hidden={true} />
              <span className="mode-label">Form View</span>
            </button>
            <button
              type="button"
              role="tab"
              aria-selected={mode === 'json'}
              onClick={() => setMode('json')}
              className={`mode-toggle-btn ${mode === 'json' ? 'active' : ''}`}
            >
              <Icon name="code" size="sm" aria-hidden={true} />
              <span className="mode-label">JSON View</span>
            </button>
          </div>
          <span className="mode-hint">
            {mode === 'form' 
              ? 'Fill in fields using the guided form' 
              : 'Edit raw JSON directly for advanced configuration'}
          </span>
        </div>
        <div className="mode-actions">
          <button
            type="button"
            onClick={handleExportArgs}
            className="btn btn-ghost btn-sm"
            aria-label="Export arguments as JSON file"
            title="Export as JSON"
          >
            <Icon name="download" size="sm" aria-hidden={true} />
            Export
          </button>
          <button
            type="button"
            onClick={handleImportArgs}
            className="btn btn-ghost btn-sm"
            aria-label="Import arguments from JSON file"
            title="Import from JSON"
          >
            <Icon name="upload" size="sm" aria-hidden={true} />
            Import
          </button>
        </div>
      </div>

      {testResult && (
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
              onClick={() => setTestResult(null)}
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
      )}

      {toolPresets.length > 0 && (
        <div className="presets-bar">
          <span className="presets-label">Presets:</span>
          <div className="presets-list">
            {toolPresets.map(preset => (
              <div key={preset.id} className="preset-chip">
                <button
                  type="button"
                  onClick={() => handleLoadPreset(preset)}
                  className="preset-load-btn"
                  aria-label={`Load preset: ${preset.name}`}
                >
                  {preset.name}
                </button>
                <button
                  type="button"
                  onClick={() => handleDeletePreset(preset.id)}
                  className="preset-delete-btn"
                  aria-label={`Delete preset: ${preset.name}`}
                >
                  <Icon name="x" size="xs" aria-hidden={true} />
                </button>
              </div>
            ))}
          </div>
          <button
            type="button"
            onClick={() => setShowPresetDialog(true)}
            className="btn btn-ghost btn-sm"
            disabled={Object.keys(value).length === 0}
          >
            <Icon name="save" size="sm" aria-hidden={true} />
            Save Preset
          </button>
        </div>
      )}

      {showPresetDialog && (
        <div className="preset-dialog" role="dialog" aria-label="Save preset">
          <input
            type="text"
            value={presetName}
            onChange={e => setPresetName(e.target.value)}
            placeholder="Preset name..."
            className="input"
            autoFocus
            onKeyDown={e => {
              if (e.key === 'Enter') handleSavePreset();
              if (e.key === 'Escape') setShowPresetDialog(false);
            }}
          />
          <button
            type="button"
            onClick={handleSavePreset}
            disabled={!presetName.trim()}
            className="btn btn-primary btn-sm"
          >
            Save
          </button>
          <button
            type="button"
            onClick={() => setShowPresetDialog(false)}
            className="btn btn-ghost btn-sm"
          >
            Cancel
          </button>
        </div>
      )}

      {toolPresets.length === 0 && Object.keys(value).length > 0 && (
        <div className="presets-bar presets-bar-empty">
          <button
            type="button"
            onClick={() => setShowPresetDialog(true)}
            className="btn btn-secondary btn-sm"
          >
            <Icon name="save" size="sm" aria-hidden={true} />
            Save as Preset
          </button>
        </div>
      )}

      {mode === 'form' ? (
        <div className="schema-form-container">
          <div className="schema-form" role="form" aria-label="Arguments form">
            {Object.entries(schema.properties).map(([key, propSchema]) => (
              <SchemaField
                key={key}
                name={key}
                schema={propSchema}
                value={value[key]}
                onChange={newVal => handleFieldChange(key, newVal)}
                required={requiredFields.has(key)}
                errors={validationErrors}
                path={key}
              />
            ))}
          </div>
        </div>
      ) : (
        <div className="json-editor-wrapper">
          <textarea
            value={jsonText}
            onChange={e => handleJsonChange(e.target.value)}
            className={`json-editor ${jsonError ? 'has-error' : ''}`}
            rows={15}
            spellCheck={false}
            aria-label="JSON arguments editor"
            aria-invalid={!!jsonError}
          />
          {jsonError && (
            <div className="json-error" role="alert">
              <Icon name="alert-triangle" size="sm" aria-hidden={true} />
              {jsonError}
            </div>
          )}
        </div>
      )}

      {validationErrors.length > 0 && mode === 'form' && (
        <div className="validation-summary" role="alert">
          <Icon name="alert-triangle" size="sm" aria-hidden={true} />
          <span>{validationErrors.length} validation error{validationErrors.length > 1 ? 's' : ''}</span>
        </div>
      )}
    </div>
  );
}

export const ToolArgumentsEditor = memo(ToolArgumentsEditorComponent);
