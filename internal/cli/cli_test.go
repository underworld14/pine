package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/underworld14/pine/internal/view"
)

// run executes the pine command tree against dir and returns combined output.
func run(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	root := newRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs(append([]string{"-C", dir}, args...))
	err := root.Execute()
	return buf.String(), err
}

func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if _, err := run(t, dir, "init", "--skip-agents"); err != nil {
		t.Fatalf("init: %v", err)
	}
	// Pin sequential IDs so these tests can assert on BUG-001, FEAT-001, etc.
	// (hash IDs are covered by the store tests).
	cfgPath := filepath.Join(dir, ".pine", "config.json")
	if raw, err := os.ReadFile(cfgPath); err == nil {
		patched := strings.ReplaceAll(string(raw), `"idStyle":"hash"`, `"idStyle":"sequential"`)
		os.WriteFile(cfgPath, []byte(patched), 0o644)
	}
	return dir
}

func TestInitCreatesStructure(t *testing.T) {
	dir := initRepo(t)
	for _, p := range []string{
		".pine/config.json", ".pine/board.json",
		".pine/templates/bug.md", ".pine/prompts/fix.md",
		".pine/learnings",
	} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Errorf("missing %s: %v", p, err)
		}
	}
	// init is idempotent.
	out, err := run(t, dir, "init", "--skip-agents")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "exists") {
		t.Errorf("second init should report existing files:\n%s", out)
	}
}

func TestCreateListJSON(t *testing.T) {
	dir := initRepo(t)
	if _, err := run(t, dir, "create", "--type", "bug", "--title", "First bug", "-p", "high", "-l", "ui"); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, dir, "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var views []view.Ticket
	if err := json.Unmarshal([]byte(out), &views); err != nil {
		t.Fatalf("list --json not valid: %v\n%s", err, out)
	}
	if len(views) != 1 || views[0].ID != "BUG-001" {
		t.Fatalf("views = %+v", views)
	}
	if views[0].Priority != "high" || len(views[0].Labels) != 1 {
		t.Errorf("fields = %+v", views[0])
	}
}

func TestDepsBlockingReadyAndClose(t *testing.T) {
	dir := initRepo(t)
	run(t, dir, "create", "--type", "feature", "--title", "dep") // FEAT-001
	run(t, dir, "create", "--type", "bug", "--title", "blocked") // BUG-001
	if _, err := run(t, dir, "dep", "add", "BUG-001", "FEAT-001"); err != nil {
		t.Fatal(err)
	}

	out, _ := run(t, dir, "list", "--blocked", "--json")
	var blocked []view.Ticket
	json.Unmarshal([]byte(out), &blocked)
	if len(blocked) != 1 || blocked[0].ID != "BUG-001" {
		t.Fatalf("blocked = %+v", blocked)
	}

	out, _ = run(t, dir, "ready", "--json")
	var ready []view.Ticket
	json.Unmarshal([]byte(out), &ready)
	if !containsID(ready, "FEAT-001") || containsID(ready, "BUG-001") {
		t.Fatalf("ready should have FEAT-001 not BUG-001: %+v", ready)
	}

	if _, err := run(t, dir, "close", "FEAT-001"); err != nil {
		t.Fatal(err)
	}
	out, _ = run(t, dir, "ready", "--json")
	json.Unmarshal([]byte(out), &ready)
	if !containsID(ready, "BUG-001") {
		t.Fatalf("BUG-001 should be ready after closing its dep: %+v", ready)
	}
}

func TestCycleRefused(t *testing.T) {
	dir := initRepo(t)
	run(t, dir, "create", "--type", "bug", "--title", "a") // BUG-001
	run(t, dir, "create", "--type", "bug", "--title", "b") // BUG-002
	run(t, dir, "dep", "add", "BUG-001", "BUG-002")
	_, err := run(t, dir, "dep", "add", "BUG-002", "BUG-001")
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected cycle refusal, got %v", err)
	}
}

