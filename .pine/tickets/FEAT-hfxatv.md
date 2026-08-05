---
id: FEAT-hfxatv
title: 'FE: Trello-smooth board (persisted order, drag polish, quick-peek, inline add, filter, skeletons)'
status: testing
priority: high
labels:
    - frontend
created: "2026-07-18T11:57:30Z"
updated: "2026-07-18T12:30:38Z"
---

# Description

Make the web Kanban board feel as close to Trello as possible. Four clusters:
persisted drag-to-reorder, drag polish + auto-scroll, a quick-peek popover, an
inline add-card composer, an in-place quick-filter, and skeletons/empty-states.
New/reworked UI uses Tailwind v4 utilities; new code is covered by tests.

# Acceptance Criteria
- [x] Manual drag-to-reorder persists (backend `order` float) and survives reload
- [x] Drag visuals: lift/tilt/shadow on the dragged card, a drop placeholder, auto-scroll
- [x] Inline "+ Add a card" composer at the bottom of each column
- [x] Quick-peek popover (status/priority/labels/delete) from a card
- [x] In-place quick-filter (text/label/priority) + per-column empty states + skeletons
- [x] Tests at every layer (Go, Vitest unit + component, Playwright e2e) green
- [x] `prefers-reduced-motion` honored; svelte-check 0 errors

# Implementation Plan

- Part A (backend): optional `order float64` in ticket frontmatter — `ticket.go`,
  `frontmatter.go` (parse/serialize, omit when 0, reject non-finite), `merge.go`
  (`mergeFloat`), `view.go`, `ticketPatch` in `server/tickets.go`.
- Part B (FE core): `lib/board-order.ts` (`effectiveKeys`/`sortByEffective`/
  `computeDropOrder`/`dropOrderInColumn`), `workspace.reorder`, `sorted()`.
- Part C: board rewrite + `TicketCard` rework + new `AddCardComposer`,
  `QuickPeekPopover`, `BoardFilterBar`; `board-filter.ts`; `app.css` drag/skeleton
  styles; `vite.config.ts` `svelteTesting()` plugin.
- Part D: Go tests, Vitest unit + first `@testing-library/svelte` component tests,
  Playwright specs (reorder/add-card/filter/quick-peek).

# Notes

Self-review by 3 adversarial agents surfaced and fixed:
- Backend: `order: nan/inf` parsed silently and broke JSON encoding of the whole
  board response — now rejected leniently (warn + unset).
- Ordering BLOCKER: `computeDropOrder` could return exactly `0` (the cross-stack
  "unset" sentinel) → card jumped to the bottom. Now guarded to never return 0.
- Ordering MAJOR: reorder under an active filter used visible neighbors only →
  could corrupt hidden cards. Now computes against the FULL column via
  `dropOrderInColumn`, which also returns null on a drop-in-place / cancelled drag
  (no redundant frontmatter write).
- Components: keep typed text on a failed inline-create; `aria-pressed` on toggle
  controls; input `aria-label`s; touch-device access to the quick-peek button;
  close the popover if its ticket is deleted elsewhere.

Pre-merge review (requesting-code-review skill) — no Critical; fixed Important +
minors: inline composer no longer hardcodes `type:'feature'` (uses the project's
first configured type, so quick-add works on any config); QuickPeekPopover now
restores focus to the trigger + traps Tab; documented the placed-vs-floating
stability trade-off; added negative-order Go round-trip + empty-target dnd tests.

Verification: `go test ./...` ✓; `web`: vitest 77 ✓, svelte-check 0 errors,
build ✓, Playwright 10/10 ✓. Changes are uncommitted (awaiting review).

# Related Files

- internal/ticket/{ticket,frontmatter,merge}.go, internal/view/view.go, internal/server/tickets.go
- web/src/lib/{board-order,board-filter,api,workspace.svelte}.ts
- web/src/routes/board/+page.svelte
- web/src/lib/components/{TicketCard,AddCardComposer,QuickPeekPopover,BoardFilterBar}.svelte
- web/src/app.css, web/vite.config.ts

# Attachments
