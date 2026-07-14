package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/underworld14/pine/internal/config"
	"github.com/underworld14/pine/internal/store"
	"github.com/underworld14/pine/internal/ticket"
)

func scaffold(t *testing.T) (*store.Store, string) {
	t.Helper()
	pine := filepath.Join(t.TempDir(), ".pine")
	if err := os.MkdirAll(filepath.Join(pine, "tickets"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default("test")
	cfg.IDStyle = "sequential"
	cfgB, _ := cfg.Bytes()
	os.WriteFile(filepath.Join(pine, "config.json"), cfgB, 0o644)
	bB, _ := config.DefaultBoard().Bytes()
	os.WriteFile(filepath.Join(pine, "board.json"), bB, 0o644)
	s, err := store.Open(pine)
	if err != nil {
		t.Fatal(err)
	}
	return s, pine
}

func reopen(t *testing.T, pine string) *store.Store {
	t.Helper()
	s, err := store.Open(pine)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func msgs(r *Report) string {
	var b strings.Builder
	for _, f := range r.Findings {
		b.WriteString(f.Msg)
		b.WriteString("\n")
	}
	return b.String()
}

func TestCleanWorkspacePasses(t *testing.T) {
	s, _ := scaffold(t)
	s.Create(store.CreateReq{Type: "bug", Title: "x"})
	r := Run(s)
	if r.HasErrors() {
		t.Errorf("clean workspace should have no errors:\n%s", msgs(r))
	}
}

func TestDetectsCycleAndDangling(t *testing.T) {
	s, pine := scaffold(t)
	s.Create(store.CreateReq{Type: "bug", Title: "a"}) // BUG-001
	s.Create(store.CreateReq{Type: "bug", Title: "b"}) // BUG-002
	writeDeps(t, pine, "BUG-001", []string{"BUG-002"})
	writeDeps(t, pine, "BUG-002", []string{"BUG-001", "GHOST-999"})

	r := Run(reopen(t, pine))
	out := msgs(r)
	if !r.HasErrors() || !strings.Contains(out, "dependency cycle") {
		t.Errorf("expected a cycle error:\n%s", out)
	}
	if !strings.Contains(out, "dangling dependency GHOST-999") {
		t.Errorf("expected dangling dependency warning:\n%s", out)
	}
}

func TestDetectsDegradedAndStray(t *testing.T) {
	_, pine := scaffold(t)
	os.WriteFile(filepath.Join(pine, "tickets", "BUG-001.md"), []byte("no frontmatter here\n"), 0o644)
	os.WriteFile(filepath.Join(pine, "tickets", "notes.txt"), []byte("scratch"), 0o644)

	r := Run(reopen(t, pine))
	out := msgs(r)
	if !r.HasErrors() || !strings.Contains(out, "malformed") {
		t.Errorf("expected malformed error:\n%s", out)
	}
	if !strings.Contains(out, "stray file") {
		t.Errorf("expected stray file warning:\n%s", out)
	}
}

func TestDetectsBrokenAttachmentRef(t *testing.T) {
	s, pine := scaffold(t)
	s.Create(store.CreateReq{Type: "bug", Title: "x",
		Body: "# Attachments\n- ../attachments/BUG-001/missing.webp\n"})
	r := Run(reopen(t, pine))
	if !r.HasErrors() || !strings.Contains(msgs(r), "missing attachment") {
		t.Errorf("expected missing-attachment error:\n%s", msgs(r))
	}
}

func TestLearningsValidation(t *testing.T) {
	s, pine := scaffold(t)
	s.Create(store.CreateReq{Type: "bug", Title: "x"})
	if _, err := s.CreateLearning(store.CreateLearningReq{
		Text: "valid insight", Scope: "global", Tags: []string{"db"},
	}); err != nil {
		t.Fatal(err)
	}
	r := Run(reopen(t, pine))
	if r.HasErrors() {
		t.Errorf("valid learning should not error:\n%s", msgs(r))
	}

	os.MkdirAll(filepath.Join(pine, "learnings"), 0o755)
	os.WriteFile(filepath.Join(pine, "learnings", "LRN-099.md"), []byte(`---
id: LRN-099
scope: ticket
ticket: BUG-999
source_agent: manual
created: 2026-07-11T00:00:00Z
---
orphan insight
`), 0o644)
	os.WriteFile(filepath.Join(pine, "learnings", "notes.txt"), []byte("scratch"), 0o644)
	os.WriteFile(filepath.Join(pine, "learnings", "LRN-100.md"), []byte("no frontmatter\n"), 0o644)

	r = Run(reopen(t, pine))
	out := msgs(r)
	if !strings.Contains(out, "dangling ticket ref BUG-999") {
		t.Errorf("expected dangling ticket warn:\n%s", out)
	}
	if !strings.Contains(out, "stray file") {
		t.Errorf("expected stray learning warn:\n%s", out)
	}
	if !r.HasErrors() || !strings.Contains(out, "malformed") {
		t.Errorf("expected malformed learning error:\n%s", out)
	}
}

func TestLearningsSupersedesDoctor(t *testing.T) {
	s, pine := scaffold(t)
	a, err := s.CreateLearning(store.CreateLearningReq{Text: "old"})
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(pine, "learnings"), 0o755)
	os.WriteFile(filepath.Join(pine, "learnings", "LRN-050.md"), []byte(`---
id: LRN-050
scope: global
source_agent: manual
supersedes: LRN-NOPE
created: 2026-07-11T00:00:00Z
---
dangling
`), 0o644)
	// Cycle: rewrite a to supersede a future mutual file.
	b, err := s.CreateLearning(store.CreateLearningReq{Text: "b", Supersedes: a.ID})
	if err != nil {
		t.Fatal(err)
	}
	pathA := filepath.Join(pine, "learnings", a.ID+".md")
	raw, _ := os.ReadFile(pathA)
	os.WriteFile(pathA, []byte(strings.Replace(string(raw), "source_agent:", "supersedes: "+b.ID+"\nsource_agent:", 1)), 0o644)

	r := Run(reopen(t, pine))
	out := msgs(r)
	if !strings.Contains(out, "dangling supersedes ref LRN-NOPE") {
		t.Errorf("expected dangling supersedes warn:\n%s", out)
	}
	if !r.HasErrors() || !strings.Contains(out, "supersede cycle") {
		t.Errorf("expected supersede cycle error:\n%s", out)
	}
}

func TestLearningsCitesDoctor(t *testing.T) {
	s, pine := scaffold(t)
	repoRoot := filepath.Dir(pine)
	okPath := filepath.Join(repoRoot, "internal", "ok.go")
	if err := os.MkdirAll(filepath.Dir(okPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(okPath, []byte("package ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	valid, err := s.CreateLearning(store.CreateLearningReq{
		Text:  "valid cite",
		Cites: []string{"internal/ok.go"},
	})
	if err != nil {
		t.Fatal(err)
	}
	oneMissing, err := s.CreateLearning(store.CreateLearningReq{
		Text:  "one missing",
		Cites: []string{"internal/ok.go", "internal/gone.go"},
	})
	if err != nil {
		t.Fatal(err)
	}
	allMissing, err := s.CreateLearning(store.CreateLearningReq{
		Text:  "all missing",
		Cites: []string{"a.go", "b.go"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.CreateLearning(store.CreateLearningReq{Text: "no cites"})
	if err != nil {
		t.Fatal(err)
	}

	r := Run(reopen(t, pine))
	out := msgs(r)
	if strings.Contains(out, valid.ID+": dangling cite") {
		t.Errorf("valid cites should not warn:\n%s", out)
	}
	if !strings.Contains(out, oneMissing.ID+": dangling cite internal/gone.go") {
		t.Errorf("expected one-missing warn:\n%s", out)
	}
	if strings.Contains(out, oneMissing.ID+": dangling cite internal/ok.go") {
		t.Errorf("ok path should not be reported missing:\n%s", out)
	}
	if !strings.Contains(out, allMissing.ID+": dangling cite a.go") ||
		!strings.Contains(out, allMissing.ID+": dangling cite b.go") {
		t.Errorf("expected all-missing warns:\n%s", out)
	}
}

func TestLearningsFrontmatterIDMismatch(t *testing.T) {
	_, pine := scaffold(t)
	os.MkdirAll(filepath.Join(pine, "learnings"), 0o755)
	os.WriteFile(filepath.Join(pine, "learnings", "LRN-005.md"), []byte(`---
id: LRN-999
scope: global
source_agent: manual
created: 2026-07-11T00:00:00Z
---
mismatched frontmatter id
`), 0o644)
	r := Run(reopen(t, pine))
	out := msgs(r)
	if !strings.Contains(out, "LRN-005: frontmatter id is LRN-999 (does not match filename)") {
		t.Errorf("expected frontmatter-id-mismatch warning:\n%s", out)
	}
}

func TestAttachmentDirOrphanAndOversizedVideo(t *testing.T) {
	s, _ := scaffold(t)
	s.Create(store.CreateReq{Type: "bug", Title: "x"}) // BUG-001

	// Lower the threshold so the test doesn't need to write 50MB of data.
	cfg := s.Config()
	cfg.Attachments.MaxVideoMB = 1
	if err := s.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}

	// Oversized video attachment on a real ticket.
	big := make([]byte, 2*1024*1024)
	if _, err := s.WriteAttachment("BUG-001", "clip.mp4", big); err != nil {
		t.Fatal(err)
	}

	// Orphaned attachments directory: no ticket BUG-999 exists.
	if _, err := s.WriteAttachment("BUG-999", "orphan.png", []byte("x")); err != nil {
		t.Fatal(err)
	}

	r := Run(s)
	out := msgs(r)
	if !strings.Contains(out, "attachments/BUG-999: orphaned directory (no such ticket)") {
		t.Errorf("expected orphaned dir warning:\n%s", out)
	}
	if !strings.Contains(out, "BUG-001: video clip.mp4 exceeds 1MB (bloats the repo)") {
		t.Errorf("expected oversized video warning:\n%s", out)
	}
}

func TestAttachmentDirVideoUnderLimitNoWarning(t *testing.T) {
	s, _ := scaffold(t)
	s.Create(store.CreateReq{Type: "bug", Title: "x"}) // BUG-001

	cfg := s.Config()
	cfg.Attachments.MaxVideoMB = 1
	if err := s.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	small := make([]byte, 1024)
	if _, err := s.WriteAttachment("BUG-001", "clip.mp4", small); err != nil {
		t.Fatal(err)
	}

	r := Run(s)
	out := msgs(r)
	if strings.Contains(out, "exceeds") {
		t.Errorf("small video should not warn:\n%s", out)
	}
	if strings.Contains(out, "orphaned directory") {
		t.Errorf("attached-to-real-ticket dir should not be orphaned:\n%s", out)
	}
}

func TestGitignoreWarnsWhenPineIsIgnored(t *testing.T) {
	s, pine := scaffold(t)
	repoRoot := filepath.Dir(pine)
	if err := os.WriteFile(filepath.Join(repoRoot, ".gitignore"),
		[]byte("node_modules/\n/.pine/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := Run(s)
	if !strings.Contains(msgs(r), ".pine is gitignored — Pine data is meant to be committed") {
		t.Errorf("expected gitignore warning:\n%s", msgs(r))
	}
}

func TestGitignoreNoWarningWhenPineNotMentioned(t *testing.T) {
	s, pine := scaffold(t)
	repoRoot := filepath.Dir(pine)
	if err := os.WriteFile(filepath.Join(repoRoot, ".gitignore"),
		[]byte("node_modules/\ndist/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := Run(s)
	if strings.Contains(msgs(r), "is gitignored") {
		t.Errorf("unexpected gitignore warning:\n%s", msgs(r))
	}
}

func TestGitignoreNoWarningWhenFileAbsent(t *testing.T) {
	s, _ := scaffold(t)
	// No .gitignore written at the repo root at all.
	r := Run(s)
	if strings.Contains(msgs(r), "is gitignored") {
		t.Errorf("unexpected gitignore warning:\n%s", msgs(r))
	}
}

func TestGitignoreNoWarningForNestedAttachmentsOnly(t *testing.T) {
	s, pine := scaffold(t)
	body := "# pine:sync begin\n# tickets=on attachments=off\nattachments/\n# pine:sync end\n"
	if err := os.WriteFile(filepath.Join(pine, ".gitignore"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	r := Run(s)
	out := msgs(r)
	if strings.Contains(out, "is gitignored") || strings.Contains(out, "tickets/") {
		t.Errorf("attachments-only ignore should not warn:\n%s", out)
	}
}

func TestGitignoreWarnsWhenTicketsIgnoredUnexpectedly(t *testing.T) {
	s, pine := scaffold(t)
	body := "# pine:sync begin\n# tickets=off attachments=off\ntickets/\nattachments/\n# pine:sync end\n"
	if err := os.WriteFile(filepath.Join(pine, ".gitignore"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	// Default config has sync.tickets=true.
	r := Run(s)
	if !strings.Contains(msgs(r), "tickets/") {
		t.Errorf("expected tickets ignore warning:\n%s", msgs(r))
	}
}

func TestGitignoreNoWarningWhenTicketsLocalByConfig(t *testing.T) {
	s, pine := scaffold(t)
	cfg := config.Default("t")
	cfg.Sync.Tickets = false
	cfg.Sync.Attachments = false
	b, err := cfg.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pine, "config.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	body := "# pine:sync begin\n# tickets=off attachments=off\ntickets/\nattachments/\n# pine:sync end\n"
	if err := os.WriteFile(filepath.Join(pine, ".gitignore"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	r := Run(s)
	if strings.Contains(msgs(r), "tickets/") {
		t.Errorf("local tickets per config should not warn:\n%s", msgs(r))
	}
}

func TestRunReportsInvalidConfigAndBoard(t *testing.T) {
	pine := filepath.Join(t.TempDir(), ".pine")
	if err := os.MkdirAll(filepath.Join(pine, "tickets"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default("t")
	cfg.Version = 0
	cfgB, err := cfg.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pine, "config.json"), cfgB, 0o644); err != nil {
		t.Fatal(err)
	}
	board := config.DefaultBoard()
	board.Columns = []config.Column{} // non-nil empty slice: ParseBoard only
	// falls back to defaults for a nil (absent/null) columns key, so this must
	// marshal to "columns":[] to actually exercise the empty-columns problem.
	boardB, err := board.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pine, "board.json"), boardB, 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := store.Open(pine)
	if err != nil {
		t.Fatal(err)
	}
	r := Run(s)
	out := msgs(r)
	if !strings.Contains(out, "config.json: config.version must be >= 1") {
		t.Errorf("expected config validation error:\n%s", out)
	}
	if !strings.Contains(out, "board.json: board.columns must not be empty") {
		t.Errorf("expected board validation error:\n%s", out)
	}
	if !r.HasErrors() {
		t.Errorf("expected HasErrors true:\n%s", out)
	}
}

func TestTicketLevelWarningsAndParentChecks(t *testing.T) {
	s, pine := scaffold(t)

	// A non-degraded ticket with a scalar "labels" field (lenient-parse
	// warning) and a frontmatter id that doesn't match its filename.
	raw := `---
id: BUG-999
title: weird
status: todo
priority: medium
labels: notalist
created: "2026-07-11T00:00:00Z"
updated: "2026-07-11T00:00:00Z"
---

body
`
	if err := os.WriteFile(filepath.Join(pine, "tickets", "BUG-777.md"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Create(store.CreateReq{Type: "epic", Title: "epic1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(store.CreateReq{Type: "bug", Title: "bad-status-priority",
		Status: "nonexistent-status", Priority: "nonexistent-priority"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(store.CreateReq{Type: "bug", Title: "child-of-non-epic", Parent: "BUG-777"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(store.CreateReq{Type: "bug", Title: "child-of-ghost", Parent: "EPIC-999"}); err != nil {
		t.Fatal(err)
	}

	r := Run(reopen(t, pine))
	out := msgs(r)
	if !strings.Contains(out, "labels was a scalar; wrapped into a list") {
		t.Errorf("expected labels warning:\n%s", out)
	}
	if !strings.Contains(out, "BUG-777: frontmatter id is BUG-999 (does not match filename)") {
		t.Errorf("expected frontmatter id mismatch warning:\n%s", out)
	}
	if !strings.Contains(out, "status nonexistent-status matches no board column") {
		t.Errorf("expected status warning:\n%s", out)
	}
	if !strings.Contains(out, "priority nonexistent-priority is not configured") {
		t.Errorf("expected priority warning:\n%s", out)
	}
	if !strings.Contains(out, "parent BUG-777 is not an epic") {
		t.Errorf("expected non-epic parent warning:\n%s", out)
	}
	if !strings.Contains(out, "parent EPIC-999 does not exist") {
		t.Errorf("expected missing parent warning:\n%s", out)
	}
}

func TestAttachmentRefsCoversPresentAndNonAttachmentPaths(t *testing.T) {
	s, pine := scaffold(t)
	tk, err := s.Create(store.CreateReq{Type: "bug", Title: "withrefs",
		Body: "# Attachments\n- ../attachments/self/real.png\n- just-a-note.txt\n"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.WriteAttachment(tk.ID, "real.png", []byte("data")); err != nil {
		t.Fatal(err)
	}

	r := Run(reopen(t, pine))
	out := msgs(r)
	if strings.Contains(out, "missing attachment") {
		t.Errorf("real.png exists and a non-attachments path should not be reported missing:\n%s", out)
	}
}

func TestStraysSkipsDirectoriesAndDotfiles(t *testing.T) {
	_, pine := scaffold(t)
	if err := os.MkdirAll(filepath.Join(pine, "tickets", "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pine, "tickets", ".DS_Store"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := Run(reopen(t, pine))
	out := msgs(r)
	if strings.Contains(out, "subdir") || strings.Contains(out, ".DS_Store") {
		t.Errorf("directories and dotfiles should be skipped, not flagged as stray:\n%s", out)
	}
}

func TestLearningsWarningsScopeAndSourceAgent(t *testing.T) {
	_, pine := scaffold(t)
	if err := os.MkdirAll(filepath.Join(pine, "learnings"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pine, "learnings", "LRN-321.md"), []byte(`---
id: LRN-321
scope: bogus-scope
source_agent: bogus-agent
tags: notalist
created: 2026-07-11T00:00:00Z
---
insight text
`), 0o644); err != nil {
		t.Fatal(err)
	}

	r := Run(reopen(t, pine))
	out := msgs(r)
	if !strings.Contains(out, "tags was a scalar; wrapped into a list") {
		t.Errorf("expected tags warning:\n%s", out)
	}
	if !strings.Contains(out, "scope bogus-scope is not valid (expected global, ticket, or component)") {
		t.Errorf("expected scope warning:\n%s", out)
	}
	if !strings.Contains(out, "source_agent bogus-agent is not recognized") {
		t.Errorf("expected source_agent warning:\n%s", out)
	}
}

func TestLearningsTicketScopeEmptyTicketField(t *testing.T) {
	_, pine := scaffold(t)
	if err := os.MkdirAll(filepath.Join(pine, "learnings"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pine, "learnings", "LRN-322.md"), []byte(`---
id: LRN-322
scope: ticket
source_agent: manual
created: 2026-07-11T00:00:00Z
---
insight text
`), 0o644); err != nil {
		t.Fatal(err)
	}

	r := Run(reopen(t, pine))
	out := msgs(r)
	if !strings.Contains(out, "LRN-322: scope is ticket but ticket field is empty") {
		t.Errorf("expected empty-ticket-field warning:\n%s", out)
	}
}

func writeDeps(t *testing.T, pine, id string, deps []string) {
	t.Helper()
	path := filepath.Join(pine, "tickets", id+".md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	depBlock := "deps:\n"
	for _, d := range deps {
		depBlock += "  - " + d + "\n"
	}
	// Insert a deps block just before the closing frontmatter delimiter.
	s := string(raw)
	idx := strings.Index(s, "\n---\n")
	if idx < 0 {
		t.Fatalf("no frontmatter in %s", id)
	}
	updated := s[:idx+1] + depBlock + s[idx+1:]
	os.WriteFile(path, []byte(updated), 0o644)
}

// findByCode returns the first finding with the given code, or a zero Finding.
func findByCode(r *Report, code string) Finding {
	for _, f := range r.Findings {
		if f.Code == code {
			return f
		}
	}
	return Finding{}
}

func TestFixDanglingDep(t *testing.T) {
	s, pine := scaffold(t)
	tk, err := s.Create(store.CreateReq{Type: "bug", Title: "has bad dep"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(tk.ID, func(x *ticket.Ticket) error { x.Deps = []string{"BUG-999"}; return nil }); err != nil {
		t.Fatal(err)
	}
	r := Run(s)
	f := findByCode(r, "dangling-dep")
	if !f.Fixable() {
		t.Fatalf("expected fixable dangling-dep, findings:\n%s", msgs(r))
	}
	if err := f.Fix(s); err != nil {
		t.Fatalf("fix: %v", err)
	}
	r2 := Run(reopen(t, pine))
	if strings.Contains(msgs(r2), "dangling dependency") {
		t.Errorf("dangling dep should be gone:\n%s", msgs(r2))
	}
}

func TestFixFrontmatterIDMismatch(t *testing.T) {
	s, pine := scaffold(t)
	// Filename says BUG-001, frontmatter claims BUG-999.
	body := "---\nid: BUG-999\ntitle: mismatch\nstatus: todo\ncreated: 2026-07-11T00:00:00Z\nupdated: 2026-07-11T00:00:00Z\n---\nbody\n"
	if err := os.WriteFile(filepath.Join(pine, "tickets", "BUG-001.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	s = reopen(t, pine)
	r := Run(s)
	f := findByCode(r, "frontmatter-id-mismatch")
	if !f.Fixable() {
		t.Fatalf("expected fixable id mismatch:\n%s", msgs(r))
	}
	if err := f.Fix(s); err != nil {
		t.Fatalf("fix: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(pine, "tickets", "BUG-001.md"))
	if !strings.Contains(string(data), "id: BUG-001") || strings.Contains(string(data), "BUG-999") {
		t.Errorf("id not canonicalized:\n%s", data)
	}
}

func TestFixDanglingCite(t *testing.T) {
	s, pine := scaffold(t)
	if err := os.MkdirAll(filepath.Join(pine, "learnings"), 0o755); err != nil {
		t.Fatal(err)
	}
	l, err := s.CreateLearning(store.CreateLearningReq{Text: "cites a gone file", Cites: []string{"internal/gone.go"}})
	if err != nil {
		t.Fatal(err)
	}
	r := Run(reopen(t, pine))
	f := findByCode(r, "dangling-cite")
	if !f.Fixable() {
		t.Fatalf("expected fixable dangling-cite:\n%s", msgs(r))
	}
	s = reopen(t, pine)
	if err := f.Fix(s); err != nil {
		t.Fatalf("fix: %v", err)
	}
	got, err := s.GetLearning(l.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Cites) != 0 {
		t.Errorf("cite should be dropped, got %v", got.Cites)
	}
}

func TestDegradedNotFixable(t *testing.T) {
	s, pine := scaffold(t)
	// A ticket file with malformed YAML frontmatter is degraded (report-only).
	bad := "---\nid: BUG-001\nstatus: [unclosed\n---\nbody\n"
	if err := os.WriteFile(filepath.Join(pine, "tickets", "BUG-001.md"), []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = s
	r := Run(reopen(t, pine))
	for _, f := range r.Findings {
		if strings.Contains(f.Msg, "malformed") && f.Fixable() {
			t.Errorf("degraded ticket must not be auto-fixable: %s", f.Msg)
		}
	}
}

func TestFixableCountAndParentFixes(t *testing.T) {
	s, pine := scaffold(t)
	if (&Report{}).FixableCount() != 0 {
		t.Fatal("empty")
	}
	bug, err := s.Create(store.CreateReq{Type: "bug", Title: "not-epic"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(store.CreateReq{Type: "bug", Title: "kid", Parent: bug.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(store.CreateReq{Type: "bug", Title: "ghost-parent", Parent: "EPIC-999"}); err != nil {
		t.Fatal(err)
	}

	r := Run(s)
	if r.FixableCount() < 2 {
		t.Fatalf("expected fixable parent findings, count=%d\n%s", r.FixableCount(), msgs(r))
	}
	f := findByCode(r, "parent-not-epic")
	if !f.Fixable() {
		t.Fatal("parent-not-epic should be fixable")
	}
	if err := f.Fix(s); err != nil {
		t.Fatal(err)
	}
	f2 := findByCode(r, "dangling-parent")
	if err := f2.Fix(s); err != nil {
		t.Fatal(err)
	}
	r2 := Run(reopen(t, pine))
	out := msgs(r2)
	if strings.Contains(out, "is not an epic") || strings.Contains(out, "does not exist") {
		t.Fatalf("parents should be cleared:\n%s", out)
	}
}

func TestStrayRenameTargetFixable(t *testing.T) {
	_, pine := scaffold(t)
	// Valid frontmatter id, invalid filename → rename target free.
	body := "---\nid: BUG-042\ntitle: stray\nstatus: todo\ncreated: 2026-07-11T00:00:00Z\nupdated: 2026-07-11T00:00:00Z\n---\nbody\n"
	if err := os.WriteFile(filepath.Join(pine, "tickets", "weird-name.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	r := Run(reopen(t, pine))
	f := findByCode(r, "stray-file")
	if !f.Fixable() {
		t.Fatalf("expected fixable stray rename:\n%s", msgs(r))
	}
	s := reopen(t, pine)
	if err := f.Fix(s); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(pine, "tickets", "BUG-042.md")); err != nil {
		t.Fatal(err)
	}
}

func TestStrayRenameTargetTakenOrInvalid(t *testing.T) {
	dir := t.TempDir()
	present := map[string]bool{"BUG-001.md": true}
	if got := strayRenameTarget(dir, "missing.md", present); got != "" {
		t.Fatalf("missing file: %q", got)
	}
	os.WriteFile(filepath.Join(dir, "x.md"), []byte("no fm\n"), 0o644)
	if got := strayRenameTarget(dir, "x.md", present); got != "" {
		t.Fatalf("no id: %q", got)
	}
	os.WriteFile(filepath.Join(dir, "y.md"), []byte("---\nid: BUG-001\ntitle: t\n---\n"), 0o644)
	if got := strayRenameTarget(dir, "y.md", present); got != "" {
		t.Fatalf("taken: %q", got)
	}
	os.WriteFile(filepath.Join(dir, "z.md"), []byte("---\nid: BUG-002\ntitle: t\n---\n"), 0o644)
	if got := strayRenameTarget(dir, "z.md", present); got != "BUG-002.md" {
		t.Fatalf("got %q", got)
	}
}

func TestRemoveStringKeepsOthers(t *testing.T) {
	got := removeString([]string{"a", "b", "a"}, "a")
	if len(got) != 1 || got[0] != "b" {
		t.Fatalf("%v", got)
	}
}

func TestCrossBranchWarnAndComponentEmpty(t *testing.T) {
	s, pine := scaffold(t)
	cfg := s.Config()
	cfg.CrossBranch.Enabled = true
	cfg.IDStyle = "sequential"
	if err := s.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(pine, "learnings"), 0o755)
	os.WriteFile(filepath.Join(pine, "learnings", "LRN-400.md"), []byte(`---
id: LRN-400
scope: component
source_agent: manual
created: 2026-07-11T00:00:00Z
---
no component field
`), 0o644)
	r := Run(reopen(t, pine))
	out := msgs(r)
	if !strings.Contains(out, "crossBranch is enabled") {
		t.Fatalf("crossBranch warn:\n%s", out)
	}
	if !strings.Contains(out, "scope is component but component field is empty") {
		t.Fatalf("component empty:\n%s", out)
	}
}

func TestFixLearningFrontmatterAndSupersedes(t *testing.T) {
	s, pine := scaffold(t)
	os.MkdirAll(filepath.Join(pine, "learnings"), 0o755)
	os.WriteFile(filepath.Join(pine, "learnings", "LRN-010.md"), []byte(`---
id: LRN-999
scope: global
source_agent: manual
created: 2026-07-11T00:00:00Z
---
mismatch
`), 0o644)
	os.WriteFile(filepath.Join(pine, "learnings", "LRN-011.md"), []byte(`---
id: LRN-011
scope: global
source_agent: manual
supersedes: LRN-NOPE
created: 2026-07-11T00:00:00Z
---
dangling
`), 0o644)
	// empty cite entry is skipped
	os.WriteFile(filepath.Join(pine, "learnings", "LRN-012.md"), []byte(`---
id: LRN-012
scope: global
source_agent: manual
cites:
  - ""
  - gone.go
created: 2026-07-11T00:00:00Z
---
cites
`), 0o644)
	s = reopen(t, pine)
	r := Run(s)
	f := findByCode(r, "learning-frontmatter-id-mismatch")
	if err := f.Fix(s); err != nil {
		t.Fatal(err)
	}
	f2 := findByCode(r, "dangling-supersedes")
	if err := f2.Fix(s); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(pine, "learnings", "LRN-010.md"))
	if !strings.Contains(string(data), "id: LRN-010") {
		t.Fatalf("id not fixed:\n%s", data)
	}
}

func TestCheckMergeDriverConfiguredNoWarn(t *testing.T) {
	_, pine := scaffold(t)
	repo := filepath.Dir(pine)
	os.WriteFile(filepath.Join(repo, ".gitattributes"), []byte("foo merge=other\n"), 0o644)
	r := &Report{}
	checkMergeDriver(r, pine)
	if len(r.Findings) != 0 {
		t.Fatalf("no pine merge: %#v", r.Findings)
	}
}
