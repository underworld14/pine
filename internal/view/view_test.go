package view

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/underworld14/pine/internal/config"
	"github.com/underworld14/pine/internal/learning"
	"github.com/underworld14/pine/internal/store"
	"github.com/underworld14/pine/internal/ticket"
)

// scaffold mirrors the pattern used across the codebase (e.g.
// internal/contextgen/contextgen_test.go) for a temp .pine store.
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

func TestBuildWithDepsAndAttachments(t *testing.T) {
	s := scaffold(t)
	blocker, err := s.Create(store.CreateReq{Type: "bug", Title: "Blocker", Status: "todo"})
	if err != nil {
		t.Fatal(err)
	}
	dependent, err := s.Create(store.CreateReq{
		Type: "bug", Title: "Dependent", Status: "todo",
		Deps: []string{blocker.ID, "BUG-does-not-exist"},
		Body: "# Acceptance Criteria\n- [x] done one\n- [ ] pending one\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.WriteAttachment(dependent.ID, "note.txt", []byte("hello")); err != nil {
		t.Fatal(err)
	}

	g := s.Graph()
	v := Build(s, g, dependent, true)

	if v.ID != dependent.ID {
		t.Errorf("ID = %q, want %q", v.ID, dependent.ID)
	}
	if v.Type != "BUG" {
		t.Errorf("Type = %q, want BUG", v.Type)
	}
	if !v.Blocked {
		t.Error("expected dependent to be Blocked (unmet dep on blocker)")
	}
	if len(v.Unmet) != 1 || v.Unmet[0] != blocker.ID {
		t.Errorf("Unmet = %v, want [%s]", v.Unmet, blocker.ID)
	}
	if len(v.Dangling) != 1 || v.Dangling[0] != "BUG-does-not-exist" {
		t.Errorf("Dangling = %v, want [BUG-does-not-exist]", v.Dangling)
	}
	if v.InCycle {
		t.Error("did not expect InCycle")
	}
	if v.Hash == "" {
		t.Error("expected non-empty Hash")
	}
	if v.Body == "" {
		t.Error("expected Body to be included when includeBody=true")
	}
	if v.Source != "local" {
		t.Errorf("Source = %q, want local", v.Source)
	}
	if len(v.Attachments) != 1 || v.Attachments[0].Name != "note.txt" {
		t.Errorf("Attachments = %+v, want one entry named note.txt", v.Attachments)
	}
	if v.Acceptance == nil || v.Acceptance.Done != 1 || v.Acceptance.Total != 2 {
		t.Errorf("Acceptance = %+v, want 1/2", v.Acceptance)
	}
	if v.Created == "" || v.Updated == "" {
		t.Error("expected Created/Updated to be formatted")
	}

	// includeBody=false must omit the body.
	v2 := Build(s, g, dependent, false)
	if v2.Body != "" {
		t.Errorf("Body = %q, want empty when includeBody=false", v2.Body)
	}
}

func TestBuildEpicProgress(t *testing.T) {
	s := scaffold(t)
	epic, err := s.Create(store.CreateReq{Type: "epic", Title: "Epic"})
	if err != nil {
		t.Fatal(err)
	}
	child1, err := s.Create(store.CreateReq{Type: "feature", Title: "Child A", Parent: epic.ID, Status: "done"})
	if err != nil {
		t.Fatal(err)
	}
	child2, err := s.Create(store.CreateReq{Type: "feature", Title: "Child B", Parent: epic.ID, Status: "todo"})
	if err != nil {
		t.Fatal(err)
	}

	g := s.Graph()
	epicTicket, err := s.Get(epic.ID)
	if err != nil {
		t.Fatal(err)
	}
	v := Build(s, g, epicTicket, false)

	if v.EpicProgress == nil {
		t.Fatal("expected EpicProgress to be set for an epic with children")
	}
	if v.EpicProgress.Done != 1 || v.EpicProgress.Total != 2 {
		t.Errorf("EpicProgress = %+v, want 1/2", v.EpicProgress)
	}
	if len(v.Children) != 2 {
		t.Fatalf("Children = %v, want 2 entries", v.Children)
	}
	ids := map[string]bool{}
	for _, c := range v.Children {
		ids[c.ID] = true
	}
	if !ids[child1.ID] || !ids[child2.ID] {
		t.Errorf("Children = %+v, want %s and %s", v.Children, child1.ID, child2.ID)
	}
}

func TestBuildAllReturnsSortedViews(t *testing.T) {
	s := scaffold(t)
	if _, err := s.Create(store.CreateReq{Type: "bug", Title: "Bug One", Body: "body one"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(store.CreateReq{Type: "bug", Title: "Bug Two", Body: "body two"}); err != nil {
		t.Fatal(err)
	}

	out := BuildAll(s, true)
	if len(out) != 2 {
		t.Fatalf("len(out) = %d, want 2", len(out))
	}
	if out[0].ID > out[1].ID {
		t.Errorf("BuildAll not sorted by ID: %s before %s", out[0].ID, out[1].ID)
	}
	if out[0].Body == "" || out[1].Body == "" {
		t.Error("expected bodies to be included when includeBody=true")
	}

	outNoBody := BuildAll(s, false)
	for _, v := range outNoBody {
		if v.Body != "" {
			t.Errorf("Body = %q, want empty when includeBody=false", v.Body)
		}
	}
}

func TestBuildOffBranchIncludeBody(t *testing.T) {
	tk := &ticket.Ticket{
		ID: "BUG-7f3k2a", Title: "x", Status: "todo",
		Labels:  []string{"urgent"},
		Deps:    []string{"BUG-other"},
		Created: time.Now(), Updated: time.Now(),
		Body: "some body text",
	}
	v := BuildOffBranch(tk, "feature-branch", true)

	if v.Body != "some body text" {
		t.Errorf("Body = %q, want %q", v.Body, "some body text")
	}
	if v.Source != "local-branch" {
		t.Errorf("Source = %q, want local-branch", v.Source)
	}
	if v.Branch != "feature-branch" {
		t.Errorf("Branch = %q, want feature-branch", v.Branch)
	}
	if !v.ReadOnly {
		t.Error("expected ReadOnly = true")
	}
	if v.Attachments == nil || len(v.Attachments) != 0 {
		t.Errorf("Attachments = %v, want non-nil empty slice", v.Attachments)
	}
	if len(v.Labels) != 1 || v.Labels[0] != "urgent" {
		t.Errorf("Labels = %v, want [urgent]", v.Labels)
	}
	if len(v.Deps) != 1 || v.Deps[0] != "BUG-other" {
		t.Errorf("Deps = %v, want [BUG-other]", v.Deps)
	}

	v2 := BuildOffBranch(tk, "feature-branch", false)
	if v2.Body != "" {
		t.Errorf("Body = %q, want empty when includeBody=false", v2.Body)
	}
}

func TestBuildLearningProjectsFields(t *testing.T) {
	created := time.Date(2026, 7, 1, 8, 30, 0, 0, time.UTC)
	l := &learning.Learning{
		ID:          "LRN-001",
		Scope:       learning.ScopeTicket,
		Tags:        []string{"backend", "auth"},
		Ticket:      "BUG-001",
		SourceAgent: learning.SourceClaudeCode,
		Supersedes:  "LRN-000",
		Cites:       []string{"internal/store/store.go"},
		Created:     created,
		Body:        "  trimmed body  \n",
		Degraded:    true,
	}

	v := BuildLearning(l)

	if v.ID != "LRN-001" {
		t.Errorf("ID = %q, want LRN-001", v.ID)
	}
	if v.Scope != learning.ScopeTicket {
		t.Errorf("Scope = %q, want %q", v.Scope, learning.ScopeTicket)
	}
	if len(v.Tags) != 2 || v.Tags[0] != "backend" || v.Tags[1] != "auth" {
		t.Errorf("Tags = %v, want [backend auth]", v.Tags)
	}
	if v.Ticket != "BUG-001" {
		t.Errorf("Ticket = %q, want BUG-001", v.Ticket)
	}
	if v.SourceAgent != learning.SourceClaudeCode {
		t.Errorf("SourceAgent = %q, want %q", v.SourceAgent, learning.SourceClaudeCode)
	}
	if v.Supersedes != "LRN-000" {
		t.Errorf("Supersedes = %q, want LRN-000", v.Supersedes)
	}
	if len(v.Cites) != 1 || v.Cites[0] != "internal/store/store.go" {
		t.Errorf("Cites = %v, want [internal/store/store.go]", v.Cites)
	}
	if v.Created != fmtTime(created) {
		t.Errorf("Created = %q, want %q", v.Created, fmtTime(created))
	}
	if v.Body != "trimmed body" {
		t.Errorf("Body = %q, want %q", v.Body, "trimmed body")
	}
	if !v.Degraded {
		t.Error("expected Degraded = true")
	}
}

func TestBuildLearningWithoutTagsOrCites(t *testing.T) {
	l := &learning.Learning{
		ID:          "LRN-002",
		Scope:       learning.ScopeGlobal,
		SourceAgent: learning.SourceManual,
		Body:        "no tags or cites",
	}

	v := BuildLearning(l)

	if len(v.Tags) != 0 {
		t.Errorf("Tags = %v, want empty", v.Tags)
	}
	if len(v.Cites) != 0 {
		t.Errorf("Cites = %v, want empty", v.Cites)
	}
	if v.Created != "" {
		t.Errorf("Created = %q, want empty for zero time", v.Created)
	}
	if v.Ticket != "" {
		t.Errorf("Ticket = %q, want empty", v.Ticket)
	}
}

func TestNonNil(t *testing.T) {
	if got := nonNil(nil); got == nil || len(got) != 0 {
		t.Errorf("nonNil(nil) = %v, want non-nil empty slice", got)
	}
	in := []string{"a", "b"}
	if got := nonNil(in); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("nonNil(%v) = %v, want unchanged", in, got)
	}
}

func TestAttachmentsOrEmpty(t *testing.T) {
	if got := attachmentsOrEmpty(nil); got == nil || len(got) != 0 {
		t.Errorf("attachmentsOrEmpty(nil) = %v, want non-nil empty slice", got)
	}
	in := []store.AttachmentInfo{{Name: "a.txt"}}
	if got := attachmentsOrEmpty(in); len(got) != 1 || got[0].Name != "a.txt" {
		t.Errorf("attachmentsOrEmpty(%v) = %v, want unchanged", in, got)
	}
}

func TestFmtTime(t *testing.T) {
	if got := fmtTime(time.Time{}); got != "" {
		t.Errorf("fmtTime(zero) = %q, want empty", got)
	}
	ts := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	if got := fmtTime(ts); got != "2026-07-04T12:00:00Z" {
		t.Errorf("fmtTime = %q, want 2026-07-04T12:00:00Z", got)
	}
}

func TestBuildOffBranchAcceptance(t *testing.T) {
	tk := &ticket.Ticket{
		ID: "BUG-7f3k2a", Title: "x", Status: "todo",
		Created: time.Now(), Updated: time.Now(),
		Body: "# Acceptance Criteria\n- [x] a\n- [ ] b\n",
	}
	v := BuildOffBranch(tk, "feature", false)
	if v.Acceptance == nil || v.Acceptance.Done != 1 || v.Acceptance.Total != 2 {
		t.Fatalf("acceptance = %+v, want 1/2", v.Acceptance)
	}

	tk.Body = "# Description\nno criteria\n"
	if v := BuildOffBranch(tk, "feature", false); v.Acceptance != nil {
		t.Errorf("no AC section should omit acceptance, got %+v", v.Acceptance)
	}
}
