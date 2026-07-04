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
	if _, err := run(t, dir, "init"); err != nil {
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
	} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Errorf("missing %s: %v", p, err)
		}
	}
	// init is idempotent.
	out, err := run(t, dir, "init")
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
	if _, err := run(t, dir, "init"); err != nil { // hash by default
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
