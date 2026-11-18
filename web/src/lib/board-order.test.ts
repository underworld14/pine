import { describe, it, expect } from 'vitest';
import {
  ORDER_GAP,
  defaultCompare,
  hasOrder,
  effectiveKeys,
  sortByEffective,
  computeDropOrder,
  dropOrderFor,
  dropOrderInColumn,
  COLUMN_PAGE,
  capColumn,
  revealNext,
  hiddenCount,
  sameRefs,
  reuseColumns,
  type BoardColumn
} from './board-order';
import type { Ticket } from './api';

function mk(id: string, extra: Partial<Ticket> = {}): Ticket {
  return {
    id, type: id.split('-')[0], title: id, status: 'todo', priority: 'medium',
    labels: [], deps: [], created: '2026-07-04T00:00:00Z', updated: '2026-07-04T00:00:00Z',
    blocked: false, hash: 'h-' + id, attachments: [], ...extra
  };
}

describe('defaultCompare', () => {
  it('ranks higher priority first', () => {
    const cards = [mk('a', { priority: 'low' }), mk('b', { priority: 'critical' }), mk('c', { priority: 'medium' })];
    expect([...cards].sort(defaultCompare).map((t) => t.id)).toEqual(['b', 'c', 'a']);
  });
  it('breaks ties by most-recent update, then id', () => {
    const cards = [
      mk('a', { updated: '2026-07-04T09:00:00Z' }),
      mk('b', { updated: '2026-07-04T11:00:00Z' })
    ];
    expect([...cards].sort(defaultCompare).map((t) => t.id)).toEqual(['b', 'a']);
  });
});

describe('hasOrder', () => {
  it('treats 0, missing and non-finite as unset', () => {
    expect(hasOrder(mk('a'))).toBe(false);
    expect(hasOrder(mk('a', { order: 0 }))).toBe(false);
    expect(hasOrder(mk('a', { order: NaN }))).toBe(false);
    expect(hasOrder(mk('a', { order: 128 }))).toBe(true);
    expect(hasOrder(mk('a', { order: -5 }))).toBe(true);
  });
});

describe('effectiveKeys', () => {
  it('spaces floating cards by ORDER_GAP in default order', () => {
    const cards = [mk('a', { priority: 'low' }), mk('b', { priority: 'critical' })];
    const keys = effectiveKeys(cards);
    expect(keys.get('b')).toBe(ORDER_GAP); // critical sorts first → slot 1
    expect(keys.get('a')).toBe(2 * ORDER_GAP);
  });
  it('keeps a placed card at its persisted order', () => {
    const keys = effectiveKeys([mk('a', { order: 99 }), mk('b')]);
    expect(keys.get('a')).toBe(99);
    expect(keys.get('b')).toBe(ORDER_GAP);
  });
});

describe('sortByEffective', () => {
  it('interleaves a placed card with floating neighbors by its order', () => {
    // Floating a,b,c get 65536/131072/196608; place d at 98304 (between a and b).
    const cards = [mk('a'), mk('b'), mk('c'), mk('d', { order: 98304, updated: '2020-01-01T00:00:00Z' })];
    expect(sortByEffective(cards).map((t) => t.id)).toEqual(['a', 'd', 'b', 'c']);
  });
  it('does not mutate the input array', () => {
    const cards = [mk('b', { priority: 'low' }), mk('a', { priority: 'critical' })];
    const copy = [...cards];
    sortByEffective(cards);
    expect(cards).toEqual(copy);
  });
});

describe('computeDropOrder', () => {
  it('returns a midpoint between two neighbors', () => {
    expect(computeDropOrder(100, 200)).toBe(150);
  });
  it('steps below the first card when dropped at the top', () => {
    expect(computeDropOrder(undefined, 100)).toBe(100 - ORDER_GAP / 2);
  });
  it('steps above the last card when dropped at the bottom', () => {
    expect(computeDropOrder(100, undefined)).toBe(100 + ORDER_GAP);
  });
  it('returns a base value for an empty column', () => {
    expect(computeDropOrder(undefined, undefined)).toBe(ORDER_GAP);
  });

  // Regression: 0 is the cross-stack "unset" sentinel, so a drop must never
  // yield exactly 0 (else the card re-floats and jumps to the bottom).
  it('never returns exactly 0', () => {
    expect(computeDropOrder(undefined, ORDER_GAP / 2)).not.toBe(0); // top-drop onto key 32768
    expect(computeDropOrder(-100, 100)).not.toBe(0); // symmetric midpoint
    expect(computeDropOrder(-ORDER_GAP, undefined)).not.toBe(0); // bottom-drop from -GAP
    // Two successive drag-to-top gestures must not collide on 0.
    const first = computeDropOrder(undefined, ORDER_GAP); // → 32768
    const second = computeDropOrder(undefined, first); // was 0 before the fix
    expect(second).not.toBe(0);
    expect(second).toBeLessThan(first);
  });
});

