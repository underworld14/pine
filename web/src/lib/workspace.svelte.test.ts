import { describe, it, expect, vi, afterEach } from 'vitest';
import { flushSync } from 'svelte';
import { workspace } from './workspace.svelte';
import { api, type Ticket } from './api';

function mk(id: string, status: string, extra: Partial<Ticket> = {}): Ticket {
  return {
    id, type: id.split('-')[0], title: id, status, priority: 'medium',
    labels: [], deps: [], created: '2026-07-04T00:00:00Z', updated: '2026-07-04T00:00:00Z',
    blocked: false, hash: 'h-' + id, attachments: [], ...extra
  };
}

function withEffects(fn: () => void) {
  const cleanup = $effect.root(() => { fn(); });
  cleanup();
}

describe('workspace columns', () => {
  it('places tickets by frontmatter status and surfaces unmapped ones', () => {
    withEffects(() => {
      workspace.board = { columns: [{ status: 'todo', title: 'Todo' }, { status: 'done', title: 'Done' }], unmapped: [] };
      workspace.tickets = { 'BUG-1': mk('BUG-1', 'todo'), 'BUG-2': mk('BUG-2', 'weird') };
      flushSync();
      const cols = workspace.columns;
      const todo = cols.find((c) => c.status === 'todo');
      expect(todo?.tickets.map((t) => t.id)).toContain('BUG-1');
      const other = cols.find((c) => c.title.startsWith('Other'));
      expect(other?.tickets.map((t) => t.id)).toContain('BUG-2');
    });
  });

  it('places a status-less ticket into the first column', () => {
    withEffects(() => {
      workspace.board = { columns: [{ status: 'todo', title: 'Todo' }, { status: 'done', title: 'Done' }], unmapped: [] };
      workspace.tickets = { x: mk('x', '') };
      flushSync();
      const todo = workspace.columns.find((c) => c.status === 'todo');
      expect(todo?.tickets.map((t) => t.id)).toContain('x');
    });
  });

  it('sorts a column by priority then recency', () => {
    withEffects(() => {
      workspace.board = { columns: [{ status: 'todo', title: 'Todo' }], unmapped: [] };
      workspace.tickets = {
        a: mk('a', 'todo', { priority: 'low', updated: '2026-07-04T10:00:00Z' }),
        b: mk('b', 'todo', { priority: 'critical', updated: '2026-07-04T09:00:00Z' })
      };
      flushSync();
      const todo = workspace.columns[0];
      expect(todo.tickets[0].id).toBe('b'); // critical outranks low
    });
  });

  it('honors a persisted manual order over the default sort', () => {
    withEffects(() => {
      workspace.board = { columns: [{ status: 'todo', title: 'Todo' }], unmapped: [] };
      // a is low priority but manually placed at the top; b is critical (floats).
      workspace.tickets = {
        a: mk('a', 'todo', { priority: 'low', order: 10 }),
        b: mk('b', 'todo', { priority: 'critical' })
      };
      flushSync();
      expect(workspace.columns[0].tickets.map((t) => t.id)).toEqual(['a', 'b']);
    });
  });
});

describe('reorder', () => {
  afterEach(() => vi.restoreAllMocks());

  it('optimistically applies order + status then reconciles from the server', async () => {
    workspace.tickets = { a: mk('a', 'todo', { hash: 'h1' }) };
    const spy = vi.spyOn(api, 'patchTicket').mockResolvedValue(
      mk('a', 'doing', { order: 150, hash: 'h2' })
    );
    await workspace.reorder('a', { status: 'doing', order: 150 });
    expect(spy).toHaveBeenCalledWith('a', expect.objectContaining({ status: 'doing', order: 150 }), 'h1');
    expect(workspace.tickets['a'].order).toBe(150);
    expect(workspace.tickets['a'].hash).toBe('h2');
  });

  it('reverts on failure when no external update landed', async () => {
    workspace.tickets = { a: mk('a', 'todo', { order: 0, hash: 'h1' }) };
    vi.spyOn(api, 'patchTicket').mockRejectedValue(new Error('boom'));
    await expect(workspace.reorder('a', { order: 500 })).rejects.toThrow('boom');
    expect(workspace.tickets['a'].order ?? 0).toBe(0); // rolled back
    expect(workspace.tickets['a'].status).toBe('todo');
  });

  it('is a no-op for a read-only (off-branch) card', async () => {
    workspace.tickets = { a: mk('a', 'todo', { readOnly: true }) };
    const spy = vi.spyOn(api, 'patchTicket');
    await workspace.reorder('a', { order: 500 });
    expect(spy).not.toHaveBeenCalled();
  });
});

