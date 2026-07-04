# Pine Pre-Mortem — Failure Modes & Prescribed Resolutions

Skeptical staff-engineer review. Each item: **risk → binding resolution**. These are decisions, not options.

## 1. Concurrent writes

**1.1 UI save silently clobbers an AI agent's file edit (or vice versa).**
Last-write-wins is unacceptable when the PRD's core loop is "agent edits files while human watches UI." Resolution: every `GET /api/tickets/{id}` returns `ETag: sha256(file bytes)`. `PUT` requires `If-Match`; on mismatch return `409` with the current disk content in the body. UI shows a "changed on disk" banner with Reload / Overwrite — no auto-merge in v1. Use content hash, NOT mtime (mtime has 1s granularity on some filesystems and changes without content change).

**1.2 Autosave-on-keystroke turns the editor into a clobbering machine.**
Continuous autosave fights agents, spams the watcher, bumps `updated` constantly, and pollutes git diffs. Resolution: explicit save (Cmd+S/button) plus idle-debounced autosave at 2s, always through the If-Match path. While the editor is dirty and an external change arrives via SSE, freeze autosave and show the conflict banner.

**1.3 ID allocation race — two creators mint BUG-007 simultaneously.**
Resolution: **filename is the canonical ID**; frontmatter `id` is advisory (server rewrites it to match filename on next save; `pine doctor` flags mismatches). Server-side creation: mutex around scan-max, then `os.OpenFile(O_CREATE|O_EXCL)` — the exclusive create is the real lock; on failure increment and retry. Agents creating files by hand can still race out-of-band; don't add lockfiles to `.pine/` (pollution) — doctor detects duplicate frontmatter IDs and the UI shows both files rather than dropping one. Parse ticket numbers numerically (`BUG-1000` after `BUG-999` is legal; never sort IDs lexicographically).

**1.4 Server rewrites destroy agent-authored file content.**
A kanban drag that re-serializes the whole file through a struct will reorder/drop unknown frontmatter keys and reflow the body → massive git noise, destroyed agent context. Resolution: parse frontmatter into an order-preserving map that retains unknown keys; on status change rewrite ONLY the frontmatter block (fixed key order, unknown keys appended verbatim) and copy body bytes untouched. Write LF always; tolerate CRLF on read.

## 2. Watcher correctness

**2.1 Event-type logic breaks under editor atomic saves.**
vim/VS Code save via write-temp + rename: you'll see Remove/Rename then Create, and naive "Remove = ticket deleted" flashes deletions into the UI. Resolution: treat every fsnotify event as only "something happened at this path." Coalesce per-path events in a 150ms debounce window, then reconcile by re-statting/re-reading the file; absent after the window → real delete. Never branch on event type.

**2.2 SSE echo loop: server's own write → fsnotify → SSE → client refetch → flicker.**
Do NOT build a "recently-written-by-me" suppression map (TOCTOU-riddled). Resolution: the store dedupes by content hash — every change (API write or watcher reconcile) re-reads, hashes, and emits SSE only if the hash differs from the cached one. Server write updates cache and emits exactly one event; the trailing watcher event hashes equal and is dropped. Frontend treats SSE as invalidation hints and refetches — never as ordered state deltas. This also makes EventSource reconnects trivial: on reconnect, refetch the ticket list; no Last-Event-ID machinery.

**2.3 fsnotify is not recursive, and watching every attachment dir exhausts kqueue fds on macOS.**
fsnotify uses kqueue on macOS (one fd per watched path) — watching `attachments/<ID>/` per ticket blows the fd budget. Resolution: watch exactly two dirs, `.pine/` and `.pine/tickets/`. Do NOT watch attachment subdirs at all: attachments only change via the server (which already knows) or rare agent action; list them with a readdir on ticket `GET` and let `pine doctor` catch orphans. If `.pine/tickets/` is created after startup (Create event on `.pine/`), add the watch then. Handle `.pine/` itself being deleted: enter an error state in the UI, don't crash.

**2.4 Editor droppings parsed as tickets.**
`.swp`, `file~`, vim's `4913`, `.DS_Store` land in `tickets/`. Resolution: the store only parses files matching `^[A-Z]+-\d+\.md$`; everything else is ignored by the watcher and listed by `pine doctor` as strays.

## 3. YAML frontmatter robustness

