package cli

import (
	"strings"
	"testing"
)

func TestImportGithub(t *testing.T) {
	dir := initRepo(t)
	origList, origRepo := ghListIssues, ghCurrentRepo
	defer func() { ghListIssues, ghCurrentRepo = origList, origRepo }()

	ghCurrentRepo = func(string) (string, error) { return "acme/widget", nil }
	ghListIssues = func(repo, state string, limit int) ([]ghIssue, error) {
		if repo != "acme/widget" {
			t.Errorf("repo = %q, want acme/widget (from ghCurrentRepo)", repo)
		}
		return []ghIssue{
			{Number: 1, Title: "crash", Body: "trace", URL: "https://github.com/acme/widget/issues/1", Labels: []ghLabel{{Name: "bug"}}},
			{Number: 2, Title: "dark mode", URL: "https://github.com/acme/widget/issues/2", Labels: []ghLabel{{Name: "enhancement"}}},
			{Number: 3, Title: "misc", URL: "https://github.com/acme/widget/issues/3"},
		}, nil
	}

	out, err := run(t, dir, "import", "github")
	if err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	// bug→BUG-001, enhancement→FEAT-001, unlabeled→default type (BUG)-002.
	for _, want := range []string{"BUG-001 ← #1", "FEAT-001 ← #2", "BUG-002 ← #3", "imported 3"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}

	// Re-running is idempotent: every issue is already imported.
	out, _ = run(t, dir, "import", "github")
	if !strings.Contains(out, "imported 0, skipped 3") {
		t.Errorf("re-run should skip all: %q", out)
	}
}

func TestImportGithubDryRunAndLabel(t *testing.T) {
	dir := initRepo(t)
	origList := ghListIssues
	defer func() { ghListIssues = origList }()

	ghListIssues = func(repo, state string, limit int) ([]ghIssue, error) {
		return []ghIssue{
			{Number: 1, Title: "a", URL: "u1", Labels: []ghLabel{{Name: "bug"}}},
			{Number: 2, Title: "b", URL: "u2", Labels: []ghLabel{{Name: "docs"}}},
		}, nil
	}

	out, _ := run(t, dir, "import", "github", "acme/widget", "--dry-run")
	if !strings.Contains(out, "would import 2") {
		t.Errorf("dry-run summary = %q", out)
	}
	// dry-run persisted nothing, so a --label bug import is fresh (skipped 0).
	out, _ = run(t, dir, "import", "github", "acme/widget", "--label", "bug")
	if !strings.Contains(out, "imported 1, skipped 0") {
		t.Errorf("label filter summary = %q", out)
	}
}
