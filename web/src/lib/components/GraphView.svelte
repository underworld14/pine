<script lang="ts">
  import { untrack } from 'svelte';
  import type { GraphData, GraphEdge, GraphNode, GraphNodeKind } from '$lib/api';
  import {
    buildAdjacency,
    createGraphSimulation,
    focusSet,
    type ForceNode,
    type GraphSimulation,
    type Pos
  } from '$lib/graph-force';

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

  const NODE_R = 10;
  const DRAG_THRESHOLD = 4;

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

  const adjacency = $derived(buildAdjacency(edges));

  let positions = $state<Record<string, Pos>>({});
  let hoverId = $state<string | null>(null);
  let sim: GraphSimulation | null = null;

  let vbX = $state(0);
  let vbY = $state(0);
  let vbW = $state(900);
  let vbH = $state(640);

  let panning = $state(false);
  let lastX = 0;
  let lastY = 0;

  let draggingNode: ForceNode | null = null;
  let dragMoved = false;
  let dragStartClient = { x: 0, y: 0 };
  let pendingClick: GraphNode | null = null;

  const focused = $derived(focusSet(hoverId, adjacency));

  function prefersReducedMotion(): boolean {
    if (typeof window === 'undefined' || !window.matchMedia) return false;
    return window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  }

  function fitViewBox(pos: Record<string, Pos>) {
    const vals = Object.values(pos);
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
      maxX = Math.max(maxX, p.x);
      maxY = Math.max(maxY, p.y);
    }
    const pad = 100;
    vbX = minX - pad;
    vbY = minY - pad;
    vbW = Math.max(400, maxX - minX + pad * 2);
    vbH = Math.max(300, maxY - minY + pad * 2);
  }

  // Stable key so filter/data changes rebuild the sim without tracking tick positions.
  const simKey = $derived.by(() => {
    const ids = visible.map((n) => n.id).join('\0');
    const eds = edges.map((e) => `${e.source}\0${e.target}\0${e.kind}`).join('\x01');
    return `${ids}\x02${eds}`;
  });

  $effect(() => {
    // Track topology key only — not array identity from Reload with same graph.
    void simKey;
    const { nodes, eds } = untrack(() => ({ nodes: visible, eds: edges }));

    sim?.stop();
    const next = createGraphSimulation(nodes, eds, {
      reducedMotion: prefersReducedMotion(),
      onTick: (pos) => {
        positions = pos;
      }
    });
    sim = next;
    const initial = next.getPositions();
    positions = initial;
    fitViewBox(initial);

    return () => {
      draggingNode = null;
      pendingClick = null;
      dragMoved = false;
      next.stop();
      if (sim === next) sim = null;
    };
  });

  function edgeColor(e: GraphEdge): string {
    if (e.kind === 'dep') return 'var(--color-accent)';
    if (e.kind === 'parent') return 'var(--color-dim)';
    return 'var(--color-warn)';
  }

  function nodeFill(kind: GraphNodeKind): string {
    if (kind === 'ticket') return 'var(--color-accent)';
    if (kind === 'topic') return 'var(--color-warn)';
    if (kind === 'learning') return 'var(--color-dim)';
    return 'var(--color-border)';
  }

  function isDimmed(id: string): boolean {
    return focused !== null && !focused.has(id);
  }

  function edgeDimmed(e: GraphEdge): boolean {
    if (!focused) return false;
    return !(focused.has(e.source) && focused.has(e.target));
  }

  function clientToSvg(svg: SVGSVGElement, clientX: number, clientY: number): Pos {
    const rect = svg.getBoundingClientRect();
    return {
      x: vbX + ((clientX - rect.left) / rect.width) * vbW,
      y: vbY + ((clientY - rect.top) / rect.height) * vbH
    };
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
    const target = e.target as Element;
    const hit = target.closest('.node-hit') as SVGElement | null;
    if (hit) {
      const id = hit.dataset.id;
      if (!id || !sim) return;
      const n = sim.nodes.find((x) => x.id === id);
      if (!n) return;
      draggingNode = n;
      dragMoved = false;
      dragStartClient = { x: e.clientX, y: e.clientY };
      pendingClick = visible.find((v) => v.id === id) ?? null;
      // Soft-hold at current pos; reheat only once drag actually starts.
      n.fx = n.x;
      n.fy = n.y;
      (e.currentTarget as Element).setPointerCapture?.(e.pointerId);
      e.stopPropagation();
      return;
    }
    panning = true;
    lastX = e.clientX;
    lastY = e.clientY;
    (e.currentTarget as Element).setPointerCapture?.(e.pointerId);
  }

  function onPointerMove(e: PointerEvent) {
    const svg = e.currentTarget as SVGSVGElement;
    if (draggingNode && sim) {
      const dx = e.clientX - dragStartClient.x;
      const dy = e.clientY - dragStartClient.y;
      const moved = Math.hypot(dx, dy) > DRAG_THRESHOLD;
      if (moved && !dragMoved) {
        dragMoved = true;
        sim.reheat();
      }
      if (!moved && !dragMoved) return;
      const p = clientToSvg(svg, e.clientX, e.clientY);
      draggingNode.fx = p.x;
      draggingNode.fy = p.y;
      positions = { ...positions, [draggingNode.id]: { x: p.x, y: p.y } };
      return;
    }
    if (!panning) return;
    const dx = ((e.clientX - lastX) / svg.clientWidth) * vbW;
    const dy = ((e.clientY - lastY) / svg.clientHeight) * vbH;
    vbX -= dx;
    vbY -= dy;
    lastX = e.clientX;
    lastY = e.clientY;
  }

  function onPointerUp() {
    if (draggingNode) {
      if (!dragMoved) {
        // Click-to-navigate: release temporary hold without energizing the sim.
        draggingNode.fx = null;
        draggingNode.fy = null;
        if (pendingClick) onnavigate?.(pendingClick);
      }
      // After a real drag, keep fx/fy so the node stays pinned (Obsidian-like).
      draggingNode = null;
      pendingClick = null;
      dragMoved = false;
      return;
    }
    panning = false;
  }

  /** Shorten edge endpoints so arrows meet the circle rim, not the center. */
  function edgeEnds(a: Pos, b: Pos): { x1: number; y1: number; x2: number; y2: number } {
    const dx = b.x - a.x;
    const dy = b.y - a.y;
    const len = Math.hypot(dx, dy) || 1;
    const ux = dx / len;
    const uy = dy / len;
    const tip = NODE_R + 5;
    return {
      x1: a.x + ux * NODE_R,
      y1: a.y + uy * NODE_R,
      x2: b.x - ux * tip,
      y2: b.y - uy * tip
    };
  }

  function labelFor(n: GraphNode): string {
    return n.id.length > 18 ? n.id.slice(0, 16) + '…' : n.id;
  }
