package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func writeTmp(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRunMergeDriverCleanFieldMerge(t *testing.T) {
	dir := t.TempDir()
	base := writeTmp(t, dir, "base", "---\nid: BUG-001\ntitle: T\nstatus: todo\nlabels:\n    - a\ncreated: 2026-07-10T00:00:00Z\nupdated: 2026-07-10T00:00:00Z\n---\nbody\n")
	ours := writeTmp(t, dir, "ours", "---\nid: BUG-001\ntitle: T\nstatus: doing\nlabels:\n    - a\ncreated: 2026-07-10T00:00:00Z\nupdated: 2026-07-11T00:00:00Z\n---\nbody\n")
	theirs := writeTmp(t, dir, "theirs", "---\nid: BUG-001\ntitle: T\nstatus: todo\nlabels:\n    - a\n    - b\ncreated: 2026-07-10T00:00:00Z\nupdated: 2026-07-10T12:00:00Z\n---\nbody\n")

	conflict, err := runMergeDriver(base, ours, theirs, ".pine/tickets/BUG-001.md")
	if err != nil {
		t.Fatal(err)
	}
	if conflict {
		t.Error("clean field merge should not conflict")
	}
	got, _ := os.ReadFile(ours)
	s := string(got)
	if !strings.Contains(s, "status: doing") {
		t.Errorf("status (one-sided) not merged:\n%s", s)
	}
	if !strings.Contains(s, "- b") {
		t.Errorf("label b (union) not merged:\n%s", s)
	}
}

func TestRunMergeDriverDegradedFallback(t *testing.T) {
	dir := t.TempDir()
	base := writeTmp(t, dir, "base", "")
	ours := writeTmp(t, dir, "ours", "not a ticket, no frontmatter\n")
	theirs := writeTmp(t, dir, "theirs", "---\nid: BUG-001\ntitle: T\n---\nbody\n")

	conflict, err := runMergeDriver(base, ours, theirs, ".pine/tickets/BUG-001.md")
	if err != nil {
		t.Fatal(err)
	}
	if !conflict {
		t.Error("a degraded side should force a conflict")
	}
	got, _ := os.ReadFile(ours)
	if !strings.Contains(string(got), "<<<<<<< ours") {
		t.Errorf("expected git-style conflict markers:\n%s", got)
	}
}

func TestSetupMergeInstallsConfig(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")
	if _, err := run(t, dir, "init", "--skip-agents"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, dir, "setup", "merge"); err != nil {
		t.Fatal(err)
	}
	ga, _ := os.ReadFile(filepath.Join(dir, ".gitattributes"))
	if !strings.Contains(string(ga), "merge=pine") {
		t.Errorf(".gitattributes missing rule:\n%s", ga)
	}
	out, _ := exec.Command("git", "-C", dir, "config", "--get", "merge.pine.driver").Output()
	if !strings.Contains(string(out), "pine merge-driver") {
		t.Errorf("driver config = %q", out)
	}
	// Idempotent second run.
	if _, err := run(t, dir, "setup", "merge"); err != nil {
		t.Fatal(err)
	}
	ga2, _ := os.ReadFile(filepath.Join(dir, ".gitattributes"))
	if strings.Count(string(ga2), "merge=pine") != 1 {
		t.Errorf("rule duplicated:\n%s", ga2)
	}
	// Remove strips it.
	if _, err := run(t, dir, "setup", "merge", "--remove"); err != nil {
		t.Fatal(err)
	}
	if out, _ := exec.Command("git", "-C", dir, "config", "--get", "merge.pine.driver").Output(); strings.TrimSpace(string(out)) != "" {
		t.Errorf("driver config should be gone, got %q", out)
	}
}

func TestDoctorWarnsUnconfiguredMergeDriver(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")
	if _, err := run(t, dir, "init", "--skip-agents"); err != nil {
		t.Fatal(err)
	}
	// .gitattributes references the driver, but git config was never set.
	os.WriteFile(filepath.Join(dir, ".gitattributes"), []byte(gitAttributesLine+"\n"), 0o644)
	out, _ := run(t, dir, "doctor")
	if !strings.Contains(out, "run 'pine setup merge'") {
		t.Errorf("doctor should warn about the unconfigured merge driver:\n%s", out)
	}
}
