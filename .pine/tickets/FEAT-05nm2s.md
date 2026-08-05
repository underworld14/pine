---
id: FEAT-05nm2s
title: Graph links + multi-repo serve
status: done
priority: high
labels:
    - graph
    - memory
    - serve
created: "2026-08-05T08:08:42Z"
updated: "2026-08-05T16:15:00Z"
---

# Description

Typed `links` frontmatter connecting tickets, memory topics, and learnings with computed backlinks; dedicated `/graph` UI; multi-repo `pine serve` auto-discovers repos registered in `~/.pine/repos.json` (auto-populated by `pine init`) via StoreRegistry; agent workflow (plan→create / done→close-evidence) installed by `pine init`'s agent wizard.

# Acceptance Criteria
- [x] Ticket + memory topic `links` field parse/serialize; doctor flags dangling links
- [x] `internal/links` ref resolve + unified graph (deps/parent/links) + backlinks
- [x] `/api/graph` + `/graph` route with GraphView (filters, zoom/pan)
- [x] Ticket neighborhood shows memory link nodes
- [x] `pine init` auto-registers repo in `~/.pine/repos.json`; `pine serve` auto-discovers all registered repos (no `--workspace` flag)
- [x] `pine init --alias <name>` chooses the routing alias (collision hint on conflict); `pine serve --only` is read-only on the registry
- [x] `/api/repos`, `/api/r/{alias}/…`, repo-tagged SSE, web RepoSwitcher (shown when >1 repo registered)
- [x] All `/api/r/{alias}/…` mutations + reads route to the alias store (not the active store); SSE tagged with the request alias
- [x] `SetActiveRepo` is race-free vs pollers (`go test -race` green with concurrent-activate test)
- [x] Go + Vitest tests green for new packages
- [x] Deferred follow-ups done: I2 (cache `LinksGraph`), I4 (alias attachment route), M2 (per-repo git poller)
- [x] Agent workflow: `pine close <ID> --evidence` attaches file-change evidence; `pine init` installs the plan→create / done→close-evidence skill (no separate `pine inject` command)

# Implementation Plan

Implemented per plan Phase 1 then Phase 2.

# Notes

- Backlinks are computed, never stored. Cycles allowed on `links` (not on `deps`).
- Multi-repo is opt-out: `pine serve` opens every repo in `~/.pine/repos.json` (auto-populated by `pine init`); `--only` restricts to the cwd repo. No `pine workspace` command — registration is automatic.
- Single-repo feel is preserved when only one repo is registered (no switcher, legacy `/api` routes the single store).
- CLI mutations other than serve remain per-repo via `-C`.
- Code review (round 2) fixed: C1 (alias routing for all mutations/reads — was silently hitting the active store), C2 (SSE tagged with request alias), I3 (SetActiveRepo race guarded by storeMu/gitMu/crossMu/searchMu + a concurrent-activate race test), I5 (mutation-routing + SSE-tagging tests), I6 (serve skips the registry write when cwd already registered), M1 (dead livesync code), M3 (backlink title map built once).
- Deferred follow-ups (round 3) implemented:
  - I2: `store.LinksGraph()` is now cached on the Store (`linksGraph` + `linksMu`) and invalidated by every ticket/learning/config mutation and by a new `watch.KindMemory` (memory/ + MEMORY.md are now watched). The graph is built outside the cache lock to avoid a s.mu→linksMu ordering inversion with write paths. Repeated `/api/graph` and `pine prompt` calls are O(1) after the first build.
  - I4: added `/api/r/{alias}/attachments/{id}/{name}` serving from the alias store (`handleServeAttachment` now uses `storeOf(r)`); the legacy `/attachments/...` route still serves the active store (correct after a repo switch). Completes the alias-routing contract for external clients.
  - M2: the git poller now runs per registered repo (`startRepoGitPoller`), each emitting a repo-tagged `git.updated`; the active repo's poller also keeps `srv.gitStatus` fresh and kicks cross-branch. The web UI keeps a per-repo git cache (`gitByRepo`) so non-active repo git updates are retained and a repo switch is instant. Poll interval is injectable (`srv.gitPollInterval`) for a fast race test.
- Code review (round 4) + agent-workflow follow-ups:
  - I1: `LinksGraph` now uses a generation counter (`linksGen`) bumped on every `InvalidateLinksGraph`. A builder snapshots the generation before building and re-checks it under the cache write-lock; if it changed (a writer invalidated mid-build) the stale graph is discarded and rebuilt — so a graph built from just-invalidated inputs can never be cached and served. New `TestLinksGraphConcurrentMutationRace` hammers the reader under `-race`.
  - I2: extended `TestMultiRepoAttachmentAlias` to POST an attachment via `/api/r/web/tickets/{id}/attachments` and assert it is NOT visible under `/api/r/api/attachments/{id}/{name}` (404), then DELETE via the web alias route and assert 404 after — proving alias-routed upload/delete hit the right store.
  - M3: `pine serve --only` is now read-only on the registry — it returns `SingleRegistry(openStore())` without auto-registering the cwd (new `TestServeOnlyDoesNotWriteRegistry`).
  - `pine init --alias <name>` lets a repo pick its routing alias (default lowercase basename); on a collision it prints a hint to use `--alias` (new `TestInitAliasFlag`). Two repos with the same basename no longer silently collide.
  - Agent workflow (no `pine inject` command): the plan→create / done→close-evidence instructions live in the skill/core templates that `pine init`'s agent wizard installs. `pine close <ID> --evidence` marks done AND appends a `## Work Evidence` section (commits referencing/touching the ticket + a `git diff --stat` from the commit at its creation time to the working tree); re-running replaces the section. New `gitx.CommitBefore`/`DiffStat` + `cli/evidence.go`; tests `TestCloseEvidence`, `TestCloseEvidenceReRunReplacesSection`, `TestDiffStatAndCommitBefore`. Repo's installed CLAUDE/AGENTS/GEMINI/skill copies refreshed via `pine setup agent -y`.
- Code review (round 4) follow-ups:
  - M5: `startRepoGitPoller` now takes its baseline git snapshot synchronously (before the goroutine loop), so `StartLiveSync` returning guarantees every poller's `prev` is anchored to current HEAD. This removes the `TestPerRepoGitPollerEmitsRepoTagged` 250ms-sleep timing race (a commit right after serve start is always observed as a change, never captured as the baseline by a racing goroutine). Test is now faster (0.32s) and stable across repeats.
  - UX: the `pine serve` startup banner marks the active repo with `(active)` in multi-repo mode.

# Related Files
- internal/links/
- internal/workspace/
- internal/server/{registry,repos,graph,livesync,git,attachments}.go
- internal/watch/watcher.go
- internal/gitx/gitx.go (CommitBefore, DiffStat)
- internal/cli/{init,serve,evidence,tickets}.go
- internal/setup/templates/{skill,core}.md
- web/src/routes/graph/
- web/src/lib/components/{GraphView,RepoSwitcher,TicketGraph}.svelte
- web/src/lib/workspace.svelte.ts

# Attachments
