---
id: FEAT-ndg5rr
title: 'Phase 6: ACL-lite agent messages and inbox'
status: todo
priority: medium
labels:
    - agents
deps:
    - FEAT-kg580g
parent: EPIC-23747d
phase: p6
created: "2026-08-08T18:23:56Z"
updated: "2026-08-08T18:23:56Z"
---

# Description

Four performatives over append-only plain files, injected into agent prompts.

Spec: `docs/superpowers/specs/2026-08-09-agent-assignment-babysitter-design.md` (section: ACL-lite messages)

# Acceptance Criteria
- [ ] `.pine/messages/<TICKET-ID>.md` append-only; `request`/`inform`/`block`/`handoff`
- [ ] `pine msg send` attributes the sender from PINE_AGENT, else `human`
- [ ] `pine inbox --agent <name>` lists messages addressed to that agent
- [ ] Messages for the ticket are injected into the run prompt
- [ ] Pine posts `inform` on park and `block` on dependency failure
- [ ] No read state is stored

# Implementation Plan

# Notes

# Related Files

# Attachments
