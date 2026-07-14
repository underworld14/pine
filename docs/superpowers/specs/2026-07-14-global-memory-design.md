# Global memory: Pine as cross-harness developer memory

**Date:** 2026-07-14
**Status:** Approved for planning

## Problem

Solo developers who switch between coding harnesses (Claude Code, Codex,
Gemini CLI, Cursor, …) lose memory at every switch:

1. **Silo:** each harness keeps its own memory store, so project knowledge
   written in one harness is invisible to the next.
2. **Personal prefs don't travel:** preferences that apply to every repo
   ("I use pnpm", "conventional commits") live per-harness or per-repo.
3. **Capture is unreliable:** sessions end without insights being saved;
   the current generic Stop-hook reminder is not enough.

Pine already has project-level memory (`.pine/MEMORY.md` + ranked
`.pine/memory/<topic>.md`, injected into `pine context`). This design
extends it so Pine is the **one canonical memory** every harness reads
and writes.

## Decisions (from brainstorm)

- Pine is **canonical**; agents are steered to it via instruction files
  and the skill. No sync bridges to harness-native memory stores.
- Global memory lives in **`~/.pine/`**, auto-merged into `pine context`.
  Cross-machine sync is the user's job (dotfiles).
- Capture is enforced by **Claude-style memory discipline in SKILL.md**
  only — no blocking gates, no inbox/review queue.
- Ticket→harness assignment/dispatch is **out of scope** (separate
  follow-up).
- Layout approach: **extend the current format** (Approach A). No
  one-file-per-fact restructuring, no migration.

## Design

### Global store

- New user-level Pine home: `~/.pine/`, overridable via the `PINE_HOME`
  environment variable (used by tests; lets users relocate, e.g. XDG).
- Layout mirrors the project store: `MEMORY.md` + `memory/<slug>.md`.
- `internal/memory` already takes a `pineDir` parameter; the same package
  serves both stores. New code: global path resolution
  (`os.UserHomeDir()`-based) and a global-flavored seed for `MEMORY.md`
  ("personal preferences that apply to every repository", with the same
  Preferences / Conventions / Gotchas / Log sections).

### Context merge

`contextgen.FormatMemoryBlock` output order becomes:

1. `## Your Preferences (global)` — global `MEMORY.md`, capped at
   2048 bytes (`ContextGlobalCap`), prefixed with the fixed line:
   *"If anything here conflicts with Project Memory, Project Memory
   wins."*
2. `## Project Memory` — unchanged (3.5 KB cap).
3. `## Memory Topics` — unchanged ranked project topics.

Global topic files are **not inlined**. They are listed by name on one
line (`Global topics: pnpm, git-habits — read ~/.pine/memory/<slug>.md
if relevant`) so agents can pull them on demand.

Precedence is by instruction and ordering only; there is no mechanical
merge.

### Opt-out

`.pine/config.json` gains `context.global_memory` (default `true`).
When `false`, `pine context` omits the global block — for shared/team
repos where personal prefs should not be injected.

### CLI surface

- `pine learn -g/--global "…"` — appends to the global store. The
  existing suggest.go routing runs against the global dir, so `-g`
  insights auto-route between global `MEMORY.md` and global topics.
  Composes with `--topic`.
- `pine context` — picks up the global block automatically; no new flags.
- `pine doctor` — new check: global store exists/readable; warns when
  global `MEMORY.md` exceeds `ContextGlobalCap` (truncation warning).

No `pine memory` command family, no import/export, no sync.

### Capture discipline (SKILL.md)

New **Memory discipline** section in Pine's SKILL.md template, applied
on every harness because the skill file travels with `pine setup`:

- **Checkpoint rule:** after finishing a task (closing a ticket, fixing
  a bug), ask "what did I learn that outlives this session?" — if
  something, save it before moving on.
- **Check-before-write:** search `MEMORY.md` + topics first; update the
  existing line rather than appending a duplicate. Delete lines that
  turned out wrong.
- **One fact per bullet**, concrete and self-contained.
- **Routing rule:** about you/any repo → `pine learn -g`; about this
  repo → `pine learn`; ephemeral ticket note → `--scope ticket`.
- **Don't save** what the repo already records (code, git history,
  CLAUDE.md).

Instruction-file blocks (CLAUDE.md / AGENTS.md / GEMINI.md) each gain
one pointer line to this section. Stop/sessionStart hook reminder text
is updated to mention `-g`. `pine setup` re-renders idempotently on
existing installs.

### Error handling

- Missing global dir → `pine context` silently omits the block;
  `pine learn -g` creates the layout on first use (`EnsureLayout`).
- No resolvable home dir → clear error on `-g` only; `pine context`
  never breaks because of the global store.
- Windows: `os.UserHomeDir()` → `%USERPROFILE%\.pine`.

### Documentation

README gains a "Global memory" subsection: what goes where, the
precedence rule, the `context.global_memory` opt-out, and a note to add
`~/.pine` to dotfiles for cross-machine sync.

## Testing

Table-driven Go tests in the existing style:

- `PINE_HOME` resolution and default home-dir fallback.
- Global seed content on first `pine learn -g`.
- Context merge: section ordering, global cap truncation, topic
  name-listing line, omission when store is missing or opt-out is set.
- `-g` routing into the global store (MEMORY.md vs topic).
- Setup template snapshots (SKILL.md discipline section, instruction
  pointer lines, hook reminder text).
- Doctor check outputs.

Existing project-store tests must pass untouched — that code path does
not change.

## Out of scope

- Sync/bridges to harness-native memory stores (one-time import may be
  a future follow-up).
- Ticket→harness assignment or dispatch.
- One-file-per-fact memory restructuring.
- Pine-managed git syncing of `~/.pine/`.
