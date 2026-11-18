import { describe, it, expect, vi } from 'vitest';
import { emptyFilter, isActive, isTextOnlyChange, matchesFilter, toggle } from './board-filter';
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

describe('matchesFilter haystack cache', () => {
  it('serves repeated keystrokes from one cached string per ticket', () => {
    const t = mk();
    const join = vi.spyOn(t.labels, 'join');
    for (const text of ['l', 'lo', 'log', 'logi', 'login']) {
      expect(matchesFilter(t, { text, labels: [], priorities: [] })).toBe(true);
    }
    expect(join).toHaveBeenCalledTimes(1);
  });

  it('is invalidated when the title or labels are replaced', () => {
    const t = mk({ title: 'Login' });
    expect(matchesFilter(t, { text: 'login', labels: [], priorities: [] })).toBe(true);
    t.title = 'Signup';
    expect(matchesFilter(t, { text: 'login', labels: [], priorities: [] })).toBe(false);
    expect(matchesFilter(t, { text: 'signup', labels: [], priorities: [] })).toBe(true);
    t.labels = ['billing'];
    expect(matchesFilter(t, { text: 'billing', labels: [], priorities: [] })).toBe(true);
  });

  it('still matches case-insensitively across id, title and labels', () => {
    const t = mk({ id: 'FEAT-AbC', title: 'Hello World', labels: ['Mixed-Case', 'two'] });
    for (const text of ['feat-abc', 'HELLO world', 'mixed-case', 'world mixed-case two']) {
      expect(matchesFilter(t, { text, labels: [], priorities: [] })).toBe(true);
    }
    expect(matchesFilter(t, { text: 'world feat', labels: [], priorities: [] })).toBe(false);
  });
});

describe('isTextOnlyChange', () => {
  const base = { text: '', labels: ['ui'], priorities: ['high'] };

  it('is true for a typing edit', () => {
    expect(isTextOnlyChange(base, { ...base, text: 'l' })).toBe(true);
  });
  it('is false once labels or priorities differ (those apply immediately)', () => {
    expect(isTextOnlyChange(base, { ...base, text: 'l', labels: [] })).toBe(false);
    expect(isTextOnlyChange(base, { ...base, priorities: ['low'] })).toBe(false);
    expect(isTextOnlyChange(base, emptyFilter())).toBe(false);
  });
  it('is false when nothing changed', () => {
    expect(isTextOnlyChange(base, { ...base })).toBe(false);
  });
});
