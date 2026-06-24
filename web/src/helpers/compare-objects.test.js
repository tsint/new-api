import { describe, it, expect } from 'vitest';

// compareObjects is used by SettingsGeneral to detect changes before saving
// via PUT /api/option/. Re-implementing here to test the logic in isolation.
function compareObjects(oldObject, newObject) {
  const changedProperties = [];
  for (const key in oldObject) {
    if (oldObject.hasOwnProperty(key) && newObject.hasOwnProperty(key)) {
      if (oldObject[key] !== newObject[key]) {
        changedProperties.push({
          key: key,
          oldValue: oldObject[key],
          newValue: newObject[key],
        });
      }
    }
  }
  return changedProperties;
}

describe('compareObjects', () => {
  it('should detect boolean toggle changes (SmartFormatRoutingEnabled)', () => {
    const oldObj = { SmartFormatRoutingEnabled: true };
    const newObj = { SmartFormatRoutingEnabled: false };
    const result = compareObjects(oldObj, newObj);

    expect(result).toHaveLength(1);
    expect(result[0].key).toBe('SmartFormatRoutingEnabled');
    expect(result[0].oldValue).toBe(true);
    expect(result[0].newValue).toBe(false);
  });

  it('should return empty when no changes', () => {
    const obj = {
      SmartFormatRoutingEnabled: true,
      DisplayTokenStatEnabled: false,
    };
    const result = compareObjects(obj, obj);
    expect(result).toHaveLength(0);
  });

  it('should detect string value changes', () => {
    const oldObj = { TopUpLink: 'https://old.com' };
    const newObj = { TopUpLink: 'https://new.com' };
    const result = compareObjects(oldObj, newObj);

    expect(result).toHaveLength(1);
    expect(result[0].key).toBe('TopUpLink');
    expect(result[0].oldValue).toBe('https://old.com');
    expect(result[0].newValue).toBe('https://new.com');
  });

  it('should ignore keys not present in both objects', () => {
    const oldObj = { SmartFormatRoutingEnabled: true, ExtraKey: 'value' };
    const newObj = { SmartFormatRoutingEnabled: true };
    const result = compareObjects(oldObj, newObj);

    expect(result).toHaveLength(0);
  });

  it('should detect multiple changed properties', () => {
    const oldObj = {
      SmartFormatRoutingEnabled: true,
      DisplayTokenStatEnabled: false,
      RetryTimes: '3',
    };
    const newObj = {
      SmartFormatRoutingEnabled: false,
      DisplayTokenStatEnabled: true,
      RetryTimes: '5',
    };
    const result = compareObjects(oldObj, newObj);

    expect(result).toHaveLength(3);
    const keys = result.map((r) => r.key);
    expect(keys).toContain('SmartFormatRoutingEnabled');
    expect(keys).toContain('DisplayTokenStatEnabled');
    expect(keys).toContain('RetryTimes');
  });
});
