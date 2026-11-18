import { describe, it, expect, vi, afterEach } from 'vitest';
import { flushSync } from 'svelte';
import { render, screen, fireEvent } from '@testing-library/svelte';
import { workspace } from '$lib/workspace.svelte';
import type { Ticket } from '$lib/api';
import { effectiveKeys } from '$lib/board-order';
import Page from './+page.svelte';

// Stand-in for the real dnd action: it records which zone gets (re)configured, and
// the tests drive consider/finalize by dispatching the events the library emits.
const dnd = vi.hoisted(() => ({ updates: [] as string[] }));
vi.mock('svelte-dnd-action', () => ({
  dndzone: (node: HTMLElement) => ({
    update: () => dnd.updates.push(node.dataset.testid ?? ''),
    destroy() {}
  })
}));

// happy-dom lacks the Web Animations API that animate:flip drives.
Element.prototype.getAnimations ??= () => [];
Element.prototype.animate ??= () =>
  ({ cancel() {}, finish() {}, finished: Promise.resolve(), onfinish: null }) as unknown as Animation;

function mk(i: number, extra: Partial<Ticket> = {}): Ticket {
  const id = `FEAT-${String(i).padStart(4, '0')}`;
  return {
    id, type: 'FEAT', title: `Card ${i}`, status: 'todo', priority: 'medium', labels: [], deps: [],
    created: '', updated: `2026-07-04T00:${String(i % 60).padStart(2, '0')}:00Z`, blocked: false,
    hash: 'h-' + id, attachments: [], ...extra
  };
}

function seed(n: number) {
  workspace.board = {
    columns: [{ status: 'todo', title: 'Todo' }, { status: 'done', title: 'Done' }],
    unmapped: []
  };
  workspace.tickets = Object.fromEntries(Array.from({ length: n }, (_, i) => [mk(i).id, mk(i)]));
  workspace.hydrated = true;
}

const cards = () => screen.getByTestId('col-list-todo').querySelectorAll('a[href^="/tickets/"]').length;