describe('dropOrderInColumn', () => {
  it('returns null for a drop-in-place / cancelled drag (no churn)', () => {
    const full = [mk('a'), mk('b'), mk('c')];
    // b dropped back under its existing upper neighbor a → unchanged.
    expect(dropOrderInColumn(full, 'b', 'a')).toBeNull();
  });

  it('computes an order for a real within-column reorder', () => {
    const full = [mk('a'), mk('b'), mk('c')];
    const order = dropOrderInColumn(full, 'c', null); // move c to the top
    expect(order).toBe(ORDER_GAP - ORDER_GAP / 2); // below a(65536)
  });

  it('places a card against the FULL column, not a filtered subset', () => {
    // Full column: a(floating), H(placed at 98304, hidden by a filter), b(floating).
    // Mover m sits at the bottom; the user (seeing only a, b, m) drops m under a.
    const full = [mk('a'), mk('H', { order: 98304 }), mk('b'), mk('m')];
    const order = dropOrderInColumn(full, 'm', 'a') as number;
    // m lands between a(65536) and H(98304) — never colliding with the hidden H.
    expect(order).toBeGreaterThan(ORDER_GAP);
    expect(order).toBeLessThan(98304);
    expect(order).not.toBe(98304);
  });

  it('computes an order for a cross-column move (target column excludes the card)', () => {
    const target = [mk('x'), mk('y')]; // target column, does not contain the mover
    const order = dropOrderInColumn(target, 'z', 'x') as number;
    expect(order).toBe((ORDER_GAP + 2 * ORDER_GAP) / 2); // between x(65536) and y(131072)
  });

  it('handles a cross-column move into an empty column', () => {
    expect(dropOrderInColumn([], 'z', null)).toBe(ORDER_GAP);
  });
});

describe('dropOrderFor', () => {
  it('places a card exactly between its new neighbors with one value', () => {
    // Column rendered a,b,c,d (all floating). User drags d between a and b.
    const arranged = [mk('a'), mk('d'), mk('b'), mk('c')];
    const order = dropOrderFor(arranged, 'd');
    // Neighbors a(65536) and b(131072) → midpoint 98304.
    expect(order).toBe((ORDER_GAP + 2 * ORDER_GAP) / 2);

    // Re-render the full column with d now carrying that order → arrangement holds.
    const placed = [mk('a'), mk('b'), mk('c'), mk('d', { order: order as number })];
    expect(sortByEffective(placed).map((t) => t.id)).toEqual(['a', 'd', 'b', 'c']);
  });
  it('handles a drop at the top of the column', () => {
    const arranged = [mk('c'), mk('a'), mk('b')];
    const order = dropOrderFor(arranged, 'c');
    expect(order).toBe(ORDER_GAP - ORDER_GAP / 2); // below a(65536)
  });
  it('returns null when the id is absent (source zone of a cross-column drag)', () => {
    expect(dropOrderFor([mk('a'), mk('b')], 'zzz')).toBeNull();
  });
});

describe('defaultCompare date ordering', () => {
  it('orders ISO timestamps newest first with a plain string compare', () => {
    const a = mk('a', { updated: '2026-07-04T10:00:00Z' });
    const b = mk('b', { updated: '2026-07-04T10:00:01Z' });
    expect(defaultCompare(a, b)).toBeGreaterThan(0);
    expect(defaultCompare(b, a)).toBeLessThan(0);
    // A missing timestamp sorts last.
    expect(defaultCompare(mk('c', { updated: '' }), a)).toBeGreaterThan(0);
  });
});

describe('capColumn', () => {
  const cards = (n: number) => Array.from({ length: n }, (_, i) => mk('t-' + i));

  it('renders everything when the column fits one page', () => {
    const all = cards(COLUMN_PAGE);
    const s = capColumn(all);
    expect(s.items).toBe(all);
    expect(s.total).toBe(COLUMN_PAGE);
    expect(hiddenCount(s)).toBe(0);
  });

  it('keeps the first page and reports the full total', () => {
    const all = cards(120);
    const s = capColumn(all);
    expect(s.items.map((t) => t.id)).toEqual(all.slice(0, COLUMN_PAGE).map((t) => t.id));
    expect(s.total).toBe(120);
    expect(hiddenCount(s)).toBe(120 - COLUMN_PAGE);
  });

  it('reveals one page at a time until nothing is hidden', () => {
    const all = cards(120);
    let limit = COLUMN_PAGE;
    limit = revealNext(limit);
    expect(capColumn(all, limit).items).toHaveLength(2 * COLUMN_PAGE);
    limit = revealNext(limit);
    const last = capColumn(all, limit);
    expect(last.items).toHaveLength(120);
    expect(hiddenCount(last)).toBe(0);
  });

  it('handles an empty column', () => {
    expect(capColumn([])).toEqual({ items: [], total: 0 });
  });
});