func TestUpdateFields(t *testing.T) {
	dir := initRepo(t)
	run(t, dir, "create", "--type", "bug", "--title", "x", "-l", "one")
	if _, err := run(t, dir, "update", "BUG-001", "--status", "doing", "--add-label", "two", "--rm-label", "one"); err != nil {
		t.Fatal(err)
	}
	out, _ := run(t, dir, "show", "BUG-001", "--json")
	var v view.Ticket
	json.Unmarshal([]byte(out), &v)
	if v.Status != "doing" {
		t.Errorf("status = %s", v.Status)
	}
	if len(v.Labels) != 1 || v.Labels[0] != "two" {
		t.Errorf("labels = %v", v.Labels)
	}
}

func TestEpicProgress(t *testing.T) {
	dir := initRepo(t)
	run(t, dir, "create", "--type", "epic", "--title", "epic")                      // EPIC-001
	run(t, dir, "create", "--type", "bug", "--title", "c1", "--parent", "EPIC-001") // BUG-001
	run(t, dir, "create", "--type", "bug", "--title", "c2", "--parent", "EPIC-001") // BUG-002
	run(t, dir, "close", "BUG-001")
	out, _ := run(t, dir, "show", "EPIC-001", "--json")
	var v view.Ticket
	json.Unmarshal([]byte(out), &v)
	if v.EpicProgress == nil || v.EpicProgress.Done != 1 || v.EpicProgress.Total != 2 {
		t.Fatalf("epic progress = %+v", v.EpicProgress)
	}
}

// createdID extracts the id from a "Created BUG-…: title" create message.
func createdID(out string) string {
	line := strings.TrimPrefix(strings.TrimSpace(out), "Created ")
	if i := strings.IndexByte(line, ':'); i >= 0 {
		return line[:i]
	}
	return ""
}

// TestHashIDsThroughCLI exercises the default hash id style end-to-end: every
// command must accept the exact (lowercase-suffix) id — regression guard for the
// bug where the CLI uppercased whole ids and broke hash lookups/deps.
func TestHashIDsThroughCLI(t *testing.T) {
	dir := t.TempDir()
	if _, err := run(t, dir, "init", "--skip-agents"); err != nil { // hash by default
		t.Fatal(err)
	}
	out, _ := run(t, dir, "create", "--type", "bug", "--title", "hashed")
	id := createdID(out)
	if !strings.HasPrefix(id, "BUG-") || len(id) <= len("BUG-") {
		t.Fatalf("expected a hash id, got %q", id)
	}

	if _, err := run(t, dir, "update", id, "--status", "doing"); err != nil {
		t.Fatalf("update %s: %v", id, err)
	}
	out2, _ := run(t, dir, "show", id, "--json")
	var v view.Ticket
	json.Unmarshal([]byte(out2), &v)
	if v.Status != "doing" {
		t.Errorf("status = %s (hash id round-trip failed)", v.Status)
	}

	// A dependency referencing a hash id must resolve, not go dangling.
	depOut, _ := run(t, dir, "create", "--type", "feature", "--title", "dep")
	depID := createdID(depOut)
	if _, err := run(t, dir, "dep", "add", id, depID); err != nil {
		t.Fatalf("dep add: %v", err)
	}
	blk, _ := run(t, dir, "show", id, "--json")
	json.Unmarshal([]byte(blk), &v)
	if !v.Blocked || len(v.Dangling) != 0 {
		t.Errorf("bug should be blocked by an existing dep, got blocked=%v dangling=%v", v.Blocked, v.Dangling)
	}
}

func containsID(vs []view.Ticket, id string) bool {
	for _, v := range vs {
		if v.ID == id {
			return true
		}
	}
	return false
}

func TestSetupAgents(t *testing.T) {
	dir := initRepo(t)
	out, err := run(t, dir, "setup", "agents")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "AGENTS.md") {
		t.Fatalf("expected install output:\n%s", out)
	}
	path := filepath.Join(dir, "AGENTS.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "pine:begin") || !strings.Contains(string(data), "pine ready") {
		t.Fatalf("AGENTS.md missing pine section:\n%s", data)
	}
	check, err := run(t, dir, "setup", "agents", "--check")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(check, "current") {
		t.Fatalf("expected current status:\n%s", check)
	}
}

