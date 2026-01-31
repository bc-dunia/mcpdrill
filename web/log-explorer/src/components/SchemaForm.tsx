import { memo, useCallback } from 'react';
import type { JSONSchema } from '../types';
import type { ValidationError } from '../hooks/useArgumentValidation';
import { Icon } from './Icon';

interface SchemaFormProps {
  schema: JSONSchema;
  value: Record<string, unknown>;
  onChange: (value: Record<string, unknown>) => void;
  errors: ValidationError[];
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

const SchemaField = memo(function SchemaField({
  name,
  schema,
  value,
  onChange,
  required,
  errors,
  path,
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

export function SchemaForm({ schema, value, onChange, errors }: SchemaFormProps) {
  const requiredFields = new Set(schema.required || []);

  return (
    <div className="schema-form" role="form" aria-label="Arguments form">
      {Object.entries(schema.properties || {}).map(([key, propSchema]) => (
        <SchemaField
          key={key}
          name={key}
          schema={propSchema}
          value={value[key]}
          onChange={newVal => onChange({ ...value, [key]: newVal })}
          required={requiredFields.has(key)}
          errors={errors}
          path={key}
        />
      ))}
    </div>
  );
}
