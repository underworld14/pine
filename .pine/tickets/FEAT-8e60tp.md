---
id: FEAT-8e60tp
title: 'Phase 5: epic queue with inherited assignment'
status: todo
priority: medium
labels:
    - agents
deps:
    - FEAT-ctn9gk
parent: EPIC-23747d
phase: p5
created: "2026-08-08T18:23:56Z"
updated: "2026-08-08T18:23:56Z"
---

# Description

`pine run <EPIC-ID>` fans an epic out into one independent run per child.

Spec: `docs/superpowers/specs/2026-08-09-agent-assignment-babysitter-design.md` (section: Epic queue)

# Acceptance Criteria
- [ ] Children without their own assignee inherit the epic's
- [ ] Each child is a separate run with its own contract and branch
- [ ] A parked child does not stop its siblings
- [ ] The command prints a per-child summary

# Implementation Plan

# Notes

# Related Files

# Attachments
