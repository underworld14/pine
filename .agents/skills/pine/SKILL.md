---
name: pine
description: Use when working in a repository that has a .pine/ directory — for tracking issues/tickets, finding what to work on next, writing progress back, and capturing durable cross-session learnings. Triggers on "what should I work on", ticket/issue lookups, and finishing a task.
---

# Pine issue tracking

This repository uses [Pine](https://github.com/underworld14/pine) — git-native issue tracking stored in `.pine/`. Tickets and learnings are branch-scoped markdown files committed with your code. Everything below is a `pine` CLI command.

## Workflow

1. Find work: `pine ready` (or `pine ready --json` for scripts).
2. Understand a ticket: `pine show <ID>` or `pine prompt <ID>`. If the ticket declares `skills:`, load those skills before starting work.
3. Fix the code in the normal source tree.
4. Write back: edit `.pine/tickets/<ID>.md` (update `status`, add fix notes) or use `pine update` / `pine close`.
5. Run `pine doctor` before committing (add `--fix` to auto-repair mechanical issues).

## Plan → ticket → close with evidence (agent workflow)

When you own the work end-to-end (planning a non-trivial change, implementing it,
then closing it out), keep Pine in the loop so the work is traceable and
auditable:

1. **Planning a non-trivial task** — before writing code, create a ticket so
   the work is tracked from the start:
   ```text
   pine create --type feature --title "<short plan title>"
   ```
   Capture the printed `<ID>`, then move it to `doing`:
   ```text
   pine update <ID> --status doing
   ```
   (Skip this for trivial, single-step changes — Pine is for real work, not
   every edit.)
2. **While working** — keep notes/decisions in `.pine/tickets/<ID>.md`. If you
   discover a durable insight, `pine learn "…"`.
3. **Before closing** — if the ticket touches production code, record a proof
   command in its frontmatter so "done" means "proven live":
   ```text
   pine update <ID> --verify 'grep -rn "NewFunc(" internal/ | grep -v _test.go'
   ```
   A grep for real callers or a smoke command both work.
4. **When the work is done** — close the ticket AND attach the file-change
   evidence in one step:
   ```text
   pine close <ID> --evidence
   ```
   This runs the ticket's `verify:` command in the repo root (a failure
   refuses the close — fix it or pass `--force` to close with the failure
   recorded), then marks the ticket `done` and appends a `## Verification`
   section (command, exit status, output) plus a `## Work Evidence` section
   (commits that reference/touch the ticket + a `git diff --stat`, capped at
   50 files, from the commit at its creation time to the working tree) — a
   durable record of what changed and why, and proof it is actually wired in.
5. **Work already committed?** File and close it with exact evidence instead of
   a creation-anchored diff — `--commit <rev>` (repeatable) records those
   commits as the ticket's `## Work Evidence`:
   ```text
   pine create --type bug --title "<what was fixed>" --status testing --commit HEAD
   pine close <ID> --evidence                     # keeps evidence recorded this way
   pine close <ID> --evidence --commit <rev>      # (re)record from specific commits
   ```
   Executors that may not touch `.pine/` can leave this to the orchestrator:
   one `pine create … --commit <sha>` per part, left in `testing` for a reviewer.

## Essential commands

```text
pine ready [--json]              # actionable unblocked tickets
pine show <ID> [--json]          # full ticket with deps and children
pine prompt <ID>                 # fix brief for bugs, implement brief for features
pine prompt <ID> --template name # render .pine/prompts/<name>.md instead
pine context [--full]            # compact briefing by default; --full = exhaustive
pine list [--all] [--json]       # open tickets by default; --all includes done
pine log <ID> [--json]           # commits that reference or touch this ticket
pine changed [--since <ref>] [--json]   # .pine/ files changed since a ref (diff-able snapshot for gates)
pine release-notes --since <ref> [--group-by type|label] [--json]   # Markdown notes from tickets closed since a ref
pine create / update / close     # CLI mutations
pine update <ID> --body-file f   # replace the markdown body (or '-' for stdin)
pine update <ID> --add-skill x / --rm-skill x   # manage the ticket's required skills
pine dep add <ID> <DEP>… / rm    # rewire dependencies after creation (refuses cycles)
pine close <ID> --evidence       # run verify:, attach evidence, mark done
pine create … --commit <rev>     # record exact commits as the ticket's work evidence
pine learn "<insight>"           # capture a durable cross-agent learning
pine learn list / search / show  # inspect learnings
pine learn supersede / rm        # replace or delete a learning
pine doctor [--fix]              # health check; --fix repairs mechanical issues
```

## Write-back rules

