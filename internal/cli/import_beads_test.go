package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/underworld14/pine/internal/config"
	"github.com/underworld14/pine/internal/store"
	"github.com/underworld14/pine/internal/ticket"
)

const sampleBeadsJSONL = `{"_type":"issue","id":"bd-epic1","title":"Auth rewrite","description":"Rewrite auth","issue_type":"epic","status":"open","priority":1,"labels":["auth"]}
{"_type":"issue","id":"bd-task1","title":"Implement JWT","description":"Add JWT login","design":"Use HS256","acceptance_criteria":"- [ ] Login works","notes":"Spike done","issue_type":"task","status":"in_progress","priority":0,"labels":["auth","backend"],"parent":"bd-epic1","dependencies":[{"issue_id":"bd-task1","depends_on_id":"bd-epic1","type":"parent-child"},{"issue_id":"bd-task1","depends_on_id":"bd-bug1","type":"blocks"},{"issue_id":"bd-task1","depends_on_id":"bd-other","type":"related"}],"comments":[{"id":"1","author":"alice","text":"Looks good","created_at":"2026-01-15T12:00:00Z"}],"assignee":"bob","owner":"alice"}
{"_type":"issue","id":"bd-bug1","title":"Login crash","description":"Null deref","issue_type":"bug","status":"closed","priority":2,"labels":["auth"]}
{"_type":"issue","id":"bd-chore1","title":"Bump deps","issue_type":"chore","status":"open","priority":3}
{"_type":"issue","id":"bd-msg1","title":"ping","issue_type":"message","status":"open","priority":2}
{"_type":"memory","key":"pref","content":"prefer tabs"}
`

