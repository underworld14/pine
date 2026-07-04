# Pine Attachment Pipeline — Design (v0.1)

Scope: ingest, optimization, storage, API, doctor checks, and testing for `.pine/attachments/`. Owns everything from "browser sends bytes" to "committable file on disk + metadata back to UI". Go, no cgo, single binary.

---

## 1. Library decisions

### 1.1 WebP encoding: `github.com/gen2brain/webp`

**Decision: use `gen2brain/webp` for encoding and as secondary decoder.** Verified real and actively maintained (31 tagged releases, ongoing commits). It is libwebp compiled to WASM and **transpiled to pure Go via wasm2go** — no cgo, no runtime WASM interpreter needed in current versions. It opportunistically uses a native `libwebp` shared library via `purego` when present on the host (much faster), falling back to the transpiled-Go path otherwise. We build with the default behavior (native-if-available, pure-Go fallback) — deterministic output is not a requirement, only valid WebP.

Why not the alternatives:

- **`HugoSmits86/nativewebp`** — genuinely pure Go and nice, but **VP8L lossless only**. No lossy encoder. Lossless WebP on JPEG photos and busy screenshots routinely *inflates* size versus source JPEG and loses the entire "2.4MB → 180KB" value proposition. Rejected as primary. Not worth carrying as a second encoder either (YAGNI).
- **PNG best-compression fallback** — kept only as an internal degradation path: if WebP encoding returns an error (should be near-never), we fall back to keep-original, not PNG re-encode. Re-encoding PNG→PNG saves ~10-20% at best; not worth a code path.

Encoder call (exact API):

```go
webp.Encode(w, img, webp.Options{Quality: cfg.Quality, Method: 4}) // lossy, q80 default
```

`Method: 4` is libwebp's default speed/size tradeoff; do not expose it in config.

### 1.2 Decoders

| Format | Package | Notes |
|---|---|---|
| PNG | `image/png` (stdlib) | |
| JPEG | `image/jpeg` (stdlib) | |
| GIF | `image/gif` (stdlib) | `gif.DecodeAll` to count frames; single-frame decodes normally |
| WebP (static) | `golang.org/x/image/webp` | decode-only, lossy+lossless, no animation — sufficient |
| Everything else | — | rejected, see §3 |

### 1.3 EXIF orientation: `github.com/rwcarlsen/goexif/exif`

Used for exactly one thing: reading the `Orientation` tag (1–8) from JPEG inputs. It's feature-frozen but the EXIF/TIFF wire format is too; it's pure Go and battle-tested. We **bake orientation into pixels** (rotate/flip during processing) and **never copy any metadata forward** — EXIF stripping (including GPS) is free by construction, since we re-encode from a decoded `image.Image`. If tag parsing fails, treat as orientation 1 and continue; never fail an upload over EXIF.

The 8 orientation cases are implemented in our own ~60-line transform (rotate90/180/270 + horizontal/vertical flip using `x/image/draw` copies). No third-party auto-orient lib.

### 1.4 Resampling: `golang.org/x/image/draw`, **CatmullRom kernel**

Primary content is screenshots with text. CatmullRom is the sharpest of the three built-in kernels and keeps text legible after downscale; the cost (~2-3x ApproxBiLinear) is irrelevant at our sizes. One kernel, not configurable.

---

## 2. Package layout & pipeline

```
internal/attach/
    attach.go      // types: Kind, Result, Config; entry point Ingest()
    sniff.go       // DetectKind([]byte) — magic-byte content sniffing
    image.go       // decodeImage, applyOrientation, downscale, encodeWebP
    names.go       // Slugify, TargetName (slug + short hash + ext)
    store.go       // Store: Dir(ticketID), WriteAtomic, Exists, List
    doctor.go      // CheckOrphans, CheckBrokenRefs, CheckOversizedVideos
```

### 2.1 Entry point

```go
type Result struct {
    FileName      string `json:"fileName"`      // "login-form-3fa9c2d1.webp"
    Path          string `json:"path"`          // "attachments/BUG-001/login-form-3fa9c2d1.webp" (relative to .pine/)
    Markdown      string `json:"markdown"`      // "![login-form](../attachments/BUG-001/login-form-3fa9c2d1.webp)"
    Mime          string `json:"mime"`
    Kind          string `json:"kind"`          // "image" | "video"
    OriginalBytes int64  `json:"originalBytes"`
    FinalBytes    int64  `json:"finalBytes"`
    Width         int    `json:"width"`         // 0 for video
    Height        int    `json:"height"`
    Optimized     bool   `json:"optimized"`     // false => passthrough/keep-original
    Warning       string `json:"warning,omitempty"` // e.g. video-size warning
    Deduplicated  bool   `json:"deduplicated"`  // true => file already existed, nothing written
}

// Ingest is synchronous and does the whole job for one file.
func Ingest(store *Store, ticketID, clientName string, data []byte, cfg Config) (Result, error)
```

