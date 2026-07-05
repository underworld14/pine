# Pine — Structured Sections & Acceptance-Criteria Checklists

**Status:** Design approved, pending spec review
**Date:** 2026-07-05

## Problem & Goal

Pine ticket bodies are freeform markdown. There is no consistent structure across
tickets and no machine-readable notion of "done." That hurts the human→AI-agent
handoff: an agent has nothing concrete to fill in and no signal for when its work
satisfies the ticket.

**Goal (both, equally):**
1. A **shared authoring template** (Description / Acceptance Criteria / Implementation
   Plan / Notes) so humans and agents structure every ticket the same way.
2. A **machine-checkable "done" signal** from acceptance-criteria checkboxes, surfaced
   on cards, in the CLI, and in AI context.

Adopts the idea behind Backlog.md's structured sections + checklists, adapted to Pine's
flat-file, frontmatter-status model.

## Key decisions (locked)

- **Progress source:** `done ÷ total` checkboxes **under the "Acceptance Criteria"
  section only**. Checkboxes elsewhere (e.g. Implementation Plan) still render and
  toggle, but do **not** affect the done signal.
- **Checkbox interaction:** click-to-toggle in the web preview, via a **dedicated server
  endpoint** (`PATCH /api/tickets/{id}/checklist`) — atomic, idempotent, and reused by
  the CLI / future MCP. No client-side body rewriting.
- **Reuse, not rebuild:** `internal/ticket/sections.go` already parses `# Heading`
  sections (`Sections`, `SectionContent`). Templates already flow through
  `store.Create → store.template()`. This feature extends both.

## Non-goals (v1)

- No schema enforcement (a ticket without an Acceptance Criteria section is valid;
  it simply has no progress signal).
- No per-item metadata (assignee/estimate on a checkbox).
- No reordering/editing checklist items except via the markdown body editor and toggle.
- Checkbox lines inside fenced code blocks are not special-cased (see Edge cases).

## Architecture

```
body markdown ──► ticket.AcceptanceProgress(body)  ──► view.Ticket.Acceptance {done,total}
                  ticket.SetChecklistItem(body,i,✓) ──► store.UpdateIfMatch (atomic)
                          ▲                                     ▲
        PATCH /api/tickets/{id}/checklist {index,checked}  ◄────┘  emits ticket.updated
                          ▲
        web preview: <input type=checkbox> click → api.setChecklistItem(id, domIndex, checked, hash)
```

Domain logic is pure and lives in `internal/ticket`; the server endpoint and the CLI/MCP
all call the same functions, so every surface behaves identically.

## Components

### 1. Domain — `internal/ticket/checklist.go` (new)
Pure, no I/O. One checkbox line regex: `^(\s*[-*]\s+)\[([ xX])\]\s+\S`.

- `AcceptanceProgress(body string) (done, total int)` — `SectionContent(body,
  "Acceptance Criteria")` (case-insensitive, already exists), count checkbox lines in
  that section. Returns `0,0` when the section is absent or has no checkboxes.
- `SetChecklistItem(body string, index int, checked bool) (newBody string, ok bool)` —
  find the `index`-th checkbox line in the **whole body** (document order, 0-based),
  set its marker to `x`/space. `ok=false` when index is out of range. **Idempotent**:
  setting an already-`x` item to checked is a no-op success. Preserves the rest of the
  line (indentation, marker char `-`/`*`, text) byte-for-byte.

Index space = the whole body (matches DOM render order in the web preview). The AC
progress counts only the AC subset; toggling addresses any checkbox.

### 2. DTO — `internal/view/view.go`
Add `Acceptance *Progress \`json:"acceptance,omitempty"\`` (reuse the existing
`Progress{Done,Total int}` type that backs `epicProgress`). Set it in `Build` and
`BuildOffBranch` via `ticket.AcceptanceProgress(t.Body)` — `nil` when `total==0` so it's
omitted. Mirror `acceptance?: { done: number; total: number }` in
`web/src/lib/api.ts` `interface Ticket`.

### 3. Server endpoint — `internal/server/tickets.go`
`PATCH /api/tickets/{id}/checklist`, body `{index:int, checked:bool, opId?:string}`,
header `If-Match: <hash>`.
- Reject off-branch: `if branch, ok := srv.offBranchRef(id); ok { readOnlyBranch → 409 }`
  (reuses the cross-branch guard).
- `srv.store.UpdateIfMatch(id, ifMatch, func(t) { nb, ok := ticket.SetChecklistItem(
  t.Body, index, checked); if !ok { return badRequest }; t.Body = nb; return nil })`.
- On `ErrConflict` (body moved on disk → index may be stale) return the existing 409
  conflict shape so the client re-hydrates. On success: `setETag`, `reindex`,
  `emit("ticket.updated", apiOrigin(opId), {ticket})`, return the view.
- Route registered next to the existing ticket routes in `server.go`.

### 4. Templates — `internal/store/create.go` + `pine init`
Update the built-in defaults and the files `pine init` writes so every type carries the
standard skeleton with example unchecked criteria:
```
# Description

# Acceptance Criteria
- [ ] …

# Implementation Plan

# Notes
```
- `featureTemplate` already has `# Acceptance Criteria` — add the example item +
  Implementation Plan/Notes.
