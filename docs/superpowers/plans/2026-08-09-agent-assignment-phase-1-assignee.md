# Agent assignment phase 1: `assignee` as a first-class field — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `assignee` a real Pine field — parsed, serialized, merged, filterable, and visible in the CLI, the HTTP API, and the board — so tickets can be owned by a named agent before any execution machinery exists.

**Architecture:** `assignee` follows the exact path `parent` and `phase` already take: a scalar on `ticket.Ticket`, a known frontmatter key written in canonical order and omitted when empty, a scalar in `Merge3`, a field on `store.Filter` and `store.CreateReq`, a field on `view.Ticket`, a query parameter and patch field on the HTTP API, and a flag on the CLI. Nothing in this phase spawns a process or reads agent config.

**Tech Stack:** Go 1.26.4 (stdlib + `gopkg.in/yaml.v3`, `spf13/cobra`, `go-chi/chi`), SvelteKit 2 with Svelte 5 runes, vitest + @testing-library/svelte.

**Spec:** `docs/superpowers/specs/2026-08-09-agent-assignment-babysitter-design.md`
**Pine ticket:** `FEAT-p0cj9t` (child of `EPIC-23747d`)

## Global Constraints

- Go toolchain is `go 1.26.4` (go.mod). Do not raise it.
- **No new module dependencies.** Everything here uses what `go.mod` already has.
- `make cover` must stay green: total coverage ≥ 90%.
- `make lint` (`go vet ./...`) clean; run `gofmt -w .` before every commit.
- `cd web && npm test` (vitest) and `npm run check` (svelte-check) must pass.
- **Backward compatibility:** tickets that already carry `assignee` as an unknown key must keep their value, must move out of `Ticket.Extra`, and must not be written twice.
- **Serialization stability:** a ticket with no assignee must serialize byte-identically to today. `assignee` is emitted only when non-empty, exactly like `parent` and `phase`.
- **Canonical key order:** `id, title, status, priority, order, assignee, labels, deps, parent, phase, links, created, updated`.
- `assignee` values are trimmed but **not** lowercased — profile names are user-chosen. Matching is case-insensitive at the filter layer.
- Commit style: conventional commits, straight to `main` (this repo's convention).

---

### Task 1: `assignee` in the ticket domain layer

**Files:**
- Modify: `internal/ticket/ticket.go:44` (struct field)
- Modify: `internal/ticket/frontmatter.go:16-19` (`knownKeys`), `:84` (Parse switch), `:125` (Serialize)
- Modify: `internal/ticket/merge.go:31` (scalar merge)
- Modify: `internal/ticket/frontmatter_test.go:117` (existing test uses `assignee` as an *unknown* key — it stops being unknown)
- Test: `internal/ticket/frontmatter_test.go`, `internal/ticket/merge_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `ticket.Ticket.Assignee string` — the trimmed agent profile name, `""` when unassigned. Parsed from the `assignee` frontmatter key; serialized in canonical position; merged as a scalar by `Merge3`.

- [ ] **Step 1: Write the failing parse/serialize tests**

Append to `internal/ticket/frontmatter_test.go`:

```go
func TestAssigneeRoundTrip(t *testing.T) {
	src := `---
id: FEAT-001
title: t
status: todo
priority: medium
assignee: backend
created: 2026-08-09T00:00:00Z
updated: 2026-08-09T00:00:00Z
---
body
`
	tk := Parse("FEAT-001", []byte(src))
	if tk.Assignee != "backend" {
		t.Fatalf("assignee = %q, want %q", tk.Assignee, "backend")
	}
	if len(tk.Extra) != 0 {
		t.Fatalf("assignee must not land in Extra, got %d extras", len(tk.Extra))
	}
	out := string(tk.Serialize())
	if !strings.Contains(out, "assignee: backend") {
		t.Errorf("assignee not serialized:\n%s", out)
	}
	// Canonical block, not appended with the unknown keys.
	if strings.Index(out, "assignee") > strings.Index(out, "created") {
		t.Errorf("assignee must precede created:\n%s", out)
	}
	if strings.Index(out, "assignee") < strings.Index(out, "priority") {
		t.Errorf("assignee must follow priority:\n%s", out)
	}
}

func TestAssigneeTrimmed(t *testing.T) {
	src := "---\nid: FEAT-003\ntitle: t\nstatus: todo\nassignee: '  backend  '\ncreated: 2026-08-09T00:00:00Z\nupdated: 2026-08-09T00:00:00Z\n---\nb\n"
	if got := Parse("FEAT-003", []byte(src)).Assignee; got != "backend" {
		t.Errorf("assignee = %q, want %q", got, "backend")
	}
}

func TestAssigneeOmittedWhenEmpty(t *testing.T) {
	src := "---\nid: FEAT-002\ntitle: t\nstatus: todo\npriority: low\ncreated: 2026-08-09T00:00:00Z\nupdated: 2026-08-09T00:00:00Z\n---\nb\n"
	out := string(Parse("FEAT-002", []byte(src)).Serialize())
	if strings.Contains(out, "assignee") {
		t.Errorf("an empty assignee must be omitted entirely:\n%s", out)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/ticket/ -run TestAssignee -v`
Expected: FAIL — `tk.Assignee undefined (type *Ticket has no field or method Assignee)`.

- [ ] **Step 3: Add the struct field**

In `internal/ticket/ticket.go`, inside `type Ticket struct`, immediately after the `Order` field:

```go
	Order    float64   // manual board sort key; 0 = unset (falls back to default sort)
	Assignee string    // agent profile that owns this ticket; "" when unassigned
	Labels   []string  // may be empty
```

- [ ] **Step 4: Register the key as known**

In `internal/ticket/frontmatter.go`, extend `knownKeys`:

```go
var knownKeys = map[string]bool{
	"id": true, "title": true, "status": true, "priority": true, "order": true,
	"assignee": true,
	"labels": true, "deps": true, "parent": true, "phase": true, "links": true,
	"created": true, "updated": true,
}
```

- [ ] **Step 5: Parse the key**

In the `switch key` block of `Parse`, add a case immediately after the `"order"` case:

```go
		case "assignee":
			t.Assignee = strings.TrimSpace(val.Value)
```

- [ ] **Step 6: Serialize the key**

In `Serialize`, immediately after the `if t.Order != 0 { … }` block and before the labels block:

```go
	if t.Assignee != "" {
		add("assignee", frontmatter.Scalar(t.Assignee))
	}
```

- [ ] **Step 7: Run the tests to verify they pass**

Run: `go test ./internal/ticket/ -run TestAssignee -v`
Expected: PASS (3 tests).

- [ ] **Step 8: Repair the unknown-keys test**

`TestUnknownKeysPreserved` in `internal/ticket/frontmatter_test.go:117` uses `assignee` as its example unknown key, which now fails (1 extra, not 2). Replace that key with one that is genuinely unknown:

```go
func TestUnknownKeysPreserved(t *testing.T) {
	src := `---
id: BUG-002
title: t
status: todo
priority: medium
reviewer: claude
estimate: 3
created: 2026-07-04T00:00:00Z
updated: 2026-07-04T00:00:00Z
---
body
`
	tk := Parse("BUG-002", []byte(src))
	if len(tk.Extra) != 2 {
		t.Fatalf("expected 2 extra keys, got %d", len(tk.Extra))
	}
	out := string(tk.Serialize())
	if !strings.Contains(out, "reviewer: claude") || !strings.Contains(out, "estimate: 3") {
		t.Errorf("extra keys not preserved:\n%s", out)
	}
	// Extra keys must come after the canonical block.
	if strings.Index(out, "reviewer") < strings.Index(out, "updated") {
		t.Errorf("extra keys not appended after canonical keys:\n%s", out)
	}
}
```

- [ ] **Step 9: Run the whole ticket package**

Run: `go test ./internal/ticket/ -v`
Expected: PASS, no failures.

- [ ] **Step 10: Write the failing merge tests**

Append to `internal/ticket/merge_test.go`:

```go
func TestMerge3AssigneeNewerWins(t *testing.T) {
	base := &Ticket{ID: "FEAT-001"}
	ours := &Ticket{ID: "FEAT-001", Assignee: "backend", Updated: time.Date(2026, 8, 9, 1, 0, 0, 0, time.UTC)}
	theirs := &Ticket{ID: "FEAT-001", Assignee: "qa", Updated: time.Date(2026, 8, 9, 2, 0, 0, 0, time.UTC)}
	m, conflict := Merge3(base, ours, theirs)
	if m.Assignee != "qa" {
		t.Errorf("assignee = %q, want %q (newer updated wins)", m.Assignee, "qa")
	}
	if !conflict {
		t.Error("a two-sided assignee divergence must be flagged as a conflict")
	}
}

func TestMerge3AssigneeOneSided(t *testing.T) {
	base := &Ticket{ID: "FEAT-001"}
	ours := &Ticket{ID: "FEAT-001", Assignee: "backend"}
	theirs := &Ticket{ID: "FEAT-001"}
	m, conflict := Merge3(base, ours, theirs)
	if m.Assignee != "backend" {
		t.Errorf("assignee = %q, want %q", m.Assignee, "backend")
	}
	if conflict {
		t.Error("a one-sided assignee change must not conflict")
	}
}
```

- [ ] **Step 11: Run the merge tests to verify they fail**

Run: `go test ./internal/ticket/ -run TestMerge3Assignee -v`
Expected: FAIL — merged `Assignee` is `""`.

- [ ] **Step 12: Merge assignee as a scalar**

In `internal/ticket/merge.go`, inside `Merge3`, immediately after the `m.Phase` line:

```go
	m.Assignee, c = mergeScalar(b.Assignee, ours.Assignee, theirs.Assignee, ours.Updated, theirs.Updated)
	conflict = conflict || c
```

- [ ] **Step 13: Run the merge tests to verify they pass**

Run: `go test ./internal/ticket/ -run TestMerge3Assignee -v`
Expected: PASS (2 tests).

- [ ] **Step 14: Commit**

```bash
gofmt -w internal/ticket
go test ./internal/ticket/
git add internal/ticket
git commit -m "feat(ticket): promote assignee to a first-class frontmatter field"
```

---

### Task 2: Store filtering and creation

**Files:**
- Modify: `internal/store/store.go:201-207` (`Filter`), `:209` (`matches`)
- Modify: `internal/store/create.go:23-34` (`CreateReq`), `:68` (`Create`)
- Test: `internal/store/store_test.go`

**Interfaces:**
- Consumes: `ticket.Ticket.Assignee` from Task 1.
- Produces:
  - `store.Filter.Assignee string` — case-insensitive exact match; `""` disables the filter.
  - `store.Filter.Unassigned bool` — when true, keeps only tickets whose assignee is empty. Combined with a non-empty `Assignee`, the result is empty (they contradict); callers must not set both.
  - `store.CreateReq.Assignee string`.

- [ ] **Step 1: Write the failing filter test**

Append to `internal/store/store_test.go`:

```go
func TestFilterByAssignee(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Create(CreateReq{Type: "FEAT", Title: "owned", Assignee: "Backend"}); err != nil {
		t.Fatalf("create owned: %v", err)
	}
	if _, err := s.Create(CreateReq{Type: "FEAT", Title: "free"}); err != nil {
		t.Fatalf("create free: %v", err)
	}

	// Case-insensitive match on the profile name.
	got := s.List(Filter{Assignee: "backend"})
	if len(got) != 1 || got[0].Title != "owned" {
		t.Fatalf("Assignee filter returned %d tickets, want 1 (%q)", len(got), "owned")
	}
	// The stored value keeps its original casing.
	if got[0].Assignee != "Backend" {
		t.Errorf("assignee = %q, want %q — casing must be preserved on write", got[0].Assignee, "Backend")
	}

	free := s.List(Filter{Unassigned: true})
	if len(free) != 1 || free[0].Title != "free" {
		t.Fatalf("Unassigned filter returned %d tickets, want 1 (%q)", len(free), "free")
	}
}
```

`newTestStore` is the existing helper in `internal/store/store_test.go`; if its name differs in the file, use whatever helper the neighbouring tests already call to build a temp store.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/store/ -run TestFilterByAssignee -v`
Expected: FAIL — `unknown field Assignee in struct literal of type CreateReq`.

- [ ] **Step 3: Add the request and filter fields**

In `internal/store/create.go`, add to `CreateReq` after `Phase`:

```go
	Phase    string
	Assignee string
	Links    []string
```

In `internal/store/store.go`, add to `Filter` after `Phase`:

```go
type Filter struct {
	Status     string
	Type       string // ID prefix, e.g. "BUG"
	Label      string
	Parent     string
	Phase      string
	Assignee   string // case-insensitive exact match; "" disables
	Unassigned bool   // keep only tickets with no assignee
}
```

- [ ] **Step 4: Apply the filter**

In `internal/store/store.go`, inside `func (f Filter) matches(t *ticket.Ticket) bool`, add before the final `return true`:

```go
	if f.Assignee != "" && !strings.EqualFold(t.Assignee, f.Assignee) {
		return false
	}
	if f.Unassigned && strings.TrimSpace(t.Assignee) != "" {
		return false
	}
```

- [ ] **Step 5: Persist the field on create**

In `internal/store/create.go`, inside the `&ticket.Ticket{…}` literal, after `Phase: req.Phase,`:

```go
		Phase:    req.Phase,
		Assignee: strings.TrimSpace(req.Assignee),
		Links:    req.Links,
```

- [ ] **Step 6: Run the test to verify it passes**

Run: `go test ./internal/store/ -run TestFilterByAssignee -v`
Expected: PASS.

- [ ] **Step 7: Run the whole store package**

Run: `go test ./internal/store/`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
gofmt -w internal/store
git add internal/store
git commit -m "feat(store): filter and create tickets by assignee"
```

---

### Task 3: View model and HTTP API

**Files:**
- Modify: `internal/view/view.go:15-28` (`Ticket`), `:70` (`Build`), `:113` (`BuildOffBranch`)
- Modify: `internal/server/tickets.go:17-23` (list filter), `:38-50` (`createBody`), `:60-70` (create handler), `:99` (`ticketPatch`), `:133` (patch apply)
- Test: `internal/server/tickets_test.go`

**Interfaces:**
- Consumes: `store.Filter.Assignee`, `store.Filter.Unassigned`, `store.CreateReq.Assignee` from Task 2.
- Produces:
  - `view.Ticket.Assignee string` with JSON tag `assignee,omitempty`.
  - `GET /api/tickets?assignee=<name>` and `GET /api/tickets?unassigned=1`.
  - `POST /api/tickets` accepts `"assignee"`.
  - `PATCH /api/tickets/{id}` accepts `"assignee"` as a nullable string; `""` clears it.

- [ ] **Step 1: Write the failing API test**

Append to `internal/server/tickets_test.go`:

```go
func TestAssigneeCreateFilterAndPatch(t *testing.T) {
	srv, _ := newTestServer(t)

	created := doJSON(t, srv, http.MethodPost, "/api/tickets", map[string]any{
		"type": "FEAT", "title": "owned", "assignee": "backend",
	})
	if created["assignee"] != "backend" {
		t.Fatalf("created assignee = %v, want %q", created["assignee"], "backend")
	}
	id, _ := created["id"].(string)

	doJSON(t, srv, http.MethodPost, "/api/tickets", map[string]any{"type": "FEAT", "title": "free"})

	list := doJSON(t, srv, http.MethodGet, "/api/tickets?assignee=BACKEND", nil)
	if n := len(list["tickets"].([]any)); n != 1 {
		t.Fatalf("?assignee returned %d tickets, want 1", n)
	}

	free := doJSON(t, srv, http.MethodGet, "/api/tickets?unassigned=1", nil)
	if n := len(free["tickets"].([]any)); n != 1 {
		t.Fatalf("?unassigned returned %d tickets, want 1", n)
	}

	cleared := doJSON(t, srv, http.MethodPatch, "/api/tickets/"+id, map[string]any{"assignee": ""})
	if _, present := cleared["assignee"]; present {
		t.Errorf("an empty assignee must be omitted from JSON, got %v", cleared["assignee"])
	}
}
```

`newTestServer` and `doJSON` are the existing helpers in `internal/server`; match the exact signatures used by the neighbouring tests in `tickets_test.go` (they may take or return different values — adapt the three call sites, not the assertions).

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/server/ -run TestAssigneeCreateFilterAndPatch -v`
Expected: FAIL — the created ticket has no `assignee` key.

- [ ] **Step 3: Add the view field**

In `internal/view/view.go`, in `type Ticket struct`, after the `Order` line:

```go
	Order    float64  `json:"order,omitempty"`
	Assignee string   `json:"assignee,omitempty"`
	Labels   []string `json:"labels"`
```

- [ ] **Step 4: Copy the field in both builders**

In `Build`, inside the `v := Ticket{…}` literal after `Order: t.Order,`:

```go
		Order:       t.Order,
		Assignee:    t.Assignee,
```

Add the identical line to the `v := Ticket{…}` literal in `BuildOffBranch` — off-branch cards must show their owner too.

- [ ] **Step 5: Read the new query parameters**

In `internal/server/tickets.go`, in `handleListTickets`:

```go
	f := store.Filter{
		Status:     q.Get("status"),
		Type:       q.Get("type"),
		Label:      q.Get("label"),
		Parent:     q.Get("parent"),
		Phase:      q.Get("phase"),
		Assignee:   q.Get("assignee"),
		Unassigned: q.Get("unassigned") != "",
	}
```

- [ ] **Step 6: Accept assignee on create**

Add to `createBody` after `Phase`:

```go
	Phase    string   `json:"phase"`
	Assignee string   `json:"assignee"`
```

and to the `store.CreateReq{…}` literal in `handleCreateTicket`:

```go
		Phase:    b.Phase,
		Assignee: b.Assignee,
```

- [ ] **Step 7: Accept assignee on patch**

Add to `ticketPatch` a nullable field alongside the others:

```go
	Assignee *string `json:"assignee"`
```

and inside the `UpdateIfMatch` callback in `handleUpdateTicket`, after the `p.Phase` block:

```go
		if p.Assignee != nil {
			u.Assignee = strings.TrimSpace(*p.Assignee)
		}
```

`strings` is already imported in this file.

- [ ] **Step 8: Run the test to verify it passes**

Run: `go test ./internal/server/ -run TestAssigneeCreateFilterAndPatch -v`
Expected: PASS.

- [ ] **Step 9: Run both packages**

Run: `go test ./internal/view/ ./internal/server/`
Expected: PASS.

- [ ] **Step 10: Commit**

```bash
gofmt -w internal/view internal/server
git add internal/view internal/server
git commit -m "feat(api): expose and filter ticket assignee over HTTP"
```

---

### Task 4: CLI — `pine assign`, create/update flags, list/ready filters

**Files:**
- Create: `internal/cli/assign.go`
- Modify: `internal/cli/root.go:51-72` (command registration)
- Modify: `internal/cli/tickets.go:20-56` (list), `:58-88` (ready), `:128-180` (create), `:186-240` (update)
- Modify: `README.md` (CLI reference)
- Test: `internal/cli/cli_test.go`

**Interfaces:**
- Consumes: `store.Filter.Assignee`, `store.Filter.Unassigned`, `store.CreateReq.Assignee`, `ticket.Ticket.Assignee`.
- Produces:
  - `pine assign <ID> <agent|none>`
  - `pine create --assignee <name>`
  - `pine update <ID> --assignee <name|none>`
  - `pine list --assignee <name>` / `--unassigned`
  - `pine ready --assignee <name>` / `--unassigned`
  - `newAssignCmd() *cobra.Command`

**Naming decision:** the spec sketched `pine ready --agent`. This plan uses `--assignee` on every command instead — one concept, one flag name. The spec has been updated to match.

- [ ] **Step 1: Write the failing CLI test**

Append to `internal/cli/cli_test.go`:

```go
func TestAssignCommand(t *testing.T) {
	dir := initRepo(t)
	if _, err := run(t, dir, "create", "--type", "feature", "--title", "owned"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := run(t, dir, "create", "--type", "feature", "--title", "free"); err != nil {
		t.Fatalf("create: %v", err)
	}

	out, err := run(t, dir, "assign", "FEAT-001", "backend")
	if err != nil {
		t.Fatalf("assign: %v (%s)", err, out)
	}
	if !strings.Contains(out, "Assigned FEAT-001 to backend") {
		t.Errorf("assign output = %q", out)
	}

	listed, err := run(t, dir, "list", "--assignee", "backend", "--json")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var views []view.Ticket
	if err := json.Unmarshal([]byte(listed), &views); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, listed)
	}
	if len(views) != 1 || views[0].ID != "FEAT-001" || views[0].Assignee != "backend" {
		t.Fatalf("list --assignee returned %+v", views)
	}

	free, _ := run(t, dir, "list", "--unassigned", "--json")
	var freeViews []view.Ticket
	if err := json.Unmarshal([]byte(free), &freeViews); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, free)
	}
	if len(freeViews) != 1 || freeViews[0].ID != "FEAT-002" {
		t.Fatalf("list --unassigned returned %+v", freeViews)
	}

	if out, err := run(t, dir, "assign", "FEAT-001", "none"); err != nil {
		t.Fatalf("unassign: %v (%s)", err, out)
	} else if !strings.Contains(out, "Unassigned FEAT-001") {
		t.Errorf("unassign output = %q", out)
	}
}

func TestCreateAndUpdateAssignee(t *testing.T) {
	dir := initRepo(t)
	if _, err := run(t, dir, "create", "--type", "bug", "--title", "b", "--assignee", "qa"); err != nil {
		t.Fatalf("create: %v", err)
	}
	shown, _ := run(t, dir, "show", "BUG-001", "--json")
	if !strings.Contains(shown, `"assignee":"qa"`) {
		t.Errorf("create --assignee not persisted: %s", shown)
	}
	if _, err := run(t, dir, "update", "BUG-001", "--assignee", "none"); err != nil {
		t.Fatalf("update: %v", err)
	}
	cleared, _ := run(t, dir, "show", "BUG-001", "--json")
	if strings.Contains(cleared, `"assignee"`) {
		t.Errorf("update --assignee none did not clear it: %s", cleared)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/cli/ -run 'TestAssignCommand|TestCreateAndUpdateAssignee' -v`
Expected: FAIL — `unknown command "assign" for "pine"`.

- [ ] **Step 3: Create the assign command**

Create `internal/cli/assign.go`:

```go
package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/underworld14/pine/internal/ticket"
)

// newAssignCmd sets or clears a ticket's agent owner. Assignment records intent
// only — nothing is executed here; see the runner introduced in phase 3.
func newAssignCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "assign <ID> <agent|none>",
		Short: "Assign a ticket to an agent profile (or 'none' to clear)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			id := normalizeID(args[0])
			agent := strings.TrimSpace(args[1])
			clear := strings.EqualFold(agent, "none")
			t, err := s.Update(id, func(u *ticket.Ticket) error {
				if clear {
					u.Assignee = ""
				} else {
					u.Assignee = agent
				}
				return nil
			})
			if err != nil {
				return err
			}
			if clear {
				fmt.Fprintf(cmd.OutOrStdout(), "Unassigned %s\n", t.ID)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Assigned %s to %s\n", t.ID, t.Assignee)
			return nil
		},
	}
}
```

- [ ] **Step 4: Register the command**

In `internal/cli/root.go`, inside `root.AddCommand(…)`, after `newUpdateCmd(),`:

```go
		newUpdateCmd(),
		newAssignCmd(),
```

- [ ] **Step 5: Add the create flag**

In `internal/cli/tickets.go`, in `newCreateCmd`: add `assignee` to the `var (…)` declaration alongside `typ, title, priority`, pass it through, and register the flag.

```go
	f.StringVar(&assignee, "assignee", "", "agent profile that owns this ticket")
```

```go
			t, err := s.Create(store.CreateReq{
				Type:     typ,
				Title:    title,
				Priority: priority,
				Labels:   labels,
				Deps:     normalizeIDs(deps),
				Parent:   normalizeID(parent),
				Phase:    phase,
				Assignee: assignee,
				Status:   status,
				Body:     body,
			})
```

- [ ] **Step 6: Add the update flag**

In `newUpdateCmd`, add `assignee` to the `var (…)` declaration, then inside the update callback after the `phase` block:

```go
				if flags.Changed("assignee") {
					if strings.EqualFold(assignee, "none") {
						u.Assignee = ""
					} else {
						u.Assignee = strings.TrimSpace(assignee)
					}
				}
```

and register:

```go
	f.StringVar(&assignee, "assignee", "", "new agent profile owner, or 'none' to clear")
```

- [ ] **Step 7: Add the list filters**

In `newListCmd`, add `assignee` to the string `var (…)` group and `unassigned` to the bool group, pass them into the filter, and register the flags:

```go
			views := collectViews(s, store.Filter{
				Status: status, Type: typ, Label: label, Parent: parent, Phase: phase,
				Assignee: assignee, Unassigned: unassigned,
			}, onlyBlocked, onlyReady)
```

```go
	f.StringVar(&assignee, "assignee", "", "filter by agent profile owner")
	f.BoolVar(&unassigned, "unassigned", false, "only tickets with no assignee")
```

- [ ] **Step 8: Add the ready filters**

In `newReadyCmd`, replace `var asJSON bool` with:

```go
	var (
		asJSON     bool
		assignee   string
		unassigned bool
	)
```

pass the filter:

```go
			ready := collectViews(s, store.Filter{Assignee: assignee, Unassigned: unassigned}, false, true)
```

and register:

```go
	cmd.Flags().StringVar(&assignee, "assignee", "", "only tickets owned by this agent profile")
	cmd.Flags().BoolVar(&unassigned, "unassigned", false, "only tickets with no assignee")
```

Note: `withEpicContext` still injects parent epics for tree output even when the epic itself does not match the filter. That is intended — the epic is context, not a result. JSON output stays the filtered set only.

- [ ] **Step 9: Run the tests to verify they pass**

Run: `go test ./internal/cli/ -run 'TestAssignCommand|TestCreateAndUpdateAssignee' -v`
Expected: PASS (2 tests).

- [ ] **Step 10: Run the whole CLI package**

Run: `go test ./internal/cli/`
Expected: PASS.

- [ ] **Step 11: Document the commands**

In `README.md`, in the CLI reference where `pine update` and `pine list` are documented, add:

```markdown
### Assigning work to an agent

Every ticket can name the agent profile that owns it. Assignment is intent
only — it records who should do the work, and nothing runs until you ask.

```sh
pine assign FEAT-13zqna backend    # set the owner
pine assign FEAT-13zqna none       # clear it

pine create --type feature --title "…" --assignee qa
pine update FEAT-13zqna --assignee frontend

pine list  --assignee backend      # what backend owns
pine ready --assignee backend      # what backend can start right now
pine list  --unassigned            # the triage pile
```
```

- [ ] **Step 12: Commit**

```bash
gofmt -w internal/cli
go test ./internal/cli/
git add internal/cli README.md
git commit -m "feat(cli): add pine assign and --assignee filters"
```

---

### Task 5: Show the owner in `pine list` tree output

**Files:**
- Modify: `internal/cli/list_tree.go:33` (`formatPrettyTicket`)
- Test: `internal/cli/list_tree_test.go`

**Interfaces:**
- Consumes: `view.Ticket.Assignee` from Task 3.
- Produces: tree lines render `@<assignee>` immediately after the title, before the epic progress counter and the blocker annotation.

- [ ] **Step 1: Write the failing renderer test**

Append to `internal/cli/list_tree_test.go`:

```go
func TestFormatPrettyTicketShowsAssignee(t *testing.T) {
	line := formatPrettyTicket(view.Ticket{
		ID: "FEAT-001", Type: "FEAT", Title: "Add login", Priority: "high", Assignee: "backend",
	})
	if !strings.Contains(line, "@backend") {
		t.Errorf("assignee missing from tree line: %q", line)
	}
	if strings.Index(line, "@backend") < strings.Index(line, "Add login") {
		t.Errorf("assignee must follow the title: %q", line)
	}
}

func TestFormatPrettyTicketOmitsEmptyAssignee(t *testing.T) {
	line := formatPrettyTicket(view.Ticket{ID: "FEAT-002", Type: "FEAT", Title: "No owner"})
	if strings.Contains(line, "@") {
		t.Errorf("unassigned ticket must not render an @ marker: %q", line)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/cli/ -run TestFormatPrettyTicket -v`
Expected: FAIL — `@backend` is absent.

- [ ] **Step 3: Render the marker**

In `internal/cli/list_tree.go`, in `formatPrettyTicket`, immediately after the title block and before the `EpicProgress` block:

```go
	if v.Assignee != "" {
		b.WriteString(" @")
		b.WriteString(v.Assignee)
	}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/cli/ -run TestFormatPrettyTicket -v`
Expected: PASS (2 tests).

- [ ] **Step 5: Run the package and check no golden output broke**

Run: `go test ./internal/cli/`
Expected: PASS. Existing tree tests use tickets with no assignee, so their expected output is unchanged.

- [ ] **Step 6: Commit**

```bash
gofmt -w internal/cli
git add internal/cli
git commit -m "feat(cli): show the assignee in list tree output"
```

---

### Task 6: Board card badge and text search

**Files:**
- Modify: `web/src/lib/api.ts:19-44` (`Ticket` interface)
- Modify: `web/src/lib/board-filter.ts:26` (`matchesFilter` haystack)
- Modify: `web/src/lib/components/TicketCard.svelte:37` (chip row)
- Test: `web/src/lib/board-filter.test.ts`, `web/src/lib/components/TicketCard.test.ts`

**Interfaces:**
- Consumes: the `assignee` JSON field from Task 3.
- Produces: `Ticket.assignee?: string` in the TS model; the board's free-text filter matches it; the card renders `@<assignee>` as a chip.

**Scope note:** a dedicated assignee facet in `BoardFilterBar` is deliberately **not** in this phase. It lands in phase 7 alongside `/agents`, when profile names are known and can populate a real chip list. Free-text search covers the need until then.

- [ ] **Step 1: Write the failing filter test**

Append to `web/src/lib/board-filter.test.ts`, using whatever ticket factory the neighbouring tests in that file already define:

```ts
it('free text matches the assignee', () => {
  const owned = mk({ id: 'FEAT-1', title: 'Add login', assignee: 'backend' });
  const other = mk({ id: 'FEAT-2', title: 'Add search' });
  const f = { ...emptyFilter(), text: 'backend' };
  expect(matchesFilter(owned, f)).toBe(true);
  expect(matchesFilter(other, f)).toBe(false);
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd web && npx vitest run src/lib/board-filter.test.ts`
Expected: FAIL — `expect(matchesFilter(owned, f)).toBe(true)` receives `false`.

- [ ] **Step 3: Add the field to the TS model**

In `web/src/lib/api.ts`, in `export interface Ticket`, after `order?: number;`:

```ts
  order?: number;
  // Agent profile that owns this ticket; absent when unassigned.
  assignee?: string;
  labels: string[];
```

- [ ] **Step 4: Extend the search haystack**

In `web/src/lib/board-filter.ts`, in `matchesFilter`:

```ts
    const hay = `${t.id} ${t.title} ${t.assignee ?? ''} ${t.labels.join(' ')}`.toLowerCase();
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `cd web && npx vitest run src/lib/board-filter.test.ts`
Expected: PASS.

- [ ] **Step 6: Write the failing card test**

Append to the `describe('TicketCard', …)` block in `web/src/lib/components/TicketCard.test.ts`:

```ts
  it('renders an @assignee chip when the ticket is owned', () => {
    render(TicketCard, { ticket: mk({ assignee: 'backend' }) });
    expect(screen.getByText('@backend')).toBeTruthy();
  });

  it('renders no @ chip when the ticket is unassigned', () => {
    render(TicketCard, { ticket: mk() });
    expect(screen.queryByText(/^@/)).toBeNull();
  });
```

- [ ] **Step 7: Run the test to verify it fails**

Run: `cd web && npx vitest run src/lib/components/TicketCard.test.ts`
Expected: FAIL — `Unable to find an element with the text: @backend`.

- [ ] **Step 8: Render the chip**

In `web/src/lib/components/TicketCard.svelte`, in the `.row` block immediately before the `{#each ticket.labels.slice(0, 2) as l}` loop:

```svelte
      {#if ticket.assignee}
        <span class="chip" style="--c: {labelColor(ticket.assignee)}" title="Assigned to {ticket.assignee}">@{ticket.assignee}</span>
      {/if}
```

This reuses the existing `chip` class and the already-imported `labelColor` helper — no new CSS.

- [ ] **Step 9: Run the tests to verify they pass**

Run: `cd web && npx vitest run src/lib/components/TicketCard.test.ts`
Expected: PASS.

- [ ] **Step 10: Run the full web suite and the type check**

Run: `cd web && npm test && npm run check`
Expected: both PASS, zero svelte-check errors.

- [ ] **Step 11: Commit**

```bash
git add web/src/lib/api.ts web/src/lib/board-filter.ts web/src/lib/board-filter.test.ts web/src/lib/components/TicketCard.svelte web/src/lib/components/TicketCard.test.ts
git commit -m "feat(web): show assignee on cards and match it in board search"
```

---

### Task 7: Verify the phase and close the ticket

**Files:**
- Modify: `.pine/tickets/FEAT-p0cj9t.md`

- [ ] **Step 1: Run the full Go suite with the coverage gate**

Run: `make cover`
Expected: PASS, `total coverage: ≥90%`. If it dropped below 90, add table cases to the `internal/ticket` tests — that package is pure and the cheapest place to recover coverage.

- [ ] **Step 2: Run the linter and the web suite**

Run: `make lint && make test-web && make check-web`
Expected: all PASS.

- [ ] **Step 3: Verify the round-trip on a real ticket**

```bash
make build-dev
./pine assign EPIC-23747d backend
./pine list --assignee backend
./pine assign EPIC-23747d none
git diff .pine/tickets/EPIC-23747d.md
```

Expected: the assign writes `assignee: backend` between `priority` and `created`; clearing it removes the line and leaves the rest of the file byte-identical.

- [ ] **Step 4: Close the ticket with evidence**

```bash
./pine close FEAT-p0cj9t --evidence
```

- [ ] **Step 5: Commit**

```bash
git add .pine/tickets
git commit -m "chore(pine): close agent assignment phase 1"
```

---

## Self-review

**Spec coverage for phase 1** — the build order lists "`assignee` as a first-class field; `pine assign`; `--assignee` filters in `list`, `ready`, and the board."

| Requirement | Task |
|---|---|
| `assignee` first-class (parse, serialize, merge) | 1 |
| Existing extra-key tickets keep their value | 1 (step 8 proves it is no longer an extra) |
| Store filtering | 2 |
| API exposure and filtering | 3 |
| `pine assign` | 4 |
| `--assignee` on create/update/list/ready | 4 |
| `--unassigned` triage filter | 2, 3, 4 |
| Board visibility | 5 (CLI tree), 6 (web card) |
| Coverage gate held | 7 |

**Deviations from the spec, both deliberate:**
1. `pine ready --agent` became `pine ready --assignee` — one concept, one flag name. The spec line has been updated to match.
2. The dedicated `BoardFilterBar` assignee facet moved to phase 7, where real profile names exist to populate it. Free-text search covers it here.

**Type consistency** — `Assignee string` (Go) / `assignee?: string` (TS) is used identically in `ticket.Ticket`, `store.Filter`, `store.CreateReq`, `view.Ticket`, `createBody`, `ticketPatch` (as `*string`), and the TS `Ticket`. `Unassigned bool` appears only on `store.Filter` and as the `?unassigned=` query parameter and `--unassigned` flag.

**Helper-name caveat** — Tasks 2, 3 and 6 call existing test helpers (`newTestStore`, `newTestServer`, `doJSON`, the vitest `mk` factory) whose exact signatures live in the neighbouring test files. Match the local call pattern; the assertions are what matter.
