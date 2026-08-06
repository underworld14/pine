<script lang="ts">
  import type { Ticket } from '$lib/api';
  import { workspace } from '$lib/workspace.svelte';
  import { neighborhood, type NeighborRef } from '$lib/graph';
  import { ui } from '$lib/ui.svelte';

  let {
    ticket,
    onSelectTicket
  }: {
    ticket: Ticket;
    onSelectTicket?: (id: string) => void;
  } = $props();

  // Uncapped lists — graph stays at cap 6; this panel shows everything.
  const n = $derived(neighborhood(ticket, workspace.tickets, Number.POSITIVE_INFINITY));

  const related = $derived(n.memory.filter((m) => m.kind === 'ticket'));
  const dangling = $derived(
    n.dangling.map(
      (id): NeighborRef => ({
        id,
        title: id,
        status: 'missing',
        priority: '',
        unmet: true,
        kind: 'ticket'
      })
    )
  );
  const blockedBy = $derived([...n.blockers, ...dangling]);
  const isEpic = $derived(ticket.type === 'EPIC' || ticket.id.startsWith('EPIC-'));
  const showChildren = $derived(isEpic || n.children.length > 0);

  const hasAny = $derived(
    !!(n.parent || blockedBy.length || n.dependents.length || showChildren || related.length)
  );

  function onRowClick(e: MouseEvent, id: string, missing = false) {
    if (missing) return;
    if (!onSelectTicket) return;
    if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey || e.button !== 0) return;
    e.preventDefault();
    onSelectTicket(id);
  }

  function addChild() {
    ui.openModal({ type: 'feature', parent: ticket.id });
  }
</script>

