# Pine — Per-Ticket Dependency & Epic Graph

**Status:** Design approved, pending spec review
**Date:** 2026-07-05

## Problem & Goal

The ticket detail page shows relationships as flat text chips: `epic: … / dep: … /
🔒 blocked`. That doesn't convey structure — you can't see at a glance *what blocks this
ticket, what waits on it, and where it sits in its epic*.

**Goal:** replace those chips with a small **neighborhood graph** on the ticket page —
the current ticket at the center, one hop out in every direction — so structure and
blocking are visible instantly. Adopts the spirit of Backlog.md's dependency graph,
scoped to what stays readable at any project size.

## Key decisions (locked)

- **Scope: per-ticket neighborhood** (Option A), on the `tickets/[id]` page. The
  whole-project map is explicitly **v2**.
- **Both relationships**, drawn differently: **dependencies** as solid arrows
  ("blocked by" / "blocks"), **epic hierarchy** (parent / children) as dashed links.
- **No backend work.** Everything the graph needs is already on `view.Ticket`
  (`deps`, `blocked`, `unmet`, `dangling`, `inCycle`, `parent`, `children`,
  `epicProgress`, `status`, `priority`) and the frontend already holds every ticket in
  the `workspace` store. This is a pure SvelteKit rendering layer.
- **Fixed radial layout, no graph library** — deterministic positions computed from
  counts. Keeps the bundle lean (Pine ships as one binary) and the picture always legible.

## Non-goals (v1)

- No whole-project `/graph` page (v2).
- No force/physics layout, no pan/zoom, no drag.
- No multi-hop expansion (exactly one hop from the center).
- No editing relationships from the graph (that stays in the markdown body / fields).

## Architecture

```
workspace.tickets (all tickets, already in memory)
        │
        ▼
graph.ts  neighborhood(ticket, tickets) ─► { parent, blockers, dependents, children, dangling }
        │                                    (pure, testable — no DOM, no fetch)
        ▼
TicketGraph.svelte  ─► deterministic radial SVG (epic ▲ · blockers ◀ · center · dependents ▶ · children ▼)
        │
        ▼
tickets/[id]/+page.svelte  replaces the .rels + .epic text blocks
```

"Dependents" (tickets waiting on this one) is the one relationship not precomputed
server-side; it's a client-side reverse scan over `workspace.tickets` — O(n) once per page
view, negligible at Pine's scale.

## Components

### 1. `web/src/lib/graph.ts` (new) — pure neighborhood computation
```ts
interface NeighborRef { id; title; status; priority; blocked?; inCycle?; }
interface Neighborhood {
  parent?: NeighborRef;          // ticket.parent resolved (dashed, above)
  blockers: NeighborRef[];       // ticket.deps resolved (solid → center); unmet = amber
  dependents: NeighborRef[];     // tickets whose deps include this id (solid center →)
  children: NeighborRef[];       // tickets whose parent is this id (dashed, below)
  dangling: string[];            // ticket.dangling — dep ids not in the set
  truncated: { blockers; dependents; children };  // counts hidden by the per-arm cap
}
function neighborhood(ticket: Ticket, all: Record<string, Ticket>, cap = 6): Neighborhood
```
- `blockers` = `ticket.deps.map(resolve)`; a blocker whose id ∈ `ticket.unmet` is drawn
  amber (still blocking), a met one green.
- `dependents` = `Object.values(all).filter(t => t.deps.includes(ticket.id))`.
- `children` = `ticket.children` when present, else reverse-scan `parent === ticket.id`.
- `inCycle` carried from each ticket so cycle members render red (incl. a self-dependency).
- Each arm capped at `cap`; overflow recorded in `truncated` (rendered as a `+N more` node).

### 2. `web/src/lib/components/TicketGraph.svelte` (new)
Props: `{ ticket: Ticket }`. Reads `workspace.tickets`, calls `neighborhood`, renders an
SVG whose `viewBox` height is derived from `max(blockers, dependents, children).length`.

