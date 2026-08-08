# Agent assignment and babysitting: Pine as a local multi-harness supervisor

**Date:** 2026-08-09
**Status:** Approved for planning

## Problem

Pine already stores tickets and memory in a form every coding harness can
read. What it cannot do is *route work to a specific harness and make sure
that work actually finishes*.

Three concrete gaps:

1. **No ownership.** A ticket cannot say "this belongs to Claude Code" or
   "this belongs to the QA agent". `assignee` survives a round-trip only
   because unknown frontmatter keys are preserved in `Ticket.Extra`; nothing
   reads it.
2. **No dispatch.** Starting work means a human copying `pine prompt <ID>`
   into a terminal, one ticket at a time, one harness at a time.
3. **Agents stop halfway, successfully.** This is the real problem. On a long
   task a harness frequently exits with status 0 while the work is only
   partly done. Exit codes carry no signal, so nothing detects it and the
   board silently lies.

The 2026-07-14 global-memory design explicitly deferred
"ticket→harness assignment/dispatch" as a separate follow-up. This is that
follow-up.

## Decisions (from brainstorm)

- **Topology: local spawn.** Pine runs on one machine and spawns harness
  subprocesses itself. Not distributed, not hook-only.
- **Completion contract: ticket state only.** No verify command, no evidence
  requirement, no self-report protocol. Harness-agnostic and zero-config.
- **Failure policy: resume with a capped attempt budget, then park.** Delta
  prompt on each retry; after N attempts the ticket gets a `needs-human`
  label and Pine stops.
- **Epic assignment: inherited default, one run per child.** Children without
  their own `assignee` belong to the epic's agent. Each child is its own run
  with its own contract and its own babysitter.
- **Agent identity: named profiles in config**, not bare harness names. Two
  profiles may share a harness with different roles.
- **Access control: ticket scoping.** An agent may only run tickets assigned
  to it. No path allowlists in v1.
- **Agent communication: ACL-lite over plain files.** Four performatives
  (`request`, `inform`, `block`, `handoff`), append-only, git-tracked.
- **Discovered work becomes a ticket, not a note.** An agent that finds
  follow-up work or tech debt files it with `pine create` instead of widening
  its own scope or burying it in Notes.
- **Everything configurable from the web UI and from `pine config`**, with a
  hard split between project config and machine config.
- **Execution is parallel** for tickets whose dependencies are satisfied in
  the base branch, bounded by a concurrency cap. This revises the initial
  serial-only decision; see *Scheduler*.
- **Pine owns no model and no API key.** Orchestration is deterministic;
  where judgement is genuinely needed, Pine delegates to one of the harness
  profiles the user already authenticated.

## Design

### State split: git-tracked vs runtime

Pine's premise is "`.pine/` is the database, git is the history". A
supervisor produces PIDs, logs, attempt counters and worktree paths. Those
must never reach a commit — otherwise every run adds hundreds of kilobytes of
diff and `pine sync` becomes unusable.

**Git-tracked** — decisions and outcomes:

| What | Where |
|---|---|
| `assignee` on a ticket | ticket frontmatter |
| Agent profile roster | `.pine/config.json` → `agents` |
| Role preamble templates | `.pine/templates/agents/*.md` |
| Agent messages | `.pine/messages/<TICKET-ID>.md` |
| Final status, labels, Work Evidence | ticket file (existing behaviour) |

**Runtime-only** — process traces, under `.pine/runs/`, added to
`.gitignore` by `pine init` and `pine setup`:

| What | Where |
|---|---|
| Run metadata (ticket, profile, attempt, PID, state, timings) | `.pine/runs/<run-id>/meta.json` |
| Raw harness output | `.pine/runs/<run-id>/attempt-N.log` |
| Per-ticket execution lease | `.pine/runs/locks/<TICKET-ID>.lock` |

A run id is `<TICKET-ID>-<UTC timestamp>` — unique, sortable, and readable in
a directory listing.

`internal/watch` must exclude `.pine/runs/`, or a live run's log writes will
storm the board with reload events.

### Packages

Four new packages, each with a single responsibility. Three of the four are
testable without ever starting a process.