func writeBeadsFixture(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestImportBeadsFromFile(t *testing.T) {
	dir := initRepo(t)
	fixture := writeBeadsFixture(t, dir, "issues.jsonl", sampleBeadsJSONL)

	out, err := run(t, dir, "import", "beads", fixture)
	if err != nil {
		t.Fatalf("import beads: %v\n%s", err, out)
	}
	for _, want := range []string{
		"EPIC-001 ← bd-epic1",
		"TASK-001 ← bd-task1",
		"BUG-001 ← bd-bug1",
		"CHORE-001 ← bd-chore1",
		"imported 4",
		"1 infra skipped",
		"1 non-issue lines skipped",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}

	s, err := store.Open(filepath.Join(dir, ".pine"))
	if err != nil {
		t.Fatal(err)
	}

	// Types auto-registered.
	for _, prefix := range []string{"TASK", "CHORE"} {
		if _, ok := s.Config().TypeByPrefix(prefix); !ok {
			t.Errorf("expected type %s in config", prefix)
		}
	}

	epic, err := s.Get("EPIC-001")
	if err != nil {
		t.Fatal(err)
	}
	if epic.Title != "Auth rewrite" || epic.Status != "todo" || epic.Priority != "high" {
		t.Errorf("epic = %+v", epic)
	}

	task, err := s.Get("TASK-001")
	if err != nil {
		t.Fatal(err)
	}
	if task.Parent != "EPIC-001" {
		t.Errorf("task parent = %q, want EPIC-001", task.Parent)
	}
	if task.Status != "doing" || task.Priority != "critical" {
		t.Errorf("task status/prio = %s/%s", task.Status, task.Priority)
	}
	if len(task.Deps) != 1 || task.Deps[0] != "BUG-001" {
		t.Errorf("task deps = %v, want [BUG-001]", task.Deps)
	}
	for _, want := range []string{"# Description", "Add JWT login", "# Design", "Use HS256", "# Acceptance Criteria", "# Notes", "# Related", "related → bd-other", "# Comments", "alice"} {
		if !strings.Contains(task.Body, want) {
			t.Errorf("task body missing %q:\n%s", want, task.Body)
		}
	}
	if ticketBeadsID(task) != "bd-task1" {
		t.Errorf("beads provenance = %q", ticketBeadsID(task))
	}

	bug, err := s.Get("BUG-001")
	if err != nil {
		t.Fatal(err)
	}
	if bug.Status != "done" {
		t.Errorf("closed bug status = %q", bug.Status)
	}

	// Idempotent re-run.
	out, err = run(t, dir, "import", "beads", fixture)
	if err != nil {
		t.Fatalf("re-import: %v\n%s", err, out)
	}
	if !strings.Contains(out, "imported 0, skipped 4 already-imported") {
		t.Errorf("re-run summary: %q", out)
	}
}

func TestImportBeadsDryRun(t *testing.T) {
	dir := initRepo(t)
	fixture := writeBeadsFixture(t, dir, "issues.jsonl", sampleBeadsJSONL)

	out, err := run(t, dir, "import", "beads", fixture, "--dry-run")
	if err != nil {
		t.Fatalf("dry-run: %v\n%s", err, out)
	}
	if !strings.Contains(out, "would import 4") {
		t.Errorf("dry-run summary: %q", out)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, ".pine", "tickets"))
	if len(entries) != 0 {
		t.Errorf("dry-run wrote %d tickets", len(entries))
	}
	// Types should not be ensured on dry-run either.
	s, err := store.Open(filepath.Join(dir, ".pine"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Config().TypeByPrefix("TASK"); ok {
		t.Error("dry-run should not mutate config types")
	}
}

func TestImportBeadsViaBDExportStub(t *testing.T) {
	dir := initRepo(t)
	orig := bdExport
	defer func() { bdExport = orig }()
	bdExport = func(d string) ([]byte, error) {
		if d != dir {
			t.Errorf("bdExport dir = %q, want %q", d, dir)
		}
		return []byte(`{"_type":"issue","id":"bd-x","title":"From bd","issue_type":"bug","status":"open","priority":2}` + "\n"), nil
	}

	out, err := run(t, dir, "import", "beads")
	if err != nil {
		t.Fatalf("import beads (stub): %v\n%s", err, out)
	}
	if !strings.Contains(out, "BUG-001 ← bd-x") || !strings.Contains(out, "imported 1") {
		t.Errorf("output: %q", out)
	}
}

func TestImportBeadsLabelAndStateFilter(t *testing.T) {
	dir := initRepo(t)
	fixture := writeBeadsFixture(t, dir, "issues.jsonl", sampleBeadsJSONL)

	out, err := run(t, dir, "import", "beads", fixture, "--label", "backend")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "imported 1") || !strings.Contains(out, "TASK-001 ← bd-task1") {
		t.Errorf("label filter: %q", out)
	}

	dir2 := initRepo(t)
	fixture2 := writeBeadsFixture(t, dir2, "issues.jsonl", sampleBeadsJSONL)
	out, err = run(t, dir2, "import", "beads", fixture2, "--state", "closed")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "BUG-001 ← bd-bug1") || !strings.Contains(out, "imported 1") {
		t.Errorf("state=closed: %q", out)
	}
}

func TestImportBeadsNoEnsureTypes(t *testing.T) {
	dir := initRepo(t)
	fixture := writeBeadsFixture(t, dir, "one.jsonl",
		`{"_type":"issue","id":"bd-t","title":"Do thing","issue_type":"task","status":"open","priority":2}`+"\n")

	out, err := run(t, dir, "import", "beads", fixture, "--no-ensure-types")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "failed") || !strings.Contains(out, "TASK not in config") {
		t.Errorf("expected type failure: %q", out)
	}
}

func TestParseBeadsJSONL(t *testing.T) {
	issues, skipped, err := parseBeadsJSONL([]byte(sampleBeadsJSONL))
	if err != nil {
		t.Fatal(err)
	}
	if skipped != 1 {
		t.Errorf("skipped non-issue = %d", skipped)
	}
	if len(issues) != 5 { // 4 work + 1 message (infra filtered later)
		t.Errorf("issues = %d", len(issues))
	}
}

