# Per-Ticket Dependency & Epic Graph — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the ticket detail page's dep/epic text chips with a small "neighborhood" graph — the current ticket centered, one hop out (blockers ◀, dependents ▶, epic ▲, children ▼).

**Architecture:** A pure `neighborhood()` function computes the one-hop graph from the tickets already held in the `workspace` store (no backend work — `dependents` is a client-side reverse scan; everything else is on `view.Ticket`). A `TicketGraph.svelte` component renders it as a deterministic radial SVG with no layout library. Solid edges = dependencies, dashed = epic hierarchy, red = cycle.

**Tech Stack:** SvelteKit 2 / Svelte 5 (runes) / TypeScript / Vitest. Frontend only.

## Global Constraints

- No backend changes. All data comes from `view.Ticket` fields already served: `deps`, `unmet`, `dangling`, `inCycle`, `parent`, `children`, `status`, `priority`.
- No graph/layout library; positions are computed arithmetically. Exactly one hop from the center.
- Every non-center node is a real `<a href="/tickets/{id}">`; a visually-hidden `<ul>` mirrors the nodes for screen readers.
- Reuse existing theme tokens: `--color-surface`, `--color-border`, `--color-accent`, `--color-warn`, `--color-danger`, `--color-text`, `--color-dim`, `--font-mono`.
- Per-arm cap = 6, with a `+N` overflow surfaced in `truncated` and the a11y list.
- Run web tests/build from `web/`.

---

### Task 1: Pure neighborhood computation

**Files:**
- Create: `web/src/lib/graph.ts`
- Test: `web/src/lib/graph.test.ts`

**Interfaces:**
- Consumes: `Ticket` from `$lib/api` (fields `deps`, `unmet`, `dangling`, `inCycle`, `parent`, `children`).
- Produces:
  ```ts
  interface NeighborRef { id: string; title: string; status: string; priority: string; unmet?: boolean; inCycle?: boolean }
  interface Neighborhood {
    parent?: NeighborRef; blockers: NeighborRef[]; dependents: NeighborRef[];
    children: NeighborRef[]; dangling: string[];
    truncated: { blockers: number; dependents: number; children: number };
  }
  function neighborhood(ticket: Ticket, all: Record<string, Ticket>, cap?: number): Neighborhood
  ```

- [ ] **Step 1: Write the failing test**

```ts
// web/src/lib/graph.test.ts
import { describe, it, expect } from 'vitest';
import { neighborhood } from './graph';
import type { Ticket } from './api';

function tk(p: Partial<Ticket>): Ticket {
  return {
    id: '', type: 'BUG', title: '', status: 'todo', priority: 'medium',
    labels: [], deps: [], created: '', updated: '', blocked: false, hash: '', attachments: [], ...p
  } as Ticket;
}

describe('neighborhood', () => {
  it('resolves blockers (with unmet), dependents, parent, dangling', () => {
    const all: Record<string, Ticket> = {
      'BUG-1': tk({ id: 'BUG-1', deps: ['FEAT-2', 'GONE-9'], unmet: ['FEAT-2'], dangling: ['GONE-9'], parent: 'EPIC-3' }),
      'FEAT-2': tk({ id: 'FEAT-2', status: 'doing' }),
      'BUG-4': tk({ id: 'BUG-4', deps: ['BUG-1'] }),
      'EPIC-3': tk({ id: 'EPIC-3', type: 'EPIC' })
    };
    const n = neighborhood(all['BUG-1'], all);
    expect(n.blockers.map((b) => b.id)).toEqual(['FEAT-2']);
    expect(n.blockers[0].unmet).toBe(true);
    expect(n.dependents.map((d) => d.id)).toEqual(['BUG-4']);
    expect(n.parent?.id).toBe('EPIC-3');
    expect(n.dangling).toEqual(['GONE-9']);
  });

  it('resolves children by reverse scan when children[] is absent', () => {
    const all: Record<string, Ticket> = {
      'EPIC-3': tk({ id: 'EPIC-3', type: 'EPIC' }),
      'BUG-5': tk({ id: 'BUG-5', parent: 'EPIC-3' })
    };
    expect(neighborhood(all['EPIC-3'], all).children.map((c) => c.id)).toEqual(['BUG-5']);
  });

  it('caps each arm and reports truncation', () => {
    const all: Record<string, Ticket> = { 'BUG-1': tk({ id: 'BUG-1' }) };
    for (let i = 0; i < 8; i++) all[`D-${i}`] = tk({ id: `D-${i}`, deps: ['BUG-1'] });
    const n = neighborhood(all['BUG-1'], all, 6);
    expect(n.dependents.length).toBe(6);
    expect(n.truncated.dependents).toBe(2);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run (from `web/`): `npx vitest run src/lib/graph.test.ts`
Expected: FAIL — `neighborhood` not found.

- [ ] **Step 3: Write minimal implementation**

```ts
// web/src/lib/graph.ts
import type { Ticket } from './api';

