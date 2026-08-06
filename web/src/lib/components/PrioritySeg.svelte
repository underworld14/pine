<script lang="ts">
  import { PRIORITIES, priorityMeta } from '$lib/ui-helpers';

  let {
    value,
    disabled = false,
    onChange,
    testIdPrefix = 'prio',
    static: isStatic = false
  }: {
    value: string;
    disabled?: boolean;
    onChange: (priority: string) => void;
    /** Prefix for `data-testid="{prefix}-{priority}"`. */
    testIdPrefix?: string;
    /** Disable scale-on-press when motion would distract. */
    static?: boolean;
  } = $props();
</script>

<div class="seg" role="group" aria-label="Priority">
  {#each PRIORITIES as p}
    {@const meta = priorityMeta(p)}
    <button
      type="button"
      data-testid={`${testIdPrefix}-${p}`}
      aria-label={meta.label}
      aria-pressed={value === p}
      disabled={disabled}
      class:active={value === p}
      class:tap={!isStatic}
      style="--c: {meta.color}"
      onclick={() => onChange(p)}
    >
      <span class="label">{meta.short}</span>
    </button>
  {/each}
</div>

<style>
  /* Concentric: outer 8px = inner 6px + 2px padding */
  .seg {
    display: inline-flex;
    align-items: stretch;
    gap: 2px;
    padding: 2px;
    border-radius: 8px;
    border: 1px solid var(--color-border);
    background: var(--color-bg);
    box-shadow: 0 0 0 1px rgb(0 0 0 / 0.04);
  }

  .seg button {
    position: relative;
    flex: 1 1 0;
    min-width: 40px;
    min-height: 40px;
    margin: 0;
    padding: 0 8px;
    border: none;
    border-radius: 6px;
    background: transparent;
    color: var(--color-dim);
    font-size: 11px;
    font-weight: 650;
    letter-spacing: 0.02em;
    line-height: 1;
    cursor: pointer;
    transition-property: transform, background-color, color, box-shadow;
    transition-duration: 150ms;
    transition-timing-function: ease-out;
  }

  .seg button.tap:active:not(:disabled) {
    transform: scale(0.96);
  }

  .seg button:hover:not(:disabled):not(.active) {
    color: var(--color-text);
    background: color-mix(in srgb, var(--color-surface-2) 80%, transparent);
  }

  .seg button.active {
    background: color-mix(in srgb, var(--c) 18%, transparent);
    color: var(--c);
    box-shadow: 0 0 0 1px color-mix(in srgb, var(--c) 35%, transparent);
  }

  .seg button:disabled {
    opacity: 0.55;
    cursor: not-allowed;
  }

  .seg button:focus-visible {
    outline: 2px solid var(--color-accent);
    outline-offset: 1px;
  }

  .label {
    display: block;
    text-align: center;
  }
</style>
