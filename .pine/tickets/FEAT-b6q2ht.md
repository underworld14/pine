---
id: FEAT-b6q2ht
title: 'Phase 7: web UI for agents, runs, and board dispatch'
status: todo
priority: high
labels:
    - agents
deps:
    - FEAT-ctn9gk
    - FEAT-ndg5rr
parent: EPIC-23747d
phase: p7
created: "2026-08-08T18:23:56Z"
updated: "2026-08-08T18:23:56Z"
---

# Description

Manage profiles, register harnesses, watch runs, and start work from the board.

Spec: `docs/superpowers/specs/2026-08-09-agent-assignment-babysitter-design.md` (sections: HTTP API, Web UI, Templates, Security)

# Acceptance Criteria
- [ ] `/agents`: profile cards, installed badge, Register-from-preset, preamble templates, Test (renders, executes nothing)
- [ ] `/runs`: run table, streaming logs over the existing SSE channel, Abort
- [ ] Board: assignee facet in BoardFilterBar, needs-human marker, per-card Run, "Run ready work" with a preview modal
- [ ] Role preamble templates land in `.pine/templates/agents/` via `pine init`
- [ ] `serveControl` off by default; with it off every write and run-start endpoint is rejected (security test required)
- [ ] Machine-scoped fields render read-only with a copyable `pine config set -g` hint

# Implementation Plan

# Notes

# Related Files

# Attachments