export interface NeighborRef {
  id: string;
  title: string;
  status: string;
  priority: string;
  unmet?: boolean;
  inCycle?: boolean;
}

export interface Neighborhood {
  parent?: NeighborRef;
  blockers: NeighborRef[];
  dependents: NeighborRef[];
  children: NeighborRef[];
  dangling: string[];
  truncated: { blockers: number; dependents: number; children: number };
}

function toRef(t: Ticket, extra: Partial<NeighborRef> = {}): NeighborRef {
  return { id: t.id, title: t.title, status: t.status, priority: t.priority, inCycle: t.inCycle, ...extra };
}

export function neighborhood(ticket: Ticket, all: Record<string, Ticket>, cap = 6): Neighborhood {
  const unmet = new Set(ticket.unmet ?? []);

  const blockersAll = (ticket.deps ?? [])
    .filter((id) => all[id])
    .map((id) => toRef(all[id], { unmet: unmet.has(id) }));

  const dependentsAll = Object.values(all)
    .filter((t) => (t.deps ?? []).includes(ticket.id))
    .map((t) => toRef(t));

  const childrenAll: NeighborRef[] =
    ticket.children && ticket.children.length
      ? ticket.children.map((c) => ({ id: c.id, title: c.title, status: c.status, priority: '' }))
      : Object.values(all).filter((t) => t.parent === ticket.id).map((t) => toRef(t));

  const parentT = ticket.parent ? all[ticket.parent] : undefined;

  return {
    parent: parentT ? toRef(parentT) : undefined,
    blockers: blockersAll.slice(0, cap),
    dependents: dependentsAll.slice(0, cap),
    children: childrenAll.slice(0, cap),
    dangling: ticket.dangling ?? [],
    truncated: {
      blockers: Math.max(0, blockersAll.length - cap),
      dependents: Math.max(0, dependentsAll.length - cap),
      children: Math.max(0, childrenAll.length - cap)
    }
  };
}
```

- [ ] **Step 4: Run test to verify it passes**

Run (from `web/`): `npx vitest run src/lib/graph.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/graph.ts web/src/lib/graph.test.ts
git commit -m "feat(web): pure neighborhood() computes a ticket's one-hop graph"
```

---

### Task 2: `TicketGraph.svelte` component

**Files:**
- Create: `web/src/lib/components/TicketGraph.svelte`

**Interfaces:**
- Consumes: `neighborhood` (Task 1), `workspace.tickets`, `Ticket`.
- Produces: `<TicketGraph {ticket} />` — renders the neighborhood SVG + a11y list, or a muted empty line.

- [ ] **Step 1: Write the component**

```svelte
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
  const isEmpty = $derived(!n.parent && !left.length && !right.length && !n.children.length);

  const midRows = $derived(Math.max(left.length, right.length, 1));
  const topBand = $derived(n.parent ? 58 : 6);
  const bottomBand = $derived(n.children.length ? 62 : 6);
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
</script>

