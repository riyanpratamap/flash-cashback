export function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}

export const isStr = (value: unknown): value is string => typeof value === 'string';

export const isInt = (value: unknown): value is number => typeof value === 'number' && Number.isInteger(value);
