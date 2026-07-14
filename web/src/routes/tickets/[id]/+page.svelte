<script lang="ts">
  import { tick } from 'svelte';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { workspace } from '$lib/workspace.svelte';
  import { toasts } from '$lib/toast.svelte';
  import { api, ApiError, type Ticket } from '$lib/api';
  import { renderMarkdown } from '$lib/markdown';
  import { priorityMeta, labelColor } from '$lib/ui-helpers';
  import { relTime, bytes } from '$lib/format';
  import { reconcileEditor } from '$lib/ticket-editor';
  import {
    insertAtCursor,
    replaceAll,
    stripAttachmentMarkdown,
    uploadingPlaceholder
  } from '$lib/insert-at-cursor';
  import { FileMentionController } from '$lib/file-mention-controller.svelte';
  import FileMentionPopup from '$lib/components/FileMentionPopup.svelte';
  import TicketGraph from '$lib/components/TicketGraph.svelte';

  const id = $derived($page.params.id);
  const ticket = $derived(workspace.get(id));
  // Off-branch tickets are read-only: every mutation is gated below and the
  // server would 409 anyway. The editor stays in preview.
  const readOnly = $derived(!!ticket?.readOnly);

  const MIN_PANE = 120;

  let mode = $state<'preview' | 'split' | 'edit'>('preview');
  let text = $state('');
  let baseHash = $state('');
  let baseBody = $state(''); // the body at baseHash — dirtiness is measured against this
  let dirty = $derived(text !== baseBody);
  let conflict = $state<Ticket | null>(null);
  let saveTimer: ReturnType<typeof setTimeout> | null = null;
  let editorShellEl = $state<HTMLDivElement | null>(null);
  let previewEl = $state<HTMLDivElement | null>(null);
  let textareaEl = $state<HTMLTextAreaElement | null>(null);
  // Locked content height so preview ↔ edit don't jump. Null = size to content.
  let paneHeight = $state<number | null>(null);
  let lightbox = $state<string | null>(null);
  const fileMention = new FileMentionController();

  /** Size the textarea to its content and sync paneHeight (allows shrink). */
  function fitTextarea() {
    const el = textareaEl;
    if (!el) return;
    el.style.height = 'auto';
    const h = Math.max(Math.round(el.scrollHeight), MIN_PANE);
    el.style.height = `${h}px`;
    paneHeight = h;
  }

  /** Measure natural preview height (without a stale fixed lock). */
  async function fitPreview() {
    paneHeight = null;
    await tick();
    const el = previewEl;
    if (!el) return;
    paneHeight = Math.max(Math.round(el.scrollHeight), MIN_PANE);
  }

  // Keep paneHeight in sync when the user vertically resizes the textarea.
  $effect(() => {
    const el = textareaEl;
    if (!el || mode === 'preview') return;
    const ro = new ResizeObserver(() => {
      const h = Math.round(el.offsetHeight);
      if (h > 0 && h !== paneHeight) paneHeight = h;
    });
    ro.observe(el);
    return () => ro.disconnect();
  });
  // Load / rebase when the ticket identity or disk version changes.
  $effect(() => {
    const t = ticket;
    if (!t) return;
    if (baseHash === '') {
      text = t.body ?? '';
      baseBody = t.body ?? '';
      baseHash = t.hash;
      return;
    }
    if (t.hash !== baseHash) {
      const r = reconcileEditor({ text, baseBody, baseHash, ticket: t });
      text = r.text;
      baseBody = r.baseBody;
      baseHash = r.baseHash;
      conflict = r.conflict;
    }
  });

  // Reset editor state when navigating to a different ticket.
  let lastId = '';
  $effect(() => {
    if (id !== lastId) {
      lastId = id;
      baseHash = '';
      baseBody = '';
      conflict = null;
      mode = 'preview';
      paneHeight = null;
    }
  });

  const preview = $derived(renderMarkdown(mode === 'edit' ? '' : text));

  async function save(force = false) {
    if (!ticket || readOnly) return;
    if (saveTimer) { clearTimeout(saveTimer); saveTimer = null; }
    // A normal save uses baseHash — the version the editor content is based on —
    // NOT ticket.hash (which the SSE stream may have already advanced to the disk
    // version). That is what lets the server return 409 when the file moved under
    // us, instead of silently overwriting an agent's edit. A forced overwrite
    // acknowledges the conflicting disk version explicitly.
    const ifMatch = force ? (conflict?.hash ?? baseHash) : baseHash;
    const opId = workspace.beginOp();
    try {
      const updated = await api.patchTicket(id, { body: text, opId }, ifMatch);
      workspace.tickets = { ...workspace.tickets, [updated.id]: updated };
      baseHash = updated.hash;
      baseBody = updated.body ?? '';
      conflict = null;
      toasts.push('Saved', 'success');
    } catch (e) {
      if (e instanceof ApiError && e.status === 409 && e.current) {
        conflict = e.current;
      } else {
        toasts.push(e instanceof Error ? e.message : 'Save failed', 'error');
      }
    }
  }

  function onEdit() {
    if (readOnly) return;
    fitTextarea();
    fileMention.onInput(textareaEl, text);
    if (saveTimer) clearTimeout(saveTimer);
    saveTimer = setTimeout(() => { if (dirty) save(); }, 2000);
  }

  function onTextareaKeydown(e: KeyboardEvent) {
    if (readOnly) return;
    const consumed = fileMention.onKeydown(e, textareaEl, text, (next, caret) => {
      text = next;
      tick().then(() => {
        fitTextarea();
        textareaEl?.focus();
        textareaEl?.setSelectionRange(caret, caret);
      });
      if (saveTimer) clearTimeout(saveTimer);
      saveTimer = setTimeout(() => { if (dirty) save(); }, 2000);
    });
    if (consumed) return;
  }

  async function startEdit(next: 'edit' | 'split' = 'edit') {
    if (readOnly) return;
    mode = next;
    await tick();
    fitTextarea();
    textareaEl?.focus();
  }

  async function finishEdit() {
    if (dirty) save();
    fileMention.close();
    mode = 'preview';
    await fitPreview();
  }

  function onPreviewDblClick(e: MouseEvent) {
    if (readOnly) return;
    const target = e.target as HTMLElement | null;
    // Let links, checkboxes, and buttons keep their own interaction.
    if (target?.closest('a, button, input, label, pre, code')) return;
    e.preventDefault();
    startEdit('edit');
  }

  function onKeydown(e: KeyboardEvent) {
    if (readOnly) return;
    if (e.key === 'Escape' && fileMention.open) {
      e.preventDefault();
      fileMention.close();
      return;
    }
    if (e.key === 'Escape' && mode !== 'preview') {
      e.preventDefault();
      finishEdit();
      return;
    }
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 's') { e.preventDefault(); save(); }
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'e') {
      e.preventDefault();
      if (mode === 'preview') startEdit('edit');
      else if (mode === 'edit') startEdit('split');
      else finishEdit();
    }
  }

  function onPointerDown(e: PointerEvent) {
    if (readOnly || mode === 'preview') return;
    const t = e.target as HTMLElement | null;
    if (t?.closest('[data-file-mention]')) return;
    if (t && editorShellEl?.contains(t)) return;
    fileMention.close();
    finishEdit();
  }

  function reloadFromDisk() {
    if (!conflict) return;
    text = conflict.body ?? '';
    baseBody = conflict.body ?? '';
    baseHash = conflict.hash;
    workspace.tickets = { ...workspace.tickets, [conflict.id]: conflict };
    conflict = null;
  }

  async function setField(patch: Record<string, unknown>) {
    if (readOnly) return;
    try { await workspace.patch(id, patch); } catch (e) { toasts.push('Update failed', 'error'); }
  }

  async function copyPrompt() {
    const p = await api.ticketPrompt(id);
    await navigator.clipboard.writeText(p);
    toasts.push('Prompt copied — paste into your AI agent', 'success');
  }

  async function del() {
    if (readOnly) return;
    if (!confirm(`Delete ${id}?`)) return;
    await workspace.remove(id);
    goto('/board');
  }

  let labelInput = $state('');
  async function addLabel() {
    const l = labelInput.trim();
    if (!l || !ticket || readOnly) return;
    await setField({ labels: [...ticket.labels, l] });
    labelInput = '';
  }
  async function removeLabel(l: string) {
    if (!ticket || readOnly) return;
    await setField({ labels: ticket.labels.filter((x) => x !== l) });
  }

  async function onChecklistChange(e: Event) {
    if (!ticket || readOnly || dirty) {
      const target = e.target as HTMLElement;
      if (target instanceof HTMLInputElement && target.type === 'checkbox') {
        target.checked = !target.checked;
        if (dirty) toasts.push('Save or discard body edits before toggling checkboxes', 'error');
      }
      return;
    }
    const target = e.target as HTMLElement;
    if (!(target instanceof HTMLInputElement) || target.type !== 'checkbox') return;
    const boxes = Array.from((e.currentTarget as HTMLElement).querySelectorAll('input[type=checkbox]'));
    const index = boxes.indexOf(target);
    if (index < 0) return;
    try {
      const updated = await api.setChecklistItem(id, index, target.checked, baseHash, workspace.beginOp());
      workspace.tickets = { ...workspace.tickets, [updated.id]: updated };
      baseHash = updated.hash;
      baseBody = updated.body ?? '';
      text = updated.body ?? '';
    } catch (err) {
      target.checked = !target.checked;
      if (err instanceof ApiError && err.status === 409 && err.current) conflict = err.current;
      else toasts.push(err instanceof Error ? err.message : 'Update failed', 'error');
    }
  }

  async function uploadFiles(files: File[]) {
    if (!files.length || readOnly || !id) return;
    // Ensure we have an editor surface so placeholders land in the body.
    const fromPreview = mode === 'preview';
    if (fromPreview) await startEdit('edit');
    const names = files.map((f, i) => f.name || `paste-${i + 1}.png`);
    const placeholders = names.map((n) => uploadingPlaceholder(n));
    const el = !fromPreview && textareaEl && document.activeElement === textareaEl ? textareaEl : null;
    let value = text;
    // From preview, append at end; otherwise insert at caret.
    let caret = fromPreview ? value.length : (el?.selectionStart ?? value.length);
    for (const ph of placeholders) {
      const pad = caret > 0 && value[caret - 1] !== '\n' ? '\n\n' : '';
      const r = insertAtCursor(value, pad + ph, { selectionStart: caret, selectionEnd: caret });
      value = r.value;
      caret = r.caret;
    }
    text = value;
    await tick();
    fitTextarea();
    textareaEl?.focus();
    textareaEl?.setSelectionRange(caret, caret);

    try {
      const results = await api.upload(id, files, { opId: workspace.beginOp() });
      let next = text;
      for (let i = 0; i < placeholders.length; i++) {
        const r = results[i];
        if (r && !r.error && r.markdown) next = replaceAll(next, placeholders[i], r.markdown);
        else next = replaceAll(next, placeholders[i], '');
      }
      text = next.replace(/\n{3,}/g, '\n\n');
      await tick();
      fitTextarea();
      const ok = results.filter((r) => !r.error);
      if (ok.length) {
        await save();
        const saved = ok.reduce((a, r) => a + (r.originalBytes - r.finalBytes), 0);
        toasts.push(saved > 0 ? `Attached · saved ${bytes(saved)}` : 'Attached', 'success');
      } else {
        toasts.push('Upload failed', 'error');
      }
    } catch {
      let cleaned = text;
      for (const ph of placeholders) cleaned = replaceAll(cleaned, ph, '');
      text = cleaned.replace(/\n{3,}/g, '\n\n');
      toasts.push('Upload failed', 'error');
    }
  }

  function onPaste(e: ClipboardEvent) {
    if (readOnly) return;
    const imgs: File[] = [];
    for (const it of e.clipboardData?.items ?? []) {
      if (it.type.startsWith('image/')) {
        const f = it.getAsFile();
        if (f) imgs.push(new File([f], f.name || `paste-${imgs.length + 1}.png`, { type: f.type }));
      }
    }
    if (imgs.length) { e.preventDefault(); uploadFiles(imgs); }
  }

  async function removeAttachment(name: string) {
    if (!ticket || readOnly || !id) return;
    try {
      await api.deleteAttachment(id, name, workspace.beginOp());
      const next = stripAttachmentMarkdown(text, id, name);
      if (next !== text) {
        text = next;
        await save();
      }
      if (lightbox?.includes(encodeURIComponent(name)) || lightbox?.endsWith('/' + name)) {
        lightbox = null;
      }
      toasts.push('Attachment removed', 'success');
    } catch (err) {
      toasts.push(err instanceof Error ? err.message : 'Delete failed', 'error');
    }
  }
