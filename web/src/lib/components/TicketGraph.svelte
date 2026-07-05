<script lang="ts">
  import type { Ticket } from '$lib/api';
  import { workspace } from '$lib/workspace.svelte';
  import { neighborhood } from '$lib/graph';

  let { ticket }: { ticket: Ticket } = $props();
  const n = $derived(neighborhood(ticket, workspace.tickets));

  const NW = 128, NH = 40, PITCH = 52, COLGAP = 168;

  const left = $derived([
    ...n.blockers.map((b) => ({ ...b, kind: 'blocker' as const })),
    ...n.dangling.map((id) => ({ id, title: id, status: '', priority: '', unmet: true, inCycle: false, kind: 'dangling' as const }))
  ]);
  const right = $derived(n.dependents);
  const leftCount = $derived(left.length + (n.truncated.blockers > 0 ? 1 : 0));
  const rightCount = $derived(right.length + (n.truncated.dependents > 0 ? 1 : 0));
  const isEmpty = $derived(!n.parent && !left.length && !right.length && !n.children.length && !n.dangling.length);

  const midRows = $derived(Math.max(leftCount, rightCount, 1));
  const topBand = $derived(n.parent ? 58 : 6);
  const bottomBand = $derived(n.children.length || n.truncated.children > 0 ? 62 : 6);
  const midH = $derived(midRows * PITCH);
  const height = $derived(topBand + midH + bottomBand);
  const width = 2 * COLGAP + NW + 60;
  const cx = COLGAP + 30;
  const centerY = $derived(topBand + midH / 2 - NH / 2);

  function colY(i: number, count: number): number {
    return topBand + (midH - count * PITCH) / 2 + i * PITCH;
  }
  function dotColor(ref: { status: string; inCycle?: boolean; unmet?: boolean }): string {
    if (ref.inCycle) return 'var(--color-danger)';
    if (ref.unmet) return 'var(--color-warn)';
    return 'var(--color-accent)';
  }
  function depMarker(ref: { inCycle?: boolean; unmet?: boolean; kind?: string }): string {
    if (ref.inCycle) return 'url(#tg-danger)';
    if (ref.unmet && ref.kind === 'blocker') return 'url(#tg-warn)';
    return 'url(#tg-dep)';
  }
</script>

