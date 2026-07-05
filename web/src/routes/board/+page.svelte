<script lang="ts">
  import { workspace } from '$lib/workspace.svelte';
  import { ui } from '$lib/ui.svelte';
  import { toasts } from '$lib/toast.svelte';
  import TicketCard from '$lib/components/TicketCard.svelte';
  import { dndzone } from 'svelte-dnd-action';
  import { flip } from 'svelte/animate';
  import type { Ticket } from '$lib/api';

  const FLIP = 200;

  // Local view of columns so drag operations feel instant; reconciled from the store.
  let cols = $state<{ status: string; title: string; items: Ticket[] }[]>([]);
  let dragging = $state(false);
  $effect(() => {
    if (dragging) return;
    cols = workspace.columns.map((c) => ({ status: c.status, title: c.title, items: [...c.tickets] }));
  });

  function handleConsider(ci: number, e: CustomEvent) {
    dragging = true;
    cols[ci].items = e.detail.items;
  }
  async function handleFinalize(ci: number, e: CustomEvent) {
    const status = cols[ci].status;
    cols[ci].items = e.detail.items;
    try {
      // Determine which ticket landed in this column with a different status.
      for (const t of e.detail.items as Ticket[]) {
        if (t.status !== status) {
          try {
            await workspace.move(t.id, status);
          } catch (err) {
            toasts.push(`Couldn't move ${t.id} — reverted`, 'error');
          }
          break;
        }
      }
    } finally {
      dragging = false;
    }
  }
</script>

<div class="board">
  {#each cols as col, ci (col.status)}
    <section class="col">
      <header>
        <span>{col.title}</span>
        <span class="count">{col.items.length}</span>
        <button class="add" title="New in {col.title}" onclick={() => ui.openModal({ status: col.status })}>+</button>
      </header>
      <div
        class="list"
        use:dndzone={{ items: col.items, flipDurationMs: FLIP, dropTargetStyle: {} }}
        onconsider={(e) => handleConsider(ci, e)}
        onfinalize={(e) => handleFinalize(ci, e)}
      >
        {#each col.items as t (t.id)}
          <div animate:flip={{ duration: FLIP }}>
            <TicketCard ticket={t} />
          </div>
        {/each}
      </div>
    </section>
  {/each}
</div>

<style>
  .board { display: flex; gap: 12px; padding: 16px; height: 100%; overflow-x: auto; align-items: flex-start; }
  .col { flex: 0 0 280px; background: var(--color-surface); border: 1px solid var(--color-border); border-radius: 10px; display: flex; flex-direction: column; max-height: calc(100vh - 32px); }
  header { display: flex; align-items: center; gap: 8px; padding: 10px 12px; font-size: 12px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.03em; color: var(--color-dim); }
  .count { background: var(--color-surface-2); border-radius: 999px; padding: 0 7px; font-size: 11px; }
  .add { margin-left: auto; background: none; border: none; color: var(--color-dim); font-size: 16px; line-height: 1; }
  .list { flex: 1; overflow-y: auto; display: flex; flex-direction: column; gap: 8px; padding: 4px 10px 12px; min-height: 40px; }
</style>
