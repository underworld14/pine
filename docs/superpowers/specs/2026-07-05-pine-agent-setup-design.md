# Pine Agent Setup Design

**Date:** 2026-07-05  
**Status:** Implemented (docs-only v1)

## Goal

Give coding agents (Claude Code, Gemini CLI, Codex, Factory, etc.) persistent,
discoverable instructions for using Pine — similar to Beads' `bd setup`, but
without hooks or a `pine prime` command in v1.

## Scope (v1)

- Agent wizard runs during `pine init` (skip with `--skip-agents`)
- Late setup via `pine setup agent`
- Three targets: `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`
- Interactive wizard with checkbox-style toggles
- Marked HTML-comment sections for idempotent install/update/remove/check
- Dynamic board-column line when `.pine/` is present
- No Cursor rules, Codex skill files, or session hooks

## Out of scope (v2)

- `pine prime` + SessionStart hooks
- `.cursor/rules/pine.mdc`, `.agents/skills/pine/SKILL.md`
- `pine init --skip-agents` / auto-install on `pine init`
- `--global` installs

## CLI

| Command | Behavior |
|---------|----------|
| `pine init` | Create `.pine/` and run agent wizard (unless `--skip-agents`) |
| `pine setup agent` | Interactive wizard (late setup) |
| `pine setup agents` | Install `AGENTS.md` only |
| `pine setup claude` | Install `CLAUDE.md` only |
| `pine setup gemini` | Install `GEMINI.md` only |

Flags: `--list`, `--check`, `--remove`, `--print` on `pine setup`; `-y` on `pine setup agent`.

## Marked sections

Sections are wrapped in HTML comments:

```markdown
<!-- pine:begin recipe=agents profile=full version=0.1.0 hash=<digest> -->
...
<!-- pine:end -->
```

- **Install:** merge or replace the marked block; preserve user content outside markers
- **Remove:** strip only the marked block; delete file if empty
- **Check:** compare recipe, pine version, content hash, and body text

## Content

Shared body from `internal/setup/templates/core.md` plus a short per-recipe header.
Includes workflow, essential CLI commands, write-back rules, and a pointer to
`pine context` for live state.

When the store is open, board column statuses from `board.json` are injected into
the write-back rules line.

## Package layout

- `internal/setup/` — recipes, merge engine, render, wizard
- `internal/cli/setup.go` — thin cobra wiring

## Testing

- `internal/setup/merge_test.go` — merge, remove, check, render
- `internal/cli/cli_test.go` — `setup agents`, `setup -y`
