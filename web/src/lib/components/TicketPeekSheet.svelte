<script lang="ts">
  import { onDestroy, tick } from 'svelte';
  import { goto } from '$app/navigation';
  import { workspace } from '$lib/workspace.svelte';
  import { toasts } from '$lib/toast.svelte';
  import { api, ApiError, type Ticket } from '$lib/api';
  import { priorityMeta, labelColor } from '$lib/ui-helpers';
  import { renderMarkdown } from '$lib/markdown';
  import { bytes } from '$lib/format';
  import { reconcileEditor } from '$lib/ticket-editor';
  import {
    insertAtCursor,
    replaceAll,
    stripAttachmentMarkdown,
    uploadingPlaceholder
  } from '$lib/insert-at-cursor';
  import PrioritySeg from '$lib/components/PrioritySeg.svelte';

  let {
    ticketId,
    onClose
  }: {
    ticketId: string;
    onClose: () => void;
  } = $props();

  const t = $derived(workspace.tickets[ticketId]);
  const readOnly = $derived(!!t?.readOnly);
  const statuses = $derived(workspace.columns.map((c) => ({ status: c.status, title: c.title })));
  const pm = $derived(t ? priorityMeta(t.priority) : null);

  let dialogEl = $state<HTMLDivElement | null>(null);
  let titleEl = $state<HTMLInputElement | null>(null);
  let textareaEl = $state<HTMLTextAreaElement | null>(null);
  let mode = $state<'preview' | 'edit'>('preview');
  let text = $state('');
  let baseHash = $state('');
  let baseBody = $state('');
  let dirty = $derived(text !== baseBody);
  let conflict = $state<Ticket | null>(null);
  let saveTimer: ReturnType<typeof setTimeout> | null = null;
  let labelInput = $state('');
  let lightbox = $state<string | null>(null);
  let lastTicketId = '';

  async function save(force = false) {
    if (!t || readOnly) return;
    if (saveTimer) {
      clearTimeout(saveTimer);
      saveTimer = null;
    }
    const ifMatch = force ? (conflict?.hash ?? baseHash) : baseHash;
    const opId = workspace.beginOp();
    try {
      const updated = await api.patchTicket(t.id, { body: text, opId }, ifMatch);
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

  /** Flush dirty body — called before close / peek-target switch. Exported for parent. */
  export async function flushPending() {
    if (saveTimer) {
      clearTimeout(saveTimer);
      saveTimer = null;
    }
    if (dirty && !conflict && !readOnly && t) {
      await save();
    }
  }

  async function requestClose() {
    await flushPending();
    onClose();
  }

  function onBodyInput() {
    if (readOnly) return;
    if (saveTimer) clearTimeout(saveTimer);
    saveTimer = setTimeout(() => {
      if (dirty) save();
    }, 2000);
  }

  async function startEdit() {
    if (readOnly) return;
    mode = 'edit';
    await tick();
    textareaEl?.focus();
  }

  async function finishEdit() {
    // Match full page: leave edit mode immediately, save in background.
    mode = 'preview';
    if (dirty && !conflict) await save();
  }

  const returnFocus = typeof document !== 'undefined' ? (document.activeElement as HTMLElement | null) : null;
  onDestroy(() => {
    // Last-resort flush if parent tore us down without requestClose.
    if (saveTimer) {
      clearTimeout(saveTimer);
      saveTimer = null;
    }
    if (dirty && !conflict && !readOnly && t) {
      const id = t.id;
      const body = text;
      const hash = baseHash;
      const opId = workspace.beginOp();
      void api.patchTicket(id, { body, opId }, hash).then((updated) => {
        workspace.tickets = { ...workspace.tickets, [updated.id]: updated };
      }).catch(() => {
        /* best-effort on unmount */
      });
    }
    returnFocus?.focus?.();
  });

  $effect(() => {
    dialogEl?.focus();
  });

  // Load / rebase body when the ticket identity or disk version changes.
  $effect(() => {
    const ticket = t;
    if (!ticket) return;
    if (ticket.id !== lastTicketId) {
      // Flush the previous ticket before swapping editor buffers.
      if (lastTicketId && dirty && !conflict) {
        const prevId = lastTicketId;
        const prevBody = text;
        const prevHash = baseHash;
        const opId = workspace.beginOp();
        void api.patchTicket(prevId, { body: prevBody, opId }, prevHash).then((updated) => {
          workspace.tickets = { ...workspace.tickets, [updated.id]: updated };
        }).catch(() => {
          /* best-effort on switch */
        });
      }
      lastTicketId = ticket.id;
      text = ticket.body ?? '';
      baseBody = ticket.body ?? '';
      baseHash = ticket.hash;
      conflict = null;
      mode = 'preview';
      return;
    }
    if (baseHash === '') {
      text = ticket.body ?? '';
      baseBody = ticket.body ?? '';
      baseHash = ticket.hash;
      return;
    }
    if (ticket.hash !== baseHash) {
      const r = reconcileEditor({ text, baseBody, baseHash, ticket });
      text = r.text;
      baseBody = r.baseBody;
      baseHash = r.baseHash;
      conflict = r.conflict;
    }
  });

  async function setField(patch: Record<string, unknown>) {
    if (!t || readOnly) return;
    try {
      await workspace.patch(t.id, patch);
    } catch (e) {
      toasts.push(e instanceof Error ? e.message : 'Update failed', 'error');
    }
  }

  function commitTitle(value: string) {
    if (!t || readOnly) return;
    const next = value.trim() || t.title;
    if (next === t.title) return;
    setField({ title: next });
  }

  function addLabel() {
    const l = labelInput.trim();
    labelInput = '';
    if (!l || !t || readOnly || t.labels.includes(l)) return;
    setField({ labels: [...t.labels, l] });
  }

  function removeLabel(l: string) {
    if (!t || readOnly) return;
    setField({ labels: t.labels.filter((x) => x !== l) });
  }

  async function reloadFromDisk() {
    if (!conflict || !t) return;
    let disk = conflict;
    try {
      disk = await api.getTicket(conflict.id);
    } catch {
      /* keep conflicted snapshot */
    }
    text = disk.body ?? '';
    baseBody = disk.body ?? '';
    baseHash = disk.hash;
    workspace.tickets = { ...workspace.tickets, [disk.id]: disk };
    conflict = null;
  }

  async function uploadFiles(files: File[]) {
    if (!files.length || readOnly || !t) return;
    const fromPreview = mode === 'preview';
    if (fromPreview) await startEdit();
    const names = files.map((f, i) => f.name || `paste-${i + 1}.png`);
    const placeholders = names.map((n) => uploadingPlaceholder(n));
    let value = text;
    let caret = fromPreview ? value.length : (textareaEl?.selectionStart ?? value.length);
    for (const ph of placeholders) {
      const pad = caret > 0 && value[caret - 1] !== '\n' ? '\n\n' : '';
      const r = insertAtCursor(value, pad + ph, { selectionStart: caret, selectionEnd: caret });
      value = r.value;
      caret = r.caret;
    }
    text = value;
    await tick();
    textareaEl?.focus();
    textareaEl?.setSelectionRange(caret, caret);

    try {
      const results = await api.upload(t.id, files, { opId: workspace.beginOp() });
      let next = text;
      for (let i = 0; i < placeholders.length; i++) {
        const r = results[i];
        if (r && !r.error && r.markdown) next = replaceAll(next, placeholders[i], r.markdown);
        else next = replaceAll(next, placeholders[i], '');
      }
      text = next.replace(/\n{3,}/g, '\n\n');
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

  async function removeAttachment(name: string) {
    if (!t || readOnly) return;
    try {
      await api.deleteAttachment(t.id, name, workspace.beginOp());
      const next = stripAttachmentMarkdown(text, t.id, name);
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

  function focusables(): HTMLElement[] {
    if (!dialogEl) return [];
    return Array.from(
      dialogEl.querySelectorAll<HTMLElement>(
        'a[href], button, select, input, textarea, [tabindex]:not([tabindex="-1"])'
      )
    ).filter((el) => !el.hasAttribute('disabled') && el.getAttribute('aria-hidden') !== 'true');
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.preventDefault();
      e.stopPropagation();
      if (lightbox) {
        lightbox = null;
        return;
      }
      if (mode === 'edit' && !readOnly) {
        void finishEdit();
        return;
      }
      void requestClose();
      return;
    }
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 's') {
      if (readOnly || mode !== 'edit') return;
      e.preventDefault();
      e.stopPropagation();
      void save();
      return;
    }
    if (e.key === 'Tab' && dialogEl) {
      e.stopPropagation();
      const list = focusables();
      if (list.length === 0) return;
      const first = list[0];
      const last = list[list.length - 1];
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

  const previewHtml = $derived(renderMarkdown(text));
</script>

<div
  class="backdrop"
  role="presentation"
  data-testid="ticket-peek-backdrop"
  onmousedown={(e) => {
    if (e.target === e.currentTarget) void requestClose();
  }}
>
  <div
    bind:this={dialogEl}
    class="sheet"
    role="dialog"
    aria-modal="true"
    aria-label={t ? `Peek ${t.id}` : 'Ticket peek'}
    tabindex="-1"
    data-testid="ticket-peek-sheet"
    onkeydown={onKey}
  >
    {#if !t}
      <p class="missing">Ticket {ticketId} not found in workspace.</p>
      <button type="button" class="ghost" onclick={() => void requestClose()}>Close</button>
    {:else}
      <header class="head">
        <div class="ids">
          {#if pm}
            <span class="prio" style={`color:${pm.color}`} aria-label={pm.label}>{pm.short}</span>
          {/if}
          <span class="id">{t.id}</span>
          {#if t.readOnly}<span class="ro">read-only</span>{/if}
        </div>
        <button type="button" class="close" aria-label="Close" onclick={() => void requestClose()}>×</button>
      </header>

      {#if readOnly && t.branch}
        <div class="ro-banner" data-testid="peek-ro-banner">
          Read-only on <strong>{t.branch}</strong>
        </div>
      {/if}

      <input
        bind:this={titleEl}
        class="title"
        data-testid="peek-title"
        value={t.title}
        readonly={readOnly}
        aria-label="Title"
        onblur={(e) => commitTitle(e.currentTarget.value)}
        onkeydown={(e) => {
          if (e.key === 'Enter') {
            e.preventDefault();
            commitTitle(e.currentTarget.value);
            e.currentTarget.blur();
          }
        }}
      />

      <div class="field">
        <label class="label" for="peek-status">Status</label>
        <select
          id="peek-status"
          data-testid="peek-status"
          value={t.status}
          disabled={readOnly}
          onchange={(e) => setField({ status: e.currentTarget.value })}
        >
          {#each statuses as s}
            <option value={s.status}>{s.title}</option>
          {/each}
          {#if !statuses.some((s) => s.status === t.status)}
            <option value={t.status}>{t.status}</option>
          {/if}
        </select>
      </div>

      <div class="field">
        <span class="label">Priority</span>
        <PrioritySeg
          value={t.priority}
          disabled={readOnly}
          testIdPrefix="peek-prio"
          onChange={(p) => setField({ priority: p })}
        />
      </div>

      <div class="field">
        <span class="label">Labels</span>
        <div class="labels" data-testid="peek-labels">
          {#each t.labels as l}
            <span class="chip" style={`--c: ${labelColor(l)}`}>
              {l}
              {#if !readOnly}
                <button type="button" aria-label={`Remove ${l}`} data-testid={`peek-label-rm-${l}`} onclick={() => removeLabel(l)}>×</button>
              {/if}
            </span>
          {/each}
          {#if !readOnly}
            <input
              bind:value={labelInput}
              placeholder="+ label"
              aria-label="Add label"
              data-testid="peek-label-input"
              onkeydown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault();
                  addLabel();
                }
              }}
            />
          {/if}
        </div>
      </div>

      {#if conflict}
        <div class="conflict" data-testid="peek-conflict" role="alert">
          Changed on disk.
          <button type="button" onclick={reloadFromDisk}>Reload</button>
          <button type="button" onclick={() => save(true)}>Overwrite</button>
        </div>
      {/if}

      <div class="field body-field">
        <div class="body-head">
          <span class="label">Description</span>
          {#if !readOnly}
            <div class="body-actions">
              {#if mode === 'preview'}
                <button type="button" class="linkish" data-testid="peek-edit-body" onclick={startEdit}>Edit</button>
              {:else}
                <button type="button" class="linkish" data-testid="peek-done-body" onclick={finishEdit}>Done</button>
                {#if dirty}<span class="dirty">unsaved · ⌘S</span>{/if}
              {/if}
              <label class="attach-btn">
                <input
                  type="file"
                  accept="image/*,video/*"
                  multiple
                  hidden
                  data-testid="peek-attach-input"
                  onchange={(e) => {
                    const input = e.currentTarget as HTMLInputElement;
                    if (input.files?.length) uploadFiles(Array.from(input.files));
                    input.value = '';
                  }}
                />
                Attach
              </label>
            </div>
          {/if}
        </div>
        {#if mode === 'edit' && !readOnly}
          <textarea
            bind:this={textareaEl}
            bind:value={text}
            data-testid="peek-body-edit"
            class="body-edit"
            spellcheck="false"
            placeholder="Write the ticket body in Markdown…"
            oninput={onBodyInput}
          ></textarea>
        {:else}
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <div
            class="body preview"
            class:editable={!readOnly}
            data-testid="peek-body-preview"
            title={!readOnly ? 'Double-click to edit' : undefined}
            ondblclick={() => {
              if (!readOnly) startEdit();
            }}
          >
            {#if text.trim()}
              {@html previewHtml}
            {:else}
              <span class="empty">No description yet.</span>
            {/if}
          </div>
        {/if}
      </div>

      {#if t.attachments.length}
        <div class="field">
          <span class="label">Attachments</span>
          <div class="attachments" data-testid="peek-attachments">
            {#each t.attachments as a}
              <div class="att">
                {#if !readOnly}
                  <button
                    type="button"
                    class="att-del"
                    title={`Remove ${a.name}`}
                    aria-label={`Remove ${a.name}`}
                    data-testid={`peek-att-del-${a.name}`}
                    onclick={() => removeAttachment(a.name)}
                  >×</button>
                {/if}
                {#if a.kind === 'image'}
                  <button type="button" class="imgbtn" onclick={() => (lightbox = a.url)}>
                    <img src={a.url} alt={a.name} />
                  </button>
                {:else if a.kind === 'video'}
                  <!-- svelte-ignore a11y_media_has_caption -->
                  <video src={a.url} controls preload="metadata"></video>
                {:else}
                  <a href={a.url}>{a.name}</a>
                {/if}
                <span class="aname">{a.name} · {bytes(a.size)}</span>
              </div>
            {/each}
          </div>
        </div>
      {/if}

      <footer class="foot">
        <a
          href={`/tickets/${t.id}`}
          data-testid="peek-open"
          class="open"
          onclick={(e) => {
            // Flush first; let the browser navigate after close.
            e.preventDefault();
            const href = `/tickets/${t.id}`;
            void requestClose().then(() => {
              goto(href);
            });
          }}
        >Open full page</a>
        <button type="button" class="ghost" data-testid="peek-close" onclick={() => void requestClose()}>Close</button>
      </footer>
    {/if}
  </div>
</div>

{#if lightbox}
  <div
    class="lightbox"
    role="dialog"
    aria-modal="true"
    aria-label="Attachment preview"
    tabindex="-1"
    data-testid="peek-lightbox"
    onclick={() => (lightbox = null)}
    onkeydown={(e) => {
      if (e.key === 'Escape' || e.key === 'Enter' || e.key === ' ') {
        e.preventDefault();
        lightbox = null;
      }
    }}
  >
    <img src={lightbox} alt="attachment" />
  </div>
{/if}

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    z-index: 90;
    background: rgb(0 0 0 / 0.35);
    display: flex;
    justify-content: flex-end;
  }
  .sheet {
    width: min(440px, 100vw);
    height: 100%;
    background: var(--color-surface);
    border-left: 1px solid var(--color-border);
    box-shadow: -8px 0 32px rgb(0 0 0 / 0.25);
    padding: 16px 18px;
    overflow: auto;
    outline: none;
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
  .head {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .ids {
    display: flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
  }
  .id {
    font-family: var(--font-mono);
    font-size: 12px;
    color: var(--color-dim);
  }
  .prio {
    font-size: 11px;
    font-weight: 650;
    letter-spacing: 0.02em;
  }
  .ro {
    font-size: 10px;
    padding: 1px 6px;
    border-radius: 999px;
    background: var(--color-surface-2);
    color: var(--color-dim);
  }
  .ro-banner {
    font-size: 11px;
    line-height: 1.35;
    padding: 6px 8px;
    border-radius: 6px;
    background: var(--color-surface-2);
    color: var(--color-dim);
  }
  .close {
    margin-left: auto;
    background: none;
    border: none;
    font-size: 20px;
    line-height: 1;
    color: var(--color-dim);
    cursor: pointer;
  }
  .close:hover {
    color: var(--color-text);
  }
  .title {
    margin: 0;
    width: 100%;
    font-size: 16px;
    font-weight: 650;
    line-height: 1.35;
    background: transparent;
    border: 1px solid transparent;
    border-radius: 6px;
    padding: 4px 6px;
    color: var(--color-text);
  }
  .title:hover:not([readonly]),
  .title:focus {
    border-color: var(--color-border);
    background: var(--color-bg);
    outline: none;
  }
  .title[readonly] {
    cursor: default;
  }
  .field {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .label {
    font-size: 10px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--color-dim);
  }
  select {
    background: var(--color-bg);
    border: 1px solid var(--color-border);
    border-radius: 6px;
    padding: 6px 8px;
    font-size: 12px;
  }
  .labels {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px;
  }
  .chip {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    font-size: 11px;
    padding: 2px 8px;
    border-radius: 999px;
    background: color-mix(in oklab, var(--c) 18%, var(--color-surface-2));
    color: var(--color-text);
  }
  .chip button {
    background: none;
    border: none;
    color: var(--color-dim);
    cursor: pointer;
    padding: 0;
    line-height: 1;
  }
  .chip button:hover {
    color: var(--color-danger, #e55);
  }
  .labels input {
    width: 72px;
    background: var(--color-bg);
    border: 1px solid var(--color-border);
    border-radius: 6px;
    padding: 3px 6px;
    font-size: 11px;
  }
  .body-head {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .body-head .label {
    margin-right: auto;
  }
  .body-actions {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .linkish {
    background: none;
    border: none;
    color: var(--color-accent);
    font-size: 11px;
    font-weight: 600;
    cursor: pointer;
    padding: 0;
  }
  .dirty {
    font-size: 10px;
    color: var(--color-warn, #c90);
  }
  .attach-btn {
    font-size: 11px;
    font-weight: 600;
    color: var(--color-dim);
    cursor: pointer;
  }
  .attach-btn:hover {
    color: var(--color-accent);
  }
  .body {
    font-size: 13px;
    line-height: 1.45;
    color: var(--color-text);
    max-height: 36vh;
    overflow: auto;
    padding: 8px 10px;
    border-radius: 8px;
    background: var(--color-bg);
    border: 1px solid var(--color-border);
  }
  .body.editable {
    cursor: text;
  }
  .body.editable:hover {
    border-color: var(--color-accent);
  }
  .body :global(h1),
  .body :global(h2),
  .body :global(h3) {
    font-size: 13px;
    margin: 0.6em 0 0.3em;
  }
  .body :global(p) {
    margin: 0.4em 0;
  }
  .empty {
    color: var(--color-dim);
    font-style: italic;
  }
  .body-edit {
    width: 100%;
    min-height: 180px;
    max-height: 42vh;
    resize: vertical;
    font-family: var(--font-mono);
    font-size: 12px;
    line-height: 1.45;
    padding: 8px 10px;
    border-radius: 8px;
    background: var(--color-bg);
    border: 1px solid var(--color-border);
    color: var(--color-text);
  }
  .body-edit:focus {
    outline: none;
    border-color: var(--color-accent);
  }
  .conflict {
    font-size: 12px;
    padding: 8px 10px;
    border-radius: 8px;
    background: color-mix(in oklab, var(--color-warn, #c90) 18%, var(--color-surface));
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    align-items: center;
  }
  .conflict button {
    background: var(--color-surface);
    border: 1px solid var(--color-border);
    border-radius: 4px;
    padding: 2px 8px;
    font-size: 11px;
    cursor: pointer;
  }
  .attachments {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .att {
    position: relative;
    display: flex;
    flex-direction: column;
    gap: 4px;
    padding: 6px;
    border-radius: 8px;
    background: var(--color-bg);
    border: 1px solid var(--color-border);
  }
  .att-del {
    position: absolute;
    top: 4px;
    right: 4px;
    background: var(--color-surface);
    border: 1px solid var(--color-border);
    border-radius: 4px;
    width: 22px;
    height: 22px;
    line-height: 1;
    cursor: pointer;
    color: var(--color-dim);
  }
  .att-del:hover {
    color: var(--color-danger, #e55);
  }
  .imgbtn {
    background: none;
    border: none;
    padding: 0;
    cursor: zoom-in;
    text-align: left;
  }
  .att img,
  .att video {
    max-width: 100%;
    max-height: 120px;
    border-radius: 4px;
    object-fit: contain;
  }
  .aname {
    font-size: 10px;
    color: var(--color-dim);
    word-break: break-all;
  }
  .foot {
    margin-top: auto;
    display: flex;
    align-items: center;
    gap: 10px;
    padding-top: 8px;
    border-top: 1px solid var(--color-border);
  }
  .open {
    background: var(--color-accent-soft);
    color: var(--color-accent);
    text-decoration: none;
    border-radius: 6px;
    padding: 6px 12px;
    font-size: 12px;
    font-weight: 600;
  }
  .ghost {
    margin-left: auto;
    background: none;
    border: none;
    color: var(--color-dim);
    font-size: 12px;
    cursor: pointer;
  }
  .ghost:hover {
    color: var(--color-text);
  }
  .missing {
    color: var(--color-dim);
    font-size: 13px;
  }
  .lightbox {
    position: fixed;
    inset: 0;
    z-index: 100;
    background: rgb(0 0 0 / 0.75);
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 24px;
  }
  .lightbox img {
    max-width: 100%;
    max-height: 100%;
    object-fit: contain;
  }
</style>
