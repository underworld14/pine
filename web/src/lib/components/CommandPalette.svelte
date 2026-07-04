<script lang="ts">
  import { ui } from '$lib/ui.svelte';
  import { workspace } from '$lib/workspace.svelte';
  import { goto } from '$app/navigation';

  let query = $state('');
  let active = $state(0);
  let inputEl = $state<HTMLInputElement | null>(null);

  interface Item { label: string; hint?: string; run: () => void; }

  const commands = (): Item[] => [
    { label: 'New Bug', hint: 'c', run: () => ui.openModal({ type: 'bug' }) },
    { label: 'New Feature', run: () => ui.openModal({ type: 'feature' }) },
    { label: 'New Epic', run: () => ui.openModal({ type: 'epic' }) },
    { label: 'Go to Dashboard', hint: 'g d', run: () => goto('/') },
    { label: 'Go to Board', hint: 'g b', run: () => goto('/board') },
    { label: 'Toggle theme', run: () => ui.toggleTheme() }
  ];

  const results = $derived.by<Item[]>(() => {
    const q = query.trim().toLowerCase();
    const cmds = commands().filter((c) => !q || c.label.toLowerCase().includes(q));
    const tickets: Item[] = workspace.list
      .filter((t) => !q || t.id.toLowerCase().includes(q) || t.title.toLowerCase().includes(q))
      .sort((a, b) => {
        const ap = a.id.toLowerCase().startsWith(q) ? 0 : 1;
        const bp = b.id.toLowerCase().startsWith(q) ? 0 : 1;
        return ap - bp || b.updated.localeCompare(a.updated);
      })
      .slice(0, 8)
      .map((t) => ({ label: `${t.id} · ${t.title}`, run: () => goto(`/tickets/${t.id}`) }));
    const items = [...cmds, ...tickets];
    if (q) items.push({ label: `Search everything for "${query}"`, run: () => goto(`/search?q=${encodeURIComponent(query)}`) });
    return items;
  });

  $effect(() => {
    if (ui.paletteOpen) { query = ''; active = 0; queueMicrotask(() => inputEl?.focus()); }
  });
  $effect(() => { active = 0; void results; });

  function run(i: number) {
    const item = results[i];
    if (!item) return;
    ui.paletteOpen = false;
    item.run();
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === 'Escape') { ui.paletteOpen = false; }
    else if (e.key === 'ArrowDown') { e.preventDefault(); active = Math.min(active + 1, results.length - 1); }
    else if (e.key === 'ArrowUp') { e.preventDefault(); active = Math.max(active - 1, 0); }
    else if (e.key === 'Enter') { e.preventDefault(); run(active); }
  }
</script>

{#if ui.paletteOpen}
  <div class="overlay" role="dialog" aria-modal="true" onmousedown={(e) => { if (e.target === e.currentTarget) ui.paletteOpen = false; }}>
    <div class="palette">
      <input bind:this={inputEl} bind:value={query} onkeydown={onKey} placeholder="Type a command or search…" />
      <ul>
        {#each results as item, i}
          <li class:active={i === active} onmousemove={() => (active = i)} onclick={() => run(i)}>
            <span>{item.label}</span>
            {#if item.hint}<kbd>{item.hint}</kbd>{/if}
          </li>
        {/each}
      </ul>
    </div>
  </div>
{/if}

<style>
  .overlay { position: fixed; inset: 0; background: rgb(0 0 0 / 0.5); display: grid; place-items: start center; padding-top: 14vh; z-index: 80; }
  .palette { width: 560px; max-width: 92vw; background: var(--color-surface); border: 1px solid var(--color-border); border-radius: 10px; overflow: hidden; box-shadow: 0 8px 32px rgb(0 0 0 / 0.45); }
  input { width: 100%; padding: 12px 14px; background: transparent; border: none; border-bottom: 1px solid var(--color-border); outline: none; font-size: 15px; }
  ul { list-style: none; margin: 0; padding: 4px; max-height: 50vh; overflow: auto; }
  li { display: flex; align-items: center; padding: 8px 10px; border-radius: 6px; font-size: 13px; cursor: pointer; }
  li.active { background: var(--color-surface-2); }
  li span { flex: 1; }
</style>
