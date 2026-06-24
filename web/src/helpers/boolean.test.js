import { describe, it, expect } from 'vitest';
import { toBoolean } from './boolean';

describe('toBoolean', () => {
  it('should return true for boolean true', () => {
    expect(toBoolean(true)).toBe(true);
  });

  it('should return false for boolean false', () => {
    expect(toBoolean(false)).toBe(false);
  });

  it('should return true for string "true"', () => {
    expect(toBoolean('true')).toBe(true);
  });

  it('should return true for string "TRUE"', () => {
    expect(toBoolean('TRUE')).toBe(true);
  });

  it('should return true for string "1"', () => {
    expect(toBoolean('1')).toBe(true);
  });

  it('should return false for string "false"', () => {
    expect(toBoolean('false')).toBe(false);
  });

  it('should return false for string "FALSE"', () => {
    expect(toBoolean('FALSE')).toBe(false);
  });

  it('should return false for string "0"', () => {
    expect(toBoolean('0')).toBe(false);
  });

  it('should return false for empty string', () => {
    expect(toBoolean('')).toBe(false);
  });

  it('should return true for number 1', () => {
    expect(toBoolean(1)).toBe(true);
  });

  it('should return false for number 0', () => {
    expect(toBoolean(0)).toBe(false);
  });

  it('should return false for null', () => {
    expect(toBoolean(null)).toBe(false);
  });

  it('should return false for undefined', () => {
    expect(toBoolean(undefined)).toBe(false);
  });

  it('should handle SmartFormatRoutingEnabled default value', () => {
    // Backend sends boolean toggle values as strings "true" / "false"
    const backendValue = 'true';
    expect(toBoolean(backendValue)).toBe(true);

    const backendDisabled = 'false';
    expect(toBoolean(backendDisabled)).toBe(false);
  });
});
