package gitx

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func gitAvailable(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
}

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=t@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestSnapshotOnRealRepo(t *testing.T) {
	gitAvailable(t)
	dir := t.TempDir()
	run(t, dir, "init", "-q")
	run(t, dir, "checkout", "-b", "main")
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello\n"), 0o644)
	run(t, dir, "add", "a.txt")
	run(t, dir, "commit", "-q", "-m", "first commit")

	// Dirty the tree.
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("new\n"), 0o644)

	c := New(dir)
	ctx := context.Background()
	if !c.IsRepo(ctx) {
		t.Fatal("should be a repo")
	}
	s := c.Snapshot(ctx, 10)
	if !s.IsRepo || s.Branch != "main" {
		t.Errorf("branch = %q", s.Branch)
	}
	if !s.Dirty {
		t.Errorf("expected dirty tree")
	}
	if len(s.Commits) != 1 || s.Commits[0].Subject != "first commit" {
		t.Errorf("commits = %+v", s.Commits)
	}
	files := c.Files(ctx)
	if len(files) != 1 || files[0] != "a.txt" {
		t.Errorf("files = %v", files)
	}
}

func TestSnapshotEmptyRepo(t *testing.T) {
	gitAvailable(t)
	dir := t.TempDir()
	run(t, dir, "init", "-q")
	run(t, dir, "checkout", "-b", "main")

	c := New(dir)
	s := c.Snapshot(context.Background(), 10)
	if !s.IsRepo {
		t.Errorf("should still be a repo with no commits")
	}
	if len(s.Commits) != 0 {
		t.Errorf("no commits expected: %+v", s.Commits)
	}
	// Branch should indicate no commits rather than crashing.
	if s.Branch == "" || s.Branch == "(unknown)" {
		t.Errorf("branch = %q", s.Branch)
	}
}

func TestNotARepo(t *testing.T) {
	gitAvailable(t)
	c := New(t.TempDir())
	if c.IsRepo(context.Background()) {
		t.Errorf("temp dir is not a repo")
	}
	s := c.Snapshot(context.Background(), 10)
	if s.IsRepo {
		t.Errorf("snapshot should report not-a-repo")
	}
}

func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	full := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBranchesListTreeAndShow(t *testing.T) {
	gitAvailable(t)
	dir := t.TempDir()
	run(t, dir, "init", "-q")
	run(t, dir, "checkout", "-b", "main")
	writeFile(t, dir, ".pine/tickets/BUG-0a1b2c.md", "on main\n")
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-q", "-m", "main ticket")

	run(t, dir, "checkout", "-q", "-b", "feature")
	writeFile(t, dir, ".pine/tickets/FEAT-3d4e5f.md", "on feature\n")
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-q", "-m", "feature ticket")
	run(t, dir, "checkout", "-q", "main") // feature is now an OFF branch

	c := New(dir)
	ctx := context.Background()

	branches := c.Branches(ctx)
	byName := map[string]Branch{}
	for _, b := range branches {
		byName[b.Name] = b
	}
	if _, ok := byName["main"]; !ok {
		t.Fatalf("main branch missing: %+v", branches)
	}
	feat, ok := byName["feature"]
	if !ok {
		t.Fatalf("feature branch missing: %+v", branches)
	}
	if feat.SHA == "" || feat.CommitDate.IsZero() {
		t.Errorf("feature branch not fully populated: %+v", feat)
	}

	// ls-tree the OFF branch's tickets dir via its pinned SHA (no checkout).
	files := c.ListTreeFiles(ctx, feat.SHA, ".pine/tickets")
	if len(files) != 2 { // both BUG (from main, inherited) and FEAT exist on feature
		t.Errorf("feature tree files = %v", files)
	}
	foundFeat := false
	for _, f := range files {
		if f == ".pine/tickets/FEAT-3d4e5f.md" {
			foundFeat = true
		}
	}
	if !foundFeat {
		t.Errorf("FEAT ticket not found in feature tree: %v", files)
	}

	// show a file that only exists on the off branch.
	content, ok := c.ShowFile(ctx, feat.SHA, ".pine/tickets/FEAT-3d4e5f.md")
	if !ok || string(content) != "on feature\n" {
		t.Errorf("ShowFile feature = %q ok=%v", content, ok)
	}
	// missing file → ok=false.
	if _, ok := c.ShowFile(ctx, feat.SHA, ".pine/tickets/NOPE-000000.md"); ok {
		t.Errorf("ShowFile of missing file should return ok=false")
	}
}

func TestShowFileSHAPinIsImmuneToBranchMovement(t *testing.T) {
	gitAvailable(t)
	dir := t.TempDir()
	run(t, dir, "init", "-q")
	run(t, dir, "checkout", "-b", "main")
	writeFile(t, dir, ".pine/tickets/BUG-0a1b2c.md", "v1\n")
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-q", "-m", "v1")

	c := New(dir)
	ctx := context.Background()
	pinned := c.Branches(ctx)[0].SHA

	// Move the branch forward: change the same file and commit.
	writeFile(t, dir, ".pine/tickets/BUG-0a1b2c.md", "v2\n")
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-q", "-m", "v2")

	// Reading via the PINNED sha still returns the old content.
	content, ok := c.ShowFile(ctx, pinned, ".pine/tickets/BUG-0a1b2c.md")
	if !ok || string(content) != "v1\n" {
		t.Errorf("pinned ShowFile = %q ok=%v, want v1", content, ok)
	}
}

