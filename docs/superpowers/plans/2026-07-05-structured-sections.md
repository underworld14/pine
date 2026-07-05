# Structured Sections & Acceptance-Criteria Checklists — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give Pine tickets a standard section template and an acceptance-criteria checkbox progress signal, with click-to-toggle checkboxes in the web UI via a dedicated server endpoint.

**Architecture:** Pure checklist logic in `internal/ticket` (parse progress, set an item); the DTO carries `acceptance {done,total}`; a `PATCH /api/tickets/{id}/checklist` endpoint applies a toggle atomically through the existing `store.UpdateIfMatch`; templates gain the standard sections; the SvelteKit app renders task-list items as real checkboxes and calls the endpoint.

**Tech Stack:** Go 1.24+ (backend), SvelteKit 2 / Svelte 5 / TypeScript / Vitest (frontend), `markdown-it` (already present) + `markdown-it-task-lists`.

## Global Constraints

- Progress counts checkboxes **only under the "Acceptance Criteria" section** (case-insensitive). Toggling addresses **any** checkbox in the body by document-order index.
- Domain logic is pure (no I/O) and lives in `internal/ticket`; every surface (endpoint, CLI, future MCP) calls it.
- Off-branch (read-only) tickets reject checklist writes with HTTP 409 (`off_branch`), reusing `srv.offBranchRef`.
- Checkbox regex: `^(\s*[-*]\s+)\[([ xX])\]\s+\S`. Uppercase `[X]` counts as checked; `SetChecklistItem` normalizes to `x`.
- The endpoint uses `If-Match` optimistic concurrency via `store.UpdateIfMatch(id, expectedHash, mut)`, which returns `store.ErrConflict` on mismatch.
- Run all Go tests from the repo root; run web tests from `web/`.

---

### Task 1: Domain — `AcceptanceProgress`

**Files:**
- Create: `internal/ticket/checklist.go`
- Test: `internal/ticket/checklist_test.go`

**Interfaces:**
- Consumes: `ticket.SectionContent(body, heading string) (string, bool)` (exists in `sections.go`).
- Produces: `func AcceptanceProgress(body string) (done, total int)`

- [ ] **Step 1: Write the failing test**

```go
// internal/ticket/checklist_test.go
package ticket

import "testing"

func TestAcceptanceProgress(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		done, tot  int
	}{
		{"no section", "# Description\nhi\n", 0, 0},
		{"empty section", "# Acceptance Criteria\n\n", 0, 0},
		{"mixed", "# Acceptance Criteria\n- [x] a\n- [ ] b\n- [X] c\n", 2, 3},
		{"only AC counts", "# Acceptance Criteria\n- [x] a\n# Implementation Plan\n- [ ] step\n", 1, 1},
		{"case-insensitive heading", "# acceptance criteria\n- [ ] a\n", 0, 1},
		{"non-checkbox bullets ignored", "# Acceptance Criteria\n- plain\n- [ ] real\n", 0, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d, tot := AcceptanceProgress(c.body)
			if d != c.done || tot != c.tot {
				t.Errorf("got %d/%d, want %d/%d", d, tot, c.done, c.tot)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ticket/ -run TestAcceptanceProgress -v`
Expected: FAIL — `undefined: AcceptanceProgress`.

- [ ] **Step 3: Write minimal implementation**

```go
// internal/ticket/checklist.go
package ticket

import (
	"regexp"
	"strings"
)

// checkboxRe matches a markdown task-list item: an optional indent, a bullet,
// then a [ ]/[x]/[X] box followed by non-empty text.
var checkboxRe = regexp.MustCompile(`^(\s*[-*]\s+)\[([ xX])\]\s+\S`)

// AcceptanceProgress counts checked and total checkbox items under the
// "Acceptance Criteria" section. Returns 0,0 when the section is absent or empty.
func AcceptanceProgress(body string) (done, total int) {
	content, ok := SectionContent(body, "Acceptance Criteria")
	if !ok {
		return 0, 0
	}
	for _, line := range strings.Split(content, "\n") {
		m := checkboxRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		total++
		if m[2] == "x" || m[2] == "X" {
			done++
		}
	}
	return done, total
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ticket/ -run TestAcceptanceProgress -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ticket/checklist.go internal/ticket/checklist_test.go
git commit -m "feat(ticket): AcceptanceProgress counts AC-section checkboxes"
```