| Package | Responsibility | Deliberately knows nothing about |
|---|---|---|
| `internal/agentprofile` | Parse and validate the `agents` config, resolve `assignee` → profile, enforce key scope, render the command line | processes, tickets |
| `internal/contract` | `Evaluate(*ticket.Ticket) Verdict` — pure, no I/O | processes, config |
| `internal/runner` | Execute exactly one attempt: prepare worktree, compose prompt, exec, capture output, wait or time out | the contract, retries |
| `internal/babysit` | The supervision loop: run → re-read ticket → evaluate → compose delta → retry to cap → park | how a process is spawned, what the contract contains |

`internal/babysit` is a library, not a command. Both `pine run` and
`pine serve` are callers — this is what makes web-initiated runs possible
without duplicating the loop.

A fifth small package, `internal/agentmsg`, owns the ACL-lite message store.

### Data model

**Ticket frontmatter.** `assignee` is promoted to a first-class field: added
to `knownKeys` in `internal/ticket/frontmatter.go`, to the `Ticket` struct,
to the serializer's key order, and to `internal/ticket/merge.go` as a scalar
field (newer `updated` wins on divergence, flagged as a conflict). Tickets
that already carry `assignee` as an extra key keep their value; the promotion
is transparent.

**Project config** — `.pine/config.json`, committed:

```json
"agents": {
  "profiles": {
    "backend":  { "harness": "claude", "preamble": "@backend",  "maxAttempts": 3 },
    "frontend": { "harness": "cursor", "preamble": "@frontend" },
    "qa":       { "harness": "gemini", "preamble": "You are QA. Write tests, not features." }
  },
  "contract": {
    "doneStatus": "done",
    "requireAcceptanceCriteria": true,
    "requirePlan": true
  },
  "maxDiscoveries": 5,
  "orchestrator": ""
}
```

`harness` names a **preset id built into the Pine binary**. Project config may
never define a `command` array — see *Security*. `preamble` is either inline
text or `@name`, resolving to `.pine/templates/agents/<name>.md`.

**Machine config** — `~/.pine/agents.json`, never committed:

```json
{
  "serveControl": false,
  "maxConcurrent": 2,
  "worktreeRoot": "~/.cache/pine/worktrees",
  "harnesses": {
    "claude": { "bin": "/opt/homebrew/bin/claude" },
    "inhouse": { "command": ["mytool", "run", "--prompt", "{{prompt}}"] }
  }
}
```

The filename is `agents.json`, **not** `config.json`, and this is load-bearing.
`internal/cli/root.go:105` identifies `~/.pine` as a memory-only store
precisely by the *absence* of `config.json`. Creating `~/.pine/config.json`
would make `isGlobalOnlyStore` return false, and `findPineDir` would then
resolve `~/.pine` as the project store from any non-repo directory under
`$HOME` — silently writing to the global store instead of erroring.

### Harness presets

Built into the binary at `internal/agentprofile/presets.go`, one entry per
known harness:

```json
{
  "id": "claude", "label": "Claude Code", "bin": "claude",
  "command": ["claude", "-p", "{{prompt}}"],
  "resume":  ["claude", "--continue", "-p", "{{prompt}}"],
  "detect":  ["claude", "--version"]
}
```

Ship presets for Claude Code, Codex, Cursor, Gemini CLI and Pi. **The exact
headless flags are to be verified against each tool during implementation,
not assumed.** Keeping them in one file makes them cheap to correct when a
harness changes its CLI.

`detect` drives the "installed" badge in the web UI and a new `pine doctor`
check. A preset whose `resume` is empty is restarted from scratch on retry
instead of resumed; the delta prompt carries the missing context either way.

### The completion contract

`contract.Evaluate` receives a parsed ticket and returns
`Verdict{Passed bool, Missing []string, Blocked bool}`.

| Check | Failure meaning |
|---|---|
| `status == contract.doneStatus` | the agent stopped before marking completion |
| Every `- [ ]` under Acceptance Criteria is `- [x]` | the work is unfinished |
| Acceptance Criteria section is non-empty | the agent never defined a target |
| Implementation Plan section is non-empty | the agent coded without a plan |
| Ticket is not `Degraded` | the agent corrupted the frontmatter |

**Blocked escape hatch.** If the ticket carries the `blocked` label, the
verdict is `Blocked` — *not* a failure. The run ends immediately with no
retry. This is how an agent says "I need a human decision" without any new
protocol: it is ordinary ticket state, visible on the board and in git.