func TestBranchesNotARepo(t *testing.T) {
	gitAvailable(t)
	c := New(t.TempDir())
	if b := c.Branches(context.Background()); len(b) != 0 {
		t.Errorf("Branches on non-repo = %v", b)
	}
	if f := c.ListTreeFiles(context.Background(), "HEAD", "."); len(f) != 0 {
		t.Errorf("ListTreeFiles on non-repo = %v", f)
	}
	if _, ok := c.ShowFile(context.Background(), "HEAD", "x"); ok {
		t.Errorf("ShowFile on non-repo should be ok=false")
	}
}

func TestLogGrepAndPathUnion(t *testing.T) {
	gitAvailable(t)
	dir := t.TempDir()
	run(t, dir, "init", "-q")
	run(t, dir, "checkout", "-b", "main")

	// Commit 1: touches the ticket file (no ID in message).
	writeFile(t, dir, ".pine/tickets/BUG-001.md", "id: BUG-001\n")
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-q", "-m", "create ticket file")

	// Commit 2: mentions the ID in the message but touches an unrelated file.
	writeFile(t, dir, "src/fix.go", "package src\n")
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-q", "-m", "fix logic for BUG-001")

	// Commit 3: unrelated entirely.
	writeFile(t, dir, "README.md", "docs\n")
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-q", "-m", "docs")

	c := New(dir)
	ctx := context.Background()
	got := c.Log(ctx, ".pine/tickets/BUG-001.md", "BUG-001", 30)
	if len(got) != 2 {
		t.Fatalf("expected 2 commits (path + grep union), got %d: %+v", len(got), got)
	}
	// Newest first: the grep-matched fix commit precedes the file-create commit.
	if got[0].Subject != "fix logic for BUG-001" || got[1].Subject != "create ticket file" {
		t.Errorf("order/content = %+v", got)
	}
	// A non-matching query returns nothing.
	if none := c.Log(ctx, ".pine/tickets/FEAT-999.md", "FEAT-999", 30); len(none) != 0 {
		t.Errorf("expected no commits, got %+v", none)
	}
}

func TestLogNotARepo(t *testing.T) {
	gitAvailable(t)
	c := New(t.TempDir())
	if got := c.Log(context.Background(), "x", "y", 10); got != nil {
		t.Errorf("non-repo Log should be nil, got %+v", got)
	}
}

func TestNullClientAndShortHash(t *testing.T) {
	n := nullClient{}
	ctx := context.Background()
	if n.IsRepo(ctx) {
		t.Fatal("null IsRepo")
	}
	s := n.Snapshot(ctx, 5)
	if s.IsRepo || s.Changes == nil || s.Commits == nil {
		t.Fatalf("%+v", s)
	}
	if n.Files(ctx) != nil || n.Branches(ctx) != nil || n.ListTreeFiles(ctx, "x", "y") != nil {
		t.Fatal("null slices")
	}
	if b, ok := n.ShowFile(ctx, "a", "b"); ok || b != nil {
		t.Fatal("ShowFile")
	}
	if n.Log(ctx, "", "", 1) != nil {
		t.Fatal("Log")
	}
	if shortHash("abcd") != "abcd" {
		t.Fatal("short")
	}
	if shortHash("abcdefghij") != "abcdefgh" {
		t.Fatal("truncate")
	}
}

func TestSnapshotNonRepo(t *testing.T) {
	gitAvailable(t)
	dir := t.TempDir()
	c := New(dir)
	s := c.Snapshot(context.Background(), 5)
	if s.IsRepo {
		t.Fatal("non-repo should not report IsRepo")
	}
}

func TestChangesAndCommitsEdgeCases(t *testing.T) {
	gitAvailable(t)
	dir := t.TempDir()
	run(t, dir, "init", "-q")
	run(t, dir, "checkout", "-b", "main")
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("1\n"), 0o644)
	run(t, dir, "add", "a.txt")
	run(t, dir, "commit", "-q", "-m", "c1")
	// Rename-like status: modify + untracked with spaces
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("2\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "new file.txt"), []byte("x\n"), 0o644)
	c := New(dir).(*cli)
	ctx := context.Background()
	changes, dirty, _ := c.changes(ctx)
	if !dirty || len(changes) == 0 {
		t.Fatalf("changes=%v dirty=%v", changes, dirty)
	}
	commits := c.commits(ctx, 0) // limit <=0 path
	if len(commits) == 0 {
		t.Fatal("expected commits")
	}
	if shortHash("") != "" {
		t.Fatal("empty hash")
	}
}


func TestLogWithPathspecDefaultLimit(t *testing.T) {
	gitAvailable(t)
	dir := t.TempDir()
	run(t, dir, "init", "-q")
	run(t, dir, "checkout", "-b", "main")
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("1\n"), 0o644)
	run(t, dir, "add", "a.txt")
	run(t, dir, "commit", "-q", "-m", "only")
	c := New(dir)
	logs := c.Log(context.Background(), "a.txt", "", 0)
	if len(logs) != 1 || logs[0].Subject != "only" {
		t.Fatalf("%+v", logs)
	}
	logs = c.Log(context.Background(), "", "only", 5)
	if len(logs) != 1 {
		t.Fatalf("grep: %+v", logs)
	}
}