</script>

<svg
  class="graph"
  class:panning
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
      {@const ends = edgeEnds(a, b)}
      <line
        class="edge"
        class:dashed={e.kind === 'parent'}
        class:link={e.kind === 'link'}
        class:dimmed={edgeDimmed(e)}
        class:focused={focused !== null && !edgeDimmed(e)}
        x1={ends.x1}
        y1={ends.y1}
        x2={ends.x2}
        y2={ends.y2}
        stroke={edgeColor(e)}
        marker-end={e.kind === 'dep' ? 'url(#gv-arrow)' : undefined}
      />
    {/if}
  {/each}

  {#each visible as n (n.id)}
    {@const p = positions[n.id]}
    {#if p}
      <g
        class="node-hit"
        class:dimmed={isDimmed(n.id)}
        class:focused={focused !== null && focused.has(n.id)}
        data-id={n.id}
        role="button"
        tabindex="0"
        aria-label={`${n.kind} ${n.id}`}
        onpointerenter={() => (hoverId = n.id)}
        onpointerleave={() => {
          if (hoverId === n.id) hoverId = null;
        }}
        onkeydown={(ev) => ev.key === 'Enter' && onnavigate?.(n)}
      >
        <circle class="node" cx={p.x} cy={p.y} r={NODE_R} fill={nodeFill(n.kind)} />
        <text class="nid" x={p.x} y={p.y + NODE_R + 12} text-anchor="middle">{labelFor(n)}</text>
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
  .graph:active,
  .graph.panning {
    cursor: grabbing;
  }
  .node {
    stroke: var(--color-surface);
    stroke-width: 2;
  }
  .nid {
    font-family: var(--font-mono);
    font-size: 10px;
    fill: var(--color-text);
    pointer-events: none;
    user-select: none;
  }
  .edge {
    stroke-width: 1.4;
    fill: none;
    opacity: 0.75;
    transition: opacity 0.15s ease;
  }
  .edge.dashed {
    stroke-dasharray: 5 4;
  }
  .edge.link {
    stroke-dasharray: 2 3;
  }
  .edge.focused {
    opacity: 1;
    stroke-width: 2;
  }
  .edge.dimmed {
    opacity: 0.12;
  }
  .node-hit {
    cursor: pointer;
    transition: opacity 0.15s ease;
  }
  .node-hit.focused .node {
    stroke: var(--color-text);
    stroke-width: 2.5;
  }
  .node-hit.dimmed {
    opacity: 0.18;
  }
  .node-hit:focus-visible .node {
    outline: 2px solid var(--color-accent);
    outline-offset: 3px;
  }
  .empty {
    color: var(--color-dim);
    font-size: 13px;
    margin-top: 12px;
  }
  @media (prefers-reduced-motion: reduce) {
    .edge,
    .node-hit {
      transition: none;
    }
  }
</style>
