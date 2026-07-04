<script lang="ts">
  import { ui } from '$lib/ui.svelte';
  import { workspace } from '$lib/workspace.svelte';
  import { api } from '$lib/api';
  import { toasts } from '$lib/toast.svelte';
  import { goto } from '$app/navigation';

  let type = $state('bug');
  let title = $state('');
  let description = $state('');
  let priority = $state('medium');
  let labelsRaw = $state('');
  let staged = $state<File[]>([]);
  let saving = $state(false);
  let titleEl = $state<HTMLInputElement | null>(null);

  // Reset + focus each time the modal opens.
  $effect(() => {
    if (ui.modalOpen) {
      type = ui.modalDefaults.type ?? 'bug';
      title = '';
      description = '';
      priority = 'medium';
      labelsRaw = '';
      staged = [];
      queueMicrotask(() => titleEl?.focus());
    }
  });

  function composeBody(): string {
    const d = description.trim();
    if (type === 'feature') return `# Description\n\n${d}\n\n# Acceptance Criteria\n`;
    if (type === 'epic') return `# Description\n\n${d}\n\n# Goals\n`;
    return `# Description\n\n${d}\n\n# Steps to Reproduce\n\n# Expected\n\n# Actual\n`;
  }

  async function submit() {
    if (!title.trim() || saving) return;
    saving = true;
    try {
      const labels = labelsRaw.split(',').map((s) => s.trim()).filter(Boolean);
      const t = await workspace.create({
        type,
        title: title.trim(),
        priority,
        labels,
        status: ui.modalDefaults.status,
        body: composeBody()
      });
      if (staged.length) {
        try { await api.upload(t.id, staged); } catch { toasts.push('Attachment upload failed', 'error'); }
      }
      ui.closeModal();
      toasts.push(`${t.id} created`, 'success', { href: `/tickets/${t.id}`, action: 'View' });
    } catch (e) {
      toasts.push(e instanceof Error ? e.message : 'Create failed', 'error');
    } finally {
      saving = false;
    }
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === 'Escape') { e.preventDefault(); ui.closeModal(); }
    if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') { e.preventDefault(); submit(); }
  }

  function onPaste(e: ClipboardEvent) {
    const items = e.clipboardData?.items;
    if (!items) return;
    for (const it of items) {
      if (it.type.startsWith('image/')) {
        const f = it.getAsFile();
        if (f) staged = [...staged, new File([f], f.name || `paste-${staged.length + 1}.png`, { type: f.type })];
      }
    }
  }

  function onDrop(e: DragEvent) {
    e.preventDefault();
    const fs = e.dataTransfer?.files;
    if (fs) staged = [...staged, ...Array.from(fs)];
  }
</script>

{#if ui.modalOpen}
  <div class="overlay" role="dialog" aria-modal="true" onmousedown={(e) => { if (e.target === e.currentTarget) ui.closeModal(); }}>
    <div class="modal" onkeydown={onKey} onpaste={onPaste} ondragover={(e) => e.preventDefault()} ondrop={onDrop} role="document">
      <div class="types">
        {#each [['bug', '🐛 Bug'], ['feature', '✦ Feature'], ['epic', '❖ Epic']] as [val, label]}
          <button class="typebtn" class:active={type === val} onclick={() => (type = val)}>{label}</button>
        {/each}
        <span class="hint"><kbd>Esc</kbd> cancel · <kbd>⌘↵</kbd> create</span>
      </div>
      <input bind:this={titleEl} bind:value={title} class="title" placeholder="Title" onkeydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); submit(); } }} />
      <textarea bind:value={description} class="desc" rows="3" placeholder="Description (optional)"></textarea>
      <div class="controls">
        <div class="seg">
          {#each ['low', 'medium', 'high', 'critical'] as p}
            <button class:active={priority === p} onclick={() => (priority = p)}>{p}</button>
          {/each}
        </div>
        <input bind:value={labelsRaw} class="labels" placeholder="labels, comma separated" />
      </div>
      {#if staged.length}
        <div class="staged">
          {#each staged as f, i}
            <div class="thumb">
              <span>{f.name}</span>
              <button onclick={() => (staged = staged.filter((_, j) => j !== i))} aria-label="Remove">×</button>
            </div>
          {/each}
        </div>
      {/if}
      <div class="foot">
        <span class="tip">Paste (⌘V) or drop a screenshot to attach</span>
        <button class="create" disabled={!title.trim() || saving} onclick={submit}>{saving ? 'Creating…' : 'Create'}</button>
      </div>
    </div>
  </div>
{/if}

<style>
  .overlay { position: fixed; inset: 0; background: rgb(0 0 0 / 0.5); display: grid; place-items: start center; padding-top: 12vh; z-index: 70; }
  .modal { width: 560px; max-width: 92vw; background: var(--color-surface); border: 1px solid var(--color-border); border-radius: 10px; padding: 16px; box-shadow: 0 8px 32px rgb(0 0 0 / 0.45); }
  .types { display: flex; align-items: center; gap: 6px; margin-bottom: 12px; }
  .typebtn { padding: 4px 10px; border-radius: 6px; border: 1px solid var(--color-border); background: var(--color-surface-2); font-size: 12px; }
  .typebtn.active { border-color: var(--color-accent); color: var(--color-accent); }
  .hint { margin-left: auto; color: var(--color-dim); font-size: 11px; }
  .title { width: 100%; padding: 8px 10px; font-size: 16px; background: var(--color-bg); border: 1px solid var(--color-border); border-radius: 6px; outline: none; }
  .title:focus { border-color: var(--color-accent); }
  .desc { width: 100%; margin-top: 8px; padding: 8px 10px; background: var(--color-bg); border: 1px solid var(--color-border); border-radius: 6px; outline: none; resize: vertical; }
  .controls { display: flex; gap: 8px; margin-top: 8px; }
  .seg { display: flex; border: 1px solid var(--color-border); border-radius: 6px; overflow: hidden; }
  .seg button { padding: 4px 8px; font-size: 11px; background: var(--color-surface-2); border: none; text-transform: capitalize; }
  .seg button.active { background: var(--color-accent-soft); color: var(--color-accent); }
  .labels { flex: 1; padding: 4px 10px; background: var(--color-bg); border: 1px solid var(--color-border); border-radius: 6px; outline: none; font-size: 12px; }
  .staged { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 8px; }
  .thumb { display: flex; align-items: center; gap: 6px; font-size: 11px; background: var(--color-surface-2); padding: 3px 8px; border-radius: 6px; }
  .thumb button { background: none; border: none; color: var(--color-dim); }
  .foot { display: flex; align-items: center; margin-top: 14px; }
  .tip { color: var(--color-dim); font-size: 11px; }
  .create { margin-left: auto; padding: 6px 16px; border-radius: 6px; border: none; background: var(--color-accent); color: #062018; font-weight: 600; }
  .create:disabled { opacity: 0.5; }
</style>
