---
id: FEAT-qsjwpw
title: Memory bullet evidence and staleness checks
status: done
priority: medium
labels:
    - agent-feedback
phase: p3
verify: grep -q checkMemoryAge internal/doctor/doctor.go
created: "2026-10-02T08:35:54Z"
updated: "2026-10-02T08:49:46Z"
---

# Description

Agent feedback: project memory can be stale with no marker — a refuted note displayed on par with true facts.

# Notes

- `pine learn --evidence "<cmd|citation>"` records the proof inline with the dated bullet.
- `pine doctor` (checkMemoryAge) flags undated bullets and aggregates bullets older than memory.StaleDays=90 per file.
- `pine context` marks stale bullets `⚠ unverified since <date>` in Project Memory and topic excerpts.

# Related Files
- internal/memory/memory.go — AppendOpts.Evidence, ParseBulletDate, StaleDays
- internal/doctor/doctor.go — checkMemoryAge
- internal/contextgen/memory.go — markStaleLines
- internal/cli/learn.go, internal/cli/learn_memory.go — --evidence flag

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

## Verification

Ran by `pine close --evidence` on 2026-10-02 — **passed** (exit 0, 5ms).

Command:

```
grep -q checkMemoryAge internal/doctor/doctor.go
```

_(no output)_
