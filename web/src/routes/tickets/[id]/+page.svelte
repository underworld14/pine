<script lang="ts">
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { workspace } from '$lib/workspace.svelte';
  import { toasts } from '$lib/toast.svelte';
  import { api, ApiError, type Ticket } from '$lib/api';
  import { renderMarkdown } from '$lib/markdown';
  import { priorityMeta, labelColor } from '$lib/ui-helpers';
  import { relTime, bytes } from '$lib/format';

  const id = $derived($page.params.id);
  const ticket = $derived(workspace.get(id));

  let mode = $state<'preview' | 'split' | 'edit'>('preview');
  let text = $state('');
  let baseHash = $state('');
  let baseBody = $state(''); // the body at baseHash — dirtiness is measured against this
  let dirty = $derived(text !== baseBody);
  let conflict = $state<Ticket | null>(null);
  let saveTimer: ReturnType<typeof setTimeout> | null = null;

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
      // Disk changed under us. If the editor is clean (unchanged from our base),
      // adopt the new version silently — the "an agent rewrote it while I watched"
      // moment. If the editor is dirty, flag a conflict instead of clobbering.
      if (text === baseBody) {
        text = t.body ?? '';
        baseBody = t.body ?? '';
        baseHash = t.hash;
        conflict = null;
      } else {
        conflict = t;
      }
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
    }
  });

  const preview = $derived(renderMarkdown(mode === 'edit' ? '' : text));

  async function save(force = false) {
    if (!ticket) return;
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
    if (saveTimer) clearTimeout(saveTimer);
    saveTimer = setTimeout(() => { if (dirty) save(); }, 2000);
  }

  function onKeydown(e: KeyboardEvent) {
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 's') { e.preventDefault(); save(); }
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'e') { e.preventDefault(); mode = mode === 'edit' ? 'preview' : mode === 'preview' ? 'split' : 'edit'; }
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
    try { await workspace.patch(id, patch); } catch (e) { toasts.push('Update failed', 'error'); }
  }

  async function copyPrompt() {
    const p = await api.ticketPrompt(id);
    await navigator.clipboard.writeText(p);
    toasts.push('Prompt copied — paste into your AI agent', 'success');
  }

  async function del() {
    if (!confirm(`Delete ${id}?`)) return;
    await workspace.remove(id);
    goto('/board');
  }

  let labelInput = $state('');
  async function addLabel() {
    const l = labelInput.trim();
    if (!l || !ticket) return;
    await setField({ labels: [...ticket.labels, l] });
    labelInput = '';
  }
  async function removeLabel(l: string) {
    if (!ticket) return;
    await setField({ labels: ticket.labels.filter((x) => x !== l) });
  }

  async function uploadFiles(files: File[]) {
    if (!files.length) return;
    try {
      const results = await api.upload(id, files, { opId: workspace.beginOp() });
      const ok = results.filter((r) => !r.error);
      if (ok.length) {
        const md = ok.map((r) => r.markdown).join('\n');
        text = text.trimEnd() + '\n\n' + md + '\n';
        await save();
        const saved = ok.reduce((a, r) => a + (r.originalBytes - r.finalBytes), 0);
        toasts.push(saved > 0 ? `Attached · saved ${bytes(saved)}` : 'Attached', 'success');
      }
    } catch { toasts.push('Upload failed', 'error'); }
  }

  function onPaste(e: ClipboardEvent) {
    const imgs: File[] = [];
    for (const it of e.clipboardData?.items ?? []) {
      if (it.type.startsWith('image/')) { const f = it.getAsFile(); if (f) imgs.push(f); }
    }
    if (imgs.length) { e.preventDefault(); uploadFiles(imgs); }
  }

  let lightbox = $state<string | null>(null);
</script>

<svelte:window onkeydown={onKeydown} />

