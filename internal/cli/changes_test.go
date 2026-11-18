package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return string(out)
}

// commitAll stages everything and commits it, so a test starts from a clean tree.
func commitAll(t *testing.T, dir, msg string) {
	t.Helper()
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-q", "--allow-empty", "-m", msg)
}

func writeRepoFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	full := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fileSHA(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func changedJSON(t *testing.T, dir string, args ...string) []pineChange {
	t.Helper()
	out, err := run(t, dir, append([]string{"changed", "--json"}, args...)...)
	if err != nil {
		t.Fatalf("changed --json: %v\n%s", err, out)
	}
	var got []pineChange
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("changed --json is not JSON: %v\n%s", err, out)
	}
	return got
}

func TestChangedListsUntrackedAndEditedTickets(t *testing.T) {
	dir := initGitRepoForEvidence(t)
	commitAll(t, dir, "pine init")

	if got := changedJSON(t, dir); len(got) != 0 {
		t.Fatalf("clean tree should report nothing, got %+v", got)
	}

	// An owner's untracked ticket is reported (not a failure by itself).
	if _, err := run(t, dir, "create", "--type", "feature", "--title", "alpha"); err != nil {
		t.Fatal(err)
	}
	got := changedJSON(t, dir)
	if len(got) != 1 {
		t.Fatalf("want 1 change, got %+v", got)
	}
	c := got[0]
	if c.Path != ".pine/tickets/FEAT-001.md" || c.Change != "added" || c.ID != "FEAT-001" ||
		c.Title != "alpha" || c.Status != "todo" || c.StatusBefore != "" {
		t.Fatalf("untracked ticket = %+v", c)
	}
	if want := fileSHA(t, filepath.Join(dir, c.Path)); c.Hash != want {
		t.Fatalf("hash = %s, want sha256 of the file %s", c.Hash, want)
	}

	// The reported hash is the one `pine show --json` exposes for If-Match.
	showOut, err := run(t, dir, "show", "FEAT-001", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var shown struct{ Hash string }
	if err := json.Unmarshal([]byte(showOut), &shown); err != nil || shown.Hash != c.Hash {
		t.Fatalf("show hash %q != changed hash %q (%v)", shown.Hash, c.Hash, err)
	}

	// Once committed it is quiet again; a status move shows before → after.
	commitAll(t, dir, "add alpha")
	if got := changedJSON(t, dir); len(got) != 0 {
		t.Fatalf("after commit want none, got %+v", got)
	}
	if _, err := run(t, dir, "update", "FEAT-001", "--status", "doing"); err != nil {
		t.Fatal(err)
	}
	got = changedJSON(t, dir)
	if len(got) != 1 || got[0].Change != "modified" || got[0].StatusBefore != "todo" || got[0].Status != "doing" {
		t.Fatalf("status move = %+v", got)
	}
}

// The gate recipe: snapshot `pine changed --json` before delegating, compare
// after. Pre-existing owner changes are in the snapshot, so only edits made
// during the part differ.
func TestChangedSnapshotRecipe(t *testing.T) {
	dir := initGitRepoForEvidence(t)
	commitAll(t, dir, "pine init")
	if _, err := run(t, dir, "create", "--type", "feature", "--title", "owner draft"); err != nil {
		t.Fatal(err)
	}
	snap, err := run(t, dir, "changed", "--json")
	if err != nil {
		t.Fatal(err)
	}
	again, _ := run(t, dir, "changed", "--json")
	if snap != again {
		t.Fatalf("output must be byte-stable when nothing changed:\n%s\n---\n%s", snap, again)
	}

	if _, err := run(t, dir, "update", "FEAT-001", "--status", "doing"); err != nil {
		t.Fatal(err)
	}
	after, _ := run(t, dir, "changed", "--json")
	if after == snap {
		t.Fatal("editing a ticket during the part must change the output")
	}
}

func TestChangedSinceRefAndNonTicketFiles(t *testing.T) {
	dir := initGitRepoForEvidence(t)
	commitAll(t, dir, "pine init")
	base := strings.TrimSpace(gitOut(t, dir, "rev-parse", "HEAD"))

	if _, err := run(t, dir, "create", "--type", "bug", "--title", "committed later"); err != nil {
		t.Fatal(err)
	}
	commitAll(t, dir, "ticket")
	writeRepoFile(t, dir, ".pine/MEMORY.md", "# memory\n")

	got := changedJSON(t, dir, "--since", base)
	byPath := map[string]pineChange{}
	for _, c := range got {
		byPath[c.Path] = c
	}
	tk, mem := byPath[".pine/tickets/BUG-001.md"], byPath[".pine/MEMORY.md"]
	if tk.Change != "added" || tk.ID != "BUG-001" || tk.StatusBefore != "" {
		t.Fatalf("committed-since ticket = %+v (all %+v)", tk, got)
	}
	if mem.Path == "" || mem.ID != "" || mem.Title != "" || mem.Hash == "" {
		t.Fatalf("non-ticket file should list path+hash only, got %+v", mem)
	}

	// Deleted tickets report the old status and no hash.
	if err := os.Remove(filepath.Join(dir, ".pine/tickets/BUG-001.md")); err != nil {
		t.Fatal(err)
	}
	for _, c := range changedJSON(t, dir) {
		if c.ID == "BUG-001" && (c.Change != "deleted" || c.Hash != "" || c.StatusBefore != "todo" || c.Status != "") {
			t.Fatalf("deleted ticket = %+v", c)
		}
	}
}

func TestChangedTextAndErrors(t *testing.T) {
	dir := initGitRepoForEvidence(t)
	commitAll(t, dir, "pine init")

	out, err := run(t, dir, "changed")
	if err != nil || !strings.Contains(out, "No changes under .pine/") {
		t.Fatalf("clean text = %q (%v)", out, err)
	}
	if _, err := run(t, dir, "create", "--type", "feature", "--title", "alpha"); err != nil {
		t.Fatal(err)
	}
	commitAll(t, dir, "alpha")
	if _, err := run(t, dir, "update", "FEAT-001", "--status", "doing"); err != nil {
		t.Fatal(err)
	}
	out, _ = run(t, dir, "changed")
	for _, want := range []string{"M", ".pine/tickets/FEAT-001.md", "FEAT-001", "todo → doing", "alpha"} {
		if !strings.Contains(out, want) {
			t.Errorf("text output missing %q:\n%s", want, out)
		}
	}

	// Every row shape: untouched-status edit, new ticket, deleted ticket, plain file.
	if _, err := run(t, dir, "create", "--type", "bug", "--title", "fresh"); err != nil {
		t.Fatal(err)
	}
	writeRepoFile(t, dir, ".pine/memory/topic.md", "# topic\n")
	if _, err := run(t, dir, "update", "FEAT-001", "--title", "alpha renamed"); err != nil {
		t.Fatal(err)
	}
	out, _ = run(t, dir, "changed")
	for _, want := range []string{"new · todo", "fresh", "A  .pine/memory/topic.md"} {
		if !strings.Contains(out, want) {
			t.Errorf("text output missing %q:\n%s", want, out)
		}
	}
	commitAll(t, dir, "more")
	if err := os.Remove(filepath.Join(dir, ".pine/tickets/BUG-001.md")); err != nil {
		t.Fatal(err)
	}
	if out, _ = run(t, dir, "changed"); !strings.Contains(out, "was todo") {
		t.Errorf("deleted ticket should read 'was todo':\n%s", out)
	}

	if _, err := run(t, dir, "changed", "--since", "no-such-ref"); err == nil || !strings.Contains(err.Error(), "unknown ref") {
		t.Fatalf("bad ref should error with 'unknown ref', got %v", err)
	}
	plain := t.TempDir()
	if _, err := run(t, plain, "init", "--skip-agents"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, plain, "changed"); err == nil || !strings.Contains(err.Error(), "git") {
		t.Fatalf("non-repo should explain it needs git, got %v", err)
	}
}

// setupReleaseRepo builds the history release-notes is judged on: a tag, then
// code commits that cite tickets, a pine-only bookkeeping commit, a commit that
// cites a longer ID sharing a prefix, and tickets closed in the working tree.
func setupReleaseRepo(t *testing.T) string {
	t.Helper()
	dir := initGitRepoForEvidence(t)
	// Already done at the tag: must not appear in the notes.
	if _, err := run(t, dir, "create", "--type", "feature", "--title", "shipped before", "--status", "done"); err != nil {
		t.Fatal(err)
	}
	commitAll(t, dir, "base")
	gitRun(t, dir, "tag", "v1")

	for _, args := range [][]string{
		{"create", "--type", "feature", "--title", "closed feature", "-l", "ui"},
		{"create", "--type", "bug", "--title", "closed bug"},
		{"create", "--type", "feature", "--title", "still open"},
	} {
		if _, err := run(t, dir, args...); err != nil {
			t.Fatal(err)
		}
	}
	writeRepoFile(t, dir, "code1.go", "package x\n")
	commitAll(t, dir, "implement FEAT-002 widget")

	// Bookkeeping commit touching only .pine/: never a release-note link.
	if _, err := run(t, dir, "update", "FEAT-002", "--status", "testing"); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", ".pine")
	gitRun(t, dir, "commit", "-q", "-m", "pine: hand off FEAT-002 for review")

	writeRepoFile(t, dir, "code2.go", "package x\n")
	gitRun(t, dir, "add", "code2.go")
	gitRun(t, dir, "commit", "-q", "-m", "fix BUG-001 crash", "-m", "unrelated: FEAT-0020")

	// Created already done after the tag, with a higher priority.
	if _, err := run(t, dir, "create", "--type", "feature", "--title", "born done", "-p", "high", "--status", "done"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, dir, "close", "FEAT-002", "BUG-001"); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestReleaseNotesMarkdown(t *testing.T) {
	dir := setupReleaseRepo(t)
	out, err := run(t, dir, "release-notes", "--since", "v1")
	if err != nil {
		t.Fatalf("release-notes: %v\n%s", err, out)
	}
	for _, want := range []string{
		"## Bugs", "- closed bug (BUG-001) · `",
		"## Features", "- closed feature (FEAT-002) · `", "- born done (FEAT-004)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("notes missing %q:\n%s", want, out)
		}
	}
	for _, bad := range []string{"still open", "shipped before", "pine: hand off", "FEAT-001"} {
		if strings.Contains(out, bad) {
			t.Errorf("notes must not contain %q:\n%s", bad, out)
		}
	}
	// Types follow config order (Bug, Feature, Epic); within a group the
	// higher-priority ticket comes first.
	if strings.Index(out, "## Bugs") > strings.Index(out, "## Features") {
		t.Errorf("groups out of config order:\n%s", out)
	}
	if strings.Index(out, "born done") > strings.Index(out, "closed feature") {
		t.Errorf("high priority should sort first:\n%s", out)
	}
	// Only the code commit that cites the ticket is linked: the pine-only
	// commit is skipped, and FEAT-0020 does not match FEAT-002.
	featLine := lineWith(out, "closed feature")
	if strings.Count(featLine, "`") != 2 {
		t.Errorf("closed feature should link exactly one commit: %q", featLine)
	}
}

func lineWith(s, sub string) string {
	for _, l := range strings.Split(s, "\n") {
		if strings.Contains(l, sub) {
			return l
		}
	}
	return ""
}

func TestReleaseNotesJSONAndGroupByLabel(t *testing.T) {
	dir := setupReleaseRepo(t)
	out, err := run(t, dir, "release-notes", "--since", "v1", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var items []struct {
		ID      string `json:"id"`
		Type    string `json:"type"`
		Title   string `json:"title"`
		Commits []struct {
			Hash    string `json:"hash"`
			Subject string `json:"subject"`
		} `json:"commits"`
	}
	if err := json.Unmarshal([]byte(out), &items); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	byID := map[string]int{}
	for i, it := range items {
		byID[it.ID] = i
	}
	if len(items) != 3 {
		t.Fatalf("want 3 closed tickets (BUG-001, FEAT-002, FEAT-004), got %+v", items)
	}
	f := items[byID["FEAT-002"]]
	if f.Type != "FEAT" || len(f.Commits) != 1 || f.Commits[0].Subject != "implement FEAT-002 widget" {
		t.Fatalf("FEAT-002 = %+v", f)
	}
	if b := items[byID["BUG-001"]]; len(b.Commits) != 1 || b.Commits[0].Subject != "fix BUG-001 crash" {
		t.Fatalf("BUG-001 = %+v", b)
	}
	if n := len(items[byID["FEAT-004"]].Commits); n != 0 {
		t.Fatalf("born-done ticket has no citing commit, got %d", n)
	}

	lab, err := run(t, dir, "release-notes", "--since", "v1", "--group-by", "label")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(lab, "## ui") || !strings.Contains(lab, "## Other") || strings.Index(lab, "## ui") > strings.Index(lab, "## Other") {
		t.Errorf("label grouping should list ui then Other:\n%s", lab)
	}
}

func TestReleaseNotesEmptyAndErrors(t *testing.T) {
	dir := setupReleaseRepo(t)
	commitAll(t, dir, "everything")

	out, err := run(t, dir, "release-notes", "--since", "HEAD")
	if err != nil || !strings.Contains(out, "No tickets closed since HEAD.") {
		t.Fatalf("empty text = %q (%v)", out, err)
	}
	if out, _ := run(t, dir, "release-notes", "--since", "HEAD", "--json"); strings.TrimSpace(out) != "[]" {
		t.Fatalf("empty JSON = %q, want []", out)
	}
	if _, err := run(t, dir, "release-notes", "--since", "no-such-ref"); err == nil || !strings.Contains(err.Error(), "unknown ref") {
		t.Fatalf("bad ref: %v", err)
	}
	if _, err := run(t, dir, "release-notes", "--since", "v1", "--group-by", "epic"); err == nil || !strings.Contains(err.Error(), "--group-by") {
		t.Fatalf("bad group-by: %v", err)
	}
	if _, err := run(t, dir, "release-notes"); err == nil {
		t.Fatal("--since is required")
	}

	cfgPath := filepath.Join(dir, ".pine", "config.json")
	raw, _ := os.ReadFile(cfgPath)
	off := strings.Replace(string(raw), `"tickets":true`, `"tickets":false`, 1)
	if off == string(raw) {
		t.Fatalf("test setup: sync.tickets not found in %s", raw)
	}
	os.WriteFile(cfgPath, []byte(off), 0o644)
	if _, err := run(t, dir, "release-notes", "--since", "v1"); err == nil || !strings.Contains(err.Error(), "not tracked") {
		t.Fatalf("sync.tickets=false should be refused, got %v", err)
	}
}

func TestPluralAndMentionedIDs(t *testing.T) {
	for in, want := range map[string]string{"Bug": "Bugs", "Feature": "Features", "Story": "Stories", "Key": "Keys", "Class": "Classes", "Epic": "Epics"} {
		if got := plural(in); got != want {
			t.Errorf("plural(%q) = %q, want %q", in, got, want)
		}
	}
	got := mentionedIDs("Fix (FEAT-001) and feat-0020, see BUG-7f3k2a.\n\nRefs FEAT-001-extra")
	want := map[string]bool{"feat-001": true, "feat-0020": true, "bug-7f3k2a": true}
	if len(got) != len(want) {
		t.Fatalf("mentionedIDs = %v", got)
	}
	for _, id := range got {
		if !want[id] {
			t.Errorf("unexpected id %q in %v", id, got)
		}
	}
}
