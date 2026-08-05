<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { api, type GraphData, type GraphNode } from '$lib/api';
  import GraphView from '$lib/components/GraphView.svelte';

  let data = $state<GraphData | null>(null);
  let err = $state('');
  let loading = $state(true);

  let showTicket = $state(true);
  let showTopic = $state(true);
  let showLearning = $state(true);
  let showMemory = $state(true);

  async function load() {
    loading = true;
    err = '';
    try {
      data = await api.graph();
    } catch (e) {
      err = e instanceof Error ? e.message : 'failed to load graph';
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    load();
  });

  function navigate(n: GraphNode) {
    if (n.kind === 'ticket') {
      goto(`/tickets/${n.id}`);
      return;
    }
    // Topics / memory / learnings stay on the graph page for now.
  }

  const counts = $derived({
    ticket: (data?.nodes ?? []).filter((n) => n.kind === 'ticket').length,
    topic: (data?.nodes ?? []).filter((n) => n.kind === 'topic').length,
    learning: (data?.nodes ?? []).filter((n) => n.kind === 'learning').length,
    memory: (data?.nodes ?? []).filter((n) => n.kind === 'memory').length,
    edges: data?.edges?.length ?? 0
  });
</script>

<div class="page">
  <header>
    <div>
      <h1>Graph</h1>
      <p class="sub">Typed links across tickets, memory topics, and learnings</p>
    </div>
    <button class="reload" onclick={load} disabled={loading}>Reload</button>
  </header>

  <div class="filters" role="group" aria-label="Filter node types">
    <label><input type="checkbox" bind:checked={showTicket} /> Tickets ({counts.ticket})</label>
    <label><input type="checkbox" bind:checked={showTopic} /> Topics ({counts.topic})</label>
    <label><input type="checkbox" bind:checked={showLearning} /> Learnings ({counts.learning})</label>
    <label><input type="checkbox" bind:checked={showMemory} /> MEMORY ({counts.memory})</label>
    <span class="meta">{counts.edges} edges</span>
  </div>

  {#if err}
    <p class="err">{err}</p>
  {:else if loading && !data}
    <p class="sub">Loading graph…</p>
  {:else if data}
    <GraphView
      {data}
      {showTicket}
      {showTopic}
      {showLearning}
      {showMemory}
      onnavigate={navigate}
    />
    {#if data.dangling?.length}
      <details class="dangling">
        <summary>{data.dangling.length} dangling link(s)</summary>
        <ul>
          {#each data.dangling as d}<li><code>{d}</code></li>{/each}
        </ul>
      </details>
    {/if}
  {/if}
</div>

<style>
  .page { padding: 20px 24px; display: flex; flex-direction: column; gap: 14px; }
  header { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; }
  h1 { margin: 0; font-size: 20px; }
  .sub { margin: 4px 0 0; color: var(--color-dim); font-size: 13px; }
  .reload { padding: 6px 12px; border-radius: 6px; border: 1px solid var(--color-border); background: var(--color-surface); color: var(--color-text); }
  .filters { display: flex; flex-wrap: wrap; gap: 14px; align-items: center; font-size: 13px; }
  .filters label { display: flex; align-items: center; gap: 6px; color: var(--color-dim); }
  .meta { margin-left: auto; color: var(--color-dim); font-family: var(--font-mono); font-size: 12px; }
  .err { color: var(--color-danger); }
  .dangling { font-size: 13px; color: var(--color-dim); }
  .dangling ul { margin: 6px 0 0; padding-left: 18px; }
  code { font-family: var(--font-mono); font-size: 12px; }
</style>
