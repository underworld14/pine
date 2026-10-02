---
id: FEAT-v95r79
title: 'update: --body-file, --add-dep, --rm-dep flags'
status: done
priority: medium
labels:
    - agent-feedback
phase: p0
verify: grep -q "add-dep" internal/cli/tickets.go
created: "2026-10-02T08:35:53Z"
updated: "2026-10-02T08:49:46Z"
---

# Description

Agent feedback: deps could not be changed after creation, and the body had to be rewritten by hand in the ticket file.

# Notes

- `pine update` gains `--body-file` (path, or `-` for stdin) to replace the markdown body via the CLI.
- `--add-dep` / `--rm-dep` rewire dependencies with the same cycle refusal as `pine dep add/rm`.
- Help text now points users at `pine dep add/rm` for dependency edits.

# Related Files
- internal/cli/tickets.go — update command flags and cycle check
- internal/cli/dep.go — reused mergeDeps/removeDeps/wouldCycle helpers
- internal/cli/cli_test.go — TestUpdateBodyFileAndDeps

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
grep -q "add-dep" internal/cli/tickets.go
```

_(no output)_
