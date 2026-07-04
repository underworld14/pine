<script lang="ts">
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { api, type SearchHit } from '$lib/api';
  import { renderMarkdown } from '$lib/markdown';
  import DOMPurify from 'dompurify';

  let query = $state($page.url.searchParams.get('q') ?? '');
  let hits = $state<SearchHit[]>([]);
  let active = $state(0);
  let timer: ReturnType<typeof setTimeout> | null = null;
  let inputEl = $state<HTMLInputElement | null>(null);

  $effect(() => { inputEl?.focus(); });

  function run() {
    if (timer) clearTimeout(timer);
    const q = query.trim();
    goto(`/search?q=${encodeURIComponent(q)}`, { replaceState: true, keepFocus: true, noScroll: true });
    if (q.length < 2) { hits = []; return; }
    timer = setTimeout(async () => {
      try {
        const res = await api.search({ q });
        hits = res.hits;
        active = 0;
      } catch { hits = []; }
    }, 150);
  }

  // Kick off an initial search if arriving with ?q=.
  $effect(() => { if (query && hits.length === 0) run(); });

  function frag(h: SearchHit): string {
    const raw = h.fragments?.body?.[0] ?? h.fragments?.title?.[0];
    if (!raw) return '';
    return DOMPurify.sanitize(raw, { ALLOWED_TAGS: ['mark'], ALLOWED_ATTR: [] });
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === 'ArrowDown') { e.preventDefault(); active = Math.min(active + 1, hits.length - 1); }
    else if (e.key === 'ArrowUp') { e.preventDefault(); active = Math.max(active - 1, 0); }
    else if (e.key === 'Enter' && hits[active]) { goto(`/tickets/${hits[active].id}`); }
  }
</script>

<div class="wrap">
  <input bind:this={inputEl} bind:value={query} oninput={run} onkeydown={onKey} placeholder="Search tickets, labels, files…" />
  {#if query.length >= 2 && hits.length === 0}
    <p class="empty">No matches for <em>{query}</em>.</p>
  {/if}
  <ul>
    {#each hits as h, i}
      <li class:active={i === active}>
        <a href={`/tickets/${h.id}`}>
          <span class="mono id">{h.id}</span>
          <span class="title">{h.title}</span>
          <span class="status">{h.status}</span>
        </a>
        {#if frag(h)}<div class="frag">{@html frag(h)}</div>{/if}
      </li>
    {/each}
  </ul>
</div>

<style>
  .wrap { padding: 20px 24px; max-width: 760px; }
  input { width: 100%; padding: 12px 14px; font-size: 16px; background: var(--color-surface); border: 1px solid var(--color-border); border-radius: 8px; outline: none; }
  input:focus { border-color: var(--color-accent); }
  ul { list-style: none; margin: 16px 0 0; padding: 0; display: flex; flex-direction: column; gap: 6px; }
  li { padding: 8px 10px; border-radius: 8px; border: 1px solid transparent; }
  li.active { border-color: var(--color-border); background: var(--color-surface); }
  a { display: flex; align-items: center; gap: 10px; text-decoration: none; color: inherit; }
  .id { color: var(--color-dim); font-size: 11px; }
  .title { flex: 1; }
  .status { font-size: 11px; color: var(--color-dim); }
  .frag { margin-top: 4px; font-size: 12px; color: var(--color-dim); padding-left: 62px; }
  .frag :global(mark) { background: var(--color-accent-soft); color: var(--color-accent); border-radius: 2px; }
  .empty { color: var(--color-dim); margin-top: 20px; }
</style>
