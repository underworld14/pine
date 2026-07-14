# Pine Agent Setup Design

**Date:** 2026-07-05  
**Updated:** 2026-07-14  
**Status:** Implemented (Beads-style agent recipes)

## Goal

Give coding agents (Claude Code, Gemini CLI, Codex, Cursor, …) persistent,
discoverable instructions for using Pine — similar to Beads' `bd setup`.

## Model

Wizard and CLI are **agent-centric**, not file-centric. Each recipe is one
agent bundle:

| Recipe (`pine setup …`) | Label | Instructions | Skill | Hook |
|-------------------------|-------|--------------|-------|------|
| `agents` | Codex | `AGENTS.md` | `.agents/skills/pine/SKILL.md` | Codex Stop |
| `claude` | Claude Code | `CLAUDE.md` | `.claude/skills/pine/SKILL.md` | Claude Stop |
| `gemini` | Gemini CLI | `GEMINI.md` | shared `.agents/skills/pine/` | — |
| `cursor` | Cursor | `AGENTS.md` (shared marker `recipe=agents`) | shared `.agents/skills/pine/` | Cursor sessionStart |

Cursor shares `AGENTS.md` + skill with Codex via `SectionRecipe`. Removing
`cursor` removes only Cursor hooks; `pine setup agents --remove` or
`pine setup --remove` clears shared files.

## Wizard

```
Pine agent setup — choose agents to integrate:
  ✓ Codex          (AGENTS.md + skill + Stop hook)
    Claude Code    (CLAUDE.md + skill + Stop hook)
    Gemini CLI     (GEMINI.md + shared skill)
    Cursor         (AGENTS.md + skill + sessionStart hook)
```

Default selection: Codex. `-y` installs all agents.

## CLI

| Command | Behavior |
|---------|----------|
| `pine init` | Create `.pine/` and run agent wizard (unless `--skip-agents`) |
| `pine setup agent` | Interactive wizard (late setup) |
| `pine setup agents\|claude\|gemini\|cursor` | Install one agent bundle |
| `pine setup --check\|--remove\|--list\|--print` | Operate on all recipes |

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
- `internal/setup/hooks_test.go` — Codex/Cursor hooks; Cursor installs shared AGENTS.md
- `internal/cli/cli_test.go` — `setup agents`, `setup agent -y`
