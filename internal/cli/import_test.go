package cli

import (
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/underworld14/pine/internal/ticket"
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

func TestParseTypeMap(t *testing.T) {
	cases := []struct {
		in   string
		want map[string]string
	}{
		{"", map[string]string{}},
		{",,,", map[string]string{}},
		{"bug", map[string]string{}}, // missing =
		{"bug=BUG", map[string]string{"bug": "BUG"}},
		{" Bug = FEAT , enhancement=EPIC ", map[string]string{"bug": "FEAT", "enhancement": "EPIC"}},
		{"a=b,skip,c=d", map[string]string{"a": "b", "c": "d"}},
	}
	for _, c := range cases {
		got := parseTypeMap(c.in)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("parseTypeMap(%q)=%v want %v", c.in, got, c.want)
		}
	}
}

func TestMapIssueType(t *testing.T) {
	overrides := map[string]string{"docs": "CHORE"}
	if got := mapIssueType([]ghLabel{{Name: "Docs"}}, overrides, "BUG"); got != "CHORE" {
		t.Errorf("override = %q", got)
	}
	if got := mapIssueType([]ghLabel{{Name: "feature"}}, nil, "BUG"); got != "FEAT" {
		t.Errorf("builtin feature = %q", got)
	}
	if got := mapIssueType([]ghLabel{{Name: "epic"}}, nil, "BUG"); got != "EPIC" {
		t.Errorf("builtin epic = %q", got)
	}
	if got := mapIssueType([]ghLabel{{Name: "other"}}, nil, "BUG"); got != "BUG" {
		t.Errorf("fallback = %q", got)
	}
}

func TestTicketGithubURL(t *testing.T) {
	if got := ticketGithubURL(&ticket.Ticket{}); got != "" {
		t.Fatalf("empty Extra: %q", got)
	}
	tk := &ticket.Ticket{Extra: []ticket.ExtraField{
		{Key: "other", Node: &yaml.Node{Value: "x"}},
		{Key: "github", Node: nil},
		{Key: "github", Node: &yaml.Node{Value: "  https://github.com/a/b/issues/1  "}},
	}}
	if got := ticketGithubURL(tk); got != "https://github.com/a/b/issues/1" {
		t.Fatalf("got %q", got)
	}
}

func TestLabelNames(t *testing.T) {
	if labelNames(nil) != nil {
		t.Fatal("nil labels")
	}
	got := labelNames([]ghLabel{{Name: "a"}, {Name: ""}, {Name: "b"}})
	if !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("%v", got)
	}
}