func TestSetupYesInstallsAll(t *testing.T) {
	dir := initRepo(t)
	if _, err := run(t, dir, "setup", "agent", "-y"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"AGENTS.md", "CLAUDE.md", "GEMINI.md"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
		if !strings.Contains(string(data), "pine:begin") {
			t.Fatalf("%s missing pine section", name)
		}
		if !strings.Contains(string(data), "Persistent learnings") {
			t.Fatalf("%s missing learnings subsection", name)
		}
	}
}

func TestLearnCreateListSearchDoctor(t *testing.T) {
	dir := initRepo(t)
	out, err := run(t, dir, "learn", "Always use the query builder", "--scope", "global", "--tags", "db,test")
	if err != nil {
		t.Fatalf("learn: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Captured LRN-") {
		t.Fatalf("unexpected output:\n%s", out)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, ".pine", "learnings"))
	if len(entries) != 1 {
		t.Fatalf("expected 1 learning file, got %d", len(entries))
	}

	if _, err := run(t, dir, "learn", "Prefer CSS variables", "--scope", "global", "--tags", "ui"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, dir, "create", "--type", "bug", "--title", "schema"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, dir, "learn", "Ticket-scoped insight", "--scope", "ticket", "--ticket", "BUG-001", "--tags", "db"); err != nil {
		t.Fatal(err)
	}

	listOut, err := run(t, dir, "learn", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var listed []view.Learning
	if err := json.Unmarshal([]byte(listOut), &listed); err != nil {
		t.Fatalf("list json: %v\n%s", err, listOut)
	}
	if len(listed) != 3 {
		t.Fatalf("want 3 learnings, got %d", len(listed))
	}

	searchOut, err := run(t, dir, "learn", "search", "query builder", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var hits []view.LearningHit
	if err := json.Unmarshal([]byte(searchOut), &hits); err != nil {
		t.Fatalf("search json: %v\n%s", err, searchOut)
	}
	if len(hits) == 0 || !strings.Contains(hits[0].Body, "query builder") {
		t.Fatalf("search missed query builder:\n%s", searchOut)
	}

	docOut, err := run(t, dir, "doctor")
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, docOut)
	}
	if strings.Contains(strings.ToLower(docOut), "malformed") {
		t.Fatalf("doctor reported learning errors:\n%s", docOut)
	}
}

