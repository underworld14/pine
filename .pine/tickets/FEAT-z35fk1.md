---
id: FEAT-z35fk1
title: Ticket-level agent skills (skills frontmatter + prompt injection)
status: done
priority: medium
labels:
    - agents
verify: go test ./...
created: "2026-10-03T02:24:55Z"
updated: "2026-10-03T02:38:18Z"
---

# Description

Tickets can declare agent skill names that the executing agent must load before starting work, so the user never has to invoke skills manually. The list is a plain frontmatter field settable at `pine create` and updatable on demand after creation (`pine update --add-skill/--rm-skill`, web PATCH). Every agent-facing surface renders it: `pine prompt` (Required Skills block), `pine context` compact + full, `pine show`, JSON DTO, HTTP API.

Fallback when a required skill is unavailable in the harness (user decision 2026-10-03): (1) load it, (2) if missing, try to find/install it via skills.sh / GitHub / `/find-skills` when the harness has it, (3) if still unavailable, record that in the ticket's `# Notes` and proceed carefully. Never deadlocks the autonomous flow.

# Acceptance Criteria

- [x] `skills` is a known frontmatter key: parsed leniently (warnings, never errors), serialized canonically after `verify` / before `links`, omitted when empty, round-trips byte-stably
- [x] `pine create --skill` (repeatable/comma) and `pine update --add-skill/--rm-skill` (dedupe, order-preserving) work, including on long-existing tickets
- [x] `pine show` (text + --json), list/ready --json, HTTP create/patch all carry `skills`
- [x] `pine prompt` renders a Required Skills block with the fallback chain in both default templates, AND appends it for stale user templates that predate `.Skills`
- [x] `pine context` compact notes skills under Ready to Work only when present (zero cost otherwise); full context detail block shows `Skills:`; conventions document the key
- [x] Cross-branch merge treats skills as a base-aware set (union, deletion honored)
- [x] Search index exposes skills; README + skill.md templates (and installed copies) document the field
- [x] `go test ./...` green, coverage ≥ 90%

# Implementation Plan

1. Domain: `Ticket.Skills` + `case "skills"` parse + serialize + `mergeSet` (internal/ticket/)
2. Store: `CreateReq.Skills`, `cloneTicket` deep copy
3. View DTO + server createBody/ticketPatch
4. CLI flags create/update + show detail line + generic `applySetEdits`
5. contextgen: PromptData.Skills, default template blocks, `templateReferencesSkills`/`appendSkillsBlock` back-compat, compact ready note, full detail line, conventions key list
6. Search index `skills` key
7. Templates/docs: skill.md + .agents/.claude installed copies, README, babysitter spec note (per-ticket skills vs p5 layer)

# Notes

Implementation summary (2026-10-03):

- `skills` frontmatter key: parse + canonical serialize (after `verify`, before `links`, omitted when empty), lenient warnings, `mergeSet` union in Merge3, `ticket.DedupeNonEmpty` shared helper for write-time dedupe.
- CLI: `pine create --skill` (repeatable/comma, deduped in store.Create so the HTTP API benefits too), `pine update --add-skill/--rm-skill` via generalized `applySetEdits` (labels and skills share it), `pine show` meta line.
- Agent surfaces: `pine prompt` renders `## Required Skills` in both default templates AND appends it for stale user templates (templateReferences/appendSkillsBlock, mirroring the learnings pattern — verified live against a repo initialized pre-field); `pine context` compact adds a per-ticket note under Ready to Work only when skills exist; full context adds a `Required skills:` meta line; conventions document the key. Missing-skill fallback: find/install via skills.sh/GitHub//find-skills, else note in `# Notes` and proceed (user decision — never deadlocks).
- Parity: view DTO `skills` (always-present array like labels/deps — omitempty broke empty-state round-trips through show --json), HTTP create + PATCH, cloneTicket deep copy, Bleve search index keyword field.
- Docs: README ticket-format section, skill.md template + regenerated .agents/.claude installed copies, babysitter spec note (per-ticket skills compose with the phase-5 project layer: union, dedupe, overlap rule holds).
- Tests: round-trip + scalar-wrapped parse, merge-set, CLI lifecycle (create/add/rm/omit/show), prompt default+stale+custom-template, compact note, full-context line, HTTP create/patch/clear. go test ./... green, -race green on changed packages, make cover 91.0%.

# Related Files

