// The single client-side source of truth: tickets, board, config, git — hydrated
// from /api/snapshot and kept live by the SSE stream. Board columns are derived
// from each ticket's frontmatter status (self-healing), so an agent editing a
// file makes the card move without any board.json change.

import { api, PineEventSource, type Board, type Config, type GitStatus, type PineEvent, type RepoInfo, type Ticket } from './api';
import { sortByEffective } from './board-order';

type Conn = 'connecting' | 'live' | 'down';

// If the event stream hasn't opened (or failed) this long after start, load the
// snapshot directly so a stuck stream can't leave the board on its skeleton.
const FALLBACK_LOAD_MS = 3000;

export interface ColumnView {
  status: string;
  title: string;
  tickets: Ticket[];
}

function uuid(): string {
  return crypto.randomUUID ? crypto.randomUUID() : Math.random().toString(36).slice(2);
}

// fullTicket normalises a ticket from a full-ticket source (PATCH/SSE/GET): the
// server omits an empty body, which must read as "" there, not "not loaded yet".
// Off-branch tickets are left alone (they never lazy-load).
function fullTicket(t: Ticket): Ticket {
  return typeof t.body === 'string' || t.readOnly ? t : { ...t, body: '' };
}

// readOnlyMessage explains why an off-branch ticket cannot be edited from here.
export function readOnlyMessage(t: Ticket): string {
  const where = t.branch ? `branch "${t.branch}"` : 'another branch';
  return `${t.id} lives on ${where} and is read-only here. Check out that branch to edit it.`;
}

class WorkspaceStore {
  tickets = $state<Record<string, Ticket>>({});
  board = $state<Board | null>(null);
  config = $state<Config | null>(null);
  git = $state<GitStatus | null>(null);
  // Per-repo git cache: every registered repo's poller emits a repo-tagged
  // git.updated, so we keep the latest snapshot per repo and only promote it to
  // `git` for the active repo. Switching repos is then instant (no git flash).
  gitByRepo = $state<Record<string, GitStatus>>({});
  connection = $state<Conn>('connecting');
  hydrated = $state(false);
  error = $state<string | null>(null);

  // Multi-repo: populated from /api/repos. When more than one repo is registered
  // in ~/.pine/repos.json (auto-populated by `pine init`), the dashboard renders
  // a repo switcher and SSE events are filtered by the active repo.
  repos = $state<RepoInfo[]>([]);
  activeRepo = $state<string>('default');

  // Ticket ids that recently changed from disk — drives the flash animation.
  flashing = $state<Record<string, number>>({});

  private recentOps = new Set<string>();
  private sse: PineEventSource | null = null;
  private inflight: Promise<void> | null = null;
  private bodyFetches = new Map<string, Promise<void>>();
  private epoch = 0; // bumped on repo switch to drop in-flight body fetches
  private loadTimer: ReturnType<typeof setTimeout> | null = null;

  list = $derived(Object.values(this.tickets));
  isMultiRepo = $derived(this.repos.length > 1);

  columns = $derived.by<ColumnView[]>(() => {
    const b = this.board;
    if (!b) return [];
    const first = b.columns[0]?.status;
    // One pass over the tickets instead of a filter per column. A ticket with no
    // status (e.g. an agent omitted the frontmatter key) falls into the first
    // column rather than disappearing from the board.
    const groups = new Map<string, Ticket[]>();
    for (const t of this.list) {
      const key = t.status === '' && first !== undefined ? first : t.status;
      const g = groups.get(key);
      if (g) g.push(t);
      else groups.set(key, [t]);
    }
    const cols: ColumnView[] = b.columns.map((c) => ({
      status: c.status,
      title: c.title,
      tickets: this.sorted(groups.get(c.status) ?? [])
    }));
    // Unmapped statuses render as an "Other" tray on the far right.
    const mapped = new Set(b.columns.map((c) => c.status));
    for (const [status, ts] of groups) {
      if (status && !mapped.has(status)) {
        cols.push({ status, title: `Other: ${status}`, tickets: this.sorted(ts) });
      }
    }
    return cols;
  });

  counts = $derived.by(() => {
    let open = 0, testing = 0, done = 0;
    for (const t of this.list) {
      if (t.status === 'done') done++;
      else if (t.status === 'testing') testing++;
      else open++;
    }
    return { open, testing, done, total: this.list.length };
  });

  // Column order: manually-placed cards keep their persisted `order`; the rest
  // fall back to priority/recency. All ordering logic lives in ./board-order.
  private sorted(ts: Ticket[]): Ticket[] {
    return sortByEffective(ts);
  }

  // Overlapping calls share one in-flight request (the SSE onopen resync and the
  // fallback paths can race on a cold load).
  hydrate(): Promise<void> {
    if (!this.inflight) {
      const p = this.load().finally(() => {
        if (this.inflight === p) this.inflight = null;
      });
      this.inflight = p;
    }
    return this.inflight;
  }

