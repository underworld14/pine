<script lang="ts">
  import '../app.css';
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { workspace } from '$lib/workspace.svelte';
  import { ui } from '$lib/ui.svelte';
  import NewIssueModal from '$lib/components/NewIssueModal.svelte';
  import CommandPalette from '$lib/components/CommandPalette.svelte';
  import Toasts from '$lib/components/Toasts.svelte';

  let { children } = $props();
  let gSeq = 0;

  onMount(() => {
    ui.initTheme();
    workspace.hydrate().catch(() => {});
    workspace.startLive();
    return () => workspace.stopLive();
  });

  function isTyping(t: EventTarget | null): boolean {
    const el = t as HTMLElement | null;
    return !!el && (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.isContentEditable);
  }

  function onKeydown(e: KeyboardEvent) {
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
      e.preventDefault();
      ui.paletteOpen = !ui.paletteOpen;
      return;
    }
    if (isTyping(e.target) || e.metaKey || e.ctrlKey || e.altKey) return;
    if (e.key === 'c') { e.preventDefault(); ui.openModal({ type: 'bug' }); }
    else if (e.key === '/') { e.preventDefault(); goto('/search'); }
    else if (e.key === 'g') { gSeq = Date.now(); }
    else if (e.key === 'd' && Date.now() - gSeq < 800) { goto('/'); }
    else if (e.key === 'b' && Date.now() - gSeq < 800) { goto('/board'); }
  }

  import { goto } from '$app/navigation';

  const project = $derived(workspace.config?.project.name ?? 'Pine');
  const git = $derived(workspace.git);
  const path = $derived($page.url.pathname);
</script>

<svelte:window onkeydown={onKeydown} />

<div class="app">
  <aside class="sidebar">
    <div class="brand">🌲 <span>{project}</span></div>
    <button class="new" onclick={() => ui.openModal({ type: 'bug' })}>+ New issue <kbd>c</kbd></button>
    <nav>
      <a href="/" class:active={path === '/'}>Dashboard <kbd>g d</kbd></a>
      <a href="/board" class:active={path.startsWith('/board')}>Board <kbd>g b</kbd></a>
      <a href="/search" class:active={path.startsWith('/search')}>Search <kbd>/</kbd></a>
    </nav>
    <div class="footer">
      {#if git?.isRepo}
        <div class="git" title={git.changes.map((c) => c.path).join('\n')}>
          ⎇ <span class="mono">{git.branch}</span>{#if git.dirty} · {git.changes.length} modified{/if}
        </div>
      {/if}
      <div class="statusbar">
        <span class="dot" class:live={workspace.connection === 'live'} class:down={workspace.connection === 'down'}></span>
        <span class="conn">{workspace.connection}</span>
        <button class="theme" onclick={() => ui.toggleTheme()} aria-label="Toggle theme">{ui.theme === 'dark' ? '🌙' : '☀️'}</button>
      </div>
    </div>
  </aside>

  <main class="content">
    {@render children()}
  </main>
</div>

<NewIssueModal />
<CommandPalette />
<Toasts />

<style>
  .app { display: grid; grid-template-columns: 210px 1fr; height: 100vh; }
  .sidebar { display: flex; flex-direction: column; gap: 4px; padding: 12px; border-right: 1px solid var(--color-border); background: var(--color-surface); }
  .brand { display: flex; align-items: center; gap: 6px; font-weight: 600; font-size: 14px; padding: 4px; }
  .brand span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .new { margin: 8px 0; padding: 7px 10px; border-radius: 6px; border: none; background: var(--color-accent); color: #062018; font-weight: 600; display: flex; align-items: center; gap: 8px; }
  .new kbd { margin-left: auto; background: rgb(0 0 0 / 0.15); color: #062018; border: none; }
  nav { display: flex; flex-direction: column; gap: 2px; }
  nav a { display: flex; align-items: center; padding: 6px 8px; border-radius: 6px; text-decoration: none; color: var(--color-dim); font-size: 13px; }
  nav a kbd { margin-left: auto; }
  nav a:hover { background: var(--color-surface-2); color: var(--color-text); }
  nav a.active { background: var(--color-accent-soft); color: var(--color-accent); }
  .footer { margin-top: auto; font-size: 11px; color: var(--color-dim); display: flex; flex-direction: column; gap: 8px; }
  .git { padding: 4px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .statusbar { display: flex; align-items: center; gap: 6px; padding: 4px; }
  .dot { width: 7px; height: 7px; border-radius: 50%; background: var(--color-warn); }
  .dot.live { background: var(--color-accent); }
  .dot.down { background: var(--color-danger); }
  .conn { flex: 1; }
  .theme { background: none; border: none; }
  .content { overflow: auto; }
</style>