- internal/ticket/ticket.go — Skills field
- internal/ticket/frontmatter.go — parse/serialize
- internal/ticket/merge.go — mergeSet
- internal/cli/tickets.go — flags + show detail
- internal/contextgen/ — prompt.go, compact.go, context.go
- internal/view/view.go — JSON DTO
- internal/server/tickets.go — API parity
- internal/search/index.go — search key
- internal/setup/templates/skill.md — agent guidance
- README.md — ticket format docs

# Attachments

## Work Evidence

Closed by `pine close --evidence` on 2026-10-03.

- Base: `149d0a15` (last commit at or before ticket created 2026-10-03)
- Files changed (base → working tree):

```
 .agents/skills/pine/SKILL.md                       |  6 +-
 .claude/skills/pine/SKILL.md                       |  6 +-
 .pine/tickets/EPIC-23747d.md                       |  9 +-
 .pine/tickets/FEAT-8e60tp.md                       |  6 +-
 .pine/tickets/FEAT-b6q2ht.md                       |  6 +-
 .pine/tickets/FEAT-ctn9gk.md                       |  6 +-
 .pine/tickets/FEAT-ndg5rr.md                       |  6 +-
 .pine/tickets/FEAT-yge4kt.md                       | 12 ++-
 AGENTS.md                                          |  6 +-
 CLAUDE.md                                          |  6 +-
 GEMINI.md                                          |  6 +-
 README.md                                          | 16 +++-
 ...026-08-09-agent-assignment-babysitter-design.md | 97 +++++++++++++++++++---
 internal/cli/cli_test.go                           | 46 ++++++++++
 internal/cli/tickets.go                            | 51 +++++++-----
 internal/contextgen/compact.go                     |  7 ++
 internal/contextgen/context.go                     |  6 +-
 internal/contextgen/contextgen_test.go             | 95 +++++++++++++++++++++
 internal/contextgen/prompt.go                      | 61 ++++++++++++--
 internal/search/index.go                           |  2 +
 internal/server/crossbranch.go                     |  6 +-
 internal/server/search.go                          |  1 +
 internal/server/server.go                          | 18 ++--
 internal/server/tickets.go                         |  6 ++
 internal/server/tickets_test.go                    | 20 +++++
 internal/setup/templates/skill.md                  |  6 +-
 internal/store/create.go                           | 11 ++-
 internal/store/store.go                            |  1 +
 internal/ticket/frontmatter.go                     |  9 +-
 internal/ticket/frontmatter_test.go                | 56 +++++++++++++
 internal/ticket/merge.go                           |  5 +-
 internal/ticket/merge_test.go                      | 13 +++
 internal/ticket/ticket.go                          | 17 ++++
 internal/view/view.go                              |  3 +
 34 files changed, 531 insertions(+), 97 deletions(-)
```

## Verification

Ran by `pine close --evidence` on 2026-10-03 — **passed** (exit 0, 8.533s).

Command:

```
go test ./...
```

Output:

```
ok  	github.com/underworld14/pine/cmd/pine	(cached)
ok  	github.com/underworld14/pine/internal/attach	(cached)
ok  	github.com/underworld14/pine/internal/cli	6.757s
ok  	github.com/underworld14/pine/internal/config	(cached)
ok  	github.com/underworld14/pine/internal/contextgen	(cached)
ok  	github.com/underworld14/pine/internal/crossbranch	(cached)
ok  	github.com/underworld14/pine/internal/doctor	(cached)
ok  	github.com/underworld14/pine/internal/frontmatter	(cached)
ok  	github.com/underworld14/pine/internal/gitx	1.941s
ok  	github.com/underworld14/pine/internal/learning	(cached)
ok  	github.com/underworld14/pine/internal/links	(cached)
ok  	github.com/underworld14/pine/internal/memory	(cached)
ok  	github.com/underworld14/pine/internal/search	(cached)
ok  	github.com/underworld14/pine/internal/selfupdate	(cached)
ok  	github.com/underworld14/pine/internal/server	6.659s
ok  	github.com/underworld14/pine/internal/setup	(cached)
ok  	github.com/underworld14/pine/internal/store	(cached)
ok  	github.com/underworld14/pine/internal/syncignore	(cached)
ok  	github.com/underworld14/pine/internal/ticket	(cached)
ok  	github.com/underworld14/pine/internal/tui	(cached)
ok  	github.com/underworld14/pine/internal/view	(cached)
ok  	github.com/underworld14/pine/internal/watch	(cached)
ok  	github.com/underworld14/pine/internal/workspace	(cached)
ok  	github.com/underworld14/pine/web	(cached)
```
