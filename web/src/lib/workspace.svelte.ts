// The single client-side source of truth: tickets, board, config, git — hydrated
// from /api/snapshot and kept live by the SSE stream. Board columns are derived
// from each ticket's frontmatter status (self-healing), so an agent editing a
// file makes the card move without any board.json change.

import { api, PineEventSource, type Board, type Config, type GitStatus, type PineEvent, type RepoInfo, type Ticket } from './api';
import { sortByEffective } from './board-order';

type Conn = 'connecting' | 'live' | 'down';

export interface ColumnView {
  status: string;
  title: string;
  tickets: Ticket[];
}

function uuid(): string {
  return crypto.randomUUID ? crypto.randomUUID() : Math.random().toString(36).slice(2);
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

  list = $derived(Object.values(this.tickets));
  isMultiRepo = $derived(this.repos.length > 1);

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

  // Column order: manually-placed cards keep their persisted `order`; the rest
  // fall back to priority/recency. All ordering logic lives in ./board-order.
  private sorted(ts: Ticket[]): Ticket[] {
    return sortByEffective(ts);
  }

  async hydrate() {
    try {
      const [snap, repos] = await Promise.all([
        api.snapshot(),
        api.repos().catch(() => null)
      ]);
      const map: Record<string, Ticket> = {};
      for (const t of snap.tickets) map[t.id] = t;
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

  async switchRepo(alias: string) {
    if (alias === this.activeRepo) return;
    await api.activateRepo(alias);
    this.activeRepo = alias;
    // Promote the cached git snapshot (if its poller has reported one) so the
    // header updates instantly; hydrate() refreshes it from the snapshot next.
    const cached = this.gitByRepo[alias];
    if (cached) this.git = cached;
    await this.hydrate();
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
      const updated = await api.patchTicket(id, { ...patch, opId }, before.hash);
      this.tickets = { ...this.tickets, [id]: updated };
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
    const cur = this.tickets[id];
    if (cur?.readOnly) throw new Error(readOnlyMessage(cur));
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
