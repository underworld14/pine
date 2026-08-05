import { describe, it, expect } from 'vitest';
import { emptyFilter, isActive, matchesFilter, toggle } from './board-filter';
import type { Ticket } from './api';

function mk(extra: Partial<Ticket> = {}): Ticket {
  return {
    id: 'BUG-1', type: 'BUG', title: 'Login is broken', status: 'todo', priority: 'high',
    labels: ['auth', 'ui'], deps: [], created: '', updated: '', blocked: false, hash: 'h', attachments: [],
    ...extra
  };
}

describe('isActive', () => {
  it('is false for an empty filter and true once any constraint is set', () => {
    expect(isActive(emptyFilter())).toBe(false);
    expect(isActive({ text: '  ', labels: [], priorities: [] })).toBe(false);
    expect(isActive({ text: 'x', labels: [], priorities: [] })).toBe(true);
    expect(isActive({ text: '', labels: ['ui'], priorities: [] })).toBe(true);
    expect(isActive({ text: '', labels: [], priorities: ['high'] })).toBe(true);
  });
});

describe('matchesFilter', () => {
  it('matches id, title, or label by case-insensitive substring', () => {
    expect(matchesFilter(mk(), { text: 'login', labels: [], priorities: [] })).toBe(true);
    expect(matchesFilter(mk(), { text: 'BUG-1', labels: [], priorities: [] })).toBe(true);
    expect(matchesFilter(mk(), { text: 'AUTH', labels: [], priorities: [] })).toBe(true);
    expect(matchesFilter(mk(), { text: 'nope', labels: [], priorities: [] })).toBe(false);
  });
  it('requires any selected label', () => {
    expect(matchesFilter(mk(), { text: '', labels: ['ui'], priorities: [] })).toBe(true);
    expect(matchesFilter(mk(), { text: '', labels: ['backend'], priorities: [] })).toBe(false);
  });
  it('requires any selected priority', () => {
    expect(matchesFilter(mk(), { text: '', labels: [], priorities: ['high'] })).toBe(true);
    expect(matchesFilter(mk(), { text: '', labels: [], priorities: ['low'] })).toBe(false);
  });
  it('ANDs constraints together', () => {
    expect(matchesFilter(mk(), { text: 'login', labels: ['ui'], priorities: ['high'] })).toBe(true);
    expect(matchesFilter(mk(), { text: 'login', labels: ['ui'], priorities: ['low'] })).toBe(false);
  });
});

describe('toggle', () => {
  it('adds then removes a value immutably', () => {
    const a = toggle([], 'ui');
    expect(a).toEqual(['ui']);
    expect(toggle(a, 'ui')).toEqual([]);
    expect(a).toEqual(['ui']); // original untouched
  });
});