  private async load() {
    try {
      const [snap, repos] = await Promise.all([
        api.snapshot(),
        api.repos().catch(() => null)
      ]);
      const prev = this.tickets;
      const map: Record<string, Ticket> = {};
      for (const t of snap.tickets) {
        // Snapshots omit bodies. Keep one we already loaded while the file is
        // unchanged; a changed hash makes it stale, so it is dropped and re-fetched.
        const old = prev[t.id];
        map[t.id] =
          typeof t.body !== 'string' && typeof old?.body === 'string' && old.hash === t.hash
            ? { ...t, body: old.body }
            : t;
      }
      this.tickets = map;
      this.board = snap.board;
      this.config = snap.config;
      this.git = snap.git;
      if (repos) {
        this.repos = repos.repos ?? [];
        this.activeRepo = repos.active ?? 'default';
      }
      // Seed the active repo's git cache from the snapshot so the first switch
      // away and back is instant; per-repo pollers fill in the rest over SSE.
      this.gitByRepo = { ...this.gitByRepo, [this.activeRepo]: snap.git };
      this.hydrated = true;
      this.error = null;
    } catch (e) {
      this.error = e instanceof Error ? e.message : 'failed to load';
      throw e;
    }
  }

  // ensureBody lazy-loads a ticket's markdown body (the snapshot omits it). It is
  // a no-op for unknown or off-branch tickets (those always ship their body — the
  // detail endpoint can't serve them) and when the body is already loaded.
  // Concurrent calls for the same id share one request.
  ensureBody(id: string): Promise<void> {
    const t = this.tickets[id];
    if (!t || t.readOnly || typeof t.body === 'string') return Promise.resolve();
    let p = this.bodyFetches.get(id);
    if (!p) {
      p = this.fetchBody(id).finally(() => {
        if (this.bodyFetches.get(id) === p) this.bodyFetches.delete(id);
      });
      this.bodyFetches.set(id, p);
    }
    return p;
  }

  private async fetchBody(id: string) {
    const epoch = this.epoch;
    const fetched = fullTicket(await api.getTicket(id));
    const cur = this.tickets[id];
    // Never clobber a newer SSE/PATCH copy (it already carries a body) with this
    // possibly older fetch; a body-less copy has nothing to lose. A repo switch
    // in between makes the fetch foreign (ids repeat across repos).
    if (epoch !== this.epoch || !cur || typeof cur.body === 'string') return;
    this.tickets = {
      ...this.tickets,
      // Same version: just attach the body. Otherwise adopt the whole fetched
      // ticket so its body and hash stay consistent (a stale hash only costs a 409).
      [id]: cur.hash === fetched.hash ? { ...cur, body: fetched.body } : fetched
    };
  }

  async switchRepo(alias: string) {
    if (alias === this.activeRepo) return;
    await api.activateRepo(alias);
    this.activeRepo = alias;
    this.epoch++;
    this.bodyFetches.clear();
    // Promote the cached git snapshot (if its poller has reported one) so the
    // header updates instantly; hydrate() refreshes it from the snapshot next.
    const cached = this.gitByRepo[alias];
    if (cached) this.git = cached;
    // An in-flight load predates the switch and holds the old repo's data.
    await this.inflight?.catch(() => {});
    await this.hydrate();
  }

  // The SSE open handler does the initial load (and every resync after a
  // reconnect), so a cold start issues a single snapshot. Two fallbacks keep the
  // board loading if the stream never opens: a failed attempt, or a long silence.
  startLive() {
    if (this.sse) return;
    this.sse = new PineEventSource(
      (ev) => this.applyEvent(ev),
      () => {
        this.connection = 'live';
        // Resync on every (re)connect; drafts live elsewhere so nothing is lost.
        this.hydrate().catch(() => {});
      },
      () => {
        this.connection = 'down';
        if (!this.hydrated) this.hydrate().catch(() => {});
      }
    );
    this.sse.connect();
    this.loadTimer = setTimeout(() => {
      if (!this.hydrated) this.hydrate().catch(() => {});
    }, FALLBACK_LOAD_MS);
  }

  stopLive() {
    if (this.loadTimer) clearTimeout(this.loadTimer);
    this.loadTimer = null;
    this.sse?.close();
    this.sse = null;
  }

  private trackOp(): string {
    const id = uuid();
    this.recentOps.add(id);
    setTimeout(() => this.recentOps.delete(id), 10000);
    return id;
  }

  // beginOp registers a new operation id so its echoed SSE event is recognized as
  // our own (no flash). Callers that mutate outside move/patch/create (e.g. the
  // editor's body save and attachment uploads) use this and pass the id along.
  beginOp(): string {
    return this.trackOp();
  }

