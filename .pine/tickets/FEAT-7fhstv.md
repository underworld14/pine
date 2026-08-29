---
id: FEAT-7fhstv
title: 'pine upgrade: self-update from GitHub Releases'
status: done
priority: high
created: "2026-08-07T02:09:38Z"
updated: "2026-08-07T02:11:57Z"
---

# Description

# Acceptance Criteria
- [ ] Define acceptance criteria

# Implementation Plan

# Notes

# Related Files

# Attachments

## Work Evidence

Closed by `pine close --evidence` on 2026-08-07.

- Base: `21c8c352` (last commit at or before ticket created 2026-08-07)
- Files changed (base → working tree):

```
 README.md                           |  20 ++++++-
 internal/cli/coverage_test.go       |  91 ++++++++++++++++++++++++++++--
 internal/cli/list_tree.go           |   5 ++
 internal/cli/list_tree_test.go      |   7 +++
 internal/cli/root.go                |  12 +++-
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
 17 files changed, 264 insertions(+), 44 deletions(-)
```
