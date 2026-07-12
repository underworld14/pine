package contextgen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/underworld14/pine/internal/config"
	"github.com/underworld14/pine/internal/gitx"
	"github.com/underworld14/pine/internal/learning"
	"github.com/underworld14/pine/internal/store"
	"github.com/underworld14/pine/internal/ticket"
)

func scaffold(t *testing.T) *store.Store {
	t.Helper()
	pine := filepath.Join(t.TempDir(), ".pine")
	if err := os.MkdirAll(filepath.Join(pine, "tickets"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default("my-app")
	cfg.IDStyle = "sequential"
	cfgB, _ := cfg.Bytes()
	os.WriteFile(filepath.Join(pine, "config.json"), cfgB, 0o644)
	bB, _ := config.DefaultBoard().Bytes()
	os.WriteFile(filepath.Join(pine, "board.json"), bB, 0o644)
	s, err := store.Open(pine)
	if err != nil {
		t.Fatal(err)
	}
	s.SetClock(func() time.Time { return time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC) })
	return s
}

func fakeGit() gitx.Status {
	return gitx.Status{
		IsRepo: true,
		Branch: "main",
		Dirty:  true,
		Changes: []gitx.Change{
			{Path: "src/login.tsx", Code: "M"},
		},
		Commits: []gitx.Commit{
			{Hash: "abc1234", Subject: "fix: guard null session", Author: "Izza",
				When: time.Date(2026, 7, 3, 9, 0, 0, 0, time.UTC)},
		},
	}
}

func TestContextContainsKeySections(t *testing.T) {
	s := scaffold(t)
	s.Create(store.CreateReq{Type: "bug", Title: "Login broken", Priority: "critical",
		Body: "# Description\n\nThe login button is dead.\n\n# Related Files\n- src/login.tsx\n"})
	s.Create(store.CreateReq{Type: "epic", Title: "Auth"})
	s.Create(store.CreateReq{Type: "feature", Title: "Child", Parent: "EPIC-001"})

	now := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	md := Context(s, fakeGit(), now)

	for _, want := range []string{
		"# Project Context: my-app",
		"## Repository",
		"Branch: `main`",
		"## Recent Commits",
		"fix: guard null session",
		"## Critical & High Priority (open)",
		"BUG-001",
		"src/login.tsx",
		"## Epics",
		"EPIC-001",
		"## Conventions",
		"pine ready",
		"deps",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("context missing %q\n---\n%s", want, md)
		}
	}
}

func TestPromptRendersDefault(t *testing.T) {
	s := scaffold(t)
	s.Create(store.CreateReq{Type: "bug", Title: "Login broken", Priority: "high",
		Labels: []string{"login", "ui"},
		Body:   "# Description\n\nDead button.\n\n# Related Files\n- src/login.tsx\n"})

	out, err := Prompt(s, fakeGit(), "BUG-001", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"# Fix Request: BUG-001 — Login broken",
		"Priority: high",
		"login, ui",
		"src/login.tsx",
		"When done",
		"status: testing",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("prompt missing %q\n---\n%s", want, out)
		}
	}
}

func TestPromptBadTemplateFallsBack(t *testing.T) {
	s := scaffold(t)
	s.Create(store.CreateReq{Type: "bug", Title: "X"})
	// A template referencing an unknown field must not fail the command.
	out, err := Prompt(s, fakeGit(), "BUG-001", "{{.Nonexistent.Field}}")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "# Fix Request: BUG-001") {
		t.Errorf("expected fallback to default template:\n%s", out)
	}
}