  applyEvent(ev: PineEvent) {
    // Cache every repo's git snapshot before the per-repo filter below drops
    // non-active events: the active repo's git.updated promotes to `git`, while
    // other repos' updates are retained in gitByRepo so a later switch is live.
    if (ev.type === 'git.updated' && ev.git) {
      this.gitByRepo = { ...this.gitByRepo, [ev.repo ?? this.activeRepo]: ev.git };
    }
    // Ignore events for other repos when serving a multi-repo workspace.
    if (this.isMultiRepo && ev.repo && ev.repo !== this.activeRepo) {
      return;
    }
    const external = ev.origin?.source === 'fs' || !this.recentOps.has(ev.origin?.opId ?? '');
    switch (ev.type) {
      case 'ticket.created':
      case 'ticket.updated':
        if (ev.ticket) {
          this.put(ev.ticket);
          if (external) this.flash(ev.ticket.id);
        }
        break;
      case 'ticket.deleted':
        if (ev.id) {
          const { [ev.id]: _, ...rest } = this.tickets;
          this.tickets = rest;
        }
        break;
      case 'board.updated':
        if (ev.board) this.board = ev.board;
        break;
      case 'config.updated':
        if (ev.config) this.config = ev.config;
        break;
      case 'git.updated':
        if (ev.git) this.git = ev.git;
        break;
      case 'crossbranch.updated': {
        // Replace the off-branch subset wholesale: keep every local (editable)
        // ticket, then add the fresh off-branch set (local always wins on id).
        const next: Record<string, Ticket> = {};
        for (const [id, t] of Object.entries(this.tickets)) {
          if (!t.readOnly) next[id] = t;
        }
        for (const t of ev.tickets ?? []) {
          if (!next[t.id]) next[t.id] = t;
        }
        this.tickets = next;
        break;
      }
    }
  }

  flash(id: string) {
    this.flashing = { ...this.flashing, [id]: (this.flashing[id] ?? 0) + 1 };
    setTimeout(() => {
      const { [id]: _, ...rest } = this.flashing;
      this.flashing = rest;
    }, 1400);
  }

  // Optimistic status move (drag & drop). Reverts on failure.
  async move(id: string, toStatus: string) {
    const before = this.tickets[id];
    // Off-branch tickets are read-only: never attempt a move the server will 409.
    if (!before || before.status === toStatus || before.readOnly) return;
    const opId = this.trackOp();
    this.tickets = { ...this.tickets, [id]: { ...before, status: toStatus } };
    try {
      this.put(await api.patchTicket(id, { status: toStatus, opId }, before.hash));
    } catch (e) {
      // Roll back only if no external update landed while the request was in
      // flight (our optimistic copy kept `before.hash`; an external change would
      // have replaced it with a different hash, which we must not clobber).
      const cur = this.tickets[id];
      if (cur && cur.hash === before.hash) {
        this.tickets = { ...this.tickets, [id]: before };
      }
      throw e;
    }
  }

  // Optimistic drag reorder: persists a new manual `order` (and `status` when the
  // card crossed columns) in one PATCH. Reverts on failure, guarding against a
  // concurrent external update just like move().
  async reorder(id: string, changes: { status?: string; order?: number }) {
    const before = this.tickets[id];
    if (!before || before.readOnly) return;
    const patch: Record<string, unknown> = {};
    if (changes.status !== undefined && changes.status !== before.status) patch.status = changes.status;
    if (changes.order !== undefined && changes.order !== before.order) patch.order = changes.order;
    if (Object.keys(patch).length === 0) return; // nothing actually changed
    const opId = this.trackOp();
    this.tickets = { ...this.tickets, [id]: { ...before, ...patch } as Ticket };
    try {
      this.put(await api.patchTicket(id, { ...patch, opId }, before.hash));
    } catch (e) {
      const cur = this.tickets[id];
      if (cur && cur.hash === before.hash) {
        this.tickets = { ...this.tickets, [id]: before };
      }
      throw e;
    }
  }

  async patch(id: string, patch: Record<string, unknown>): Promise<Ticket> {
    const cur = this.tickets[id];
    if (cur?.readOnly) throw new Error(readOnlyMessage(cur));
    const opId = this.trackOp();
    const updated = await api.patchTicket(id, { ...patch, opId }, cur?.hash);
    this.put(updated);
    return updated;
  }

  async create(input: Record<string, unknown>): Promise<Ticket> {
    const opId = this.trackOp();
    const t = await api.createTicket({ ...input, opId });
    this.put(t);
    return t;
  }

  async remove(id: string) {
    const cur = this.tickets[id];
    if (cur?.readOnly) throw new Error(readOnlyMessage(cur));
    const opId = this.trackOp();
    await api.deleteTicket(id, opId);
    const { [id]: _, ...rest } = this.tickets;
    this.tickets = rest;
  }

  // put stores a full ticket (PATCH/SSE/GET response) in the map.
  put(t: Ticket) {
    this.tickets = { ...this.tickets, [t.id]: fullTicket(t) };
  }

  get(id: string): Ticket | undefined {
    return this.tickets[id];
  }

  allLabels(): string[] {
    const set = new Set<string>();
    for (const t of this.list) for (const l of t.labels) set.add(l);
    return [...set].sort();
  }
}

export const workspace = new WorkspaceStore();
