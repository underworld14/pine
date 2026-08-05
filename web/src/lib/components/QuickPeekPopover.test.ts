import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import QuickPeekPopover from './QuickPeekPopover.svelte';
import { workspace } from '$lib/workspace.svelte';
import type { Ticket } from '$lib/api';

function mk(extra: Partial<Ticket> = {}): Ticket {
  return {
    id: 'BUG-1', type: 'BUG', title: 'Login is broken', status: 'todo', priority: 'high',
    labels: [], deps: [], created: '', updated: '', blocked: false, hash: 'h', attachments: [], ...extra
  };
}

describe('QuickPeekPopover', () => {
  afterEach(() => vi.restoreAllMocks());

  it('renders the ticket summary and status options from the board', () => {
    workspace.tickets = { 'BUG-1': mk() };
    workspace.board = { columns: [{ status: 'todo', title: 'Todo' }, { status: 'doing', title: 'Doing' }], unmapped: [] };
    render(QuickPeekPopover, { props: { ticket: mk(), anchor: null, onClose: vi.fn() } });
    expect(screen.getByTestId('quick-peek')).toBeTruthy();
    expect(screen.getByText('BUG-1')).toBeTruthy();
    const select = screen.getByTestId('quick-peek-status') as HTMLSelectElement;
    expect(Array.from(select.options).map((o) => o.value)).toEqual(['todo', 'doing']);
  });

  it('patches status when the select changes', async () => {
    workspace.tickets = { 'BUG-1': mk() };
    workspace.board = { columns: [{ status: 'todo', title: 'Todo' }, { status: 'doing', title: 'Doing' }], unmapped: [] };
    const spy = vi.spyOn(workspace, 'patch').mockResolvedValue(mk({ status: 'doing' }));
    render(QuickPeekPopover, { props: { ticket: mk(), anchor: null, onClose: vi.fn() } });
    await fireEvent.change(screen.getByTestId('quick-peek-status'), { target: { value: 'doing' } });
    expect(spy).toHaveBeenCalledWith('BUG-1', { status: 'doing' });
  });

  it('closes on Escape', async () => {
    workspace.tickets = { 'BUG-1': mk() };
    workspace.board = { columns: [{ status: 'todo', title: 'Todo' }], unmapped: [] };
    const onClose = vi.fn();
    render(QuickPeekPopover, { props: { ticket: mk(), anchor: null, onClose } });
    await fireEvent.keyDown(screen.getByTestId('quick-peek'), { key: 'Escape' });
    expect(onClose).toHaveBeenCalled();
  });
});
