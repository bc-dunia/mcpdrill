import { useState, useEffect, useCallback, useMemo, memo, type MouseEvent } from 'react';
import type { FetchedTool, JSONSchema } from '../types';
import { Icon } from './Icon';
import { discoverTools } from '../api/index';

type FilterType = 'all' | 'no-params' | 'with-params';
type BehaviorFilter = 'all' | 'read-only' | 'mutable' | 'destructive';

interface ToolSelectorProps {
  targetUrl: string;
  selectedTool: string | null;
  onSelect: (toolName: string, schema?: JSONSchema) => void;
  tools?: FetchedTool[];
  onToolsFetched?: (tools: FetchedTool[]) => void;
  headers?: Record<string, string>;
}

interface SchemaDisplayProps {
  schema: JSONSchema;
  depth?: number;
}

const SchemaDisplay = memo(function SchemaDisplay({ schema, depth = 0 }: SchemaDisplayProps) {
  const indent = depth * 16;
  
  if (!schema) return null;
  
  const renderType = (s: JSONSchema = schema) => {
    if (s.enum) {
      return `enum: [${s.enum.map(v => JSON.stringify(v)).join(', ')}]`;
    }
    if (s.type === 'array' && s.items) {
      return `array<${s.items.type || 'any'}>`;
    }
    return s.type || 'any';
  };

  if (schema.type === 'object' && schema.properties) {
    const required = new Set(schema.required || []);
    return (
      <div className="schema-object" style={{ marginLeft: indent }}>
        {Object.entries(schema.properties).map(([key, propSchema]) => (
          <div key={key} className="schema-property">
            <span className="schema-key">
              {key}
              {required.has(key) && <span className="schema-required" title="Required">*</span>}
            </span>
            <span className="schema-type">{renderType(propSchema)}</span>
            {propSchema.description && (
              <span className="schema-desc">{propSchema.description}</span>
            )}
            {propSchema.type === 'object' && propSchema.properties && (
              <SchemaDisplay schema={propSchema} depth={depth + 1} />
            )}
          </div>
        ))}
      </div>
    );
  }

  return (
    <div className="schema-simple" style={{ marginLeft: indent }}>
      <span className="schema-type">{renderType()}</span>
      {schema.description && <span className="schema-desc">{schema.description}</span>}
    </div>
  );
});

