package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// initGitRepoForEvidence creates a real git repo with an initial commit, runs
// `pine init`, and pins sequential IDs so the test can assert on FEAT-001.
func initGitRepoForEvidence(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	dir := t.TempDir()
	env := append(os.Environ(),
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=t@example.com")
	runGit := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	runGit("init", "-q")
	runGit("checkout", "-b", "main", "-q")
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# repo\n"), 0o644)
	runGit("add", "README.md")
	runGit("commit", "-q", "-m", "init")
	if _, err := run(t, dir, "init", "--skip-agents"); err != nil {
		t.Fatalf("pine init: %v", err)
	}
	cfgPath := filepath.Join(dir, ".pine", "config.json")
	raw, _ := os.ReadFile(cfgPath)
	os.WriteFile(cfgPath, []byte(strings.ReplaceAll(string(raw), `"idStyle":"hash"`, `"idStyle":"sequential"`)), 0o644)
	return dir
}

// TestCloseEvidence verifies `pine close --evidence` marks the ticket done AND
// appends a "## Work Evidence" section listing the commits that reference it
// and the files changed since its creation.
func TestCloseEvidence(t *testing.T) {
	dir := initGitRepoForEvidence(t)
	if _, err := run(t, dir, "create", "--type", "feature", "--title", "with evidence"); err != nil {
		t.Fatal(err)
	}
	// Sleep past git's 1-second date resolution so the work commit lands in a
	// strictly later second than the ticket's `created` (CommitBefore is
	// inclusive at second resolution).
	time.Sleep(1100 * time.Millisecond)

	env := append(os.Environ(),
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=t@example.com")
	os.WriteFile(filepath.Join(dir, "code.go"), []byte("package main\n"), 0o644)
	for _, args := range [][]string{
		{"add", "code.go", ".pine/tickets/FEAT-001.md"},
		{"commit", "-q", "-m", "implement FEAT-001"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	out, err := run(t, dir, "close", "--evidence", "FEAT-001")
	if err != nil {
		t.Fatalf("close --evidence: %v\n%s", err, out)
	}
	if !strings.Contains(out, "with work evidence") {
		t.Fatalf("close output missing evidence marker: %s", out)
	}

	body, _ := os.ReadFile(filepath.Join(dir, ".pine", "tickets", "FEAT-001.md"))
	got := string(body)
	if !strings.Contains(got, "status: done") {
		t.Fatalf("ticket not marked done:\n%s", got)
	}
	if !strings.Contains(got, "## Work Evidence") {
		t.Fatalf("ticket body missing Work Evidence section:\n%s", got)
	}
	if !strings.Contains(got, "implement FEAT-001") {
		t.Fatalf("evidence missing the commit subject:\n%s", got)
	}
	if !strings.Contains(got, "code.go") {
		t.Fatalf("evidence missing code.go in diffstat:\n%s", got)
	}
}

// TestCloseEvidenceReRunReplacesSection verifies a second `pine close --evidence`
// replaces the prior Work Evidence section instead of duplicating it.
func TestCloseEvidenceReRunReplacesSection(t *testing.T) {
	dir := initGitRepoForEvidence(t)
	run(t, dir, "create", "--type", "feature", "--title", "rerun")
	time.Sleep(1100 * time.Millisecond)

	env := append(os.Environ(),
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=t@example.com")
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\n"), 0o644)
	for _, args := range [][]string{
		{"add", "a.go", ".pine/tickets/FEAT-001.md"},
		{"commit", "-q", "-m", "first FEAT-001"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	if _, err := run(t, dir, "close", "--evidence", "FEAT-001"); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(dir, ".pine", "tickets", "FEAT-001.md"))
	if c := strings.Count(string(before), "## Work Evidence"); c != 1 {
		t.Fatalf("expected 1 evidence section, got %d", c)
	}
	// Re-run; the section should be replaced, not duplicated.
	if _, err := run(t, dir, "close", "--evidence", "FEAT-001"); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(dir, ".pine", "tickets", "FEAT-001.md"))
	if c := strings.Count(string(after), "## Work Evidence"); c != 1 {
		t.Fatalf("expected 1 evidence section after re-run, got %d", c)
	}
}

// TestAppendEvidenceBranches covers every appendEvidence path directly: replace
// when a section already exists (with and without preceding content), append to
// a non-empty body, and the empty-body case.
func TestAppendEvidenceBranches(t *testing.T) {
	ev := "## Work Evidence\n\nbody\n"
	cases := []struct {
		name string
		body string
		want string
	}{
		{"empty body", "", ev},
		{"whitespace body", "   \n  \n", ev},
		{"append to body", "## Description\n\nnotes", "## Description\n\nnotes\n\n" + ev},
		{"replace with kept", "## Description\n\nnotes\n\n## Work Evidence\n\nold", "## Description\n\nnotes\n\n" + ev},
		{"replace empty kept", "## Work Evidence\n\nold", ev},
	}
	for _, c := range cases {
		got := appendEvidence(c.body, ev)
		if got != c.want {
			t.Errorf("%s: appendEvidence = %q, want %q", c.name, got, c.want)
		}
	}
}

// TestShortSHA covers both shortSHA branches.
func TestShortSHA(t *testing.T) {
	if got := shortSHA("0123456789abcdef"); got != "01234567" {
		t.Errorf("shortSHA(long) = %q, want 01234567", got)
	}
	if got := shortSHA("short"); got != "short" {
		t.Errorf("shortSHA(short) = %q, want short", got)
	}
	if got := shortSHA(""); got != "" {
		t.Errorf("shortSHA(\"\") = %q, want \"\"", got)
	}
}
