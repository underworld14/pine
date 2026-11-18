---
id: BUG-fxvdav
title: Web board slow with hundreds of tickets (bodies in snapshot, uncapped evidence, uncapped columns)
status: done
priority: medium
labels:
    - agents
parent: EPIC-6j541d
verify: go test ./internal/server -run 'TestSnapshotBodiesQuery|TestSnapshotWithoutBodiesIsSmall|TestSnapshotKeepsOffBranchBodies|TestJSONIsGzipped|TestEventStreamIsNotGzipped' && (cd web && npm test --silent)
created: "2026-11-18T04:06:18Z"
updated: "2026-11-18T04:28:57Z"
---

# Description
The web board was very slow on a repo with 585 tickets. Cause: ticket files totalled 9.9 MB (7.8 MB of it uncapped `

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

Ran by `pine close --evidence` on 2026-11-18 — **passed** (exit 0, 6.154s).

Command:

```
go test ./internal/server -run 'TestSnapshotBodiesQuery|TestSnapshotWithoutBodiesIsSmall|TestSnapshotKeepsOffBranchBodies|TestJSONIsGzipped|TestEventStreamIsNotGzipped' && (cd web && npm test --silent)
```

Output:

```
ok  	github.com/underworld14/pine/internal/server	1.190s

 RUN  v3.2.7 /Users/kilas/projects/pine/web

11:28:54 AM [vite-plugin-svelte] src/routes/tickets/[id]/+page.svelte:569:10 noninteractive element cannot have nonnegative tabIndex value
https://svelte.dev/e/a11y_no_noninteractive_tabindex
11:28:54 AM [vite-plugin-svelte] src/routes/tickets/[id]/+page.svelte:605:14 `<video>` elements must have a `<track kind="captions">`
https://svelte.dev/e/a11y_media_has_caption
 ✓ src/lib/components/PrioritySeg.test.ts (3 tests) 24ms
 ✓ src/lib/components/CommandPalette.test.ts (2 tests) 31ms
 ✓ src/lib/workspace.svelte.test.ts (32 tests) 154ms
 ✓ src/lib/components/TicketRelations.test.ts (4 tests) 41ms
 ✓ src/lib/components/TicketGraph.test.ts (2 tests) 45ms
 ✓ src/lib/graph-force.test.ts (10 tests) 268ms
 ✓ src/routes/tickets/[id]/page.test.ts (3 tests) 86ms
 ✓ src/lib/components/TicketPeekSheet.test.ts (15 tests) 487ms
 ✓ src/lib/components/AddCardComposer.test.ts (3 tests) 73ms
 ✓ src/lib/components/QuickPeekPopover.test.ts (3 tests) 125ms
 ✓ src/lib/components/BoardFilterBar.test.ts (4 tests) 46ms
 ✓ src/lib/components/TicketCard.test.ts (3 tests) 124ms
 ✓ src/lib/markdown.test.ts (1 test) 28ms
 ✓ src/lib/board-order.test.ts (34 tests) 22ms
 ✓ src/lib/ticket-editor.test.ts (8 tests) 26ms
 ✓ src/routes/board/page.test.ts (10 tests) 1392ms
   ✓ board page > debounces typing in the filter but applies label/priority clicks at once  339ms
 ✓ src/lib/insert-at-cursor.test.ts (13 tests) 9ms
 ✓ src/lib/board-filter.test.ts (12 tests) 12ms
 ✓ src/lib/graph.test.ts (5 tests) 9ms
 ✓ src/lib/file-mention.test.ts (8 tests) 9ms
 ✓ src/lib/ui-helpers.test.ts (1 test) 7ms

 Test Files  21 passed (21)
      Tests  176 passed (176)
   Start at  11:28:53
   Duration  3.51s (transform 3.34s, setup 9.19s, collect 4.95s, tests 3.02s, environment 5.75s, prepare 1.45s)
```
