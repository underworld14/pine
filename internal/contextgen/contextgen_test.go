package contextgen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/underworld14/pine/internal/config"
	"github.com/underworld14/pine/internal/gitx"
	"github.com/underworld14/pine/internal/store"
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
