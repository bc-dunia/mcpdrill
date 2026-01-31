import { useMemo } from 'react';
import type { JSONSchema } from '../types';

export interface ValidationError {
  path: string;
  message: string;
}

export function validateValue(schema: JSONSchema, value: unknown, path: string): ValidationError[] {
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

export function validateObject(schema: JSONSchema, value: Record<string, unknown>): ValidationError[] {
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

export function useArgumentValidation(schema: JSONSchema | undefined, value: Record<string, unknown>) {
  const validationErrors = useMemo(() => {
    if (!schema) return [];
    return validateObject(schema, value);
  }, [schema, value]);

  return {
    validationErrors,
    isValid: validationErrors.length === 0,
  };
}
