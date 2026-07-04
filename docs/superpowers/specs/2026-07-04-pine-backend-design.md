# Pine — Go Backend & CLI Design

**Scope:** PRD v0.1 + v0.3 (core workspace + AI features). Single binary, no cgo, port 3412, disk is source of truth.

---

## 1. Guiding invariants

1. **Disk is truth.** Every read path ultimately derives from `.pine/` files. The in-memory cache, Bleve index, and browser state are all disposable projections, rebuilt from disk at any time.
2. **One write path.** All mutations (API, CLI) go through `internal/store`, which writes atomically. External writers (AI agents, `$EDITOR`) bypass the store — that's expected; the watcher reconciles.
3. **`.pine/` stays clean.** No index files, no lockfiles, no caches. Bleve is `NewMemOnly()`, rebuilt at startup (<10k tickets ⇒ sub-second). Temp files during atomic writes are dot-prefixed and short-lived.
4. **Idempotent event flow.** We do **not** distinguish self-writes from external writes. Every disk change — ours or an agent's — flows through the same watcher → cache → index → SSE pipeline. Applying the same state twice is a no-op; this kills all cache-coherence edge cases at the cost of one redundant re-read per own write.

---

## 2. Module & repo layout

Module: `github.com/izzadev/pine`, Go 1.24+.

```
pine/
├── go.mod
├── Makefile                      # web, build, test, dev targets
├── cmd/
│   └── pine/
│       └── main.go               # 5 lines: cli.Execute(version)
├── internal/
│   ├── cli/                      # cobra commands (one file per command)
│   │   ├── root.go               # root cmd, global --dir flag, version
│   │   ├── init.go  serve.go  open.go  context.go
│   │   ├── prompt.go  export.go  doctor.go
│   ├── config/                   # config.json + board.json load/save/defaults/validate
│   │   ├── config.go  board.go
│   ├── ticket/                   # pure domain: parse/serialize, no I/O
│   │   ├── ticket.go  frontmatter.go  sections.go  id.go
│   ├── store/                    # filesystem store: CRUD, ID alloc, attachments, templates
│   │   ├── store.go  create.go  write.go  attachments.go  templates.go
│   ├── media/                    # attachment optimizer (image → WebP)
│   │   └── optimize.go
│   ├── search/                   # Bleve mem-only index
│   │   └── index.go
│   ├── gitx/                     # Git interface + go-git impl + CLI impl
│   │   ├── gitx.go  gogit.go  cligit.go  cache.go
│   ├── watch/                    # fsnotify + debounce → normalized change events
│   │   └── watcher.go
│   ├── server/                   # chi router, handlers, SSE hub, static serving
│   │   ├── server.go  routes.go  tickets.go  board.go
│   │   ├── attachments.go  search.go  git.go  sse.go  errors.go
│   ├── contextgen/               # pine context / pine prompt markdown builders
│   │   ├── context.go  prompt.go  defaults/fix.md (go:embed)
│   └── doctor/                   # checks, shared by `pine doctor`
│       └── doctor.go
├── web/                          # SvelteKit (Svelte 5) app, adapter-static
│   ├── src/ ... svelte.config.js
│   ├── build/                    # adapter-static output (gitignored)
│   └── embed.go                  # package web; //go:embed all:build
└── testdata/                     # (per-package testdata/ dirs, see §12)
```

**Frontend embed.** `web/embed.go`:

```go
package web

import "embed"

//go:embed all:build
var Assets embed.FS   // Assets contains "build/..."
```

`make build` = `npm --prefix web run build && go build ./cmd/pine`. SvelteKit uses `adapter-static` with `fallback: "index.html"` (SPA mode). A `web/build/.gitkeep`-style stub plus a `//go:build dev` variant of `embed.go` (empty FS) lets `go build -tags dev` succeed without the frontend; `pine serve --dev` reverse-proxies all non-`/api`, non-`/attachments` requests to the Vite dev server at `http://localhost:5173`.

**Dependencies** (all pure Go, no cgo):

