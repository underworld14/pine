<script lang="ts">
  import { onDestroy } from 'svelte';
  import { workspace } from '$lib/workspace.svelte';
  import { toasts } from '$lib/toast.svelte';
  import { priorityMeta } from '$lib/ui-helpers';
  import type { Ticket } from '$lib/api';

  let { ticket, anchor = null, onClose }: { ticket: Ticket; anchor?: DOMRect | null; onClose: () => void } = $props();

  // Track the live ticket so edits (and external SSE updates) reflect instantly.
  const t = $derived(workspace.tickets[ticket.id] ?? ticket);
  const statuses = $derived(workspace.columns.map((c) => ({ status: c.status, title: c.title })));
  const pm = $derived(priorityMeta(t.priority));

  let newLabel = $state('');
  let dialogEl = $state<HTMLDivElement | null>(null);

  const WIDTH = 300;
  // Anchor beside the card, flipping to the left / clamping to the viewport so
  // the popover never spills off-screen.
  const pos = $derived.by(() => {
    if (!anchor) return { left: Math.max(8, (window.innerWidth - WIDTH) / 2), top: 80 };
    let left = anchor.right + 8;
    if (left + WIDTH > window.innerWidth - 8) left = Math.max(8, anchor.left - WIDTH - 8);
    const top = Math.min(Math.max(8, anchor.top), Math.max(8, window.innerHeight - 340));
    return { left, top };
  });

  $effect(() => {
    dialogEl?.focus();
  });

  // Return focus to the trigger (the card's peek button) when the popover closes.
  const returnFocus = typeof document !== 'undefined' ? (document.activeElement as HTMLElement | null) : null;
  onDestroy(() => returnFocus?.focus?.());

  async function setField(patch: Record<string, unknown>) {
    try {
      await workspace.patch(t.id, patch);
    } catch (e) {
      toasts.push(e instanceof Error ? e.message : 'Update failed', 'error');
    }
  }
  function addLabel() {
    const l = newLabel.trim();
    newLabel = '';
    if (!l || t.labels.includes(l)) return;
    setField({ labels: [...t.labels, l] });
  }
  function removeLabel(l: string) {
    setField({ labels: t.labels.filter((x) => x !== l) });
  }
  async function del() {
    try {
      await workspace.remove(t.id);
      onClose();
      toasts.push(`${t.id} deleted`, 'info');
    } catch (e) {
      toasts.push(e instanceof Error ? e.message : 'Delete failed', 'error');
    }
  }
  function onKey(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.preventDefault();
      onClose();
      return;
    }
    // Trap Tab within the dialog so focus can't wander onto the blocked board.
    if (e.key === 'Tab' && dialogEl) {
      const focusable = Array.from(
        dialogEl.querySelectorAll<HTMLElement>('a[href], button, select, input, [tabindex]:not([tabindex="-1"])')
      ).filter((el) => !el.hasAttribute('disabled'));
      if (focusable.length === 0) return;
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      const active = document.activeElement;
      if (e.shiftKey && (active === first || active === dialogEl)) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && active === last) {
        e.preventDefault();
        first.focus();
      }
    }
  }
</script>

<div class="fixed inset-0 z-[80]" role="presentation" onmousedown={(e) => { if (e.target === e.currentTarget) onClose(); }}>
  <div
    bind:this={dialogEl}
    role="dialog"
    aria-modal="true"
    aria-label={`Quick edit ${t.id}`}
    tabindex="-1"
    data-testid="quick-peek"
    onkeydown={onKey}
    style={`left:${pos.left}px; top:${pos.top}px`}
    class="fixed w-[300px] max-w-[92vw] rounded-[10px] border border-border bg-surface p-3 shadow-[0_8px_32px_rgb(0_0_0/0.45)] outline-none"
  >
    <div class="flex items-center gap-2">
      <span class="text-[10px]" style={`color:${pm.color}`} title={pm.label}>{pm.glyph}</span>
      <span class="mono text-[11px] text-dim">{t.id}</span>
      <button type="button" onclick={onClose} aria-label="Close" class="ml-auto text-dim hover:text-text">×</button>
    </div>

    <div class="mt-1.5 line-clamp-3 text-[13px]">{t.title}</div>

    <div class="mt-3 text-[10px] uppercase tracking-wide text-dim">Status</div>
    <select
      data-testid="quick-peek-status"
      aria-label="Status"
      value={t.status}
      onchange={(e) => setField({ status: e.currentTarget.value })}
      class="mt-1 w-full rounded-md border border-border bg-bg px-2 py-1 text-[12px] outline-none focus:border-accent"
    >
      {#each statuses as s}
        <option value={s.status}>{s.title}</option>
      {/each}
    </select>

    <div class="mt-3 text-[10px] uppercase tracking-wide text-dim">Priority</div>
    <div class="mt-1 flex overflow-hidden rounded-md border border-border" role="group" aria-label="Priority">
      {#each ['low', 'medium', 'high', 'critical'] as p}
        <button
          type="button"
          data-testid={`quick-peek-prio-${p}`}
          aria-pressed={t.priority === p}
          onclick={() => setField({ priority: p })}
          class="flex-1 border-0 px-1 py-1 text-[11px] capitalize transition-colors"
          class:bg-accent-soft={t.priority === p}
          class:text-accent={t.priority === p}
          class:bg-surface-2={t.priority !== p}
          class:text-dim={t.priority !== p}
        >
          {p}
        </button>
      {/each}
    </div>

    <div class="mt-3 text-[10px] uppercase tracking-wide text-dim">Labels</div>
    <div class="mt-1 flex flex-wrap items-center gap-1">
      {#each t.labels as l}
        <span class="inline-flex items-center gap-1 rounded-full bg-surface-2 px-2 py-0.5 text-[11px] text-dim">
          {l}
          <button type="button" onclick={() => removeLabel(l)} aria-label={`Remove ${l}`} class="hover:text-danger">×</button>
        </span>
      {/each}
      <input
        bind:value={newLabel}
        placeholder="+ label"
        aria-label="Add label"
        onkeydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); addLabel(); } }}
        class="w-20 rounded-md border border-border bg-bg px-2 py-0.5 text-[11px] outline-none focus:border-accent"
      />
    </div>

    <div class="mt-4 flex items-center gap-2 border-t border-border pt-2.5">
      <a
        href={`/tickets/${t.id}`}
        data-testid="quick-peek-open"
        onclick={onClose}
        class="rounded-md bg-surface-2 px-3 py-1 text-[12px] text-text hover:text-accent"
      >
        Open
      </a>
      <button
        type="button"
        data-testid="quick-peek-delete"
        onclick={del}
        class="ml-auto text-[12px] text-dim hover:text-danger"
      >
        Delete
      </button>
    </div>
  </div>
</div>
