---
id: FEAT-wdgfy6
title: Peek sheet edit parity with full ticket page
status: done
priority: medium
created: "2026-08-06T03:48:33Z"
updated: "2026-08-06T03:53:46Z"
---

# Description

# Acceptance Criteria
- [ ] Define acceptance criteria

# Implementation Plan

# Notes

# Related Files

# Attachments

## Work Evidence

Closed by `pine close --evidence` on 2026-08-06.

- Base: `034f6411` (last commit at or before ticket created 2026-08-06)
- Files changed (base → working tree):

```
 .pine/templates/epic.md                        |   2 -
 internal/cli/coverage_test.go                  |  16 +++-
 internal/cli/init.go                           |   2 +-
 internal/cli/tickets.go                        |  15 +++-
 internal/store/create.go                       |   2 +-
 web/src/lib/components/BoardFilterBar.svelte   |   9 +-
 web/src/lib/components/BoardFilterBar.test.ts  |   2 +
 web/src/lib/components/NewIssueModal.svelte    |  27 ++++--
 web/src/lib/components/QuickPeekPopover.svelte |  25 ++----
 web/src/lib/components/TicketCard.svelte       |   4 +-
 web/src/lib/components/TicketCard.test.ts      |   3 +-
 web/src/lib/components/TicketGraph.svelte      | 118 ++++++++++++++++++-------
 web/src/lib/graph.test.ts                      |   8 ++
 web/src/lib/ui-helpers.ts                      |  29 ++++--
 web/src/lib/ui.svelte.ts                       |   4 +-
 web/src/routes/+page.svelte                    |   4 +-
 web/src/routes/tickets/[id]/+page.svelte       |  66 ++++++++++----
 17 files changed, 237 insertions(+), 99 deletions(-)
```
