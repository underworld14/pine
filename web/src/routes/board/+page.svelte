<script lang="ts">
  import { workspace } from '$lib/workspace.svelte';
  import { ui } from '$lib/ui.svelte';
  import { toasts } from '$lib/toast.svelte';
  import TicketCard from '$lib/components/TicketCard.svelte';
  import AddCardComposer from '$lib/components/AddCardComposer.svelte';
  import BoardFilterBar from '$lib/components/BoardFilterBar.svelte';
  import QuickPeekPopover from '$lib/components/QuickPeekPopover.svelte';
  import { dndzone } from 'svelte-dnd-action';
  import { flip } from 'svelte/animate';
  import { dropOrderInColumn } from '$lib/board-order';
  import { emptyFilter, isActive, matchesFilter, type BoardFilter } from '$lib/board-filter';
  import type { Ticket } from '$lib/api';

  // Honor reduced-motion for the reorder/move animations too (CSS transitions are
  // already disabled globally, but flip is a Web-Animations timeline).
  const reduce =
    typeof window !== 'undefined' && window.matchMedia?.('(prefers-reduced-motion: reduce)').matches;
  const FLIP = reduce ? 0 : 200;

  let dragging = $state(false);
  let filter = $state<BoardFilter>(emptyFilter());
  let peek = $state<{ ticket: Ticket; anchor: DOMRect } | null>(null);

  // Local, filtered view of columns so drag operations feel instant; reconciled
  // from the store whenever we're not mid-drag.
  let cols = $state<{ status: string; title: string; items: Ticket[] }[]>([]);
  $effect(() => {
    if (dragging) return;
    const f = filter;
    cols = workspace.columns.map((c) => ({
      status: c.status,
      title: c.title,
      items: c.tickets.filter((t) => matchesFilter(t, f))
    }));
  });

  const labels = $derived(workspace.allLabels());
  const filtering = $derived(isActive(filter));

  // Close the quick-peek popover if its ticket disappears (deleted elsewhere).
  $effect(() => {
    if (peek && !workspace.tickets[peek.ticket.id]) peek = null;
  });

  // Give the dragged clone a lifted, tilted look (styled by .dragging in app.css).
  function transformDragged(el: HTMLElement | undefined) {
    el?.classList.add('dragging');
  }

  function handleConsider(ci: number, e: CustomEvent) {
    dragging = true;
    cols[ci].items = e.detail.items;
  }

  async function handleFinalize(ci: number, e: CustomEvent) {
    const status = cols[ci].status;
    const items = e.detail.items as Ticket[];
    cols[ci].items = items;
    const movedId: string | undefined = e.detail.info?.id;
    try {
      if (!movedId) return;
      const moved = items.find((t) => t.id === movedId);
      if (!moved) return; // source zone of a cross-column drag: nothing to persist here
      if (moved.readOnly) {
        const where = moved.branch ? `branch "${moved.branch}"` : 'another branch';
        toasts.push(`${moved.id} lives on ${where} — check it out to move it`, 'error');
        return; // the reconcile effect snaps it back
      }
      // Compute the order against the FULL column (unfiltered) so a drop made
      // while the quick-filter hides cards can't corrupt their relative order.
      const idx = items.findIndex((t) => t.id === movedId);
      const upperNeighborId = idx > 0 ? items[idx - 1].id : null;
      const full = workspace.columns.find((c) => c.status === status)?.tickets ?? [];
      const order = dropOrderInColumn(full, movedId, upperNeighborId);
      if (order == null) return; // drop-in-place / cancelled drag — nothing to persist
      // One PATCH covers within-column reorder and cross-column move.
      try {
        await workspace.reorder(movedId, { status, order });
      } catch {
        toasts.push(`Couldn't move ${moved.id} — reverted`, 'error');
      }
    } finally {
      dragging = false;
    }
  }

  async function addCard(status: string, title: string) {
    // Use the project's first configured type (by prefix, which the server
    // resolves directly) so quick-add works on any config — not just ones that
    // happen to define a "feature" type.
    const type = workspace.config?.types?.[0]?.prefix ?? 'feature';
    try {
      await workspace.create({ type, title, status });
    } catch (e) {
      toasts.push(e instanceof Error ? e.message : 'Create failed', 'error');
      throw e; // let the composer keep the typed text for a retry
    }
  }
</script>

<div class="flex h-full flex-col">
  {#if workspace.hydrated}
    <BoardFilterBar {filter} {labels} onChange={(f) => (filter = f)} />
  {/if}

  <div class="flex flex-1 items-start gap-3 overflow-x-auto p-4" data-testid="board">
    {#if !workspace.hydrated}
      {#each Array(4) as _, i (i)}
        <section class="flex w-[280px] flex-none flex-col gap-2 rounded-[10px] border border-border bg-surface p-3">
          <div class="h-3 w-20 animate-pulse rounded bg-surface-2"></div>
          {#each Array(3) as _, j (j)}
            <div class="h-14 animate-pulse rounded-lg bg-surface-2"></div>
          {/each}
        </section>
      {/each}
    {:else}
      {#each cols as col, ci (col.status)}
        <section
          class="flex max-h-[calc(100vh-96px)] w-[280px] flex-none flex-col rounded-[10px] border border-border bg-surface"
          data-testid={`col-${col.status}`}
        >
          <header class="flex items-center gap-2 px-3 py-2.5 text-[12px] font-semibold uppercase tracking-wide text-dim">
            <span>{col.title}</span>
            <span class="rounded-full bg-surface-2 px-[7px] text-[11px]">{col.items.length}</span>
            <button
              class="ml-auto border-0 bg-transparent text-[16px] leading-none text-dim hover:text-text"
              title="New in {col.title}"
              onclick={() => ui.openModal({ status: col.status })}
            >
              +
            </button>
          </header>

          <div
            class="flex flex-1 flex-col gap-2 overflow-y-auto px-2.5 pb-1 pt-1"
            style="min-height: 40px"
            data-testid={`col-list-${col.status}`}
            use:dndzone={{
              items: col.items,
              flipDurationMs: FLIP,
              dropTargetStyle: {},
              dropTargetClasses: ['drop-active'],
              transformDraggedElement: transformDragged
            }}
            onconsider={(e) => handleConsider(ci, e)}
            onfinalize={(e) => handleFinalize(ci, e)}
          >
            {#each col.items as t (t.id)}
              <div animate:flip={{ duration: FLIP }}>
                <TicketCard ticket={t} onPeek={(tk, anchor) => (peek = { ticket: tk, anchor })} />
              </div>
            {/each}
          </div>

          {#if col.items.length === 0}
            <div class="px-3 pb-1 pt-1 text-[11px] text-dim" data-testid={`col-empty-${col.status}`}>
              {filtering ? 'No matching cards' : 'No cards yet'}
            </div>
          {/if}

          <div class="px-2.5 pb-3 pt-1">
            <AddCardComposer status={col.status} onSubmit={(title) => addCard(col.status, title)} />
          </div>
        </section>
      {/each}
    {/if}
  </div>
</div>

{#if peek}
  <QuickPeekPopover ticket={peek.ticket} anchor={peek.anchor} onClose={() => (peek = null)} />
{/if}