---

### Task 2: Domain — `SetChecklistItem`

**Files:**
- Modify: `internal/ticket/checklist.go`
- Test: `internal/ticket/checklist_test.go`

**Interfaces:**
- Produces: `func SetChecklistItem(body string, index int, checked bool) (newBody string, ok bool)` — flips the `index`-th checkbox line in the whole body (document order, 0-based). Idempotent. `ok=false` when out of range.

- [ ] **Step 1: Write the failing test**

```go
// append to internal/ticket/checklist_test.go
func TestSetChecklistItem(t *testing.T) {
	body := "# Acceptance Criteria\n- [ ] a\n- [x] b\n# Impl\n  - [ ] c\n"

	// check the first item
	nb, ok := SetChecklistItem(body, 0, true)
	if !ok || nb != "# Acceptance Criteria\n- [x] a\n- [x] b\n# Impl\n  - [ ] c\n" {
		t.Fatalf("index 0 ->true: ok=%v\n%q", ok, nb)
	}
	// idempotent: setting an already-checked item true is a no-op success
	nb2, ok := SetChecklistItem(nb, 1, true)
	if !ok || nb2 != nb {
		t.Errorf("idempotent set failed: ok=%v changed=%v", ok, nb2 != nb)
	}
	// check the third item (preserves indentation and '-' marker)
	nb3, ok := SetChecklistItem(body, 2, true)
	if !ok || nb3 != "# Acceptance Criteria\n- [ ] a\n- [x] b\n# Impl\n  - [x] c\n" {
		t.Errorf("index 2 ->true:\n%q", nb3)
	}
	// out of range
	if _, ok := SetChecklistItem(body, 9, true); ok {
		t.Errorf("out-of-range index should return ok=false")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ticket/ -run TestSetChecklistItem -v`
Expected: FAIL — `undefined: SetChecklistItem`.

- [ ] **Step 3: Write minimal implementation**

```go
// append to internal/ticket/checklist.go
// SetChecklistItem sets the index-th checkbox in body (document order, 0-based)
// to checked. It preserves the line's indentation, bullet, and text. It is
// idempotent. ok is false when index is out of range.
func SetChecklistItem(body string, index int, checked bool) (string, bool) {
	lines := strings.Split(body, "\n")
	n := -1
	for i, line := range lines {
		m := checkboxRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		n++
		if n != index {
			continue
		}
		mark := " "
		if checked {
			mark = "x"
		}
		// m[1] is the "indent+bullet" prefix; replace only the "[.]" box (3 chars).
		rest := line[len(m[1])+3:]
		lines[i] = m[1] + "[" + mark + "]" + rest
		return strings.Join(lines, "\n"), true
	}
	return body, false
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/ticket/ -run 'TestSetChecklistItem|TestAcceptanceProgress' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ticket/checklist.go internal/ticket/checklist_test.go
git commit -m "feat(ticket): SetChecklistItem flips a checkbox by index"
```

---

### Task 3: DTO — expose `acceptance` on the ticket view

**Files:**
- Modify: `internal/view/view.go` (struct + `Build` + `BuildOffBranch`)
- Test: `internal/view/view_test.go` (create)

**Interfaces:**
- Consumes: `ticket.AcceptanceProgress`, existing `view.Progress{Done, Total int}` (view.go:57).
- Produces: `view.Ticket.Acceptance *Progress` (JSON `acceptance`, omitempty).

- [ ] **Step 1: Write the failing test**