func TestResolveBeadsType(t *testing.T) {
	cases := []struct {
		in, wantPrefix string
		ok             bool
	}{
		{"bug", "BUG", true},
		{"feature", "FEAT", true},
		{"enhancement", "FEAT", true},
		{"task", "TASK", true},
		{"", "TASK", true},
		{"message", "", false},
		{"gate", "", false},
	}
	for _, c := range cases {
		prefix, _, ok := resolveBeadsType(c.in, nil)
		if ok != c.ok || prefix != c.wantPrefix {
			t.Errorf("resolveBeadsType(%q)=%q,%v want %q,%v", c.in, prefix, ok, c.wantPrefix, c.ok)
		}
	}
	prefix, _, ok := resolveBeadsType("task", map[string]string{"task": "FEAT"})
	if !ok || prefix != "FEAT" {
		t.Errorf("override = %q,%v", prefix, ok)
	}
}

func TestMapBeadsStatusPriority(t *testing.T) {
	board := config.DefaultBoard()
	if got := mapBeadsStatus("in_progress", nil, board); got != "doing" {
		t.Errorf("in_progress → %q", got)
	}
	if got := mapBeadsStatus("closed", nil, board); got != "done" {
		t.Errorf("closed → %q", got)
	}
	if got := mapBeadsStatus("open", map[string]string{"open": "testing"}, board); got != "testing" {
		t.Errorf("override → %q", got)
	}
	if got := mapBeadsStatus("weird", nil, board); got != "todo" {
		t.Errorf("unknown status fallback → %q", got)
	}
	if got := mapBeadsPriority(0); got != "critical" {
		t.Errorf("p0 → %q", got)
	}
	if got := mapBeadsPriority(99); got != "medium" {
		t.Errorf("bad prio → %q", got)
	}
}

func TestComposeBeadsBody(t *testing.T) {
	parent := "bd-e"
	body := composeBeadsBody(beadsIssue{
		Description:        "desc",
		Design:             "des",
		AcceptanceCriteria: "ac",
		Notes:              "n",
		Parent:             &parent,
		Dependencies: []beadsDep{
			{DependsOnID: "bd-e", Type: "parent-child"},
			{DependsOnID: "bd-x", Type: "discovered-from"},
		},
		Comments: []beadsComment{{Author: "a", Text: "hi", CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}},
	})
	for _, want := range []string{"# Description", "desc", "# Design", "# Acceptance Criteria", "# Notes", "# Related", "discovered-from → bd-x", "# Comments", "**a**"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in body:\n%s", want, body)
		}
	}
}

func TestTicketBeadsID(t *testing.T) {
	if ticketBeadsID(&ticket.Ticket{}) != "" {
		t.Fatal("empty")
	}
	tk := &ticket.Ticket{Extra: []ticket.ExtraField{
		{Key: "beads", Node: &yaml.Node{Value: "  bd-abc  "}},
	}}
	if got := ticketBeadsID(tk); got != "bd-abc" {
		t.Fatalf("got %q", got)
	}
}

func TestBeadsParentID(t *testing.T) {
	p := "bd-parent"
	iss := beadsIssue{Parent: &p}
	if beadsParentID(iss) != "bd-parent" {
		t.Fatal("from parent field")
	}
	iss2 := beadsIssue{Dependencies: []beadsDep{{DependsOnID: "bd-e", Type: "parent-child"}}}
	if beadsParentID(iss2) != "bd-e" {
		t.Fatal("from dep")
	}
}