Layout (fixed):
- **Center**: current ticket, accent border, status/priority styling.
- **Parent epic**: single node above, dashed edge down; shows `epicProgress` (`3 / 7 done`).
- **Blockers**: column on the **left**, solid arrows pointing **into** the center; amber if unmet.
- **Dependents**: column on the **right**, solid arrows **from** the center.
- **Children** (epics only): row **below**, dashed edges down.
- **Dangling** blockers: dim dashed "missing" node so a broken ref is visible, not silent.

Styling reuses `priorityMeta` + status colors from `ui-helpers.ts`; solid edges =
dependency, dashed = epic; cycle members + their edges are red (`--color-danger`).

Interaction & a11y:
- Every non-center node is a real `<a href="/tickets/{id}">` (keyboard-focusable, visible
  focus ring); the center is inert.
- SVG has `role="img"` + an `aria-label` summarizing the neighborhood; a visually-hidden
  `<ul>` mirrors the nodes as links so screen readers and no-SVG fallbacks get the same info.
- No essential animation; honor `prefers-reduced-motion`.

### 3. `web/src/routes/tickets/[id]/+page.svelte`
Replace the `.rels` chip block and the `.epic` children list (current lines ~189–207)
with `<TicketGraph {ticket} />`, rendered only when the neighborhood is non-empty
(any parent/blocker/dependent/child/dangling). When empty, show a muted
"No dependencies or epic links." The `epicProgress` that the old `.epic` block displayed
now lives on the epic/center node.

## Layout math (deterministic)

- Node box ≈ 120×40, row pitch 52, column gap 150.
- `rows = max(blockers.length, dependents.length, children? ...)`; SVG height =
  `topBand(epic) + rows*pitch + bottomBand(children)`.
- Center vertically aligned to the taller of blockers/dependents; each column top-aligned
  and centered around the center row. Arrows drawn from box edge to box edge.
All arithmetic; no runtime layout engine.

## Edge cases

- **No relationships** → component renders the muted empty line, no SVG.
- **Dangling dep** (`ticket.dangling`) → dim dashed node labeled with the missing id.
- **Cycle** → center and the cyclic direct neighbor both red, their connecting edge red;
  **self-dependency** → a small red self-loop on the center.
- **Large fan-out** → capped per arm at 6 with a `+N more` node; the hidden ids remain in
  the visually-hidden list for a11y / findability (no silent truncation).
- **Off-branch (read-only) center** → graph still renders; neighbor nodes link normally
  (an off-branch neighbor resolves via the overlay or shows "not found" like any id).
- **Off-branch neighbors missing locally** → shown from whatever the store has; a
  neighbor id absent from the store renders as a dim "unknown" node (same as dangling).

## Testing

**Unit (`web/src/lib/graph.test.ts`, Vitest):** `neighborhood()` —
- resolves blockers from `deps`, marks unmet vs met;
- computes dependents via reverse scan;
- resolves parent and children (both `children[]` and reverse-scan paths);
- surfaces `dangling`; flags `inCycle` (incl. self-dependency);
- caps each arm and reports `truncated`.

**Component smoke:** renders center + one of each arm; nodes are links to
`/tickets/{id}`; empty neighborhood shows the muted line.

**End-to-end:** open a ticket that has a blocker, a dependent, and an epic parent → the
graph shows all three with correct edge styles; clicking the epic node navigates to it;
a ticket in a dependency cycle shows red.

## Critical files

| File | Change |
|---|---|
| `web/src/lib/graph.ts` (new) | pure `neighborhood(ticket, all, cap)` + types |
| `web/src/lib/components/TicketGraph.svelte` (new) | deterministic radial SVG + a11y list |
| `web/src/routes/tickets/[id]/+page.svelte` | mount graph; retire the `.rels`/`.epic` text blocks |
| `web/src/lib/ui-helpers.ts` | reuse `priorityMeta` / status colors (add a small status-color helper if needed) |

## v2 (noted, not built)

Whole-project `/graph` route: all tickets laid out as a layered dependency DAG with epic
clusters and highlighted cycles. Reuses the node/edge rendering built here; adds a DAG
layout pass (e.g. layered/Sugiyama) and viewport controls. Deferred because it needs a
real layout algorithm and degrades in readability past ~50–100 tickets.