describe('applyEvent', () => {
  it('applies an external ticket update and flashes it', () => {
    withEffects(() => {
      workspace.tickets = {};
      workspace.applyEvent({ type: 'ticket.updated', seq: 1, origin: { source: 'fs' }, ticket: mk('BUG-9', 'doing') });
      flushSync();
      expect(workspace.tickets['BUG-9'].status).toBe('doing');
      expect(workspace.flashing['BUG-9']).toBeGreaterThan(0);
    });
  });

  it('removes a deleted ticket', () => {
    withEffects(() => {
      workspace.tickets = { 'BUG-9': mk('BUG-9', 'todo') };
      workspace.applyEvent({ type: 'ticket.deleted', seq: 2, origin: { source: 'fs' }, id: 'BUG-9' });
      flushSync();
      expect(workspace.tickets['BUG-9']).toBeUndefined();
    });
  });
});

function snapshotOf(tickets: Ticket[]) {
  return {
    tickets,
    board: { columns: [{ status: 'todo', title: 'Todo' }], unmapped: [] },
    config: { project: { name: 'x' }, types: [], priorities: [] },
    git: { isRepo: false, branch: '', dirty: false, changes: [], truncated: false, commits: [] },
    seq: 1
  };
}

function deferred<T>() {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej; });
  return { promise, resolve, reject };
}

describe('hydrate', () => {
  afterEach(() => {
    vi.restoreAllMocks();
    workspace.tickets = {};
  });

  function stubLoad(tickets: Ticket[]) {
    const snap = vi.spyOn(api, 'snapshot').mockResolvedValue(snapshotOf(tickets));
    vi.spyOn(api, 'repos').mockResolvedValue({ active: 'default', repos: [] });
    return snap;
  }

  it('keeps an already-loaded body while the hash is unchanged', async () => {
    workspace.tickets = { a: mk('a', 'todo', { hash: 'h1', body: 'loaded' }) };
    stubLoad([mk('a', 'todo', { hash: 'h1' })]); // snapshot omits the body
    await workspace.hydrate();
    expect(workspace.tickets['a'].body).toBe('loaded');
  });

  it('drops a loaded body whose hash changed (it is stale)', async () => {
    workspace.tickets = { a: mk('a', 'todo', { hash: 'h1', body: 'old' }) };
    stubLoad([mk('a', 'todo', { hash: 'h2' })]);
    await workspace.hydrate();
    expect(workspace.tickets['a'].body).toBeUndefined();
    expect(workspace.tickets['a'].hash).toBe('h2');
  });

  it('keeps a body the snapshot itself carries (off-branch tickets)', async () => {
    workspace.tickets = {};
    stubLoad([mk('b', 'todo', { readOnly: true, body: 'from snapshot' })]);
    await workspace.hydrate();
    expect(workspace.tickets['b'].body).toBe('from snapshot');
  });

  it('requests the snapshot without bodies', async () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify(snapshotOf([])), { status: 200 })
    );
    await api.snapshot();
    expect(fetchSpy.mock.calls[0][0]).toBe('/api/snapshot?bodies=0');
  });

  it('coalesces overlapping calls into one request', async () => {
    const d = deferred<ReturnType<typeof snapshotOf>>();
    const snap = vi.spyOn(api, 'snapshot').mockReturnValue(d.promise);
    vi.spyOn(api, 'repos').mockResolvedValue({ active: 'default', repos: [] });
    const p1 = workspace.hydrate();
    const p2 = workspace.hydrate();
    expect(p2).toBe(p1);
    d.resolve(snapshotOf([mk('a', 'todo')]));
    await Promise.all([p1, p2]);
    expect(snap).toHaveBeenCalledTimes(1);

    // Once settled, the next call is a fresh request.
    await workspace.hydrate();
    expect(snap).toHaveBeenCalledTimes(2);
  });

  it('lets a failed load be retried', async () => {
    const snap = vi.spyOn(api, 'snapshot').mockRejectedValueOnce(new Error('down'));
    vi.spyOn(api, 'repos').mockResolvedValue({ active: 'default', repos: [] });
    await expect(workspace.hydrate()).rejects.toThrow('down');
    snap.mockResolvedValue(snapshotOf([]));
    await expect(workspace.hydrate()).resolves.toBeUndefined();
  });
});

