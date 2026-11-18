# 🌲 Pine

**A git-native, local-first workspace for AI-assisted development.**

[![CI](https://github.com/underworld14/pine/actions/workflows/ci.yml/badge.svg)](https://github.com/underworld14/pine/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/underworld14/pine?sort=semver)](https://github.com/underworld14/pine/releases)
[![Coverage](https://img.shields.io/badge/coverage-%E2%89%A590%25-brightgreen)](https://github.com/underworld14/pine/actions/workflows/ci.yml)
[![Codecov](https://codecov.io/gh/underworld14/pine/branch/main/graph/badge.svg)](https://codecov.io/gh/underworld14/pine)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/underworld14/pine)](https://goreportcard.com/report/github.com/underworld14/pine)

Pine keeps your bugs, features, epics, project context, and persistent learnings
as plain files inside your repository, so humans and AI agents (Claude Code,
Codex, Gemini CLI, …) work from the same source of truth. No cloud, no accounts,
no database — the repo is the database and git is the history.

A single binary gives you three surfaces over the same `.pine/` folder:

- a **Beads-style CLI** for tickets, dependencies, epics, and learnings from the terminal;
- a **beautiful local web UI** (kanban board, markdown editor, attachments, search);
- **AI context/prompt generation** so an agent understands the project instantly.

When an agent edits a ticket file on disk, the board updates live in your browser.
When you drag a card, the file changes on disk. It is the same data, always.

---

## Installation

### Download a release (recommended)

Grab the binary for your platform from the
[Releases page](https://github.com/underworld14/pine/releases). Each archive is a
single self-contained `pine` binary with the web UI built in — no runtime
dependencies.

macOS / Linux:

```sh
# pick the asset for your OS/arch, e.g. pine_0.1.0_darwin_arm64.tar.gz
tar -xzf pine_*_*.tar.gz
sudo mv pine /usr/local/bin/
pine --version
```

Windows: download the `_windows_amd64.zip`, extract `pine.exe`, and add it to
your PATH.

### With Go

```sh
go install github.com/underworld14/pine/cmd/pine@latest
```

This gives you the full CLI, HTTP API, and live sync. (The bundled web UI ships
in the release binaries and `make build` only; a `go install` build serves a
small placeholder page in place of the UI.)

### Upgrade in place

Once `pine` is on your `PATH` (release download or `go install`), update to the
latest GitHub release binary (full embedded UI):

```sh
pine upgrade --check   # current vs latest; exit 1 if an update is available
pine upgrade           # download, verify checksums, replace this binary
pine upgrade --force   # reinstall latest even if versions already match
```

`pine upgrade` always installs the official release archive over the current
executable, so a `go install` build is upgraded to the release binary. Optional
`GITHUB_TOKEN` / `GH_TOKEN` raises GitHub API rate limits. After upgrading,
run a new `pine` process to pick up the new binary (on Windows a
`pine.exe.old` leftover may remain until the upgrading process exits).

(`pine update` is unrelated — it edits ticket fields.)

### Build from source

Requires **Go 1.26+** and **Node 20+**.

```sh
git clone https://github.com/underworld14/pine
cd pine
make build            # builds the SvelteKit UI and embeds it into ./pine
./pine --version
```

Backend-only (no Node required) — serves a dev placeholder for the UI:

```sh
make build-dev
```

---

## Quick start

```sh
cd your-repo
pine init                               # create .pine/
pine create --type bug --title "Login button dead" -p high -l login,ui
pine open                               # launch the web UI (localhost:3412)
```

### The terminal workflow (Beads-style)

Everything works without leaving the shell; every read command takes `--json`
for agents.

```sh
pine create --type epic --title "Auth system"
pine create --type feature --title "Login form" --parent EPIC-001 -p high
pine create --type bug --title "Button dead" --parent EPIC-001 --deps FEAT-001

pine list                      # open tickets as epic → children tree (Beads-style)
pine list --all                # include done tickets
pine list --blocked            # tickets waiting on dependencies (🔒)
pine ready                     # actionable work as epic → children tree (open + unblocked)
pine dep tree BUG-001          # dependency tree
pine close FEAT-001            # → BUG-001 becomes ready
pine show EPIC-001             # epic with child progress (1/2 done)
pine update BUG-001 --status doing
pine log BUG-001               # commits that mention or touch this ticket
pine create --type bug --title "Fix login" --status testing --commit HEAD   # file committed work with exact evidence
pine release-notes --since v1.2.0   # Markdown notes from tickets closed since a tag
pine changed --json            # what changed under .pine/ (snapshot it in a gate)
pine doctor --fix              # health check; auto-repair the mechanical issues
pine import github             # pull open GitHub issues in as tickets (via gh)
pine import beads              # pull Beads issues in as tickets (via bd export)
```

Dependency cycles are refused at write time. A ticket is **blocked** while any of
its `deps` is not `done`, and **ready** otherwise — computed from the files, never
stored, so agents editing files can't desync it.

### AI context

```sh
pine context | pbcopy          # a compact project briefing for your agent (default)
pine context --full            # the exhaustive briefing (every table, ACs, commits)
pine prompt BUG-001            # a fix-request prompt (bugs) or implement prompt (features)
pine export --format md        # all tickets as markdown (or --format json)
```

`pine context` is **compact by default** — board counts, ready work, the
tickets whose *Related Files* intersect your uncommitted changes (or whose ID a
recent commit mentions), memory, and conventions — so dozens of done tickets no
longer flood the agent's context window before work starts. `--full` restores
the exhaustive layout.

`pine context` includes a **Conventions** block that teaches the agent how to
write back to `.pine/` (edit `status` to move a ticket, use `deps`/`parent`, run
`pine ready`/`pine close`). It also surfaces **Your Preferences (global)**
(`~/.pine/MEMORY.md`, see [Global memory](#global-memory)), **Project Memory**
(`.pine/MEMORY.md`), ranked **memory topics** under `.pine/memory/`, and
tip-resolved **Relevant Learnings** (`LRN-*`, usually ticket-scoped). Memory
bullets older than 90 days are marked `⚠ unverified since <date>` so an agent
knows which lines predate recent refactors and should be re-probed.

```sh
pine learn suggest "prefer query builder" --cites internal/store/query.go
pine learn "Always use the query builder" --to MEMORY.md
pine learn "Usage icons need text-white" --to memory/analytics.md --cites apps/web/.../x.ts
pine learn "Fixed only here" --scope ticket --ticket BUG-014
pine learn "gateway honors toolsets" --new-topic agent-api --evidence "probe: curl /api/toolsets"
pine learn list                    # MEMORY/topics + LRN tips
pine learn search "migration"
pine learn show MEMORY.md
pine learn show memory/analytics.md
pine learn supersede LRN-001 "..." # LRN replacement only
pine learn rm LRN-002
```

Durable insights go into **MEMORY.md** (prefs / project-wide rules) or
**memory/<topic>.md** (append to an existing topic when relevant). Pine
`learn suggest` ranks destinations; a confident match auto-appends. Use
`--scope ticket` only for ephemeral ticket notes (`LRN-*`). Do not create a
learning for routine ticket completion. Existing global `LRN-*` files still
work; `--legacy-lrn` creates one deliberately. Citation-stale LRNs stay hidden
from list/search/context by default (`--include-stale` to audit).

### Global memory

Everything above is scoped to one repository. Preferences that follow *you* —
your tools, your style, your habits — belong in the machine-wide store at
`~/.pine/`, so they survive switching repo **and** switching agent:

```sh
pine learn -g "I use pnpm, never npm"           # → ~/.pine/MEMORY.md
pine learn -g "Always squash before merging" --new-topic git-habits
pine learn list -g                              # inspect it
pine learn search -g pnpm
pine learn show -g MEMORY.md
```

`-g` works **outside** a pine repo — no `.pine`, no git required — because a
personal preference isn't about any one project. It is the only thing that ever
creates `~/.pine`; every read (`pine context`, `list -g`, `doctor`) leaves a
missing store missing.

| | Project memory | Global memory |
|---|---|---|
| Path | `.pine/MEMORY.md`, `.pine/memory/<topic>.md` | `~/.pine/MEMORY.md`, `~/.pine/memory/<topic>.md` |
| Command | `pine learn "…"` | `pine learn -g "…"` |
| Scope | this repository | you, everywhere |
| Committed | yes, with your code | no — it's yours |

**Precedence.** `pine context` renders global preferences *above* Project
Memory, with a fixed line stating the project wins on conflict. Nothing is
merged mechanically: the agent reads both and the nearer, more specific rule
takes priority.

**MEMORY.md vs topics.** `-g` appends to `~/.pine/MEMORY.md` directly rather
than suggesting a topic — unlike the project store, which ranks destinations.
That is deliberate: only global `MEMORY.md` is injected into context (capped at
2 KB), while global topics are listed *by name* for the agent to read on demand.
A fact auto-filed into a topic would be a fact the agent stops seeing. Use
`--new-topic` / `--to` when you want that trade knowingly. `pine doctor` warns
when `~/.pine/MEMORY.md` outgrows the cap.

**Relocating and syncing.** Set `PINE_HOME` to move the store (Windows defaults
to `%USERPROFILE%\.pine`). Pine does not sync it for you — add `~/.pine` to your
dotfiles if you want it on every machine.

**Opting out.** In a shared repo where your personal preferences shouldn't be
injected, add to `.pine/config.json`:

```json
{ "context": { "globalMemory": false } }
```

This also applies to `pine serve`'s web UI, which renders context through the
same generator.

> `--cites` paths are repo-relative, so a citation captured with `-g` will
> dangle in every other repo. Prefer plain statements in global memory.

### Agent setup

`pine init` runs an interactive checklist (↑/↓, space to toggle, enter to confirm)
to install instructions for your coding agent (Claude Code, Gemini CLI, Codex,
Factory, …). Forgot to set it up?

```sh
pine setup agent        # interactive wizard (pick agents)
pine setup agent -y     # install all agents (Codex, Claude, Gemini, Cursor)
pine setup agents       # Codex: AGENTS.md + skill + Stop hook
pine setup claude       # Claude Code: CLAUDE.md + skill + Stop hook
pine setup gemini       # Gemini CLI: GEMINI.md + shared skill
pine setup cursor       # Cursor: AGENTS.md + skill + sessionStart hook
pine setup --check      # verify sections are current
pine setup --remove     # strip pine sections
```

Use `pine init --skip-agents` to skip the wizard (e.g. in CI). Each root file
(`AGENTS.md`, `CLAUDE.md`, `GEMINI.md`) gets a short marked
`<!-- pine:begin ... -->` summary (always-on rules + pointer to the skill).
The full workflow lives in the **pine skill**
(`.claude/skills/pine/SKILL.md` for Claude Code;
`.agents/skills/pine/SKILL.md` for Codex / Cursor / Gemini / generic agents).
Cursor reads `AGENTS.md` natively — `pine setup cursor` installs that file,
the shared skill, and Cursor hooks.
Re-run `pine setup agent` after upgrading Pine to refresh stale sections.

Setup also installs learn-reminder hooks where the agent supports them:

- **Claude Code** — `Stop` hook in `.claude/settings.json`
- **Codex** — `Stop` hook in `.codex/hooks.json` (with `pine setup agents`)
- **Cursor** — `sessionStart` hook in `.cursor/hooks.json` (soft
  `additional_context`; no auto-continue)

Skills and hooks are idempotent and removed by `pine setup --remove`.

---

## How it stores data

Everything lives in `.pine/` and is meant to be committed:

```
.pine/
  config.json           # project settings, ticket types, priorities, optimizer
  board.json            # kanban columns (statuses only — never ticket ids)
  MEMORY.md             # project preferences and standing rules
  memory/               # topic files (append durable domain insights)
  tickets/
    BUG-001.md          # YAML frontmatter + markdown body
  learnings/
    LRN-001.md          # ticket-scoped / legacy one-shot insights
  attachments/
    BUG-001/login.webp  # optimized on ingest
  templates/            # bug.md, feature.md, epic.md
  prompts/              # fix.md (bugs), implement.md (features)
  .gitignore            # optional: keep tickets/attachments local (see sync)
```

By default **tickets** are tracked (branch-scoped with your code) and
**attachments** stay local via `.pine/.gitignore`. Project memory
(`MEMORY.md` / `memory/`) is always committed so it follows you across machines.
Change this later with `pine setup sync`, or pass `--sync-tickets` /
`--sync-attachments` (and `--no-*`) to `pine init`.

Your personal memory lives outside any repo, in the machine-wide store — same
layout, never committed with a project (see [Global memory](#global-memory)):

```
~/.pine/                # or $PINE_HOME; %USERPROFILE%\.pine on Windows
  MEMORY.md             # your preferences, in every repo
  memory/               # your topic files (read on demand by agents)
```

A ticket file:

```markdown
---
id: BUG-001
title: Login button not working
status: testing
priority: high
labels:
  - login
  - ui
deps:
  - FEAT-002
parent: EPIC-001
verify: grep -rn "renderLogin(" apps/web/src | grep -v _test
skills:
  - impeccable
  - find-docs
created: 2026-07-04T10:12:00Z
updated: 2026-07-04T11:00:00Z
---

# Description
...

# Related Files
- apps/web/src/login/button.tsx — the dead onClick handler
- apps/web/src/login/ — whole folder needs guard checks
```

The **filename is the canonical id**; frontmatter `status` decides which board
column a ticket is in. Pine parses leniently — a malformed or agent-written file
is surfaced as a read-only "degraded" ticket rather than lost, and `pine doctor`
reports every problem (schema errors, dangling deps, dependency cycles, broken
attachment references, orphaned directories, stray files, dangling `supersedes`
refs, supersede cycles, and memory bullets that are undated or older than
90 days).

**Body sections.** Sections are `#`-level headings; the two Pine reads are
`# Acceptance Criteria` (checkboxes drive progress counters) and
`# Related Files`. A Related Files entry is a bullet whose first token is a
repo-relative path — directories (with or without trailing `/`), backticks,
and a leading `@` all work, and anything after the path is a free-form
annotation that survives extraction. `pine context` uses these paths to decide
which tickets are relevant to your current changes, and `pine prompt` lists
them for the agent.

**verify.** `verify:` in the frontmatter is a shell command proving the work is
live in production code (a `grep` for callers, a smoke test). `pine close <ID>
--evidence` runs it in the repo root before closing: pass → the ticket closes
with a `## Verification` section; fail → the close is refused (pass `--force`
to close anyway and record the failure). Set it with `pine create --verify` /
`pine update --verify`, or let `pine prompt`'s implement template ask the agent
to fill one in.

**skills.** `skills:` in the frontmatter names agent skills (open SKILL.md
ecosystem names — `impeccable`, `find-docs`, …) the executing agent must load
before starting the ticket, so you never have to invoke them manually. Every
agent-facing surface restates them: `pine prompt <ID>` renders a **Required
Skills** block (with the fallback: find/install the skill via skills.sh or
GitHub, and if it stays unavailable, note that in the ticket's `# Notes` and
proceed carefully), `pine context` flags them under Ready to Work, and
`pine show` lists them. Set or change them at any time — including on tickets
created long ago — with `pine create --skill <name>` and
`pine update <ID> --add-skill <name> / --rm-skill <name>`.

**JSON.** `pine list|show|ready|create --json` emit the same ticket DTO the
HTTP API uses: `id`, `type`, `title`, `status`, `priority`, `labels`, `deps`,
`parent`, `phase`, `verify`, `skills`, `links`, `created`, `updated`, computed
`blocked`/`unmet`/`dangling`/`inCycle`, `epicProgress`, `acceptance`, `hash`,
and (for `show`/`create`) the full `body`.

A learning file:

```markdown
---
id: LRN-001
scope: global
tags:
  - db
source_agent: manual
created: 2026-07-11T10:00:00Z
---

Always use the query builder — raw SQL caused schema drift (see BUG-014).
```

The **filename is the canonical id**. Optional frontmatter `supersedes: LRN-xxx`
marks a replacement; `superseded_by` is derived at read time and never stored.

---

## Pine & git branches

Because `.pine/` is committed alongside your code, **tickets are versioned with
your branches** — exactly like source files (unless you opt out with
`pine setup sync` / `--no-sync-tickets`). Switching branches changes which
tickets you see: a ticket created and committed on `dev` won't appear while
you're on `main` (it's not lost — it returns on `dev`, or when the branches
merge). Uncommitted new tickets stay visible across branches, since git leaves
untracked files alone.

This is a deliberate trade-off of the "everything is files" model. If you prefer a
single global backlog, keep `.pine/` mastered on your mainline (create/close
tickets there and let them flow to feature branches via merge), or run Pine
against a git worktree pinned to one branch.

**Merge-safe IDs.** New tickets get random, collision-resistant IDs like
`BUG-7f3k2a` by default (`"idStyle": "hash"` in `config.json`), so two branches —
or two AI agents — never mint the same ID. Prefer the classic sequential
`BUG-001`? Set `"idStyle": "sequential"`; just note that concurrent branches can
then choose the same number and clash on merge (`pine doctor` flags duplicates).

**Field-level merges.** Run `pine setup merge` once per clone to register Pine's
git merge driver for `.pine/tickets/*.md`. When two branches edit the same
ticket, git then merges it field by field — one side's `status` change and the
other's new label combine cleanly instead of producing a raw YAML conflict.
Genuine divergences (both sides rewrite the body, or change the same scalar) are
still surfaced for review. The `.gitattributes` rule is committed and shared;
the git config is local, so each teammate runs `pine setup merge` after cloning
(`pine doctor` reminds anyone who hasn't).

**Commit history.** `pine log <ID>` lists the commits that mention a ticket or
touched its file — a quick audit trail linking tickets to the code that changed
for them.

**Committing ticket changes.** Pine never runs `git commit`; tickets are plain
files, so how they enter history is up to you. Stage `.pine/tickets/<ID>.md` in
the code commit it describes. For status-only moves (a hand-off, a review
verdict) write the verdict as a section in the ticket body and fold it into the
next code commit, or make one `pine:` commit per session — a commit per status
move buries the real history.
The review record survives squashing because it lives in the ticket, so note
the review base SHA in the hand-off section instead of deriving it from
per-status commits.

**Work evidence.** `pine close <ID> --evidence` appends the commits that cite the
ticket and a `git diff --stat` (capped at 50 files) from the ticket's creation to
the working tree, after running its `verify:` command. For work that is already
committed, `pine create … --commit <rev>` (repeatable) records exactly those
commits instead; `pine close --evidence` keeps such a section, and
`pine close --evidence --commit <rev>` re-records it.

**Release notes.** `pine release-notes --since <ref>` lists the tickets that
became done since a tag, branch, or commit — grouped by type (or
`--group-by label`), highest priority first, each linking the commits that cite
it. Commits that only touch `.pine/` (hand-offs, review verdicts) and merge
commits are skipped, so bookkeeping never reaches the notes. The output is
Markdown, ready for `gh release create v1.3.0 --notes-file -`; `--json` gives the
same data for scripts. It needs tickets tracked in git (`sync.tickets`).

**Gating agent edits.** A gate that must not fail on `.pine/` changes that
existed before the work started (an owner's untracked ticket, say) can compare
snapshots of `pine changed --json` instead of rejecting every `.pine/` change.
The output is sorted by path, has no timestamps, and carries each file's hash and
each ticket's status before and after:

```sh
pine changed --json > "$SNAP/pine.json"                 # before delegating
pine changed --json | diff -q - "$SNAP/pine.json"       # after: fails only on new edits
```

`--since <ref>` compares against any commit, tag, or branch instead of `HEAD`.

**Importing.** Already have a GitHub backlog? `pine import github` pulls open
issues in as tickets via your existing `gh` CLI auth (idempotent — re-running
skips anything already imported). Migrating from [Beads](https://github.com/gastownhall/beads)?
`pine import beads` runs `bd export` in the current project (or accepts a JSONL
file / stdin), maps issue types (bug/task/epic/chore/…), status, priority, epic
parents, and blocking deps, and records `beads: <id>` so re-runs are idempotent.

> For contrast, Beads keeps issues *global* across branches by storing them in a
> Dolt database on a separate git ref rather than as files on your branches — a
> different point in the design space (global + cell-level merge, but not plain,
> hand-editable files).

## Web UI

`pine serve` (or `pine open`) serves the UI on `http://127.0.0.1:3412`
and opens your default browser (`--open=false` to skip).
(localhost only, with Host/Origin checks — no auth, no external access).

- **Dashboard** — at-a-glance triage lists.
- **Board** — drag & drop kanban; blocked cards show 🔒; cards glide + flash when
  an agent changes a file on disk.
- **Ticket** — frontmatter controls, split markdown editor with a "changed on
  disk" conflict banner, dependency/epic chips, attachment grid + lightbox, and a
  one-click **Copy AI prompt**.
- **New issue** in ≤10s: press `c`, type a title, paste a screenshot (`⌘V`),
  `⌘↵`. Screenshots are downscaled and re-encoded to WebP on the way in.
- **Search** (`/`) and a **command palette** (`⌘K`).

Persistent learnings (`pine learn`) are a CLI/AI-context surface only — they
don't appear in the web UI, its board, or its search.

Attachments are optimized on upload: images are EXIF-oriented, downscaled to
2000px, and re-encoded to lossy WebP (kept only if smaller); videos pass through
with an oversize warning. `pine optimize` back-fills files dropped in by hand.

---

## VS Code extension

Prefer to stay in your editor? The **[Pine VS Code extension](editors/vscode/)**
opens the same board in a VS Code tab — no terminal, no manual `pine serve`. Run
**Pine: Open Board** and it starts Pine for you and embeds the UI; **Pine: Create
Bug / Create Feature** add tickets from the command palette. It reuses the exact
web UI, so live sync, attachments, and search all behave identically. See
[`editors/vscode/README.md`](editors/vscode/README.md) for details.

---

## Development

```sh
make test        # Go unit + integration tests
make test-web    # frontend (vitest)
make e2e         # Playwright end-to-end (requires: cd web && npx playwright install)
make lint        # go vet
```

## Tech

Go (cobra CLI, chi router, Bleve in-memory search, fsnotify watcher, SSE) with a
SvelteKit 2 / Svelte 5 / Tailwind v4 UI embedded via `go:embed`. WebP encoding is
pure-Go (no cgo), so the binary cross-compiles cleanly.

## Contributing

Contributions of all kinds are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md)
for local setup and how to run the test suite, and please open an issue for bugs
or feature ideas.

## License

[MIT](LICENSE) © underworld14