```go
// internal/view/view_test.go
package view

import (
	"testing"
	"time"

	"github.com/underworld14/pine/internal/ticket"
)

func TestBuildOffBranchAcceptance(t *testing.T) {
	tk := &ticket.Ticket{
		ID: "BUG-7f3k2a", Title: "x", Status: "todo",
		Created: time.Now(), Updated: time.Now(),
		Body: "# Acceptance Criteria\n- [x] a\n- [ ] b\n",
	}
	v := BuildOffBranch(tk, "feature", false)
	if v.Acceptance == nil || v.Acceptance.Done != 1 || v.Acceptance.Total != 2 {
		t.Fatalf("acceptance = %+v, want 1/2", v.Acceptance)
	}

	tk.Body = "# Description\nno criteria\n"
	if v := BuildOffBranch(tk, "feature", false); v.Acceptance != nil {
		t.Errorf("no AC section should omit acceptance, got %+v", v.Acceptance)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/view/ -run TestBuildOffBranchAcceptance -v`
Expected: FAIL — `v.Acceptance undefined`.

- [ ] **Step 3: Write minimal implementation**

In `internal/view/view.go`, add the field to the `Ticket` struct (next to `EpicProgress`):
```go
	Acceptance   *Progress  `json:"acceptance,omitempty"`
```

Add a helper near the bottom of the file:
```go
// acceptanceProgress returns the AC checkbox progress, or nil when there are none.
func acceptanceProgress(body string) *Progress {
	done, total := ticket.AcceptanceProgress(body)
	if total == 0 {
		return nil
	}
	return &Progress{Done: done, Total: total}
}
```

In `Build`, before `return v`, add:
```go
	v.Acceptance = acceptanceProgress(t.Body)
```

In `BuildOffBranch`, before `return v`, add the same line:
```go
	v.Acceptance = acceptanceProgress(t.Body)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/view/ -run TestBuildOffBranchAcceptance -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/view/view.go internal/view/view_test.go
git commit -m "feat(view): expose acceptance-criteria progress on the ticket DTO"
```

---

### Task 4: Server — `PATCH /api/tickets/{id}/checklist`

**Files:**
- Modify: `internal/server/tickets.go` (handler), `internal/server/server.go:64` (route)
- Test: `internal/server/checklist_test.go` (create)

**Interfaces:**
- Consumes: `ticket.SetChecklistItem`, `store.UpdateIfMatch(id, expectedHash, mut)` (returns `store.ErrConflict`), `srv.offBranchRef`, `readOnlyBranch`, `apiOrigin`, `srv.emit`, `srv.setETag`, `srv.reindex`, `decodeJSON`, `badRequest`, `writeErr`, `writeJSON`, `view.Build`.
- Produces: `func (srv *Server) handleSetChecklist(w http.ResponseWriter, r *http.Request)`.

