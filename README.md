# 🌲 Pine

**A git-native, local-first workspace for AI-assisted development.**

[![CI](https://github.com/underworld14/pine/actions/workflows/ci.yml/badge.svg)](https://github.com/underworld14/pine/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/underworld14/pine?sort=semver)](https://github.com/underworld14/pine/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/underworld14/pine)](https://goreportcard.com/report/github.com/underworld14/pine)

Pine keeps your bugs, features, epics, and project context as plain files inside
your repository, so humans and AI agents (Claude Code, Codex, Gemini CLI, …) work
from the same source of truth. No cloud, no accounts, no database — the repo is
the database and git is the history.

A single binary gives you three surfaces over the same `.pine/` folder:

- a **Beads-style CLI** for tickets, dependencies, and epics from the terminal;
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

### Build from source

Requires **Go 1.24+** and **Node 20+**.

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

pine list --blocked            # tickets waiting on dependencies (🔒)
pine ready                     # actionable work: open and unblocked, most urgent first
pine dep tree BUG-001          # dependency tree
pine close FEAT-001            # → BUG-001 becomes ready
pine show EPIC-001             # epic with child progress (1/2 done)
pine update BUG-001 --status doing
```

Dependency cycles are refused at write time. A ticket is **blocked** while any of
its `deps` is not `done`, and **ready** otherwise — computed from the files, never
stored, so agents editing files can't desync it.

### AI context

```sh
pine context | pbcopy          # a full project briefing for your agent
pine prompt BUG-001            # a fix-request prompt for one ticket
pine export --format md        # all tickets as markdown (or --format json)
```

`pine context` includes a **Conventions** block that teaches the agent how to
write back to `.pine/` (edit `status` to move a ticket, use `deps`/`parent`, run
`pine ready`/`pine close`).

---

## How it stores data

Everything lives in `.pine/` and is meant to be committed:

```
.pine/
  config.json           # project settings, ticket types, priorities, optimizer
  board.json            # kanban columns (statuses only — never ticket ids)
  tickets/
    BUG-001.md          # YAML frontmatter + markdown body
  attachments/
    BUG-001/login.webp  # optimized on ingest
  templates/            # bug.md, feature.md, epic.md
  prompts/fix.md        # the pine prompt template
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
created: 2026-07-04T10:12:00Z
updated: 2026-07-04T11:00:00Z
---

# Description
...
```

The **filename is the canonical id**; frontmatter `status` decides which board
column a ticket is in. Pine parses leniently — a malformed or agent-written file
is surfaced as a read-only "degraded" ticket rather than lost, and `pine doctor`
reports every problem (schema errors, dangling deps, dependency cycles, broken
attachment references, orphaned directories, stray files).

---

## Pine & git branches

Because `.pine/` is committed alongside your code, **tickets are versioned with
your branches** — exactly like source files. Switching branches changes which
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

> For contrast, [Beads](https://github.com/gastownhall/beads) keeps issues
> *global* across branches by storing them in a Dolt database on a separate git
> ref rather than as files on your branches — a different point in the design
> space (global + cell-level merge, but not plain, hand-editable files).

## Web UI

`pine serve` (or `pine open`) serves the UI on `http://127.0.0.1:3412`
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

Attachments are optimized on upload: images are EXIF-oriented, downscaled to
2000px, and re-encoded to lossy WebP (kept only if smaller); videos pass through
with an oversize warning. `pine optimize` back-fills files dropped in by hand.

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