### 2.2 Stages (in order)

1. **Sniff** — `DetectKind(data[:512])`. Magic bytes only, never trust extension or client MIME. `http.DetectContentType` plus a custom `ftyp`-box check at offset 4 (brand `qt ` → `video/quicktime`; `isom/mp42/avc1/...` → `video/mp4`, which stdlib sniffing misses for some MOVs). Allowed kinds: png, jpeg, gif, webp, mp4, mov. Anything else → `ErrUnsupportedType` (HTTP 415). **SVG is rejected** — it's not in the PRD's supported list and it's a stored-XSS vector when served back to the browser.
2. **Route**:
   - video → **passthrough** (§3.6), skip to stage 8.
   - animated GIF (`gif.DecodeAll` frame count > 1) → passthrough as `.gif`, `Optimized:false`. *Explicit v1 decision: we do not re-encode animations.*
   - animated WebP (RIFF `ANIM` chunk present) → passthrough.
   - already-small still image: `len(data) < 32*1024` **and** dimensions ≤ `maxDimension` → passthrough (converting a 6KB icon to WebP is churn, not optimization).
   - `cfg.Optimize == false` → passthrough everything.
3. **Bomb check** — `image.DecodeConfig` (header-only, no pixel allocation) *before* full decode. Reject if `width*height > 50_000_000` (50MP: admits 48MP phone JPEGs; worst-case transient RGBA ≈ 200MB) → HTTP 422 with a clear message. Request body itself is capped earlier by `http.MaxBytesReader` (§5).
4. **Decode** — full decode via `image.Decode` (formats registered per §1.2).
5. **Auto-orient** — JPEG only: read Orientation, apply pixel transform. Result is always orientation-1 pixels; no metadata carried.
6. **Downscale** — if long edge > `cfg.MaxDimension` (default 2000): scale proportionally to long edge = 2000 with `draw.CatmullRom.Scale` into `image.NewRGBA`. **Never upscale.**
7. **Encode + keep-smaller rule** — encode WebP at `cfg.Quality` (default 80) into a buffer.
   - If **no downscale happened** and `len(webpBytes) >= len(data)` → keep original bytes/extension (`Optimized:false`). This protects flat-color PNGs that are already tiny and pre-optimized WebPs.
   - If a downscale happened, the WebP always wins (byte comparison against a larger-dimension original is meaningless) — write the WebP.
   - If the encoder errors: log, keep original, `Optimized:false`. An upload must never fail because optimization failed.
8. **Name** (§2.3) → **dedup check** → **atomic write** (§2.4) → return `Result`.

Alpha note (verified behavior): libwebp lossy encodes the alpha plane separately and near-losslessly at default settings; transparent PNG screenshots survive q80 with intact edges. We leave `Exact:false` (RGB under fully-transparent pixels may be discarded — invisible, saves bytes).

### 2.3 Filename strategy

```
<slug>-<hash8>.<ext>
login-form-3fa9c2d1.webp
```

