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
pine learn "<insight>"           # capture a durable cross-agent learning
pine learn list / search / show  # inspect learnings
pine learn supersede / rm        # replace or delete a learning
pine doctor [--fix]              # health check; --fix repairs mechanical issues
```

## Write-back rules

- Tickets live in `.pine/tickets/*.md` with YAML frontmatter: `id`, `title`, `status`, `priority`, `labels`, `deps`, `parent`.
- Move a ticket on the board by editing its `status`{{BOARD_COLUMNS_LINE}}
- `deps` lists ticket IDs that block this one; a ticket is **ready** when every dep is `done`.
- `parent` points at an EPIC ticket for grouping.
- Attachments live in `.pine/attachments/<ID>/` and are referenced relatively from the ticket body.
- **Do not** use markdown TODO lists for issue tracking — use Pine tickets.
- Prefer editing ticket files over ad-hoc notes or separate task files.

## Live context

Run `pine context` at the start of a session for current tickets, git state, ready work, and relevant learnings.

## Persistent learnings

Capture cross-session knowledge without bloating `.pine/learnings/` with one file per insight:

```text
pine learn suggest "<insight>" [--cites path]   # rank MEMORY.md / topics (no write)
pine learn "<insight>" [--cites path]           # auto-append when confident; else requires --to
pine learn "<insight>" --to MEMORY.md           # project prefs / general rules
pine learn "<insight>" --to memory/analytics.md # domain topic (append)
pine learn "<insight>" --new-topic analytics    # create topic + first bullet
pine learn "<insight>" --scope ticket --ticket ID   # rare ticket-scoped LRN-*
pine learn list / search / show MEMORY.md
```

**Where to put insights**

- `MEMORY.md` — user preferences, project-wide rules, “never do X”
- `memory/<topic>.md` — domain gotchas (append to an existing topic when relevant)
- `LRN-*` — only ticket-scoped / ephemeral notes (`--scope ticket`)

**When to capture**

- You're corrected by the user on something non-obvious (a convention, a gotcha, a "don't do X").
- You discover a workaround or project-specific constraint not documented elsewhere.
- You finish a task and realize the approach should be the default next time.

**Do not** mint a learning for routine ticket completion, changelogs, or facts already in docs/code.

Guidance:

- Prefer `pine learn suggest` then `--to` / auto-append over creating new LRN files.
- If a new insight replaces an older **LRN**, use `pine learn supersede <id> "..."` (or `--supersedes <id>`).
- If the insight references specific files, add `--cites path/to/file`.
- Pass `--source <claude-code|codex|gemini|manual>` matching whichever agent you are.
- If the insight text is exactly "list", "search", "show", or "suggest", pass it via `--text`.

Relevant MEMORY / topics / learnings are included automatically in `pine context`.
