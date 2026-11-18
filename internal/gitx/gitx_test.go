package gitx

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	if n.CommitBefore(ctx, time.Now()) != "" || n.DiffStat(ctx, "HEAD") != "" {
		t.Fatal("CommitBefore/DiffStat")
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

// TestDiffStatAndCommitBefore covers the work-evidence primitives used by
// `pine close --evidence`: CommitBefore anchors to a timestamp, DiffStat
// summarizes tracked changes from a base to the working tree (committed +
// unstaged-tracked; untracked files are not shown by `git diff` and are
// surfaced separately via Snapshot.Changes).
func TestDiffStatAndCommitBefore(t *testing.T) {
	gitAvailable(t)
	dir := t.TempDir()
	run(t, dir, "init", "-q")
	run(t, dir, "checkout", "-b", "main")
	// Use explicit committer dates so the test is deterministic without sleeps:
	// git `--before` is inclusive at second resolution, so we space the commits
	// across distinct seconds.
	commitAt := func(date, msg string) {
		t.Helper()
		cmd := exec.Command("git", "-C", dir, "commit", "-q", "-m", msg)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=t@example.com",
			"GIT_AUTHOR_DATE="+date, "GIT_COMMITTER_DATE="+date)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git commit: %v\n%s", err, out)
		}
	}
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("1\n"), 0o644)
	run(t, dir, "add", "a.txt")
	commitAt("2026-08-05T09:00:00Z", "one")
	// Anchor time strictly between the two commits (both UTC, matching first).
	first, _ := time.Parse(time.RFC3339, "2026-08-05T09:00:30Z")
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("1\n2\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("new\n"), 0o644)
	run(t, dir, "add", "a.txt", "b.txt")
	commitAt("2026-08-05T09:00:31Z", "two")

	c := New(dir)
	ctx := context.Background()
	oneSHA := strings.TrimSpace(func() string {
		out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD~1").Output()
		if err != nil {
			t.Fatalf("rev-parse HEAD~1: %v", err)
		}
		return string(out)
	}())

	// CommitBefore(first) returns the "one" commit (09:00:00 < 09:00:30 < 09:00:31).
	base := c.CommitBefore(ctx, first)
	if base != oneSHA {
		t.Fatalf("CommitBefore(first) = %q, want one %q", base, oneSHA)
	}

	// DiffStat(base) includes the "two" commit's tracked changes (a.txt, b.txt).
	stat := c.DiffStat(ctx, base)
	if stat == "" || !strings.Contains(stat, "a.txt") || !strings.Contains(stat, "b.txt") {
		t.Fatalf("DiffStat(base) missing tracked changes:\n%s", stat)
	}

	// DiffStat("") reports only uncommitted tracked changes (vs HEAD). Modify a
	// tracked file without committing and expect it to appear.
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("1\n2\n3\n"), 0o644)
	uncommitted := c.DiffStat(ctx, "")
	if !strings.Contains(uncommitted, "a.txt") {
		t.Fatalf("DiffStat(\"\") missing uncommitted tracked change:\n%s", uncommitted)
	}

	// CommitBefore(zero) is empty (no anchor for an unknown time).
	if c.CommitBefore(ctx, time.Time{}) != "" {
		t.Error("CommitBefore(zero) should be empty")
	}
	// DiffStat on a non-repo is empty.
	if got := New(t.TempDir()).DiffStat(ctx, ""); got != "" {
		t.Errorf("DiffStat on non-repo = %q, want empty", got)
	}
}

func revParse(t *testing.T, dir, rev string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "rev-parse", rev).Output()
	if err != nil {
		t.Fatalf("rev-parse %s: %v", rev, err)
	}
	return strings.TrimSpace(string(out))
}

func TestResolveCommit(t *testing.T) {
	gitAvailable(t)
	dir := t.TempDir()
	run(t, dir, "init", "-q")
	run(t, dir, "checkout", "-b", "main")
	writeFile(t, dir, "a.txt", "1\n")
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-q", "-m", "first subject")
	run(t, dir, "tag", "v1")
	writeFile(t, dir, "a.txt", "2\n")
	run(t, dir, "commit", "-q", "-am", "second subject")

	c := New(dir)
	ctx := context.Background()
	want := revParse(t, dir, "HEAD~1")

	for _, rev := range []string{"v1", "HEAD~1", want} {
		sha, cm, ok := c.ResolveCommit(ctx, rev)
		if !ok || sha != want || cm.Subject != "first subject" || cm.Author != "Test" || cm.Hash != shortHash(want) {
			t.Errorf("ResolveCommit(%q) = %q %+v %v", rev, sha, cm, ok)
		}
	}
	for _, bad := range []string{"", "nope", "--output=/tmp/x", "-1", "v1\nHEAD"} {
		if _, _, ok := c.ResolveCommit(ctx, bad); ok {
			t.Errorf("ResolveCommit(%q) should fail", bad)
		}
	}
	if _, _, ok := New(t.TempDir()).ResolveCommit(ctx, "HEAD"); ok {
		t.Error("ResolveCommit on a non-repo should fail")
	}
}