| Package | Use |
|---|---|
| `github.com/spf13/cobra` | CLI |
| `github.com/go-chi/chi/v5` | Router |
| `github.com/blevesearch/bleve/v2` | Search (scorch, mem-only) |
| `github.com/go-git/go-git/v5` | Git (behind interface) |
| `github.com/fsnotify/fsnotify` | Watcher |
| `gopkg.in/yaml.v3` | Frontmatter (yaml.Node for order/round-trip) |
| `golang.org/x/image` | `draw` (CatmullRom resize), `webp` decode |
| `github.com/gen2brain/webp` | **Lossy WebP encode via wazero (WASM libwebp — no cgo)** |
| `github.com/rwcarlsen/goexif` | Read JPEG EXIF orientation before re-encode |

Binary will land ~40–60 MB (bleve + go-git + wazero). Acceptable for a dev tool; note in README.

---

## 3. Domain model & file formats

### 3.1 Ticket struct (`internal/ticket`)

```go
type Ticket struct {
    ID       string      // "BUG-001"
    Type     string      // "BUG" — derived from ID prefix, not stored in frontmatter
    Title    string
    Status   string      // authoritative; must match a board column
    Priority string
    Labels   []string
    Created  time.Time   // RFC3339 UTC
    Updated  time.Time
    Extra    []ExtraField // unknown frontmatter keys, order-preserved (agents may add fields)
    Body     string      // raw markdown after closing '---', verbatim
}

type ExtraField struct{ Key string; Node *yaml.Node }
```

**Key decision: the body is opaque.** We never parse body sections into struct fields and re-render them. `Parse` → `Serialize` round-trips byte-identically for the body. The UI edits the body wholesale (markdown editor), so nothing is lost. Section access for search/prompts is read-only:

```go
// sections.go
// Sections splits on level-1 ATX headings ("# Description").
func Sections(body string) []Section          // Section{Heading, Content string}
func Section(body, heading string) (string, bool)  // case-insensitive lookup
func RelatedFiles(body string) []string        // "- path" bullets under "# Related Files"
func AttachmentRefs(body string) []string      // links under "# Attachments"
```

### 3.2 Frontmatter parse/serialize (`frontmatter.go`)

- `Parse(raw []byte) (*Ticket, error)`: file must begin `---\n` (tolerate CRLF and a UTF-8 BOM); find the closing `\n---\n`; `yaml.Unmarshal` the block into a `yaml.Node` document; walk the mapping — known keys (`id,title,status,priority,labels,created,updated`) into struct fields, unknown keys retained in `Extra` in original order. Timestamps parsed leniently: RFC3339, `2006-01-02`, or empty → zero value. Missing/blank `id` or `title` ⇒ `*ParseError`.
- `Serialize(t *Ticket) []byte`: build a `yaml.Node` mapping with children in **canonical order** — `id, title, status, priority, labels, created, updated`, then `Extra` fields — and `yaml.Marshal` the node (yaml.v3 handles quoting). Emit `---\n<yaml>---\n\n<Body>`. Canonical order + node-based marshal ⇒ stable diffs, correct quoting, unknown-key preservation.

