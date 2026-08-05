<script lang="ts">
  import type { GraphData, GraphEdge, GraphNode, GraphNodeKind } from '$lib/api';

  let {
    data,
    showTicket = true,
    showTopic = true,
    showLearning = true,
    showMemory = true,
    onnavigate
  }: {
    data: GraphData;
    showTicket?: boolean;
    showTopic?: boolean;
    showLearning?: boolean;
    showMemory?: boolean;
    onnavigate?: (node: GraphNode) => void;
  } = $props();

  const NODE_W = 120;
  const NODE_H = 36;

  type Pos = { x: number; y: number };

  const visible = $derived(
    (data.nodes ?? []).filter((n) => {
      if (n.kind === 'ticket') return showTicket;
      if (n.kind === 'topic') return showTopic;
      if (n.kind === 'learning') return showLearning;
      if (n.kind === 'memory') return showMemory;
      return true;
    })
  );

  const visibleIds = $derived(new Set(visible.map((n) => n.id)));

  const edges = $derived(
    (data.edges ?? []).filter((e) => visibleIds.has(e.source) && visibleIds.has(e.target))
  );

  const positions = $derived(layoutRings(visible));

  let vbX = $state(0);
  let vbY = $state(0);
  let vbW = $state(900);
  let vbH = $state(640);
  let dragging = $state(false);
  let lastX = 0;
  let lastY = 0;

  $effect(() => {
    // Fit viewBox loosely around laid-out nodes when data changes.
    const vals = Object.values(positions);
    if (!vals.length) {
      vbX = 0;
      vbY = 0;
      vbW = 900;
      vbH = 640;
      return;
    }
    let minX = Infinity,
      minY = Infinity,
      maxX = -Infinity,
      maxY = -Infinity;
    for (const p of vals) {
      minX = Math.min(minX, p.x);
      minY = Math.min(minY, p.y);
      maxX = Math.max(maxX, p.x + NODE_W);
      maxY = Math.max(maxY, p.y + NODE_H);
    }
    const pad = 80;
    vbX = minX - pad;
    vbY = minY - pad;
    vbW = Math.max(400, maxX - minX + pad * 2);
    vbH = Math.max(300, maxY - minY + pad * 2);
  });

  function layoutRings(nodes: GraphNode[]): Record<string, Pos> {
    const byKind: Record<string, GraphNode[]> = {
      ticket: [],
      learning: [],
      topic: [],
      memory: [],
      unknown: []
    };
    for (const n of nodes) {
      (byKind[n.kind] ?? byKind.unknown).push(n);
    }
    for (const k of Object.keys(byKind)) {
      byKind[k].sort((a, b) => a.id.localeCompare(b.id));
    }
    const cx = 450;
    const cy = 320;
    const radii: Record<string, number> = {
      ticket: 0,
      learning: 160,
      topic: 280,
      memory: 380,
      unknown: 420
    };
    const out: Record<string, Pos> = {};
    // Tickets in a compact grid at center when many.
    const tickets = byKind.ticket;
    if (tickets.length <= 1) {
      if (tickets[0]) out[tickets[0].id] = { x: cx - NODE_W / 2, y: cy - NODE_H / 2 };
    } else {
      const cols = Math.ceil(Math.sqrt(tickets.length));
      const gap = 16;
      const totalW = cols * (NODE_W + gap) - gap;
      const rows = Math.ceil(tickets.length / cols);
      const totalH = rows * (NODE_H + gap) - gap;
      tickets.forEach((n, i) => {
        const col = i % cols;
        const row = Math.floor(i / cols);
        out[n.id] = {
          x: cx - totalW / 2 + col * (NODE_W + gap),
          y: cy - totalH / 2 + row * (NODE_H + gap)
        };
      });
    }
    for (const kind of ['learning', 'topic', 'memory', 'unknown'] as GraphNodeKind[]) {
      const list = byKind[kind] ?? [];
      const r = radii[kind] ?? 400;
      list.forEach((n, i) => {
        const angle = (2 * Math.PI * i) / Math.max(list.length, 1) - Math.PI / 2;
        out[n.id] = {
          x: cx + Math.cos(angle) * r - NODE_W / 2,
          y: cy + Math.sin(angle) * r - NODE_H / 2
        };
      });
    }
    return out;
  }

  function edgeColor(e: GraphEdge): string {
    if (e.kind === 'dep') return 'var(--color-accent)';
    if (e.kind === 'parent') return 'var(--color-dim)';
    return 'var(--color-warn)';
  }

  function nodeStroke(kind: GraphNodeKind): string {
    if (kind === 'ticket') return 'var(--color-accent)';
    if (kind === 'topic') return 'var(--color-warn)';
    if (kind === 'learning') return 'var(--color-dim)';
    return 'var(--color-border)';
  }

  function onWheel(e: WheelEvent) {
    e.preventDefault();
    const factor = e.deltaY > 0 ? 1.1 : 0.9;
    const mx = vbX + (e.offsetX / (e.currentTarget as SVGSVGElement).clientWidth) * vbW;
    const my = vbY + (e.offsetY / (e.currentTarget as SVGSVGElement).clientHeight) * vbH;
    const nw = vbW * factor;
    const nh = vbH * factor;
    vbX = mx - ((mx - vbX) / vbW) * nw;
    vbY = my - ((my - vbY) / vbH) * nh;
    vbW = nw;
    vbH = nh;
  }

  function onPointerDown(e: PointerEvent) {
    if ((e.target as Element).closest('a,button,.node-hit')) return;
    dragging = true;
    lastX = e.clientX;
    lastY = e.clientY;
    (e.currentTarget as Element).setPointerCapture?.(e.pointerId);
  }
  function onPointerMove(e: PointerEvent) {
    if (!dragging) return;
    const svg = e.currentTarget as SVGSVGElement;
    const dx = ((e.clientX - lastX) / svg.clientWidth) * vbW;
    const dy = ((e.clientY - lastY) / svg.clientHeight) * vbH;
    vbX -= dx;
    vbY -= dy;
    lastX = e.clientX;
    lastY = e.clientY;
  }
  function onPointerUp() {
    dragging = false;
  }

  function clickNode(n: GraphNode) {
    onnavigate?.(n);
  }
