<script lang="ts">
  import { emptyFilter, isActive, toggle, type BoardFilter } from '$lib/board-filter';

  let { filter, labels, onChange }: { filter: BoardFilter; labels: string[]; onChange: (f: BoardFilter) => void } = $props();

  const PRIORITIES = ['critical', 'high', 'medium', 'low'];
  const active = $derived(isActive(filter));
</script>

<div data-testid="board-filter" class="flex flex-wrap items-center gap-2 px-4 pt-3">
  <input
    data-testid="board-filter-text"
    aria-label="Filter cards"
    value={filter.text}
    oninput={(e) => onChange({ ...filter, text: e.currentTarget.value })}
    placeholder="Filter cards…"
    class="w-52 rounded-md border border-border bg-bg px-2.5 py-1 text-[12px] outline-none focus:border-accent"
  />

  <div class="flex items-center gap-1" role="group" aria-label="Filter by priority">
    {#each PRIORITIES as p}
      <button
        type="button"
        data-testid={`board-filter-prio-${p}`}
        aria-pressed={filter.priorities.includes(p)}
        onclick={() => onChange({ ...filter, priorities: toggle(filter.priorities, p) })}
        class="rounded-md border px-2 py-0.5 text-[11px] capitalize transition-colors"
        class:border-accent={filter.priorities.includes(p)}
        class:text-accent={filter.priorities.includes(p)}
        class:border-border={!filter.priorities.includes(p)}
        class:text-dim={!filter.priorities.includes(p)}
      >
        {p}
      </button>
    {/each}
  </div>

  {#if labels.length}
    <div class="flex flex-wrap items-center gap-1" role="group" aria-label="Filter by label">
      {#each labels as l}
        <button
          type="button"
          data-testid={`board-filter-label-${l}`}
          aria-pressed={filter.labels.includes(l)}
          onclick={() => onChange({ ...filter, labels: toggle(filter.labels, l) })}
          class="rounded-full border px-2 py-0.5 text-[11px] transition-colors"
          class:border-accent={filter.labels.includes(l)}
          class:text-accent={filter.labels.includes(l)}
          class:border-border={!filter.labels.includes(l)}
          class:text-dim={!filter.labels.includes(l)}
        >
          {l}
        </button>
      {/each}
    </div>
  {/if}

  {#if active}
    <button
      type="button"
      data-testid="board-filter-clear"
      onclick={() => onChange(emptyFilter())}
      class="ml-auto text-[11px] text-dim underline-offset-2 hover:text-text hover:underline"
    >
      Clear filter
    </button>
  {/if}
</div>
