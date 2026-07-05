import type { Ticket } from './api';

export interface NeighborRef {
  id: string;
  title: string;
  status: string;
  priority: string;
  unmet?: boolean;
  inCycle?: boolean;
}

export interface Neighborhood {
  parent?: NeighborRef;
  blockers: NeighborRef[];
  dependents: NeighborRef[];
  children: NeighborRef[];
  dangling: string[];
  truncated: { blockers: number; dependents: number; children: number };
}

function toRef(t: Ticket, extra: Partial<NeighborRef> = {}): NeighborRef {
  return { id: t.id, title: t.title, status: t.status, priority: t.priority, inCycle: t.inCycle, ...extra };
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
      ? ticket.children.map((c) => ({ id: c.id, title: c.title, status: c.status, priority: '' }))
      : Object.values(all).filter((t) => t.parent === ticket.id).map((t) => toRef(t));

  const parentT = ticket.parent ? all[ticket.parent] : undefined;

  return {
    parent: parentT ? toRef(parentT) : undefined,
    blockers: blockersAll.slice(0, cap),
    dependents: dependentsAll.slice(0, cap),
    children: childrenAll.slice(0, cap),
    dangling: ticket.dangling ?? [],
    truncated: {
      blockers: Math.max(0, blockersAll.length - cap),
      dependents: Math.max(0, dependentsAll.length - cap),
      children: Math.max(0, childrenAll.length - cap)
    }
  };
}