func TestResolveBeadsTypeCustomAndInvalid(t *testing.T) {
	prefix, tt, ok := resolveBeadsType("spike", nil)
	if !ok || prefix != "SPIKE" || tt.Name != "Spike" {
		t.Fatalf("spike = %q %+v %v", prefix, tt, ok)
	}
	prefix, tt, ok = resolveBeadsType("my-widget", nil)
	if !ok || prefix != "MYWIDGET" {
		t.Fatalf("custom = %q %+v %v", prefix, tt, ok)
	}
	if _, _, ok := resolveBeadsType("123bad", nil); ok {
		t.Fatal("numeric-leading custom should fail")
	}
	if _, _, ok := resolveBeadsType("!!!", nil); ok {
		t.Fatal("symbol-only type should fail")
	}
}

func TestParseBeadsJSONLEdgeCases(t *testing.T) {
	raw := []byte(strings.Join([]string{
		"",
		`{"_type":"issue","id":"bd-1","title":"ok","issue_type":"bug"}`,
		`{"_type":"template","id":"t1"}`,
		`{"id":"bd-legacy","title":"no type field","issue_type":"task"}`,
		`{"_type":"","title":"no id"}`,
		`not-json`,
	}, "\n") + "\n")
	_, _, err := parseBeadsJSONL(raw)
	if err == nil {
		t.Fatal("expected JSON error")
	}

	okRaw := []byte(strings.Join([]string{
		"",
		`{"_type":"issue","id":"bd-1","title":"ok","issue_type":"bug"}`,
		`{"_type":"template","id":"t1"}`,
		`{"id":"bd-legacy","title":"legacy","issue_type":"task"}`,
		`{"_type":"","title":"no id"}`,
	}, "\n") + "\n")
	issues, skipped, err := parseBeadsJSONL(okRaw)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 2 { // bd-1 + legacy
		t.Fatalf("issues=%d want 2", len(issues))
	}
	if skipped != 2 { // template + empty-_type without id
		t.Fatalf("skipped=%d want 2", skipped)
	}
}

