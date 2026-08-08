---
id: FEAT-yge4kt
title: 'Phase 8: orchestrator review of diff versus acceptance criteria'
status: todo
priority: low
labels:
    - agents
deps:
    - FEAT-kg580g
parent: EPIC-23747d
phase: p8
created: "2026-08-08T18:23:56Z"
updated: "2026-08-08T18:23:56Z"
---

# Description

Close the biggest hole in a state-only contract — a ticked checkbox is not
correct work — by delegating review to an already-authenticated profile.

Spec: `docs/superpowers/specs/2026-08-09-agent-assignment-babysitter-design.md` (section: Pine as orchestrator)

# Acceptance Criteria
- [ ] `agents.orchestrator` names an existing profile; empty disables the feature
- [ ] A `ContractExtension` reviews the run's diff against the acceptance criteria and can fail the contract
- [ ] Pine holds no API key and no model credentials of its own
- [ ] The review verdict is attached to the run and posted as an `inform` message

# Implementation Plan

# Notes

# Related Files

# Attachments