The two failure paths are deliberately distinct: `blocked` means *stop and
ask*, `parked` means *tried N times and could not finish*.

### The babysit loop

```
lease ticket (fail fast if already leased)
set status = doing            # safety net, not a contract requirement
for attempt in 1..maxAttempts:
    runner.Run(attempt, prompt)      # prompt carries the delta from attempt-1
    reload ticket from the worktree
    verdict = contract.Evaluate(ticket)
    if verdict.Blocked: end as blocked; break
    if verdict.Passed:  end as passed; break
else:
    add label needs-human; end as parked
release lease  (worktree and branch are kept)
```

**The prompt is layered**, each layer sourced from something that already
exists:

1. `pine context` — the project briefing
2. The profile preamble
3. `pine prompt <ID>` — the rendered ticket
4. Messages addressed to this agent about this ticket
5. The contract, stated explicitly — including the `blocked` escape hatch and
   the file-it-do-not-absorb rule for discovered work
6. On attempt ≥ 2: the delta — `Verdict.Missing` rendered as a checklist

The runner exports `PINE_RUN_ID`, `PINE_AGENT` and `PINE_TICKET` into the
child environment so that any `pine` command the agent runs is attributed
correctly (notably `pine msg send`, whose sender defaults to `$PINE_AGENT`,
or `human` when unset).

### Worktrees and branches

Each run works in `<worktreeRoot>/<repo-id>/<TICKET-ID>` on branch
`pine/<TICKET-ID>`, where `repo-id` is a short stable hash of the absolute
repository root so two clones of the same project never share worktrees.
Worktrees live **outside** the repository: a full checkout churning inside
`.pine/` would fight the watcher even with an ignore rule.

Because the agent edits `.pine/tickets/<ID>.md` inside its worktree, the
contract reads the ticket from there. The main board still sees progress —
`internal/crossbranch` already surfaces tickets from other branches and is
enabled by default in this repo's config.

An existing `pine/<ID>` branch is reused rather than rejected; resuming work
must feel ordinary.

### Scheduler

A ticket is **runnable** when it has an `assignee`, is not `done`, holds no
active lease, and every ticket in its `deps` is `done` **on the base branch**.

The **base branch** is whatever the main checkout has checked out at the
moment of scheduling; it is the branch every worktree is created from.

The base-branch requirement is strict on purpose. A dependency that is `done`
only on its own unmerged branch means the dependent agent would build against
code that does not exist in its worktree.

Pine starts runnable tickets up to `maxConcurrent`, each in its own worktree.
As runs finish, the scheduler re-evaluates.

Parallel execution is safe on the Pine side because `internal/ticket/merge.go`
and the installed merge driver already perform field-aware three-way merges of
ticket files. Source-code conflicts remain, and those are inherently a human
concern.

**Pine never merges automatically.** The contract verifies ticket state, not
correctness; integrating on the strength of ticked checkboxes would be
reckless. The consequence is honest: the next wave stays blocked until a human
merges. Auto-merge is a future opt-in and should be gated on a verify command,
which this design deliberately does not include.

### Epic queue

