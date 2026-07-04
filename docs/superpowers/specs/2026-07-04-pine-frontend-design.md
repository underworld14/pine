# Pine Web UI — Frontend Architecture & UX Design

**Scope:** entire web frontend for `pine serve` (localhost:3412). SvelteKit + Svelte 5 (runes), adapter-static SPA, embedded into the Go binary via `go:embed`. This document also pins the exact HTTP/SSE contract the frontend consumes so the backend design can match it.

**Guiding principles**

1. **Disk is truth.** The UI is a live *view* of `.pine/`. Every screen must tolerate the files changing underneath it at any moment (AI agents edit files directly) — and make that moment *visible and delightful*, not confusing.
2. **Speed is the feature.** New issue ≤10s, zero-latency optimistic UI, keyboard-first everything.
3. **No generic-looking UI.** One deliberate aesthetic ("Midnight Pine", §8), hand-rolled components on headless primitives — no component-kit look.
4. **YAGNI.** No i18n, no theming system beyond dark/light, no virtualized lists (<10k tickets, boards show far fewer), no offline mode (server is localhost).

---

## 1. Stack & Project Setup

| Concern | Choice | Version pin (at design time) |
|---|---|---|
| Framework | SvelteKit 2.x + Svelte 5 (runes only, no legacy stores) | `svelte@^5`, `@sveltejs/kit@^2` |
| Adapter | `@sveltejs/adapter-static`, SPA fallback | fallback: `index.html` |
| Language | TypeScript, `strict: true` | |
| Styling | Tailwind CSS v4 (CSS-first `@theme`, via `@tailwindcss/vite`) | `tailwindcss@^4` |
| Headless a11y primitives | **bits-ui** (Svelte-5 native: Dialog, DropdownMenu, Tooltip, Select) — styled by us | |
| Drag & drop | **svelte-dnd-action** | see §5.2 |
| Editor | **CodeMirror 6** (`codemirror`, `@codemirror/lang-markdown`, `@codemirror/view`) | |
| Markdown preview | **markdown-it** + `markdown-it-task-lists` + **DOMPurify** + highlight.js (subset build) | |
| Fuzzy matching | **fuzzysort** (~2 kB) | |
| Fonts | Inter Variable (UI) + JetBrains Mono (ids/code/kbd), self-hosted woff2 — CSP blocks external hosts | |

No axios, no tanstack-query, no component library CSS. Fetch + runes stores are sufficient for a localhost API.

### 1.1 Config files

`web/svelte.config.js`

```js
import adapter from '@sveltejs/adapter-static';
export default {
  kit: {
    adapter: adapter({ pages: 'build', assets: 'build', fallback: 'index.html' }),
    // no path prefix: chi serves us at /
  }
};
```

`web/src/routes/+layout.ts`

```ts
export const ssr = false;       // pure SPA; chi serves index.html for all non-/api paths
export const prerender = false;
```

`web/vite.config.ts` — dev proxy so `npm run dev` (port 5173, HMR) talks to a running `pine serve`:

```ts
import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
export default {
  plugins: [tailwindcss(), sveltekit()],
  server: {
    proxy: {
      '/api': { target: 'http://localhost:3412', ws: false },
      '/pine-files': 'http://localhost:3412'
    }
  }
};
```

**Build contract with Go:** `npm run build` emits `web/build/`; the Go server embeds it (`//go:embed all:web/build`) and serves `index.html` as fallback for any GET that isn't `/api/*` or `/pine-files/*`.

### 1.2 Directory structure