describe('ensureBody', () => {
  afterEach(() => {
    vi.restoreAllMocks();
    workspace.tickets = {};
  });

  it('fetches and attaches the body of a body-less ticket', async () => {
    workspace.tickets = { a: mk('a', 'todo', { hash: 'h1' }) };
    const get = vi.spyOn(api, 'getTicket').mockResolvedValue(mk('a', 'todo', { hash: 'h1', body: 'hello' }));
    await workspace.ensureBody('a');
    expect(get).toHaveBeenCalledWith('a');
    expect(workspace.tickets['a'].body).toBe('hello');
  });

  it('is a no-op for missing, read-only, or already-loaded tickets', async () => {
    workspace.tickets = {
      ro: mk('ro', 'todo', { readOnly: true }),
      loaded: mk('loaded', 'todo', { body: '' }) // empty string is still loaded
    };
    const get = vi.spyOn(api, 'getTicket');
    await workspace.ensureBody('nope');
    await workspace.ensureBody('ro');
    await workspace.ensureBody('loaded');
    expect(get).not.toHaveBeenCalled();
  });

  it('coalesces concurrent calls for the same id', async () => {
    workspace.tickets = { a: mk('a', 'todo', { hash: 'h1' }) };
    const d = deferred<Ticket>();
    const get = vi.spyOn(api, 'getTicket').mockReturnValue(d.promise);
    const p1 = workspace.ensureBody('a');
    const p2 = workspace.ensureBody('a');
    d.resolve(mk('a', 'todo', { hash: 'h1', body: 'x' }));
    await Promise.all([p1, p2]);
    expect(get).toHaveBeenCalledTimes(1);
  });

  it('reads an omitted (empty) body from the server as ""', async () => {
    workspace.tickets = { a: mk('a', 'todo', { hash: 'h1' }) };
    vi.spyOn(api, 'getTicket').mockResolvedValue(mk('a', 'todo', { hash: 'h1' }));
    await workspace.ensureBody('a');
    expect(workspace.tickets['a'].body).toBe('');
  });

  it('never overwrites a newer SSE copy with an older fetch', async () => {
    workspace.tickets = { a: mk('a', 'todo', { hash: 'h1' }) };
    const d = deferred<Ticket>();
    vi.spyOn(api, 'getTicket').mockReturnValue(d.promise);
    const p = workspace.ensureBody('a');
    // A newer version (with its body) lands over SSE while the GET is in flight.
    workspace.applyEvent({ type: 'ticket.updated', seq: 1, origin: { source: 'fs' }, ticket: mk('a', 'todo', { hash: 'h2', body: 'newer' }) });
    d.resolve(mk('a', 'todo', { hash: 'h1', body: 'older' }));
    await p;
    expect(workspace.tickets['a'].hash).toBe('h2');
    expect(workspace.tickets['a'].body).toBe('newer');
  });

  it('adopts the fetched version when the body-less copy is outdated', async () => {
    workspace.tickets = { a: mk('a', 'todo', { hash: 'h1' }) };
    vi.spyOn(api, 'getTicket').mockResolvedValue(mk('a', 'todo', { hash: 'h2', body: 'fresh' }));
    await workspace.ensureBody('a');
    // Body and hash stay consistent so a later save's If-Match is truthful.
    expect(workspace.tickets['a'].hash).toBe('h2');
    expect(workspace.tickets['a'].body).toBe('fresh');
  });

  it('drops the result when the ticket was deleted meanwhile', async () => {
    workspace.tickets = { a: mk('a', 'todo') };
    const d = deferred<Ticket>();
    vi.spyOn(api, 'getTicket').mockReturnValue(d.promise);
    const p = workspace.ensureBody('a');
    workspace.applyEvent({ type: 'ticket.deleted', seq: 2, origin: { source: 'fs' }, id: 'a' });
    d.resolve(mk('a', 'todo', { body: 'zombie' }));
    await p;
    expect(workspace.tickets['a']).toBeUndefined();
  });

  it('rejects when the fetch fails so callers can show an error', async () => {
    workspace.tickets = { a: mk('a', 'todo') };
    vi.spyOn(api, 'getTicket').mockRejectedValue(new Error('404'));
    await expect(workspace.ensureBody('a')).rejects.toThrow('404');
    // A later call retries (the failed promise is not cached).
    const get = vi.spyOn(api, 'getTicket').mockResolvedValue(mk('a', 'todo', { body: 'ok' }));
    await workspace.ensureBody('a');
    expect(get).toHaveBeenCalled();
    expect(workspace.tickets['a'].body).toBe('ok');
  });
});

describe('full-ticket sources treat an omitted body as empty', () => {
  afterEach(() => {
    vi.restoreAllMocks();
    workspace.tickets = {};
  });

  it('SSE events and PATCH responses never leave a body-less (unloaded) ticket', async () => {
    workspace.applyEvent({ type: 'ticket.created', seq: 1, origin: { source: 'fs' }, ticket: mk('a', 'todo') });
    expect(workspace.tickets['a'].body).toBe('');
    vi.spyOn(api, 'patchTicket').mockResolvedValue(mk('a', 'todo', { hash: 'h2' }));
    await workspace.patch('a', { title: 'x' });
    expect(workspace.tickets['a'].body).toBe('');
  });

  it('leaves off-branch tickets alone', () => {
    workspace.applyEvent({ type: 'ticket.updated', seq: 1, origin: { source: 'fs' }, ticket: mk('a', 'todo', { readOnly: true }) });
    expect(workspace.tickets['a'].body).toBeUndefined();
  });
});