</script>

<svelte:window onkeydown={onKeydown} onpointerdown={onPointerDown} />

{#if !ticket}
  <div class="missing">Ticket <span class="mono">{id}</span> not found. <a href="/board">Back to board</a></div>
{:else}
  <div class="page" onpaste={onPaste}>
    <div class="bar">
      <a href="/board" class="back">←</a>
      <button class="mono id" onclick={() => navigator.clipboard.writeText(ticket.id)} title="Copy id">{ticket.id}</button>
      <input class="title" value={ticket.title} readonly={readOnly} onblur={(e) => setField({ title: (e.target as HTMLInputElement).value })} />
      <span class="spacer"></span>
      {#if !readOnly}
        <button class="ghost" onclick={copyPrompt}>Copy AI prompt</button>
        <button class="ghost danger" onclick={del}>Delete</button>
      {/if}
    </div>

    {#if readOnly}
      <div class="ro-banner">
        <span class="ro-badge">⑂ {ticket.branch}</span>
        Read-only — this ticket lives on branch <strong>{ticket.branch}</strong>. Check out that branch to edit it.
        {#if ticket.branch}
          <button class="ro-copy" onclick={() => navigator.clipboard.writeText(ticket.branch ?? '')} title="Copy branch name">copy</button>
        {/if}
      </div>
    {/if}

    <div class="meta">
      <select value={ticket.status} disabled={readOnly} onchange={(e) => setField({ status: (e.target as HTMLSelectElement).value })}>
        {#each workspace.board?.columns ?? [] as c}<option value={c.status}>{c.title}</option>{/each}
        {#if !(workspace.board?.columns ?? []).some((c) => c.status === ticket.status)}<option value={ticket.status}>{ticket.status}</option>{/if}
      </select>
      <div class="seg">
        {#each ['low', 'medium', 'high', 'critical'] as p}
          <button class:active={ticket.priority === p} disabled={readOnly} style="--c: {priorityMeta(p).color}" onclick={() => setField({ priority: p })} title={p}>{priorityMeta(p).glyph}</button>
        {/each}
      </div>
      <div class="labels">
        {#each ticket.labels as l}
          <span class="chip" style="--c: {labelColor(l)}">{l}{#if !readOnly}<button onclick={() => removeLabel(l)}>×</button>{/if}</span>
        {/each}
        {#if !readOnly}
          <input bind:value={labelInput} placeholder="+ label" onkeydown={(e) => { if (e.key === 'Enter') addLabel(); }} />
        {/if}
      </div>
      <span class="updated">updated {relTime(ticket.updated)}</span>
      {#if ticket.acceptance?.total}<span class="updated">AC {ticket.acceptance.done}/{ticket.acceptance.total}</span>{/if}
    </div>

    <TicketGraph {ticket} />
    {#if ticket.epicProgress}
      <p class="epic-summary">Children ({ticket.epicProgress.done}/{ticket.epicProgress.total} done)</p>
    {/if}

    {#if conflict}
      <div class="conflict">
        ⚠ Changed on disk (probably an AI agent).
        <button onclick={reloadFromDisk}>Reload from disk</button>
        <button onclick={() => save(true)}>Keep mine & overwrite</button>
      </div>
    {/if}

    <div class="editor-shell" bind:this={editorShellEl}>
      <div class="editor-head">
        {#if !readOnly}
          {#if mode === 'preview'}
            <span class="edit-hint">Double-click to edit · <kbd>⌘E</kbd></span>
          {:else}
            <button class="done" onclick={finishEdit}>Done</button>
            <div class="modes">
              <button class:active={mode === 'edit'} onclick={() => startEdit('edit')}>Edit</button>
              <button class:active={mode === 'split'} onclick={() => startEdit('split')}>Split</button>
            </div>
            {#if dirty}<span class="dirty">unsaved · <kbd>⌘S</kbd></span>{/if}
            <span class="edit-hint dim"><kbd>@</kbd> file · <kbd>Esc</kbd> or click outside</span>
          {/if}
        {/if}
      </div>

      <div
        class="editor"
        class:split={mode === 'split' && !readOnly}
        class:editing={mode !== 'preview' && !readOnly}
        class:previewing={mode === 'preview' || readOnly}
        class:height-locked={paneHeight != null}
      >
        {#if mode !== 'preview' && !readOnly}
          <textarea
            bind:this={textareaEl}
            bind:value={text}
            oninput={onEdit}
            onkeydown={onTextareaKeydown}
            spellcheck="false"
            placeholder="# Description&#10;&#10;Write the ticket body in Markdown…&#10;&#10;Type @ to link a file"
            style:height={paneHeight ? `${paneHeight}px` : undefined}
          ></textarea>
        {/if}
        {#if mode !== 'edit' || readOnly}
          <div
            class="preview"
            class:editable={!readOnly && mode === 'preview'}
            bind:this={previewEl}
            role={!readOnly && mode === 'preview' ? 'button' : undefined}
            tabindex={!readOnly && mode === 'preview' ? 0 : undefined}
            title={!readOnly && mode === 'preview' ? 'Double-click to edit' : undefined}
            ondblclick={onPreviewDblClick}
            onkeydown={(e) => {
              if (readOnly || mode !== 'preview') return;
              if (e.key === 'Enter') { e.preventDefault(); startEdit('edit'); }
            }}
            onchange={onChecklistChange}
            style:height={paneHeight ? `${paneHeight}px` : undefined}
          >{@html readOnly || dirty ? preview.replaceAll('<input ', '<input disabled ') : preview}</div>
        {/if}
      </div>
    </div>

    {#if ticket.attachments.length}
      <div class="attachments">
        {#each ticket.attachments as a}
          <div class="att">
            {#if !readOnly}
              <button
                type="button"
                class="att-del"
                title="Remove {a.name}"
                aria-label="Remove {a.name}"
                onclick={(e) => { e.stopPropagation(); removeAttachment(a.name); }}
              >×</button>
            {/if}
            {#if a.kind === 'image'}
              <button class="imgbtn" onclick={() => (lightbox = a.url)}><img src={a.url} alt={a.name} /></button>
            {:else if a.kind === 'video'}
              <video src={a.url} controls preload="metadata"></video>
            {:else}
              <a href={a.url}>{a.name}</a>
            {/if}
            <span class="aname">{a.name} · {bytes(a.size)}</span>
          </div>
        {/each}
      </div>
    {/if}
  </div>

  {#if lightbox}
    <div class="lightbox" onclick={() => (lightbox = null)}>
      <img src={lightbox} alt="attachment" />
    </div>
  {/if}

  {#if fileMention.open}
    <div data-file-mention>
      <FileMentionPopup
        items={fileMention.items}
        active={fileMention.active}
        loading={fileMention.loading}
        top={fileMention.top}
        left={fileMention.left}
        onSelect={(item) =>
          fileMention.select(item, textareaEl, text, (next, caret) => {
            text = next;
            tick().then(() => {
              fitTextarea();
              textareaEl?.focus();
              textareaEl?.setSelectionRange(caret, caret);
            });
            if (saveTimer) clearTimeout(saveTimer);
            saveTimer = setTimeout(() => { if (dirty) save(); }, 2000);
          })
        }
        onHover={(i) => (fileMention.active = i)}
      />
    </div>
  {/if}
{/if}

<style>
  .page { padding: 16px 24px; max-width: 900px; }
  .missing { padding: 40px; color: var(--color-dim); }
  .bar { display: flex; align-items: center; gap: 10px; }
  .back { text-decoration: none; color: var(--color-dim); font-size: 18px; }
  .id { background: none; border: none; color: var(--color-dim); font-size: 12px; }
  .title { flex: 1; font-size: 20px; font-weight: 650; background: none; border: 1px solid transparent; border-radius: 6px; padding: 4px 6px; }
  .title:hover, .title:focus { border-color: var(--color-border); outline: none; }
  .spacer { flex: 0; }
  .ghost {
    background: var(--color-surface-2);
    box-shadow: 0 0 0 1px rgba(255, 255, 255, 0.08);
    border: none;
    border-radius: 6px;
    padding: 5px 10px;
    font-size: 12px;
    transition-property: scale, box-shadow;
    transition-duration: 150ms;
    transition-timing-function: ease-out;
  }
  .ghost:active:not(:disabled) { scale: 0.96; }
  .ghost.danger { color: var(--color-danger); }
  :root[data-theme='light'] .ghost { box-shadow: 0 0 0 1px rgba(0, 0, 0, 0.06), 0 1px 2px -1px rgba(0, 0, 0, 0.06); }
  .ro-banner {
    display: flex; align-items: center; gap: 10px; margin: 12px 0;
    padding: 8px 12px; font-size: 13px; border-radius: 8px;
    background: color-mix(in srgb, var(--color-dim) 10%, var(--color-surface));
    border: 1px dashed var(--color-border); color: var(--color-dim);
  }
  .ro-badge { font-family: var(--font-mono); font-size: 11px; padding: 1px 8px; border-radius: 999px; background: var(--color-surface-2); color: var(--color-dim); white-space: nowrap; }
  .ro-copy { background: var(--color-surface-2); border: 1px solid var(--color-border); border-radius: 6px; padding: 2px 8px; font-size: 11px; margin-left: auto; }
  .title[readonly] { border-color: transparent; opacity: 0.85; }
  select:disabled, .seg button:disabled { opacity: 0.6; }
  .meta { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; margin: 12px 0; }
  select { background: var(--color-surface-2); border: 1px solid var(--color-border); border-radius: 6px; padding: 4px 8px; font-size: 12px; }
  .seg { display: flex; border: 1px solid var(--color-border); border-radius: 6px; overflow: hidden; }
  .seg button { padding: 3px 8px; background: var(--color-surface-2); border: none; }
  .seg button.active { background: var(--color-accent-soft); color: var(--c); }
  .labels { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
  .chip { font-size: 11px; padding: 1px 4px 1px 8px; border-radius: 999px; background: color-mix(in srgb, var(--c) 18%, transparent); color: var(--c); display: inline-flex; align-items: center; gap: 4px; }
  .chip button { background: none; border: none; color: inherit; }
  .labels input { width: 80px; background: none; border: 1px dashed var(--color-border); border-radius: 6px; padding: 2px 6px; font-size: 11px; }
  .updated { margin-left: auto; color: var(--color-dim); font-size: 11px; }
  .epic-summary { font-size: 12px; color: var(--color-dim); margin: -4px 0 12px; }
  .conflict { background: color-mix(in srgb, var(--color-warn) 12%, var(--color-surface)); border: 1px solid var(--color-warn); border-radius: 8px; padding: 8px 12px; margin-bottom: 12px; display: flex; align-items: center; gap: 10px; font-size: 13px; }
  .conflict button { background: var(--color-surface-2); border: 1px solid var(--color-border); border-radius: 6px; padding: 4px 10px; font-size: 12px; }
  .editor-head { display: flex; align-items: center; gap: 10px; margin-bottom: 8px; min-height: 28px; }
  .edit-hint { color: var(--color-dim); font-size: 12px; }
  .edit-hint.dim { margin-left: auto; opacity: 0.75; }
  .done {
    background: var(--color-accent-soft);
    color: var(--color-accent);
    border: none;
    border-radius: 6px;
    padding: 4px 12px;
    font-size: 12px;
    font-weight: 600;
    transition-property: scale, background-color;
    transition-duration: 150ms;
    transition-timing-function: ease-out;
  }
  .done:active { scale: 0.96; }
  .modes { display: flex; box-shadow: 0 0 0 1px rgba(255, 255, 255, 0.08); border-radius: 6px; overflow: hidden; }
  .modes button {
    padding: 4px 12px;
    background: var(--color-surface-2);
    border: none;
    font-size: 12px;
    min-height: 28px;
    transition-property: background-color, color, scale;
    transition-duration: 150ms;
    transition-timing-function: ease-out;
  }
  .modes button:active { scale: 0.96; }
  .modes button.active { background: var(--color-accent-soft); color: var(--color-accent); }
  :root[data-theme='light'] .modes { box-shadow: 0 0 0 1px rgba(0, 0, 0, 0.06); }
  .dirty { color: var(--color-warn); font-size: 11px; }
  .editor {
    border-radius: 10px;
    overflow: hidden;
    min-height: 120px;
    background: var(--color-surface);
    box-shadow: 0 0 0 1px rgba(255, 255, 255, 0.08);
    transition-property: box-shadow, background-color;
    transition-duration: 180ms;
    transition-timing-function: ease-out;
  }
  .editor.previewing { background: transparent; }
  .editor.previewing:hover { box-shadow: 0 0 0 1px rgba(255, 255, 255, 0.13); }
  .editor.editing { box-shadow: 0 0 0 1px color-mix(in srgb, var(--color-accent) 45%, transparent); }
  :root[data-theme='light'] .editor {
    box-shadow:
      0 0 0 1px rgba(0, 0, 0, 0.06),
      0 1px 2px -1px rgba(0, 0, 0, 0.06),
      0 2px 4px 0 rgba(0, 0, 0, 0.04);
  }
  :root[data-theme='light'] .editor.previewing:hover {
    box-shadow:
      0 0 0 1px rgba(0, 0, 0, 0.08),
      0 1px 2px -1px rgba(0, 0, 0, 0.08),
      0 2px 4px 0 rgba(0, 0, 0, 0.06);
  }
  .editor.split { display: grid; grid-template-columns: 1fr 1fr; }
  textarea {
    display: block;
    width: 100%;
    min-height: 120px;
    box-sizing: border-box;
    padding: 16px 18px;
    background: var(--color-surface);
    border: none;
    outline: none;
    resize: vertical;
    overflow: auto;
    font-family: var(--font-sans);
    font-size: 14px;
    line-height: 1.7;
    letter-spacing: 0.01em;
  }
  .editor.split textarea {
    border-right: 1px solid var(--color-border);
    font-family: var(--font-mono);
    font-size: 13px;
  }
  .preview {
    box-sizing: border-box;
    padding: 16px 18px;
    font-size: 14px;
    line-height: 1.7;
    overflow: auto;
    text-wrap: pretty;
    transition: height 180ms ease-out;
  }
  .editor.height-locked .preview { min-height: 120px; }
  .preview.editable { cursor: text; }
  .preview.editable:focus-visible {
    outline: 2px solid var(--color-accent);
    outline-offset: -2px;
  }
  .preview :global(h1) {
    font-size: 15px;
    font-weight: 650;
    color: var(--color-dim);
    margin: 1.1em 0 0.35em;
    letter-spacing: 0.02em;
    text-wrap: balance;
  }
  .preview :global(h1:first-child) { margin-top: 0; }
  .preview :global(h2) {
    font-size: 14px;
    font-weight: 600;
    color: var(--color-dim);
    margin: 1em 0 0.3em;
    text-wrap: balance;
  }
  .preview :global(p) { margin: 0.35em 0 0.7em; }
  .preview :global(ul), .preview :global(ol) { margin: 0.35em 0 0.7em; padding-left: 1.35em; }
  .preview :global(li) { margin: 0.2em 0; }
  .preview :global(code) { font-family: var(--font-mono); background: var(--color-surface-2); padding: 1px 5px; border-radius: 4px; font-size: 0.9em; }
  .preview :global(pre) { background: var(--color-surface-2); padding: 12px; border-radius: 8px; overflow: auto; }
  .preview :global(pre code) { background: none; padding: 0; }
  .preview :global(img) {
    max-width: 100%;
    border-radius: 8px;
    outline: 1px solid rgba(255, 255, 255, 0.1);
    outline-offset: -1px;
  }
  :root[data-theme='light'] .preview :global(img) { outline-color: rgba(0, 0, 0, 0.1); }
  .preview :global(a) { color: var(--color-accent); }
  .attachments { display: flex; flex-wrap: wrap; gap: 12px; margin-top: 16px; }
  .att { position: relative; display: flex; flex-direction: column; gap: 4px; width: 140px; }
  .att-del {
    position: absolute;
    top: 4px;
    right: 4px;
    z-index: 2;
    width: 22px;
    height: 22px;
    padding: 0;
    border: none;
    border-radius: 999px;
    background: rgb(0 0 0 / 0.72);
    color: #fff;
    font-size: 14px;
    line-height: 1;
    display: grid;
    place-items: center;
    opacity: 0.85;
    transition-property: opacity, scale, background-color;
    transition-duration: 150ms;
    transition-timing-function: ease-out;
  }
  .att:hover .att-del,
  .att:focus-within .att-del { opacity: 1; }
  .att-del:hover { background: var(--color-danger); }
  .att-del:active { scale: 0.94; }
  .imgbtn {
    padding: 0;
    border: none;
    border-radius: 8px;
    overflow: hidden;
    background: none;
    box-shadow: 0 0 0 1px rgba(255, 255, 255, 0.08);
    transition-property: scale, box-shadow;
    transition-duration: 150ms;
    transition-timing-function: ease-out;
  }
  .imgbtn:active { scale: 0.96; }
  :root[data-theme='light'] .imgbtn { box-shadow: 0 0 0 1px rgba(0, 0, 0, 0.06); }
  .att img, .att video {
    width: 140px;
    height: 90px;
    object-fit: cover;
    display: block;
    outline: 1px solid rgba(255, 255, 255, 0.1);
    outline-offset: -1px;
  }
  :root[data-theme='light'] .att img,
  :root[data-theme='light'] .att video { outline-color: rgba(0, 0, 0, 0.1); }
  .aname { font-size: 10px; color: var(--color-dim); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .lightbox { position: fixed; inset: 0; background: rgb(0 0 0 / 0.8); display: grid; place-items: center; z-index: 90; padding: 40px; }
  .lightbox img { max-width: 100%; max-height: 100%; border-radius: 8px; outline: 1px solid rgba(255, 255, 255, 0.1); outline-offset: -1px; }
</style>
