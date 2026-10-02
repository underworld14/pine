---
id: FEAT-n4rw00
title: verify frontmatter field run by close --evidence
status: done
priority: medium
labels:
    - agent-feedback
phase: p2
verify: grep -q "case \"verify\":" internal/ticket/frontmatter.go
created: "2026-10-02T08:35:53Z"
updated: "2026-10-02T08:49:46Z"
---

# Description

EPIC-12 audit: 8 "done" tickets existed only in library and test code with no production callers. Done must prove it is live.

# Notes

- New `verify:` frontmatter field (create --verify / update --verify, 'none' clears), round-tripped and exposed in --json.
- `pine close --evidence` runs it via `sh -c` in the repo root (2 min timeout), appends a `

## Verification

Ran by `pine close --evidence` on 2026-10-02 — **passed** (exit 0, 4ms).

Command:

```
grep -q "case \"verify\":" internal/ticket/frontmatter.go
```

_(no output)_

## Work Evidence

Closed by `pine close --evidence` on 2026-10-02.

- Base: `30431afa` (last commit at or before ticket created 2026-10-02)
- Files changed (base → working tree):

```
 .agents/skills/pine/SKILL.md           |  49 ++++++++++---
 .claude/skills/pine/SKILL.md           |  49 ++++++++++---
 README.md                              |  49 +++++++++++--
 internal/cli/aicmds.go                 |  70 +++++++++++++++---
 internal/cli/cli_test.go               | 102 ++++++++++++++++++++++++--
 internal/cli/coverage_test.go          |  38 +++++++++-
 internal/cli/evidence.go               | 125 +++++++++++++++++++++++++++++---
 internal/cli/evidence_test.go          |  58 +++++++++++++++
 internal/cli/init.go                   |   6 +-
 internal/cli/learn.go                  |  13 ++--
 internal/cli/learn_memory.go           |   4 +-
 internal/cli/learn_memory_unit_test.go |  25 +++++++
 internal/cli/tickets.go                | 128 ++++++++++++++++++++++++++++-----
 internal/contextgen/context.go         |  14 +++-
 internal/contextgen/contextgen_test.go |  70 ++++++++++++++++++
 internal/contextgen/memory.go          |  30 +++++++-
 internal/contextgen/memory_test.go     |  23 ++++++
 internal/contextgen/prompt.go          |  16 +++++
 internal/doctor/doctor.go              |  54 ++++++++++++++
 internal/doctor/doctor_test.go         |  37 ++++++++++
 internal/memory/memory.go              |  41 +++++++++--
 internal/memory/memory_test.go         |  32 +++++++++
 internal/server/aicontext.go           |  29 +++++++-
 internal/store/create.go               |   2 +
 internal/ticket/frontmatter.go         |   9 ++-
 internal/ticket/frontmatter_test.go    |  30 ++++++++
 internal/ticket/ticket.go              |   1 +
 internal/view/view.go                  |   3 +
 28 files changed, 1012 insertions(+), 95 deletions(-)
```
