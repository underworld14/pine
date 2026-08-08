---
id: FEAT-4hwdct
title: 'Phase 2: pine config, agents schema, harness presets'
status: todo
priority: high
labels:
    - agents
deps:
    - FEAT-p0cj9t
parent: EPIC-23747d
phase: p2
created: "2026-08-08T18:23:56Z"
updated: "2026-08-08T18:23:56Z"
---

# Description

Declarative agent profiles and the project/machine config split. Still zero
execution.

Spec: `docs/superpowers/specs/2026-08-09-agent-assignment-babysitter-design.md` (sections: Data model, Harness presets, `pine config`)

# Acceptance Criteria
- [ ] `internal/agentprofile` resolves `assignee` to a profile and renders a command line
- [ ] Project config holds `agents.profiles`, `agents.contract`, `agents.maxDiscoveries`
- [ ] Machine config is `~/.pine/agents.json` — NOT `config.json` (would break `isGlobalOnlyStore`)
- [ ] Project config is rejected if it defines a `command` array or `serveControl`
- [ ] `pine config get/set/unset/list` with `-g`; scope errors name the correct command
- [ ] Presets ship for Claude Code, Codex, Cursor, Gemini, Pi with verified headless flags
- [ ] `pine doctor` flags unknown harness, missing binary, assignee naming no profile

# Implementation Plan

# Notes

# Related Files

# Attachments
