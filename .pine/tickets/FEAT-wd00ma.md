---
id: FEAT-wd00ma
title: Epic relations UI and graph clipping fix
status: done
priority: medium
created: "2026-08-06T03:22:20Z"
updated: "2026-08-06T03:27:11Z"
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
 web/src/lib/components/TicketGraph.svelte | 121 +++++++++++++++++++++---------
 web/src/lib/graph.test.ts                 |   8 ++
 web/src/routes/tickets/[id]/+page.svelte  |  39 ++++++++--
 3 files changed, 129 insertions(+), 39 deletions(-)
```
