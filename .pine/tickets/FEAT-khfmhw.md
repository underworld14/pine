---
id: FEAT-khfmhw
title: 'FE: svelte-check CI gate, a11y, e2e coverage'
status: done
priority: high
created: "2026-07-14T16:02:30Z"
updated: "2026-07-14T16:06:08Z"
---

# Description

Close the FE verification gap: type errors blocked `npm run check` from CI, modals lacked keyboard a11y, and Playwright only covered create + search.

# Acceptance Criteria
- [x] `npm run check` exits 0 (errors)
- [x] CI `web` job runs `npm run check` before build
- [x] CommandPalette / NewIssueModal / lightbox keyboard-dismissable
- [x] E2E covers board move, 409 conflict banner, attachments, live-sync

# Implementation Plan
1. Fix `$page.params.id`, vitest/config (upgrade vitest 3), markdown-it-task-lists shim
2. Gate check in CI + Makefile `check-web`
3. A11y dialog labeling + Escape / tabindex
4. Add Playwright specs + testids

# Notes
- Upgraded `vitest` 2→3 so `defineConfig` from `vitest/config` type-checks against Vite 6 (vitest 2 nested Vite 5).
- Board DnD e2e uses svelte-dnd-action keyboard path (Enter / focus zone / Enter); pointer drag was unreliable.
- Conflict "Reload" exits edit mode (pointerdown outside editor shell) — assert preview text.
- Code review: `type="button"` on NewIssueModal non-submit controls + block Enter on labels (form wrap regression).

# Related Files
- `web/vite.config.ts`, `web/src/markdown-it-task-lists.d.ts`
- `web/src/routes/tickets/[id]/+page.svelte`, `web/src/routes/board/+page.svelte`
- `web/src/lib/components/CommandPalette.svelte`, `NewIssueModal.svelte`
- `.github/workflows/ci.yml`, `Makefile`
- `web/e2e/{board-dnd,conflict,attachments,live-sync}.spec.ts`

# Attachments