- [ ] **Step 1: Write the failing test** (reuses the cross-branch test harness's `do`, `writeTicketFile`, `newCrossBranchServer` from `internal/server/crossbranch_test.go`)

```go
// internal/server/checklist_test.go
package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/underworld14/pine/internal/config"
	"github.com/underworld14/pine/internal/store"
)

func acBody(id string) string {
	return "---\nid: " + id + "\ntitle: " + id + "\nstatus: todo\nupdated: 2026-07-01T10:00:00Z\n---\n\n# Acceptance Criteria\n- [ ] one\n- [ ] two\n"
}

// newLocalServer builds a server over a temp .pine with one ticket already on disk.
func newLocalServer(t *testing.T, id, body string) *httptest.Server {
	t.Helper()
	repo := t.TempDir()
	pine := filepath.Join(repo, ".pine")
	if err := os.MkdirAll(filepath.Join(pine, "tickets"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default("test")
	b, _ := cfg.Bytes()
	os.WriteFile(filepath.Join(pine, "config.json"), b, 0o644)
	bb, _ := config.DefaultBoard().Bytes()
	os.WriteFile(filepath.Join(pine, "board.json"), bb, 0o644)
	os.WriteFile(filepath.Join(pine, "tickets", id+".md"), []byte(body), 0o644)
	st, err := store.Open(pine)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(New(st, "test").Handler())
	t.Cleanup(ts.Close)
	return ts
}

func hashOf(t *testing.T, jsonBody string) string {
	t.Helper()
	var m map[string]any
	json.Unmarshal([]byte(jsonBody), &m)
	h, _ := m["hash"].(string)
	return h
}

func TestChecklistToggle(t *testing.T) {
	ts := newLocalServer(t, "BUG-0a1b2c", acBody("BUG-0a1b2c"))

	_, gb := do(t, "GET", ts.URL+"/api/tickets/BUG-0a1b2c", "", nil)
	hash := hashOf(t, gb)

	resp, body := do(t, "PATCH", ts.URL+"/api/tickets/BUG-0a1b2c/checklist",
		`{"index":0,"checked":true}`,
		map[string]string{"Content-Type": "application/json", "If-Match": `"` + hash + `"`})
	if resp.StatusCode != 200 {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, `"acceptance":{"done":1,"total":2}`) {
		t.Errorf("expected 1/2 progress: %s", body)
	}
	if !strings.Contains(body, "- [x] one") {
		t.Errorf("body should show first box checked: %s", body)
	}

	if r, _ := do(t, "PATCH", ts.URL+"/api/tickets/BUG-0a1b2c/checklist",
		`{"index":9,"checked":true}`, map[string]string{"Content-Type": "application/json"}); r.StatusCode != 400 {
		t.Errorf("bad index want 400, got %d", r.StatusCode)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -run TestChecklistToggle -v`
Expected: FAIL — route not registered (404).

- [ ] **Step 3: Write minimal implementation**

Add the route in `internal/server/server.go` after line 64 (`r.Patch("/{id}", …)`):
```go
			r.Patch("/{id}/checklist", srv.handleSetChecklist)
```

Add the handler to `internal/server/tickets.go`:
```go
// setChecklistBody is the PATCH /checklist request.
type setChecklistBody struct {
	Index   int    `json:"index"`
	Checked bool   `json:"checked"`
	OpID    string `json:"opId"`
}

func (srv *Server) handleSetChecklist(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if branch, ok := srv.offBranchRef(id); ok {
		writeErr(w, readOnlyBranch(id, branch))
		return
	}
	var b setChecklistBody
	if err := decodeJSON(r, &b); err != nil {
		writeErr(w, badRequest(err.Error()))
		return
	}
	ifm := strings.Trim(r.Header.Get("If-Match"), `"`)
	updated, err := srv.store.UpdateIfMatch(id, ifm, func(t *ticket.Ticket) error {
		nb, ok := ticket.SetChecklistItem(t.Body, b.Index, b.Checked)
		if !ok {
			return badRequest("checklist index out of range")
		}
		t.Body = nb
		return nil
	})
	if errors.Is(err, store.ErrConflict) {
		cur, _ := srv.store.Get(id)
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":   map[string]any{"code": "conflict", "message": "ticket changed on disk"},
			"current": view.Build(srv.store, srv.store.Graph(), cur, true),
		})
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	srv.setETag(w, id)
	srv.reindex(id)
	v := view.Build(srv.store, srv.store.Graph(), updated, true)
	srv.emit("ticket.updated", apiOrigin(b.OpID), map[string]any{"ticket": v})
	writeJSON(w, http.StatusOK, v)
}
```
(`errors`, `store`, `strings`, `ticket`, `view`, `chi` are already imported in tickets.go.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/server/ -run TestChecklistToggle -v`
Expected: PASS.

- [ ] **Step 5: Add and verify the off-branch 409 case**

```go
// append to internal/server/checklist_test.go
func TestChecklistOffBranchRejected(t *testing.T) {
	ts := newCrossBranchServer(t, "hash") // FEAT-3d4e5f is off-branch (from crossbranch_test.go)
	r, body := do(t, "PATCH", ts.URL+"/api/tickets/FEAT-3d4e5f/checklist",
		`{"index":0,"checked":true}`, map[string]string{"Content-Type": "application/json"})
	if r.StatusCode != http.StatusConflict || !strings.Contains(body, "off_branch") {
		t.Errorf("want 409 off_branch, got %d: %s", r.StatusCode, body)
	}
}
```
Run: `go test ./internal/server/ -run TestChecklist -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/server/server.go internal/server/tickets.go internal/server/checklist_test.go
git commit -m "feat(server): PATCH /tickets/{id}/checklist toggles a box atomically"
```

