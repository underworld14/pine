// Typed client for the Pine API plus the SSE event stream.

export type Priority = 'low' | 'medium' | 'high' | 'critical';

export interface Attachment {
  name: string;
  size: number;
  mime: string;
  kind: 'image' | 'video' | 'other';
  url: string;
}

export interface ChildRef {
  id: string;
  title: string;
  status: string;
}

export interface Ticket {
  id: string;
  type: string;
  title: string;
  status: string;
  priority: Priority | string;
  labels: string[];
  deps: string[];
  parent?: string;
  created: string;
  updated: string;
  blocked: boolean;
  unmet?: string[];
  dangling?: string[];
  inCycle?: boolean;
  children?: ChildRef[];
  epicProgress?: { done: number; total: number };
  hash: string;
  degraded?: boolean;
  body?: string;
  attachments: Attachment[];
  // Cross-branch provenance. source is 'local' for the checked-out working tree
  // or 'local-branch' for a ticket read from another git branch. branch names
  // that branch; readOnly marks a ticket that cannot be edited from here.
  source?: string;
  branch?: string;
  readOnly?: boolean;
}

export interface Column {
  status: string;
  title: string;
}

export interface Board {
  columns: Column[];
  unmapped: string[];
}

export interface GitCommit {
  hash: string;
  subject: string;
  author: string;
  when: string;
}

export interface GitStatus {
  isRepo: boolean;
  branch: string;
  dirty: boolean;
  changes: { path: string; code: string }[];
  truncated: boolean;
  commits: GitCommit[];
}

export interface Config {
  project: { name: string };
  types: { prefix: string; name: string }[];
  priorities: string[];
}

export interface Snapshot {
  tickets: Ticket[];
  board: Board;
  config: Config;
  git: GitStatus;
  seq: number;
}

export interface SearchHit {
  id: string;
  score: number;
  title: string;
  status: string;
  type: string;
  fragments?: Record<string, string[]>;
}

export interface AttachmentResult extends Attachment {
  path: string;
  markdown: string;
  originalBytes: number;
  finalBytes: number;
  optimized: boolean;
  deduplicated: boolean;
  warning?: string;
  error?: string;
}

export interface PineEvent {
  type: string;
  seq: number;
  origin: { source: 'api' | 'fs'; opId?: string };
  ticket?: Ticket;
  tickets?: Ticket[]; // crossbranch.updated carries the full off-branch set
  id?: string;
  board?: Board;
  config?: Config;
  git?: GitStatus;
}

export class ApiError extends Error {
  status: number;
  code: string;
  current?: Ticket;
  constructor(status: number, code: string, message: string, current?: Ticket) {
    super(message);
    this.status = status;
    this.code = code;
    this.current = current;
  }
}

async function req<T>(method: string, path: string, body?: unknown, headers: Record<string, string> = {}): Promise<T> {
  const opts: RequestInit = { method, headers: { ...headers } };
  if (body !== undefined) {
    (opts.headers as Record<string, string>)['Content-Type'] = 'application/json';
    opts.body = JSON.stringify(body);
  }
  const res = await fetch(path, opts);
  if (res.status === 204) return undefined as T;
  const text = await res.text();
  const data = text ? JSON.parse(text) : undefined;
  if (!res.ok) {
    const code = data?.error?.code ?? 'error';
    const msg = data?.error?.message ?? res.statusText;
    throw new ApiError(res.status, code, msg, data?.current);
  }
  return data as T;
}

export const api = {
  snapshot: () => req<Snapshot>('GET', '/api/snapshot'),
  createTicket: (input: Record<string, unknown>) => req<Ticket>('POST', '/api/tickets', input),
  getTicket: (id: string) => req<Ticket>('GET', `/api/tickets/${id}`),
  patchTicket: (id: string, patch: Record<string, unknown>, ifMatch?: string) =>
    req<Ticket>('PATCH', `/api/tickets/${id}`, patch, ifMatch ? { 'If-Match': ifMatch } : {}),
  deleteTicket: (id: string, opId?: string) =>
    req<void>('DELETE', `/api/tickets/${id}${opId ? `?opId=${opId}` : ''}`),
  search: (params: Record<string, string>) => {
    const q = new URLSearchParams(params).toString();
    return req<{ indexing: boolean; hits: SearchHit[] }>('GET', `/api/search?${q}`);
  },
  files: (q: string) => req<{ files: string[] }>('GET', `/api/files?q=${encodeURIComponent(q)}`),
  ticketPrompt: async (id: string) => (await fetch(`/api/tickets/${id}/prompt`)).text(),
  context: async () => (await fetch('/api/context')).text(),
  async upload(id: string, files: File[], opts: { opId?: string; onProgress?: (p: number) => void } = {}): Promise<AttachmentResult[]> {
    const form = new FormData();
    for (const f of files) form.append('files', f, f.name);
    const url = `/api/tickets/${id}/attachments${opts.opId ? `?opId=${encodeURIComponent(opts.opId)}` : ''}`;
    return await new Promise((resolve, reject) => {
      const xhr = new XMLHttpRequest();
      xhr.open('POST', url);
      xhr.upload.onprogress = (e) => { if (e.lengthComputable && opts.onProgress) opts.onProgress(e.loaded / e.total); };
      xhr.onload = () => {
        try {
          const data = JSON.parse(xhr.responseText);
          if (xhr.status >= 200 && xhr.status < 300) resolve(data.attachments ?? []);
          else reject(new ApiError(xhr.status, 'upload_failed', data?.error?.message ?? 'upload failed'));
        } catch (e) { reject(e); }
      };
      xhr.onerror = () => reject(new Error('network error'));
      xhr.send(form);
    });
  }
};

// PineEventSource wraps EventSource with reconnect/backoff and snapshot resync.
export class PineEventSource {
  private es: EventSource | null = null;
  private backoff = 500;
  private closed = false;
  private readonly types = [
    'ticket.created', 'ticket.updated', 'ticket.deleted',
    'board.updated', 'config.updated', 'git.updated', 'crossbranch.updated'
  ];

  constructor(
    private onEvent: (ev: PineEvent) => void,
    private onOpen: () => void,
    private onDown: () => void
  ) {}

  connect() {
    if (this.closed) return;
    this.es = new EventSource('/api/events');
    this.es.onopen = () => { this.backoff = 500; this.onOpen(); };
    this.es.onerror = () => {
      this.es?.close();
      this.onDown();
      if (this.closed) return;
      const wait = this.backoff + Math.random() * 250;
      this.backoff = Math.min(this.backoff * 2, 8000);
      setTimeout(() => this.connect(), wait);
    };
    for (const t of this.types) {
      this.es.addEventListener(t, (e) => {
        try {
          const data = JSON.parse((e as MessageEvent).data);
          this.onEvent({ type: t, ...data });
        } catch { /* ignore malformed */ }
      });
    }
  }

  close() {
    this.closed = true;
    this.es?.close();
  }
}
