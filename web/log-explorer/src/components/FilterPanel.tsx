import { memo, useState, useEffect, useCallback } from 'react';
import type { LogFilters } from '../types'
import { ConfirmDialog } from './ConfirmDialog';

interface FilterPanelProps {
  filters: LogFilters;
  onChange: (filters: LogFilters) => void;
}

const emptyFilters: LogFilters = {
  stage: '',
  stage_id: '',
  worker_id: '',
  session_id: '',
  vu_id: '',
  operation: '',
  tool_name: '',
  error_type: '',
  error_code: '',
};

function FilterPanelComponent({ filters, onChange }: FilterPanelProps) {
  const [showClearConfirm, setShowClearConfirm] = useState(false);
  
  const handleChange = (field: keyof LogFilters, value: string) => {
    onChange({ ...filters, [field]: value });
  };

  const handleClear = useCallback(() => {
    onChange(emptyFilters);
    setShowClearConfirm(false);
  }, [onChange]);

  const handleClearClick = () => {
    const activeCount = Object.values(filters).filter(v => v !== '').length;
    if (activeCount > 1) {
      setShowClearConfirm(true);
    } else {
      handleClear();
    }
  };

  const hasActiveFilters = Object.values(filters).some(v => v !== '');
  const activeFilterCount = Object.values(filters).filter(v => v !== '').length;

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      const target = e.target as HTMLElement;
      const isInInput = target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.tagName === 'SELECT';
      
      if (e.key === 'Escape' && hasActiveFilters && !showClearConfirm && !isInInput) {
        e.preventDefault();
        const activeCount = Object.values(filters).filter(v => v !== '').length;
        if (activeCount > 1) {
          setShowClearConfirm(true);
        } else {
          handleClear();
        }
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [hasActiveFilters, showClearConfirm, handleClear, filters]);

  return (
    <>
      <ConfirmDialog
        isOpen={showClearConfirm}
        title="Clear All Filters"
        message={`Are you sure you want to clear all ${activeFilterCount} active filters? This will reset your current view.`}
        confirmLabel="Clear Filters"
        cancelLabel="Keep Filters"
        variant="warning"
        onConfirm={handleClear}
        onCancel={() => setShowClearConfirm(false)}
      />
      <div className="filter-panel" role="search" aria-label="Filter logs">
        <div className="filter-panel-header">
          <h3 id="filter-panel-heading">
            Filters
            {hasActiveFilters && (
              <span className="filter-count" aria-label={`${activeFilterCount} active`}>
                {activeFilterCount}
              </span>
            )}
          </h3>
          {hasActiveFilters && (
            <button 
              onClick={handleClearClick} 
              className="btn btn-ghost btn-sm"
              aria-label="Clear all filters (Escape)"
            >
              Clear all
              <span className="keyboard-shortcut" aria-hidden="true">
                <kbd>Esc</kbd>
              </span>
            </button>
          )}
        </div>
      
      <div className="filter-grid filter-grid-expanded">
        <div className="filter-field">
          <label htmlFor="filter-stage">Stage</label>
          <input
            id="filter-stage"
            type="text"
            value={filters.stage}
            onChange={e => handleChange('stage', e.target.value)}
            placeholder="e.g., baseline"
            className="input"
          />
        </div>

        <div className="filter-field">
          <label htmlFor="filter-stage-id">Stage ID</label>
          <input
            id="filter-stage-id"
            type="text"
            value={filters.stage_id}
            onChange={e => handleChange('stage_id', e.target.value)}
            placeholder="e.g., stg_abc123"
            className="input"
          />
        </div>

        <div className="filter-field">
          <label htmlFor="filter-worker">Worker ID</label>
          <input
            id="filter-worker"
            type="text"
            value={filters.worker_id}
            onChange={e => handleChange('worker_id', e.target.value)}
            placeholder="e.g., worker-1"
            className="input"
          />
        </div>

        <div className="filter-field">
          <label htmlFor="filter-session">Session ID</label>
          <input
            id="filter-session"
            type="text"
            value={filters.session_id}
            onChange={e => handleChange('session_id', e.target.value)}
            placeholder="e.g., ses_abc123"
            className="input"
          />
        </div>

        <div className="filter-field">
          <label htmlFor="filter-vu">VU ID</label>
          <input
            id="filter-vu"
            type="text"
            value={filters.vu_id}
            onChange={e => handleChange('vu_id', e.target.value)}
            placeholder="e.g., vu_001"
            className="input"
          />
        </div>

        <div className="filter-field">
          <label htmlFor="filter-operation">Operation</label>
          <input
            id="filter-operation"
            type="text"
            value={filters.operation}
            onChange={e => handleChange('operation', e.target.value)}
            placeholder="e.g., tools/call"
            className="input"
          />
        </div>

        <div className="filter-field">
          <label htmlFor="filter-tool">Tool Name</label>
          <input
            id="filter-tool"
            type="text"
            value={filters.tool_name}
            onChange={e => handleChange('tool_name', e.target.value)}
            placeholder="e.g., list_files"
            className="input"
          />
        </div>

        <div className="filter-field">
          <label htmlFor="filter-error">Error Type</label>
          <input
            id="filter-error"
            type="text"
            value={filters.error_type}
            onChange={e => handleChange('error_type', e.target.value)}
            placeholder="e.g., timeout"
            className="input"
          />
        </div>

        <div className="filter-field">
          <label htmlFor="filter-error-code">Error Code</label>
          <input
            id="filter-error-code"
            type="text"
            value={filters.error_code}
            onChange={e => handleChange('error_code', e.target.value)}
            placeholder="e.g., CONN_REFUSED"
            className="input"
          />
        </div>
      </div>
      </div>
    </>
  )
}

export const FilterPanel = memo(FilterPanelComponent);
