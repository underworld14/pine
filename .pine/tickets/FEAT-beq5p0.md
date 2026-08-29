---
id: FEAT-beq5p0
title: Ready tree with epic context, drop --flat, add phase
status: done
priority: high
created: "2026-08-07T01:23:35Z"
updated: "2026-08-07T01:26:32Z"
---

# Description

Make `pine ready` show epic→child trees (injecting non-ready parent epics as
context headers), remove `pine list --flat`, and add optional first-class
`phase` frontmatter (p0/p1/…).

# Acceptance Criteria
- [x] `pine ready` text output is an epic→children tree
- [x] Non-ready parent epics appear as headers when they have ready children; JSON stays ready-only
- [x] `--flat` removed from `pine list`
- [x] Optional `phase` field end-to-end (frontmatter, CLI create/update/list, API, tree/show)

# Implementation Plan
- Ready: `withEpicContext` + `renderTicketTree`
- Drop `--flat` / `renderTicketTable`
- Wire `phase` through ticket → view → store → server → CLI → web api.ts

# Notes

# Related Files
- internal/cli/tickets.go, list_tree.go
- internal/ticket/{ticket,frontmatter,merge}.go
- internal/view/view.go, internal/store/{store,create}.go
- internal/server/{tickets,crossbranch}.go
- web/src/lib/api.ts, README.md

# Attachments

## Work Evidence

Closed by `pine close --evidence` on 2026-08-07.

- Base: `21c8c352` (last commit at or before ticket created 2026-08-07)
- Files changed (base → working tree):

```
 README.md                           |   3 +-
 internal/cli/coverage_test.go       |  91 ++++++++++++++++++++++++++++--
 internal/cli/list_tree.go           |   5 ++
 internal/cli/list_tree_test.go      |   7 +++
 internal/cli/tickets.go             | 108 ++++++++++++++++++++++++++----------
 internal/contextgen/context.go      |   2 +-
 internal/server/crossbranch.go      |   3 +
 internal/server/tickets.go          |   7 +++
 internal/store/create.go            |   2 +
 internal/store/store.go             |   4 ++
 internal/ticket/frontmatter.go      |   8 ++-
 internal/ticket/frontmatter_test.go |  30 ++++++++++
 internal/ticket/merge.go            |   4 +-
 internal/ticket/ticket.go           |   1 +
 internal/view/view.go               |   3 +
 web/src/lib/api.ts                  |   1 +
 16 files changed, 238 insertions(+), 41 deletions(-)
```