- `slug` = original basename minus extension, lowercased, non-`[a-z0-9]` runs → `-`, trimmed, max 48 chars. Empty/pasted blobs (clipboard paste has no name) → `paste-20260704-153012`.
- `hash8` = first 8 hex chars of SHA-256 of the **original uploaded bytes** (not the optimized output — so the name is stable regardless of optimizer settings).
- `ext` = `webp` if we wrote optimized output, else the sniffed original extension (never the client's claimed extension).

**Dedup (decided: yes, and it's free):** naming is deterministic on content, so re-uploading the same screenshot to the same ticket produces the same target path. If the target file already exists → write nothing, return the existing file with `Deduplicated:true`. That's the whole dedup feature — no index, no cross-ticket dedup (YAGNI: cross-ticket sharing breaks the "delete ticket dir = delete its attachments" mental model). Hash collision within a ticket at 32 bits ≈ never at <100 attachments; the slug differing makes it rarer still.

### 2.4 Atomic write

`Store.WriteAtomic(ticketID, name, data)`:
1. `os.MkdirAll(.pine/attachments/<TICKET-ID>, 0o755)`
2. Write to `.pine/attachments/<TICKET-ID>/.tmp-<rand>` , `f.Sync()`, close.
3. `os.Rename` to final name (same directory ⇒ atomic on all three OSes for our purposes).

The fsnotify watcher must **ignore paths whose basename starts with `.tmp-`** (contract with the sync layer). The rename produces one clean CREATE event; the UI's attachment list refresh keys off that, so files dropped in by AI agents or `git pull` surface identically to uploads.

---

## 3. Edge cases (decisions, not options)

| Case | Decision |
|---|---|
| Animated GIF | Passthrough untouched. Stated v1 limitation; re-encoding animation is a rabbit hole (frame timing, disposal modes) for a rare input. |
| Animated WebP | Passthrough (x/image/webp can't decode it anyway; detect via `ANIM` chunk). |
| Transparent PNG | Converted; lossy WebP preserves alpha well (§2.2 note). Golden test asserts alpha survives. |
| Oversized image | `DecodeConfig` pre-check, 50MP cap → 422 `{"error":"image too large (max 50 megapixels)"}`. |
| Oversized request | `http.MaxBytesReader`: 64MB for image uploads is plenty; one global 500MB hard cap covers video. 413 on breach. |
| SVG | Rejected, 415. Security + not in PRD's format list. |
| Corrupted file | Sniff passes but decode fails → 422 `{"error":"could not decode image: <detail>"}`. Nothing written. |
| Static WebP input larger than maxDimension | Decoded, downscaled, re-encoded like any image. WebP within dimensions → keep-smaller rule usually passes it through. |
| Duplicate upload | Content-hash-deterministic name ⇒ idempotent, `Deduplicated:true` (§2.3). |
| MP4/MOV | Passthrough byte-for-byte, no probing/transcoding. If size > `cfg.MaxVideoMB`: still accepted, `Warning:"video is 84MB (recommended max 50MB); large files bloat the git repo"` in the response (UI shows amber badge) and flagged by `pine doctor`. |
| Path traversal | Ticket ID validated against `^[A-Z]+-\d+$` before touching the filesystem; filename is fully synthesized by us. |

---

## 4. Config (`.pine/config.json`)

```json
{
  "attachments": {
    "optimize": true,
    "maxDimension": 2000,
    "quality": 80,
    "maxVideoMB": 50
  }
}
```

All keys optional; these are the defaults, applied via a `Config.withDefaults()` in `internal/attach`. `optimize:false` turns the pipeline into pure sniff→validate→store. No other knobs (no kernel choice, no per-format settings, no alpha quality — YAGNI).

---

## 5. API contract

```
POST   /api/tickets/{id}/attachments        multipart/form-data, field "files" (1..n)
DELETE /api/tickets/{id}/attachments/{name}
GET    /attachments/{ticketID}/{name}       static serve, sniffed Content-Type,
                                            Cache-Control: no-cache, ETag = file mtime+size,
                                            no directory listing
```

- Paste and drag are **the same endpoint**: the browser wraps the clipboard/drop blob in `FormData`. No base64 JSON variant.
- Response `201`:

```json
{
  "attachments": [
    {
      "fileName": "login-form-3fa9c2d1.webp",
      "path": "attachments/BUG-001/login-form-3fa9c2d1.webp",
      "markdown": "![login-form](../attachments/BUG-001/login-form-3fa9c2d1.webp)",
      "mime": "image/webp",
      "kind": "image",
      "originalBytes": 2516582,
      "finalBytes": 184320,
      "width": 2000,
      "height": 1250,
      "optimized": true,
      "deduplicated": false
    }
  ]
}
```

`originalBytes`/`finalBytes` directly drive the UI badge "optimized 2.4 MB → 180 KB". `markdown` is relative to the *ticket file* (`.pine/tickets/X.md` → `../attachments/...`) so embeds render both in Pine and on GitHub. Multi-file requests process sequentially; per-file failures return per-file error entries rather than failing the batch (mixed results ⇒ 207-style body with `"error"` on failed entries, still HTTP 201 if ≥1 succeeded, 4xx if all failed).

- **Sync, not async.** Latency budget: p95 ≤ 1.5s for a 5K retina screenshot (decode ~150ms + CatmullRom scale ~100ms + WebP q80 encode 200–600ms transpiled-Go path, ~3x faster when native libwebp is found via purego). Well under the paste-and-keep-typing threshold; a queue/job system would be pure overhead at these numbers.
- Upload does **not** edit the ticket file. The frontend inserts the returned `markdown` into the editor / Attachments section itself. New Issue modal flow: pasted images are held as browser blobs; on Save, the ticket is created first, then blobs upload against the real ID. Keeps the backend stateless and files-first.

---

## 6. `pine doctor` + `pine optimize`

Doctor checks (in `internal/attach/doctor.go`, invoked by the doctor command):

1. **Orphaned dirs** — `attachments/<X>/` where `tickets/<X>.md` doesn't exist. Warn + suggest `git rm -r` (never auto-delete).
2. **Broken refs** — any `attachments/...` path referenced in a ticket body that doesn't exist on disk. Error-level.
3. **Unreferenced files** — file exists, no ticket mentions it. Info-level only (agents legitimately drop files before writing refs).
4. **Oversized videos** — any video > `maxVideoMB`. Warn with per-file sizes and repo-bloat note.
5. **Stray temp files** — leftover `.tmp-*` from a crashed write. Warn + safe to delete.

**`pine optimize` — decided: yes, ship it in v1, it's ~150 lines of reuse.** Rationale: AI agents and humans drop raw PNGs into `attachments/` directly (that's the whole point of file-first), so the upload-time optimizer alone leaves unoptimized bytes accumulating. Behavior: walk all attachment dirs, run each still image through the exact same `Ingest` stages 2–8; on conversion, write new file, delete old, and **rewrite references** in the owning ticket's markdown (exact string replace of the old relative path). `--dry-run` prints the would-be savings table. Videos and animations untouched. Skips anything the pipeline would pass through, so it's idempotent.

---

## 7. Testing

`internal/attach/testdata/` fixtures (small, committed):

| Fixture | Asserts |
|---|---|
| `screenshot.png` (1x, ~3000px wide, text-heavy) | downscaled to 2000 long edge, output `.webp`, `finalBytes < originalBytes`, `Optimized:true` |
| `photo-rot6.jpg` (EXIF Orientation=6) | output width/height swapped vs raw pixels; decode-back and sample corner pixels to prove rotation baked in; no EXIF in output |
| `transparent.png` | output `.webp`; decode-back: corner pixel alpha == 0, opaque region alpha == 255 |
| `animated.gif` (3 frames) | passthrough byte-identical, `.gif`, `Optimized:false` |
| `tiny-icon.png` (200×200, 4KB) | passthrough byte-identical, `Optimized:false` (no upscale, no convert) |
| `already-small.webp` | keep-smaller rule returns original |
| `clip.mp4` (2s) + size-warning case (config `maxVideoMB:0` to force warning) | passthrough + `Warning` populated |
| `bomb-header.png` (crafted 60MP header) | 422 before allocation (assert via `DecodeConfig` path, no OOM) |
| `evil.svg`, `truncated.jpg` | 415 / 422 respectively, nothing written |
| duplicate upload (same bytes twice) | second call `Deduplicated:true`, dir contains one file |

**Do not byte-golden the WebP output** — encoder bytes shift across gen2brain/webp versions and native-vs-transpiled paths. Golden the *properties*: format, dimensions, size ceiling (`finalBytes < 0.5 * originalBytes` for the screenshot fixture), and decode-back pixel checks. Store dedup/name tests golden the *filenames* (deterministic by design). One integration test drives the multipart endpoint end-to-end through chi, including the `.tmp-*`-ignore contract with a real fsnotify watcher.

---

## 8. Dependency summary (new modules this pipeline adds)

```
github.com/gen2brain/webp          // WebP encode (+decode backup); pure Go via wasm2go, purego fast path
golang.org/x/image                 // draw (CatmullRom), webp decode
github.com/rwcarlsen/goexif        // EXIF Orientation read only
// stdlib: image/{png,jpeg,gif}, net/http (sniff, MaxBytesReader), crypto/sha256
```

No cgo anywhere; `nodynamic` build tag on gen2brain/webp is available if the purego dynamic-loading path ever misbehaves on a platform, without touching our code.

Sources: [gen2brain/webp](https://github.com/gen2brain/webp), [HugoSmits86/nativewebp](https://github.com/HugoSmits86/nativewebp), [nativewebp on pkg.go.dev](https://pkg.go.dev/github.com/HugoSmits86/nativewebp), [x/image webp encoding proposal (golang/go#45121)](https://github.com/golang/go/issues/45121)