---

### Task 5: Templates — standard sections in new tickets

**Files:**
- Modify: `internal/store/create.go:14-18` (built-in templates)
- Modify: `internal/cli/init.go` (if it writes template files — verify)
- Test: `internal/store/create_test.go` (add a case)

**Interfaces:** none new — verifies `Create` seeds bodies from `template(prefix)`.

- [ ] **Step 1: Write the failing test**

```go
// add to internal/store/create_test.go — use the same temp-store helper the neighboring
// tests use (open a temp .pine store); if none exists, inline store.Open on t.TempDir().
func TestCreateSeedsAcceptanceCriteria(t *testing.T) {
	s := openTestStore(t) // match the helper name used by other tests in this file
	for _, typ := range []string{"BUG", "FEAT"} {
		tk, err := s.Create(CreateReq{Type: typ, Title: "x"})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(tk.Body, "# Acceptance Criteria") {
			t.Errorf("%s body missing Acceptance Criteria:\n%s", typ, tk.Body)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestCreateSeedsAcceptanceCriteria -v`
Expected: FAIL — `bugTemplate` has no Acceptance Criteria.

- [ ] **Step 3: Update the built-in templates**

In `internal/store/create.go`:
```go
const (
	bugTemplate     = "\n# Description\n\n# Steps\n\n# Expected\n\n# Actual\n\n# Acceptance Criteria\n- [ ] \n\n# Related Files\n\n# Attachments\n"
	featureTemplate = "\n# Description\n\n# Acceptance Criteria\n- [ ] \n\n# Implementation Plan\n\n# Notes\n\n# Related Files\n\n# Attachments\n"
	epicTemplate    = "\n# Description\n\n# Goals\n\n# Child Tickets\n"
)
```

