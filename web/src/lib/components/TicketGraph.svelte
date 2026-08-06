<script lang="ts">
  import type { Ticket } from '$lib/api';
  import { workspace } from '$lib/workspace.svelte';
  import { neighborhood } from '$lib/graph';

  let {
    ticket,
    onSelectTicket,
    onOverflow
  }: {
    ticket: Ticket;
    onSelectTicket?: (id: string) => void;
    onOverflow?: (section: 'blocked' | 'blocks' | 'children' | 'related') => void;
  } = $props();

  const n = $derived(neighborhood(ticket, workspace.tickets));

  const NW = 128, NH = 40, PITCH = 52, COLGAP = 168;
  const CHILD_W = 92, CHILD_GAP = 8, CHILD_PER_ROW = 4, CHILD_ROW = 48;
  const PARENT_Y = 16;

  const left = $derived([
    ...n.blockers.map((b) => ({ ...b, kind: 'blocker' as const })),
    ...n.dangling.map((id) => ({ id, title: id, status: '', priority: '', unmet: true, inCycle: false, kind: 'dangling' as const }))
  ]);
  const right = $derived(n.dependents);
  const leftCount = $derived(left.length + (n.truncated.blockers > 0 ? 1 : 0));
  const rightCount = $derived(right.length + (n.truncated.dependents > 0 ? 1 : 0));
  const isEmpty = $derived(!n.parent && !left.length && !right.length && !n.children.length && !n.dangling.length && !n.memory.length);

  const childRows = $derived(
    n.children.length || n.truncated.children > 0
      ? Math.ceil((n.children.length + (n.truncated.children > 0 ? 1 : 0)) / CHILD_PER_ROW)
      : 0
  );
  const childSlots = $derived(n.children.length + (n.truncated.children > 0 ? 1 : 0));
  const midRows = $derived(Math.max(leftCount, rightCount, 1));
  const topBand = $derived(n.parent ? 72 : 12);
  const memBand = $derived(n.memory.length || n.truncated.memory > 0 ? 56 : 0);
  const bottomBand = $derived((childRows ? childRows * CHILD_ROW + 14 : 6) + memBand);
  const midH = $derived(midRows * PITCH);
  const height = $derived(topBand + midH + bottomBand);
  const width = 2 * COLGAP + NW + 60;
  const cx = COLGAP + 30;
  const centerY = $derived(topBand + midH / 2 - NH / 2);

  function hrefFor(ref: { id: string; kind?: string }): string | null {
    if (ref.kind === 'ticket' || (!ref.kind && !ref.id.includes('/'))) return `/tickets/${ref.id}`;
    if (ref.kind === 'topic' || ref.id.startsWith('memory/')) return '/graph';
    if (ref.kind === 'memory' || ref.id === 'MEMORY') return '/graph';
    return '/graph';
  }

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

  /** Place child / +more cells in wrapped rows centered under the ticket. */
  function childCell(index: number, total: number): { x: number; y: number } {
    const row = Math.floor(index / CHILD_PER_ROW);
    const col = index % CHILD_PER_ROW;
    const rowLen = Math.min(CHILD_PER_ROW, total - row * CHILD_PER_ROW);
    const rowWidth = rowLen * CHILD_W + (rowLen - 1) * CHILD_GAP;
    const startX = cx + NW / 2 - rowWidth / 2;
    return {
      x: startX + col * (CHILD_W + CHILD_GAP),
      y: topBand + midH + 16 + row * CHILD_ROW
    };
  }

  function onTicketClick(e: MouseEvent, id: string) {
    if (!onSelectTicket) return;
    if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey || e.button !== 0) return;
    e.preventDefault();
    onSelectTicket(id);
  }

  function overflow(section: 'blocked' | 'blocks' | 'children' | 'related') {
    onOverflow?.(section);
  }
</script>

