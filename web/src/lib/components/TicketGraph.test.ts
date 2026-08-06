import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import TicketGraph from './TicketGraph.svelte';
import { workspace } from '$lib/workspace.svelte';
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

describe('TicketGraph', () => {
  afterEach(() => {
    workspace.tickets = {};
  });

  it('invokes onSelectTicket when a neighbor node is clicked', async () => {
    const parent = mk({ id: 'EPIC-1', type: 'EPIC', title: 'Epic' });
    const child = mk({ id: 'FEAT-1', type: 'FEAT', title: 'Feat', parent: 'EPIC-1' });
    workspace.tickets = { 'EPIC-1': parent, 'FEAT-1': child };
    const onSelectTicket = vi.fn();
    render(TicketGraph, { props: { ticket: child, onSelectTicket } });
    const link = document.querySelector('svg a[href="/tickets/EPIC-1"]') as HTMLAnchorElement;
    expect(link).toBeTruthy();
    await fireEvent.click(link);
    expect(onSelectTicket).toHaveBeenCalledWith('EPIC-1');
  });

  it('invokes onOverflow for truncated children', async () => {
    const kids = Array.from({ length: 8 }, (_, i) =>
      mk({ id: `FEAT-${i}`, type: 'FEAT', title: `T${i}`, parent: 'EPIC-1' })
    );
    const epic = mk({
      id: 'EPIC-1',
      type: 'EPIC',
      title: 'Epic',
      children: kids.map((k) => ({ id: k.id, title: k.title, status: k.status }))
    });
    workspace.tickets = Object.fromEntries([['EPIC-1', epic], ...kids.map((k) => [k.id, k])]);
    const onOverflow = vi.fn();
    render(TicketGraph, { props: { ticket: epic, onOverflow } });
    const more = screen.getByText('+2 more');
    await fireEvent.click(more);
    expect(onOverflow).toHaveBeenCalledWith('children');
  });
});
