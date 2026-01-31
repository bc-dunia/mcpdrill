import { useState, useCallback, memo, useEffect } from 'react';
import type { JSONSchema, ArgumentPreset } from '../types';
import { Icon } from './Icon';
import { ArgumentModeSection } from './ArgumentModeSection';
import { ArgumentPresets } from './ArgumentPresets';
import { SchemaForm } from './SchemaForm';
import { useArgumentValidation } from '../hooks/useArgumentValidation';
import { usePresets } from '../hooks/usePresets';
import { ToolTestPanel, ToolTestPanelButton, ToolTestPanelResult } from './ToolTestPanel';

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

  useEffect(() => {
    if (mode === 'json') {
      setJsonText(JSON.stringify(value, null, 2));
      setJsonError(null);
    }
  }, [mode, value]);

  const { validationErrors, isValid } = useArgumentValidation(schema, value);

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

  const handlePresetLoad = useCallback((args: Record<string, unknown>) => {
    if (mode === 'json') {
      setJsonText(JSON.stringify(args, null, 2));
    }
  }, [mode]);

  const {
    toolPresets,
    presetName,
    setPresetName,
    showPresetDialog,
    setShowPresetDialog,
    savePreset,
    loadPreset,
    deletePreset,
  } = usePresets({
    toolName,
    value,
    externalPresets,
    onSavePreset,
    onChange,
    onAfterLoad: handlePresetLoad,
  });

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

  return (
    <ToolTestPanel
      targetUrl={targetUrl}
      toolName={toolName}
      value={value}
      headers={headers}
      isValid={isValid}
    >
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
            <ToolTestPanelButton />
          </div>
        </div>

        <p className="tool-args-description">
          Configure the arguments that will be passed to this tool during the load test.
          Fields marked with <span className="required-indicator">*</span> are required.
        </p>

        <ArgumentModeSection
          mode={mode}
          onModeChange={setMode}
          onExport={handleExportArgs}
          onImport={handleImportArgs}
        />

        <ToolTestPanelResult />

        <ArgumentPresets
          toolPresets={toolPresets}
          value={value}
          presetName={presetName}
          setPresetName={setPresetName}
          showPresetDialog={showPresetDialog}
          setShowPresetDialog={setShowPresetDialog}
          savePreset={savePreset}
          loadPreset={loadPreset}
          deletePreset={deletePreset}
        />

        {mode === 'form' ? (
          <div className="schema-form-container">
            <SchemaForm schema={schema} value={value} onChange={onChange} errors={validationErrors} />
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
    </ToolTestPanel>
  );
}

export const ToolArgumentsEditor = memo(ToolArgumentsEditorComponent);
