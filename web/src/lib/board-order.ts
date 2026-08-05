// Board card ordering — the logic behind Trello-style drag-to-reorder that
// *sticks*. A card's manual position is persisted as a single `order` float in
// its frontmatter (see internal/ticket). Sorting stays a client concern: this
// module is pure and framework-free so it can be exhaustively unit-tested.
//
// The scheme mirrors Trello's `pos`: floating (never-placed) cards occupy
// ORDER_GAP-spaced slots derived from the default priority/recency sort, while a
// manually-placed card keeps its own key. Dropping a card assigns it the
// midpoint of its two neighbors' keys — one write, and no other card moves.

import type { Ticket } from './api';

export const ORDER_GAP = 65536;

const PRIORITY_RANK: Record<string, number> = { critical: 3, high: 2, medium: 1, low: 0 };

// defaultCompare is the legacy board sort — higher priority first, then most
// recently updated first — now used only to place cards without an explicit
// order. The id tiebreak keeps the sort total and stable.
export function defaultCompare(a: Ticket, b: Ticket): number {
  const pa = PRIORITY_RANK[a.priority] ?? 1;
  const pb = PRIORITY_RANK[b.priority] ?? 1;
  if (pa !== pb) return pb - pa;
  const ua = a.updated ?? '';
  const ub = b.updated ?? '';
  if (ua !== ub) return ub.localeCompare(ua);
  return a.id.localeCompare(b.id);
}

// hasOrder reports whether a card has been manually placed. 0 / missing /
// non-finite all mean "unset" so malformed data falls back to the default sort.
export function hasOrder(t: Ticket): boolean {
  return typeof t.order === 'number' && t.order !== 0 && Number.isFinite(t.order);
}

// effectiveKeys maps every card in a column onto one numeric axis: manually
// placed cards keep their `order`; floating cards get ORDER_GAP-spaced slots in
// default order. Because both live on the same axis, a persisted midpoint
// interleaves naturally with still-floating neighbors.
//
// Trade-off of the hybrid scheme: floating keys are recomputed from the default
// sort each render, while placed keys are absolute. So changing a *floating*
// card's priority/recency can shift its slot past a placed neighbor's fixed key.
// This self-corrects once a column is fully placed; a future column-rebalance
// pass (also covering the midpoint precision limit) would remove it entirely.
export function effectiveKeys(tickets: Ticket[]): Map<string, number> {
  const keys = new Map<string, number>();
  const floating = tickets.filter((t) => !hasOrder(t)).sort(defaultCompare);
  floating.forEach((t, i) => keys.set(t.id, (i + 1) * ORDER_GAP));
  for (const t of tickets) {
    if (hasOrder(t)) keys.set(t.id, t.order as number);
  }
  return keys;
}

// sortByEffective returns the column's cards in board order (top to bottom).
export function sortByEffective(tickets: Ticket[]): Ticket[] {
  const keys = effectiveKeys(tickets);
  return [...tickets].sort((a, b) => {
    const ka = keys.get(a.id) ?? 0;
    const kb = keys.get(b.id) ?? 0;
    if (ka !== kb) return ka - kb;
    return defaultCompare(a, b);
  });
}

// computeDropOrder returns the `order` to persist for a card dropped between the
// neighbor above (prevKey) and below (nextKey). Either is undefined at an end of
// the list; both are undefined for an empty column.
//
// NOTE: repeatedly dropping into the same shrinking gap halves it each time, so
// after ~50 consecutive same-slot drops float64 can no longer represent a
// distinct midpoint. That's far beyond hand-reachable; a periodic rebalance pass
// (renumber a column onto fresh ORDER_GAP slots) is a future refinement.
export function computeDropOrder(prevKey?: number, nextKey?: number): number {
  let v: number;
  if (prevKey == null && nextKey == null) v = ORDER_GAP;
  else if (prevKey == null) v = (nextKey as number) - ORDER_GAP / 2;
  else if (nextKey == null) v = (prevKey as number) + ORDER_GAP;
  else v = (prevKey + nextKey) / 2;
  // 0 is the cross-stack "unset" sentinel (frontmatter omits it, hasOrder ignores
  // it), so never return an exact 0 — otherwise the card would re-float and jump.
  // Nudge to a nearby non-zero key that still sits between the neighbors.
  if (v !== 0) return v;
  if (nextKey != null) return nextKey / 2 || nextKey - ORDER_GAP / 2;
  if (prevKey != null) return prevKey / 2 || prevKey + ORDER_GAP;
  return ORDER_GAP;
}

// dropOrderFor computes the order to persist for `movedId` given the column's
// desired top-to-bottom arrangement (`items`). Reference keys come from the
// other cards (the moved card is excluded so its old position never skews the
// midpoint). Returns null when `movedId` is absent.
export function dropOrderFor(items: Ticket[], movedId: string): number | null {
  const idx = items.findIndex((t) => t.id === movedId);
  if (idx === -1) return null;
  const others = items.filter((t) => t.id !== movedId);
  const keys = effectiveKeys(others);
  const prev = items[idx - 1];
  const next = items[idx + 1];
  const prevKey = prev ? keys.get(prev.id) : undefined;
  const nextKey = next ? keys.get(next.id) : undefined;
  return computeDropOrder(prevKey, nextKey);
}

// dropOrderInColumn computes the order to persist for `movedId` against the FULL
// (unfiltered) column, so a quick-filter can never corrupt hidden cards' order.
// `full` is the column's current tickets in store order (it excludes the card on
// a cross-column move); `upperNeighborId` is the id of the visible card the moved
// card was dropped directly below, or null when dropped at the top. The card is
// slotted just under that neighbor within the full column, then a midpoint is
// taken against its real (full-column) neighbors.
//
// Returns null when the arrangement is unchanged — a drop-in-place or a cancelled
// drag (which svelte-dnd-action reports on the origin zone with the card restored
// to its slot) — so no redundant write churns the ticket file.
export function dropOrderInColumn(
  full: Ticket[],
  movedId: string,
  upperNeighborId: string | null
): number | null {
  const others = full.filter((t) => t.id !== movedId);
  let insertAt = 0;
  if (upperNeighborId) {
    const li = others.findIndex((t) => t.id === upperNeighborId);
    insertAt = li === -1 ? others.length : li + 1;
  }
  const moved = full.find((t) => t.id === movedId) ?? ({ id: movedId } as Ticket);
  const arranged = [...others.slice(0, insertAt), moved, ...others.slice(insertAt)];
  if (arranged.length === full.length && arranged.every((t, i) => t.id === full[i].id)) {
    return null; // unchanged order (same-column no-op or cancelled drag)
  }
  return dropOrderFor(arranged, movedId);
}
