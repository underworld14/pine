import {
  forceCenter,
  forceCollide,
  forceLink,
  forceManyBody,
  forceSimulation,
  type Simulation,
  type SimulationLinkDatum,
  type SimulationNodeDatum
} from 'd3-force';
import type { GraphEdge, GraphNode, GraphNodeKind } from './api';

export type Pos = { x: number; y: number };

export interface ForceNode extends SimulationNodeDatum, GraphNode {
  id: string;
}

export interface ForceLink extends SimulationLinkDatum<ForceNode> {
  kind: GraphEdge['kind'];
}

const CX = 450;
const CY = 320;
const NODE_GAP = 48;

const RING_RADII: Record<GraphNodeKind, number> = {
  ticket: 0,
  learning: 160,
  topic: 280,
  memory: 380,
  unknown: 420
};

/** Deterministic ring/grid seed so reload isn't chaotic. */
export function seedPositions(nodes: GraphNode[]): Record<string, Pos> {
  const byKind: Record<string, GraphNode[]> = {
    ticket: [],
    learning: [],
    topic: [],
    memory: [],
    unknown: []
  };
  for (const n of nodes) {
    (byKind[n.kind] ?? byKind.unknown).push(n);
  }
  for (const k of Object.keys(byKind)) {
    byKind[k].sort((a, b) => a.id.localeCompare(b.id));
  }

  const out: Record<string, Pos> = {};
  const tickets = byKind.ticket;
  if (tickets.length <= 1) {
    if (tickets[0]) out[tickets[0].id] = { x: CX, y: CY };
  } else {
    const cols = Math.ceil(Math.sqrt(tickets.length));
    const rows = Math.ceil(tickets.length / cols);
    const totalW = (cols - 1) * NODE_GAP;
    const totalH = (rows - 1) * NODE_GAP;
    tickets.forEach((n, i) => {
      const col = i % cols;
      const row = Math.floor(i / cols);
      out[n.id] = {
        x: CX - totalW / 2 + col * NODE_GAP,
        y: CY - totalH / 2 + row * NODE_GAP
      };
    });
  }

  for (const kind of ['learning', 'topic', 'memory', 'unknown'] as GraphNodeKind[]) {
    const list = byKind[kind] ?? [];
    const r = RING_RADII[kind] ?? 400;
    list.forEach((n, i) => {
      const angle = (2 * Math.PI * i) / Math.max(list.length, 1) - Math.PI / 2;
      out[n.id] = {
        x: CX + Math.cos(angle) * r,
        y: CY + Math.sin(angle) * r
      };
    });
  }
  return out;
}

/** Undirected adjacency for hover focus. */
export function buildAdjacency(
  edges: { source: string; target: string }[]
): Map<string, Set<string>> {
  const adj = new Map<string, Set<string>>();
  const add = (a: string, b: string) => {
    if (!adj.has(a)) adj.set(a, new Set());
    adj.get(a)!.add(b);
  };
  for (const e of edges) {
    add(e.source, e.target);
    add(e.target, e.source);
  }
  return adj;
}

/** Self + one-hop neighbors, or null when nothing is hovered. */
export function focusSet(
  nodeId: string | null,
  adj: Map<string, Set<string>>
): Set<string> | null {
  if (!nodeId) return null;
  const set = new Set<string>([nodeId]);
  for (const n of adj.get(nodeId) ?? []) set.add(n);
  return set;
}

export interface GraphSimOptions {
  reducedMotion?: boolean;
  onTick?: (positions: Record<string, Pos>) => void;
  /** Silent ticks when reducedMotion (default 80). */
  settleTicks?: number;
}

export interface GraphSimulation {
  simulation: Simulation<ForceNode, ForceLink>;
  nodes: ForceNode[];
  links: ForceLink[];
  getPositions: () => Record<string, Pos>;
  tick: () => void;
  stop: () => void;
  reheat: () => void;
}

function positionsFrom(nodes: ForceNode[]): Record<string, Pos> {
  const out: Record<string, Pos> = {};
  for (const n of nodes) {
    out[n.id] = { x: n.x ?? 0, y: n.y ?? 0 };
  }
  return out;
}

export function createGraphSimulation(
  graphNodes: GraphNode[],
  graphEdges: GraphEdge[],
  opts: GraphSimOptions = {}
): GraphSimulation {
  const seed = seedPositions(graphNodes);
  const nodes: ForceNode[] = graphNodes.map((n) => ({
    ...n,
    x: seed[n.id]?.x ?? CX,
    y: seed[n.id]?.y ?? CY
  }));

  const idSet = new Set(nodes.map((n) => n.id));
  const links: ForceLink[] = graphEdges
    .filter((e) => idSet.has(e.source) && idSet.has(e.target))
    .map((e) => ({ source: e.source, target: e.target, kind: e.kind }));

  const simulation = forceSimulation<ForceNode, ForceLink>(nodes)
    .force(
      'link',
      forceLink<ForceNode, ForceLink>(links)
        .id((d) => d.id)
        .distance(90)
        .strength(0.45)
    )
    .force('charge', forceManyBody<ForceNode>().strength(-220))
    .force('center', forceCenter(CX, CY).strength(0.05))
    .force('collide', forceCollide<ForceNode>().radius(22).strength(0.8))
    .alphaDecay(0.028)
    .velocityDecay(0.35);

  const emit = () => opts.onTick?.(positionsFrom(nodes));

  simulation.on('tick', emit);

  const api: GraphSimulation = {
    simulation,
    nodes,
    links,
    getPositions: () => positionsFrom(nodes),
    tick: () => {
      simulation.tick();
      emit();
    },
    stop: () => {
      simulation.on('tick', null);
      simulation.stop();
    },
    reheat: () => {
      simulation.alpha(0.4);
      if (opts.reducedMotion) {
        // Stay timer-stopped: silent ticks only (respect prefers-reduced-motion).
        const n = Math.min(opts.settleTicks ?? 80, 40);
        for (let i = 0; i < n; i++) simulation.tick();
        emit();
        return;
      }
      simulation.restart();
    }
  };

  if (opts.reducedMotion) {
    simulation.stop();
    const n = opts.settleTicks ?? 80;
    for (let i = 0; i < n; i++) simulation.tick();
    emit();
  }

  return api;
}
