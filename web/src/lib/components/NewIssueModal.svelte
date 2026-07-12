<script lang="ts">
  import { tick } from 'svelte';
  import { ui } from '$lib/ui.svelte';
  import { workspace } from '$lib/workspace.svelte';
  import { api } from '$lib/api';
  import { toasts } from '$lib/toast.svelte';
  import {
    insertAtCursor,
    removeUploadPlaceholder,
    rewriteStagedUploads,
    uploadPlaceholder
  } from '$lib/insert-at-cursor';

  type Staged = { key: string; file: File };

  let type = $state('bug');
  let title = $state('');
  let description = $state('');
  let priority = $state('medium');
  let labelsRaw = $state('');
  let staged = $state<Staged[]>([]);
  let saving = $state(false);
  let titleEl = $state<HTMLInputElement | null>(null);
  let descEl = $state<HTMLTextAreaElement | null>(null);
  let keySeq = 0;

  // Object-URL previews for staged image files (client-side blobs; work anywhere,
  // including a VS Code webview). Cached per File so we create each URL once.
  const urlCache = new Map<File, string>();
  function previewUrl(f: File): string | null {
    if (!f.type.startsWith('image/')) return null;
    let u = urlCache.get(f);
    if (!u) {
      u = URL.createObjectURL(f);
      urlCache.set(f, u);
    }
    return u;
  }
  // Revoke URLs for files that are no longer staged (remove / reset / submit).
  $effect(() => {
    const current = new Set(staged.map((s) => s.file));
    for (const [file, url] of urlCache) {
      if (!current.has(file)) {
        URL.revokeObjectURL(url);
        urlCache.delete(file);
      }
    }
  });

  // Reset + focus each time the modal opens.
  $effect(() => {
    if (ui.modalOpen) {
      type = ui.modalDefaults.type ?? 'bug';
      title = '';
      description = '';
      priority = 'medium';
      labelsRaw = '';
      staged = [];
      keySeq = 0;
      queueMicrotask(() => titleEl?.focus());
    }
  });

  function composeBody(): string {
    const d = description.trim();
    if (type === 'feature') return `# Description\n\n${d}\n\n# Acceptance Criteria\n`;
    if (type === 'epic') return `# Description\n\n${d}\n\n# Goals\n`;
    return `# Description\n\n${d}\n\n# Steps to Reproduce\n\n# Expected\n\n# Actual\n`;
  }

  function nextKey(): string {
    keySeq += 1;
    return `u${keySeq}`;
  }

  function padBefore(value: string, pos: number): string {
    if (pos <= 0) return '';
    return value[pos - 1] === '\n' ? '' : '\n\n';
  }

  async function insertImagePlaceholders(items: Staged[]) {
    const images = items.filter((s) => s.file.type.startsWith('image/'));
    if (!images.length) return;
    const el = descEl && document.activeElement === descEl ? descEl : null;
    let value = description;
    let caret = el?.selectionStart ?? value.length;
    for (const item of images) {
      const snippet = padBefore(value, caret) + uploadPlaceholder(item.key, item.file.name);
      const r = insertAtCursor(value, snippet, { selectionStart: caret, selectionEnd: caret });
      value = r.value;
      caret = r.caret;
    }
    description = value;
    await tick();
    if (descEl) {
      descEl.focus();
      descEl.setSelectionRange(caret, caret);
    }
  }

  function stageFiles(list: FileList | File[], opts: { insertMarkdown: boolean }) {
    const arr = Array.from(list).filter((f) => f && f.size >= 0);
    if (!arr.length) return;
    const added: Staged[] = [];
    for (const f of arr) {
      const name = f.name || `paste-${staged.length + added.length + 1}.png`;
      const file = f.name ? f : new File([f], name, { type: f.type });
      added.push({ key: nextKey(), file });
    }
    staged = [...staged, ...added];
    if (opts.insertMarkdown) void insertImagePlaceholders(added);
  }

  async function submit() {
    if (!title.trim() || saving) return;
    saving = true;
    try {
      const labels = labelsRaw.split(',').map((s) => s.trim()).filter(Boolean);
      const snapshot = [...staged];
      let body = composeBody();
      const t = await workspace.create({
        type,
        title: title.trim(),
        priority,
        labels,
        status: ui.modalDefaults.status,
        body
      });
      if (snapshot.length) {
        try {
          const results = await api.upload(
            t.id,
            snapshot.map((s) => s.file),
            { opId: workspace.beginOp() }
          );
          const next = rewriteStagedUploads(
            body,
            snapshot.map((s) => s.key),
            results
          );
          if (next !== body) await workspace.patch(t.id, { body: next });
          body = next;
        } catch {
          let cleaned = body;
          for (const item of snapshot) cleaned = removeUploadPlaceholder(cleaned, item.key);
          if (cleaned !== body) {
            try { await workspace.patch(t.id, { body: cleaned }); } catch { /* ignore */ }
          }
          toasts.push('Attachment upload failed', 'error');
        }
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
    const imgs: File[] = [];
    for (const it of items) {
      if (it.type.startsWith('image/')) {
        const f = it.getAsFile();
        if (f) imgs.push(new File([f], f.name || `paste-${staged.length + imgs.length + 1}.png`, { type: f.type }));
      }
    }
    if (!imgs.length) return;
    e.preventDefault();
    stageFiles(imgs, { insertMarkdown: true });
  }

  function onDrop(e: DragEvent) {
    e.preventDefault();
    const fs = e.dataTransfer?.files;
    if (fs?.length) stageFiles(fs, { insertMarkdown: true });
  }

  function onPick(e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    if (input.files?.length) stageFiles(input.files, { insertMarkdown: true });
    input.value = '';
  }

  function removeStaged(i: number) {
    const item = staged[i];
    if (!item) return;
    description = removeUploadPlaceholder(description, item.key);
    staged = staged.filter((_, j) => j !== i);
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
      <textarea bind:this={descEl} bind:value={description} class="desc" rows="3" placeholder="Description (optional)"></textarea>
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
          {#each staged as s, i}
            <div class="thumb" class:image={previewUrl(s.file)}>
              {#if previewUrl(s.file)}
                <img class="preview" src={previewUrl(s.file)} alt={s.file.name} />
              {/if}
              <span class="name">{s.file.name}</span>
              <button class="rm" onclick={() => removeStaged(i)} aria-label="Remove">×</button>
            </div>
          {/each}
        </div>
      {/if}
      <div class="foot">
        <label class="attach">
          <input type="file" accept="image/*,video/*" multiple onchange={onPick} hidden />
          Attach files
        </label>
        <span class="tip">or paste (⌘V) / drop a screenshot</span>
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
  .staged { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 8px; }
  .thumb { display: flex; align-items: center; gap: 6px; font-size: 11px; background: var(--color-surface-2); padding: 3px 8px; border-radius: 6px; }
  .thumb.image { position: relative; flex-direction: column; align-items: flex-start; gap: 4px; padding: 6px; }
  .preview { width: 84px; height: 84px; object-fit: cover; border-radius: 4px; display: block; background: var(--color-bg); }
  .thumb .name { max-width: 84px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .thumb .rm { background: none; border: none; color: var(--color-dim); cursor: pointer; }
  .thumb.image .rm { position: absolute; top: 4px; right: 4px; width: 18px; height: 18px; line-height: 1; border-radius: 4px; background: rgb(0 0 0 / 0.55); color: #fff; }
  .foot { display: flex; align-items: center; gap: 10px; margin-top: 14px; }
  .attach { font-size: 11px; padding: 4px 10px; border: 1px solid var(--color-border); border-radius: 6px; background: var(--color-surface-2); cursor: pointer; }
  .tip { color: var(--color-dim); font-size: 11px; }
  .create { margin-left: auto; padding: 6px 16px; border-radius: 6px; border: none; background: var(--color-accent); color: #062018; font-weight: 600; }
  .create:disabled { opacity: 0.5; }
</style>