func TestLearnSupersedes(t *testing.T) {
	dir := initRepo(t)
	out, err := run(t, dir, "learn", "old UNIQUE_STALE_ZZZ")
	if err != nil {
		t.Fatal(err)
	}
	oldID := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(out), "Captured "))
	out2, err := run(t, dir, "learn", "new UNIQUE_TIP_YYY", "--supersedes", oldID)
	if err != nil {
		t.Fatal(err)
	}
	newID := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(out2), "Captured "))

	show, err := run(t, dir, "learn", "show", oldID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(show, "superseded by: "+newID) {
		t.Fatalf("show old missing superseded by:\n%s", show)
	}
	showNew, err := run(t, dir, "learn", "show", newID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(showNew, "supersedes: "+oldID) {
		t.Fatalf("show new missing supersedes:\n%s", showNew)
	}

	listOut, err := run(t, dir, "learn", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var listed []view.Learning
	if err := json.Unmarshal([]byte(listOut), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ID != newID {
		t.Fatalf("default list should only show tip: %#v", listed)
	}

	searchOut, err := run(t, dir, "learn", "search", "UNIQUE_STALE_ZZZ", "--json")
	if err != nil {
		t.Fatalf("default search: %v\n%s", err, searchOut)
	}
	var hits []view.LearningHit
	if err := json.Unmarshal([]byte(searchOut), &hits); err != nil {
		t.Fatalf("search json: %v\n%s", err, searchOut)
	}
	if len(hits) != 0 {
		t.Fatalf("default search should not find stale text: %s", searchOut)
	}
	searchInc, err := run(t, dir, "learn", "search", "UNIQUE_STALE_ZZZ", "--include-superseded", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(searchInc), &hits); err != nil {
		t.Fatalf("include search json: %v\n%s", err, searchInc)
	}
	if len(hits) == 0 {
		t.Fatalf("include-superseded should find stale text: %s", searchInc)
	}
}

func TestLearnCites(t *testing.T) {
	dir := initRepo(t)
	citedRel := "internal/foo.go"
	citedAbs := filepath.Join(dir, citedRel)
	if err := os.MkdirAll(filepath.Dir(citedAbs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(citedAbs, []byte("package foo\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := run(t, dir, "learn", "race in UNIQUE_CITE_AAA", "--cites", citedRel)
	if err != nil {
		t.Fatal(err)
	}
	id := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(out), "Captured "))

	show, err := run(t, dir, "learn", "show", id)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(show, "✓ "+citedRel) {
		t.Fatalf("show should mark cite present:\n%s", show)
	}

	docOut, err := run(t, dir, "doctor")
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, docOut)
	}
	if strings.Contains(docOut, "dangling cite") {
		t.Fatalf("doctor should not warn while file exists:\n%s", docOut)
	}

	if err := os.Remove(citedAbs); err != nil {
		t.Fatal(err)
	}

	show2, err := run(t, dir, "learn", "show", id)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(show2, "✗ "+citedRel) {
		t.Fatalf("show should mark cite missing:\n%s", show2)
	}

	docOut2, _ := run(t, dir, "doctor")
	if !strings.Contains(docOut2, "dangling cite "+citedRel) {
		t.Fatalf("doctor should report dangling cite:\n%s", docOut2)
	}

	listOut, err := run(t, dir, "learn", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var listed []view.Learning
	if err := json.Unmarshal([]byte(listOut), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Fatalf("default list should hide citation-stale: %#v", listed)
	}

	listInc, err := run(t, dir, "learn", "list", "--include-stale", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(listInc), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ID != id || !listed[0].Stale {
		t.Fatalf("include-stale should show marked entry: %#v", listed)
	}

	searchOut, err := run(t, dir, "learn", "search", "UNIQUE_CITE_AAA", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var hits []view.LearningHit
	if err := json.Unmarshal([]byte(searchOut), &hits); err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Fatalf("default search should hide citation-stale: %s", searchOut)
	}
	searchInc, err := run(t, dir, "learn", "search", "UNIQUE_CITE_AAA", "--include-stale", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(searchInc), &hits); err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || !hits[0].Stale {
		t.Fatalf("include-stale search: %#v", hits)
	}
}

func TestLearnCitesAndSupersedesIndependent(t *testing.T) {
	dir := initRepo(t)
	citedRel := "internal/both.go"
	citedAbs := filepath.Join(dir, citedRel)
	if err := os.MkdirAll(filepath.Dir(citedAbs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(citedAbs, []byte("package both\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := run(t, dir, "learn", "old BOTH_FILTER_QQQ", "--cites", citedRel)
	if err != nil {
		t.Fatal(err)
	}
	oldID := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(out), "Captured "))
	if _, err := run(t, dir, "learn", "new tip", "--supersedes", oldID); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(citedAbs); err != nil {
		t.Fatal(err)
	}

	listOut, err := run(t, dir, "learn", "list", "--include-superseded", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var listed []view.Learning
	if err := json.Unmarshal([]byte(listOut), &listed); err != nil {
		t.Fatal(err)
	}
	for _, l := range listed {
		if l.ID == oldID {
			t.Fatalf("include-superseded alone should still hide citation-stale: %#v", listed)
		}
	}

	listStale, err := run(t, dir, "learn", "list", "--include-stale", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(listStale), &listed); err != nil {
		t.Fatal(err)
	}
	for _, l := range listed {
		if l.ID == oldID {
			t.Fatalf("include-stale alone should still hide superseded: %#v", listed)
		}
	}

	listBoth, err := run(t, dir, "learn", "list", "--include-superseded", "--include-stale", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(listBoth), &listed); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, l := range listed {
		if l.ID == oldID {
			found = true
		}
	}
	if !found {
		t.Fatalf("both flags should show entry: %#v", listed)
	}
}

func TestLearnSupersedeAndRm(t *testing.T) {
	dir := initRepo(t)
	// Capture an initial global learning.
	if _, err := run(t, dir, "learn", "old rule about retries", "--tags", "net"); err != nil {
		t.Fatalf("learn: %v", err)
	}
	// Supersede it; the new learning should inherit tags.
	out, err := run(t, dir, "learn", "supersede", "LRN-001", "new rule about retries")
	if err != nil {
		t.Fatalf("supersede: %v", err)
	}
	if !strings.Contains(out, "supersedes LRN-001") {
		t.Errorf("supersede output = %q", out)
	}
	// list hides the superseded LRN-001, shows LRN-002.
	out, err = run(t, dir, "learn", "list")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "LRN-001") || !strings.Contains(out, "LRN-002") {
		t.Errorf("list after supersede = %q", out)
	}
	// rm --yes deletes LRN-002.
	if _, err := run(t, dir, "learn", "rm", "LRN-002", "--yes"); err != nil {
		t.Fatalf("rm: %v", err)
	}
	out, _ = run(t, dir, "learn", "list", "--include-superseded")
	if strings.Contains(out, "LRN-002") {
		t.Errorf("LRN-002 should be gone after rm:\n%s", out)
	}
}

func TestLearnComponentScopeCLI(t *testing.T) {
	dir := initRepo(t)
	if _, err := run(t, dir, "learn", "prefer atomic writes here", "--scope", "component", "--component", "internal/store"); err != nil {
		t.Fatalf("learn component: %v", err)
	}
	out, err := run(t, dir, "learn", "list", "--scope", "component", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var got []view.Learning
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if len(got) != 1 || got[0].Component != "internal/store" || got[0].Scope != "component" {
		t.Errorf("component learning = %+v", got)
	}
	// Missing --component is rejected.
	if _, err := run(t, dir, "learn", "x", "--scope", "component"); err == nil {
		t.Error("component scope without --component should error")
	}
}

func TestDoctorFixCLI(t *testing.T) {
	dir := initRepo(t)
	// Plant a ticket whose frontmatter id disagrees with its filename.
	body := "---\nid: BUG-999\ntitle: mismatch\nstatus: todo\ncreated: 2026-07-11T00:00:00Z\nupdated: 2026-07-11T00:00:00Z\n---\nbody\n"
	if err := os.WriteFile(filepath.Join(dir, ".pine", "tickets", "BUG-001.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	// --dry-run reports it as fixable, changes nothing.
	out, _ := run(t, dir, "doctor", "--dry-run")
	if !strings.Contains(out, "can be auto-fixed") || !strings.Contains(out, "[fixable]") {
		t.Errorf("dry-run output = %q", out)
	}
	data, _ := os.ReadFile(filepath.Join(dir, ".pine", "tickets", "BUG-001.md"))
	if !strings.Contains(string(data), "BUG-999") {
		t.Error("dry-run must not modify files")
	}
	// --json includes a fixable finding.
	out, _ = run(t, dir, "doctor", "--json")
	if !strings.Contains(out, "\"fixable\": true") || !strings.Contains(out, "frontmatter-id-mismatch") {
		t.Errorf("json output = %q", out)
	}
	// --fix applies it; a follow-up doctor is clean of the mismatch.
	out, _ = run(t, dir, "doctor", "--fix")
	if !strings.Contains(out, "fixed:") {
		t.Errorf("fix output = %q", out)
	}
	data, _ = os.ReadFile(filepath.Join(dir, ".pine", "tickets", "BUG-001.md"))
	if strings.Contains(string(data), "BUG-999") || !strings.Contains(string(data), "id: BUG-001") {
		t.Errorf("fix did not canonicalize id:\n%s", data)
	}
}

func TestDoctorFixJSONIsValid(t *testing.T) {
	dir := initRepo(t)
	// Plant a fixable frontmatter-id mismatch.
	body := "---\nid: BUG-999\ntitle: mismatch\nstatus: todo\ncreated: 2026-07-11T00:00:00Z\nupdated: 2026-07-11T00:00:00Z\n---\nbody\n"
	if err := os.WriteFile(filepath.Join(dir, ".pine", "tickets", "BUG-001.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _ := run(t, dir, "doctor", "--fix", "--json")
	var findings []map[string]any
	if err := json.Unmarshal([]byte(out), &findings); err != nil {
		t.Fatalf("doctor --fix --json must emit valid JSON, got parse error %v:\n%s", err, out)
	}
	// The fix was applied, so the mismatch should not remain in the JSON.
	for _, f := range findings {
		if code, _ := f["code"].(string); code == "frontmatter-id-mismatch" {
			t.Errorf("fixed finding should not remain in the post-fix JSON: %v", f)
		}
	}
}