func changeMap(cs []Change) map[string]string {
	m := map[string]string{}
	for _, c := range cs {
		m[c.Path] = c.Code
	}
	return m
}

func TestChangedFiles(t *testing.T) {
	gitAvailable(t)
	dir := t.TempDir()
	run(t, dir, "init", "-q")
	run(t, dir, "checkout", "-b", "main")
	writeFile(t, dir, ".gitignore", ".pine/attachments/\n")
	writeFile(t, dir, ".pine/tickets/A.md", "a\n")
	writeFile(t, dir, ".pine/tickets/B.md", "b\n")
	writeFile(t, dir, ".pine/tickets/E.md", "e\n")
	writeFile(t, dir, "src/x.go", "package src\n")
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-q", "-m", "base")
	base := revParse(t, dir, "HEAD")

	// Committed after base.
	writeFile(t, dir, ".pine/tickets/E.md", "e2\n")
	run(t, dir, "commit", "-q", "-am", "edit E")
	// Uncommitted: modify A, delete B, stage D, leave C untracked, ignore an attachment.
	writeFile(t, dir, ".pine/tickets/A.md", "a2\n")
	run(t, dir, "rm", "-q", ".pine/tickets/B.md")
	writeFile(t, dir, ".pine/tickets/D.md", "d\n")
	run(t, dir, "add", ".pine/tickets/D.md")
	writeFile(t, dir, ".pine/tickets/C.md", "c\n")
	writeFile(t, dir, ".pine/attachments/z.bin", "z\n")
	writeFile(t, dir, "src/x.go", "package src // changed\n")

	c := New(dir)
	ctx := context.Background()
	got := changeMap(c.ChangedFiles(ctx, base, ".pine"))
	want := map[string]string{
		".pine/tickets/A.md": "M",
		".pine/tickets/B.md": "D",
		".pine/tickets/C.md": "A",
		".pine/tickets/D.md": "A",
		".pine/tickets/E.md": "M",
	}
	if len(got) != len(want) {
		t.Fatalf("ChangedFiles = %v, want %v", got, want)
	}
	for p, code := range want {
		if got[p] != code {
			t.Errorf("%s = %q, want %q (all: %v)", p, got[p], code, got)
		}
	}
	// Results are sorted by path so callers get deterministic output.
	cs := c.ChangedFiles(ctx, base, ".pine")
	for i := 1; i < len(cs); i++ {
		if cs[i-1].Path > cs[i].Path {
			t.Fatalf("not sorted: %v", cs)
		}
	}
	// Against HEAD only the uncommitted changes remain (E was committed).
	if head := changeMap(c.ChangedFiles(ctx, "HEAD", ".pine")); len(head) != 4 || head[".pine/tickets/E.md"] != "" {
		t.Errorf("ChangedFiles(HEAD) = %v", head)
	}
	if got := c.ChangedFiles(ctx, "no-such-rev", ".pine"); len(got) != 0 {
		t.Errorf("bad rev should yield nothing, got %v", got)
	}
	if got := New(t.TempDir()).ChangedFiles(ctx, "HEAD", ".pine"); len(got) != 0 {
		t.Errorf("non-repo should yield nothing, got %v", got)
	}
}

func TestChangedFilesFromSubdirIsRelative(t *testing.T) {
	gitAvailable(t)
	dir := t.TempDir()
	run(t, dir, "init", "-q")
	run(t, dir, "checkout", "-b", "main")
	writeFile(t, dir, "app/.pine/tickets/A.md", "a\n")
	writeFile(t, dir, "other/.pine/tickets/Z.md", "z\n")
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-q", "-m", "base")
	writeFile(t, dir, "app/.pine/tickets/A.md", "a2\n")
	writeFile(t, dir, "other/.pine/tickets/Z.md", "z2\n")

	got := changeMap(New(filepath.Join(dir, "app")).ChangedFiles(context.Background(), "HEAD", ".pine"))
	if len(got) != 1 || got[".pine/tickets/A.md"] != "M" {
		t.Fatalf("want only app-relative .pine/tickets/A.md, got %v", got)
	}
}