{#if isEmpty}
  <p class="empty">No dependencies, epic links, or memory links.</p>
{:else}
  <svg class="graph" viewBox={`0 0 ${width} ${height}`} role="img" aria-label={`Relationships for ${ticket.id}`}>
    <defs>
      <marker id="tg-dep" markerWidth="8" markerHeight="8" refX="6.5" refY="4" orient="auto"><path d="M0,0 L8,4 L0,8 Z" fill="var(--color-accent)"/></marker>
      <marker id="tg-warn" markerWidth="8" markerHeight="8" refX="6.5" refY="4" orient="auto"><path d="M0,0 L8,4 L0,8 Z" fill="var(--color-warn)"/></marker>
      <marker id="tg-danger" markerWidth="8" markerHeight="8" refX="6.5" refY="4" orient="auto"><path d="M0,0 L8,4 L0,8 Z" fill="var(--color-danger)"/></marker>
    </defs>

    {#if n.parent}
      <path class="edge dashed" d={`M${cx + NW / 2},${topBand} L${cx + NW / 2},${centerY}`} />
      <a href={`/tickets/${n.parent.id}`} onclick={(e) => onTicketClick(e, n.parent!.id)}>
        <rect class="node epic" x={cx} y={PARENT_Y} width={NW} height={NH - 6} rx="9" />
        <text class="nid" x={cx + 12} y={PARENT_Y + 17}>{n.parent.id}</text>
        <text class="nsub" x={cx + 12} y={PARENT_Y + 29}>epic</text>
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
        <a href={`/tickets/${ref.id}`} onclick={(e) => onTicketClick(e, ref.id)}>
          <rect class="node" x="0" y={y} width={NW - 40} height={NH} rx="9" />
          <circle cx="14" cy={y + 14} r="4" fill={dotColor(ref)} />
          <text class="nid" x="26" y={y + 18}>{ref.id}</text>
          <text class="nsub" x="12" y={y + 30}>{ref.unmet ? 'blocks this' : ref.status}</text>
        </a>
      {/if}
    {/each}
    {#if n.truncated.blockers > 0}
      {@const y = colY(left.length, leftCount)}
      <a class="overflow-link" href="#rel-blocked" onclick={(e) => { e.preventDefault(); overflow('blocked'); }}>
        <text class="overflow" x="12" y={y + 18}>+{n.truncated.blockers} more</text>
      </a>
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
      <a href={`/tickets/${ref.id}`} onclick={(e) => onTicketClick(e, ref.id)}>
        <rect class="node" x={cx + COLGAP} y={y} width={NW - 40} height={NH} rx="9" />
        <circle cx={cx + COLGAP + 14} cy={y + 14} r="4" fill={dotColor(ref)} />
        <text class="nid" x={cx + COLGAP + 26} y={y + 18}>{ref.id}</text>
        <text class="nsub" x={cx + COLGAP + 12} y={y + 30}>waits on this</text>
      </a>
    {/each}
    {#if n.truncated.dependents > 0}
      {@const y = colY(right.length, rightCount)}
      <a class="overflow-link" href="#rel-blocks" onclick={(e) => { e.preventDefault(); overflow('blocks'); }}>
        <text class="overflow" x={cx + COLGAP + 12} y={y + 18}>+{n.truncated.dependents} more</text>
      </a>
    {/if}

    {#each n.children as ref, i (ref.id)}
      {@const pos = childCell(i, childSlots)}
      <path class="edge dashed" d={`M${cx + NW / 2},${centerY + NH} L${pos.x + CHILD_W / 2},${pos.y}`} />
      <a href={`/tickets/${ref.id}`} onclick={(e) => onTicketClick(e, ref.id)}>
        <rect class="node" x={pos.x} y={pos.y} width={CHILD_W} height={NH - 8} rx="8" />
        <text class="nid" x={pos.x + 10} y={pos.y + 15}>{ref.id}</text>
        <text class="nsub" x={pos.x + 10} y={pos.y + 27}>{ref.status}</text>
      </a>
    {/each}
    {#if n.truncated.children > 0}
      {@const pos = childCell(n.children.length, childSlots)}
      <a class="overflow-link" href="#rel-children" onclick={(e) => { e.preventDefault(); overflow('children'); }}>
        <text class="overflow" x={pos.x} y={pos.y + 15}>+{n.truncated.children} more</text>
      </a>
    {/if}

    {#each n.memory as ref, i (ref.id)}
      {@const memW = 100}
      {@const startX = cx + NW / 2 - (n.memory.length * (memW + 8) - 8) / 2}
      {@const x = startX + i * (memW + 8)}
      {@const y = topBand + midH + (childRows ? childRows * CHILD_ROW + 14 : 12)}
      <path class="edge dashed mem" d={`M${cx + NW / 2},${centerY + NH} L${x + memW / 2},${y}`} />
      {@const href = hrefFor(ref)}
      {#if href}
        {#if ref.kind === 'ticket'}
          <a href={href} onclick={(e) => onTicketClick(e, ref.id)}>
            <rect class="node mem" x={x} y={y} width={memW} height={NH - 8} rx="8" />
            <text class="nid" x={x + 8} y={y + 15}>{ref.title}</text>
            <text class="nsub" x={x + 8} y={y + 27}>{ref.kind ?? 'link'}</text>
          </a>
        {:else}
          <a href={href}>
            <rect class="node mem" x={x} y={y} width={memW} height={NH - 8} rx="8" />
            <text class="nid" x={x + 8} y={y + 15}>{ref.title}</text>
            <text class="nsub" x={x + 8} y={y + 27}>{ref.kind ?? 'link'}</text>
          </a>
        {/if}
      {/if}
    {/each}
    {#if n.truncated.memory > 0}
      {@const memW = 100}
      {@const startX = cx + NW / 2 - (n.memory.length * (memW + 8) - 8) / 2}
      {@const x = startX + n.memory.length * (memW + 8)}
      {@const y = topBand + midH + (childRows ? childRows * CHILD_ROW + 14 : 12)}
      <!-- Memory/topic overflow is not a Related list target (Related = ticket links only). -->
      <text class="overflow" x={x} y={y + 15}>+{n.truncated.memory} more</text>
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
    {#each n.memory as m}<li>Linked {m.kind ?? 'ref'} {m.id}</li>{/each}
    {#each n.dangling as id}<li>Missing dependency {id}</li>{/each}
  </ul>
{/if}

<style>
  .graph { width: 100%; height: auto; max-width: 560px; display: block; margin: 12px 0 12px; }
  .empty { color: var(--color-dim); font-size: 13px; margin: 8px 0 14px; }
  .node { fill: var(--color-surface); stroke: var(--color-border); stroke-width: 1.5; }
  .node.center { stroke: var(--color-accent); stroke-width: 2.5; }
  .node.epic { stroke-dasharray: 4 3; }
  .node.missing { fill: none; stroke: var(--color-border); stroke-dasharray: 3 3; }
  .node.mem { stroke: var(--color-accent); stroke-dasharray: 2 2; }
  .nid { font-family: var(--font-mono); font-size: 11px; fill: var(--color-text); }
  .nid.dim { fill: var(--color-dim); }
  .nsub { font-family: var(--font-mono); font-size: 8.5px; fill: var(--color-dim); }
  .overflow { font-family: var(--font-mono); font-size: 10px; fill: var(--color-dim); }
  .overflow-link { cursor: pointer; }
  .overflow-link:hover .overflow { fill: var(--color-accent); }
  .edge { stroke: var(--color-accent); stroke-width: 1.5; fill: none; }
  .edge.warn { stroke: var(--color-warn); }
  .edge.cycle { stroke: var(--color-danger); }
  .edge.dashed { stroke: var(--color-dim); stroke-dasharray: 5 4; }
  .edge.dim { stroke: var(--color-border); }
  .edge.mem { stroke: var(--color-accent); opacity: 0.7; }
  a { cursor: pointer; }
  a:focus-visible rect { outline: 2px solid var(--color-accent); outline-offset: 1px; }
  .sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
  @media (prefers-reduced-motion: reduce) { .graph { transition: none; } }
</style>
