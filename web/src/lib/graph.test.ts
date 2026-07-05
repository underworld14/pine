import { describe, it, expect } from 'vitest';
import { neighborhood } from './graph';
import type { Ticket } from './api';

function tk(p: Partial<Ticket>): Ticket {
  return {
    id: '', type: 'BUG', title: '', status: 'todo', priority: 'medium',
    labels: [], deps: [], created: '', updated: '', blocked: false, hash: '', attachments: [], ...p
  } as Ticket;
}

describe('neighborhood', () => {
  it('resolves blockers (with unmet), dependents, parent, dangling', () => {
    const all: Record<string, Ticket> = {
      'BUG-1': tk({ id: 'BUG-1', deps: ['FEAT-2', 'GONE-9'], unmet: ['FEAT-2'], dangling: ['GONE-9'], parent: 'EPIC-3' }),
      'FEAT-2': tk({ id: 'FEAT-2', status: 'doing' }),
      'BUG-4': tk({ id: 'BUG-4', deps: ['BUG-1'] }),
      'EPIC-3': tk({ id: 'EPIC-3', type: 'EPIC' })
    };
    const n = neighborhood(all['BUG-1'], all);
    expect(n.blockers.map((b) => b.id)).toEqual(['FEAT-2']);
    expect(n.blockers[0].unmet).toBe(true);
    expect(n.dependents.map((d) => d.id)).toEqual(['BUG-4']);
    expect(n.parent?.id).toBe('EPIC-3');
    expect(n.dangling).toEqual(['GONE-9']);
  });

  it('resolves children by reverse scan when children[] is absent', () => {
    const all: Record<string, Ticket> = {
      'EPIC-3': tk({ id: 'EPIC-3', type: 'EPIC' }),
      'BUG-5': tk({ id: 'BUG-5', parent: 'EPIC-3' })
    };
    expect(neighborhood(all['EPIC-3'], all).children.map((c) => c.id)).toEqual(['BUG-5']);
  });

  it('caps each arm and reports truncation', () => {
    const all: Record<string, Ticket> = { 'BUG-1': tk({ id: 'BUG-1' }) };
    for (let i = 0; i < 8; i++) all[`D-${i}`] = tk({ id: `D-${i}`, deps: ['BUG-1'] });
    const n = neighborhood(all['BUG-1'], all, 6);
    expect(n.dependents.length).toBe(6);
    expect(n.truncated.dependents).toBe(2);
  });
});