Then, in `internal/cli/init.go`, search for `templates` / `bug.md` / `feature.md`. If `init` writes template files, update those literals to match the constants above. If it does NOT (relies on built-in defaults from `store.template`), leave a one-line note in the commit and skip.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run TestCreateSeedsAcceptanceCriteria -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/create.go internal/store/create_test.go internal/cli/init.go
git commit -m "feat(templates): seed new tickets with Acceptance Criteria + standard sections"
```

---

### Task 6: AI context — include Acceptance Criteria + progress

**Files:**
- Modify: `internal/contextgen/context.go` (near line 178, where `SectionContent(body, "Description")` is rendered)
- Test: `internal/contextgen/contextgen_test.go` (add a case)

**Interfaces:** Consumes `ticket.AcceptanceProgress`, `ticket.SectionContent`.

- [ ] **Step 1: Read the surrounding code**

Open `internal/contextgen/context.go` around line 178 to see the writer variable (a `*strings.Builder`) and the exported render function the existing tests call. Match those names in the steps below.

- [ ] **Step 2: Write the failing test**

```go
// add to internal/contextgen/contextgen_test.go — mirror the call shape other tests use.
func TestContextIncludesAcceptance(t *testing.T) {
	tk := &ticket.Ticket{ID: "BUG-0a1b2c", Title: "x", Status: "todo",
		Body: "# Acceptance Criteria\n- [x] a\n- [ ] b\n"}
	out := renderDetail(tk) // replace with the real per-ticket render helper used nearby
	if !strings.Contains(out, "Acceptance Criteria") || !strings.Contains(out, "1/2") {
		t.Errorf("context missing acceptance progress:\n%s", out)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/contextgen/ -run TestContextIncludesAcceptance -v`
Expected: FAIL — acceptance text absent (or fix the helper name until it compiles, then FAIL on the assertion).

- [ ] **Step 4: Write minimal implementation**

In `internal/contextgen/context.go`, after the Description block (~line 178), add (using the surrounding `*strings.Builder` variable, shown here as `b`):
```go
	if ac, ok := ticket.SectionContent(t.Body, "Acceptance Criteria"); ok {
		done, total := ticket.AcceptanceProgress(t.Body)
		fmt.Fprintf(b, "\n## Acceptance Criteria (%d/%d)\n%s\n", done, total, ac)
	}
```
Ensure `fmt` is imported.

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/contextgen/ -run TestContextIncludesAcceptance -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/contextgen/context.go internal/contextgen/contextgen_test.go
git commit -m "feat(contextgen): include acceptance criteria + progress in AI context"
```

---

### Task 7: Web — render task-list items as real checkboxes

**Files:**
- Modify: `web/package.json` (add dep), `web/src/lib/markdown.ts`
- Test: `web/src/lib/markdown.test.ts` (create)

**Interfaces:** Produces checkbox `<input>` elements in `renderMarkdown` output.

- [ ] **Step 1: Install the plugin**

Run (from `web/`): `npm install markdown-it-task-lists@2`
Expected: adds `markdown-it-task-lists` to `dependencies`.

- [ ] **Step 2: Write the failing test**

```ts
// web/src/lib/markdown.test.ts
import { describe, it, expect } from 'vitest';
import { renderMarkdown } from './markdown';

describe('renderMarkdown task lists', () => {
  it('renders "- [ ]" as an enabled checkbox', () => {
    const html = renderMarkdown('# Acceptance Criteria\n- [ ] a\n- [x] b\n');
    expect(html).toContain('type="checkbox"');
    expect(html).not.toContain('disabled');
    expect((html.match(/type="checkbox"/g) ?? []).length).toBe(2);
  });
});
```

- [ ] **Step 3: Run test to verify it fails**

Run (from `web/`): `npx vitest run src/lib/markdown.test.ts`
Expected: FAIL — no checkbox inputs.

- [ ] **Step 4: Wire the plugin + keep `checked` through sanitize**

In `web/src/lib/markdown.ts`:
```ts
import MarkdownIt from 'markdown-it';
import taskLists from 'markdown-it-task-lists';
import DOMPurify from 'dompurify';

const md = new MarkdownIt({ html: true, linkify: true, breaks: false })
  .use(taskLists, { enabled: true, label: false });
```
And in `renderMarkdown`, add `checked` to `ADD_ATTR`:
```ts
  return DOMPurify.sanitize(raw, {
    USE_PROFILES: { html: true },
    FORBID_TAGS: ['style', 'form', 'script', 'iframe'],
    ADD_ATTR: ['target', 'checked'],
    ALLOW_DATA_ATTR: false
  });
```

- [ ] **Step 5: Run test to verify it passes**

Run (from `web/`): `npx vitest run src/lib/markdown.test.ts`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add web/package.json web/package-lock.json web/src/lib/markdown.ts web/src/lib/markdown.test.ts
git commit -m "feat(web): render markdown task lists as interactive checkboxes"
```

---

### Task 8: Web — API client method + `acceptance` type

**Files:**
- Modify: `web/src/lib/api.ts` (`interface Ticket` + `api` object)

**Interfaces:** Produces `api.setChecklistItem(id, index, checked, ifMatch?, opId?)`.

- [ ] **Step 1: Add the `acceptance` field to `interface Ticket`** (after `epicProgress`)

```ts
  acceptance?: { done: number; total: number };
```

- [ ] **Step 2: Add the client method** (in the `api` object, after `patchTicket`)

```ts
  setChecklistItem: (id: string, index: number, checked: boolean, ifMatch?: string, opId?: string) =>
    req<Ticket>('PATCH', `/api/tickets/${id}/checklist`, { index, checked, opId }, ifMatch ? { 'If-Match': ifMatch } : {}),
```

- [ ] **Step 3: Type-check**

Run (from `web/`): `npm run check`
Expected: no *new* errors beyond the pre-existing baseline (`$page.params.id` and `vite.config.ts` errors that already exist on `main`).

- [ ] **Step 4: Commit**

```bash
git add web/src/lib/api.ts
git commit -m "feat(web): api.setChecklistItem + acceptance field"
```

---

### Task 9: Web — progress pill on cards + interactive checkboxes on detail

**Files:**
- Modify: `web/src/lib/components/TicketCard.svelte`, `web/src/routes/tickets/[id]/+page.svelte`

**Interfaces:** Consumes `ticket.acceptance`, `api.setChecklistItem`, `workspace`, `ApiError`, `toasts`.

- [ ] **Step 1: Add the progress pill to `TicketCard.svelte`**

In the `.foot` row, before the `.spacer`, add:
```svelte
{#if ticket.acceptance?.total}
  <span class="ac" title="Acceptance criteria">
    {#each Array(ticket.acceptance.total) as _, i}<span class="tick" class:on={i < ticket.acceptance.done}></span>{/each}
    {ticket.acceptance.done}/{ticket.acceptance.total}
  </span>
{/if}
```
Add to the `<style>`:
```css
  .ac { display: inline-flex; align-items: center; gap: 3px; font-size: 10px; color: var(--color-dim); }
  .ac .tick { width: 5px; height: 5px; border-radius: 1px; background: var(--color-border); }
  .ac .tick.on { background: var(--color-accent); }
```

- [ ] **Step 2: Add an `AC N/M` chip to the detail meta row**

In `web/src/routes/tickets/[id]/+page.svelte`, in the `.meta` block:
```svelte
{#if ticket.acceptance?.total}<span class="updated">AC {ticket.acceptance.done}/{ticket.acceptance.total}</span>{/if}
```

- [ ] **Step 3: Wire checkbox toggling on the detail page**

Add the handler near the other functions:
```ts
  async function onChecklistChange(e: Event) {
    if (!ticket || ticket.readOnly) return;
    const target = e.target as HTMLElement;
    if (!(target instanceof HTMLInputElement) || target.type !== 'checkbox') return;
    const boxes = Array.from((e.currentTarget as HTMLElement).querySelectorAll('input[type=checkbox]'));
    const index = boxes.indexOf(target);
    if (index < 0) return;
    try {
      const updated = await api.setChecklistItem(id, index, target.checked, ticket.hash, workspace.beginOp());
      workspace.tickets = { ...workspace.tickets, [updated.id]: updated };
      baseHash = updated.hash;
      baseBody = updated.body ?? '';
      text = updated.body ?? '';
    } catch (err) {
      target.checked = !target.checked; // revert optimistic DOM flip
      if (err instanceof ApiError && err.status === 409 && err.current) conflict = err.current;
      else toasts.push(err instanceof Error ? err.message : 'Update failed', 'error');
    }
  }
```
On the `<div class="preview">` element, add `onchange={onChecklistChange}`. For read-only tickets, disable the boxes by post-processing the html:
```svelte
{@html ticket.readOnly ? preview.replaceAll('<input ', '<input disabled ') : preview}
```

- [ ] **Step 4: Build to verify it compiles**

Run (from `web/`): `npm run build`
Expected: build succeeds.

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/components/TicketCard.svelte web/src/routes/tickets/[id]/+page.svelte
git commit -m "feat(web): checklist progress pill + click-to-toggle checkboxes"
```

---

### Task 10: End-to-end verification

- [ ] **Step 1: Full backend suite**

Run: `go test ./...` then `go vet ./...`
Expected: all pass, vet clean.

- [ ] **Step 2: Web suite + build**

Run (from `web/`): `npx vitest run` then `npm run build`
Expected: tests pass, build succeeds.

- [ ] **Step 3: Manual e2e**

```bash
make build
cd /tmp && rm -rf ac-demo && mkdir ac-demo && cd ac-demo && git init -q && git checkout -b main
../path/to/pine init && ../path/to/pine serve --port 3599 &
```
Open `http://localhost:3599`, create a Feature (confirm the body has `# Acceptance Criteria`), open it, tick a box in the preview. Expected: the card shows `1/1`, the detail meta shows `AC 1/1`, and `.pine/tickets/FEAT-*.md` on disk shows `- [x]`. Stop the server (`kill %1`).

- [ ] **Step 4: Final commit (only if fixes were needed)**

```bash
git commit -am "test: structured-sections end-to-end fixes" || true
```
