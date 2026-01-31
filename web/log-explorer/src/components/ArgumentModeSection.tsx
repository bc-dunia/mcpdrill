import { Icon } from './Icon';

interface ArgumentModeSectionProps {
  mode: 'form' | 'json';
  onModeChange: (mode: 'form' | 'json') => void;
  onExport: () => void;
  onImport: () => void;
}

export function ArgumentModeSection({ mode, onModeChange, onExport, onImport }: ArgumentModeSectionProps) {
  return (
    <div className="tool-args-mode-section">
      <div className="mode-toggle-container">
        <div className="mode-toggle-group" role="tablist" aria-label="Editor mode">
          <button
            type="button"
            role="tab"
            aria-selected={mode === 'form'}
            onClick={() => onModeChange('form')}
            className={`mode-toggle-btn ${mode === 'form' ? 'active' : ''}`}
          >
            <Icon name="layout" size="sm" aria-hidden={true} />
            <span className="mode-label">Form View</span>
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={mode === 'json'}
            onClick={() => onModeChange('json')}
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
          onClick={onExport}
          className="btn btn-ghost btn-sm"
          aria-label="Export arguments as JSON file"
          title="Export as JSON"
        >
          <Icon name="download" size="sm" aria-hidden={true} />
          Export
        </button>
        <button
          type="button"
          onClick={onImport}
          className="btn btn-ghost btn-sm"
          aria-label="Import arguments from JSON file"
          title="Import from JSON"
        >
          <Icon name="upload" size="sm" aria-hidden={true} />
          Import
        </button>
      </div>
    </div>
  );
}