- Tickets live in `.pine/tickets/*.md` with YAML frontmatter: `id`, `title`, `status`, `priority`, `labels`, `deps`, `parent`, `phase`, `verify`, `skills`, `links`.
- Move a ticket on the board by editing its `status` (board columns: todo, doing, testing, done).
- `deps` lists ticket IDs that block this one; a ticket is **ready** when every dep is `done`. Rewire them with `pine dep add/rm` or `pine update --add-dep/--rm-dep` — no need to create tickets in dependency order.
- `parent` points at an EPIC ticket for grouping.
- `verify:` is a shell command proving the work is live (grep for callers, smoke test); `pine close --evidence` runs it and refuses to close on failure.
- `skills:` names agent skills the executing agent must load before starting the ticket — the user should not have to invoke them manually. Manage with `pine update <ID> --add-skill <name>` / `--rm-skill <name>`; `pine prompt` and `pine context` restate them. If a required skill is missing on the machine, try to find/install it (skills.sh, GitHub, or a skill finder like `/find-skills`); if it stays unavailable, note that in the ticket's `# Notes` and proceed carefully.
- The body uses `#`-level sections. Pine reads `# Acceptance Criteria` (checkboxes drive progress) and `# Related Files` — one bullet per repo-relative path, annotation after the path is fine:
  ```markdown
  # Related Files
  - internal/cli/tickets.go — add the flag
  - internal/contextgen/    — whole folder
  ```
  `pine context` uses these paths to pick tickets relevant to your current changes.
- Attachments live in `.pine/attachments/<ID>/` and are referenced relatively from the ticket body.
- **Committing ticket changes:** Pine never runs `git commit`; ticket files are plain files. Stage `.pine/tickets/<ID>.md` in the code commit it describes. For status-only moves (hand-off, review verdict) write the verdict as a section in the ticket body and fold it into the next code commit, or make one `pine:` commit per session — not one per move. The record survives squashing because it lives in the ticket. `pine release-notes` ignores commits that only touch `.pine/`.
- **Do not** use markdown TODO lists for issue tracking — use Pine tickets.
- Prefer editing ticket files over ad-hoc notes or separate task files.

## Live context

Run `pine context` at the start of a session. It is compact by default — board counts, ready work, tickets relevant to your uncommitted changes and recent commits, memory, conventions — so done work does not flood your context window. Pass `--full` when you need every open ticket table, acceptance criteria, epics, and recent commits.

## Persistent learnings

Capture cross-session knowledge without bloating `.pine/learnings/` with one file per insight:

```text
pine learn suggest "<insight>" [--cites path]   # rank MEMORY.md / topics (no write)
pine learn "<insight>" [--cites path]           # auto-append when confident; else requires --to
pine learn "<insight>" --to MEMORY.md           # project prefs / general rules
pine learn "<insight>" --to memory/analytics.md # domain topic (append)
pine learn "<insight>" --new-topic analytics    # create topic + first bullet
pine learn "<insight>" --evidence "<cmd|citation>"  # record the proof inline with the bullet
pine learn "<insight>" --scope ticket --ticket ID   # rare ticket-scoped LRN-*
pine learn list / search / show MEMORY.md
pine learn -g "<insight>"                       # personal: applies in every repo (~/.pine)
pine learn -g "<insight>" --new-topic pnpm      # personal topic
pine learn list -g / search -g / show -g MEMORY.md
```

**Where to put insights**

- `~/.pine/MEMORY.md` — personal, cross-repo preferences (`pine learn -g`; works in any repo)
- `MEMORY.md` — project-wide rules for *this* repo, “never do X here”
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

## Memory discipline

Pine is the memory that survives you switching harness (Claude Code, Codex, Gemini CLI,
Cursor). Keep it small, current, and true.

**Checkpoint** — after finishing a task (closing a ticket, fixing a bug), ask: “what did I
learn that outlives this session?” If something, save it before moving on.

**Check before you write** — search first (`pine learn search "<term>"`, add `-g` for personal
memory). Update the existing line instead of appending a near-duplicate; delete lines that
turned out wrong.

**One fact per bullet** — concrete and self-contained; readable a month later with no session
context. No ticket IDs, no dates (pine prefixes the write date itself).

**Stale memory** — every bullet carries the date it was written; bullets older than
90 days are marked `⚠ unverified since <date>` in `pine context` and flagged by
`pine doctor`. When a probe or code review refutes an old line, delete it or
append a fresh dated bullet with `--evidence "<probe>"` stating the newer
finding — do not leave contradicted lines sitting next to true ones.

**Where it goes**

- About you, or true in any repo (tools, style, habits) → `pine learn -g "…"` → `~/.pine/`
- About this repo → `pine learn "…"` → `.pine/MEMORY.md` or `.pine/memory/<topic>.md`
- Ephemeral note about one ticket → `pine learn "…" --scope ticket --ticket <ID>`

**Do not save** what the repo already records: code, git history, the instruction files,
routine ticket completion, or anything already in the docs.

If global and project memory disagree, the project wins.

Relevant MEMORY / topics / learnings — plus your global preferences from `~/.pine/` — are
included automatically in `pine context`.