```
web/
├── package.json / svelte.config.js / vite.config.ts / tsconfig.json
├── playwright.config.ts
├── static/fonts/            # InterVariable.woff2, JetBrainsMono[wght].woff2
├── src/
│   ├── app.html             # <html data-theme="dark"> default; tiny inline theme-restore script
│   ├── app.css              # Tailwind v4 entry + @theme tokens (§8)
│   ├── lib/
│   │   ├── api/
│   │   │   ├── types.ts     # Ticket, Board, Config, GitStatus, SSE payloads (mirrors Go JSON)
│   │   │   ├── client.ts    # thin fetch wrapper: json(), etag/If-Match, ApiError
│   │   │   ├── upload.ts    # XHR-based uploads with progress (fetch has no upload progress)
│   │   │   └── sse.ts       # PineEventSource: reconnect/backoff/dispatch (§4)
│   │   ├── stores/
│   │   │   ├── workspace.svelte.ts  # tickets/board/config/git — the one source of client state (§3)
│   │   │   ├── draft.svelte.ts      # per-open-ticket editor draft + conflict state (§3.4)
│   │   │   ├── uploads.svelte.ts    # upload queue, progress, optimization results (§6)
│   │   │   └── ui.svelte.ts         # modal open-state, palette, toasts, theme
│   │   ├── components/
│   │   │   ├── shell/       # Sidebar.svelte, GitStatusFooter.svelte, ConnectionDot.svelte
│   │   │   ├── board/       # Board.svelte, Column.svelte, TicketCard.svelte
│   │   │   ├── ticket/      # TicketPage.svelte, FrontmatterBar.svelte, MarkdownEditor.svelte,
│   │   │   │                #   Preview.svelte, ConflictBanner.svelte
│   │   │   ├── issue/       # NewIssueModal.svelte
│   │   │   ├── attachments/ # DropOverlay.svelte, AttachmentGrid.svelte, UploadRow.svelte,
│   │   │   │                #   Lightbox.svelte
│   │   │   ├── palette/     # CommandPalette.svelte, PaletteItem.svelte
│   │   │   ├── search/      # SearchPage.svelte, ResultRow.svelte, FilterChips.svelte
│   │   │   └── ui/          # Button, Kbd, Badge, PriorityIcon, StatusPill, LabelChip,
│   │   │                    #   Toast, EmptyState, RelativeTime
│   │   ├── actions/         # shortcut.ts (global key map), pasteFiles.ts, autofocus.ts
│   │   └── utils/           # markdown.ts (md-it+DOMPurify pipeline), fmt.ts (bytes, reltime), fuzzy.ts
│   └── routes/
│       ├── +layout.svelte   # shell: sidebar, palette, toasts, DropOverlay, SSE lifecycle
│       ├── +layout.ts
│       ├── +page.svelte     # Dashboard
│       ├── board/+page.svelte
│       ├── tickets/[id]/+page.svelte
│       └── search/+page.svelte
└── tests/
    ├── unit/                # vitest
    └── e2e/                 # playwright
```

---

## 2. API Contract (what the frontend consumes)

All JSON under `/api`, attachments served statically under `/pine-files`. This is the frozen contract the Go design must implement.

| Method & path | Purpose | Notes |
|---|---|---|
| `GET /api/snapshot` | Single-request hydration | `{ tickets: Ticket[], board, config, git, seq }` — `seq` aligns with SSE stream |
| `POST /api/tickets` | Create | body `{ type, title, body, priority, labels, staged: string[], opId }` → `201 Ticket` |
| `PATCH /api/tickets/:id` | Update fields/body | header `If-Match: <hash>`; body any of `{ title, status, priority, labels, body, opId }` → `Ticket`; **409** → `{ current: Ticket }` |
| `DELETE /api/tickets/:id` | Delete | body `{ opId }` |
| `POST /api/board/move` | Atomic drag&drop | `{ ticketId, toColumn, index, opId }` — server updates frontmatter `status` **and** `board.json` order in one operation |
| `GET /api/search?q=&type=&status=&label=` | Bleve search | → `{ hits: [{ id, score, fragments: { title?, body? } }] }` (fragments contain `<mark>` from Bleve highlighting) |
| `GET /api/tickets/:id/prompt` | `pine prompt` as HTTP | `text/plain` — powers the "Copy AI prompt" button |
| `GET /api/git/status` | Poll fallback | normally pushed via SSE `git.updated` |
| `POST /api/staging` | Upload before ticket exists (multipart, XHR) | → `StagedAttachment` (§6.2) |
| `POST /api/tickets/:id/attachments` | Attach: multipart **or** `{ staged: [tokens] }` | → `AttachmentMeta[]` |
| `DELETE /api/tickets/:id/attachments/:name` | Remove file | |
| `GET /api/events` | SSE stream | §4 |
| `GET /pine-files/attachments/:ticketId/:name` | Serve attachment bytes | immutable cache headers |

### 2.1 Core types (`lib/api/types.ts`)

```ts
export type Priority = 'low' | 'medium' | 'high' | 'critical';

export interface Ticket {
  id: string;                 // "BUG-001" | "FEAT-002"
  type: 'bug' | 'feature';    // server-derived from id prefix
  title: string;
  status: string;             // must match a board column's `status`
  priority: Priority;
  labels: string[];
  created: string;            // ISO 8601
  updated: string;
  body: string;               // markdown after frontmatter, opaque to the UI
  hash: string;               // server hash of file content → If-Match / conflict detection
  attachments: AttachmentMeta[]; // server-derived from attachments/<id>/ dir listing
}

export interface AttachmentMeta {
  name: string; size: number; mime: string;
  width?: number; height?: number;       // images
  originalSize?: number;                 // set when optimizer shrank it → "2.4MB → 180KB"
  url: string;                           // /pine-files/attachments/BUG-001/login.webp
}

export interface Board {
  columns: { id: string; title: string; status: string }[];  // from board.json, in order
  order: Record<string, string[]>;                           // columnId → ticketIds (manual order)
}

export interface GitStatus {
  branch: string; dirty: number;          // modified+untracked count
  modified: string[];
  recentCommits: { sha: string; subject: string; when: string }[];
}
```

