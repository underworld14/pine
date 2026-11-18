---
topic: web
updated: 2026-11-18T04:29:05Z
---

# web

- 2026-07-14: Board DnD e2e: prefer svelte-dnd-action keyboard path (focus listitem, Enter, Tab to zone, Enter) — Playwright pointer/HTML5 drag does not reliably drive it. (cites: web/e2e/board-dnd.spec.ts)
- 2026-07-14: web vite.config: import defineConfig from vitest/config only with vitest 3+ (Vite 6). vitest 2 nests Vite 5 and breaks plugin types / test field. (cites: web/vite.config.ts)
- 2026-11-18: Web ticket bodies are lazy: /api/snapshot?bodies=0 omits them and the view JSON omits an empty body (omitempty), so a missing body means 'not loaded' only for snapshot data; PATCH, SSE and GET always carry the body and a missing one there means empty. Never seed an editor buffer from 'body ?? ""' — the next autosave would wipe the ticket. (cites: web/src/lib/workspace.svelte.ts) (evidence: web/src/lib/ticket-editor.ts bodyPending(); e2e: board-scale.spec.ts 'closing a ticket opened cold never wipes its body')
