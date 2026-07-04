<script lang="ts">
  import { toasts } from '$lib/toast.svelte';
</script>

<div class="stack">
  {#each toasts.items as t (t.id)}
    <div class="toast {t.kind}">
      <span>{t.msg}</span>
      {#if t.href}<a href={t.href} onclick={() => toasts.dismiss(t.id)}>{t.action ?? 'View'}</a>{/if}
      <button class="x" onclick={() => toasts.dismiss(t.id)} aria-label="Dismiss">×</button>
    </div>
  {/each}
</div>

<style>
  .stack { position: fixed; bottom: 16px; right: 16px; display: flex; flex-direction: column; gap: 8px; z-index: 60; }
  .toast {
    display: flex; align-items: center; gap: 10px;
    background: var(--color-surface-2); border: 1px solid var(--color-border);
    border-radius: 8px; padding: 8px 12px; font-size: 12px;
    box-shadow: 0 8px 32px rgb(0 0 0 / 0.45); max-width: 340px;
  }
  .toast.success { border-color: color-mix(in srgb, var(--color-accent) 50%, var(--color-border)); }
  .toast.error { border-color: color-mix(in srgb, var(--color-danger) 50%, var(--color-border)); }
  a { color: var(--color-accent); text-decoration: none; }
  .x { background: none; border: none; color: var(--color-dim); font-size: 16px; line-height: 1; }
</style>