describe('board page', () => {
  afterEach(() => {
    vi.restoreAllMocks();
    vi.useRealTimers();
    workspace.tickets = {};
    workspace.hydrated = false;
  });

  it('caps a long column at one page and shows the total in the header badge', async () => {
    seed(120);
    render(Page);
    flushSync();
    expect(cards()).toBe(50);
    expect(screen.getByTestId('col-count-todo').textContent).toBe('120');
    expect(screen.getByTestId('col-count-done').textContent).toBe('0');
    expect(screen.getByTestId('col-more-todo').textContent).toContain('Show 50 more');
  });

  it('reveals the next page per click and drops the button once everything shows', async () => {
    seed(120);
    render(Page);
    flushSync();
    await fireEvent.click(screen.getByTestId('col-more-todo'));
    expect(cards()).toBe(100);
    expect(screen.getByTestId('col-count-todo').textContent).toBe('120');
    expect(screen.getByTestId('col-more-todo').textContent).toContain('Show 20 more');
    await fireEvent.click(screen.getByTestId('col-more-todo'));
    expect(cards()).toBe(120);
    expect(screen.queryByTestId('col-more-todo')).toBeNull();
  });

  it('has no button for a column that fits one page', () => {
    seed(50);
    render(Page);
    flushSync();
    expect(cards()).toBe(50);
    expect(screen.queryByTestId('col-more-todo')).toBeNull();
  });

  it('keeps a column expanded while other data changes', async () => {
    seed(120);
    render(Page);
    flushSync();
    await fireEvent.click(screen.getByTestId('col-more-todo'));
    workspace.tickets = { ...workspace.tickets, [mk(500).id]: mk(500) };
    flushSync();
    expect(cards()).toBe(100);
    expect(screen.getByTestId('col-count-todo').textContent).toBe('121');
  });

  it('debounces typing in the filter but applies label/priority clicks at once', async () => {
    vi.useFakeTimers();
    seed(120);
    workspace.tickets = { ...workspace.tickets, [mk(7).id]: mk(7, { priority: 'critical', title: 'Needle' }) };
    render(Page);
    flushSync();
    const box = screen.getByTestId('board-filter-text') as HTMLInputElement;

    await fireEvent.input(box, { target: { value: 'needle' } });
    expect(box.value).toBe('needle'); // the input itself is never delayed
    expect(cards()).toBe(50); // ...but the columns haven't been rebuilt yet
    await vi.advanceTimersByTimeAsync(119);
    expect(cards()).toBe(50);
    await vi.advanceTimersByTimeAsync(2);
    expect(cards()).toBe(1);
    expect(screen.getByTestId('col-count-todo').textContent).toBe('1');

    // Clearing is a discrete click: applied immediately, no timer needed.
    await fireEvent.click(screen.getByTestId('board-filter-clear'));
    expect(cards()).toBe(50);
    expect(screen.getByTestId('col-count-todo').textContent).toBe('120');

    // Backspacing the box empty is applied at once too.
    await fireEvent.input(box, { target: { value: 'needle' } });
    await vi.advanceTimersByTimeAsync(121);
    expect(cards()).toBe(1);
    await fireEvent.input(box, { target: { value: '' } });
    expect(cards()).toBe(50);

    await fireEvent.click(screen.getByTestId('board-filter-prio-critical'));
    expect(cards()).toBe(1);
  });

  it('applies only the last keystroke of a burst', async () => {
    vi.useFakeTimers();
    seed(120);
    render(Page);
    flushSync();
    const box = screen.getByTestId('board-filter-text');
    for (const v of ['C', 'Ca', 'Car', 'Card 11']) {
      await fireEvent.input(box, { target: { value: v } });
      await vi.advanceTimersByTimeAsync(50);
    }
    expect(cards()).toBe(50); // 50ms since the last key: still waiting
    await vi.advanceTimersByTimeAsync(100);
    // "Card 11" + "Card 110".."Card 119"
    expect(screen.getByTestId('col-count-todo').textContent).toBe('11');
  });

  it('does not reconfigure an untouched column when another column changes', async () => {
    seed(0);
    workspace.tickets = {
      ...Object.fromEntries([mk(1), mk(2), mk(3)].map((t) => [t.id, t])),
      ...Object.fromEntries([mk(11, { status: 'done' }), mk(12, { status: 'done' })].map((t) => [t.id, t]))
    };
    render(Page);
    flushSync();
    dnd.updates.length = 0;

    // A card in `done` changes (e.g. an agent edits its file).
    workspace.tickets = { ...workspace.tickets, [mk(11).id]: mk(11, { status: 'done', title: 'Renamed' }) };
    flushSync();
    expect(dnd.updates).toContain('col-list-done');
    expect(dnd.updates).not.toContain('col-list-todo');
  });

  describe('dropping into a capped column', () => {
    function finalize(list: string, items: Ticket[], id: string) {
      const el = screen.getByTestId(list);
      el.dispatchEvent(new CustomEvent('finalize', { detail: { items, info: { id, trigger: 'droppedIntoZone' } } }));
    }

    it('persists an order against the full column, below the last visible card and above the first hidden one', async () => {
      seed(120);
      const reorder = vi.spyOn(workspace, 'reorder').mockResolvedValue(undefined);
      render(Page);
      flushSync();
      const full = workspace.columns[0].tickets;
      const slice = full.slice(0, 50);
      const moved = slice[0];

      // The dndzone only knows the 50 rendered cards: move the first to the bottom of them.
      finalize('col-list-todo', [...slice.slice(1), moved], moved.id);
      await vi.waitFor(() => expect(reorder).toHaveBeenCalledTimes(1));

      const [id, changes] = reorder.mock.calls[0];
      expect(id).toBe(moved.id);
      expect(changes.status).toBe('todo');
      const keys = effectiveKeys(full.filter((t) => t.id !== moved.id));
      expect(changes.order).toBeGreaterThan(keys.get(slice[49].id)!);
      expect(changes.order).toBeLessThan(keys.get(full[50].id)!);
    });

    it('persists a cross-column drop into a capped column', async () => {
      seed(0);
      workspace.tickets = {
        ...Object.fromEntries(Array.from({ length: 80 }, (_, i) => [mk(i, { status: 'done' }).id, mk(i, { status: 'done' })])),
        [mk(900).id]: mk(900)
      };
      const reorder = vi.spyOn(workspace, 'reorder').mockResolvedValue(undefined);
      render(Page);
      flushSync();
      const full = workspace.columns[1].tickets;
      const slice = full.slice(0, 50);
      const incoming = workspace.tickets[mk(900).id];

      // Dropped between the 3rd and 4th visible cards of `done`.
      finalize('col-list-done', [...slice.slice(0, 3), incoming, ...slice.slice(3)], incoming.id);
      await vi.waitFor(() => expect(reorder).toHaveBeenCalledTimes(1));
      const [id, changes] = reorder.mock.calls[0];
      const keys = effectiveKeys(full);
      expect(id).toBe(incoming.id);
      expect(changes.status).toBe('done');
      expect(changes.order).toBeGreaterThan(keys.get(slice[2].id)!);
      expect(changes.order).toBeLessThan(keys.get(slice[3].id)!);
    });

    it('does not rebuild columns mid-drag, then reconciles once the drop is done', async () => {
      seed(120);
      vi.spyOn(workspace, 'reorder').mockResolvedValue(undefined);
      render(Page);
      flushSync();
      const slice = workspace.columns[0].tickets.slice(0, 50);

      // consider = drag in flight (a card hovering gives the zone 51 items).
      const extra = mk(901);
      screen.getByTestId('col-list-todo').dispatchEvent(
        new CustomEvent('consider', { detail: { items: [...slice, extra], info: { id: extra.id } } })
      );
      flushSync();
      workspace.tickets = { ...workspace.tickets, [mk(902).id]: mk(902) }; // external change mid-drag
      flushSync();
      expect(cards()).toBe(51); // the drag's arrangement wasn't clobbered
      expect(screen.getByTestId('col-count-todo').textContent).toBe('120'); // badge stays on the store total

      // Cancelled drag: the zone reports its original items back.
      finalize('col-list-todo', slice, slice[0].id);
      await vi.waitFor(() => expect(screen.getByTestId('col-count-todo').textContent).toBe('121'));
      expect(cards()).toBe(50);
    });
  });
});