{#if hasAny}
  <div class="relations" data-testid="ticket-relations">
    {#if n.parent}
      <section id="rel-epic" class="section" tabindex="-1" aria-labelledby="rel-epic-h">
        <h3 id="rel-epic-h" class="heading">Epic</h3>
        <ul class="list">
          <li>
            <a
              class="row"
              href={`/tickets/${n.parent.id}`}
              data-testid={`rel-row-${n.parent.id}`}
              onclick={(e) => onRowClick(e, n.parent!.id)}
            >
              <span class="id">{n.parent.id}</span>
              <span class="title">{n.parent.title || '—'}</span>
              <span class="status">{n.parent.status}</span>
            </a>
          </li>
        </ul>
      </section>
    {/if}

    {#if blockedBy.length}
      <section id="rel-blocked" class="section" tabindex="-1" aria-labelledby="rel-blocked-h">
        <h3 id="rel-blocked-h" class="heading">Blocked by</h3>
        <ul class="list scroll">
          {#each blockedBy as ref (ref.id)}
            <li>
              {#if ref.status === 'missing'}
                <span class="row missing" data-testid={`rel-row-${ref.id}`}>
                  <span class="id">{ref.id}</span>
                  <span class="title">missing dependency</span>
                  <span class="status">missing</span>
                </span>
              {:else}
                <a
                  class="row"
                  class:unmet={ref.unmet}
                  href={`/tickets/${ref.id}`}
                  data-testid={`rel-row-${ref.id}`}
                  onclick={(e) => onRowClick(e, ref.id)}
                >
                  <span class="id">{ref.id}</span>
                  <span class="title">{ref.title || '—'}</span>
                  <span class="status">{ref.unmet ? 'blocking' : ref.status}</span>
                </a>
              {/if}
            </li>
          {/each}
        </ul>
      </section>
    {/if}

    {#if n.dependents.length}
      <section id="rel-blocks" class="section" tabindex="-1" aria-labelledby="rel-blocks-h">
        <h3 id="rel-blocks-h" class="heading">Blocks</h3>
        <ul class="list scroll">
          {#each n.dependents as ref (ref.id)}
            <li>
              <a
                class="row"
                href={`/tickets/${ref.id}`}
                data-testid={`rel-row-${ref.id}`}
                onclick={(e) => onRowClick(e, ref.id)}
              >
                <span class="id">{ref.id}</span>
                <span class="title">{ref.title || '—'}</span>
                <span class="status">{ref.status}</span>
              </a>
            </li>
          {/each}
        </ul>
      </section>
    {/if}

    {#if showChildren}
      <section id="rel-children" class="section" tabindex="-1" aria-labelledby="rel-children-h">
        <div class="section-head">
          <h3 id="rel-children-h" class="heading">
            Children
            {#if ticket.epicProgress}
              <span class="meta">({ticket.epicProgress.done}/{ticket.epicProgress.total} done)</span>
            {:else if isEpic}
              <span class="meta">(0)</span>
            {/if}
          </h3>
          {#if isEpic && !ticket.readOnly}
            <button type="button" class="add-child" data-testid="add-child" onclick={addChild}>
              + Add child
            </button>
          {/if}
        </div>
        {#if n.children.length}
          <ul class="list scroll children">
            {#each n.children as ref (ref.id)}
              <li>
                <a
                  class="row"
                  href={`/tickets/${ref.id}`}
                  data-testid={`rel-row-${ref.id}`}
                  onclick={(e) => onRowClick(e, ref.id)}
                >
                  <span class="id">{ref.id}</span>
                  <span class="title">{ref.title || '—'}</span>
                  <span class="status">{ref.status}</span>
                </a>
              </li>
            {/each}
          </ul>
        {:else}
          <p class="empty-children">No child tickets yet.</p>
        {/if}
      </section>
    {/if}

    {#if related.length}
      <section id="rel-related" class="section" tabindex="-1" aria-labelledby="rel-related-h">
        <h3 id="rel-related-h" class="heading">Related</h3>
        <ul class="list scroll">
          {#each related as ref (ref.id)}
            <li>
              <a
                class="row"
                href={`/tickets/${ref.id}`}
                data-testid={`rel-row-${ref.id}`}
                onclick={(e) => onRowClick(e, ref.id)}
              >
                <span class="id">{ref.id}</span>
                <span class="title">{ref.title || '—'}</span>
                <span class="status">{ref.status}</span>
              </a>
            </li>
          {/each}
        </ul>
      </section>
    {/if}
  </div>
{/if}

<style>
  .relations {
    display: flex;
    flex-direction: column;
    gap: 12px;
    margin: 0 0 16px;
    max-width: 720px;
  }
  .section {
    border: 1px solid var(--color-border);
    border-radius: 10px;
    background: var(--color-surface);
    padding: 8px 10px 6px;
  }
  .section:focus {
    outline: none;
  }
  .section:focus-visible {
    outline: 2px solid var(--color-accent);
    outline-offset: 2px;
  }
  .section-head {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 6px;
  }
  .heading {
    margin: 0;
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.04em;
    text-transform: uppercase;
    color: var(--color-dim);
  }
  .meta {
    font-weight: 500;
    text-transform: none;
    letter-spacing: 0;
    margin-left: 4px;
  }
  .add-child {
    margin-left: auto;
    font-size: 11px;
    padding: 3px 8px;
    border-radius: 6px;
    border: 1px solid var(--color-border);
    background: var(--color-surface-2);
    color: var(--color-text);
    cursor: pointer;
  }
  .add-child:hover {
    border-color: var(--color-accent);
    color: var(--color-accent);
  }
  .empty-children {
    margin: 0 0 4px;
    padding: 6px 8px;
    font-size: 12px;
    color: var(--color-dim);
  }
  .list {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  .list.scroll {
    max-height: 220px;
    overflow: auto;
  }
  .list.children {
    max-height: 320px;
  }
  .row {
    display: grid;
    grid-template-columns: minmax(88px, auto) 1fr auto;
    gap: 10px;
    align-items: center;
    padding: 6px 8px;
    border-radius: 6px;
    text-decoration: none;
    color: inherit;
    font-size: 12px;
  }
  a.row:hover {
    background: var(--color-surface-2);
  }
  a.row:focus-visible {
    outline: 2px solid var(--color-accent);
    outline-offset: 1px;
  }
  .row.unmet .status {
    color: var(--color-warn);
  }
  .row.missing {
    opacity: 0.65;
    cursor: default;
  }
  .id {
    font-family: var(--font-mono);
    font-size: 11px;
    color: var(--color-dim);
  }
  .title {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .status {
    font-family: var(--font-mono);
    font-size: 10px;
    color: var(--color-dim);
  }
</style>