Canonical file (also the doctor's reference format):

```markdown
---
id: BUG-001
title: Login button not working
status: todo
priority: high
labels:
  - login
  - ui
created: 2026-07-04T10:12:00Z
updated: 2026-07-04T10:12:00Z
---

# Description
...
```

### 3.3 IDs (`id.go`)

`^([A-Z][A-Z0-9]*)-([0-9]+)$`. Format: `fmt.Sprintf("%s-%03d", prefix, n)` (min-width 3, grows naturally to `BUG-1000`). `SplitID(id) (prefix string, n int, err error)`.

**Allocation — no counter file** (a counter would be merge-conflict bait). `store.Create`:

1. Hold the store's write mutex (serializes all in-process creators — API + CLI share one process).
2. `os.ReadDir(tickets/)`, regex-match names, compute `max(n)+1` for the prefix.
3. `os.OpenFile(path, O_CREATE|O_EXCL|O_WRONLY, 0644)` — if `ErrExist` (an AI agent created the same ID between scan and create), increment and retry, max 100 attempts.

O_EXCL is the cross-process guard; the scan is O(dir-entries) on names only — trivial at <10k files. Residual risk: two **git branches** each mint `BUG-042` and merge — undetectable at write time; `pine doctor` reports duplicate IDs and `id ≠ filename` mismatches.

### 3.4 Status model — resolving the dual source of truth

**Frontmatter `status` is authoritative for a ticket's column. `board.json` owns only column definitions and per-column manual ordering.** Valid statuses are defined **solely** by `board.json` columns (config.json does *not* repeat them — no third source). Drag-and-drop = `PATCH /api/tickets/{id}` (rewrites frontmatter) **plus** `PUT /api/board/order` for position. Deleting `board.json` loses only manual ordering; tickets are untouched.

### 3.5 `config.json` (exact schema)

```json
{
  "version": 1,
  "project": { "name": "my-app" },
  "types": [
    { "prefix": "BUG",  "name": "Bug" },
    { "prefix": "FEAT", "name": "Feature" }
  ],
  "priorities": ["low", "medium", "high", "critical"],
  "attachments": {
    "optimize": true,
    "maxImageDimension": 2000,
    "webpQuality": 80,
    "videoWarnSizeMB": 50
  },
  "git": { "backend": "gogit" }
}
```

`git.backend`: `"gogit" | "cli"` — escape hatch per locked decision 6. Unknown JSON keys are preserved on save (decode into `map[string]json.RawMessage` overlay, write back known + unknown).

### 3.6 `board.json` (exact schema)

```json
{
  "version": 1,
  "columns": [
    { "status": "todo",    "title": "Todo" },
    { "status": "doing",   "title": "Doing" },
    { "status": "testing", "title": "Testing" },
    { "status": "done",    "title": "Done" }
  ],
  "order": {
    "todo":  ["FEAT-002", "BUG-003"],
    "doing": []
  }
}
```

One status per column (YAGNI: no status-groups). `order` lists ticket IDs per column, top-first; tickets with that status not listed are appended sorted by `updated` desc; stale IDs are pruned whenever `order` is next written (never proactively — keeps board.json diffs quiet).

### 3.7 Templates & prompts

- `.pine/templates/bug.md`, `feature.md`: **body-only** markdown skeletons (the `# Description … # Attachments` sections). Frontmatter is always generated by the store. On create with empty body, store loads `templates/<type-name-lowercased>.md` if present, else a built-in default.
- `.pine/prompts/fix.md`: Go `text/template` over `contextgen.PromptData` (fields: `.Ticket`, `.Sections`, `.RepoSummary`, `.Git`, `.Attachments`). `pine prompt` uses this file if present, else the embedded default (`internal/contextgen/defaults/fix.md`).

---

## 4. Store layer (`internal/store`)

```go
type Store struct {
    root string          // abs path to .pine
    mu   sync.RWMutex
    cache map[string]*ticket.Ticket  // id → parsed ticket, disk mirror
    cfg   *config.Config
    board *config.Board
}

func Open(pineDir string) (*Store, error)         // loads config, board, scans+parses all tickets
func (s *Store) List(f Filter) []*ticket.Ticket    // from cache; Filter{Status, Type, Label string}
func (s *Store) Get(id string) (*ticket.Ticket, error)
func (s *Store) Create(req CreateReq) (*ticket.Ticket, error)   // §3.3 alloc; sets Created/Updated
func (s *Store) Update(id string, mut func(*ticket.Ticket) error) (*ticket.Ticket, error)
func (s *Store) Delete(id string) error            // removes .md + attachments/<id>/ recursively
func (s *Store) Reload(path string) (ChangeKind, *ticket.Ticket, error) // watcher entry point
func (s *Store) Attachments(id string) []AttachmentInfo   // ReadDir attachments/<id>/
func (s *Store) SaveAttachment(id, name string, r io.Reader) (AttachmentInfo, error)
func (s *Store) DeleteAttachment(id, name string) error
func (s *Store) Board() *config.Board
func (s *Store) SetBoardOrder(column string, ids []string) error
func (s *Store) Config() *config.Config
```

**Atomic writes** (`write.go`): write to `filepath.Join(dir, ".tmp-"+base+"-"+rand)`, `f.Sync()`, `os.Rename` onto target (Go's `os.Rename` replaces existing files on Windows via `MOVEFILE_REPLACE_EXISTING`). Dot-prefix means the watcher and doctor ignore in-flight temp files. Same routine for tickets, config.json, board.json.

**Update semantics:** last-write-wins. `Update` re-reads nothing from disk (cache is watcher-fresh); it applies `mut`, bumps `Updated`, serializes, atomic-writes, updates cache synchronously (so the API response is immediately consistent), and lets the watcher's subsequent idempotent re-read drive SSE + search (§1.4). No optimistic-locking headers in v1 — single user, live sync makes conflicts visible instantly.

**Attachment storage:** `attachments/<ID>/<filename>`. Filenames: slugified original (`[a-z0-9._-]`, collapse dashes), collision → `name-2.ext`; pasted blobs → `paste-20260704-153012.webp`. `SaveAttachment` runs the optimizer (§9) for PNG/JPEG when `attachments.optimize`, then atomic-writes. Path traversal blocked: reject any name that changes after `filepath.Base` + slugify.

---

## 5. Watcher & live sync (`internal/watch`, `internal/server/sse.go`)

### 5.1 Watcher

fsnotify on `.pine/`, `.pine/tickets/`, `.pine/attachments/`, plus every `attachments/<ID>/` subdir (fsnotify is non-recursive; on `Create` of a directory under `attachments/`, add a watch). Ignore: names starting with `.`, `~` suffixes, `.swp`/`.swx`, `4913` (vim), anything under `templates/` and `prompts/` except broadcast-as-config-class below.

**Debounce:** per-path timer, 150 ms; events for the same path coalesce; a flush emits a batch. Handling is uniform for Create/Write/Rename/Remove/Chmod: **stat the path** — exists → `upsert`, missing → `delete`. (Renames onto existing files from our atomic writes surface as Create on the target; we don't care which op it was.)

```go
type Event struct {
    Kind Kind     // KindTicket, KindAttachment, KindConfig, KindBoard
    Op   Op       // OpUpsert, OpDelete
    Path string   // abs
    ID   string   // ticket id when derivable (from filename or parent dir)
}
func New(pineDir string) (*Watcher, error)
func (w *Watcher) Events() <-chan []Event   // debounced batches
```

### 5.2 Coordinator (in `server.go`)

One goroutine consumes batches:

- `KindTicket/OpUpsert` → `store.Reload(path)`; parse OK → update cache, `search.Upsert`, broadcast `ticket.updated` (or `.created` if new to cache). Parse error → remove from cache/index, broadcast `ticket.invalid`.
- `KindTicket/OpDelete` → drop cache + index, broadcast `ticket.deleted`.
- `KindAttachment` → broadcast `ticket.updated` for the owning ID (attachment list is computed on read).
- `KindConfig` / `KindBoard` → reload file; broadcast `config.updated` / `board.updated`. Board reload revalidates ticket statuses lazily (unknown status ⇒ ticket shows in a UI "unmapped" tray; backend doesn't rewrite tickets on its own).

### 5.3 SSE endpoint & event schema

`GET /api/events` — `text/event-stream`. Hub: register/unregister channels (buffer 64); a slow client whose buffer fills is dropped (it will reconnect and resync). Heartbeat comment `: ping` every 25 s. Monotonic `id:` per event; **no replay** — on `open`/reconnect the client must refetch `/api/tickets`, `/api/board`, `/api/config` (documented contract; Last-Event-ID ignored).

Wire format — `event:` carries the type, `data:` a JSON object:

```
id: 42
event: ticket.updated
data: {"ticket": { ...full ticket JSON, §7.2... }}
```

| event | data payload |
|---|---|
| `ticket.created` | `{"ticket": Ticket}` |
| `ticket.updated` | `{"ticket": Ticket}` |
| `ticket.deleted` | `{"id": "BUG-001"}` |
| `ticket.invalid` | `{"path": ".pine/tickets/BUG-002.md", "error": "yaml: line 3: ..."}` |
| `board.updated`  | `{"board": Board}` |
| `config.updated` | `{"config": Config}` |
| `git.updated`    | `{"git": GitStatus}` (§8) |

---

## 6. Search (`internal/search`)

`bleve.NewMemOnly(mapping)`; rebuilt in `Open` by indexing the store cache; incrementally maintained by the coordinator (`Upsert(t)`, `Delete(id)`).

**Document mapping** (custom `IndexMapping`, default analyzer `standard`, `en` stemming off — code-ish terms):

| field | type | notes |
|---|---|---|
| `id` | keyword | exact match, boost 10 |
| `title` | text | boost 3 |
| `body` | text | full body markdown |
| `labels` | keyword | |
| `relatedFiles` | text (simple analyzer) | from `ticket.RelatedFiles` — satisfies "search by file" |
| `status`, `priority`, `type` | keyword | filter-only (`IncludeInAll: false`) |

**Query** (`Search(q string, f Filter, limit int)`): a `ConjunctionQuery` of (a) optional filter `TermQuery`s for status/type/priority, and (b) a `DisjunctionQuery` over the user text:

- `TermQuery(upper(q))` on `id`, boost 10 (typing "bug-3" jumps to the ticket)
- `MatchQuery` on `title`, fuzziness 1, boost 3
- `PrefixQuery` on `title`, boost 2 (live-as-you-type in ⌘K)
- `MatchQuery` on `body`, fuzziness 1
- `MatchQuery` on `labels` and `relatedFiles`

Default limit 20, highlight (simple highlighter) on `title`+`body`. Result: `[]Hit{ID, Score, Fragments map[string][]string}` — handlers join with the store cache for display fields.

---

## 7. HTTP API (`internal/server`)

### 7.1 Server setup

`chi.NewRouter()` with `middleware.Recoverer`, a minimal request logger (dev only), `middleware.RequestSize(1 << 30)` on upload routes. Bind `127.0.0.1:3412` (flags `--port`, `--host`; localhost-only by default — that is the auth model). No CORS (same-origin).

### 7.2 Ticket JSON shape

```json
{
  "id": "BUG-001", "type": "BUG",
  "title": "Login button not working",
  "status": "todo", "priority": "high",
  "labels": ["login", "ui"],
  "created": "2026-07-04T10:12:00Z", "updated": "2026-07-04T11:00:00Z",
  "body": "# Description\n...",
  "attachments": [
    {"name": "login.png", "size": 48210, "url": "/attachments/BUG-001/login.png",
     "markdownPath": "../attachments/BUG-001/login.png", "kind": "image"}
  ],
  "path": ".pine/tickets/BUG-001.md",
  "extra": {"assignee": "claude"}
}
```

`markdownPath` is the canonical in-body reference — relative from `tickets/`, so links render on GitHub and in VS Code; the web UI rewrites `../attachments/…` → `/attachments/…` when rendering.

### 7.3 Route table

| Method & path | Purpose | Request → Response |
|---|---|---|
| `GET /api/health` | liveness/version | → `{"ok":true,"version":"0.1.0","project":"my-app"}` |
| `GET /api/tickets` | list | query `status`,`type`,`label` (repeatable), `sort=updated\|created\|priority` → `{"tickets":[Ticket]}` (bodies included; <10k tickets, it's fine — UI may pass `fields=summary` to omit `body`) |
| `POST /api/tickets` | create (≤10 s flow) | `{"type":"BUG","title":"...","priority":"high","labels":[],"body":""}` (empty body ⇒ template) → `201 Ticket` |
| `GET /api/tickets/{id}` | read | → `Ticket` |
| `PUT /api/tickets/{id}` | full update | `{"title","status","priority","labels","body"}` → `Ticket` |
| `PATCH /api/tickets/{id}` | partial (drag-drop, inline edits) | any subset of PUT fields → `Ticket` |
| `DELETE /api/tickets/{id}` | delete ticket + its attachments dir | → `204` |
| `GET /api/tickets/{id}/prompt` | generated fix prompt | → `text/markdown` (contextgen.Prompt) |
| `GET /api/board` | columns + ordering | → `{"columns":[{"status","title"}],"order":{...},"unmapped":["OLD-001"]}` |
| `PUT /api/board/order` | persist manual order | `{"column":"todo","ids":["BUG-003","FEAT-002"]}` → `204` |
| `POST /api/tickets/{id}/attachments` | upload (multipart, field `file`, repeatable) | → `201 {"attachments":[AttachmentInfo], "warnings":["mobile.mp4 is 87MB (>50MB)"]}` |
| `DELETE /api/tickets/{id}/attachments/{name}` | remove file | → `204` |
| `GET /api/search` | search | `?q=&status=&type=&limit=` → `{"hits":[{"id","score","title","status","fragments":{...}}]}` |
| `GET /api/git` | git status | → `GitStatus` (§8) |
| `GET /api/config` | read config | → config.json content |
| `PUT /api/config` | write config | full config → `200` (validated; unknown keys preserved) |
| `GET /api/context` | generated context.md | → `text/markdown` (UI "Copy AI context" button — QA-without-terminal metric) |
| `GET /api/events` | SSE stream | §5.3 |
| `GET /attachments/{id}/{name}` | serve attachment | sanitized (`chi` params, reject `..`), `http.ServeContent` (Range works for MP4/MOV scrubbing) |
| `GET /*` | embedded UI | serve from `web.Assets` (sub-FS `build`); on miss and non-`/api`, non-`/attachments` → `index.html` (SPA fallback); `Cache-Control: no-cache` on index, immutable on `_app/*` hashed assets |

Validation on create/PUT/PATCH: `type` prefix must exist in config, `status` in board columns, `priority` in config list; violations → `422 validation_failed` with per-field details.

---

## 8. Git integration (`internal/gitx`)

```go
type Client interface {
    IsRepo() bool
    CurrentBranch() (string, error)              // "main", or short SHA when detached
    Status() (WorkStatus, error)                  // dirty worktree summary
    RecentCommits(n int) ([]Commit, error)
}

type WorkStatus struct {
    Dirty   bool
    Changes []Change // {Path string; Code string} Code ∈ "M","A","D","R","??"
}
type Commit struct { Hash, Subject, Author string; When time.Time }
```

- `gogit.go` — default (`git.backend: "gogit"`): `git.PlainOpenWithOptions(dir, DetectDotGit: true)`; branch via `Head()`; commits via `repo.Log(&LogOptions{})` taking `n`; status via `worktree.Status()`.
- **Known pitfall:** `worktree.Status()` hashes the entire worktree (no index-mtime shortcut, no fsmonitor) — seconds-slow on big repos. Mitigations, in order: (1) **`cache.go` wraps any `Client` with a 5 s TTL cache**, computed off-request (see poller below), so HTTP handlers never block on it; (2) `Changes` capped at 100 entries + a `truncated` flag; (3) `cligit.go` — `git.backend: "cli"` shells out to `git rev-parse --abbrev-ref HEAD`, `git status --porcelain=v2 -z`, `git log -n --format=%H%x00%s%x00%an%x00%aI`, selected in config when go-git bites. The interface (locked decision 6) makes this a config flip, not a refactor.
- **Poller:** goroutine every 5 s recomputes branch + status + last 10 commits, compares a hash of the result, broadcasts `git.updated` SSE only on change. `GET /api/git` serves the cached snapshot instantly.

`GitStatus` JSON:

```json
{ "isRepo": true, "branch": "main", "dirty": true,
  "changes": [{"path": "src/login.tsx", "code": "M"}], "truncated": false,
  "commits": [{"hash": "abc1234", "subject": "fix: guard null session",
               "author": "Izza", "when": "2026-07-04T09:41:00Z"}] }
```

Not a repo ⇒ `{"isRepo": false}` and everything git-related degrades gracefully (UI hides the panel, context.md omits the section).

---

## 9. Attachment optimizer (`internal/media`)

`Optimize(name string, in []byte, cfg config.Attachments) (outName string, out []byte, warns []string)`

Per-type policy:

| Input | Action |
|---|---|
| PNG, JPEG | Decode (stdlib). JPEG: read EXIF orientation (goexif), apply rotation. Downscale if long edge > `maxImageDimension` (2000) via `x/image/draw.CatmullRom`. Encode lossy WebP `q=webpQuality` (80) with `gen2brain/webp` (wazero — no cgo). **Re-encode inherently strips EXIF/GPS.** Keep result only if `len(out) < len(in)`; else keep original bytes/name unchanged. On success, extension becomes `.webp`. |
| GIF | Pass through untouched (usually animated recordings; animated re-encode is out of scope). |
| WEBP | Pass through (already efficient). |
| MP4, MOV | Pass through; append warning if size > `videoWarnSizeMB`. |
| anything else | Reject → `415 unsupported_media_type` (sniffed via `http.DetectContentType` + extension, not trust-the-client). |

`attachments.optimize: false` disables the PNG/JPEG branch entirely. Optimizer runs synchronously inside the upload handler (a 2000px WebP encode is ~100–300 ms — fine for the <10 s flow; multipart files processed sequentially).

---

## 10. CLI (`internal/cli`, cobra)

Global flag `--dir` (default `.`): locate `.pine/` by walking up from dir to the git root / filesystem root (like git does), so `pine` works from subdirectories. Root command prints help. Exit codes: `0` ok, `1` error (doctor: problems found), `2` usage.

### `pine init`
Creates `.pine/{config.json, board.json, tickets/, attachments/, templates/bug.md, templates/feature.md, prompts/fix.md}` with defaults (§3.5–3.7; project.name = directory basename). Idempotent: existing files are never overwritten; missing ones are filled in (reported per-file: `created` / `exists`). Warns (doesn't fail) if not inside a git repo, and if `.pine` matches a `.gitignore` rule (data is meant to be committed). Prints: `Run 'pine open' to start.`

### `pine serve`
Flags: `--port 3412`, `--host 127.0.0.1`, `--open`, `--dev` (Vite proxy). Boot order: `store.Open` (load config/board, parse all tickets, report invalid files to stderr but keep serving) → build Bleve index → start watcher + coordinator + git poller → `http.Server`. Prints `Pine serving <name> on http://127.0.0.1:3412`. SIGINT/SIGTERM → `server.Shutdown(ctx 5s)`, close watcher. Port busy → clear error suggesting `--port`.

### `pine open`
Alias for `pine serve --open`, except: first probes `GET /api/health` on the target port; if a Pine for **this** project is already running, just opens the browser and exits. Browser launch: `open` / `xdg-open` / `rundll32 url.dll,FileProtocolHandler` per GOOS.

### `pine context`
Prints to **stdout** (composable: `pine context | pbcopy`, or an agent runs it directly); `--out <path>` writes a file instead. Structure (built by `contextgen.Context`):

```markdown
# Project Context: my-app
> Generated by Pine v0.1.0 · 2026-07-04T12:00:00Z

## Repository
- Branch: `main` · uncommitted changes: 3 files
- Modified: `src/login.tsx`, `src/api/auth.ts`, …

## Recent Commits
- `abc1234` fix: guard null session — Izza, 2026-07-04

## Critical & High Priority (open)
### BUG-004 · Payment fails on retry · status: doing · priority: critical
Labels: payments. Related files: src/pay.ts
> First 2 non-empty lines of Description…

## Open Tickets
| ID | Title | Status | Priority | Labels |
|----|-------|--------|----------|--------|
| BUG-003 | … | todo | high | ui |

## In Testing
(same table shape — what a QA/verify agent should look at)

## Recently Done (last 7 days)
- FEAT-002 Dark mode (done 2026-07-02)

## Conventions
- Tickets live in `.pine/tickets/*.md` (frontmatter: id/title/status/priority/labels).
- To update a ticket, edit its file; set `status` to move it on the board.
- Attachments: `.pine/attachments/<ID>/`.
```

The Conventions block is what makes "AI understands the project by reading `.pine/`" true in practice — it teaches the agent to write back.

### `pine prompt <ID>`
Stdout; `--out` optional. Rendered from `.pine/prompts/fix.md` (or embedded default) with `PromptData`:

```markdown
# Fix Request: BUG-021 — Login button not working

## Repository Summary
Project: my-app · Branch: main · 3 uncommitted files
Recent commits: …(5)

## Issue
**Status:** testing · **Priority:** high · **Labels:** login, ui

### Description / ### Steps to Reproduce / ### Expected / ### Actual
(verbatim from the ticket's sections)

## Related Files
- src/login.tsx

## Attachments
- .pine/attachments/BUG-021/login.png (image, 48 KB)

## Suggested Fix
(included only if the ticket body has a "# Suggested Fix" section)

## Acceptance Criteria
- The behavior described in **Expected** occurs when following **Steps**.
- No regressions in the Related Files listed above.

## When Done
- Edit `.pine/tickets/BUG-021.md`: set `status: testing` and update `updated`.
- Note your changes under a `# Fix Notes` section in the ticket.
```

### `pine export`
`--format md|json` (default `md`), `--out` (default stdout). `md`: all tickets concatenated, grouped by board column, each as `## [ID] Title` + metadata line + body. `json`: array of ticket objects (§7.2 shape, minus `url` fields). No zip — attachments already live in the repo.

### `pine doctor`
Read-only; ✓/!/✗ per check; exit 1 if any ✗. Checks (`internal/doctor`):

1. `.pine/` exists; `config.json`, `board.json` parse and validate (versions, non-empty columns/types).
2. Git: repo detected; `.pine/` not ignored; warns if `.pine/tickets` has uncommitted files older than 7 days ("you may want to commit your tickets").
3. Every `tickets/*.md` parses; required fields present; `id` matches filename; `status` ∈ board columns; `priority` ∈ config; timestamps parseable.
4. Duplicate IDs (post-merge hazard, §3.3).
5. Attachment integrity: every link under each ticket's `# Attachments` resolves to a file; orphan `attachments/<ID>/` dirs with no ticket; unsupported file types present.
6. `board.json` order references only existing tickets.

---

## 11. Error handling conventions

- **Sentinels + wrapping:** `store.ErrNotFound`, `store.ErrExists`, `ticket.ErrInvalidID`; rich errors as types (`*ticket.ParseError{Path, Line, Msg}`, `*config.ValidationError{Field, Msg}`). Always `fmt.Errorf("…: %w", err)`; no error libraries.
- **HTTP mapping** in one place (`server/errors.go`): `writeErr(w, err)` switches on `errors.Is/As` → status + body `{"error":{"code":"not_found","message":"ticket BUG-999 not found","details":{}}}`. Codes: `not_found`, `already_exists`, `validation_failed` (422), `unsupported_media_type` (415), `parse_error` (500 with path), `internal` (500, message logged not leaked).
- **CLI:** errors to stderr prefixed `pine: `; cobra `SilenceUsage: true` so runtime errors don't dump help.
- **Watcher/coordinator:** never crash on bad files — log, emit `ticket.invalid`, continue. A malformed ticket must not take down the server (AI agents will write malformed tickets).

---

## 12. Testing strategy

| Layer | Approach |
|---|---|
| `ticket` | Table-driven parse/serialize with golden files in `internal/ticket/testdata/` (valid, CRLF, BOM, unknown keys, missing fields, no frontmatter). Property: `Serialize(Parse(x))` is stable after one normalization pass; body round-trips byte-identically. |
| `store` | Fixture tree `internal/store/testdata/pine-basic/` (a complete fake `.pine/` with 6 tickets, attachments, templates), copied to `t.TempDir()` via `os.CopyFS`. Table-driven CRUD; ID-race test: N goroutines × `Create` ⇒ unique IDs; O_EXCL retry test by pre-creating the next filename externally. |
| `media` | Golden inputs (tiny PNG/JPEG incl. EXIF-rotated); assert dimension cap, output < input or passthrough, orientation applied, `.webp` naming. |
| `search` | Index fixture set; table of query → expected top hit (id-exact, fuzzy title, file-path term, filtered). |
| `gitx` | Build a throwaway repo in `t.TempDir()` with go-git (init, commit ×3, dirty a file); assert branch/status/commits; run same table against both `gogit` and `cligit` impls (cli cases skipped if no `git` in PATH). |
| `server` | `httptest.NewServer` over a real store in temp dir — full vertical tests (no store mocks): create→patch status→board order→upload→search→delete; SSE test: connect, mutate a file **directly on disk**, assert `ticket.updated` arrives (this is the live-sync contract test). |
| `watch` | Real fsnotify in temp dir; write/rename/delete; assert debounced batch shape with eventually-style polling (generous timeouts for CI). |
| `contextgen`, `doctor` | Golden markdown/output over the fixture tree (timestamps injected via a `now func() time.Time` field for determinism). |
| e2e (make target) | Build binary; `pine init && pine doctor && pine serve --port 0`-style smoke against `/api/health`. |

---

## 13. Startup sequence (wiring summary)

```
pine serve
 └─ findPineDir(walk up) → config.Load → store.Open (parse all, cache)
    → search.Build(cache) → gitx.New(cfg.Git.Backend) + cache wrapper
    → watch.New(.pine) ─┐
    → server.New(store, search, gitx, hub) ── chi routes ── web.Assets
    → coordinator goroutine: watch.Events → store.Reload → search.Upsert/Delete → hub.Broadcast
    → git poller goroutine (5s) → hub.Broadcast(git.updated)
    → ListenAndServe 127.0.0.1:3412 · Shutdown on SIGINT
```

Everything downstream of `.pine/` is derived and disposable; the only durable artifacts Pine ever produces are markdown, JSON, and media files a human would want to commit.