</script>

<svg
  class="graph"
  viewBox={`${vbX} ${vbY} ${vbW} ${vbH}`}
  role="img"
  aria-label="Knowledge graph"
  onwheel={onWheel}
  onpointerdown={onPointerDown}
  onpointermove={onPointerMove}
  onpointerup={onPointerUp}
  onpointercancel={onPointerUp}
>
  <defs>
    <marker id="gv-arrow" markerWidth="8" markerHeight="8" refX="6.5" refY="4" orient="auto"
      ><path d="M0,0 L8,4 L0,8 Z" fill="var(--color-accent)" /></marker
    >
  </defs>

  {#each edges as e (e.kind + e.source + e.target)}
    {@const a = positions[e.source]}
    {@const b = positions[e.target]}
    {#if a && b}
      <line
        class="edge"
        class:dashed={e.kind === 'parent'}
        class:link={e.kind === 'link'}
        x1={a.x + NODE_W / 2}
        y1={a.y + NODE_H / 2}
        x2={b.x + NODE_W / 2}
        y2={b.y + NODE_H / 2}
        stroke={edgeColor(e)}
        marker-end="url(#gv-arrow)"
      />
    {/if}
  {/each}

  {#each visible as n (n.id)}
    {@const p = positions[n.id]}
    {#if p}
      <g class="node-hit" role="button" tabindex="0" onclick={() => clickNode(n)} onkeydown={(ev) => ev.key === 'Enter' && clickNode(n)}>
        <rect
          class="node"
          x={p.x}
          y={p.y}
          width={NODE_W}
          height={NODE_H}
          rx="9"
          stroke={nodeStroke(n.kind)}
        />
        <text class="nid" x={p.x + 10} y={p.y + 15}>{n.id.length > 16 ? n.id.slice(0, 14) + '…' : n.id}</text>
        <text class="nsub" x={p.x + 10} y={p.y + 28}>{n.kind}{n.status ? ` · ${n.status}` : ''}</text>
      </g>
    {/if}
  {/each}
</svg>

{#if !visible.length}
  <p class="empty">No nodes match the current filters.</p>
{/if}

<style>
  .graph {
    width: 100%;
    height: min(70vh, 640px);
    display: block;
    background: var(--color-surface);
    border: 1px solid var(--color-border);
    border-radius: 10px;
    cursor: grab;
    touch-action: none;
  }
  .graph:active {
    cursor: grabbing;
  }
  .node {
    fill: var(--color-surface-2, var(--color-surface));
    stroke-width: 1.8;
  }
  .nid {
    font-family: var(--font-mono);
    font-size: 11px;
    fill: var(--color-text);
  }
  .nsub {
    font-family: var(--font-mono);
    font-size: 9px;
    fill: var(--color-dim);
  }
  .edge {
    stroke-width: 1.4;
    fill: none;
    opacity: 0.85;
  }
  .edge.dashed {
    stroke-dasharray: 5 4;
  }
  .edge.link {
    stroke-dasharray: 2 3;
  }
  .node-hit {
    cursor: pointer;
  }
  .node-hit:focus-visible .node {
    outline: 2px solid var(--color-accent);
  }
  .empty {
    color: var(--color-dim);
    font-size: 13px;
    margin-top: 12px;
  }
</style>