- `bugTemplate` keeps Steps/Expected/Actual but gains `# Acceptance Criteria`.
- `epicTemplate` unchanged (epics track children, not criteria).
- `pine init` (internal/cli/init.go) writes the same skeletons to
  `.pine/templates/{bug,feature,epic}.md`. Existing repos are unaffected (their template
  files win); the win shows up for new `pine init` and for anyone using built-in defaults.

### 5. Web — progress readout
- `web/src/lib/components/TicketCard.svelte`: when `ticket.acceptance?.total`, render a
  compact pill `▓▓▓░░ 3/5` (a tiny bar + count) in the card footer.
- `web/src/routes/tickets/[id]/+page.svelte`: show `Acceptance N/M` in the meta row.

### 6. Web — interactive checkboxes
- `web/src/lib/markdown.ts`: render GFM task-list items (`- [ ]` / `- [x]`) as real
  `<input type="checkbox">` (checked per state) via a small markdown-it rule (or the
  `markdown-it-task-lists` plugin, bundled by Vite). Do **not** use a `data-*` index
  attribute — DOMPurify runs with `ALLOW_DATA_ATTR:false` and would strip it. `<input>`
  with `type`/`checked` survives the existing sanitize config.
- `tickets/[id]/+page.svelte`: event-delegate `change` on the preview container; on a
  checkbox change, compute its index = position among all
  `container.querySelectorAll('input[type=checkbox]')` (DOM order == body document order
  == server index), then call `api.setChecklistItem(id, index, checked, ticket.hash)`.
  Update the store from the response (SSE also echoes). On 409 conflict, reuse the
  existing conflict banner / reload path.
- **Read-only:** when `ticket.readOnly` (off-branch) or in preview of a degraded ticket,
  render checkboxes `disabled` and ignore changes (server 409s regardless).

### 7. API client — `web/src/lib/api.ts`
`setChecklistItem(id, index, checked, ifMatch) => req('PATCH',
'/api/tickets/'+id+'/checklist', {index, checked, opId}, {'If-Match': ifMatch})`.

### 8. AI context — `internal/contextgen`
Include the Acceptance Criteria section and its `done/total` in generated ticket context
/ prompts, so an agent sees the definition of done and current progress.

## Data flow (toggle)

1. User clicks an unchecked box in the rendered preview.
2. Component computes DOM-order index `i`, calls `PATCH …/checklist {index:i,
   checked:true}` with `If-Match: <hash the client last saw>`.
3. Server rejects if off-branch (409); else atomically flips the `i`-th checkbox in the
   body under the store lock (If-Match guards against a disk change moving the index),
   re-derives `Acceptance`, emits `ticket.updated`.
4. Client applies the returned ticket (progress pill + preview update); other clients get
   the SSE echo.

## Edge cases

- **No Acceptance Criteria section** → `AcceptanceProgress` returns `0,0` → `acceptance`
  omitted → no pill. Valid ticket.
- **Checkbox inside a fenced code block** → the line regex counts it, but markdown-it
  renders it as code text, not a checkbox → DOM index and body index can drift. Accepted
  v1 limitation (AC sections are simple bullet lists); documented, not special-cased.
- **Stale index** (body changed between load and click) → If-Match mismatch → 409 →
  client reloads. No silent wrong-item toggle.
- **Uppercase `[X]`** counted as checked; `SetChecklistItem` normalizes to `x`.
- **Nested checklists** render and toggle in document order; progress still counts only
  AC-section items.
- **Off-branch (read-only) tickets** → checkboxes disabled; endpoint 409s.

## Testing

**Go unit (`internal/ticket/checklist_test.go`):** `AcceptanceProgress` — no section,
empty section, mixed `[ ]/[x]/[X]`, checkboxes both inside and outside AC (only AC
counted), nested items; `SetChecklistItem` — toggle on/off, idempotent set, out-of-range
→ `ok=false`, preserves indentation/marker/text, multiple sections index correctly.

**Go server (`internal/server/…_test.go`):** `PATCH …/checklist` flips a box and bumps
progress; If-Match conflict → 409; off-branch id → 409; bad index → 400; emits
`ticket.updated`.

**Web:** markdown renders task items as checkboxes; DOM-order index matches; a change
calls the endpoint with the right index; card shows the progress pill; off-branch/readOnly
checkboxes are disabled.

**End-to-end:** `pine init` (new template has AC), create a ticket, open it, tick a box →
card shows 1/1 and the file on disk shows `- [x]`.

## Critical files

| File | Change |
|---|---|
| `internal/ticket/checklist.go` (new) | `AcceptanceProgress`, `SetChecklistItem` |
| `internal/view/view.go` | `Acceptance *Progress` on the DTO; set in `Build`/`BuildOffBranch` |
| `internal/server/tickets.go`, `server.go` | `PATCH …/checklist` handler + route |
| `internal/store/create.go`, `internal/cli/init.go` | standard section templates |
| `internal/contextgen/*` | AC + progress in agent context |
| `web/src/lib/api.ts` | `acceptance` field + `setChecklistItem` |
| `web/src/lib/markdown.ts` | render task-list checkboxes |
| `web/src/lib/components/TicketCard.svelte` | progress pill |
| `web/src/routes/tickets/[id]/+page.svelte` | interactive checkboxes + meta progress |
