import { useCallback, useEffect, useMemo, useState } from 'react';
import type { ArgumentPreset } from '../types';

const STORAGE_KEY = 'mcpdrill-arg-presets';

export function loadPresets(): ArgumentPreset[] {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    return stored ? JSON.parse(stored) : [];
  } catch {
    return [];
  }
}

export function savePresetsToStorage(presets: ArgumentPreset[]) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(presets));
  } catch {
    console.error('Failed to save presets');
  }
}

interface UsePresetsOptions {
  toolName: string;
  value: Record<string, unknown>;
  externalPresets?: ArgumentPreset[];
  onSavePreset?: (preset: Omit<ArgumentPreset, 'id' | 'createdAt'>) => void;
  onChange: (args: Record<string, unknown>) => void;
  onAfterLoad?: (args: Record<string, unknown>) => void;
}

export function usePresets({
  toolName,
  value,
  externalPresets,
  onSavePreset,
  onChange,
  onAfterLoad,
}: UsePresetsOptions) {
  const [presets, setPresets] = useState<ArgumentPreset[]>([]);
  const [presetName, setPresetName] = useState('');
  const [showPresetDialog, setShowPresetDialog] = useState(false);

  useEffect(() => {
    const stored = loadPresets();
    setPresets(externalPresets || stored);
  }, [externalPresets]);

  const toolPresets = useMemo(() =>
    presets.filter(p => p.toolName === toolName),
    [presets, toolName],
  );

  const savePreset = useCallback(() => {
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

  const loadPreset = useCallback((preset: ArgumentPreset) => {
    onChange(preset.arguments);
    onAfterLoad?.(preset.arguments);
  }, [onChange, onAfterLoad]);

  const deletePreset = useCallback((presetId: string) => {
    const updated = presets.filter(p => p.id !== presetId);
    setPresets(updated);
    savePresetsToStorage(updated);
  }, [presets]);

  return {
    presets,
    toolPresets,
    presetName,
    setPresetName,
    showPresetDialog,
    setShowPresetDialog,
    savePreset,
    loadPreset,
    deletePreset,
  };
}