**3.1 One malformed agent-written file must never nuke the board — and must never silently vanish.**
Silent disappearance is the worst failure: agent creates a ticket, human never sees it. Resolution: lenient parse with per-field fallbacks, applied **in memory only** (never auto-rewrite the file; `pine doctor` reports every fallback taken). Exact policy:
- `status` missing/unknown → `todo`; `priority` ∉ {low,medium,high,critical} → `medium`; `labels` as scalar string → wrap in 1-element array; `title` missing → filename.
- `created`/`updated` missing/unparseable → file mtime; for sorting always use `max(frontmatter.updated, mtime)` since agents won't bump `updated`.
- Whole frontmatter unparseable (yaml.v3 errors hard on duplicate keys — catch it), or non-UTF8 body → render a "degraded" ticket: title = filename, status = todo, raw body shown read-only, excluded from Bleve, prominent doctor error. Tolerate `\r` and trailing whitespace on the `---` delimiters.

## 4. board.json vs frontmatter

**4.1 Two sources of truth for status/position = guaranteed drift.**
Resolution: frontmatter `status` is the **only** source of a ticket's column. `board.json` defines columns only: `{"version":1,"columns":[{"id":"todo","name":"Todo"},...]}` — it never contains ticket IDs. Within-column order is computed (priority desc, then updated desc); **cut manual card reordering from v1** — it's the thing that would force ticket arrays into board.json. Status matching no column → synthetic "Other: <status>" column rendered at far right, doctor flag, file untouched. Never auto-rewrite a ticket to "fix" its status.

## 5. Localhost security

**5.1 Binding all interfaces by accident.**
`http.ListenAndServe(":3412")` exposes the repo to the LAN. Resolution: hardcode listen on `127.0.0.1:3412`. No `0.0.0.0` flag in v1.

**5.2 Drive-by CSRF and DNS rebinding against a no-auth API.**
Any web page can `fetch("http://localhost:3412/api/...", {method:"POST"})`, and rebinding defeats same-origin for reads. Resolution: middleware on ALL routes rejects unless `Host` ∈ {`localhost:3412`,`127.0.0.1:3412`,`[::1]:3412`} (kills rebinding); on non-GET, `Origin` header must be absent or match `http://localhost:3412` / `http://127.0.0.1:3412`, else 403. Send zero CORS headers (default-deny). This is ~30 lines; do not skip it because "it's localhost."

**5.3 Path traversal via attachment filenames and ticket IDs.**
Resolution: never trust client filenames — sanitize to `[A-Za-z0-9._-]`, reject `..`, empty, `:` (Windows ADS), and Windows reserved names (CON, NUL, …). Route `/api/attachments/{id}/{file}` validates `id` against `^[A-Z]+-\d+$`, then `filepath.Clean` + verify the resolved absolute path is prefixed by `.pine/attachments/`. Serve with `X-Content-Type-Options: nosniff` and Content-Type strictly from the extension whitelist (png/jpeg/gif/webp/mp4/mov); anything else gets `Content-Disposition: attachment`.

**5.4 Markdown XSS from agent-written ticket bodies.**
Agents will paste HTML into markdown. Resolution: render client-side, sanitize with DOMPurify `USE_PROFILES: {html: true}` (excludes SVG/MathML mXSS vectors) plus `FORBID_TAGS: ['style','form']`, plus a `afterSanitizeAttributes` hook restricting `img`/`video` `src` to relative `attachments/…` or `/api/attachments/…` paths — no external URLs, which also keeps rendering offline-clean.

## 6. Cross-platform

**6.1 Windows backslashes leak into stored paths.**
Resolution: all persisted paths (Related Files, Attachments section, JSON, API payloads) are forward-slash relative — Related Files relative to repo root, attachments as `attachments/<ID>/<file>` relative to `.pine/`. `filepath.ToSlash` at every write boundary, `filepath.FromSlash` at every read boundary. One helper package, enforced in review.

**6.2 Atomic write-rename fails on Windows when the target is open.**
`os.Rename` over an open file (editor, Defender, indexer) errors on Windows. Resolution: temp-file-in-same-dir + rename, wrapped in a retry loop (5 attempts, 50ms backoff) on Windows; the watcher's file reads get the same retry for sharing violations.

**6.3 Browser open & port collision underdesigned.**
Resolution: use `github.com/pkg/browser` (never shell-interpolate the URL). Port 3412 busy → probe `GET /api/health` (returns `{app:"pine", repo:<abs path>}`): same repo → print "already running" and just open it; anything else → walk up to 3421 for a free port and print it. `pine open` discovers a running server's port via a runtime file at `os.UserCacheDir()/pine/<sha1(repoPath)>.json` (port+pid, stale-pid checked) — NEVER a runtime file inside `.pine/`.

## 7. Bleve

**7.1 Startup rebuild blocks serve.**
Bleve indexes roughly low-thousands of docs/sec; 10k tickets ≈ several seconds. Resolution: `bleve.NewMemOnly()`, built asynchronously AFTER the listener is up, using `index.NewBatch()` in batches of ~200. Search API returns `{"indexing": true, "results": [...partial]}` until done; UI shows a subtle "indexing…" state. Typical repos (<500 tickets) finish sub-second. Index drift is prevented structurally: the store goroutine is the single writer to cache + index in the same code path (see 2.2); every restart is a free full rebuild.