describe('startLive', () => {
  class FakeES {
    static instances: FakeES[] = [];
    onopen: (() => void) | null = null;
    onerror: (() => void) | null = null;
    constructor(public url: string) { FakeES.instances.push(this); }
    addEventListener() {}
    close() {}
  }

  function stubLoad() {
    const snap = vi.spyOn(api, 'snapshot').mockResolvedValue(snapshotOf([]));
    vi.spyOn(api, 'repos').mockResolvedValue({ active: 'default', repos: [] });
    return snap;
  }

  afterEach(() => {
    workspace.stopLive();
    workspace.hydrated = false;
    vi.restoreAllMocks();
    vi.useRealTimers();
    vi.unstubAllGlobals();
    FakeES.instances = [];
  });

  it('issues a single snapshot on a cold start (stream open is the only trigger)', async () => {
    vi.useFakeTimers();
    vi.stubGlobal('EventSource', FakeES);
    workspace.hydrated = false;
    const snap = stubLoad();
    workspace.startLive();
    expect(snap).not.toHaveBeenCalled(); // nothing loads before the stream opens
    FakeES.instances[0].onopen!();
    await vi.advanceTimersByTimeAsync(5000);
    expect(snap).toHaveBeenCalledTimes(1);
    expect(workspace.hydrated).toBe(true);
    expect(workspace.connection).toBe('live');
  });

  it('re-hydrates on every later reconnect', async () => {
    vi.useFakeTimers();
    vi.stubGlobal('EventSource', FakeES);
    workspace.hydrated = false;
    const snap = stubLoad();
    workspace.startLive();
    FakeES.instances[0].onopen!();
    await vi.advanceTimersByTimeAsync(10);
    FakeES.instances[0].onerror!(); // stream drops; hydrated, so no fallback load
    expect(workspace.connection).toBe('down');
    await vi.advanceTimersByTimeAsync(10_000); // backoff elapses, a new stream connects
    expect(FakeES.instances.length).toBe(2);
    FakeES.instances[1].onopen!();
    await vi.advanceTimersByTimeAsync(10);
    expect(snap).toHaveBeenCalledTimes(2);
  });

  it('falls back to a direct load when the stream fails before the first hydrate', async () => {
    vi.useFakeTimers();
    vi.stubGlobal('EventSource', FakeES);
    workspace.hydrated = false;
    const snap = stubLoad();
    workspace.startLive();
    FakeES.instances[0].onerror!();
    await vi.advanceTimersByTimeAsync(10);
    expect(snap).toHaveBeenCalledTimes(1);
    expect(workspace.hydrated).toBe(true);
  });

  it('falls back to a direct load when the stream stays silent', async () => {
    vi.useFakeTimers();
    vi.stubGlobal('EventSource', FakeES);
    workspace.hydrated = false;
    const snap = stubLoad();
    workspace.startLive();
    await vi.advanceTimersByTimeAsync(2000);
    expect(snap).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(2000);
    expect(snap).toHaveBeenCalledTimes(1);
  });

  it('does not fall back once the stream opened and hydrated', async () => {
    vi.useFakeTimers();
    vi.stubGlobal('EventSource', FakeES);
    workspace.hydrated = false;
    const snap = stubLoad();
    workspace.startLive();
    FakeES.instances[0].onopen!();
    await vi.advanceTimersByTimeAsync(10_000);
    expect(snap).toHaveBeenCalledTimes(1);
  });
});

describe('workspace columns (single pass)', () => {
  it('groups every unmapped status in first-seen order, after the mapped columns', () => {
    withEffects(() => {
      workspace.board = { columns: [{ status: 'todo', title: 'Todo' }, { status: 'done', title: 'Done' }], unmapped: [] };
      workspace.tickets = {
        a: mk('a', 'weird'), b: mk('b', 'done'), c: mk('c', 'odd'), d: mk('d', 'weird'), e: mk('e', 'todo')
      };
      flushSync();
      const cols = workspace.columns;
      expect(cols.map((c) => c.title)).toEqual(['Todo', 'Done', 'Other: weird', 'Other: odd']);
      expect(cols[2].tickets.map((t) => t.id).sort()).toEqual(['a', 'd']);
      expect(cols[1].tickets.map((t) => t.id)).toEqual(['b']);
    });
  });

  it('does not place status-less tickets anywhere when there are no columns', () => {
    withEffects(() => {
      workspace.board = { columns: [], unmapped: [] };
      workspace.tickets = { x: mk('x', ''), y: mk('y', 'weird') };
      flushSync();
      expect(workspace.columns.map((c) => c.title)).toEqual(['Other: weird']);
    });
  });
});
