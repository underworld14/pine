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