{#if isEmpty}
  <p class="empty">No dependencies or epic links.</p>
{:else}
  <svg class="graph" viewBox={`0 0 ${width} ${height}`} role="img" aria-label={`Relationships for ${ticket.id}`}>
    <defs>
      <marker id="tg-dep" markerWidth="8" markerHeight="8" refX="6.5" refY="4" orient="auto"><path d="M0,0 L8,4 L0,8 Z" fill="var(--color-accent)"/></marker>
      <marker id="tg-warn" markerWidth="8" markerHeight="8" refX="6.5" refY="4" orient="auto"><path d="M0,0 L8,4 L0,8 Z" fill="var(--color-warn)"/></marker>
      <marker id="tg-danger" markerWidth="8" markerHeight="8" refX="6.5" refY="4" orient="auto"><path d="M0,0 L8,4 L0,8 Z" fill="var(--color-danger)"/></marker>
    </defs>

    {#if n.parent}
      <path class="edge dashed" d={`M${cx + NW / 2},${topBand} L${cx + NW / 2},${centerY}`} />
      <a href={`/tickets/${n.parent.id}`}>
        <rect class="node epic" x={cx} y="8" width={NW} height={NH - 6} rx="9" />
        <text class="nid" x={cx + 12} y="25">{n.parent.id}</text>
        <text class="nsub" x={cx + 12} y="37">epic</text>
      </a>
    {/if}

    {#each left as ref, i (ref.id)}
      {@const y = colY(i, leftCount)}
      <path class="edge" class:warn={ref.unmet && ref.kind === 'blocker' && !ref.inCycle && !ticket.inCycle}
            class:cycle={ref.inCycle || (ticket.inCycle && ref.kind === 'blocker')} class:dim={ref.kind === 'dangling'}
            d={`M${cx - 40},${y + NH / 2} L${cx},${centerY + NH / 2}`}
            marker-end={ref.inCycle || (ticket.inCycle && ref.kind === 'blocker') ? 'url(#tg-danger)' : depMarker(ref)} />
      {#if ref.kind === 'dangling'}
        <rect class="node missing" x="0" y={y} width={NW - 40} height={NH} rx="9" />
        <text class="nid dim" x="12" y={y + 18}>{ref.id}</text>
        <text class="nsub" x="12" y={y + 30}>missing</text>
      {:else}
        <a href={`/tickets/${ref.id}`}>
          <rect class="node" x="0" y={y} width={NW - 40} height={NH} rx="9" />
          <circle cx="14" cy={y + 14} r="4" fill={dotColor(ref)} />
          <text class="nid" x="26" y={y + 18}>{ref.id}</text>
          <text class="nsub" x="12" y={y + 30}>{ref.unmet ? 'blocks this' : ref.status}</text>
        </a>
      {/if}
    {/each}
    {#if n.truncated.blockers > 0}
      {@const y = colY(left.length, leftCount)}
      <text class="overflow" x="12" y={y + 18}>+{n.truncated.blockers} more</text>
    {/if}

    <g>
      <rect class="node center" x={cx} y={centerY} width={NW} height={NH} rx="10" />
      <circle cx={cx + 14} cy={centerY + 14} r="4.5" fill={dotColor({ status: ticket.status, inCycle: ticket.inCycle, unmet: ticket.blocked })} />
      <text class="nid" x={cx + 26} y={centerY + 17} font-weight="600">{ticket.id}</text>
      <text class="nsub" x={cx + 12} y={centerY + 30}>{ticket.blocked ? '🔒 blocked' : ticket.status}</text>
    </g>

    {#each right as ref, i (ref.id)}
      {@const y = colY(i, rightCount)}
      <path class="edge" class:cycle={ref.inCycle || ticket.inCycle} d={`M${cx + NW},${centerY + NH / 2} L${cx + COLGAP},${y + NH / 2}`} marker-end={depMarker(ref)} />
      <a href={`/tickets/${ref.id}`}>
        <rect class="node" x={cx + COLGAP} y={y} width={NW - 40} height={NH} rx="9" />
        <circle cx={cx + COLGAP + 14} cy={y + 14} r="4" fill={dotColor(ref)} />
        <text class="nid" x={cx + COLGAP + 26} y={y + 18}>{ref.id}</text>
        <text class="nsub" x={cx + COLGAP + 12} y={y + 30}>waits on this</text>
      </a>
    {/each}
    {#if n.truncated.dependents > 0}
      {@const y = colY(right.length, rightCount)}
      <text class="overflow" x={cx + COLGAP + 12} y={y + 18}>+{n.truncated.dependents} more</text>
    {/if}

    {#each n.children as ref, i (ref.id)}
      {@const childW = 92}
      {@const startX = cx + NW / 2 - (n.children.length * (childW + 8) - 8) / 2}
      {@const x = startX + i * (childW + 8)}
      {@const y = topBand + midH + 16}
      <path class="edge dashed" d={`M${cx + NW / 2},${centerY + NH} L${x + childW / 2},${y}`} />
      <a href={`/tickets/${ref.id}`}>
        <rect class="node" x={x} y={y} width={childW} height={NH - 8} rx="8" />
        <text class="nid" x={x + 10} y={y + 15}>{ref.id}</text>
        <text class="nsub" x={x + 10} y={y + 27}>{ref.status}</text>
      </a>
    {/each}
    {#if n.truncated.children > 0}
      {@const childW = 92}
      {@const startX = cx + NW / 2 - (n.children.length * (childW + 8) - 8) / 2}
      {@const x = startX + n.children.length * (childW + 8)}
      {@const y = topBand + midH + 16}
      <text class="overflow" x={x} y={y + 15}>+{n.truncated.children} more</text>
    {/if}
  </svg>

  <ul class="sr-only">
    {#if n.parent}<li>Epic: <a href={`/tickets/${n.parent.id}`}>{n.parent.id}</a></li>{/if}
    {#each n.blockers as b}<li>Blocked by <a href={`/tickets/${b.id}`}>{b.id}</a> ({b.status})</li>{/each}
    {#if n.truncated.blockers > 0}<li>+{n.truncated.blockers} more blockers not shown</li>{/if}
    {#each n.dependents as d}<li>Blocks <a href={`/tickets/${d.id}`}>{d.id}</a></li>{/each}
    {#if n.truncated.dependents > 0}<li>+{n.truncated.dependents} more dependents not shown</li>{/if}
    {#each n.children as c}<li>Child <a href={`/tickets/${c.id}`}>{c.id}</a></li>{/each}
    {#if n.truncated.children > 0}<li>+{n.truncated.children} more children not shown</li>{/if}
    {#each n.dangling as id}<li>Missing dependency {id}</li>{/each}
  </ul>
{/if}

<style>
  .graph { width: 100%; height: auto; max-width: 560px; display: block; margin: 4px 0 12px; }
  .empty { color: var(--color-dim); font-size: 13px; margin: 8px 0 14px; }
  .node { fill: var(--color-surface); stroke: var(--color-border); stroke-width: 1.5; }
  .node.center { stroke: var(--color-accent); stroke-width: 2.5; }
  .node.epic { stroke-dasharray: 4 3; }
  .node.missing { fill: none; stroke: var(--color-border); stroke-dasharray: 3 3; }
  .nid { font-family: var(--font-mono); font-size: 11px; fill: var(--color-text); }
  .nid.dim { fill: var(--color-dim); }
  .nsub { font-family: var(--font-mono); font-size: 8.5px; fill: var(--color-dim); }
  .overflow { font-family: var(--font-mono); font-size: 10px; fill: var(--color-dim); }
  .edge { stroke: var(--color-accent); stroke-width: 1.5; fill: none; }
  .edge.warn { stroke: var(--color-warn); }
  .edge.cycle { stroke: var(--color-danger); }
  .edge.dashed { stroke: var(--color-dim); stroke-dasharray: 5 4; }
  .edge.dim { stroke: var(--color-border); }
  a { cursor: pointer; }
  a:focus-visible rect { outline: 2px solid var(--color-accent); outline-offset: 1px; }
  .sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
  @media (prefers-reduced-motion: reduce) { .graph { transition: none; } }
</style>
