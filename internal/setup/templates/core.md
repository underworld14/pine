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
pine create / update / close  # CLI mutations
pine doctor                   # health check
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