The ticket **body stays opaque markdown** in the UI (sections like `# Steps` are just content; the server parses them for search/context, the editor doesn't). Only the New Issue modal *generates* the section skeleton. This keeps the editor honest with disk truth — an AI agent can restructure the body freely and the UI never fights it.

---

## 3. State Layer (runes stores)

One store owns all workspace data. Components never fetch; they read `workspace.*` and call its methods.

### 3.1 `workspace.svelte.ts`

```ts
class WorkspaceStore {
  tickets = $state<Record<string, Ticket>>({});
  board   = $state<Board | null>(null);
  config  = $state<Config | null>(null);
  git     = $state<GitStatus | null>(null);
  connection = $state<'connecting' | 'live' | 'down'>('connecting');
  hydrated = $state(false);

  // Board view = board.order joined with live tickets, self-healing:
  columns = $derived.by(() => this.board?.columns.map(col => {
    const ordered = (this.board!.order[col.id] ?? [])
      .map(id => this.tickets[id])
      .filter(t => t && t.status === col.status);
    const strays = Object.values(this.tickets)              // AI created a file, never touched board.json
      .filter(t => t.status === col.status && !ordered.includes(t))
      .sort((a, b) => b.updated.localeCompare(a.updated));
    return { ...col, tickets: [...ordered, ...strays] };
  }) ?? []);

  async hydrate() { /* GET /api/snapshot → assign all + lastSeq; hydrated = true */ }
  async move(ticketId: string, toColumn: string, index: number) { /* §3.2 */ }
  async patchTicket(id: string, patch: Partial<Ticket>, ifMatch: string) { /* returns Ticket | throws Conflict */ }
  async createTicket(input: NewTicketInput): Promise<Ticket> { /* optimistic insert w/ temp render, §5.4 */ }
  applyEvent(ev: PineEvent) { /* §4.2 */ }
}
export const workspace = new WorkspaceStore();
```

**Self-healing derivation is the key trick:** the Kanban never trusts `board.json` alone. Tickets whose frontmatter `status` disagrees with their `order` position follow the *frontmatter* (disk truth); tickets missing from `order` still appear. An AI agent editing only `status: done` in a `.md` file makes the card move — no board.json edit required.

### 3.2 Optimistic updates & reconciliation

Every mutation carries a client-generated `opId` (`crypto.randomUUID()`). Flow for drag&drop:

```
move():
  1. snapshot = structuredClone of affected slices
  2. mutate tickets[id].status + board.order locally   (instant UI)
  3. recentOps.add(opId)                               (ring buffer, 50 entries, 10s TTL)
  4. POST /api/board/move { ..., opId }
     · 2xx → replace ticket with server's canonical copy (new hash/updated)
     · error → restore snapshot, error toast "Couldn't move BUG-012 — reverted"
```

The server tags SSE events it emits as a result of API-driven writes with the originating `opId` (backend correlates its own file writes with watcher events). Client reconciliation rule, in `applyEvent`:

- `origin.opId ∈ recentOps` → **silently absorb** (assign canonical ticket state, no flash — it's our own echo).
- otherwise → **external change**: apply state *and* trigger the external-change affordance (§4.3).

If the backend ever fails to correlate (race), worst case is a false "external change" flash on our own edit — cosmetic, acceptable, and why we don't build anything more elaborate.

### 3.3 Ticket creation (New Issue modal)

Create is *not* rendered optimistically as a fake card (the real ID is server-assigned, e.g. next `BUG-042`). Instead: modal save → `POST /api/tickets` → server responds in single-digit ms on localhost → insert ticket, close modal, toast `BUG-042 created · View`. Perceived latency is zero without fake-ID complexity.

### 3.4 Editor drafts & disk conflicts (`draft.svelte.ts`)

The ticket editor never binds directly to `workspace.tickets[id]`. It owns a draft:

```ts
class DraftStore {
  base = $state<Ticket | null>(null);   // ticket as loaded (holds .hash)
  text = $state('');                    // CM6 document
  dirty = $derived(this.base !== null && this.text !== this.base.body);
  conflict = $state<Ticket | null>(null);  // incoming disk version while dirty
  saving = $state(false);
}
```

- **Autosave:** 750 ms debounce after last keystroke; `⌘S` flushes immediately. Save = `PATCH` with `If-Match: base.hash`; success re-bases (`base = response`, conflict impossible to miss).
- **SSE `ticket.updated` for the open ticket:**
  - draft **clean** → re-base + swap editor text (CM6 transaction preserving cursor when the change is remote-only), subtle flash on the frontmatter bar. The "AI rewrote the ticket while I'm looking at it" moment just works.
  - draft **dirty** → do *not* touch the text. Set `conflict = incomingTicket`, show **ConflictBanner** pinned above the editor:
    > ⚠ **Changed on disk** (probably an AI agent) — *Reload from disk* · *Keep mine & overwrite*
    - *Reload* → discard draft, load incoming.
    - *Overwrite* → `PATCH` with `If-Match: conflict.hash` (i.e., acknowledge the new disk version and clobber it). No merge UI — YAGNI; git history is the safety net, which is the whole product thesis.
- **409 on save** (we lost a race the SSE hadn't delivered yet): identical banner, fed from the `{ current }` response body.

Frontmatter controls (title/status/priority/labels) save immediately on change (each its own small `PATCH`) — they don't participate in the body draft, so a status flip is never held hostage by unsaved prose.

---

## 4. SSE Client

### 4.1 Transport (`lib/api/sse.ts`)

Wrap `EventSource` for controllable backoff (native retry is too opaque):

- Connect to `GET /api/events`. Each event: `id: <seq>`, `event: <type>`, `data: <json>`.
- On `error` → close, reconnect with exponential backoff `500ms → 1s → 2s → 4s → 8s (cap)`, jittered. `workspace.connection = 'down'` after the first failed attempt.
- **On every (re)open after a disconnect: refetch `GET /api/snapshot`** and replace store state wholesale (drafts are preserved — they live in `draft`, not `workspace`). No replay-buffer protocol; localhost snapshots are cheap and this is bulletproof. `connection = 'live'`.
- Event types: `ticket.created` `ticket.updated` `ticket.deleted` `board.updated` `config.updated` `git.updated` `attachments.changed` — payload always `{ seq, origin: { source: 'api', opId?: string } | { source: 'fs' }, ... }`.

Connection surfaced as a small dot in the sidebar footer (green pulse = live, amber = reconnecting). If down > 5s, a quiet pill: *"Reconnecting to pine serve…"*. Never a blocking overlay.

### 4.2 Event → store

`applyEvent` in `workspace` is a plain reducer: upsert/delete ticket, replace board/config/git. All origin-checking logic from §3.2 lives here — components never see raw events.

### 4.3 The wow moment: external changes made visible

When `origin.source === 'fs'` (or unmatched opId):

1. **Card flash:** the affected `TicketCard` gets `data-flash` for 1.4s → CSS animation: 2px accent ring pulses in + fades, background tints `--color-accent/8%` and decays. Runs wherever the card is visible (board, dashboard, search).
2. **Card movement:** board columns are keyed `{#each column.tickets as t (t.id)}` with `animate:flip={{ duration: 250 }}` — when an agent flips `status: doing → testing` in the file, the card *glides* to the next column with the flash ring on. This is the demo moment; the flip+flash combination is specified, not optional polish.
3. **Coalesced toast** (bottom-right, 4s): "**BUG-012** updated from disk" — multiple events within 2s collapse to "*3 tickets updated from disk*". Click → navigate (single) or board (multiple). Suppressed for `git.updated` (too chatty; the sidebar git footer just live-updates).
4. Open-editor case handled in §3.4.

---

## 5. Screens & Flows

### 5.0 App shell (`+layout.svelte`)

- **Left sidebar, 208px** (collapsible to 56px icon rail, persisted): logo/project name (from config), nav — Dashboard, Board, Search — with `Kbd` hints (`g d`, `g b`, `/`). Primary **"+ New issue"** button (accent, `c`).
- **Sidebar footer:** git status — `⎇ main · 3 modified` (tooltip lists files), connection dot, theme toggle.
- Mounted once in layout: `CommandPalette`, `NewIssueModal`, `Toaster`, `DropOverlay`, global `shortcut` action on `<svelte:window>`, SSE lifecycle (`onMount` connect / `onDestroy` close).
- Hydration: layout awaits `workspace.hydrate()`; show content immediately when done — a bare centered pine-tree glyph spinner only if >150ms (localhost: effectively never).
- Server unreachable at boot → full-screen friendly state: *"Can't reach pine serve — is it still running in `<repo>`?"* with retry (auto-retries on the SSE backoff cadence).

### 5.1 Dashboard (`/`)

Purpose per PRD: at-a-glance triage, not analytics.

- **Header row:** project name, git branch chip, counts (`12 open · 3 testing · 41 done`).
- **Four dense lists** (PRD: Recent Bugs / Recent Features / Testing / Done — each max 7, "View all → board filtered"): rows = `PriorityIcon · ID (mono) · title · RelativeTime`, click → ticket.
- **Recent activity** derived client-side: tickets sorted by `updated` desc — no dedicated endpoint (YAGNI).
- Empty state (fresh `pine init`): centered card — *"No issues yet. Press `c` to create your first one — it takes 10 seconds."*

### 5.2 Kanban board (`/board`)

Columns rendered from `workspace.columns` (§3.1).

**DnD: `svelte-dnd-action`.** Justification: mature, actively maintained with Svelte 5 support, handles multi-container lists + FLIP animations natively, and ships **keyboard-accessible DnD** (focus card, space to lift, arrows to move) for free — building keyboard DnD custom is weeks of work; pragmatic-drag-and-drop is framework-agnostic but needs all rendering glue hand-written. Custom DnD is rejected: it's the highest-bug-density widget class in frontend.

- `Column.svelte` wraps its list in `use:dndzone={{ items, flipDurationMs: 250, dropTargetStyle: {} }}`; on `finalize` → `workspace.move(id, columnId, index)` (optimistic, §3.2). Drop-target affordance: column background lifts one surface step + dashed inset outline in accent/30%.
- **TicketCard** (dense, ~64px): line 1 — `PriorityIcon` + `ID` in mono caps + label chips (max 2 + "+n"); line 2 — title (2-line clamp); footer — attachment count 📎, relative updated time. Click → ticket page. `data-flash` behavior per §4.3.
- Column header: title + count; subtle WIP-less (no limits — YAGNI).
- Board filters (top bar): text filter (client-side, instant), priority and label chips. Filtered-out cards during a drag are simply hidden (dnd list is the filtered list; server move API uses index *within column order* — client translates filtered index → absolute index before POSTing).

### 5.3 Ticket detail (`/tickets/[id]`)

Two vertical zones:

**FrontmatterBar** (sticky): back-link, `ID` (mono, click-to-copy), inline-editable title (renders as h1, becomes input on click/`e`), `StatusPill` (Select from board columns), priority segmented control (4 icons), `LabelChip` token input (autocomplete from all existing labels), created/updated meta, overflow menu: **Copy AI prompt** (fetches `/api/tickets/:id/prompt` → clipboard → toast "Prompt copied — paste into Claude Code"), Copy file path (`.pine/tickets/BUG-001.md`), Delete.

**Body editor — CodeMirror 6, three modes:** `Preview` (default when navigating in — tickets are read far more than written), `Split`, `Edit`; toggle group top-right, `⌘E` cycles. 

- CM6 setup: `lang-markdown`, line-wrapping, custom theme matching tokens (§8), no line numbers (it's prose), `⌘S` flush-save keymap, paste/drop handlers for attachments (§6.3).
- Preview pane: `markdown.ts` pipeline — `markdown-it` (`html: true, linkify: true`) + `markdown-it-task-lists` + highlight.js (core + ~15 common langs) → **DOMPurify** strict allowlist (no scripts/styles/iframes, `target=_blank rel=noopener` forced on links). A markdown-it core rule rewrites relative image/video srcs `attachments/…` → `/pine-files/attachments/…` so ticket files stay portable/relative on disk but render in-app.
- Split mode: 50/50, preview scroll loosely synced by line-fraction (simple proportional sync — not source-map precise, YAGNI).
- Below the body: **AttachmentGrid** (§6.4) — always rendered from `ticket.attachments` (server dir listing), independent of whether the markdown mentions them.
- ConflictBanner per §3.4.

### 5.4 New Issue modal — the ≤10s path

`bits-ui` Dialog, 560px, opens from: `c` anywhere, palette, sidebar button, board column "+" (pre-sets status to that column).

**Layout, top→bottom:** type toggle `[🐛 Bug] [✦ Feature]` (Bug preselected; `⌘1`/`⌘2`) · **Title input — autofocused** · description textarea (3 rows, grows; plain textarea, *not* CM6 — modal must be feather-light) · priority segmented (Medium preselected) · labels token input · attachment strip · footer: `Esc Cancel` — `⌘↵ Create`.

**Speed engineering:**
- `⌘Enter` submits from *any* focused field. `Enter` in the title field also submits (single-field fast path). Title is the only required field.
- Smart defaults: status = first board column, priority medium, body composed from the type's template (`# Description` with the typed description, then empty `# Steps / # Expected / # Actual` for bugs; Description only for features — templates from `.pine/templates/` when present, built-in fallback otherwise).
- **Paste screenshot anywhere in the modal:** a `pasteFiles` action on the dialog root catches `⌘V` with image clipboard data → instantly appears in the attachment strip as an uploading thumbnail (§6.2 staging upload starts immediately, in parallel with typing). Drag-drop onto the modal does the same.
- Submit while uploads in-flight: button shows "Creating…" and awaits outstanding staging uploads (typically already done — optimizer runs in the seconds the user spent typing), then `POST /api/tickets` with staged tokens.
- On success: modal closes instantly, toast `BUG-042 created · View`. Focus returns to the prior element — **the user is back to testing**. On failure: modal stays open with inline error, nothing lost.
- `Esc`: closes immediately if pristine; if title/description/attachments present → tiny confirm ("Discard draft?").

**The 10-second trace:** `c` → type title → `⌘V` screenshot → `⌘↵` = 4 interactions. This is the Playwright-enforced budget (§9).

### 5.5 Search (`/search?q=`)

- `/` focuses the omnipresent search affordance; typing ≥2 chars debounced 150ms → `GET /api/search`.
- Results page: query input (URL-synced), **FilterChips** (type / status / priority / label — mapped to query params), result rows: `ID · title (with <mark> from Bleve fragments) · body snippet · status pill · updated`. `↑↓` + `Enter` keyboard navigation. Empty: "No matches for *q* — press `c` to create it as an issue" (turns dead-ends into capture).
- `<mark>` fragments from the server are sanitized through the same DOMPurify pipeline (allowlist includes `mark` only for this surface).

### 5.6 Command palette (`⌘K`)

**Hand-rolled** on bits-ui Dialog + `fuzzysort` (~150 lines; cmdk is React, Svelte ports are unmaintained — owning it is cheaper than adopting a dead dep).

- Single input, sectioned results, `↑↓/Enter`, all mouse-optional.
- **Empty query:** Commands (New Bug `c`, New Feature, Go to Board `g b`, Go to Dashboard `g d`, Copy AI prompt for current ticket, Toggle theme) + 5 most-recently-updated tickets.
- **With query:** fuzzysort over commands + all ticket `id + title` (client-side — full ticket list is already in the store; sub-ms for 10k), plus a pinned last row: *"Search everything for 'q' ↵"* → `/search?q=` (bridges to Bleve full-text since palette fuzzy only sees ids/titles).
- Ticket ids get exact-prefix boost ("BUG-1" ranks BUG-001x first).

---

## 6. Attachment UX

### 6.1 Ingestion surfaces

1. **Paste** — modal (§5.4) and ticket editor: CM6 paste handler intercepts image clipboard data.
2. **Drag & drop** — global `DropOverlay` in the layout: `dragenter` with files anywhere over the window dims the app and shows a drop target ("Drop to attach to **BUG-012**" on a ticket page; on other pages, "Drop to create a new issue" → opens the modal pre-seeded). Counts drag depth correctly (the classic enter/leave flicker bug is specified away).
3. **Upload button** — attachment grid's "+" tile → file picker (`accept` per PRD types).

Client-side pre-validation: extension/mime allowlist (PNG/JPEG/GIF/WEBP/MP4/MOV), size cap from `config.maxUploadMB` (server re-validates). Rejection = shake animation on the drop target + toast with the reason.

### 6.2 Upload pipeline (`uploads.svelte.ts` + `api/upload.ts`)

Uploads use **XHR** (fetch still has no upload progress) to `POST /api/staging` (pre-create) or `POST /api/tickets/:id/attachments` (existing ticket). Queue store:

```ts
interface UploadItem {
  id: string; file: File; ticketId?: string;    // undefined = staged for modal
  state: 'uploading' | 'optimizing' | 'done' | 'error';
  progress: number;                              // 0–1 from XHR upload events
  result?: AttachmentMeta & { token?: string };  // token when staged
  error?: string;
}
```

Server response includes optimizer output; **UploadRow** renders the payoff explicitly: thumbnail · name · then on completion a green stat line — **`2.4 MB → 180 KB · webp`** (or `2.4 MB · kept original` when the optimizer didn't win, or `⚠ 84 MB video — consider trimming` for large videos, which pass through per the locked decision). The `uploading → optimizing` state shows because optimization happens server-side after byte receipt (progress bar full, spinner label "Optimizing…").

Failure state: row turns red with message + **Retry** / **Remove**; the modal never loses other state.

### 6.3 In-editor paste/drop (existing ticket)

Paste image into CM6 → immediately insert placeholder `![Uploading login.png…]()` at cursor → upload to `/api/tickets/:id/attachments` → on success, replace placeholder with `![login](attachments/BUG-012/login.webp)` (relative path — file stays portable) via a CM6 transaction; on failure, replace with nothing + toast. GitHub-style, zero modal.

### 6.4 AttachmentGrid + Lightbox

- Grid of 120px tiles: images render the actual file (already ≤2000px & webp — browser downscaling is fine; **no server thumbnails, YAGNI**), videos render a `<video preload="metadata">` first frame + duration badge, hover → filename/size, `×` on hover → confirm-less delete with 5s **Undo** toast (undo = re-upload is impossible; instead delete is server-side deferred 5s — *backend contract:* `DELETE` supports `?defer=5s` cancellation… **rejected, YAGNI**: delete is immediate, confirm dialog instead. Small friction on a rare destructive action is correct).
- Click image → **Lightbox** (custom, ~80 lines): full-screen dimmed overlay, centered image, `←/→` cycles the ticket's images, `Esc` closes, filename + size caption, "Copy markdown reference" button. Videos open playing `<video controls>` in the same overlay.

---

## 7. Keyboard Map (global `shortcut` action)

| Key | Action | | Key | Action |
|---|---|---|---|---|
| `c` | New issue modal | | `⌘K` | Command palette |
| `/` | Focus search | | `g d` / `g b` | Go dashboard / board |
| `e` | Edit mode (ticket page) | | `⌘E` | Cycle editor mode |
| `⌘S` | Flush-save draft | | `⌘↵` | Submit modal |
| `Esc` | Close overlay / back out | | `↑↓ ↵` | List navigation (palette, search) |

Rules: single-letter shortcuts suppressed while any input/CM6 has focus; sequences (`g d`) have a 800ms window; every menu item and tooltip shows its `Kbd`. Focus is never trapped except in dialogs (bits-ui handles trap/restore); focus rings never removed.

---

## 8. Design Language — "Midnight Pine"

A fast, dense, terminal-adjacent dev tool. Linear-grade polish, but warmer and unmistakably *pine*: near-black surfaces with a faint green cast, one living accent, mono details everywhere data appears.

- **Dark by default** (dev tool at 1am is the persona); light theme fully supported via `data-theme` on `<html>`, restored pre-paint by an inline script in `app.html`.
- **Typography:** Inter Variable — UI at **13px/1.45** base (desktop-tool density), 15px for editor prose, 20px/650 for page titles. **JetBrains Mono** for ticket IDs, timestamps, file paths, `Kbd`, code. Tabular numerals on counts. Tight tracking (-0.01em) on headings.
- **Spacing/density:** 4px grid; cards pad 10/12px; list rows 32px; radius 6px controls / 8px cards / 10px overlays. Hairline borders `1px` at `white/8%` (dark) instead of shadows; shadows only on floating layers (palette, modals, lightbox): `0 8px 32px rgb(0 0 0 / .45)`.
- **Tokens** (`app.css`, Tailwind v4 `@theme` — exact values, dark shown):

```css
@theme {
  --color-bg: #0e1210;          /* near-black, green-cast */
  --color-surface: #151a17;     /* cards, columns */
  --color-surface-2: #1c2320;   /* hover, raised */
  --color-border: rgb(255 255 255 / 0.08);
  --color-text: #e6eae7;  --color-text-dim: #9aa59e;
  --color-accent: #34d399;      /* pine — actions, focus, flash */
  --color-danger: #f87171; --color-warn: #fbbf24; --color-info: #60a5fa;
  --font-sans: 'Inter Variable', system-ui;
  --font-mono: 'JetBrains Mono', ui-monospace;
}
/* light theme overrides under [data-theme="light"]: bg #fafbfa, surface #ffffff,
   border black/8%, accent #059669 — AA contrast verified both themes */
```

- **Semantic color:** priority — critical `--color-danger` filled icon, high `--color-warn`, medium `--color-info`, low `--color-text-dim` (icon shape differs too — never color-only). Status pills tint at 12% alpha with solid text. Labels get deterministic hue from a hash of the label name (fixed 8-hue palette, consistent across sessions).
- **Motion:** 120–180ms `cubic-bezier(0.2, 0, 0, 1)`; board FLIP 250ms; external-change flash 1.4s (§4.3); modal: 140ms scale `0.98→1` + fade. Everything behind `prefers-reduced-motion`.
- **Voice:** microcopy is terse and developer-native ("Changed on disk", "Reverted", `pine prompt` verbatim). No exclamation marks.

---

## 9. Testing

### 9.1 Vitest (unit + component) — `vitest` + `@testing-library/svelte` + `happy-dom`

- **workspace store:** hydrate; optimistic move → SSE echo with matching opId absorbed silently; SSE with foreign origin applies + marks flash; move failure rolls back; stray-ticket self-healing (ticket with status not in `board.order` appears in the right column).
- **draft store:** clean-draft SSE re-base; dirty-draft SSE sets conflict; 409 path; If-Match chain after save.
- **markdown pipeline:** XSS corpus (`<script>`, `onerror=`, `javascript:` links) fully neutralized; relative `attachments/` src rewriting; task-list rendering.
- **SSE wrapper:** backoff schedule, snapshot-refetch-on-reopen (mocked EventSource).
- **Components:** NewIssueModal (autofocus on open, Enter-in-title submits, ⌘Enter from textarea submits, Esc-pristine closes / Esc-dirty confirms, paste event enqueues upload); CommandPalette ranking (exact-id-prefix beats fuzzy); TicketCard flash class.

### 9.2 Playwright (e2e smoke) — runs against the **real compiled `pine` binary** serving a temp fixture repo (`webServer` in config runs `make e2e-serve`: builds web → builds Go → `pine init` in a tmp dir → `pine serve`)

1. **`create-issue-under-10s.spec.ts` — the metric, enforced:** open board → `c` → type title → paste a PNG from clipboard (CDP `Browser.setClipboard` / `page.evaluate` ClipboardItem) → `⌘Enter` → card visible on board. Asserts (a) exactly 4 user gestures, (b) `performance.now()` from keypress `c` to card-in-DOM **< 3000ms** (leaving the human 7s of the 10s budget), (c) the created `.md` and optimized `.webp` exist on disk with correct frontmatter.
2. **`external-change.spec.ts` — the wow moment:** load board → test writes `status: testing` into `BUG-001.md` via `fs` → expect card to move columns and carry the flash attribute, toast appears. Also: open editor, make it dirty, mutate file on disk → ConflictBanner appears.
3. **`dnd.spec.ts`:** drag card between columns → frontmatter + board.json updated on disk; kill/restart assertion that UI state == disk state.
4. **`search.spec.ts`:** create 3 tickets, query body text, marked fragment shown, Enter navigates.

CI: vitest on every push; Playwright suite on the repo's cross-platform matrix (it doubles as the Go↔frontend contract test).

---

## 10. Dependency Budget (client, gz)

| Dep | ~Size | Why kept |
|---|---|---|
| Svelte 5 runtime | ~12 kB | framework |
| CodeMirror 6 (md setup) | ~120 kB | only serious embeddable editor; lazy-loaded on ticket route (`import()` — board/dashboard/modal never pay for it) |
| markdown-it + task-lists + DOMPurify | ~45 kB | preview pipeline |
| highlight.js (subset) | ~30 kB | code blocks in previews; lazy with CM6 chunk |
| svelte-dnd-action | ~15 kB | §5.2 |
| bits-ui (used parts) | ~20 kB | a11y dialogs/menus/tooltips |
| fuzzysort | ~2 kB | palette |
| Fonts (2 × woff2) | ~180 kB | self-hosted (CSP), cached forever |

Initial route JS target **< 150 kB gz** (editor chunk excluded). No runtime CSS-in-JS, no icon font — inline SVGs (Lucide, copied per-icon into `ui/icons`, tree-shaken by construction).

---

## 11. Backend Contract Requirements (summary for the Go design)

The frontend design above requires the server to provide, exactly:

1. `GET /api/snapshot` returning tickets **with bodies** + board + config + git + `seq`.
2. Content `hash` per ticket; `If-Match` honored on `PATCH`, `409 { current }` on mismatch.
3. `POST /api/board/move` updating frontmatter + `board.json` atomically.
4. SSE events with `origin` correlation (`opId` echo for API-driven writes vs `source: 'fs'` for watcher-only changes) — this single field powers the entire optimistic-reconciliation and wow-moment design.
5. Staging uploads (`POST /api/staging`, tokens redeemed at ticket creation, staged files kept in OS temp — **never** inside `.pine/`) with optimizer results (`originalSize`, final `size`, `mime`) in the response.
6. Static attachment serving at `/pine-files/attachments/...` and SPA fallback to embedded `index.html` for all other GETs.