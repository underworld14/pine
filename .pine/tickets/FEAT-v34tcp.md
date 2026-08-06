---
id: FEAT-v34tcp
title: Obsidian-lite interactive GraphView (force layout, drag, hover focus)
status: done
priority: medium
created: "2026-08-06T04:02:52Z"
updated: "2026-08-06T04:05:34Z"
---

# Description

Upgrade `/graph` GraphView to an Obsidian-style interactive force graph: d3-force layout, draggable nodes, hover neighbor focus, while keeping zoom/pan, filters, and custom SVG rendering. TicketGraph (detail page) stays radial.

# Acceptance Criteria
- [x] Force-directed layout via `d3-force`, seeded from deterministic ring/grid
- [x] Circular nodes colored by kind; short id labels
- [x] Drag nodes (pin fx/fy); background pan + wheel zoom unchanged
- [x] Hover focuses self + 1-hop neighbors; dims the rest
- [x] Filters rebuild the simulation; `prefers-reduced-motion` settles silently
- [x] Unit tests for seed/adjacency/focus/simulation; svelte-check clean

# Implementation Plan
1. Add `d3-force`
2. `web/src/lib/graph-force.ts` + tests
3. Rewrite `GraphView.svelte`
4. Subtitle hint on `/graph`

# Notes
Implemented. TicketGraph intentionally out of scope.

# Related Files
- web/src/lib/graph-force.ts
- web/src/lib/graph-force.test.ts
- web/src/lib/components/GraphView.svelte
- web/src/routes/graph/+page.svelte
- web/package.json

# Attachments

## Work Evidence

Closed by `pine close --evidence` on 2026-08-06.

- Base: `034f6411` (last commit at or before ticket created 2026-08-06)
- Files changed (base → working tree):

```
 .pine/templates/epic.md                        |   2 -
 internal/cli/coverage_test.go                  |  16 +-
 internal/cli/init.go                           |   2 +-
 internal/cli/tickets.go                        |  15 +-
 internal/store/create.go                       |   2 +-
 web/package-lock.json                          |  50 +++++
 web/package.json                               |   2 +
 web/src/lib/components/BoardFilterBar.svelte   |   9 +-
 web/src/lib/components/BoardFilterBar.test.ts  |   2 +
 web/src/lib/components/GraphView.svelte        | 282 ++++++++++++++++---------
 web/src/lib/components/NewIssueModal.svelte    |  27 ++-
 web/src/lib/components/QuickPeekPopover.svelte |  25 +--
 web/src/lib/components/TicketCard.svelte       |   4 +-
 web/src/lib/components/TicketCard.test.ts      |   3 +-
 web/src/lib/components/TicketGraph.svelte      | 118 ++++++++---
 web/src/lib/graph.test.ts                      |   8 +
 web/src/lib/ui-helpers.ts                      |  29 ++-
 web/src/lib/ui.svelte.ts                       |   4 +-
 web/src/routes/+page.svelte                    |   4 +-
 web/src/routes/graph/+page.svelte              |   2 +-
 web/src/routes/tickets/[id]/+page.svelte       |  66 ++++--
 21 files changed, 471 insertions(+), 201 deletions(-)
```
