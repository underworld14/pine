import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import TicketRelations from './TicketRelations.svelte';
import { workspace } from '$lib/workspace.svelte';
import { ui } from '$lib/ui.svelte';
import type { Ticket } from '$lib/api';

function mk(extra: Partial<Ticket> = {}): Ticket {
  return {
    id: 'BUG-1',
    type: 'BUG',
    title: 'Login is broken',
    status: 'todo',
    priority: 'high',
    labels: [],
    deps: [],
    created: '',
    updated: '',
    blocked: false,
    hash: 'h',
    attachments: [],
    ...extra
  };
}

describe('TicketRelations', () => {
  afterEach(() => {
    workspace.tickets = {};
  });

  it('renders Epic, Blocked by, Blocks, and Children sections', () => {
    const epic = mk({ id: 'EPIC-1', type: 'EPIC', title: 'Epic', children: [
      { id: 'FEAT-1', title: 'Child one', status: 'todo' },
      { id: 'FEAT-2', title: 'Child two', status: 'done' }
    ], epicProgress: { done: 1, total: 2 } });
    const feat = mk({
      id: 'FEAT-1',
      type: 'FEAT',
      title: 'Child one',
      parent: 'EPIC-1',
      deps: ['FEAT-0'],
      blocked: true,
      unmet: ['FEAT-0']
    });
    const blocker = mk({ id: 'FEAT-0', type: 'FEAT', title: 'Blocker', status: 'doing' });
    const waiter = mk({ id: 'FEAT-9', type: 'FEAT', title: 'Waits', deps: ['FEAT-1'] });
    workspace.tickets = {
      'EPIC-1': epic,
      'FEAT-0': blocker,
      'FEAT-1': feat,
      'FEAT-2': mk({ id: 'FEAT-2', type: 'FEAT', title: 'Child two', status: 'done', parent: 'EPIC-1' }),
      'FEAT-9': waiter
    };

    render(TicketRelations, { props: { ticket: feat } });
    expect(screen.getByTestId('ticket-relations')).toBeTruthy();
    expect(document.getElementById('rel-epic-h')?.textContent).toBe('Epic');
    expect(screen.getByText('Blocked by')).toBeTruthy();
    expect(screen.getByText('Blocks')).toBeTruthy();
    expect(screen.getByTestId('rel-row-EPIC-1')).toBeTruthy();
    expect(screen.getByTestId('rel-row-FEAT-0')).toBeTruthy();
    expect(screen.getByTestId('rel-row-FEAT-9')).toBeTruthy();
  });

  it('lists all children with progress and invokes onSelectTicket', async () => {
    const kids = Array.from({ length: 8 }, (_, i) =>
      mk({ id: `FEAT-${i}`, type: 'FEAT', title: `T${i}`, parent: 'EPIC-1', status: i === 0 ? 'done' : 'todo' })
    );
    const epic = mk({
      id: 'EPIC-1',
      type: 'EPIC',
      title: 'Big epic',
      children: kids.map((k) => ({ id: k.id, title: k.title, status: k.status })),
      epicProgress: { done: 1, total: 8 }
    });
    workspace.tickets = Object.fromEntries([['EPIC-1', epic], ...kids.map((k) => [k.id, k])]);

    const onSelectTicket = vi.fn();
    render(TicketRelations, { props: { ticket: epic, onSelectTicket } });
    expect(screen.getByText('(1/8 done)')).toBeTruthy();
    expect(screen.getByTestId('rel-row-FEAT-7')).toBeTruthy();
    expect(screen.getByTestId('add-child')).toBeTruthy();
    await fireEvent.click(screen.getByTestId('rel-row-FEAT-3'));
    expect(onSelectTicket).toHaveBeenCalledWith('FEAT-3');
  });

  it('shows Add child on an empty epic and opens the create modal', async () => {
    const epic = mk({ id: 'EPIC-1', type: 'EPIC', title: 'Empty epic' });
    workspace.tickets = { 'EPIC-1': epic };
    const openSpy = vi.spyOn(ui, 'openModal');
    render(TicketRelations, { props: { ticket: epic } });
    expect(screen.getByTestId('add-child')).toBeTruthy();
    expect(screen.getByText('No child tickets yet.')).toBeTruthy();
    await fireEvent.click(screen.getByTestId('add-child'));
    expect(openSpy).toHaveBeenCalledWith({ type: 'feature', parent: 'EPIC-1' });
  });

  it('renders nothing when the ticket has no relationships', () => {
    const t = mk();
    workspace.tickets = { 'BUG-1': t };
    render(TicketRelations, { props: { ticket: t } });
    expect(screen.queryByTestId('ticket-relations')).toBeNull();
  });
});
