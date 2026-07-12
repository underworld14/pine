---
name: pine
description: Use when working in a repository that has a .pine/ directory — for tracking issues/tickets, finding what to work on next, writing progress back, and capturing durable cross-session learnings. Triggers on "what should I work on", ticket/issue lookups, and finishing a task.
---

# Pine issue tracking

This repository uses [Pine](https://github.com/underworld14/pine) — git-native issue tracking stored in `.pine/`. Tickets and learnings are branch-scoped markdown files committed with your code. Everything below is a `pine` CLI command.

## Workflow

1. Find work: `pine ready` (or `pine ready --json` for scripts).
2. Understand a ticket: `pine show <ID>` or `pine prompt <ID>`.
3. Fix the code in the normal source tree.
4. Write back: edit `.pine/tickets/<ID>.md` (update `status`, add fix notes) or use `pine update` / `pine close`.
5. Run `pine doctor` before committing (add `--fix` to auto-repair mechanical issues).

## Essential commands

```text
pine ready [--json]              # actionable unblocked tickets
pine show <ID> [--json]          # full ticket with deps and children
pine prompt <ID>                 # paste-ready fix brief for one ticket
pine context                     # full project briefing (run at session start)
pine list [--json]               # filterable ticket list
pine log <ID> [--json]           # commits that reference or touch this ticket
pine create / update / close     # CLI mutations
pine doctor [--fix]              # health check; --fix repairs mechanical issues
```

## Write-back rules

- Tickets live in `.pine/tickets/*.md` with YAML frontmatter: `id`, `title`, `status`, `priority`, `labels`, `deps`, `parent`.
- Move a ticket on the board by editing its `status`{{BOARD_COLUMNS_LINE}}.
- `deps` lists ticket IDs that block this one; a ticket is **ready** when every dep is `done`.
- `parent` points at an EPIC ticket for grouping.
- **Do not** use markdown TODO lists for issue tracking — use Pine tickets.

## Persistent learnings

Capture cross-session, cross-agent knowledge in `.pine/learnings/` with `pine learn`:

```text
pine learn "<insight>" --scope <global|ticket|component> [--tags a,b] [--ticket ID] [--component path]
pine learn list [--scope S] [--json]      # newest first; hides superseded/stale
pine learn search "<query>"               # full-text search
pine learn show <id>                       # detail + supersede chain
pine learn supersede <old-id> "<new>"      # replace an outdated learning
pine learn rm <id>                         # delete permanently
```

Capture a learning when:

- You're corrected by the user on something non-obvious (a convention, a gotcha, a "don't do X").
- You discover a workaround or project-specific constraint not documented elsewhere.
- You finish a task and realize the approach should be the default next time.

Guidance:

- If a new insight replaces an older one, use `pine learn supersede <id> "..."` (or `--supersedes <id>`) rather than leaving both active.
- If the insight references specific files, add `--cites path/to/file` so Pine flags it when that file is deleted.
- Pass `--source <claude-code|codex|gemini|manual>` matching whichever agent you are, so learnings stay attributable (default is `manual`).
- If the insight text is exactly "list", "search", or "show", pass it via `--text` (those are subcommand names).

Do this **before ending your turn**. Skip routine facts already covered by docs, comments, or file structure — this is not a changelog. Relevant learnings are included automatically in `pine context`.