`pine run <EPIC-ID>` resolves children, applies inherited assignment
(children without an `assignee` take the epic's), and hands the resulting set
to the scheduler. Each child is an independent run: one contract, one
babysitter, one branch. A parked child does not stop its siblings; the command
reports a per-child summary at the end.

### Discovered work

An agent working a ticket routinely finds things that are not its job: a bug
in a neighbouring package, a missing migration, tech debt it had to route
around. Today those either get silently absorbed into the current ticket
(scope creep, and the contract can no longer describe what "done" means) or
end up as a sentence in Notes that nobody reads again.

The rule, stated in the injected contract: **file it, do not absorb it.**

```
pine create --type bug --title "…" --label debt --parent EPIC-x
```

**Automatic attribution, no new flag.** The runner already exports
`PINE_TICKET` and `PINE_AGENT`. When `PINE_TICKET` is set, `store.Create`
stamps the new ticket with `links: [<origin ticket>]` and the label
`discovered`. Because `links` is the existing typed-graph field, the
relationship renders in `TicketRelations` and the graph view with no new UI.

**Not auto-assigned.** Discovered tickets are created without an `assignee`
and with a `triage` label. An agent must not be able to grow its own queue;
routing stays a human decision until the orchestrator profile takes it over
in phase 8. `pine ready --unassigned` and a board filter surface the triage
pile.

**Tech debt is a label, not a type.** `debt` on a normal bug or feature.
Ticket types are config-driven, so a team that wants a `DEBT` prefix can add
one; the design does not need it.

**Bounded.** `agents.maxDiscoveries` (project config, default 5) caps tickets
created per run. Past the cap the runner rejects further creates for that run
and the delta prompt says so — a confused agent must not be able to file two
hundred tickets.

**This is what makes `blocked` clean.** An agent that cannot finish because of
an external problem now has a tracked path: file the blocker, add it to its
own `deps`, apply the `blocked` label. The contract returns `Blocked`, the run
stops without retrying, and the reason exists as real work in the graph rather
than as prose. The scheduler will pick the dependent ticket up again once the
blocker is done and merged.

Each run's summary — in the CLI and in `/runs` — lists the tickets it created,
and Pine posts an `inform` message on the origin ticket naming them.

### ACL-lite messages

Stored at `.pine/messages/<TICKET-ID>.md`, append-only markdown:

```md
## 2026-08-09T04:12:00Z backend → qa · handoff

Endpoint /api/agents is ready. Auth path is untested.
```

Four performatives, chosen because each maps to something that actually
happens in this system:

| Performative | Meaning |
|---|---|
| `request` | asks another agent to do something |
| `inform` | reports a result or a finding |
| `block` | states that the sender is stuck, and why |
| `handoff` | transfers ownership of the next step |

Sender may be a profile name, `human`, or `pine` itself — Pine posts `inform`
when a run parks and `block` when a dependency fails. This gives Pine a voice
in the conversation without giving it a model.

Append-only markdown merges by union in practice, and the format is plain
enough to read in a diff. **There is no read state in v1**: `pine inbox
--agent qa` lists every message addressed to that agent, and prompt injection
filters by ticket. Read tracking is state that can only rot; add it if the
absence ever hurts.

### CLI surface

```
pine assign <ID> <agent>              # sugar; pine update --assignee also added
pine run <ID>                         # spawn and babysit
pine run <EPIC-ID>                    # queue children through the scheduler
pine run --ready                      # every runnable assigned ticket
pine runs                             # state, attempt, agent, duration
pine runs logs <run-id>
pine runs abort <run-id>
pine runs clean                       # explicit; refuses dirty worktrees
pine msg send --to qa --perform handoff --re FEAT-x "..."
pine inbox [--agent qa]
```

`pine run <ID>` for execution and `pine runs` for inspection are kept
separate so a ticket ID can never be mistaken for a subcommand.

`--dry-run` renders the exact command and the full prompt and executes
nothing. This is what makes the feature auditable before it is trusted.

`pine list` and `pine ready` gain `--assignee` and `--unassigned`. One concept
gets one flag name: `pine ready --assignee <name>` is the agent-facing entry
point, and `pine ready --unassigned` is the triage pile of discovered work.

### `pine config`

A new command, following the `-g` convention already established by
`pine learn -g`:

```
pine config list  [-g]
pine config get   <key> [-g]
pine config set   <key> <value> [-g]
pine config unset <key> [-g]
```

A key registry records the scope of every key, so scope errors are generated
rather than hand-written:

```
$ pine config set agents.serveControl true
error: agents.serveControl is machine-local and is never committed.
       use: pine config set -g agents.serveControl true
```

The command also becomes the supported way to toggle settings that previously
required editing JSON by hand (`crossBranch.enabled`, `sync.attachments`, …),
so its value is not limited to this feature.

### HTTP API

```
GET    /api/agents                 # profiles + installed status
PUT    /api/agents/{name}
DELETE /api/agents/{name}
GET    /api/agents/presets         # built-in harness templates
POST   /api/agents/{name}/test     # render command + prompt, execute nothing

GET    /api/runs
POST   /api/runs                   # { ticket | epic | ready, agent? }
GET    /api/runs/{id}
POST   /api/runs/{id}/abort

GET    /api/inbox?agent=qa
POST   /api/messages
```

Run state changes and log lines are delivered over the **existing** SSE
`/events` stream as new event types; no second channel.

`PUT /api/agents/{name}` is intentionally narrow rather than a whole-config
write: rewriting `config.json` from a browser races with any text editor open
on the same file.

### Web UI

**`/agents`** — profile manager. One card per profile: name, harness,
installed badge (from `detect`), assigned-ticket count, parked-run count.
A **Register harness** action starts from a preset or from Custom. The
preamble editor offers the role templates. A **Test** action calls
`/test` and shows the exact command and full prompt without executing.

**`/runs`** — the control room. Table of runs (ticket, agent, state,
attempt N of M, duration); selecting one streams its log; Abort is available
per run.

**Board** — assignee badge on cards, an `assignee` filter in
`BoardFilterBar`, a red marker for `needs-human`, a **Run** action in
`QuickPeekPopover` and `TicketCard`, and a **Run ready work** button in the
header that opens a preview (which tickets, which agents, how many run
concurrently, what is held back by dependencies) before starting. A side
panel surfaces "N branches ready to merge" from `crossbranch`.

Fields sourced from machine config render read-only with a `machine` badge
and a copyable `pine config set -g …` hint. Project-scoped fields are
editable.

### Templates

Two distinct kinds, deliberately not mixed:

1. **Harness presets** — built into the binary, served by
   `/api/agents/presets`, used to prefill the Register form.
2. **Role preambles** — `.pine/templates/agents/{backend,frontend,qa,reviewer}.md`,
   following the existing `.pine/templates/{bug,epic,feature}.md` convention.
   Plain markdown, committed, editable from the UI or an editor. `pine init`
   writes the four defaults.

### Security

Until now `pine serve` could at most write files into the repository. Once the
UI can define a command and execute it, the local server becomes a code
execution path. Two rules follow.

**Run control is off by default.** Starting runs and writing profiles require
`serveControl`, set only via `pine config set -g agents.serveControl true` or
`pine serve --agents`. Without it, `/agents` and `/runs` still render but are
read-only: profiles and logs are visible, starting and editing are not. The
toggle is CLI-only by construction — the UI cannot be used to grant the UI
power.

**Project config may never define a command.** It may only reference a preset
id. If a committed file could specify an argv, cloning a hostile repository
and running `pine run` would be arbitrary code execution — the
`.vscode/tasks.json` hole. Custom harnesses are registered per machine. The
trade-off is real: a team using an in-house harness configures it once per
machine. Trust-on-first-use could relax this later; it is not built now.

The existing localhost binding and Origin checks in
`internal/server/security.go` remain and are now load-bearing.

### Pine as orchestrator

Pine owns **no model and no API key**. Selecting runnable tickets, spawning,
evaluating the contract and composing a delta prompt are all deterministic.
Embedding an LLM would add credentials, cost and nondeterminism, and would
contradict the README's first promise — *"No cloud, no accounts."*

Judgement is genuinely useful in four places: routing new tickets, reviewing a
diff against its acceptance criteria, splitting an epic, and reconciling
conflicting `block` messages. Pine can have all of them without a key, by
delegating to a profile the user already authenticated:

```json
"agents": { "orchestrator": "reviewer" }
```

Pine spawns that profile with an orchestrator prompt and requests structured
output. The config key and a `ContractExtension` interface are defined in this
design; only **diff-versus-acceptance-criteria review** is scheduled for
implementation (phase 7), because it closes the largest hole in the
state-only contract: a ticked checkbox is not correct work.

### Error handling

The governing rule: **Pine never discards an agent's work automatically.**
Failed, parked and aborted runs keep their worktree and branch. Cleanup
happens only through `pine runs clean`, which refuses worktrees with
uncommitted changes and reports their paths.

| Situation | Behaviour |
|---|---|
| Harness binary missing | Fail before creating a worktree; the message includes the exact `pine config set -g` command |
| Non-zero exit | Still evaluate the contract — the work may be complete despite a poor exit. Exit code goes to the log, not to the verdict |
| Process hangs | Per-attempt timeout (default 30 min, per-profile). Kill the process group; count one failed attempt |
| Frontmatter corrupted by the agent | `ticket.Parse` already returns `Degraded` without losing data; the contract fails with that specific reason and the delta prompt asks for a repair |
| Ticket untouched | Contract fails; the delta prompt becomes maximally explicit |
| Supervisor dies mid-run | The lease holds a PID; on next start a dead PID marks the run `orphaned` with its worktree intact, and offers resume |
| Stale lease | Same PID liveness check; a dead holder's lease is reclaimed |
| Branch already exists | Reused, not rejected |
| Log growth | Per-attempt cap (default 10 MB), truncated in the middle with a marker. `.pine/runs/` is gitignored |
| Two starters race a ticket | The lease is the single arbiter: one active run per ticket |

## Testing

Hard rule: **no test may invoke a real AI harness.** Every executed command in
tests is a fake.

- **`internal/contract`** — table-driven over in-memory ticket fixtures. The
  cheapest and most important tests in the feature; this is where coverage
  must be thick against the repo's ≥90% gate. Cases: each check failing
  alone, all passing, `blocked` short-circuit, `Degraded` input, absent
  sections, mixed checkbox states.
- **`internal/agentprofile`** — pure: `assignee` resolution, `{{prompt}}`
  rendering, preset inheritance, `@name` preamble resolution, and **scope
  enforcement** (reject `serveControl` in project config; reject `command` in
  project config).
- **`internal/runner`** — `sh -c` fakes: one that writes a satisfying ticket
  and exits 0, one that exits 1, one that sleeps to exercise the timeout and
  process-group kill, one that writes broken frontmatter.
- **`internal/babysit`** — runner and contract stubbed behind interfaces.
  Cases: pass on attempt 1; pass on attempt 3; park after the cap; stop
  immediately on `blocked`; delta prompt contains exactly `Verdict.Missing`;
  lease released on every exit path.
- **Scheduler** — dependency-wave ordering, `maxConcurrent` respected, a
  dependency done only on a side branch does *not* unlock its dependent.
- **`internal/server`** — existing `httptest` patterns. Mandatory security
  test: with `serveControl` off, every write and run-start endpoint is
  rejected.
- **`internal/agentmsg`** — append ordering, sender attribution from
  `PINE_AGENT`, inbox filtering by agent and by ticket.
- **Discovered work** — `store.Create` under `PINE_TICKET` stamps `links` and
  the `discovered` label and leaves `assignee` empty; the `maxDiscoveries` cap
  rejects the sixth create in a run; a create with no `PINE_TICKET` in the
  environment behaves exactly as it does today.
- **Web** — vitest for `/agents` and `/runs` components following the existing
  `*.test.ts` convention; the existing Playwright e2e suite covers the board
  additions.
- **`pine doctor`** — profile referencing an unknown harness; binary absent;
  `assignee` naming a profile that does not exist.

## Build order

Each phase is independently shippable and gets **its own implementation
plan** — this spec is too large for a single plan, and phases 4 onward should
be re-read against what phases 1–3 actually taught us before being planned.

Phases 1 and 2 execute nothing, so if the supervision approach needs to
change, everything already released still stands on its own.

1. `assignee` as a first-class field; `pine assign`; `--assignee` filters in
   `list`, `ready`, and the board.
2. `pine config`; the `agents` config schema; harness presets; `pine doctor`
   checks.
3. `internal/contract`, `internal/runner`, `internal/babysit`; `pine run`,
   `pine runs`; discovered-work stamping and the `maxDiscoveries` cap.
4. Scheduler: dependency waves, `maxConcurrent`, `pine run --ready`.
5. Epic queue with inherited assignment.
6. ACL-lite: `internal/agentmsg`, `pine msg`, `pine inbox`, prompt injection.
7. Web UI: `/agents`, `/runs`, board actions, HTTP endpoints, `serveControl`.
8. Orchestrator review (diff versus acceptance criteria) via the delegated
   profile.

## Out of scope

- Distributed execution across machines or CI.
- Auto-merge of completed branches. Requires a verify command first.
- Verify commands, evidence requirements and self-report sentinels in the
  contract — considered and deliberately excluded.
- Path-level ACLs. Ticket scoping only.
- Full FIPA-ACL: conversation ids, reply threading, ontologies, the wider
  performative set. No harness speaks it.
- A real-time inter-agent message bus. Runs are independent processes; the
  message store is the channel.
- Pine holding its own model credentials, in any phase.
- Trust-on-first-use for repository-defined commands.
- Read state for the message inbox.
