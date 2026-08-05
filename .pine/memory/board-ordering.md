---
topic: board-ordering
updated: 2026-07-18T12:31:14Z
---

# board-ordering

- 2026-07-18: Board card ordering is a client-side presentation concern: the only persisted state is an optional 'order' float in ticket frontmatter (internal/ticket, omitted when 0). web/src/lib/board-order.ts computes effective keys (floating cards get 65536-spaced slots from the default priority/recency sort; placed cards keep their order) and drops via midpoint. Gotchas: 0 is the cross-stack 'unset' sentinel (frontmatter omit + hasOrder), so computeDropOrder must never return 0; drop order must be computed against the FULL column (dropOrderInColumn) not a filtered subset, or a quick-filter corrupts hidden cards' order; and reject non-finite 'order' at parse or it breaks json.Marshal of the whole board response.
