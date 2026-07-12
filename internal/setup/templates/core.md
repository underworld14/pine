## Pine issue tracking

This repository uses [Pine](https://github.com/underworld14/pine) — git-native issue tracking stored in `.pine/`. Tickets are branch-scoped markdown files committed with your code.

### Workflow

1. Find work: `pine ready` (or `pine ready --json` for scripts).
2. Understand a ticket: `pine show <ID>` or `pine prompt <ID>`.
3. Fix the code in the normal source tree.
4. Write back: edit `.pine/tickets/<ID>.md` (update `status`, add fix notes) or use `pine update` / `pine close`.
5. Run `pine doctor` before committing.

### Essential commands

```text
pine ready [--json]           # actionable unblocked tickets
pine show <ID> [--json]       # full ticket with deps and children
pine prompt <ID>              # paste-ready fix brief for one ticket
pine context                  # full project briefing (run at session start)
pine list [--json]            # filterable ticket list
pine log <ID> [--json]        # commits that reference or touch this ticket
pine create / update / close  # CLI mutations
pine learn "<insight>"        # capture a durable cross-agent learning
pine learn list                # list captured learnings
pine learn search "<query>"   # search learnings
pine learn show <id>           # one learning's detail and supersede chain
pine learn supersede <id> "…"  # replace an outdated learning
pine learn rm <id>             # delete a learning
pine doctor [--fix]           # health check; --fix repairs mechanical issues
```

### Write-back rules

- Tickets live in `.pine/tickets/*.md` with YAML frontmatter: `id`, `title`, `status`, `priority`, `labels`, `deps`, `parent`.
- Move a ticket on the board by editing its `status`{{BOARD_COLUMNS_LINE}}.
- `deps` lists ticket IDs that block this one; a ticket is **ready** when every dep is `done`.
- `parent` points at an EPIC ticket for grouping.
- Attachments live in `.pine/attachments/<ID>/` and are referenced relatively from the ticket body.
- **Do not** use markdown TODO lists for issue tracking — use Pine tickets.
- Prefer editing ticket files over ad-hoc notes or separate task files.

### Live context

Run `pine context` at the start of a session for current tickets, git state, and ready work.

### Persistent learnings

This project uses `pine learn` to capture cross-session, cross-agent knowledge in `.pine/learnings/`.

Call `pine learn "<insight>" --scope <global|ticket|component> [--tags a,b] [--ticket ID] [--component path]` when:

- You're corrected by the user on something non-obvious (a convention, a gotcha, a "don't do X")
- You discover a workaround or project-specific constraint not already documented elsewhere
- You finish a task and realize the approach should be the default next time

If a new insight replaces an earlier learning rather than adding to it, use `pine learn supersede <id> "<new insight>"` (or `pine learn ... --supersedes <id>`) instead of leaving both active. To remove one outright, use `pine learn rm <id>`.

If your insight references specific files, add `--cites path/to/file` so Pine can flag it automatically if that file is later deleted.

Pass `--source <claude-code|codex|gemini|manual>` matching whichever agent you are, so learnings stay attributable across agents (default is `manual`). If your insight text is exactly "list", "search", or "show", pass it via `--text` instead of as a positional argument — e.g. `pine learn --text "list"`.

Do this before ending your turn. Skip routine facts already covered by docs, comments, or file structure — this is not a changelog.

Relevant learnings are included automatically in `pine context`. Run `pine learn search "<topic>"` for anything not surfaced there.
