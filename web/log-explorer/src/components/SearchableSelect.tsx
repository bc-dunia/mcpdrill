import { useState, useRef, useEffect, memo } from 'react';

interface Option {
  value: string;
  label: string;
  sublabel?: string;
}

interface Props {
  options: Option[];
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  disabled?: boolean;
  id?: string;
}

export const SearchableSelect = memo(function SearchableSelect({
  options,
  value,
  onChange,
  placeholder = 'Select...',
  disabled = false,
  id,
}: Props) {
  const [isOpen, setIsOpen] = useState(false);
  const [search, setSearch] = useState('');
  const [highlightedIndex, setHighlightedIndex] = useState(-1);
  const inputRef = useRef<HTMLInputElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const listRef = useRef<HTMLUListElement>(null);
  
  const filtered = (options || []).filter(opt => 
    opt?.label?.toLowerCase().includes(search.toLowerCase()) ||
    opt?.sublabel?.toLowerCase().includes(search.toLowerCase())
  );
  
  const selected = (options || []).find(opt => opt?.value === value);
  
  useEffect(() => {
    if (isOpen && inputRef.current) {
      inputRef.current.focus();
      setHighlightedIndex(-1);
    }
  }, [isOpen]);

  useEffect(() => {
    setHighlightedIndex(-1);
  }, [search]);
  
  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setIsOpen(false);
        setSearch('');
      }
    };
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, []);

  useEffect(() => {
    if (highlightedIndex >= 0 && listRef.current) {
      const item = listRef.current.children[highlightedIndex] as HTMLElement;
      item?.scrollIntoView({ block: 'nearest' });
    }
  }, [highlightedIndex]);

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Escape') {
      setIsOpen(false);
      setSearch('');
      return;
    }
    
    if (!isOpen) {
      if (e.key === 'ArrowDown' || e.key === 'Enter' || e.key === ' ') {
        e.preventDefault();
        setIsOpen(true);
      }
      return;
    }

    if (e.key === 'ArrowDown') {
      e.preventDefault();
      setHighlightedIndex(prev => 
        prev < filtered.length - 1 ? prev + 1 : 0
      );
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setHighlightedIndex(prev => 
        prev > 0 ? prev - 1 : filtered.length - 1
      );
    } else if (e.key === 'Enter' && highlightedIndex >= 0) {
      e.preventDefault();
      const opt = filtered[highlightedIndex];
      if (opt) {
        onChange(opt.value);
        setIsOpen(false);
        setSearch('');
      }
    }
  };
  
  return (
    <div 
      ref={containerRef}
      className={`searchable-select ${isOpen ? 'is-open' : ''} ${disabled ? 'is-disabled' : ''}`}
      onKeyDown={handleKeyDown}
    >
      <button
        type="button"
        className="searchable-select-trigger"
        onClick={() => !disabled && setIsOpen(!isOpen)}
        disabled={disabled}
        aria-expanded={isOpen}
        aria-haspopup="listbox"
        id={id}
      >
        <span className="searchable-select-value">
          {selected ? selected.label : placeholder}
        </span>
        <span className="searchable-select-arrow" aria-hidden="true">▼</span>
      </button>
      
      {isOpen && (
        <div className="searchable-select-dropdown">
          <input
            ref={inputRef}
            type="text"
            className="searchable-select-search"
            placeholder="Type to search..."
            value={search}
            onChange={e => setSearch(e.target.value)}
            aria-label="Search options"
          />
          <ul 
            ref={listRef}
            className="searchable-select-list"
            role="listbox"
            aria-activedescendant={highlightedIndex >= 0 ? `option-${filtered[highlightedIndex]?.value}` : undefined}
          >
            {filtered.length === 0 ? (
              <li className="searchable-select-empty">No results found</li>
            ) : (
              filtered.map((opt, index) => (
                <li
                  key={opt.value}
                  id={`option-${opt.value}`}
                  role="option"
                  aria-selected={opt.value === value}
                  className={`searchable-select-option ${opt.value === value ? 'is-selected' : ''} ${index === highlightedIndex ? 'is-highlighted' : ''}`}
                  onClick={() => {
                    onChange(opt.value);
                    setIsOpen(false);
                    setSearch('');
                  }}
                  onMouseEnter={() => setHighlightedIndex(index)}
                >
                  <span className="option-label">{opt.label}</span>
                  {opt.sublabel && <span className="option-sublabel">{opt.sublabel}</span>}
                </li>
              ))
            )}
          </ul>
        </div>
      )}
    </div>
  );
});
