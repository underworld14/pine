import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import { readable } from 'svelte/store';
import { workspace } from '$lib/workspace.svelte';
import { api, type Ticket } from '$lib/api';
import Page from './+page.svelte';

vi.mock('$app/stores', () => ({ page: readable({ params: { id: 'BUG-1' } }) }));
vi.mock('$app/navigation', () => ({ goto: vi.fn() }));

function mk(extra: Partial<Ticket> = {}): Ticket {
  return {
    id: 'BUG-1', type: 'BUG', title: 'Login is broken', status: 'todo', priority: 'high',
    labels: [], deps: [], created: '', updated: '', blocked: false, hash: 'h', attachments: [],
    body: 'Real description', ...extra
  };
}

// The detail page seeds its editor buffer from the ticket body. Snapshots omit
// bodies, so it must wait for the lazy load — seeding from `body ?? ''` would let
// an autosave / ⌘S / Done overwrite the real description with an empty one.
// (Only the loading phase is covered here: happy-dom can't mount the page's
// editor header — the unmodified page fails the same way — so seeding from the
// arrived body is covered by TicketPeekSheet.test.ts and e2e/board-scale.spec.ts.)
describe('ticket detail page with a lazily-loaded body', () => {
  afterEach(() => {
    vi.restoreAllMocks();
    workspace.tickets = {};
  });

  function setup(t: Ticket) {
    workspace.tickets = { 'BUG-1': t };
    workspace.board = { columns: [{ status: 'todo', title: 'Todo' }], unmapped: [] };
  }

  it('shows a loading state and requests the body', async () => {
    const t = mk();
    delete t.body;
    setup(t);
    const ensure = vi.spyOn(workspace, 'ensureBody').mockReturnValue(new Promise(() => {}));
    render(Page);
    expect(await screen.findByTestId('body-loading')).toBeTruthy();
    expect(ensure).toHaveBeenCalledWith('BUG-1');
  });

  it('never saves an unloaded body: ⌘S, ⌘E and Escape stay inert while loading', async () => {
    const t = mk();
    delete t.body;
    setup(t);
    vi.spyOn(workspace, 'ensureBody').mockReturnValue(new Promise(() => {}));
    const patch = vi.spyOn(api, 'patchTicket');
    render(Page);
    await screen.findByTestId('body-loading');

    await fireEvent.keyDown(window, { key: 's', metaKey: true });
    await fireEvent.keyDown(window, { key: 'e', metaKey: true });
    await fireEvent.keyDown(window, { key: 'Escape' });
    await fireEvent.pointerDown(document.body);
    expect(patch).not.toHaveBeenCalled();
    expect(document.querySelector('textarea')).toBeNull(); // no edit surface to type into
    expect(screen.getByTestId('body-loading')).toBeTruthy();
  });

  it('shows a retry when the load fails and does not save', async () => {
    const t = mk();
    delete t.body;
    setup(t);
    const ensure = vi.spyOn(workspace, 'ensureBody').mockRejectedValueOnce(new Error('boom'));
    const patch = vi.spyOn(api, 'patchTicket');
    render(Page);
    const box = await screen.findByTestId('body-loading');
    await vi.waitFor(() => expect(box.textContent).toContain("Couldn't load"));
    ensure.mockReturnValue(new Promise(() => {}));
    await fireEvent.click(box.querySelector('button')!);
    expect(ensure).toHaveBeenCalledTimes(2);
    await fireEvent.keyDown(window, { key: 's', metaKey: true });
    expect(patch).not.toHaveBeenCalled();
  });
});
