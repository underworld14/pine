---
id: FEAT-0reph4
title: Document Related Files format, JSON shape, new flags
status: done
priority: medium
labels:
    - agent-feedback
phase: p5
verify: grep -q "Related Files" README.md
created: "2026-10-02T08:35:54Z"
updated: "2026-10-02T08:49:47Z"
---

# Description

Agent feedback: the Related Files format was undocumented (guessed) and the --json shape unclear.

# Notes

- `pine create --help` documents the body sections with a Related Files example and the JSON DTO fields.
- README gains a Ticket format section: verify field, Related Files grammar, JSON shape, dep rewiring, compact context.
- Both agent skill files (.agents/.claude) teach the new commands: --full, --body-file, dep add, verify + close --evidence --force, learn --evidence, stale-memory discipline.

# Related Files
- README.md — ticket format and AI context sections
- .agents/skills/pine/SKILL.md, .claude/skills/pine/SKILL.md — agent guidance
- internal/cli/tickets.go — create Long help

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

Ran by `pine close --evidence` on 2026-10-02 — **passed** (exit 0, 4ms).

Command:

```
grep -q "Related Files" README.md
```

_(no output)_