func TestContextIncludesAcceptance(t *testing.T) {
	s := scaffold(t)
	s.Create(store.CreateReq{Type: "bug", Title: "AC critical", Priority: "critical",
		Body: "# Acceptance Criteria\n- [x] a\n- [ ] b\n"})
	s.Create(store.CreateReq{Type: "feature", Title: "AC medium", Priority: "medium",
		Body: "# Acceptance Criteria\n- [ ] only one\n"})

	now := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	md := Context(s, fakeGit(), now)
	if !strings.Contains(md, "Acceptance Criteria") || !strings.Contains(md, "1/2") {
		t.Errorf("context missing critical acceptance progress:\n%s", md)
	}
	if !strings.Contains(md, "## Acceptance Criteria Progress") || !strings.Contains(md, "FEAT-001") || !strings.Contains(md, "0/1") {
		t.Errorf("context missing medium-priority acceptance progress:\n%s", md)
	}
}

func TestExportGroupsByColumn(t *testing.T) {
	s := scaffold(t)
	s.Create(store.CreateReq{Type: "bug", Title: "Todo bug"})
	md := ExportMarkdown(s)
	if !strings.Contains(md, "## Todo") || !strings.Contains(md, "Todo bug") {
		t.Errorf("export missing Todo group:\n%s", md)
	}
}

func TestContextIncludesLearnings(t *testing.T) {
	s := scaffold(t)
	if _, err := s.CreateLearning(store.CreateLearningReq{
		Text: "Always use the query builder", Tags: []string{"db"},
	}); err != nil {
		t.Fatal(err)
	}
	md := Context(s, fakeGit(), time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC))
	if !strings.Contains(md, "## Relevant Learnings") {
		t.Fatalf("missing learnings section:\n%s", md)
	}
	if !strings.Contains(md, "query builder") {
		t.Fatalf("missing learning body:\n%s", md)
	}
}

func TestContextOmitsCitationStaleLearnings(t *testing.T) {
	s := scaffold(t)
	repoRoot := filepath.Dir(s.Root())
	cited := filepath.Join(repoRoot, "internal", "stale.go")
	if err := os.MkdirAll(filepath.Dir(cited), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cited, []byte("package stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateLearning(store.CreateLearningReq{
		Text:  "UNIQUE_CITE_STALE_PHRASE about retry",
		Cites: []string{"internal/stale.go"},
	}); err != nil {
		t.Fatal(err)
	}
	md := Context(s, fakeGit(), time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC))
	if !strings.Contains(md, "UNIQUE_CITE_STALE_PHRASE") {
		t.Fatalf("valid cite should appear:\n%s", md)
	}
	if err := os.Remove(cited); err != nil {
		t.Fatal(err)
	}
	md2 := Context(s, fakeGit(), time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC))
	if strings.Contains(md2, "UNIQUE_CITE_STALE_PHRASE") {
		t.Fatalf("citation-stale learning must not appear in context:\n%s", md2)
	}
}

func TestContextOmitsLearningsWhenEmpty(t *testing.T) {
	s := scaffold(t)
	md := Context(s, fakeGit(), time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC))
	if strings.Contains(md, "## Relevant Learnings") {
		t.Fatalf("empty learnings should omit section:\n%s", md)
	}
}

