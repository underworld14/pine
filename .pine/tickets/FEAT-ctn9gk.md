---
id: FEAT-ctn9gk
title: 'Phase 6: parallel scheduler with dependency waves'
status: todo
priority: high
labels:
    - agents
deps:
    - FEAT-kg580g
parent: EPIC-23747d
phase: p6
created: "2026-08-08T18:23:56Z"
updated: "2026-10-02T08:59:19Z"
---

# Description

Run every runnable assigned ticket concurrently, bounded by maxConcurrent,
respecting dependencies satisfied in the base branch.

Spec: `docs/superpowers/specs/2026-08-09-agent-assignment-babysitter-design.md` (section: Scheduler)

# Acceptance Criteria
- [ ] A ticket is runnable only when its deps are `done` on the base branch
- [ ] Concurrency is capped by `agents.maxConcurrent` (machine config)
- [ ] `pine run --ready` starts the current wave; `pine runs` shows all of them
- [ ] Pine never merges automatically; completed branches are reported, not integrated
- [ ] A dep done only on a side branch does not unlock its dependent (regression test)

# Implementation Plan

# Notes

# Related Files

# Attachments
