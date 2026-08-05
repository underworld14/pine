import type { Ticket } from './api';

export interface NeighborRef {
  id: string;
  title: string;
  status: string;
  priority: string;
  unmet?: boolean;
  inCycle?: boolean;
  kind?: 'ticket' | 'memory' | 'learning' | 'topic';
}

export interface Neighborhood {
  parent?: NeighborRef;
  blockers: NeighborRef[];
  dependents: NeighborRef[];
  children: NeighborRef[];
  memory: NeighborRef[];
  dangling: string[];
  truncated: { blockers: number; dependents: number; children: number; memory: number };
}

function toRef(t: Ticket, extra: Partial<NeighborRef> = {}): NeighborRef {
  return { id: t.id, title: t.title, status: t.status, priority: t.priority, inCycle: t.inCycle, kind: 'ticket', ...extra };
}

function linkKind(ref: string): NeighborRef['kind'] {
  if (ref === 'MEMORY' || ref === 'MEMORY.md') return 'memory';
  if (ref.startsWith('memory/')) return 'topic';
  if (ref.toUpperCase().startsWith('LRN-')) return 'learning';
  return 'ticket';
}

function linkTitle(ref: string): string {
  if (ref.startsWith('memory/')) return ref.slice('memory/'.length);
  return ref;
}

export function neighborhood(ticket: Ticket, all: Record<string, Ticket>, cap = 6): Neighborhood {
  const unmet = new Set(ticket.unmet ?? []);

  const blockersAll = (ticket.deps ?? [])
    .filter((id) => all[id])
    .map((id) => toRef(all[id], { unmet: unmet.has(id) }));

  const dependentsAll = Object.values(all)
    .filter((t) => (t.deps ?? []).includes(ticket.id))
    .map((t) => toRef(t));

  const childrenAll: NeighborRef[] =
    ticket.children && ticket.children.length
      ? ticket.children.map((c) => ({ id: c.id, title: c.title, status: c.status, priority: '', kind: 'ticket' as const }))
      : Object.values(all).filter((t) => t.parent === ticket.id).map((t) => toRef(t));

  const parentT = ticket.parent ? all[ticket.parent] : undefined;

  const memoryAll: NeighborRef[] = (ticket.links ?? []).map((ref) => {
    const kind = linkKind(ref);
    if (kind === 'ticket' && all[ref]) {
      return toRef(all[ref]);
    }
    return {
      id: ref,
      title: linkTitle(ref),
      status: kind ?? 'topic',
      priority: '',
      kind
    };
  });

  return {
    parent: parentT ? toRef(parentT) : undefined,
    blockers: blockersAll.slice(0, cap),
    dependents: dependentsAll.slice(0, cap),
    children: childrenAll.slice(0, cap),
    memory: memoryAll.slice(0, cap),
    dangling: ticket.dangling ?? [],
    truncated: {
      blockers: Math.max(0, blockersAll.length - cap),
      dependents: Math.max(0, dependentsAll.length - cap),
      children: Math.max(0, childrenAll.length - cap),
      memory: Math.max(0, memoryAll.length - cap)
    }
  };
}
