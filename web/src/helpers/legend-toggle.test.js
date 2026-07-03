import { describe, it, expect } from 'vitest';
import { handleLegendToggle } from './legend-toggle';

describe('handleLegendToggle', () => {
  const allUsers = ['zhaoyf', 'huangdc', 'liteng', 'hanmc', 'yanggang', 'huangjl', 'liuxx'];

  it('should select the clicked user when no previous selection exists', () => {
    const result = handleLegendToggle(null, 'huangjl', allUsers);
    expect(result.action).toBe('select');
    expect(result.selectedUsers).toEqual(['huangjl']);
    expect(result.newLastSelected).toBe('huangjl');
  });

  it('should restore all users when clicking the already-selected user again', () => {
    const result = handleLegendToggle('huangjl', 'huangjl', allUsers);
    expect(result.action).toBe('restore');
    expect(result.selectedUsers).toEqual(allUsers);
    expect(result.newLastSelected).toBeNull();
  });

  it('should select the new user when clicking a different user', () => {
    const result = handleLegendToggle('huangjl', 'zhaoyf', allUsers);
    expect(result.action).toBe('select');
    expect(result.selectedUsers).toEqual(['zhaoyf']);
    expect(result.newLastSelected).toBe('zhaoyf');
  });

  it('should handle single-user list correctly', () => {
    const singleUser = ['huangjl'];

    const first = handleLegendToggle(null, 'huangjl', singleUser);
    expect(first.action).toBe('select');
    expect(first.selectedUsers).toEqual(['huangjl']);
    expect(first.newLastSelected).toBe('huangjl');

    const second = handleLegendToggle('huangjl', 'huangjl', singleUser);
    expect(second.action).toBe('restore');
    expect(second.selectedUsers).toEqual(['huangjl']);
    expect(second.newLastSelected).toBeNull();
  });

  it('should select the first user when clicking from restored state', () => {
    const step1 = handleLegendToggle(null, 'huangjl', allUsers);
    expect(step1.action).toBe('select');

    const step2 = handleLegendToggle('huangjl', 'huangjl', allUsers);
    expect(step2.action).toBe('restore');

    const step3 = handleLegendToggle(null, 'liteng', allUsers);
    expect(step3.action).toBe('select');
    expect(step3.selectedUsers).toEqual(['liteng']);
    expect(step3.newLastSelected).toBe('liteng');
  });

  it('should support model names for call trend chart legend toggle', () => {
    const allModels = ['gpt-4', 'gpt-3.5-turbo', 'claude-3-opus'];

    const selectResult = handleLegendToggle(null, 'gpt-4', allModels);
    expect(selectResult.action).toBe('select');
    expect(selectResult.selectedUsers).toEqual(['gpt-4']);

    const deselectResult = handleLegendToggle('gpt-4', 'gpt-4', allModels);
    expect(deselectResult.action).toBe('restore');
    expect(deselectResult.selectedUsers).toEqual(allModels);
    expect(deselectResult.newLastSelected).toBeNull();
  });
});