**7.2 Default analyzer mangles code identifiers.**
The standard analyzer splits `login.tsx` → `login`,`tsx` (fine) but leaves `handleSubmit` as one token and gives no exact-path match. Resolution: field mappings — `title`/`body`: standard analyzer; `related_files`/`labels`: dual-indexed as (a) keyword field for exact match and (b) custom analyzer = tokenize on `/ . _ -` + `blevesearch/bleve/v2/analysis/token/camelcase` + lowercase. Query = DisjunctionQuery of MatchQuery (fuzziness 1) + PrefixQuery on title for typeahead.

## 8. go-git

**8.1 `worktree.Status()` on the request path will hang the UI on real repos.**
go-git status is O(worktree), no untracked cache, no fsmonitor — minutes on monorepos. Resolution: never call it per-request. Git panel data comes from a background refresher with a 5s TTL cache and a 3s context timeout; on timeout, degrade to branch-name-only. Inside the locked `internal/git` interface: use `exec git status --porcelain=v2 -z` when a `git` binary is on PATH (the interface exists precisely for this), go-git Status only as the no-binary fallback. Branch/log/HEAD stay on go-git (those are fast).

**8.2 Detached HEAD and zero-commit repos crash the git panel.**
Resolution: `Head()` error `ErrReferenceNotFound` (fresh repo) → branch label from the HEAD symref + "(no commits)", empty commit list, no error surfaced. Detached HEAD → label `detached @ <short-sha>`. Both are first-class states, unit-tested.

**8.3 `pine init` in a subdirectory creates a stranded `.pine/`.**
Resolution: init and serve both walk up to find `.git` (which may be a **file** in linked worktrees — use `PlainOpenWithOptions{DetectDotGit:true}` semantics) and anchor `.pine/` at repo root, printing where. No `.git` found → proceed in cwd with a loud "not a git repo; git features disabled" warning; `pine doctor` repeats it. Refusing outright would break the "local-first, files-first" promise.

## 9. Scope sentinel — cuts and missing pieces

**9.1 CUT from v1:** (a) Smart File Detection ("login" → auto-link `src/login.tsx`) — replace with explicit file-path autocomplete in the editor fed by `git ls-files`; wrong auto-links poison AI prompts. (b) `pine export` — it appears in the CLI section but NOT in the v0.1/v0.3 roadmap; no defined consumer. (c) Dashboard "Projects" list — single-project is locked. (d) Manual kanban card ordering (see 4.1). (e) Template management UI — `pine init` writes static `templates/bug.md` + `templates/feature.md` consumed by the New Issue modal; nothing more.

**9.2 MISSING — `pine context` output location.** Writing `context.md` to repo root collides with user files and is derived data that shouldn't be committed; writing into `.pine/` violates the "only user-meaningful files" constraint. Resolution: `pine context` prints to **stdout** by default (agents pipe or run it directly), optional `--out <path>` for humans. Same for `pine prompt`; `--save` writes to `.pine/prompts/<ID>.md` (that dir is user-meaningful, per PRD).

**9.3 MISSING — no pure-Go WebP encoder in the stdlib; the optimizer brief will hit a cgo wall.** `golang.org/x/image/webp` is decode-only. Resolution: `github.com/gen2brain/webp` (libwebp compiled to WASM, run via wazero — no cgo, costs a few MB of binary). Any encode failure → keep the original file, log, move on. Never fail an upload because optimization failed.

**9.4 MISSING — EXIF orientation vs EXIF stripping.** Go's `image/jpeg` decoder ignores the orientation tag; strip EXIF without rotating and every phone screenshot renders sideways forever. Resolution: read orientation before decode, apply the rotation/flip to pixels, THEN re-encode (re-encoding to WebP drops EXIF for free — that IS the strip). Also: pasted images arrive nameless — name them `paste-YYYYMMDD-HHMMSS.png`-style pre-optimization, dedupe with `-2` suffixes.

**9.5 MISSING — ticket deletion & attachment lifecycle.** Define it: `DELETE /api/tickets/{id}` removes the ticket file AND `attachments/<ID>/` in one operation; `pine doctor` flags orphaned attachment dirs and dangling `Attachments:` references (that's its "broken attachment" check, made precise).

**9.6 MISSING — schema versioning.** `config.json` and `board.json` both carry `"version": 1` from day one, and config carries the locked optimizer flag: `{"version":1,"name":"my-app","attachments":{"optimize":true,"maxEdgePx":2000,"webpQuality":80}}`. Unknown keys are preserved on rewrite (agents will add keys; don't eat them).