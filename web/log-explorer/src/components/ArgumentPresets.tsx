import type { ArgumentPreset } from '../types';
import { Icon } from './Icon';

interface ArgumentPresetsProps {
  toolPresets: ArgumentPreset[];
  value: Record<string, unknown>;
  presetName: string;
  setPresetName: (name: string) => void;
  showPresetDialog: boolean;
  setShowPresetDialog: (show: boolean) => void;
  savePreset: () => void;
  loadPreset: (preset: ArgumentPreset) => void;
  deletePreset: (presetId: string) => void;
}

export function ArgumentPresets({
  toolPresets,
  value,
  presetName,
  setPresetName,
  showPresetDialog,
  setShowPresetDialog,
  savePreset,
  loadPreset,
  deletePreset,
}: ArgumentPresetsProps) {
  return (
    <>
      {toolPresets.length > 0 && (
        <div className="presets-bar">
          <span className="presets-label">Presets:</span>
          <div className="presets-list">
            {toolPresets.map(preset => (
              <div key={preset.id} className="preset-chip">
                <button
                  type="button"
                  onClick={() => loadPreset(preset)}
                  className="preset-load-btn"
                  aria-label={`Load preset: ${preset.name}`}
                >
                  {preset.name}
                </button>
                <button
                  type="button"
                  onClick={() => deletePreset(preset.id)}
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
              if (e.key === 'Enter') savePreset();
              if (e.key === 'Escape') setShowPresetDialog(false);
            }}
          />
          <button
            type="button"
            onClick={savePreset}
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
    </>
  );
}
