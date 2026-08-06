import { describe, it, expect } from 'vitest';
import { priorityMeta, PRIORITIES } from './ui-helpers';

describe('priorityMeta', () => {
  it('exposes readable short labels for every priority', () => {
    expect(PRIORITIES).toEqual(['low', 'medium', 'high', 'critical']);
    expect(priorityMeta('low').short).toBe('Low');
    expect(priorityMeta('medium').short).toBe('Med');
    expect(priorityMeta('high').short).toBe('High');
    expect(priorityMeta('critical').short).toBe('Crit');
    expect(priorityMeta('critical').label).toBe('Critical');
    expect(priorityMeta('critical').color).toContain('danger');
  });
});
