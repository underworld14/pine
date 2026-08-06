---
id: FEAT-36c20m
title: Tree/graph list rendering for pine list
status: done
priority: medium
created: "2026-08-06T03:39:34Z"
updated: "2026-08-06T03:43:28Z"
---

# Description

Make `pine list` render like Beads `bd list` — hierarchical tree by parent/epic,
with status icons and blocker annotations. Keep `--flat` for the classic table.

# Acceptance Criteria
- [x] Default `pine list` nests children under epics with box-drawing connectors
- [x] Status icons: ○ todo / ◐ doing / ◑ testing / ● blocked / ✓ done
- [x] Blocked tickets show unmet dep IDs; epics show (done/total) progress
- [x] `--flat` restores the previous table
- [x] Unit + CLI tests cover tree rendering

# Implementation Plan
- Research bd list --tree format
- Add `list_tree.go` + wire into `newListCmd`
- Tests in `list_tree_test.go`; adjust coverage expectations

# Notes
- bd defaults `--tree` to true with `--flat` to disable; Pine mirrors that.
- Nesting uses Pine `parent` field (not dep edges). Deps only annotate blockers.
- Did not touch web TicketRelations work.

# Related Files
- internal/cli/list_tree.go
- internal/cli/list_tree_test.go
- internal/cli/tickets.go
- internal/cli/coverage_test.go

# Attachments

## Work Evidence

Closed by `pine close --evidence` on 2026-08-06.

- Base: `034f6411` (last commit at or before ticket created 2026-08-06)
- Files changed (base → working tree):

```
 .pine/templates/epic.md                     |   2 -
 internal/cli/coverage_test.go               |  16 +++-
 internal/cli/init.go                        |   2 +-
 internal/cli/tickets.go                     |  15 +++-
 internal/store/create.go                    |   2 +-
 web/src/lib/components/NewIssueModal.svelte |  15 ++++
 web/src/lib/components/TicketGraph.svelte   | 118 ++++++++++++++++++++--------
 web/src/lib/graph.test.ts                   |   8 ++
 web/src/lib/ui.svelte.ts                    |   4 +-
 web/src/routes/tickets/[id]/+page.svelte    |  47 +++++++++--
 10 files changed, 180 insertions(+), 49 deletions(-)
```
