package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=t@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestLogCommand(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")
	gitRun(t, dir, "checkout", "-b", "main")

	if _, err := run(t, dir, "init", "--skip-agents"); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(dir, ".pine", "config.json")
	raw, _ := os.ReadFile(cfgPath)
	os.WriteFile(cfgPath, []byte(strings.ReplaceAll(string(raw), `"idStyle":"hash"`, `"idStyle":"sequential"`)), 0o644)

	if _, err := run(t, dir, "create", "--type", "bug", "--title", "boom"); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-q", "-m", "work on BUG-001")

	out, err := run(t, dir, "log", "BUG-001")
	if err != nil {
		t.Fatalf("log: %v\n%s", err, out)
	}
	if !strings.Contains(out, "work on BUG-001") {
		t.Errorf("log output should list the commit:\n%s", out)
	}

	out, err = run(t, dir, "log", "BUG-001", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "\"subject\"") {
		t.Errorf("json log missing subject:\n%s", out)
	}

	if _, err := run(t, dir, "log", "BUG-777"); err == nil {
		t.Error("log on a nonexistent ticket should error")
	}
}
