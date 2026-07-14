<script lang="ts">
  import type { FileSuggestItem } from '$lib/file-mention';

  interface Props {
    items: FileSuggestItem[];
    active: number;
    loading?: boolean;
    top: number;
    left: number;
    onSelect: (item: FileSuggestItem) => void;
    onHover: (index: number) => void;
  }

  let { items, active, loading = false, top, left, onSelect, onHover }: Props = $props();
</script>

<div
  class="fixed z-[90] min-w-[280px] max-w-[420px] max-h-[240px] overflow-auto rounded-[10px] border border-border bg-surface shadow-[0_8px_32px_rgb(0_0_0_/_0.45)]"
  style:top="{top}px"
  style:left="{left}px"
  role="listbox"
  aria-label="File and folder suggestions"
>
  {#if loading && items.length === 0}
    <div class="px-3 py-2 text-xs text-dim">Searching…</div>
  {:else if items.length === 0}
    <div class="px-3 py-2 text-xs text-dim">No matching files</div>
  {:else}
    <ul class="m-0 list-none p-1">
      {#each items as item, i}
        <li>
          <button
            type="button"
            role="option"
            aria-selected={i === active}
            class="flex w-full items-center gap-2 rounded-md px-2.5 py-1.5 text-left text-[13px] transition-colors duration-150
              {i === active ? 'bg-surface-2 text-text' : 'text-text hover:bg-surface-2/80'}"
            onmousemove={() => onHover(i)}
            onclick={() => onSelect(item)}
          >
            <span
              class="inline-flex h-5 w-5 shrink-0 items-center justify-center rounded text-[10px] font-mono
                {item.kind === 'dir' ? 'bg-accent-soft text-accent' : 'bg-surface-2 text-dim'}"
              aria-hidden="true"
            >{item.kind === 'dir' ? '/' : '·'}</span>
            <span class="min-w-0 flex-1 truncate font-mono text-[12px]">{item.path}</span>
            <span class="shrink-0 text-[10px] uppercase tracking-wide text-dim">{item.kind}</span>
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</div>