function ToolSelectorComponent({ 
  targetUrl, 
  selectedTool, 
  onSelect, 
  tools: externalTools,
  onToolsFetched,
  headers,
}: ToolSelectorProps) {
  const [tools, setTools] = useState<FetchedTool[]>(externalTools || []);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [searchQuery, setSearchQuery] = useState('');
  const [expandedTool, setExpandedTool] = useState<string | null>(null);
  const [copyFeedback, setCopyFeedback] = useState<string | null>(null);
  const [activeFilter, setActiveFilter] = useState<FilterType>('all');
  const [behaviorFilter, setBehaviorFilter] = useState<BehaviorFilter>('all');
  const [selectedGroup, setSelectedGroup] = useState<string | null>(null);

  const fetchTools = useCallback(async () => {
    if (!targetUrl) {
      setError('Target URL is required');
      return;
    }

    setLoading(true);
    setError(null);

    try {
      const fetchedTools: FetchedTool[] = await discoverTools(targetUrl, headers);
      setTools(fetchedTools);
      onToolsFetched?.(fetchedTools);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to fetch tools';
      setError(message);
    } finally {
      setLoading(false);
    }
  }, [targetUrl, headers, onToolsFetched]);

  useEffect(() => {
    if (externalTools) {
      setTools(externalTools);
    }
  }, [externalTools]);

  const hasRequiredParams = useCallback((tool: FetchedTool): boolean => {
    const schema = tool.inputSchema;
    if (!schema || !schema.properties) return false;
    return (schema.required?.length ?? 0) > 0;
  }, []);

  const getToolGroup = useCallback((toolName: string): string => {
    const underscoreIdx = toolName.indexOf('_');
    if (underscoreIdx > 0 && underscoreIdx < toolName.length - 1) {
      return toolName.substring(0, underscoreIdx);
    }
    return 'other';
  }, []);

  const toolGroups = useMemo(() => {
    const groups = new Map<string, number>();
    tools.forEach(tool => {
      const group = getToolGroup(tool.name);
      groups.set(group, (groups.get(group) || 0) + 1);
    });
    return Array.from(groups.entries())
      .filter(([, count]) => count >= 2)
      .sort((a, b) => b[1] - a[1]);
  }, [tools, getToolGroup]);

  const behaviorCounts = useMemo(() => ({
    all: tools.length,
    readOnly: tools.filter(t => t.annotations?.readOnlyHint === true).length,
    mutable: tools.filter(t => !t.annotations?.readOnlyHint && !t.annotations?.destructiveHint).length,
    destructive: tools.filter(t => t.annotations?.destructiveHint === true).length,
  }), [tools]);

  const filteredTools = useMemo(() => {
    let result = tools;
    
    if (activeFilter === 'no-params') {
      result = result.filter(tool => !hasRequiredParams(tool));
    } else if (activeFilter === 'with-params') {
      result = result.filter(tool => hasRequiredParams(tool));
    }

    if (behaviorFilter === 'read-only') {
      result = result.filter(tool => tool.annotations?.readOnlyHint === true);
    } else if (behaviorFilter === 'mutable') {
      result = result.filter(tool => !tool.annotations?.readOnlyHint && !tool.annotations?.destructiveHint);
    } else if (behaviorFilter === 'destructive') {
      result = result.filter(tool => tool.annotations?.destructiveHint === true);
    }

    if (selectedGroup) {
      result = result.filter(tool => getToolGroup(tool.name) === selectedGroup);
    }
    
    if (searchQuery.trim()) {
      const query = searchQuery.toLowerCase();
      result = result.filter(tool => 
        tool.name.toLowerCase().includes(query) ||
        (tool.description?.toLowerCase().includes(query) ?? false)
      );
    }
    
    return result;
  }, [tools, searchQuery, activeFilter, behaviorFilter, selectedGroup, hasRequiredParams, getToolGroup]);

  const filterCounts = useMemo(() => ({
    all: tools.length,
    noParams: tools.filter(t => !hasRequiredParams(t)).length,
    withParams: tools.filter(t => hasRequiredParams(t)).length,
  }), [tools, hasRequiredParams]);

  const handleSelectRandom = useCallback(() => {
    if (filteredTools.length === 0) return;
    const randomIndex = Math.floor(Math.random() * filteredTools.length);
    const tool = filteredTools[randomIndex];
    onSelect(tool.name, tool.inputSchema);
  }, [filteredTools, onSelect]);

  const isValidSelection = useMemo(() => {
    if (!selectedTool) return false;
    return tools.some(t => t.name === selectedTool);
  }, [selectedTool, tools]);

  const handleCopyToolName = useCallback(async (toolName: string, e: MouseEvent) => {
    e.stopPropagation();
    try {
      await navigator.clipboard.writeText(toolName);
      setCopyFeedback(toolName);
      setTimeout(() => setCopyFeedback(null), 2000);
    } catch {
      console.error('Failed to copy to clipboard');
    }
  }, []);

  const handleToggleExpand = useCallback((toolName: string, e: MouseEvent) => {
    e.stopPropagation();
    setExpandedTool(prev => prev === toolName ? null : toolName);
  }, []);

  const handleSelectTool = useCallback((tool: FetchedTool) => {
    onSelect(tool.name, tool.inputSchema);
  }, [onSelect]);

  return (
    <div className="tool-selector" role="region" aria-label="Tool Selection">
      <div className="tool-selector-header">
        <div className="tool-selector-title">
          <Icon name="wrench" size="md" aria-hidden={true} />
          <h3>Available Tools</h3>
          {tools.length > 0 && (
            <span className="tool-count">{tools.length} tools</span>
          )}
        </div>
        <button
          type="button"
          onClick={fetchTools}
          disabled={loading || !targetUrl}
          className="btn btn-secondary btn-sm"
          aria-label={loading ? 'Fetching tools...' : 'Fetch tools from server'}
        >
          <Icon name={loading ? 'loader' : 'refresh'} size="sm" aria-hidden={true} />
          {loading ? 'Fetching...' : 'Fetch Tools'}
        </button>
      </div>

      {error && (
        <div className="tool-selector-error" role="alert">
          <Icon name="alert-triangle" size="sm" aria-hidden={true} />
          <span>{error}</span>
          <button 
            type="button" 
            onClick={fetchTools} 
            className="btn btn-ghost btn-xs"
            aria-label="Retry fetching tools"
          >
            Retry
          </button>
        </div>
      )}

      {tools.length > 0 && (
        <>
          <div className="tool-search">
            <Icon name="search" size="sm" aria-hidden={true} />
            <input
              type="text"
              placeholder="Search tools..."
              value={searchQuery}
              onChange={e => setSearchQuery(e.target.value)}
              className="input"
              aria-label="Search tools"
            />
            {searchQuery && (
              <button
                type="button"
                onClick={() => setSearchQuery('')}
                className="btn btn-ghost btn-xs"
                aria-label="Clear search"
              >
                <Icon name="x" size="sm" aria-hidden={true} />
              </button>
            )}
          </div>

          <div className="tool-filters">
            <div className="filter-chips" role="group" aria-label="Filter tools">
              <button
                type="button"
                className={`filter-chip ${activeFilter === 'all' ? 'active' : ''}`}
                onClick={() => setActiveFilter('all')}
                aria-pressed={activeFilter === 'all'}
              >
                All ({filterCounts.all})
              </button>
              <button
                type="button"
                className={`filter-chip ${activeFilter === 'no-params' ? 'active' : ''}`}
                onClick={() => setActiveFilter('no-params')}
                aria-pressed={activeFilter === 'no-params'}
              >
                No params ({filterCounts.noParams})
              </button>
              <button
                type="button"
                className={`filter-chip ${activeFilter === 'with-params' ? 'active' : ''}`}
                onClick={() => setActiveFilter('with-params')}
                aria-pressed={activeFilter === 'with-params'}
              >
                With params ({filterCounts.withParams})
              </button>
            </div>
            <button
              type="button"
              onClick={handleSelectRandom}
              disabled={filteredTools.length === 0}
              className="btn btn-ghost btn-xs"
              title="Select a random tool from the filtered list"
              aria-label="Select random tool"
            >
              <Icon name="dice" size="sm" aria-hidden={true} />
              Random
            </button>
          </div>

          {(behaviorCounts.readOnly > 0 || behaviorCounts.destructive > 0) && (
            <div className="tool-filters tool-filters-behavior">
              <span className="filter-label">Behavior:</span>
              <div className="filter-chips" role="group" aria-label="Filter by behavior">
                <button
                  type="button"
                  className={`filter-chip ${behaviorFilter === 'all' ? 'active' : ''}`}
                  onClick={() => setBehaviorFilter('all')}
                  aria-pressed={behaviorFilter === 'all'}
                >
                  All
                </button>
                {behaviorCounts.readOnly > 0 && (
                  <button
                    type="button"
                    className={`filter-chip filter-chip-safe ${behaviorFilter === 'read-only' ? 'active' : ''}`}
                    onClick={() => setBehaviorFilter('read-only')}
                    aria-pressed={behaviorFilter === 'read-only'}
                  >
                    <Icon name="shield" size="xs" aria-hidden={true} />
                    Read-only ({behaviorCounts.readOnly})
                  </button>
                )}
                {behaviorCounts.mutable > 0 && (
                  <button
                    type="button"
                    className={`filter-chip ${behaviorFilter === 'mutable' ? 'active' : ''}`}
                    onClick={() => setBehaviorFilter('mutable')}
                    aria-pressed={behaviorFilter === 'mutable'}
                  >
                    Mutable ({behaviorCounts.mutable})
                  </button>
                )}
                {behaviorCounts.destructive > 0 && (
                  <button
                    type="button"
                    className={`filter-chip filter-chip-danger ${behaviorFilter === 'destructive' ? 'active' : ''}`}
                    onClick={() => setBehaviorFilter('destructive')}
                    aria-pressed={behaviorFilter === 'destructive'}
                  >
                    <Icon name="alert-triangle" size="xs" aria-hidden={true} />
                    Destructive ({behaviorCounts.destructive})
                  </button>
                )}
              </div>
            </div>
          )}

          {toolGroups.length > 0 && (
            <div className="tool-filters tool-filters-groups">
              <span className="filter-label">Groups:</span>
              <div className="filter-chips filter-chips-scrollable" role="group" aria-label="Filter by group">
                <button
                  type="button"
                  className={`filter-chip ${selectedGroup === null ? 'active' : ''}`}
                  onClick={() => setSelectedGroup(null)}
                  aria-pressed={selectedGroup === null}
                >
                  All
                </button>
                {toolGroups.map(([group, count]) => (
                  <button
                    key={group}
                    type="button"
                    className={`filter-chip ${selectedGroup === group ? 'active' : ''}`}
                    onClick={() => setSelectedGroup(selectedGroup === group ? null : group)}
                    aria-pressed={selectedGroup === group}
                  >
                    {group} ({count})
                  </button>
                ))}
              </div>
            </div>
          )}

          {selectedTool && (
            <div className={`tool-validation ${isValidSelection ? 'valid' : 'invalid'}`}>
              {isValidSelection ? (
                <>
                  <Icon name="check-circle" size="sm" aria-hidden={true} />
                  <span>Selected: <strong>{selectedTool}</strong></span>
                </>
              ) : (
                <>
                  <Icon name="alert-triangle" size="sm" aria-hidden={true} />
                  <span>Tool "{selectedTool}" not found on server</span>
                </>
              )}
            </div>
          )}

          <div className="tool-list" role="listbox" aria-label="Available tools">
            {filteredTools.length === 0 ? (
              <div className="tool-list-empty">
                <Icon name="search" size="lg" aria-hidden={true} />
                <p>No tools match "{searchQuery}"</p>
              </div>
            ) : (
              filteredTools.map(tool => {
                const isSelected = selectedTool === tool.name;
                const isExpanded = expandedTool === tool.name;
                const hasSchema = tool.inputSchema && Object.keys(tool.inputSchema).length > 0;

                return (
                  <div
                    key={tool.name}
                    className={`tool-item ${isSelected ? 'selected' : ''}`}
                    role="option"
                    aria-selected={isSelected}
                    onClick={() => handleSelectTool(tool)}
                    tabIndex={0}
                    onKeyDown={e => {
                      if (e.key === 'Enter' || e.key === ' ') {
                        e.preventDefault();
                        handleSelectTool(tool);
                      }
                    }}
                  >
                    <div className="tool-item-header">
                      <div className="tool-item-info">
                        {isSelected && (
                          <Icon name="check" size="sm" className="tool-selected-icon" aria-hidden={true} />
                        )}
                        <span className="tool-name">{tool.name}</span>
                        <button
                          type="button"
                          onClick={e => handleCopyToolName(tool.name, e)}
                          className="btn btn-ghost btn-xs tool-copy-btn"
                          aria-label={`Copy tool name: ${tool.name}`}
                        >
                          <Icon 
                            name={copyFeedback === tool.name ? 'check' : 'copy'} 
                            size="xs" 
                            aria-hidden={true} 
                          />
                        </button>
                      </div>
                      {hasSchema && (
                        <button
                          type="button"
                          onClick={e => handleToggleExpand(tool.name, e)}
                          className="btn btn-ghost btn-xs tool-expand-btn"
                          aria-expanded={isExpanded}
                          aria-label={isExpanded ? 'Collapse schema' : 'Expand schema'}
                        >
                          <Icon 
                            name={isExpanded ? 'chevron-up' : 'chevron-down'} 
                            size="sm" 
                            aria-hidden={true} 
                          />
                        </button>
                      )}
                    </div>
                    
                    {tool.description && (
                      <p className="tool-description">{tool.description}</p>
                    )}

                    {isExpanded && hasSchema && (
                      <div className="tool-schema-panel">
                        <div className="tool-schema-header">
                          <Icon name="code" size="sm" aria-hidden={true} />
                          <span>Input Schema</span>
                        </div>
                        <SchemaDisplay schema={tool.inputSchema!} />
                      </div>
                    )}
                  </div>
                );
              })
            )}
          </div>
        </>
      )}

      {tools.length === 0 && !loading && !error && (
        <div className="tool-selector-empty">
          <Icon name="inbox" size="xl" aria-hidden={true} />
          <p>No tools loaded</p>
          <p className="muted">Click "Fetch Tools" to discover available tools from the target server</p>
        </div>
      )}
    </div>
  );
}

export const ToolSelector = memo(ToolSelectorComponent);
