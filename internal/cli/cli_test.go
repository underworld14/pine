package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/underworld14/pine/internal/config"
	"github.com/underworld14/pine/internal/doctor"
	"github.com/underworld14/pine/internal/learning"
	"github.com/underworld14/pine/internal/store"
	"github.com/underworld14/pine/internal/tui"
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
		".pine/.gitignore",
	} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Errorf("missing %s: %v", p, err)
		}
	}
	gi, err := os.ReadFile(filepath.Join(dir, ".pine", ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gi), "attachments/") {
		t.Errorf("default init should ignore attachments:\n%s", gi)
	}
	if strings.Contains(string(gi), "tickets/") {
		t.Errorf("default init should track tickets:\n%s", gi)
	}
	cfg, err := config.Load(filepath.Join(dir, ".pine", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Sync.Tickets || cfg.Sync.Attachments {
		t.Errorf("config.sync = %+v, want tickets on / attachments off", cfg.Sync)
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

func TestInitSyncFlags(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, dir, "init", "--skip-agents", "--no-sync-tickets", "--sync-attachments")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "tickets=off") || !strings.Contains(out, "attachments=on") {
		t.Errorf("expected sync summary in output:\n%s", out)
	}
	gi, err := os.ReadFile(filepath.Join(dir, ".pine", ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gi), "tickets/") {
		t.Errorf("expected tickets/ ignore:\n%s", gi)
	}
	if strings.Contains(string(gi), "attachments/") {
		t.Errorf("attachments should be tracked:\n%s", gi)
	}
	cfg, err := config.Load(filepath.Join(dir, ".pine", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Sync.Tickets || !cfg.Sync.Attachments {
		t.Errorf("config.sync = %+v", cfg.Sync)
	}
}

func TestSetupSyncFlags(t *testing.T) {
	dir := initRepo(t)
	out, err := run(t, dir, "setup", "sync", "--no-sync-tickets", "--no-sync-attachments")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "tickets=off") {
		t.Errorf("expected summary:\n%s", out)
	}
	gi, err := os.ReadFile(filepath.Join(dir, ".pine", ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gi), "tickets/") || !strings.Contains(string(gi), "attachments/") {
		t.Errorf("expected both ignores:\n%s", gi)
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
		if !strings.Contains(string(data), "pine learn") || !strings.Contains(string(data), "load the pine skill") {
			t.Fatalf("%s missing summary / skill pointer:\n%s", name, data)
		}
	}
	for _, skill := range []string{
		".agents/skills/pine/SKILL.md",
		".claude/skills/pine/SKILL.md",
	} {
		data, err := os.ReadFile(filepath.Join(dir, skill))
		if err != nil {
			t.Fatalf("missing skill %s: %v", skill, err)
		}
		if !strings.Contains(string(data), "Persistent learnings") || !strings.Contains(string(data), "Essential commands") {
			t.Fatalf("%s should hold the full workflow:\n%s", skill, data)
		}
	}
	for _, hook := range []string{
		".claude/settings.json",
		".codex/hooks.json",
		".cursor/hooks.json",
		".codex/hooks/pine-learn-reminder.sh",
		".cursor/hooks/pine-learn-reminder.sh",
	} {
		if _, err := os.Stat(filepath.Join(dir, hook)); err != nil {
			t.Fatalf("missing hook artifact %s: %v", hook, err)
		}
	}
}

func TestLearnCreateListSearchDoctor(t *testing.T) {
	dir := initRepo(t)
	out, err := run(t, dir, "learn", "Always use the query builder", "--scope", "global", "--tags", "db,test", "--legacy-lrn")
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

	if _, err := run(t, dir, "learn", "Prefer CSS variables", "--scope", "global", "--tags", "ui", "--legacy-lrn"); err != nil {
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
	out, err := run(t, dir, "learn", "old UNIQUE_STALE_ZZZ", "--legacy-lrn")
	if err != nil {
		t.Fatal(err)
	}
	oldID := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(out), "Captured "))
	out2, err := run(t, dir, "learn", "new UNIQUE_TIP_YYY", "--supersedes", oldID, "--legacy-lrn")
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

	out, err := run(t, dir, "learn", "race in UNIQUE_CITE_AAA", "--cites", citedRel, "--legacy-lrn")
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

	out, err := run(t, dir, "learn", "old BOTH_FILTER_QQQ", "--cites", citedRel, "--legacy-lrn")
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
	if _, err := run(t, dir, "learn", "old rule about retries", "--tags", "net", "--legacy-lrn"); err != nil {
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

func TestLearnRmDeclined(t *testing.T) {
	dir := initRepo(t)
	if _, err := run(t, dir, "learn", "keep me", "--legacy-lrn"); err != nil {
		t.Fatalf("learn: %v", err)
	}

	prev := confirmDeleteFn
	confirmDeleteFn = func(summary string, in io.Reader, out io.Writer) (bool, error) {
		if !strings.Contains(summary, "LRN-001") {
			t.Fatalf("unexpected summary: %q", summary)
		}
		return false, nil
	}
	t.Cleanup(func() { confirmDeleteFn = prev })

	out, err := run(t, dir, "learn", "rm", "LRN-001")
	if err != nil {
		t.Fatalf("rm declined: %v", err)
	}
	if !strings.Contains(out, "Cancelled.") {
		t.Fatalf("expected Cancelled output, got:\n%s", out)
	}
	// Learning should still exist.
	if _, err := run(t, dir, "learn", "show", "LRN-001"); err != nil {
		t.Fatalf("learning should still exist after declined rm: %v", err)
	}
}

func TestLearnRmAbort(t *testing.T) {
	dir := initRepo(t)
	if _, err := run(t, dir, "learn", "keep me", "--legacy-lrn"); err != nil {
		t.Fatalf("learn: %v", err)
	}

	prev := confirmDeleteFn
	confirmDeleteFn = func(summary string, in io.Reader, out io.Writer) (bool, error) {
		return false, tui.ErrCancelled
	}
	t.Cleanup(func() { confirmDeleteFn = prev })

	out, err := run(t, dir, "learn", "rm", "LRN-001")
	if err != nil {
		t.Fatalf("rm abort: %v", err)
	}
	if !strings.Contains(out, "Cancelled.") {
		t.Fatalf("expected Cancelled output, got:\n%s", out)
	}
}

func TestLearnMemoryAppendAndSuggest(t *testing.T) {
	dir := initRepo(t)
	out, err := run(t, dir, "learn", "Prefer dark UI chrome", "--to", "MEMORY.md")
	if err != nil {
		t.Fatalf("learn MEMORY: %v\n%s", err, out)
	}
	if !strings.Contains(out, "MEMORY.md") {
		t.Fatalf("expected MEMORY append output:\n%s", out)
	}
	mem, err := os.ReadFile(filepath.Join(dir, ".pine", "MEMORY.md"))
	if err != nil || !strings.Contains(string(mem), "Prefer dark UI chrome") {
		t.Fatalf("MEMORY.md missing insight: %v\n%s", err, mem)
	}

	out, err = run(t, dir, "learn", "Usage icons need text-white", "--new-topic", "analytics",
		"--cites", "apps/web/src/modules/analytics/lib/usage.ts")
	if err != nil {
		t.Fatalf("new-topic: %v\n%s", err, out)
	}
	topic := filepath.Join(dir, ".pine", "memory", "analytics.md")
	data, err := os.ReadFile(topic)
	if err != nil || !strings.Contains(string(data), "text-white") {
		t.Fatalf("topic missing: %v\n%s", err, data)
	}

	// Second related insight should auto-append to analytics when cites match.
	out, err = run(t, dir, "learn", "AI Usage Logs type icons colored square never bg-muted",
		"--cites", "apps/web/src/modules/analytics/lib/usage.ts")
	if err != nil {
		t.Fatalf("auto topic: %v\n%s", err, out)
	}
	if !strings.Contains(out, "memory/analytics.md") {
		t.Fatalf("expected auto-append to analytics:\n%s", out)
	}
	data, _ = os.ReadFile(topic)
	if !strings.Contains(string(data), "bg-muted") {
		t.Fatalf("second bullet missing:\n%s", data)
	}

	sug, err := run(t, dir, "learn", "suggest", "usage type icon colors", "--cites", "apps/web/src/modules/analytics/lib/usage.ts", "--json")
	if err != nil {
		t.Fatalf("suggest: %v\n%s", err, sug)
	}
	if !strings.Contains(sug, "memory/analytics.md") {
		t.Fatalf("suggest missing analytics:\n%s", sug)
	}

	show, err := run(t, dir, "learn", "show", "memory/analytics.md")
	if err != nil || !strings.Contains(show, "text-white") {
		t.Fatalf("show topic: %v\n%s", err, show)
	}
}

func TestLearnComponentScopeCLI(t *testing.T) {
	dir := initRepo(t)
	if _, err := run(t, dir, "learn", "prefer atomic writes here", "--scope", "component", "--component", "internal/store", "--legacy-lrn"); err != nil {
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

func TestLearnMemoryAmbiguousAndJSON(t *testing.T) {
	dir := initRepo(t)
	// No topics yet → ambiguous (not confident) for a random insight.
	out, err := run(t, dir, "learn", "completely novel insight about quux widgets")
	if err == nil {
		t.Fatalf("expected ambiguous, got success:\n%s", out)
	}
	if !strings.Contains(out, "Ambiguous destination") {
		t.Fatalf("want recommendations printed:\n%s", out)
	}

	out, err = run(t, dir, "learn", "another novel insight about zorp", "--json")
	if err == nil {
		t.Fatalf("expected ambiguous json error, got:\n%s", out)
	}
	if !strings.Contains(out, "recommendations") || !strings.Contains(out, "ambiguous") {
		t.Fatalf("json ambiguous payload:\n%s", out)
	}

	out, err = run(t, dir, "learn", "Prefer dark chrome", "--to", "MEMORY.md", "--json")
	if err != nil {
		t.Fatalf("to MEMORY json: %v\n%s", err, out)
	}
	if !strings.Contains(out, `"path": "MEMORY.md"`) {
		t.Fatalf("json dest:\n%s", out)
	}

	out, err = run(t, dir, "learn", "topic tip", "--to", "memory/widgets.md", "--json")
	if err != nil {
		t.Fatalf("to topic json: %v\n%s", err, out)
	}
	if !strings.Contains(out, "memory/widgets.md") {
		t.Fatalf("json topic:\n%s", out)
	}

	// bad --to
	if _, err := run(t, dir, "learn", "x", "--to", "foo/bar/baz.md"); err == nil {
		t.Fatal("bad --to should fail")
	}
}

func TestLearnSuggestTextAndShowMemory(t *testing.T) {
	dir := initRepo(t)
	if _, err := run(t, dir, "learn", "Prefer dark chrome", "--to", "MEMORY.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, dir, "learn", "billing tip", "--new-topic", "billing"); err != nil {
		t.Fatal(err)
	}

	sug, err := run(t, dir, "learn", "suggest", "prefer dark chrome")
	if err != nil {
		t.Fatalf("suggest text: %v\n%s", err, sug)
	}
	if !strings.Contains(sug, "MEMORY.md") {
		t.Fatalf("suggest missing MEMORY:\n%s", sug)
	}

	sug, err = run(t, dir, "learn", "suggest", "billing tip about invoices", "--component", "billing", "--json")
	if err != nil {
		t.Fatalf("suggest json: %v\n%s", err, sug)
	}
	if !strings.Contains(sug, "recommendations") {
		t.Fatalf("%s", sug)
	}

	show, err := run(t, dir, "learn", "show", "MEMORY.md")
	if err != nil || !strings.Contains(show, "Prefer dark chrome") {
		t.Fatalf("show MEMORY: %v\n%s", err, show)
	}
	show, err = run(t, dir, "learn", "show", "MEMORY.md", "--json")
	if err != nil || !strings.Contains(show, "Prefer dark chrome") {
		t.Fatalf("show MEMORY json: %v\n%s", err, show)
	}
	show, err = run(t, dir, "learn", "show", "billing")
	if err != nil || !strings.Contains(show, "billing tip") {
		t.Fatalf("show bare slug: %v\n%s", err, show)
	}
	show, err = run(t, dir, "learn", "show", "memory/billing.md", "--json")
	if err != nil || !strings.Contains(show, `"slug"`) {
		t.Fatalf("show topic json: %v\n%s", err, show)
	}

	list, err := run(t, dir, "learn", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(list, "MEMORY.md") || !strings.Contains(list, "memory/billing.md") {
		t.Fatalf("list memory section:\n%s", list)
	}

	// search should include memory docs
	searchOut, err := run(t, dir, "learn", "search", "dark chrome")
	if err != nil {
		t.Fatalf("search: %v\n%s", err, searchOut)
	}
}

func TestLearnShowMissingMemory(t *testing.T) {
	dir := initRepo(t)
	// Ensure layout but empty MEMORY by truncating after Ensure via write empty? Default seed is non-empty.
	// Missing topic file:
	out, err := run(t, dir, "learn", "show", "memory/nope.md")
	if err == nil {
		t.Fatalf("expected missing topic error, got:\n%s", out)
	}
	// Non-memory id should fall through to LRN path
	out, err = run(t, dir, "learn", "show", "LRN-999")
	if err == nil {
		t.Fatalf("expected missing LRN error, got:\n%s", out)
	}
}

func TestSetupAgentYesInstalls(t *testing.T) {
	dir := initRepo(t)
	out, err := run(t, dir, "setup", "agent", "-y")
	if err != nil {
		t.Fatalf("setup agent -y: %v\n%s", err, out)
	}
	for _, p := range []string{"AGENTS.md", "CLAUDE.md", "GEMINI.md"} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Errorf("missing %s after setup: %v", p, err)
		}
	}
}

func TestLearnShowEmptyMEMORYJSON(t *testing.T) {
	dir := initRepo(t)
	mem := filepath.Join(dir, ".pine", "MEMORY.md")
	if err := os.WriteFile(mem, []byte("   \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, dir, "learn", "show", "MEMORY.md", "--json")
	if err == nil {
		t.Fatalf("expected error for empty MEMORY, got:\n%s", out)
	}
	if !strings.Contains(err.Error(), "MEMORY.md not found") && !strings.Contains(out, "MEMORY.md not found") {
		t.Fatalf("err=%v out=%s", err, out)
	}
}

func TestLearnListEmptyMemorySection(t *testing.T) {
	dir := initRepo(t)
	out, err := run(t, dir, "learn", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "MEMORY / topics:") || !strings.Contains(out, "(no topic files yet)") {
		t.Fatalf("expected empty memory section:\n%s", out)
	}
}

func TestLearnComponentRoutesToMemory(t *testing.T) {
	dir := initRepo(t)
	out, err := run(t, dir, "learn", "prefer atomic renames in the store layer",
		"--scope", "component", "--component", "internal/store", "--to", "memory/store.md")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out, "memory/store.md") {
		t.Fatalf("expected memory topic append:\n%s", out)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, ".pine", "learnings"))
	if len(entries) != 0 {
		t.Fatalf("should not create LRN without --legacy-lrn, got %d", len(entries))
	}
	data, err := os.ReadFile(filepath.Join(dir, ".pine", "memory", "store.md"))
	if err != nil || !strings.Contains(string(data), "atomic renames") {
		t.Fatalf("topic missing: %v %s", err, data)
	}
}

func TestLearnSupersedeInheritViaTextFlag(t *testing.T) {
	dir := initRepo(t)
	run(t, dir, "create", "--type", "bug", "--title", "Login")
	if _, err := run(t, dir, "learn", "old ticket insight", "--scope", "ticket", "--ticket", "BUG-001", "--tags", "auth"); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, dir, "learn", "supersede", "LRN-001", "--text", "new ticket insight with inherit")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	show, err := run(t, dir, "learn", "show", "LRN-002", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(show, `"scope": "ticket"`) && !strings.Contains(show, `"scope":"ticket"`) {
		t.Fatalf("should inherit ticket scope:\n%s", show)
	}
	if !strings.Contains(show, "BUG-001") {
		t.Fatalf("should inherit ticket:\n%s", show)
	}
	if !strings.Contains(show, "auth") {
		t.Fatalf("should inherit tags:\n%s", show)
	}
}

func TestLearnTextFlagAndBothArgsError(t *testing.T) {
	dir := initRepo(t)
	if _, err := run(t, dir, "learn", "--text", "via flag only", "--to", "MEMORY.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, dir, "learn"); err == nil {
		t.Fatal("empty learn should fail")
	}
	if _, err := run(t, dir, "learn", "positional", "--text", "also"); err == nil {
		t.Fatal("both text sources should fail")
	}
}

func TestLearnLegacyJSON(t *testing.T) {
	dir := initRepo(t)
	out, err := run(t, dir, "learn", "json lrn", "--legacy-lrn", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "LRN-") || !strings.Contains(out, "json lrn") {
		t.Fatalf("%s", out)
	}
}

func TestLearnSuggestTextConfidentAndEmptyish(t *testing.T) {
	dir := initRepo(t)
	_, _ = run(t, dir, "learn", "Prefer dark mode chrome for dashboards", "--to", "MEMORY.md")
	out, err := run(t, dir, "learn", "suggest", "Prefer dark mode chrome for dashboards")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "MEMORY.md") {
		t.Fatalf("%s", out)
	}
	// show MEMORY without trailing newline still prints
	os.WriteFile(filepath.Join(dir, ".pine", "MEMORY.md"), []byte("no-nl"), 0o644)
	show, err := run(t, dir, "learn", "show", "MEMORY.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(show, "no-nl") {
		t.Fatalf("%s", show)
	}
}

func TestLearnShowBareTopicSlug(t *testing.T) {
	dir := initRepo(t)
	run(t, dir, "learn", "topic body", "--new-topic", "widgets")
	out, err := run(t, dir, "learn", "show", "widgets")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "topic body") {
		t.Fatalf("%s", out)
	}
}

func TestLearnSupersedeJSON(t *testing.T) {
	dir := initRepo(t)
	run(t, dir, "learn", "old rule", "--legacy-lrn")
	out, err := run(t, dir, "learn", "supersede", "LRN-001", "new rule", "--json")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out, "LRN-002") || !strings.Contains(out, "new rule") {
		t.Fatalf("%s", out)
	}
}

func TestCiteStatusesAndScopeTarget(t *testing.T) {
	dir := initRepo(t)
	s, err := store.Open(filepath.Join(dir, ".pine"))
	if err != nil {
		t.Fatal(err)
	}
	if citeStatuses(s, nil) != nil {
		t.Fatal("empty")
	}
	got := citeStatuses(s, []string{"", "nope.go"})
	if len(got) != 1 || got[0].Path != "nope.go" || got[0].Exists {
		t.Fatalf("%+v", got)
	}
	l := &learning.Learning{Scope: learning.ScopeComponent, Component: "internal/x"}
	if scopeTarget(l) != "internal/x" {
		t.Fatalf("%q", scopeTarget(l))
	}
	l = &learning.Learning{Scope: learning.ScopeTicket, Ticket: "BUG-001"}
	if scopeTarget(l) != "BUG-001" {
		t.Fatalf("%q", scopeTarget(l))
	}
}

func TestScopeTargetGlobalEmpty(t *testing.T) {
	l := &learning.Learning{Scope: learning.ScopeGlobal}
	if scopeTarget(l) != "" {
		t.Fatal(scopeTarget(l))
	}
}

func TestLearnSuggestConfidentHint(t *testing.T) {
	dir := initRepo(t)
	run(t, dir, "learn", "UNIQUE_CONFIDENT_MEMORY_PHRASE dark chrome", "--to", "MEMORY.md")
	out, err := run(t, dir, "learn", "suggest", "UNIQUE_CONFIDENT_MEMORY_PHRASE dark chrome")
	if err != nil {
		t.Fatal(err)
	}
	// Either auto-append hint or not-confident message should appear.
	if !strings.Contains(out, "Auto-append") && !strings.Contains(out, "Not confident") && !strings.Contains(out, "MEMORY.md") {
		t.Fatalf("%s", out)
	}
}

func TestLearnShowNewPrefixUnhandled(t *testing.T) {
	dir := initRepo(t)
	// NEW: paths resolve but file missing → error from ReadFile
	_, err := run(t, dir, "learn", "show", "NEW:does-not-exist-yet")
	if err == nil {
		t.Fatal("expected missing topic error")
	}
}

func TestLearnRmMissingAndSupersedeMissingText(t *testing.T) {
	dir := initRepo(t)
	if _, err := run(t, dir, "learn", "rm", "LRN-999", "--yes"); err == nil {
		t.Fatal("rm missing should fail")
	}
	run(t, dir, "learn", "keep", "--legacy-lrn")
	if _, err := run(t, dir, "learn", "supersede", "LRN-001"); err == nil {
		t.Fatal("supersede without text should fail")
	}
	if _, err := run(t, dir, "learn", "supersede", "LRN-999", "text"); err == nil {
		t.Fatal("supersede missing old should fail")
	}
}

func TestLearnListJSONEmpty(t *testing.T) {
	dir := initRepo(t)
	out, err := run(t, dir, "learn", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "[") {
		t.Fatalf("%s", out)
	}
}

func TestLevelNameAndDoctorJSON(t *testing.T) {
	if levelName(doctor.LevelOK) != "ok" {
		t.Fatal(levelName(doctor.LevelOK))
	}
	if levelName(doctor.LevelWarn) != "warn" || levelName(doctor.LevelError) != "error" {
		t.Fatal("warn/error")
	}
	if levelName(doctor.Level(99)) != "ok" {
		t.Fatal("default")
	}
	dir := initRepo(t)
	out, err := run(t, dir, "doctor", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"level"`) {
		t.Fatalf("%s", out)
	}
}

func TestDoctorDryRunFixableCount(t *testing.T) {
	dir := initRepo(t)
	os.WriteFile(filepath.Join(dir, ".pine", "tickets", "notes.txt"), []byte("scratch"), 0o644)
	// Make a fixable dangling dep
	run(t, dir, "create", "--type", "bug", "--title", "x")
	raw, _ := os.ReadFile(filepath.Join(dir, ".pine", "tickets", "BUG-001.md"))
	s := string(raw)
	idx := strings.Index(s, "\n---\n")
	updated := s[:idx+1] + "deps:\n  - GHOST-999\n" + s[idx+1:]
	os.WriteFile(filepath.Join(dir, ".pine", "tickets", "BUG-001.md"), []byte(updated), 0o644)
	out, err := run(t, dir, "doctor", "--dry-run")
	if err != nil {
		// may still be ok if only warnings
		_ = err
	}
	if !strings.Contains(out, "fixable") || !strings.Contains(out, "auto-fixed") {
		t.Fatalf("%s", out)
	}
}
