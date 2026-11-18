package cli

import (
	"encoding/json"
	"errors"
	"fmt"
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

// TestCloseEvidenceVerify covers the `verify:` frontmatter command: a passing
// command closes the ticket with a Verification section, a failing one refuses
// the close (ticket stays open) unless --force is passed.
func TestCloseEvidenceVerify(t *testing.T) {
	dir := initGitRepoForEvidence(t)
	run(t, dir, "create", "--type", "feature", "--title", "verified", "--verify", "grep -q feature .pine/tickets/FEAT-001.md")

	// Passing command: close succeeds and records the section.
	if _, err := run(t, dir, "close", "--evidence", "FEAT-001"); err != nil {
		t.Fatalf("close --evidence with passing verify: %v", err)
	}
	body, _ := os.ReadFile(filepath.Join(dir, ".pine", "tickets", "FEAT-001.md"))
	got := string(body)
	if !strings.Contains(got, "status: done") || !strings.Contains(got, "## Verification") || !strings.Contains(got, "**passed**") {
		t.Fatalf("expected done ticket with passing Verification section:\n%s", got)
	}
	if !strings.Contains(got, "## Work Evidence") {
		t.Fatalf("work evidence should still be attached:\n%s", got)
	}

	// Failing command: close refused, ticket stays open.
	run(t, dir, "create", "--type", "bug", "--title", "failing verify", "--verify", "exit 3")
	_, err := run(t, dir, "close", "--evidence", "BUG-001")
	if err == nil || !strings.Contains(err.Error(), "verification FAILED (exit 3)") {
		t.Fatalf("expected verification refusal, got %v", err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, ".pine", "tickets", "BUG-001.md"))
	if strings.Contains(string(raw), "status: done") {
		t.Fatalf("ticket must stay open after failed verification:\n%s", raw)
	}

	// --force closes anyway and records the failure.
	if _, err := run(t, dir, "close", "--evidence", "--force", "BUG-001"); err != nil {
		t.Fatalf("close --force: %v", err)
	}
	raw, _ = os.ReadFile(filepath.Join(dir, ".pine", "tickets", "BUG-001.md"))
	if !strings.Contains(string(raw), "status: done") || !strings.Contains(string(raw), "**FAILED**") {
		t.Fatalf("expected done ticket with recorded failure:\n%s", raw)
	}
}

// TestAppendSectionPreservesSiblingSections checks that replacing Work
// Evidence keeps a trailing Verification section (stable section order on
// re-closing a --force ticket).
func TestAppendSectionPreservesSiblingSections(t *testing.T) {
	body := "## Description\n\nnotes\n\n## Work Evidence\n\nold\n\n## Verification\n\nold-ver\n"
	got := appendSection(body, "Work Evidence", "## Work Evidence\n\nnew\n")
	want := "## Description\n\nnotes\n\n## Work Evidence\n\nnew\n\n## Verification\n\nold-ver\n"
	if got != want {
		t.Fatalf("appendSection = %q, want %q", got, want)
	}
	got = appendSection(got, "Verification", "## Verification\n\nnew-ver\n")
	want = "## Description\n\nnotes\n\n## Work Evidence\n\nnew\n\n## Verification\n\nnew-ver\n"
	if got != want {
		t.Fatalf("appendSection = %q, want %q", got, want)
	}
}

// TestTailLinesTrims covers the trim path (output longer than the cap) and the
// short-circuit (output within the cap).
func TestTailLinesTrims(t *testing.T) {
	var sb strings.Builder
	for i := 1; i <= 50; i++ {
		fmt.Fprintf(&sb, "line %d\n", i)
	}
	got := tailLines(sb.String(), 40)
	if !strings.Contains(got, "… (10 earlier lines trimmed)") || !strings.Contains(got, "line 50") || strings.Contains(got, "line 9\n") {
		t.Fatalf("tailLines(50, 40) = %q", got)
	}
	if got := tailLines("short\n", 40); got != "short\n" {
		t.Fatalf("tailLines short-circuit = %q", got)
	}
}

// TestVerifySectionNoteOnNonExitError covers the section() branch where the
// command failed without an exit code (lookup error, signal, timeout): the
// error is rendered as a parenthetical note instead of being swallowed.
func TestVerifySectionNoteOnNonExitError(t *testing.T) {
	r := verifyResult{
		ran:      true,
		err:      &exec.Error{Name: "sh", Err: errors.New("not found")},
		exitCode: -1,
		duration: time.Millisecond,
		output:   "partial output",
	}
	s := r.section()
	if !strings.Contains(s, "**FAILED**") {
		t.Fatalf("expected FAILED status:\n%s", s)
	}
	if !strings.Contains(s, `(exec: "sh": not found)`) {
		t.Fatalf("expected error note:\n%s", s)
	}
	if !strings.Contains(s, "partial output") {
		t.Fatalf("expected output kept:\n%s", s)
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

func ticketBody(t *testing.T, dir, id string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".pine", "tickets", id+".md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func ticketStatus(t *testing.T, dir, id string) string {
	t.Helper()
	out, err := run(t, dir, "show", id, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct{ Status string }
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatal(err)
	}
	return v.Status
}

func codeCommit(t *testing.T, dir, file, msg string) {
	t.Helper()
	writeRepoFile(t, dir, file, "package x\n")
	gitRun(t, dir, "add", file)
	gitRun(t, dir, "commit", "-q", "-m", msg)
}

func TestCreateCommitEvidence(t *testing.T) {
	dir := initGitRepoForEvidence(t)
	codeCommit(t, dir, "code.go", "feat: implement thing")

	out, err := run(t, dir, "create", "--type", "bug", "--title", "recorded",
		"--status", "testing", "--commit", "HEAD")
	if err != nil {
		t.Fatalf("create --commit: %v\n%s", err, out)
	}
	body := ticketBody(t, dir, "BUG-001")
	for _, want := range []string{
		"## Work Evidence",
		"Recorded from commits by `pine create --commit`",
		"- Commits (1):",
		"feat: implement thing",
		"code.go",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("ticket body missing %q:\n%s", want, body)
		}
	}
	// The template sections are still there ahead of the evidence.
	if !strings.Contains(body, "# Acceptance Criteria") {
		t.Errorf("template body lost:\n%s", body)
	}
	if got := ticketStatus(t, dir, "BUG-001"); got != "testing" {
		t.Errorf("status = %q, want testing (never done on create)", got)
	}
}

func TestCreateCommitBadRevCreatesNothing(t *testing.T) {
	dir := initGitRepoForEvidence(t)
	codeCommit(t, dir, "code.go", "feat: implement thing")

	_, err := run(t, dir, "create", "--type", "bug", "--title", "x", "--commit", "HEAD", "--commit", "no-such-commit")
	if err == nil || !strings.Contains(err.Error(), `unknown commit "no-such-commit"`) {
		t.Fatalf("want unknown commit error, got %v", err)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, ".pine", "tickets"))
	if len(entries) != 0 {
		t.Fatalf("a bad rev must create nothing, found %d ticket files", len(entries))
	}
}

func TestCloseKeepsRecordedEvidence(t *testing.T) {
	dir := initGitRepoForEvidence(t)
	codeCommit(t, dir, "code.go", "feat: implement thing")
	if _, err := run(t, dir, "create", "--type", "bug", "--title", "recorded",
		"--commit", "HEAD", "--verify", "echo verified-ok"); err != nil {
		t.Fatal(err)
	}
	// Unrelated work lands afterwards; a creation-anchored diff would swallow it.
	codeCommit(t, dir, "other.go", "feat: something unrelated")

	out, err := run(t, dir, "close", "--evidence", "BUG-001")
	if err != nil {
		t.Fatalf("close --evidence: %v\n%s", err, out)
	}
	if !strings.Contains(out, "kept recorded commit evidence") {
		t.Errorf("output should say the recorded evidence was kept: %s", out)
	}
	body := ticketBody(t, dir, "BUG-001")
	if !strings.Contains(body, "code.go") || strings.Contains(body, "other.go") || strings.Contains(body, "Closed by `pine close --evidence`") {
		t.Errorf("recorded evidence must be kept untouched:\n%s", body)
	}
	if !strings.Contains(body, "## Verification") || !strings.Contains(body, "verified-ok") {
		t.Errorf("verify: must still run and be recorded:\n%s", body)
	}
	if got := ticketStatus(t, dir, "BUG-001"); got != "done" {
		t.Errorf("status = %q, want done", got)
	}
}

func TestCloseEvidenceCommitRerecords(t *testing.T) {
	dir := initGitRepoForEvidence(t)
	if _, err := run(t, dir, "create", "--type", "feature", "--title", "plain"); err != nil {
		t.Fatal(err)
	}
	codeCommit(t, dir, "code.go", "feat: first")
	codeCommit(t, dir, "more.go", "feat: second")

	out, err := run(t, dir, "close", "--evidence", "--commit", "HEAD~1", "FEAT-001")
	if err != nil {
		t.Fatalf("close --evidence --commit: %v\n%s", err, out)
	}
	body := ticketBody(t, dir, "FEAT-001")
	if !strings.Contains(body, "Recorded from commits by `pine close --evidence --commit`") ||
		!strings.Contains(body, "feat: first") || strings.Contains(body, "feat: second") {
		t.Errorf("should record exactly HEAD~1:\n%s", body)
	}
	// Re-running with another commit replaces the section rather than stacking.
	if _, err := run(t, dir, "close", "--evidence", "--commit", "HEAD", "FEAT-001"); err != nil {
		t.Fatal(err)
	}
	body = ticketBody(t, dir, "FEAT-001")
	if strings.Count(body, "## Work Evidence") != 1 || !strings.Contains(body, "feat: second") || strings.Contains(body, "feat: first") {
		t.Errorf("re-record should replace the section:\n%s", body)
	}
}

func TestCloseCommitFlagErrors(t *testing.T) {
	dir := initGitRepoForEvidence(t)
	if _, err := run(t, dir, "create", "--type", "feature", "--title", "plain"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, dir, "close", "--commit", "HEAD", "FEAT-001"); err == nil || !strings.Contains(err.Error(), "--evidence") {
		t.Fatalf("--commit without --evidence should be refused, got %v", err)
	}
	if _, err := run(t, dir, "close", "--evidence", "--commit", "nope", "FEAT-001"); err == nil || !strings.Contains(err.Error(), "unknown commit") {
		t.Fatalf("bad rev should be refused, got %v", err)
	}
	if got := ticketStatus(t, dir, "FEAT-001"); got == "done" {
		t.Fatal("a refused close must leave the ticket open")
	}
}

func TestHasRecordedEvidence(t *testing.T) {
	rec := "## Work Evidence\n\n" + recordedEvidenceMarker + " `pine create --commit` on 2026-01-01.\n"
	for name, tc := range map[string]struct {
		body string
		want bool
	}{
		"recorded":         {"# Notes\n\n" + rec, true},
		"recorded+sibling": {rec + "\n## Verification\n\nok\n", true},
		"auto evidence":    {"## Work Evidence\n\nClosed by `pine close --evidence` on 2026-01-01.\n", false},
		"marker elsewhere": {"## Work Evidence\n\nauto\n\n## Notes\n\n" + recordedEvidenceMarker + " me\n", false},
		"no section":       {"# Notes\n", false},
	} {
		if got := hasRecordedEvidence(tc.body); got != tc.want {
			t.Errorf("%s: hasRecordedEvidence = %v, want %v", name, got, tc.want)
		}
	}
}
