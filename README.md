# 🌲 Pine

**A git-native, local-first workspace for AI-assisted development.**

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

## Install / build

Requires Go 1.24+ and Node 20+ (only to build the embedded UI).

```sh
make build      # builds the SvelteKit UI and embeds it into ./pine
./pine --help
```

For backend-only development the UI is optional:

```sh
make build-dev  # binary without the embedded UI (serves a dev placeholder)
make dev        # runs `pine serve --dev`, proxying the UI to `npm run dev`
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

## License

MIT
