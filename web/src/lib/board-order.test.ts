import { describe, it, expect } from 'vitest';
import {
  ORDER_GAP,
  defaultCompare,
  hasOrder,
  effectiveKeys,
  sortByEffective,
  computeDropOrder,
  dropOrderFor,
  dropOrderInColumn
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
