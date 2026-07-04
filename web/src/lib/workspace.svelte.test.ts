import { describe, it, expect } from 'vitest';
import { flushSync } from 'svelte';
import { workspace } from './workspace.svelte';
import type { Ticket } from './api';

function mk(id: string, status: string, extra: Partial<Ticket> = {}): Ticket {
  return {
    id, type: id.split('-')[0], title: id, status, priority: 'medium',
    labels: [], deps: [], created: '2026-07-04T00:00:00Z', updated: '2026-07-04T00:00:00Z',
    blocked: false, hash: 'h-' + id, attachments: [], ...extra
  };
}

function withEffects(fn: () => void) {
  const cleanup = $effect.root(() => { fn(); });
  cleanup();
}

describe('workspace columns', () => {
  it('places tickets by frontmatter status and surfaces unmapped ones', () => {
    withEffects(() => {
      workspace.board = { columns: [{ status: 'todo', title: 'Todo' }, { status: 'done', title: 'Done' }], unmapped: [] };
      workspace.tickets = { 'BUG-1': mk('BUG-1', 'todo'), 'BUG-2': mk('BUG-2', 'weird') };
      flushSync();
      const cols = workspace.columns;
      const todo = cols.find((c) => c.status === 'todo');
      expect(todo?.tickets.map((t) => t.id)).toContain('BUG-1');
      const other = cols.find((c) => c.title.startsWith('Other'));
      expect(other?.tickets.map((t) => t.id)).toContain('BUG-2');
    });
  });

  it('sorts a column by priority then recency', () => {
    withEffects(() => {
      workspace.board = { columns: [{ status: 'todo', title: 'Todo' }], unmapped: [] };
      workspace.tickets = {
        a: mk('a', 'todo', { priority: 'low', updated: '2026-07-04T10:00:00Z' }),
        b: mk('b', 'todo', { priority: 'critical', updated: '2026-07-04T09:00:00Z' })
      };
      flushSync();
      const todo = workspace.columns[0];
      expect(todo.tickets[0].id).toBe('b'); // critical outranks low
    });
  });
});

describe('applyEvent', () => {
  it('applies an external ticket update and flashes it', () => {
    withEffects(() => {
      workspace.tickets = {};
      workspace.applyEvent({ type: 'ticket.updated', seq: 1, origin: { source: 'fs' }, ticket: mk('BUG-9', 'doing') });
      flushSync();
      expect(workspace.tickets['BUG-9'].status).toBe('doing');
      expect(workspace.flashing['BUG-9']).toBeGreaterThan(0);
    });
  });

  it('removes a deleted ticket', () => {
    withEffects(() => {
      workspace.tickets = { 'BUG-9': mk('BUG-9', 'todo') };
      workspace.applyEvent({ type: 'ticket.deleted', seq: 2, origin: { source: 'fs' }, id: 'BUG-9' });
      flushSync();
      expect(workspace.tickets['BUG-9']).toBeUndefined();
    });
  });
});
