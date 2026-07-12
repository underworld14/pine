## Pine issue tracking

This repository uses [Pine](https://github.com/underworld14/pine) — git-native issue tracking in `.pine/` (tickets + learnings, branch-scoped, committed with your code).

### Always do

- Track work with **Pine tickets** — do **not** use markdown TODO lists for issue tracking.
- Start with `pine context`; pick work with `pine ready`.
- Write progress back to `.pine/tickets/<ID>.md` (or `pine update` / `pine close`). Move tickets by editing `status`{{BOARD_COLUMNS_LINE}}
- Capture durable insights with `pine learn "…"` into `.pine/MEMORY.md` or `.pine/memory/<topic>.md` (not a new LRN file per ticket). Use `--scope ticket` only for ephemeral ticket notes.

### Full workflow

When you need the complete Pine workflow (commands, write-back rules, learnings lifecycle), **load the pine skill**:

- Codex / Factory / Gemini / generic agents: `.agents/skills/pine/SKILL.md`
- Claude Code: `.claude/skills/pine/SKILL.md`

If no skill file is installed, use `pine context` and `pine --help`.
