---
id: FEAT-kg580g
title: 'Phase 3: contract, runner, babysit loop, pine run'
status: todo
priority: high
labels:
    - agents
deps:
    - FEAT-4hwdct
parent: EPIC-23747d
phase: p3
created: "2026-08-08T18:23:56Z"
updated: "2026-08-08T18:23:56Z"
---

# Description

The core: spawn a harness in a worktree, verify the completion contract, resume
with a delta prompt up to a cap, then park.

Spec: `docs/superpowers/specs/2026-08-09-agent-assignment-babysitter-design.md` (sections: Packages, The completion contract, The babysit loop,
Worktrees and branches, Discovered work, Error handling)

# Acceptance Criteria
- [ ] `internal/contract` is pure: status, AC checkboxes, plan non-empty, not degraded; `blocked` label short-circuits with no retry
- [ ] `internal/runner` spawns one attempt in a worktree outside the repo, exports PINE_RUN_ID/PINE_AGENT/PINE_TICKET, enforces a timeout
- [ ] `internal/babysit` retries with a delta prompt to the cap, then applies the `needs-human` label
- [ ] Per-ticket lease prevents two concurrent runs of the same ticket
- [ ] Discovered work: create under PINE_TICKET stamps `links` and `discovered`, leaves assignee empty, honours `maxDiscoveries`
- [ ] `.pine/runs/` is gitignored and excluded from the watcher
- [ ] No test invokes a real AI harness

# Implementation Plan

# Notes

# Related Files

# Attachments
