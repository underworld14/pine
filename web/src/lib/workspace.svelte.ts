// The single client-side source of truth: tickets, board, config, git — hydrated
// from /api/snapshot and kept live by the SSE stream. Board columns are derived
// from each ticket's frontmatter status (self-healing), so an agent editing a
// file makes the card move without any board.json change.

import { api, PineEventSource, type Board, type Config, type GitStatus, type PineEvent, type Ticket } from './api';

type Conn = 'connecting' | 'live' | 'down';

export interface ColumnView {
  status: string;
  title: string;
  tickets: Ticket[];
}

function uuid(): string {
  return crypto.randomUUID ? crypto.randomUUID() : Math.random().toString(36).slice(2);
}

const PRIORITY_RANK: Record<string, number> = { critical: 3, high: 2, medium: 1, low: 0 };

class WorkspaceStore {
  tickets = $state<Record<string, Ticket>>({});
  board = $state<Board | null>(null);
  config = $state<Config | null>(null);
  git = $state<GitStatus | null>(null);
  connection = $state<Conn>('connecting');
  hydrated = $state(false);
  error = $state<string | null>(null);

  // Ticket ids that recently changed from disk — drives the flash animation.
  flashing = $state<Record<string, number>>({});

  private recentOps = new Set<string>();
  private sse: PineEventSource | null = null;

  list = $derived(Object.values(this.tickets));

  columns = $derived.by<ColumnView[]>(() => {
    const b = this.board;
    if (!b) return [];
    const first = b.columns[0]?.status;
    const cols: ColumnView[] = b.columns.map((c) => ({
      status: c.status,
      title: c.title,
      // A ticket with no status (e.g. an agent omitted the frontmatter key) falls
      // into the first column rather than disappearing from the board.
      tickets: this.sorted(this.list.filter((t) => t.status === c.status || (t.status === '' && c.status === first)))
    }));
    // Unmapped statuses render as an "Other" tray on the far right.
    const mapped = new Set(b.columns.map((c) => c.status));
    const others = this.list.filter((t) => t.status && !mapped.has(t.status));
    if (others.length) {
      const byStatus = new Map<string, Ticket[]>();
      for (const t of others) {
        (byStatus.get(t.status) ?? byStatus.set(t.status, []).get(t.status)!).push(t);
      }
      for (const [status, ts] of byStatus) {
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

  private sorted(ts: Ticket[]): Ticket[] {
    return [...ts].sort((a, b) => {
      const pa = PRIORITY_RANK[a.priority] ?? 1;
      const pb = PRIORITY_RANK[b.priority] ?? 1;
      if (pa !== pb) return pb - pa;
      return b.updated.localeCompare(a.updated);
    });
  }

  async hydrate() {
    try {
      const snap = await api.snapshot();
      const map: Record<string, Ticket> = {};
      for (const t of snap.tickets) map[t.id] = t;
      this.tickets = map;
      this.board = snap.board;
      this.config = snap.config;
      this.git = snap.git;
      this.hydrated = true;
      this.error = null;
    } catch (e) {
      this.error = e instanceof Error ? e.message : 'failed to load';
      throw e;
    }
  }

  startLive() {
    if (this.sse) return;
    this.sse = new PineEventSource(
      (ev) => this.applyEvent(ev),
      () => {
        this.connection = 'live';
        // Resync on every (re)connect; drafts live elsewhere so nothing is lost.
        this.hydrate().catch(() => {});
      },
      () => { this.connection = 'down'; }
    );
    this.sse.connect();
  }

  stopLive() {
    this.sse?.close();
    this.sse = null;
  }

  private trackOp(): string {
    const id = uuid();
    this.recentOps.add(id);
    setTimeout(() => this.recentOps.delete(id), 10000);
    return id;
  }

  applyEvent(ev: PineEvent) {
    const external = ev.origin?.source === 'fs' || !this.recentOps.has(ev.origin?.opId ?? '');
    switch (ev.type) {
      case 'ticket.created':
      case 'ticket.updated':
        if (ev.ticket) {
          this.tickets = { ...this.tickets, [ev.ticket.id]: ev.ticket };
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
    if (!before || before.status === toStatus) return;
    const opId = this.trackOp();
    this.tickets = { ...this.tickets, [id]: { ...before, status: toStatus } };
    try {
      const updated = await api.patchTicket(id, { status: toStatus, opId }, before.hash);
      this.tickets = { ...this.tickets, [id]: updated };
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

  async patch(id: string, patch: Record<string, unknown>): Promise<Ticket> {
    const opId = this.trackOp();
    const cur = this.tickets[id];
    const updated = await api.patchTicket(id, { ...patch, opId }, cur?.hash);
    this.tickets = { ...this.tickets, [id]: updated };
    return updated;
  }

  async create(input: Record<string, unknown>): Promise<Ticket> {
    const opId = this.trackOp();
    const t = await api.createTicket({ ...input, opId });
    this.tickets = { ...this.tickets, [t.id]: t };
    return t;
  }

  async remove(id: string) {
    const opId = this.trackOp();
    await api.deleteTicket(id, opId);
    const { [id]: _, ...rest } = this.tickets;
    this.tickets = rest;
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
