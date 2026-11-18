<!-- pine:begin recipe=claude profile=full version=0.1.0-dev hash=df8aa0e0752e3778 -->
Claude Code: read this before working in the repository.

## Pine issue tracking

This repository uses [Pine](https://github.com/underworld14/pine) — git-native issue tracking in `.pine/` (tickets + learnings, branch-scoped, committed with your code).

### Always do

- Track work with **Pine tickets** — do **not** use markdown TODO lists for issue tracking.
- Start with `pine context` (compact briefing by default; `--full` for everything); pick work with `pine ready`.
- Planning a non-trivial change? `pine create --type feature --title "…"` first, move it to `doing`, and when done `pine close <ID> --evidence` (runs the ticket's `verify:` proof command — refusing to close when it fails — then marks done and attaches the file-change evidence). Run `pine prompt <ID>` for a ready-made brief.
- Commit ticket edits with the code they describe — Pine never commits for you. Stage `.pine/tickets/<ID>.md` in that commit (or make one `pine:` commit per session) rather than one commit per status move.
- Write progress back to `.pine/tickets/<ID>.md` (or `pine update` / `pine close`). Move tickets by editing `status` (board columns: todo, doing, testing, done).
- Capture durable insights with `pine learn "…"` into `.pine/MEMORY.md` or `.pine/memory/<topic>.md` (not a new LRN file per ticket). Use `--scope ticket` only for ephemeral ticket notes.
- Preferences that apply in **every** repo (your tools, style, habits) belong in your machine-wide memory: `pine learn -g "…"` → `~/.pine/`. Project memory wins on conflict.

### Full workflow

When you need the complete Pine workflow (commands, write-back rules, learnings lifecycle), **load the pine skill**:

- Codex / Factory / Gemini / generic agents: `.agents/skills/pine/SKILL.md`
- Claude Code: `.claude/skills/pine/SKILL.md`

If no skill file is installed, use `pine context` and `pine --help`.
<!-- pine:end -->
