<script lang="ts">
  import type { Ticket } from '$lib/api';
  import { workspace } from '$lib/workspace.svelte';
  import { priorityMeta, labelColor } from '$lib/ui-helpers';
  import { relTime } from '$lib/format';

  let { ticket }: { ticket: Ticket } = $props();
  const pm = $derived(priorityMeta(ticket.priority));
  const flash = $derived(workspace.flashing[ticket.id] ?? 0);
</script>

<a
  href={`/tickets/${ticket.id}`}
  class="card"
  class:flash={flash > 0}
  data-flash={flash}
>
  <div class="row">
    <span class="prio" style="color: {pm.color}" title={pm.label}>{pm.glyph}</span>
    <span class="mono id">{ticket.id}</span>
    {#if ticket.blocked}
      <span class="lock" title="Blocked by {ticket.unmet?.length ?? ''} unmet dependencies">🔒</span>
    {/if}
    <span class="spacer"></span>
    {#each ticket.labels.slice(0, 2) as l}
      <span class="chip" style="--c: {labelColor(l)}">{l}</span>
    {/each}
    {#if ticket.labels.length > 2}<span class="more">+{ticket.labels.length - 2}</span>{/if}
  </div>
  <div class="title">{ticket.title}</div>
  <div class="foot">
    {#if ticket.attachments?.length}<span>📎 {ticket.attachments.length}</span>{/if}
    {#if ticket.parent}<span class="mono parent">{ticket.parent}</span>{/if}
    <span class="spacer"></span>
    <span class="time">{relTime(ticket.updated)}</span>
  </div>
</a>

<style>
  .card {
    display: block;
    background: var(--color-surface);
    border: 1px solid var(--color-border);
    border-radius: 8px;
    padding: 8px 10px;
    text-decoration: none;
    color: inherit;
    transition: border-color 0.14s, transform 0.14s;
  }
  .card:hover { border-color: color-mix(in srgb, var(--color-accent) 40%, var(--color-border)); }
  .row { display: flex; align-items: center; gap: 6px; }
  .prio { font-size: 10px; }
  .id { font-size: 11px; color: var(--color-dim); }
  .lock { font-size: 11px; }
  .spacer { flex: 1; }
  .chip {
    font-size: 10px; padding: 1px 6px; border-radius: 999px;
    background: color-mix(in srgb, var(--c) 18%, transparent);
    color: var(--c); white-space: nowrap;
  }
  .more { font-size: 10px; color: var(--color-dim); }
  .title { margin-top: 4px; font-size: 13px; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
  .foot { display: flex; align-items: center; gap: 8px; margin-top: 6px; font-size: 11px; color: var(--color-dim); }
  .parent { font-size: 10px; }
</style>