func TestPromptIncludesTicketLearnings(t *testing.T) {
	s := scaffold(t)
	tk, err := s.Create(store.CreateReq{Type: "bug", Title: "Login broken", Labels: []string{"login"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateLearning(store.CreateLearningReq{
		Text: "Never hardcode session cookies", Scope: "ticket", Ticket: tk.ID, Tags: []string{"login"},
	}); err != nil {
		t.Fatal(err)
	}
	out, err := Prompt(s, fakeGit(), tk.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Relevant Learnings") || !strings.Contains(out, "session cookies") {
		t.Fatalf("prompt missing learnings:\n%s", out)
	}
}

func TestContextResolvesSupersedeTip(t *testing.T) {
	s := scaffold(t)
	a, err := s.CreateLearning(store.CreateLearningReq{Text: "stale rule UNIQUE_STALE_PHRASE"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateLearning(store.CreateLearningReq{Text: "mid rule", Supersedes: a.ID})
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.CreateLearning(store.CreateLearningReq{Text: "current tip UNIQUE_TIP_PHRASE", Supersedes: b.ID})
	if err != nil {
		t.Fatal(err)
	}
	md := Context(s, fakeGit(), time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC))
	if !strings.Contains(md, "UNIQUE_TIP_PHRASE") || !strings.Contains(md, c.ID) {
		t.Fatalf("context should include tip C (%s):\n%s", c.ID, md)
	}
	if strings.Contains(md, "UNIQUE_STALE_PHRASE") || strings.Contains(md, a.ID) {
		t.Fatalf("context must not include superseded A:\n%s", md)
	}
	if strings.Contains(md, "mid rule") || strings.Contains(md, b.ID) {
		t.Fatalf("context must not include mid-chain B:\n%s", md)
	}
}

func TestPromptInjectsLearningsWhenStaleTemplate(t *testing.T) {
	s := scaffold(t)
	tk, err := s.Create(store.CreateReq{Type: "bug", Title: "Login broken"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateLearning(store.CreateLearningReq{
		Text: "Stale-template insight", Scope: "ticket", Ticket: tk.ID,
	}); err != nil {
		t.Fatal(err)
	}
	// Pre-learnings fix.md (no .Learnings field) — matches upgraded workspaces.
	stale := "# Fix Request: {{.ID}} — {{.Title}}\n\n{{.Body}}\n\n## Acceptance Criteria\n- done\n"
	out, err := Prompt(s, fakeGit(), tk.ID, stale)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "## Relevant Learnings") || !strings.Contains(out, "Stale-template insight") {
		t.Fatalf("stale template should still get injected learnings:\n%s", out)
	}
}

func TestPromptInjectIgnoresBodyHeadingCollision(t *testing.T) {
	s := scaffold(t)
	tk, err := s.Create(store.CreateReq{
		Type:  "bug",
		Title: "Heading trap",
		Body:  "# Description\n\nSee ## Relevant Learnings and ## Acceptance Criteria in docs.\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateLearning(store.CreateLearningReq{
		Text: "Real injected insight XYZ", Scope: "ticket", Ticket: tk.ID,
	}); err != nil {
		t.Fatal(err)
	}
	stale := "# Fix {{.ID}}\n\n{{.Body}}\n\n## Acceptance Criteria\n- done\n"
	out, err := Prompt(s, fakeGit(), tk.ID, stale)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Real injected insight XYZ") {
		t.Fatalf("body headings must not suppress learnings inject:\n%s", out)
	}
	// Learnings block should be after the rendered body mention, as a real section.
	bodyIdx := strings.Index(out, "See ## Relevant Learnings")
	blockIdx := strings.LastIndex(out, "## Relevant Learnings")
	if bodyIdx < 0 || blockIdx <= bodyIdx {
		t.Fatalf("expected injected section after body mention:\n%s", out)
	}
}

func TestPromptCommentMentioningLearningsDoesNotSuppressInjection(t *testing.T) {
	s := scaffold(t)
	tk, err := s.Create(store.CreateReq{Type: "bug", Title: "Comment trap"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateLearning(store.CreateLearningReq{
		Text: "UNIQUE_COMMENT_TRAP_INSIGHT", Scope: "ticket", Ticket: tk.ID,
	}); err != nil {
		t.Fatal(err)
	}
	// A template with a comment mentioning .Learnings but never actually
	// rendering it — the old naive substring check would wrongly treat this
	// as "already handled" and skip injecting the real block.
	tmpl := "# Fix {{.ID}}\n\n{{/* note: .Learnings is available but not rendered here */}}\n{{.Body}}\n"
	out, err := Prompt(s, fakeGit(), tk.ID, tmpl)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "UNIQUE_COMMENT_TRAP_INSIGHT") {
		t.Fatalf("learnings should be injected when template only mentions .Learnings in a comment:\n%s", out)
	}
}

func TestFormatLearningsOverflowNote(t *testing.T) {
	ls := []*learning.Learning{
		{ID: "LRN-001", Scope: "global", Body: "\na\n"},
		{ID: "LRN-002", Scope: "global", Body: "\nb\n"},
		{ID: "LRN-003", Scope: "global", Body: "\nc\n"},
	}
	block := FormatLearningsBlock(ls, 5)
	if !strings.Contains(block, "+5 more") {
		t.Fatalf("expected overflow note:\n%s", block)
	}
}

func TestCodeListAndExcerptHelpers(t *testing.T) {
	if got := codeList(nil, 3); got != "" {
		t.Fatalf("empty: %q", got)
	}
	got := codeList([]string{"a.go", "b.go", "c.go", "d.go"}, 2)
	if got != "`a.go`, `b.go`, … (+2 more)" {
		t.Fatalf("codeList=%q", got)
	}
	if got := excerpt("# Description\n\n\nFirst real line.\n\nSecond.\n"); got != "First real line." {
		t.Fatalf("excerpt desc=%q", got)
	}
	if got := excerpt("\n\n# ignored\nBare line\n"); got != "Bare line" {
		t.Fatalf("excerpt bare=%q", got)
	}
	if got := excerpt("\n\n# only\n"); got != "" {
		t.Fatalf("excerpt empty=%q", got)
	}
}

func TestWriteAcceptanceBlockEdges(t *testing.T) {
	var b strings.Builder
	writeAcceptanceBlock(&b, "# Description\nno ac\n")
	if b.Len() != 0 {
		t.Fatalf("no section: %q", b.String())
	}
	writeAcceptanceBlock(&b, "# Acceptance Criteria\n\njust prose\n")
	if b.Len() != 0 {
		t.Fatalf("no checkboxes: %q", b.String())
	}
	writeAcceptanceBlock(&b, "# Acceptance Criteria\n- [x] done\n- [ ] todo\n")
	if !strings.Contains(b.String(), "(1/2)") {
		t.Fatalf("progress: %q", b.String())
	}
}

func TestAppendLearningsBlockEdges(t *testing.T) {
	if got := appendLearningsBlock("out", nil, 0); got != "out" {
		t.Fatalf("%q", got)
	}
	ls := []*learning.Learning{{ID: "LRN-001", Scope: "global", Body: "\ntip\n"}}
	got := appendLearningsBlock("body-no-nl", ls, 0)
	if !strings.HasPrefix(got, "body-no-nl\n\n") || !strings.Contains(got, "tip") {
		t.Fatalf("%q", got)
	}
}

func TestExportWriteTicketMeta(t *testing.T) {
	s := scaffold(t)
	parent, err := s.Create(store.CreateReq{Type: "epic", Title: "Parent"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := s.Create(store.CreateReq{
		Type: "bug", Title: "Child", Parent: parent.ID,
		Labels: []string{"ui"}, Body: "details here",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(child.ID, func(tt *ticket.Ticket) error {
		tt.Deps = []string{parent.ID}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	md := ExportMarkdown(s)
	if !strings.Contains(md, "labels: ui") || !strings.Contains(md, "parent: "+parent.ID) {
		t.Fatalf("export meta:\n%s", md)
	}
	if !strings.Contains(md, "deps: "+parent.ID) {
		t.Fatalf("export deps:\n%s", md)
	}
	if !strings.Contains(md, "details here") {
		t.Fatalf("export body:\n%s", md)
	}
}

func TestContextIncludesTestingSection(t *testing.T) {
	s := scaffold(t)
	tk, err := s.Create(store.CreateReq{Type: "bug", Title: "In QA"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(tk.ID, func(tt *ticket.Ticket) error {
		tt.Status = ticket.StatusTesting
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	md := Context(s, fakeGit(), time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC))
	if !strings.Contains(md, "## In Testing") || !strings.Contains(md, tk.ID) {
		t.Fatalf("%s", md)
	}
}