{#if isEmpty}
  <p class="empty">No dependencies or epic links.</p>
{:else}
  <svg class="graph" viewBox={`0 0 ${width} ${height}`} role="img" aria-label={`Relationships for ${ticket.id}`}>
    <defs>
      <marker id="tg-dep" markerWidth="8" markerHeight="8" refX="6.5" refY="4" orient="auto"><path d="M0,0 L8,4 L0,8 Z" fill="var(--color-accent)"/></marker>
      <marker id="tg-warn" markerWidth="8" markerHeight="8" refX="6.5" refY="4" orient="auto"><path d="M0,0 L8,4 L0,8 Z" fill="var(--color-warn)"/></marker>
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
      {@const y = colY(i, left.length)}
      <path class="edge" class:warn={ref.unmet && ref.kind === 'blocker'} class:dim={ref.kind === 'dangling'}
            d={`M${cx - 40},${y + NH / 2} L${cx},${centerY + NH / 2}`}
            marker-end={ref.unmet && ref.kind === 'blocker' ? 'url(#tg-warn)' : 'url(#tg-dep)'} />
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

    <g>
      <rect class="node center" x={cx} y={centerY} width={NW} height={NH} rx="10" />
      <circle cx={cx + 14} cy={centerY + 14} r="4.5" fill={dotColor({ status: ticket.status, inCycle: ticket.inCycle, unmet: ticket.blocked })} />
      <text class="nid" x={cx + 26} y={centerY + 17} font-weight="600">{ticket.id}</text>
      <text class="nsub" x={cx + 12} y={centerY + 30}>{ticket.blocked ? '🔒 blocked' : ticket.status}</text>
    </g>

    {#each right as ref, i (ref.id)}
      {@const y = colY(i, right.length)}
      <path class="edge" d={`M${cx + NW},${centerY + NH / 2} L${cx + COLGAP},${y + NH / 2}`} marker-end="url(#tg-dep)" />
      <a href={`/tickets/${ref.id}`}>
        <rect class="node" x={cx + COLGAP} y={y} width={NW - 40} height={NH} rx="9" />
        <circle cx={cx + COLGAP + 14} cy={y + 14} r="4" fill={dotColor(ref)} />
        <text class="nid" x={cx + COLGAP + 26} y={y + 18}>{ref.id}</text>
        <text class="nsub" x={cx + COLGAP + 12} y={y + 30}>waits on this</text>
      </a>
    {/each}

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
  </svg>

  <ul class="sr-only">
    {#if n.parent}<li>Epic: <a href={`/tickets/${n.parent.id}`}>{n.parent.id}</a></li>{/if}
    {#each n.blockers as b}<li>Blocked by <a href={`/tickets/${b.id}`}>{b.id}</a> ({b.status})</li>{/each}
    {#each n.dependents as d}<li>Blocks <a href={`/tickets/${d.id}`}>{d.id}</a></li>{/each}
    {#each n.children as c}<li>Child <a href={`/tickets/${c.id}`}>{c.id}</a></li>{/each}
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
  .edge { stroke: var(--color-accent); stroke-width: 1.5; fill: none; }
  .edge.warn { stroke: var(--color-warn); }
  .edge.dashed { stroke: var(--color-dim); stroke-dasharray: 5 4; }
  .edge.dim { stroke: var(--color-border); }
  a { cursor: pointer; }
  a:focus-visible rect { outline: 2px solid var(--color-accent); outline-offset: 1px; }
  .sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
  @media (prefers-reduced-motion: reduce) { .graph { transition: none; } }
</style>
```

- [ ] **Step 2: Type-check compiles**

Run (from `web/`): `npm run check`
Expected: no new errors beyond the pre-existing baseline.

- [ ] **Step 3: Commit**

```bash
git add web/src/lib/components/TicketGraph.svelte
git commit -m "feat(web): TicketGraph neighborhood SVG component"
```

---

### Task 3: Mount the graph on the ticket detail page

**Files:**
- Modify: `web/src/routes/tickets/[id]/+page.svelte` (import + replace the `.rels` and `.epic` blocks, ~lines 189–207)

**Interfaces:** Consumes `TicketGraph`.

- [ ] **Step 1: Import the component**

In the `<script>` block of `web/src/routes/tickets/[id]/+page.svelte`, add:
```ts
  import TicketGraph from '$lib/components/TicketGraph.svelte';
```

- [ ] **Step 2: Replace the text relationship blocks**

Delete the existing `{#if ticket.parent || ticket.deps.length} … .rels … {/if}` block and the `{#if ticket.epicProgress} … .epic … {/if}` block (currently ~lines 189–207), and put in their place:
```svelte
    <TicketGraph {ticket} />
```
Leave the surrounding conflict banner / editor markup unchanged.

- [ ] **Step 3: Remove now-unused `.rels` / `.epic` styles**

In the page's `<style>`, delete the `.rels`, `.rels a`, `.dep.unmet`, `.blocked`, `.epic`, `.epic strong`, `.child`, `.cstatus` rules that the removed blocks used (they no longer have markup). Leave every other rule intact. (Svelte warns on unused selectors, so this keeps `npm run check`/build clean.)

- [ ] **Step 4: Build to verify it compiles**

Run (from `web/`): `npm run build`
Expected: build succeeds with no unused-selector warnings for the removed classes.

- [ ] **Step 5: Commit**

```bash
git add web/src/routes/tickets/[id]/+page.svelte
git commit -m "feat(web): show the neighborhood graph on the ticket page"
```

---

### Task 4: End-to-end verification

- [ ] **Step 1: Web unit suite + build**

Run (from `web/`): `npx vitest run` then `npm run build`
Expected: all tests pass (including `graph.test.ts`), build succeeds.

- [ ] **Step 2: Manual e2e**

```bash
make build
cd /tmp && rm -rf graph-demo && mkdir graph-demo && cd graph-demo && git init -q && git checkout -b main
/path/to/pine init
/path/to/pine serve --port 3599 &
```
Create three tickets, then wire relationships by editing their frontmatter/body:
- an EPIC,
- a FEAT with `parent: EPIC-…`,
- a BUG with `deps: [FEAT-…]`.
Open the BUG. Expected: the graph shows the FEAT as a blocker on the left (amber if the FEAT isn't done), nothing on the right; open the FEAT and it shows the EPIC above (dashed) and the BUG on the right ("waits on this"). Click the epic node → navigates to it. Stop the server (`kill %1`).

- [ ] **Step 3: Cycle check (optional)**

Make two tickets depend on each other (`A.deps=[B]`, `B.deps=[A]`). Open either → both the center and the neighbor render red (from `inCycle`).

- [ ] **Step 4: Final commit (only if fixes were needed)**

```bash
git commit -am "test: dependency-graph end-to-end fixes" || true
```
