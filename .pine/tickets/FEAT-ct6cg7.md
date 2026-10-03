---
id: FEAT-ct6cg7
title: 'Phase 5: skills and recipe layer'
status: todo
priority: high
labels:
    - agents
deps:
    - FEAT-kg580g
parent: EPIC-23747d
phase: p5
created: "2026-10-02T08:59:19Z"
updated: "2026-10-02T08:59:19Z"
---

# Description

Curated third-party agent skills (superpowers, antislop, impeccable,
web-design-guidelines, find-docs, …) become a layer Pine orchestrates: Pine
curates and sequences them, the open SKILL.md ecosystem distributes them.
Nothing is vendored into this repository or the binary — upstream skills are
actively maintained and a fork would rot.

This is what makes the babysitter serve non-expert users: the contract
requires non-empty Acceptance Criteria and Implementation Plan, and the
planning skills are how a novice produces good ones before attempt 1.

Design: `docs/superpowers/specs/2026-08-09-agent-assignment-babysitter-design.md` (section: Skills layer)

# Acceptance Criteria
- [ ] Project config stores skill references as name + pinned version only; skill content is never committed or vendored (same trust model as harness presets: project references, machine fetches)
- [ ] `pine skills list/add/remove` resolves against the open skills registry (skills.sh / SKILL.md standard) and installs into per-harness skill directories; every update shows a content diff before it is applied
- [ ] Recipes (`pine init --recipe frontend|backend|…`) install a deliberately small curated set and map each skill to a loop phase: planning, during-run, review
- [ ] Scope split mirrors the config split: method skills install user-global, project skills install in-repo
- [ ] The babysit prompt gains a skills layer selected by phase; overlapping skills (e.g. two UI-craft skills) are never co-injected
- [ ] `pine doctor` flags version drift and content changed since pin
- [ ] No test fetches skill content; the resolver is faked

# Implementation Plan

# Notes

Recommended starter set (curated, revisable): superpowers (methodology:
brainstorm/plan/TDD/subagent review), antislop (load-always filter),
impeccable or Anthropic's frontend-design (craft), web-design-guidelines
(audit), find-docs (docs accuracy). Five to eight skills, not a bundle.

# Related Files

# Attachments