func TestLoadBeadsJSONLFileAndErrors(t *testing.T) {
	dir := t.TempDir()
	path := writeBeadsFixture(t, dir, "a.jsonl", `{"_type":"issue","id":"bd-1","title":"T","issue_type":"bug"}`+"\n")
	data, err := loadBeadsJSONL([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "bd-1") {
		t.Fatalf("unexpected data: %s", data)
	}
	if _, err := loadBeadsJSONL([]string{filepath.Join(dir, "missing.jsonl")}); err == nil {
		t.Fatal("expected missing file error")
	}
}

func TestImportBeadsLimitStatusMapTypeMapAndExtras(t *testing.T) {
	dir := initRepo(t)
	ext := "gh-9"
	fixture := writeBeadsFixture(t, dir, "mix.jsonl", strings.Join([]string{
		`{"_type":"issue","id":"bd-a","title":"A","issue_type":"spike","status":"open","priority":2,"external_ref":"gh-9"}`,
		`{"_type":"issue","id":"bd-b","title":"B","issue_type":"story","status":"open","priority":2}`,
		`{"_type":"issue","id":"bd-c","title":"C","issue_type":"milestone","status":"open","priority":2}`,
		`{"_type":"issue","id":"bd-skip","title":"","issue_type":"bug","status":"open","priority":2}`,
		`{"_type":"issue","id":"bd-closed","title":"Closed","issue_type":"bug","status":"closed","priority":2}`,
	}, "\n") + "\n")
	_ = ext

	out, err := run(t, dir, "import", "beads", fixture,
		"--state", "open",
		"--limit", "2",
		"--status-map", "open=testing",
		"--type-map", "spike=SPIKE",
	)
	if err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	if !strings.Contains(out, "imported 2") {
		t.Errorf("limit summary: %q", out)
	}
	if !strings.Contains(out, "filtered") {
		t.Errorf("expected filtered closed issue: %q", out)
	}

	s, err := store.Open(filepath.Join(dir, ".pine"))
	if err != nil {
		t.Fatal(err)
	}
	// First two open work items after epic-sort (none are epics): sorted by id → bd-a, bd-b.
	a, err := s.Get("SPIKE-001")
	if err != nil {
		t.Fatalf("SPIKE-001: %v\n%s", err, out)
	}
	if a.Status != "testing" {
		t.Errorf("status-map → %q", a.Status)
	}
	var hasExt bool
	for _, e := range a.Extra {
		if e.Key == "beads_external_ref" && e.Node != nil && e.Node.Value == "gh-9" {
			hasExt = true
		}
	}
	if !hasExt {
		t.Errorf("missing beads_external_ref on %v", a.Extra)
	}
}

func TestImportBeadsCycleDepSkipped(t *testing.T) {
	dir := initRepo(t)
	// Mutual blocks between two bugs → second edge would cycle after first is wired.
	fixture := writeBeadsFixture(t, dir, "cycle.jsonl", strings.Join([]string{
		`{"_type":"issue","id":"bd-1","title":"One","issue_type":"bug","status":"open","priority":2,"dependencies":[{"issue_id":"bd-1","depends_on_id":"bd-2","type":"blocks"}]}`,
		`{"_type":"issue","id":"bd-2","title":"Two","issue_type":"bug","status":"open","priority":2,"dependencies":[{"issue_id":"bd-2","depends_on_id":"bd-1","type":"blocks"}]}`,
	}, "\n") + "\n")

	out, err := run(t, dir, "import", "beads", fixture)
	if err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	if !strings.Contains(out, "would create cycle") && !strings.Contains(out, "imported 2") {
		// At least one cycle skip warning expected when wiring the second direction.
		t.Logf("output:\n%s", out)
	}
	s, err := store.Open(filepath.Join(dir, ".pine"))
	if err != nil {
		t.Fatal(err)
	}
	b1, _ := s.Get("BUG-001")
	b2, _ := s.Get("BUG-002")
	// Exactly one direction may be kept; both kept would be a cycle refused by doctor/graph.
	g := ticket.NewGraph([]*ticket.Ticket{b1, b2})
	if len(g.Cycles()) > 0 {
		t.Fatalf("imported a dependency cycle: %v / %v", b1.Deps, b2.Deps)
	}
}

func TestComposeBeadsBodyEdgeCases(t *testing.T) {
	body := composeBeadsBody(beadsIssue{
		Dependencies: []beadsDep{
			{Type: "", DependsOnID: "x"},
			{Type: "related", DependsOnID: ""},
			{Type: "relates-to", DependsOnID: "bd-z"},
		},
		Comments: []beadsComment{{Text: "anon note"}}, // empty author, zero time
	})
	if !strings.Contains(body, "relates-to → bd-z") {
		t.Fatalf("related missing: %s", body)
	}
	if !strings.Contains(body, "**unknown**: anon note") {
		t.Fatalf("anonymous comment: %s", body)
	}
	if composeBeadsBody(beadsIssue{}) != "\n" {
		t.Fatalf("empty body want newline, got %q", composeBeadsBody(beadsIssue{}))
	}
}

func TestMapBeadsStatusNilBoard(t *testing.T) {
	if got := mapBeadsStatus("open", nil, nil); got != "todo" {
		t.Fatalf("nil board = %q", got)
	}
}

func TestBeadsExtraFieldsExternalRef(t *testing.T) {
	ref := "jira-1"
	extras := beadsExtraFields(beadsIssue{
		ID:          "bd-x",
		ExternalRef: &ref,
		CreatedAt:   time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
	})
	keys := map[string]string{}
	for _, e := range extras {
		if e.Node != nil {
			keys[e.Key] = e.Node.Value
		}
	}
	if keys["beads_external_ref"] != "jira-1" || keys["beads_created"] == "" {
		t.Fatalf("%v", keys)
	}
}

func TestRealBDExportMissingBinary(t *testing.T) {
	t.Setenv("PATH", "/nonexistent")
	_, err := realBDExport("")
	if err == nil || !strings.Contains(err.Error(), "bd") {
		t.Fatalf("want bd missing error, got %v", err)
	}
}