func TestRangeLog(t *testing.T) {
	gitAvailable(t)
	dir := t.TempDir()
	run(t, dir, "init", "-q")
	run(t, dir, "checkout", "-b", "main")
	writeFile(t, dir, "a.txt", "0\n")
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-q", "-m", "base")
	run(t, dir, "tag", "v1")

	writeFile(t, dir, "src/fix.go", "package src\n")
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-q", "-m", "fix thing (FEAT-001)", "-m", "body mentions BUG-002")
	writeFile(t, dir, ".pine/tickets/FEAT-001.md", "t\n")
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-q", "-m", "pine: hand off FEAT-001 for review")
	writeFile(t, dir, "src/more.go", "package src\n")
	writeFile(t, dir, ".pine/tickets/FEAT-001.md", "t2\n")
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-q", "-m", "code plus ticket FEAT-001")

	c := New(dir)
	ctx := context.Background()

	// With .pine excluded, the pine-only commit disappears.
	got := c.RangeLog(ctx, "v1", "HEAD", ".pine")
	if len(got) != 2 {
		t.Fatalf("RangeLog = %+v, want 2 code commits", got)
	}
	if got[0].Subject != "code plus ticket FEAT-001" || got[1].Subject != "fix thing (FEAT-001)" {
		t.Errorf("order/subjects = %q, %q", got[0].Subject, got[1].Subject)
	}
	if !strings.Contains(got[1].Message, "body mentions BUG-002") || !strings.Contains(got[1].Message, "fix thing (FEAT-001)") {
		t.Errorf("Message should hold subject and body, got %q", got[1].Message)
	}
	if got[1].Hash == "" || got[1].Author != "Test" || got[1].When.IsZero() {
		t.Errorf("commit fields not populated: %+v", got[1])
	}
	// Without an exclusion every commit in the range is returned.
	if all := c.RangeLog(ctx, "v1", "HEAD", ""); len(all) != 3 {
		t.Errorf("RangeLog without exclude = %d commits, want 3", len(all))
	}
	if got := c.RangeLog(ctx, "no-such", "HEAD", ".pine"); len(got) != 0 {
		t.Errorf("bad range should yield nothing, got %+v", got)
	}
	if got := New(t.TempDir()).RangeLog(ctx, "a", "b", ""); len(got) != 0 {
		t.Errorf("non-repo should yield nothing, got %+v", got)
	}
}

func TestCommitStat(t *testing.T) {
	gitAvailable(t)
	dir := t.TempDir()
	run(t, dir, "init", "-q")
	run(t, dir, "checkout", "-b", "main")
	writeFile(t, dir, "a.txt", "1\n")
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-q", "-m", "one")
	writeFile(t, dir, "b.txt", "2\n")
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-q", "-m", "two")

	c := New(dir)
	ctx := context.Background()
	got := c.CommitStat(ctx, []string{revParse(t, dir, "HEAD~1"), revParse(t, dir, "HEAD")})
	for _, want := range []string{"one", "a.txt", "two", "b.txt", "file changed"} {
		if !strings.Contains(got, want) {
			t.Errorf("CommitStat missing %q:\n%s", want, got)
		}
	}
	if c.CommitStat(ctx, nil) != "" {
		t.Error("no revs should yield an empty stat")
	}
	for _, bad := range [][]string{{"--output=/tmp/x"}, {"", "HEAD"}, {"no-such-commit"}} {
		if got := c.CommitStat(ctx, bad); got != "" {
			t.Errorf("CommitStat(%q) = %q, want empty", bad, got)
		}
	}
	if New(t.TempDir()).CommitStat(ctx, []string{"HEAD"}) != "" {
		t.Error("non-repo should yield an empty stat")
	}
}

// The diffstat embedded in work evidence is capped: a long-lived repo would
// otherwise write a line per file changed since the ticket was created.
func TestDiffStatIsCapped(t *testing.T) {
	gitAvailable(t)
	dir := t.TempDir()
	run(t, dir, "init", "-q")
	run(t, dir, "checkout", "-b", "main")
	writeFile(t, dir, "seed.txt", "s\n")
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-q", "-m", "seed")
	base := revParse(t, dir, "HEAD")
	for i := 0; i < 80; i++ {
		writeFile(t, dir, filepath.Join("gen", "f"+strings.Repeat("x", i%7)+string(rune('a'+i%26))+string(rune('a'+i/26))+".txt"), "x\n")
	}
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-q", "-m", "many files")

	stat := New(dir).DiffStat(context.Background(), base)
	lines := strings.Split(strings.TrimSpace(stat), "\n")
	if len(lines) > diffStatMaxFiles+2 {
		t.Fatalf("DiffStat not capped: %d lines\n%s", len(lines), stat)
	}
	if !strings.Contains(lines[len(lines)-1], "80 files changed") {
		t.Errorf("summary line must still report the real total, got %q", lines[len(lines)-1])
	}
}

func TestNullClientRangeAndChanged(t *testing.T) {
	n := nullClient{}
	ctx := context.Background()
	if _, _, ok := n.ResolveCommit(ctx, "HEAD"); ok {
		t.Fatal("ResolveCommit")
	}
	if n.ChangedFiles(ctx, "HEAD", ".pine") != nil || n.RangeLog(ctx, "a", "b", "") != nil || n.CommitStat(ctx, []string{"x"}) != "" {
		t.Fatal("null range/changed/stat")
	}
}
