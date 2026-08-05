<script lang="ts">
  import { tick } from 'svelte';

  let { status, onSubmit }: { status: string; onSubmit: (title: string) => void | Promise<void> } = $props();

  let open = $state(false);
  let title = $state('');
  let busy = $state(false);
  let inputEl = $state<HTMLTextAreaElement | null>(null);

  async function begin() {
    open = true;
    await tick();
    inputEl?.focus();
  }
  function close() {
    open = false;
    title = '';
  }
  async function submit() {
    const t = title.trim();
    if (!t || busy) return;
    busy = true;
    try {
      await onSubmit(t);
      title = ''; // stay open for rapid entry, Trello-style
      await tick();
      inputEl?.focus();
    } catch {
      // Keep the typed text so the user can retry a failed create.
    } finally {
      busy = false;
    }
  }
  function onKey(e: KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      submit();
    } else if (e.key === 'Escape') {
      e.preventDefault();
      close();
    }
  }
</script>

{#if open}
  <div class="flex flex-col gap-1.5">
    <textarea
      bind:this={inputEl}
      bind:value={title}
      data-testid={`add-card-input-${status}`}
      class="w-full resize-none rounded-md border border-border bg-bg px-2.5 py-2 text-[13px] outline-none focus:border-accent"
      rows="2"
      placeholder="Title — Enter to add, Esc to cancel"
      onkeydown={onKey}
      onblur={() => {
        if (!title.trim()) close();
      }}
    ></textarea>
    <div class="flex items-center gap-2">
      <button
        type="button"
        onclick={submit}
        disabled={!title.trim() || busy}
        class="rounded-md bg-accent px-3 py-1 text-[12px] font-semibold text-[#062018] disabled:opacity-50"
      >
        {busy ? 'Adding…' : 'Add card'}
      </button>
      <button type="button" onclick={close} class="text-[12px] text-dim hover:text-text">Cancel</button>
    </div>
  </div>
{:else}
  <button
    type="button"
    data-testid={`add-card-${status}`}
    onclick={begin}
    class="flex w-full items-center gap-1.5 rounded-md px-2 py-1.5 text-left text-[12px] text-dim transition-colors hover:bg-surface-2 hover:text-text"
  >
    <span class="text-[14px] leading-none">+</span> Add a card
  </button>
{/if}
