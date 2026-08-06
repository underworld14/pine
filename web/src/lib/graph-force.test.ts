import { describe, it, expect } from 'vitest';
import type { GraphEdge, GraphNode } from './api';
import {
  buildAdjacency,
  createGraphSimulation,
  focusSet,
  seedPositions
} from './graph-force';

function node(p: Partial<GraphNode> & Pick<GraphNode, 'id' | 'kind'>): GraphNode {
  return { title: p.id, ...p };
}

describe('seedPositions', () => {
  it('places a single ticket at the center', () => {
    const pos = seedPositions([node({ id: 'BUG-1', kind: 'ticket' })]);
    expect(pos['BUG-1']).toEqual({ x: 450, y: 320 });
  });

  it('lays out multiple tickets in a deterministic grid', () => {
    const tickets = ['A', 'B', 'C', 'D'].map((id) => node({ id, kind: 'ticket' }));
    const pos = seedPositions(tickets);
    expect(Object.keys(pos).sort()).toEqual(['A', 'B', 'C', 'D']);
    // All distinct and roughly clustered near center.
    const xs = Object.values(pos).map((p) => p.x);
    const ys = Object.values(pos).map((p) => p.y);
    expect(new Set(xs).size).toBeGreaterThan(1);
    expect(Math.max(...xs) - Math.min(...xs)).toBeLessThan(400);
    expect(Math.max(...ys) - Math.min(...ys)).toBeLessThan(400);
  });

  it('places topics on a ring outside the ticket cluster', () => {
    const pos = seedPositions([
      node({ id: 'BUG-1', kind: 'ticket' }),
      node({ id: 'memory/web', kind: 'topic' })
    ]);
    const t = pos['BUG-1'];
    const topic = pos['memory/web'];
    const dist = Math.hypot(topic.x - t.x, topic.y - t.y);
    expect(dist).toBeGreaterThan(100);
  });
});

describe('buildAdjacency + focusSet', () => {
  const edges: GraphEdge[] = [
    { source: 'A', target: 'B', kind: 'dep' },
    { source: 'B', target: 'C', kind: 'parent' },
    { source: 'A', target: 'D', kind: 'link' }
  ];

  it('builds undirected adjacency', () => {
    const adj = buildAdjacency(edges);
    expect([...adj.get('A')!].sort()).toEqual(['B', 'D']);
    expect([...adj.get('B')!].sort()).toEqual(['A', 'C']);
    expect([...adj.get('C')!]).toEqual(['B']);
  });

  it('focusSet includes self and one-hop neighbors', () => {
    const adj = buildAdjacency(edges);
    expect(focusSet(null, adj)).toBeNull();
    expect([...focusSet('A', adj)!].sort()).toEqual(['A', 'B', 'D']);
    expect([...focusSet('C', adj)!].sort()).toEqual(['B', 'C']);
  });

  it('focusSet for isolated node is just itself', () => {
    const adj = buildAdjacency(edges);
    expect([...focusSet('Z', adj)!]).toEqual(['Z']);
  });
});

describe('createGraphSimulation', () => {
  const nodes: GraphNode[] = [
    node({ id: 'A', kind: 'ticket' }),
    node({ id: 'B', kind: 'ticket' }),
    node({ id: 'T', kind: 'topic', title: 'memory/web' })
  ];
  const edges: GraphEdge[] = [
    { source: 'A', target: 'B', kind: 'dep' },
    { source: 'A', target: 'T', kind: 'link' }
  ];

  it('seeds nodes and produces positions after silent ticks (reduced motion)', () => {
    const sim = createGraphSimulation(nodes, edges, { reducedMotion: true });
    const pos = sim.getPositions();
    expect(pos['A']).toBeDefined();
    expect(pos['B']).toBeDefined();
    expect(pos['T']).toBeDefined();
    expect(Number.isFinite(pos['A'].x)).toBe(true);
    expect(Number.isFinite(pos['A'].y)).toBe(true);
    sim.stop();
  });

  it('calls onTick at least once when not reduced-motion', async () => {
    let ticks = 0;
    const sim = createGraphSimulation(nodes, edges, {
      onTick: () => {
        ticks++;
      }
    });
    // Allow a couple animation frames / timer ticks.
    await new Promise((r) => setTimeout(r, 50));
    expect(ticks).toBeGreaterThan(0);
    sim.stop();
  });

  it('exposes node refs that can be pinned via fx/fy', () => {
    const sim = createGraphSimulation(nodes, edges, { reducedMotion: true });
    const n = sim.nodes.find((x) => x.id === 'A')!;
    n.fx = 10;
    n.fy = 20;
    sim.reheat();
    // After silent settle with pin, A stays near the pin.
    for (let i = 0; i < 20; i++) sim.tick();
    const pos = sim.getPositions();
    expect(pos['A'].x).toBeCloseTo(10, 0);
    expect(pos['A'].y).toBeCloseTo(20, 0);
    sim.stop();
  });

  it('reheat with reducedMotion settles synchronously without restarting the timer', async () => {
    let ticks = 0;
    const sim = createGraphSimulation(nodes, edges, {
      reducedMotion: true,
      onTick: () => {
        ticks++;
      }
    });
    const afterCreate = ticks;
    sim.reheat();
    const afterReheat = ticks;
    expect(afterReheat).toBeGreaterThan(afterCreate);
    await new Promise((r) => setTimeout(r, 50));
    expect(ticks).toBe(afterReheat);
    sim.stop();
  });
});
