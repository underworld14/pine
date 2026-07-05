import { describe, it, expect } from 'vitest';
import { reconcileEditor } from './ticket-editor';
import type { Ticket } from './api';

function mk(body: string, hash: string): Ticket {
  return {
    id: 'BUG-001',
    type: 'BUG',
    title: 'x',
    status: 'todo',
    priority: 'medium',
    labels: [],
    deps: [],
    created: '2026-07-04T00:00:00Z',
    updated: '2026-07-04T00:00:00Z',
    blocked: false,
    hash,
    body,
    attachments: []
  };
}

describe('reconcileEditor', () => {
  it('no-ops when hash unchanged', () => {
    const r = reconcileEditor({
      text: 'draft',
      baseBody: 'old',
      baseHash: 'h1',
      ticket: mk('disk', 'h1')
    });
    expect(r).toEqual({ text: 'draft', baseBody: 'old', baseHash: 'h1', conflict: null });
  });

  it('silently adopts disk when editor is clean', () => {
    const r = reconcileEditor({
      text: 'old',
      baseBody: 'old',
      baseHash: 'h1',
      ticket: mk('new body', 'h2')
    });
    expect(r.text).toBe('new body');
    expect(r.baseBody).toBe('new body');
    expect(r.baseHash).toBe('h2');
    expect(r.conflict).toBeNull();
  });

  it('keeps draft when only metadata changed on disk', () => {
    const r = reconcileEditor({
      text: 'unsaved draft',
      baseBody: 'old',
      baseHash: 'h1',
      ticket: mk('old', 'h2')
    });
    expect(r.text).toBe('unsaved draft');
    expect(r.baseBody).toBe('old');
    expect(r.baseHash).toBe('h2');
    expect(r.conflict).toBeNull();
  });

  it('flags conflict when disk body changed while editor is dirty', () => {
    const ticket = mk('agent rewrite', 'h2');
    const r = reconcileEditor({
      text: 'unsaved draft',
      baseBody: 'old',
      baseHash: 'h1',
      ticket
    });
    expect(r.text).toBe('unsaved draft');
    expect(r.conflict).toBe(ticket);
  });
});
