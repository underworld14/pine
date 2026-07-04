<script lang="ts">
  import { workspace } from '$lib/workspace.svelte';
  import { ui } from '$lib/ui.svelte';
  import { relTime } from '$lib/format';
  import { priorityMeta } from '$lib/ui-helpers';
  import type { Ticket } from '$lib/api';

  const c = $derived(workspace.counts);

  function pick(filter: (t: Ticket) => boolean, n = 7): Ticket[] {
    return workspace.list.filter(filter).sort((a, b) => b.updated.localeCompare(a.updated)).slice(0, n);
  }
  const bugs = $derived(pick((t) => t.type === 'BUG' && t.status !== 'done'));
  const feats = $derived(pick((t) => t.type === 'FEAT' && t.status !== 'done'));
  const testing = $derived(pick((t) => t.status === 'testing'));
  const done = $derived(pick((t) => t.status === 'done'));
</script>

<div class="wrap">
  <header>
    <h1>{workspace.config?.project.name ?? 'Pine'}</h1>
    <div class="counts">
      <span>{c.open} open</span><span>·</span><span>{c.testing} testing</span><span>·</span><span>{c.done} done</span>
    </div>
  </header>

  {#if workspace.hydrated && c.total === 0}
    <div class="empty">
      <p>No issues yet. Press <kbd>c</kbd> to create your first one — it takes 10 seconds.</p>
      <button onclick={() => ui.openModal({ type: 'bug' })}>+ New issue</button>
    </div>
  {:else}
    <div class="grid">
      {#each [['Recent Bugs', bugs], ['Recent Features', feats], ['In Testing', testing], ['Recently Done', done]] as [title, items]}
        <section>
          <h2>{title}</h2>
          {#if (items as Ticket[]).length === 0}
            <p class="none">Nothing here.</p>
          {:else}
            {#each items as Ticket[] as t}
              <a class="row" href={`/tickets/${t.id}`}>
                <span class="prio" style="color: {priorityMeta(t.priority).color}">{priorityMeta(t.priority).glyph}</span>
                <span class="mono id">{t.id}</span>
                <span class="title">{t.title}</span>
                {#if t.blocked}<span title="blocked">🔒</span>{/if}
                <span class="time">{relTime(t.updated)}</span>
              </a>
            {/each}
          {/if}
        </section>
      {/each}
    </div>
  {/if}
</div>

<style>
  .wrap { padding: 20px 24px; max-width: 1100px; }
  header { display: flex; align-items: baseline; gap: 16px; margin-bottom: 20px; }
  h1 { font-size: 20px; font-weight: 650; margin: 0; }
  .counts { color: var(--color-dim); display: flex; gap: 6px; font-size: 12px; }
  .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(320px, 1fr)); gap: 16px; }
  section { background: var(--color-surface); border: 1px solid var(--color-border); border-radius: 10px; padding: 12px 14px; }
  h2 { font-size: 12px; text-transform: uppercase; letter-spacing: 0.04em; color: var(--color-dim); margin: 0 0 8px; }
  .row { display: flex; align-items: center; gap: 8px; padding: 5px 4px; border-radius: 6px; text-decoration: none; color: inherit; font-size: 13px; }
  .row:hover { background: var(--color-surface-2); }
  .prio { font-size: 10px; }
  .id { color: var(--color-dim); font-size: 11px; }
  .title { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .time { color: var(--color-dim); font-size: 11px; }
  .none { color: var(--color-dim); font-size: 12px; }
  .empty { text-align: center; padding: 80px 20px; color: var(--color-dim); }
  .empty button { margin-top: 12px; padding: 8px 16px; border-radius: 6px; border: none; background: var(--color-accent); color: #062018; font-weight: 600; }
</style>