{#if !ticket}
  <div class="missing">Ticket <span class="mono">{id}</span> not found. <a href="/board">Back to board</a></div>
{:else}
  <div class="page" onpaste={onPaste}>
    <div class="bar">
      <a href="/board" class="back">←</a>
      <button class="mono id" onclick={() => navigator.clipboard.writeText(ticket.id)} title="Copy id">{ticket.id}</button>
      <input class="title" value={ticket.title} onblur={(e) => setField({ title: (e.target as HTMLInputElement).value })} />
      <span class="spacer"></span>
      <button class="ghost" onclick={copyPrompt}>Copy AI prompt</button>
      <button class="ghost danger" onclick={del}>Delete</button>
    </div>

    <div class="meta">
      <select value={ticket.status} onchange={(e) => setField({ status: (e.target as HTMLSelectElement).value })}>
        {#each workspace.board?.columns ?? [] as c}<option value={c.status}>{c.title}</option>{/each}
        {#if !(workspace.board?.columns ?? []).some((c) => c.status === ticket.status)}<option value={ticket.status}>{ticket.status}</option>{/if}
      </select>
      <div class="seg">
        {#each ['low', 'medium', 'high', 'critical'] as p}
          <button class:active={ticket.priority === p} style="--c: {priorityMeta(p).color}" onclick={() => setField({ priority: p })} title={p}>{priorityMeta(p).glyph}</button>
        {/each}
      </div>
      <div class="labels">
        {#each ticket.labels as l}
          <span class="chip" style="--c: {labelColor(l)}">{l}<button onclick={() => removeLabel(l)}>×</button></span>
        {/each}
        <input bind:value={labelInput} placeholder="+ label" onkeydown={(e) => { if (e.key === 'Enter') addLabel(); }} />
      </div>
      <span class="updated">updated {relTime(ticket.updated)}</span>
    </div>

    {#if ticket.parent || ticket.deps.length}
      <div class="rels">
        {#if ticket.parent}<span>epic: <a class="mono" href={`/tickets/${ticket.parent}`}>{ticket.parent}</a></span>{/if}
        {#each ticket.deps as d}
          {@const dep = workspace.get(d)}
          <span class="dep" class:unmet={dep && dep.status !== 'done'}>dep: <a class="mono" href={`/tickets/${d}`}>{d}</a>{#if dep} <em>({dep.status})</em>{/if}</span>
        {/each}
        {#if ticket.blocked}<span class="blocked">🔒 blocked</span>{/if}
      </div>
    {/if}

    {#if ticket.epicProgress}
      <div class="epic">
        <strong>Children</strong> ({ticket.epicProgress.done}/{ticket.epicProgress.total} done)
        {#each ticket.children ?? [] as c}
          <a class="child" href={`/tickets/${c.id}`}><span class="mono">{c.id}</span> <span class="cstatus">[{c.status}]</span> {c.title}</a>
        {/each}
      </div>
    {/if}

    {#if conflict}
      <div class="conflict">
        ⚠ Changed on disk (probably an AI agent).
        <button onclick={reloadFromDisk}>Reload from disk</button>
        <button onclick={() => save(true)}>Keep mine & overwrite</button>
      </div>
    {/if}

    <div class="editor-head">
      <div class="modes">
        {#each ['preview', 'split', 'edit'] as m}
          <button class:active={mode === m} onclick={() => (mode = m as typeof mode)}>{m}</button>
        {/each}
      </div>
      {#if dirty}<span class="dirty">unsaved · <kbd>⌘S</kbd></span>{/if}
    </div>

    <div class="editor" class:split={mode === 'split'}>
      {#if mode !== 'preview'}
        <textarea bind:value={text} oninput={onEdit} spellcheck="false"></textarea>
      {/if}
      {#if mode !== 'edit'}
        <div class="preview">{@html preview}</div>
      {/if}
    </div>

    {#if ticket.attachments.length}
      <div class="attachments">
        {#each ticket.attachments as a}
          <div class="att">
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
  .ghost { background: var(--color-surface-2); border: 1px solid var(--color-border); border-radius: 6px; padding: 5px 10px; font-size: 12px; }
  .ghost.danger { color: var(--color-danger); }
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
  .rels { display: flex; gap: 14px; flex-wrap: wrap; font-size: 12px; color: var(--color-dim); margin-bottom: 10px; }
  .rels a { color: var(--color-accent); text-decoration: none; }
  .dep.unmet { color: var(--color-warn); }
  .blocked { color: var(--color-danger); }
  .epic { background: var(--color-surface); border: 1px solid var(--color-border); border-radius: 8px; padding: 10px 12px; margin-bottom: 12px; font-size: 13px; }
  .epic strong { font-size: 12px; }
  .child { display: block; margin-top: 4px; text-decoration: none; color: inherit; }
  .cstatus { color: var(--color-dim); font-size: 11px; }
  .conflict { background: color-mix(in srgb, var(--color-warn) 12%, var(--color-surface)); border: 1px solid var(--color-warn); border-radius: 8px; padding: 8px 12px; margin-bottom: 12px; display: flex; align-items: center; gap: 10px; font-size: 13px; }
  .conflict button { background: var(--color-surface-2); border: 1px solid var(--color-border); border-radius: 6px; padding: 4px 10px; font-size: 12px; }
  .editor-head { display: flex; align-items: center; gap: 12px; margin-bottom: 6px; }
  .modes { display: flex; border: 1px solid var(--color-border); border-radius: 6px; overflow: hidden; }
  .modes button { padding: 3px 12px; background: var(--color-surface-2); border: none; font-size: 12px; text-transform: capitalize; }
  .modes button.active { background: var(--color-accent-soft); color: var(--color-accent); }
  .dirty { color: var(--color-warn); font-size: 11px; }
  .editor { border: 1px solid var(--color-border); border-radius: 8px; overflow: hidden; min-height: 300px; }
  .editor.split { display: grid; grid-template-columns: 1fr 1fr; }
  textarea { width: 100%; min-height: 300px; padding: 14px; background: var(--color-bg); border: none; outline: none; resize: vertical; font-family: var(--font-mono); font-size: 13px; line-height: 1.6; }
  .editor.split textarea { border-right: 1px solid var(--color-border); }
  .preview { padding: 14px 18px; font-size: 14px; line-height: 1.6; overflow: auto; }
  .preview :global(h1) { font-size: 18px; margin: 0.6em 0 0.3em; }
  .preview :global(h2) { font-size: 15px; color: var(--color-dim); margin: 0.8em 0 0.3em; }
  .preview :global(code) { font-family: var(--font-mono); background: var(--color-surface-2); padding: 1px 5px; border-radius: 4px; font-size: 0.9em; }
  .preview :global(pre) { background: var(--color-surface); padding: 12px; border-radius: 8px; overflow: auto; }
  .preview :global(img) { max-width: 100%; border-radius: 8px; }
  .preview :global(a) { color: var(--color-accent); }
  .attachments { display: flex; flex-wrap: wrap; gap: 12px; margin-top: 16px; }
  .att { display: flex; flex-direction: column; gap: 4px; width: 140px; }
  .imgbtn { padding: 0; border: 1px solid var(--color-border); border-radius: 8px; overflow: hidden; background: none; }
  .att img, .att video { width: 140px; height: 90px; object-fit: cover; display: block; }
  .aname { font-size: 10px; color: var(--color-dim); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .lightbox { position: fixed; inset: 0; background: rgb(0 0 0 / 0.8); display: grid; place-items: center; z-index: 90; padding: 40px; }
  .lightbox img { max-width: 100%; max-height: 100%; border-radius: 8px; }
</style>
