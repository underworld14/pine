---
id: FEAT-p0cj9t
title: 'Phase 1: assignee as a first-class ticket field'
status: todo
priority: high
labels:
    - agents
parent: EPIC-23747d
phase: p1
created: "2026-08-08T18:23:56Z"
updated: "2026-08-08T18:23:56Z"
---

# Description

Make `assignee` a real Pine field: parsed, serialized, merged, filterable, and
visible in CLI, HTTP API, and board. Records ownership only — nothing executes.

Spec: `docs/superpowers/specs/2026-08-09-agent-assignment-babysitter-design.md`
Plan: `docs/superpowers/plans/2026-08-09-agent-assignment-phase-1-assignee.md`

# Acceptance Criteria
- [ ] `assignee` is a known frontmatter key, written between `order` and `labels`, omitted when empty
- [ ] `Merge3` merges it as a scalar; one-sided change does not conflict
- [ ] `pine assign <ID> <agent|none>` works; `--assignee` on create/update/list/ready; `--unassigned` on list/ready
- [ ] API: `?assignee=`, `?unassigned=`, POST and PATCH accept `assignee`
- [ ] Board cards show an @agent chip; free-text filter matches it
- [ ] `make cover` stays at or above 90%

# Implementation Plan

See the plan document — 7 tasks, TDD, one commit each.

# Notes

# Related Files

# Attachments