describe('dropping into a capped column', () => {
  // 120 floating cards, all in `todo`: the board renders only the first 50.
  const full = Array.from({ length: 120 }, (_, i) =>
    mk('t-' + String(i).padStart(3, '0'), { updated: `2026-07-04T${String(23 - Math.floor(i / 6)).padStart(2, '0')}:00:00Z` })
  );
  const sorted = sortByEffective(full);
  const visible = capColumn(sorted).items;

  it('slots a card dropped at the end of the visible page just above the first hidden card', () => {
    const lastVisible = visible[visible.length - 1];
    const firstHidden = sorted[visible.length];
    const moved = sorted[0];
    // The dndzone only knows the slice: the card now sits under the last visible card.
    const order = dropOrderInColumn(sorted, moved.id, lastVisible.id) as number;
    const keys = effectiveKeys(sorted.filter((t) => t.id !== moved.id));
    expect(order).toBeGreaterThan(keys.get(lastVisible.id)!);
    expect(order).toBeLessThan(keys.get(firstHidden.id)!);
  });

  it('keeps a card dropped at the top of the page above everything, hidden cards included', () => {
    const moved = sorted[10];
    const order = dropOrderInColumn(sorted, moved.id, null) as number;
    const keys = effectiveKeys(sorted.filter((t) => t.id !== moved.id));
    expect(order).toBeLessThan(Math.min(...keys.values()));
  });

  it('persists nothing when the dropped slice is unchanged (cancelled drag)', () => {
    const moved = visible[3];
    expect(dropOrderInColumn(sorted, moved.id, visible[2].id)).toBeNull();
  });

  it('lets a card from another column land inside the capped page', () => {
    const incoming = mk('other-1', { status: 'doing' });
    const order = dropOrderInColumn(sorted, incoming.id, visible[4].id) as number;
    const keys = effectiveKeys(sorted);
    expect(order).toBeGreaterThan(keys.get(visible[4].id)!);
    expect(order).toBeLessThan(keys.get(sorted[5].id)!);
  });
});

describe('sameRefs / reuseColumns', () => {
  const t1 = mk('a');
  const t2 = mk('b');

  function col(status: string, items: Ticket[], extra: Partial<BoardColumn> = {}): BoardColumn {
    return { status, title: status, total: items.length, items, ...extra };
  }

  it('sameRefs compares identity and order, not content', () => {
    expect(sameRefs([t1, t2], [t1, t2])).toBe(true);
    expect(sameRefs([t1, t2], [t2, t1])).toBe(false);
    expect(sameRefs([t1], [{ ...t1 }])).toBe(false);
    expect(sameRefs([t1], [t1, t2])).toBe(false);
  });

  it('reuses an unchanged column and replaces only the changed one', () => {
    const prev = [col('todo', [t1]), col('doing', [t2])];
    const next = [col('todo', [t1]), col('doing', [t2, mk('c')])];
    const out = reuseColumns(prev, next);
    expect(out[0]).toBe(prev[0]); // same object → dndzone is not reconfigured
    expect(out[1]).toBe(next[1]);
  });

  it('does not reuse a column whose ticket object was replaced', () => {
    const prev = [col('todo', [t1])];
    const next = [col('todo', [{ ...t1, title: 'renamed' }])];
    expect(reuseColumns(prev, next)[0]).toBe(next[0]);
  });

  it('does not reuse when the count or title changed even if cards are the same', () => {
    const prev = [col('todo', [t1])];
    expect(reuseColumns(prev, [col('todo', [t1], { total: 9 })])[0].total).toBe(9);
    expect(reuseColumns(prev, [col('todo', [t1], { title: 'Backlog' })])[0].title).toBe('Backlog');
  });

  it('matches by status, so a reordered or new column still reuses its twin', () => {
    const prev = [col('todo', [t1]), col('doing', [t2])];
    const next = [col('doing', [t2]), col('todo', [t1]), col('done', [])];
    const out = reuseColumns(prev, next);
    expect(out[0]).toBe(prev[1]);
    expect(out[1]).toBe(prev[0]);
    expect(out[2]).toBe(next[2]);
  });
});
