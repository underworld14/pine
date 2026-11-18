import { describe, it, expect, vi, afterEach } from 'vitest';
import { flushSync } from 'svelte';
import { render, screen } from '@testing-library/svelte';
import CommandPalette from './CommandPalette.svelte';
import { workspace } from '$lib/workspace.svelte';
import { ui } from '$lib/ui.svelte';
import type { Ticket } from '$lib/api';

vi.mock('$app/navigation', () => ({ goto: vi.fn() }));

function mk(id: string): Ticket {
  return {
    id, type: 'BUG', title: `Ticket ${id}`, status: 'todo', priority: 'medium', labels: [], deps: [],
    created: '', updated: '2026-07-04T00:00:00Z', blocked: false, hash: 'h-' + id, attachments: []
  };
}

describe('CommandPalette', () => {
  afterEach(() => {
    ui.paletteOpen = false;
    workspace.tickets = {};
    vi.restoreAllMocks();
  });

  it('does not scan or sort tickets while closed, however often they change', () => {
    const list = vi.spyOn(Object.getPrototypeOf(workspace), 'list', 'get');
    render(CommandPalette);
    flushSync();
    list.mockClear();
    for (let i = 0; i < 5; i++) {
      workspace.tickets = { ...workspace.tickets, [`BUG-${i}`]: mk(`BUG-${i}`) };
      flushSync();
    }
    expect(list).not.toHaveBeenCalled();
  });

  it('computes and shows matching tickets once opened', () => {
    workspace.tickets = { 'BUG-7': mk('BUG-7') };
    render(CommandPalette);
    expect(screen.queryByRole('dialog')).toBeNull();
    ui.paletteOpen = true;
    flushSync();
    expect(screen.getByRole('dialog')).toBeTruthy();
    expect(screen.getByText('BUG-7 · Ticket BUG-7')).toBeTruthy();
  });
});
