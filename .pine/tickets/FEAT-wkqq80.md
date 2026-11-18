---
id: FEAT-wkqq80
title: 'Docs: how to commit ticket edits; drop nonexistent pine inject'
status: done
priority: medium
labels:
    - agents
parent: EPIC-6j541d
verify: '! grep -rn ''pine inject'' internal/setup/templates README.md && grep -q ''Committing ticket changes'' README.md'
created: "2026-11-18T04:06:18Z"
updated: "2026-11-18T04:28:50Z"
---

# Description
Pine never commits, but its docs never said how ticket edits should enter history, and the agent templates advertised a nonexistent `pine inject`. Document the convention and the new commands.

# Acceptance Criteria
- [x] README: Committing ticket changes, Work evidence, Release notes, Gating agent edits
- [x] skill.md / core.md: commit guidance and new commands; `pine inject` removed
- [x] Repo's own CLAUDE/AGENTS/GEMINI and skill copies regenerated

# Related Files
- README.md
- internal/setup/templates/core.md
- internal/setup/templates/skill.md

## Work Evidence

Closed by `pine close --evidence` on 2026-11-18.

- Base: `c3a83ce1` (last commit at or before ticket created 2026-11-18)
- Files changed (base → working tree):

```
 .agents/skills/pine/SKILL.md                   |  20 +-
 .claude/skills/pine/SKILL.md                   |  20 +-
 AGENTS.md                                      |   5 +-
 CLAUDE.md                                      |   5 +-
 GEMINI.md                                      |   5 +-
 README.md                                      |  41 ++++
 internal/cli/evidence.go                       |  94 +++++++-
 internal/cli/evidence_test.go                  | 164 +++++++++++++
 internal/cli/root.go                           |   2 +
 internal/cli/tickets.go                        |  64 ++++-
 internal/contextgen/context.go                 |   2 +-
 internal/crossbranch/crossbranch_test.go       |  30 ++-
 internal/gitx/gitx.go                          | 163 ++++++++++++-
 internal/gitx/gitx_test.go                     | 252 +++++++++++++++++++-
 internal/server/server.go                      |  15 +-
 internal/server/server_test.go                 |  17 +-
 internal/setup/templates/core.md               |   3 +-
 internal/setup/templates/skill.md              |  20 +-
 web/src/lib/api.ts                             |   6 +-
 web/src/lib/board-filter.test.ts               |  49 +++-
 web/src/lib/board-filter.ts                    |  32 ++-
 web/src/lib/board-order.test.ts                | 140 ++++++++++-
 web/src/lib/board-order.ts                     |  60 ++++-
 web/src/lib/components/CommandPalette.svelte   |   8 +-
 web/src/lib/components/TicketPeekSheet.svelte  |  73 ++++--
 web/src/lib/components/TicketPeekSheet.test.ts |  99 ++++++++
 web/src/lib/ticket-editor.test.ts              |  35 ++-
 web/src/lib/ticket-editor.ts                   |  25 +-
 web/src/lib/workspace.svelte.test.ts           | 310 +++++++++++++++++++++++++
 web/src/lib/workspace.svelte.ts                | 130 +++++++++--
 web/src/routes/+layout.svelte                  |   3 +-
 web/src/routes/board/+page.svelte              |  88 +++++--
 web/src/routes/tickets/[id]/+page.svelte       |  61 ++++-
 33 files changed, 1912 insertions(+), 129 deletions(-)
```

## Verification

Ran by `pine close --evidence` on 2026-11-18 — **passed** (exit 0, 8ms).

Command:

```
! grep -rn 'pine inject' internal/setup/templates README.md && grep -q 'Committing ticket changes' README.md
```

_(no output)_
