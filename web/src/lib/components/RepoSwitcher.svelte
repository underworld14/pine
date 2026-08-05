<script lang="ts">
  import { workspace } from '$lib/workspace.svelte';

  let busy = $state(false);
  let err = $state('');

  async function onChange(e: Event) {
    const alias = (e.target as HTMLSelectElement).value;
    busy = true;
    err = '';
    try {
      await workspace.switchRepo(alias);
    } catch (ex) {
      err = ex instanceof Error ? ex.message : 'switch failed';
    } finally {
      busy = false;
    }
  }
</script>

{#if workspace.isMultiRepo}
  <div class="switcher" title="Repos registered in ~/.pine">
    <label for="repo-switch">Repo</label>
    <select id="repo-switch" value={workspace.activeRepo} onchange={onChange} disabled={busy}>
      {#each workspace.repos as r (r.alias)}
        <option value={r.alias}>{r.alias} · {r.project}</option>
      {/each}
    </select>
    {#if err}<span class="err">{err}</span>{/if}
  </div>
{/if}

<style>
  .switcher {
    display: flex;
    flex-direction: column;
    gap: 4px;
    padding: 6px 8px;
    margin: 4px 0 8px;
    border-radius: 6px;
    background: var(--color-surface-2, var(--color-surface));
    border: 1px solid var(--color-border);
  }
  label {
    font-size: 10px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--color-dim);
  }
  select {
    font-size: 12px;
    font-family: var(--font-mono);
    background: var(--color-surface);
    color: var(--color-text);
    border: 1px solid var(--color-border);
    border-radius: 4px;
    padding: 4px 6px;
  }
  .err {
    font-size: 11px;
    color: var(--color-danger);
  }
</style>
