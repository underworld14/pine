import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import TicketPeekSheet from './TicketPeekSheet.svelte';
import { workspace } from '$lib/workspace.svelte';
import { api } from '$lib/api';
import type { Ticket } from '$lib/api';

function mk(extra: Partial<Ticket> = {}): Ticket {
  return {
    id: 'BUG-1',
    type: 'BUG',
    title: 'Login is broken',
    status: 'todo',
    priority: 'high',
    labels: ['auth'],
    deps: [],
    created: '',
    updated: '',
    blocked: false,
    hash: 'h',
    attachments: [],
    body: 'Fix the login form.',
    ...extra
  };
}

describe('TicketPeekSheet', () => {
  afterEach(() => {
    vi.restoreAllMocks();
    workspace.tickets = {};
  });

  it('renders ticket details and closes on Escape', async () => {
    workspace.tickets = { 'BUG-1': mk() };
    workspace.board = { columns: [{ status: 'todo', title: 'Todo' }, { status: 'doing', title: 'Doing' }], unmapped: [] };
    const onClose = vi.fn();
    render(TicketPeekSheet, { props: { ticketId: 'BUG-1', onClose } });
    expect(screen.getByTestId('ticket-peek-sheet')).toBeTruthy();
    expect((screen.getByTestId('peek-title') as HTMLInputElement).value).toBe('Login is broken');
    expect(screen.getByTestId('peek-open').getAttribute('href')).toBe('/tickets/BUG-1');
    expect(screen.getByTestId('peek-body-preview').textContent).toContain('Fix the login form');
    const sheet = screen.getByTestId('ticket-peek-sheet');
    const ev = new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true });
    const stopSpy = vi.spyOn(ev, 'stopPropagation');
    sheet.dispatchEvent(ev);
    await vi.waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(stopSpy).toHaveBeenCalled();
  });

  it('patches priority from the shared segment', async () => {
    workspace.tickets = { 'BUG-1': mk() };
    workspace.board = { columns: [{ status: 'todo', title: 'Todo' }], unmapped: [] };
    const spy = vi.spyOn(workspace, 'patch').mockResolvedValue(mk({ priority: 'critical' }));
    render(TicketPeekSheet, { props: { ticketId: 'BUG-1', onClose: vi.fn() } });
    expect(screen.getByTestId('peek-prio-critical').textContent).toContain('Crit');
    await fireEvent.click(screen.getByTestId('peek-prio-critical'));
    expect(spy).toHaveBeenCalledWith('BUG-1', { priority: 'critical' });
  });

  it('patches status from the sheet', async () => {
    workspace.tickets = { 'BUG-1': mk() };
    workspace.board = { columns: [{ status: 'todo', title: 'Todo' }, { status: 'doing', title: 'Doing' }], unmapped: [] };
    const spy = vi.spyOn(workspace, 'patch').mockResolvedValue(mk({ status: 'doing' }));
    render(TicketPeekSheet, { props: { ticketId: 'BUG-1', onClose: vi.fn() } });
    await fireEvent.change(screen.getByTestId('peek-status'), { target: { value: 'doing' } });
    expect(spy).toHaveBeenCalledWith('BUG-1', { status: 'doing' });
  });

  it('patches title on blur', async () => {
    workspace.tickets = { 'BUG-1': mk() };
    workspace.board = { columns: [{ status: 'todo', title: 'Todo' }], unmapped: [] };
    const spy = vi.spyOn(workspace, 'patch').mockResolvedValue(mk({ title: 'New title' }));
    render(TicketPeekSheet, { props: { ticketId: 'BUG-1', onClose: vi.fn() } });
    const title = screen.getByTestId('peek-title') as HTMLInputElement;
    await fireEvent.input(title, { target: { value: 'New title' } });
    await fireEvent.blur(title);
    expect(spy).toHaveBeenCalledWith('BUG-1', { title: 'New title' });
  });

  it('adds and removes labels', async () => {
    workspace.tickets = { 'BUG-1': mk({ labels: ['auth'] }) };
    workspace.board = { columns: [{ status: 'todo', title: 'Todo' }], unmapped: [] };
    const spy = vi.spyOn(workspace, 'patch').mockResolvedValue(mk());
    render(TicketPeekSheet, { props: { ticketId: 'BUG-1', onClose: vi.fn() } });
    const input = screen.getByTestId('peek-label-input');
    await fireEvent.input(input, { target: { value: 'urgent' } });
    await fireEvent.keyDown(input, { key: 'Enter' });
    expect(spy).toHaveBeenCalledWith('BUG-1', { labels: ['auth', 'urgent'] });

    await fireEvent.click(screen.getByTestId('peek-label-rm-auth'));
    expect(spy).toHaveBeenCalledWith('BUG-1', { labels: [] });
  });

  it('toggles body edit and saves with ⌘S', async () => {
    workspace.tickets = { 'BUG-1': mk() };
    workspace.board = { columns: [{ status: 'todo', title: 'Todo' }], unmapped: [] };
    const patchSpy = vi.spyOn(api, 'patchTicket').mockResolvedValue(mk({ body: 'Updated body', hash: 'h2' }));
    vi.spyOn(workspace, 'beginOp').mockReturnValue('op1');
    render(TicketPeekSheet, { props: { ticketId: 'BUG-1', onClose: vi.fn() } });

    await fireEvent.click(screen.getByTestId('peek-edit-body'));
    const ta = screen.getByTestId('peek-body-edit') as HTMLTextAreaElement;
    expect(ta.value).toContain('Fix the login form');
    await fireEvent.input(ta, { target: { value: 'Updated body' } });

    const sheet = screen.getByTestId('ticket-peek-sheet');
    await fireEvent.keyDown(sheet, { key: 's', metaKey: true });
    expect(patchSpy).toHaveBeenCalledWith('BUG-1', { body: 'Updated body', opId: 'op1' }, 'h');
  });

  it('Escape finishes body edit before closing', async () => {
    workspace.tickets = { 'BUG-1': mk() };
    workspace.board = { columns: [{ status: 'todo', title: 'Todo' }], unmapped: [] };
    vi.spyOn(api, 'patchTicket').mockResolvedValue(mk({ hash: 'h2' }));
    vi.spyOn(workspace, 'beginOp').mockReturnValue('op1');
    const onClose = vi.fn();
    render(TicketPeekSheet, { props: { ticketId: 'BUG-1', onClose } });

    await fireEvent.click(screen.getByTestId('peek-edit-body'));
    expect(screen.getByTestId('peek-body-edit')).toBeTruthy();

    const sheet = screen.getByTestId('ticket-peek-sheet');
    await fireEvent.keyDown(sheet, { key: 'Escape' });
    expect(onClose).not.toHaveBeenCalled();
    expect(screen.getByTestId('peek-body-preview')).toBeTruthy();

    await fireEvent.keyDown(sheet, { key: 'Escape' });
    expect(onClose).toHaveBeenCalled();
  });

  it('flushes dirty body before Close', async () => {
    workspace.tickets = { 'BUG-1': mk() };
    workspace.board = { columns: [{ status: 'todo', title: 'Todo' }], unmapped: [] };
    const patchSpy = vi.spyOn(api, 'patchTicket').mockResolvedValue(mk({ body: 'Updated body', hash: 'h2' }));
    vi.spyOn(workspace, 'beginOp').mockReturnValue('op1');
    const onClose = vi.fn();
    render(TicketPeekSheet, { props: { ticketId: 'BUG-1', onClose } });

    await fireEvent.click(screen.getByTestId('peek-edit-body'));
    const ta = screen.getByTestId('peek-body-edit') as HTMLTextAreaElement;
    await fireEvent.input(ta, { target: { value: 'Updated body' } });

    await fireEvent.click(screen.getByTestId('peek-close'));
    await vi.waitFor(() => {
      expect(patchSpy).toHaveBeenCalledWith('BUG-1', { body: 'Updated body', opId: 'op1' }, 'h');
      expect(onClose).toHaveBeenCalled();
    });
  });

  it('disables edits when readOnly', async () => {
    workspace.tickets = {
      'BUG-1': mk({ readOnly: true, branch: 'feature/x', labels: ['auth'] })
    };
    workspace.board = { columns: [{ status: 'todo', title: 'Todo' }], unmapped: [] };
    const spy = vi.spyOn(workspace, 'patch');
    render(TicketPeekSheet, { props: { ticketId: 'BUG-1', onClose: vi.fn() } });

    expect(screen.getByTestId('peek-ro-banner')).toBeTruthy();
    expect((screen.getByTestId('peek-title') as HTMLInputElement).readOnly).toBe(true);
    expect((screen.getByTestId('peek-status') as HTMLSelectElement).disabled).toBe(true);
    expect(screen.queryByTestId('peek-edit-body')).toBeNull();
    expect(screen.queryByTestId('peek-label-input')).toBeNull();

    await fireEvent.click(screen.getByTestId('peek-prio-low'));
    expect(spy).not.toHaveBeenCalled();
  });

  it('lists attachments and can remove them', async () => {
    workspace.tickets = {
      'BUG-1': mk({
        attachments: [
          { name: 'shot.png', size: 1200, mime: 'image/png', kind: 'image', url: '/att/shot.png' }
        ]
      })
    };
    workspace.board = { columns: [{ status: 'todo', title: 'Todo' }], unmapped: [] };
    const delSpy = vi.spyOn(api, 'deleteAttachment').mockResolvedValue(undefined as never);
    vi.spyOn(workspace, 'beginOp').mockReturnValue('op1');
    render(TicketPeekSheet, { props: { ticketId: 'BUG-1', onClose: vi.fn() } });

    expect(screen.getByTestId('peek-attachments').textContent).toContain('shot.png');
    await fireEvent.click(screen.getByTestId('peek-att-del-shot.png'));
    expect(delSpy).toHaveBeenCalledWith('BUG-1', 'shot.png', 'op1');
  });
});
