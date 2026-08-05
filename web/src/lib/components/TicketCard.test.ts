import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import TicketCard from './TicketCard.svelte';
import type { Ticket } from '$lib/api';

function mk(extra: Partial<Ticket> = {}): Ticket {
  return {
    id: 'BUG-1', type: 'BUG', title: 'Login is broken', status: 'todo', priority: 'high',
    labels: ['ui'], deps: [], created: '', updated: '2026-07-04T00:00:00Z',
    blocked: false, hash: 'h', attachments: [], ...extra
  };
}

describe('TicketCard', () => {
  it('renders the id, title, and a label chip', () => {
    render(TicketCard, { ticket: mk() });
    expect(screen.getByText('BUG-1')).toBeTruthy();
    expect(screen.getByText('Login is broken')).toBeTruthy();
    expect(screen.getByText('ui')).toBeTruthy();
  });

  it('invokes onPeek with the ticket when the peek button is clicked', async () => {
    const onPeek = vi.fn();
    render(TicketCard, { ticket: mk(), onPeek });
    await fireEvent.click(screen.getByTestId('peek-BUG-1'));
    expect(onPeek).toHaveBeenCalledWith(expect.objectContaining({ id: 'BUG-1' }), expect.anything());
  });

  it('hides the peek button on read-only (off-branch) cards', () => {
    render(TicketCard, { ticket: mk({ readOnly: true, branch: 'feature/x' }), onPeek: vi.fn() });
    expect(screen.queryByTestId('peek-BUG-1')).toBeNull();
  });
});
