package store

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/underworld14/pine/internal/learning"
	"github.com/underworld14/pine/internal/ticket"
)

// Tests CreateLearning / ListLearnings / GetLearning / scanLearnings.
// Called only by `go test ./internal/store/`.

func TestCreateLearningGlobal(t *testing.T) {
	s := scaffold(t)
	l, err := s.CreateLearning(CreateLearningReq{
		Text:  "Always use the query builder",
		Scope: learning.ScopeGlobal,
		Tags:  []string{"db", "migration"},
	})
	must(t, err)
	if !strings.HasPrefix(l.ID, "LRN-") {
		t.Fatalf("id = %q", l.ID)
	}
	if l.Scope != learning.ScopeGlobal {
		t.Errorf("scope = %q", l.Scope)
	}
	if l.SourceAgent != learning.SourceManual {
		t.Errorf("source = %q", l.SourceAgent)
	}
	path := filepath.Join(s.Root(), "learnings", l.ID+".md")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file missing: %v", err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "query builder") {
		t.Errorf("body missing from file:\n%s", data)
	}
}

func TestCreateLearningTicketScope(t *testing.T) {
	s := scaffold(t)
	tk, err := s.Create(CreateReq{Type: "bug", Title: "schema drift"})
	must(t, err)
	l, err := s.CreateLearning(CreateLearningReq{
		Text:   "Related to schema drift fix",
		Scope:  learning.ScopeTicket,
		Ticket: tk.ID,
		Tags:   []string{"db"},
	})
	must(t, err)
	if l.Ticket != tk.ID {
		t.Errorf("ticket = %q want %q", l.Ticket, tk.ID)
	}
}

func TestCreateLearningTicketMissing(t *testing.T) {
	s := scaffold(t)
	_, err := s.CreateLearning(CreateLearningReq{
		Text:   "orphan",
		Scope:  learning.ScopeTicket,
		Ticket: "BUG-999",
	})
	if err == nil {
		t.Fatal("expected error for missing ticket")
	}
}

func TestCreateLearningRequiresText(t *testing.T) {
	s := scaffold(t)
	_, err := s.CreateLearning(CreateLearningReq{Scope: learning.ScopeGlobal})
	if err == nil {
		t.Fatal("expected error for empty text")
	}
}

func TestListLearningsFilter(t *testing.T) {
	s := scaffold(t)
	_, err := s.CreateLearning(CreateLearningReq{Text: "a", Tags: []string{"db"}})
	must(t, err)
	_, err = s.CreateLearning(CreateLearningReq{Text: "b", Tags: []string{"ui"}})
	must(t, err)
	got := s.ListLearnings(LearningFilter{Tags: []string{"db"}})
	if len(got) != 1 || !strings.Contains(got[0].Body, "a") {
		t.Fatalf("filter tags: got %d %#v", len(got), got)
	}
}

func TestScanLearningsOnOpen(t *testing.T) {
	s := scaffold(t)
	l, err := s.CreateLearning(CreateLearningReq{Text: "persisted insight"})
	must(t, err)
	s2, err := Open(s.Root())
	must(t, err)
	got, err := s2.GetLearning(l.ID)
	must(t, err)
	if !strings.Contains(got.Body, "persisted insight") {
		t.Errorf("body = %q", got.Body)
	}
}

func TestLearningSequentialIDs(t *testing.T) {
	s := scaffold(t) // sequential
	a, err := s.CreateLearning(CreateLearningReq{Text: "one"})
	must(t, err)
	b, err := s.CreateLearning(CreateLearningReq{Text: "two"})
	must(t, err)
	if a.ID != "LRN-001" || b.ID != "LRN-002" {
		t.Fatalf("ids = %s, %s", a.ID, b.ID)
	}
	if !ticket.ValidID(a.ID) {
		t.Errorf("invalid id %q", a.ID)
	}
}

func TestCreateLearningSupersedes(t *testing.T) {
	s := scaffold(t)
	old, err := s.CreateLearning(CreateLearningReq{Text: "old rule"})
	must(t, err)
	neu, err := s.CreateLearning(CreateLearningReq{Text: "new rule", Supersedes: old.ID})
	must(t, err)
	if neu.Supersedes != old.ID {
		t.Fatalf("supersedes = %q", neu.Supersedes)
	}
	active := s.ListLearnings(LearningFilter{})
	if len(active) != 1 || active[0].ID != neu.ID {
		t.Fatalf("default list should hide superseded: %#v", active)
	}
	all := s.ListLearnings(LearningFilter{IncludeSuperseded: true})
	if len(all) != 2 {
		t.Fatalf("include-superseded want 2, got %d", len(all))
	}
}

func TestCreateLearningCitesAndStaleFilter(t *testing.T) {
	s := scaffold(t)
	repoRoot := filepath.Dir(s.Root())
	cited := filepath.Join(repoRoot, "internal", "foo.go")
	must(t, os.MkdirAll(filepath.Dir(cited), 0o755))
	must(t, os.WriteFile(cited, []byte("package foo\n"), 0o644))

	l, err := s.CreateLearning(CreateLearningReq{
		Text:  "foo has a race",
		Cites: []string{"internal/foo.go"},
	})
	must(t, err)
	if len(l.Cites) != 1 || l.Cites[0] != "internal/foo.go" {
		t.Fatalf("cites = %v", l.Cites)
	}
	active := s.ListLearnings(LearningFilter{})
	if len(active) != 1 || active[0].ID != l.ID {
		t.Fatalf("valid cites should list: %#v", active)
	}

	must(t, os.Remove(cited))
	hidden := s.ListLearnings(LearningFilter{})
	if len(hidden) != 0 {
		t.Fatalf("default list should hide citation-stale: %#v", hidden)
	}
	shown := s.ListLearnings(LearningFilter{IncludeStale: true})
	if len(shown) != 1 || shown[0].ID != l.ID {
		t.Fatalf("include-stale want 1, got %#v", shown)
	}
}

func TestCreateLearningAbsentCitesNoRegression(t *testing.T) {
	s := scaffold(t)
	l, err := s.CreateLearning(CreateLearningReq{Text: "no cites"})
	must(t, err)
	if len(l.Cites) != 0 {
		t.Fatalf("cites = %v", l.Cites)
	}
	if len(s.ListLearnings(LearningFilter{})) != 1 {
		t.Fatal("learning without cites should list normally")
	}
}

func TestReloadLearning(t *testing.T) {
	s := scaffold(t)
	l, err := s.CreateLearning(CreateLearningReq{Text: "original insight"})
	must(t, err)
	path := filepath.Join(s.Root(), "learnings", l.ID+".md")

	// Echo of same content → no change.
	ch, err := s.ReloadLearning(path)
	must(t, err)
	if ch.Changed {
		t.Fatal("same content should not report Changed")
	}

	raw, _ := os.ReadFile(path)
	updated := strings.Replace(string(raw), "original insight", "edited insight", 1)
	must(t, os.WriteFile(path, []byte(updated), 0o644))
	ch, err = s.ReloadLearning(path)
	must(t, err)
	if !ch.Changed {
		t.Fatal("edited content should report Changed")
	}
	got, err := s.GetLearning(l.ID)
	must(t, err)
	if !strings.Contains(got.Body, "edited insight") {
		t.Fatalf("body = %q", got.Body)
	}

	must(t, os.Remove(path))
	ch, err = s.ReloadLearning(path)
	must(t, err)
	if !ch.Removed || !ch.Changed {
		t.Fatalf("delete should remove: %+v", ch)
	}
	if _, err := s.GetLearning(l.ID); err == nil {
		t.Fatal("expected not found after remove")
	}
}

func TestCreateLearningTicketRequired(t *testing.T) {
	s := scaffold(t)
	_, err := s.CreateLearning(CreateLearningReq{Text: "orphan", Scope: learning.ScopeTicket})
	if err == nil || !strings.Contains(err.Error(), "--ticket is required") {
		t.Fatalf("expected --ticket-required error, got %v", err)
	}
}

func TestCreateLearningGlobalIgnoresTicket(t *testing.T) {
	s := scaffold(t)
	tk, err := s.Create(CreateReq{Type: "bug", Title: "unrelated"})
	must(t, err)
	l, err := s.CreateLearning(CreateLearningReq{Text: "global note", Scope: learning.ScopeGlobal, Ticket: tk.ID})
	must(t, err)
	if l.Ticket != "" {
		t.Fatalf("global-scoped learning should ignore --ticket, got %q", l.Ticket)
	}
}

func TestCreateLearningRejectsPathTraversalCite(t *testing.T) {
	s := scaffold(t)
	_, err := s.CreateLearning(CreateLearningReq{Text: "x", Cites: []string{"../../../etc/passwd"}})
	if err == nil || !strings.Contains(err.Error(), "..") {
		t.Fatalf("expected path-traversal rejection, got %v", err)
	}
	_, err = s.CreateLearning(CreateLearningReq{Text: "x", Cites: []string{"/etc/passwd"}})
	if err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("expected absolute-path rejection, got %v", err)
	}
}

func TestCiteExistsRejectsDirectory(t *testing.T) {
	s := scaffold(t)
	repoRoot := filepath.Dir(s.Root())
	dir := filepath.Join(repoRoot, "internal", "adir")
	must(t, os.MkdirAll(dir, 0o755))
	if s.CiteExists("internal/adir") {
		t.Fatal("a cited directory must not be treated as an existing citation")
	}
}

func TestLearningFilterScopeCaseInsensitive(t *testing.T) {
	s := scaffold(t)
	_, err := s.CreateLearning(CreateLearningReq{Text: "x", Scope: "GLOBAL"})
	must(t, err)
	got := s.ListLearnings(LearningFilter{Scope: "Global"})
	if len(got) != 1 {
		t.Fatalf("case-insensitive scope filter: got %d, want 1", len(got))
	}
}

func TestConcurrentCreateLearningUniqueIDs(t *testing.T) {
	s := scaffold(t)
	const n = 20
	var wg sync.WaitGroup
	ids := make(chan string, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if l, err := s.CreateLearning(CreateLearningReq{Text: "x"}); err == nil {
				ids <- l.ID
			}
		}()
	}
	wg.Wait()
	close(ids)
	seen := map[string]bool{}
	for id := range ids {
		if seen[id] {
			t.Errorf("duplicate learning id %s", id)
		}
		seen[id] = true
	}
	if len(seen) != n {
		t.Errorf("got %d unique learning ids, want %d", len(seen), n)
	}
}

func TestPersistedSelfLoopSupersedeHiddenFromList(t *testing.T) {
	s := scaffold(t)
	path := filepath.Join(s.Root(), "learnings", "LRN-005.md")
	must(t, os.MkdirAll(filepath.Dir(path), 0o755))
	must(t, os.WriteFile(path, []byte(`---
id: LRN-005
scope: global
source_agent: manual
supersedes: LRN-005
created: 2026-07-11T00:00:00Z
---
self-referential
`), 0o644))
	s2, err := Open(s.Root())
	must(t, err)
	if got := s2.ListLearnings(LearningFilter{}); len(got) != 0 {
		t.Fatalf("self-superseding learning should be hidden from the default list: %#v", got)
	}
	all := s2.AllLearnings()
	if len(all) != 1 || all[0].ID != "LRN-005" {
		t.Fatalf("AllLearnings should still include it: %#v", all)
	}
}

func TestCreateLearningSupersedesMissing(t *testing.T) {
	s := scaffold(t)
	_, err := s.CreateLearning(CreateLearningReq{Text: "x", Supersedes: "LRN-999"})
	if err == nil {
		t.Fatal("expected missing supersedes target error")
	}
}

func TestCreateLearningSupersedesSelfCycle(t *testing.T) {
	s := scaffold(t)
	a, err := s.CreateLearning(CreateLearningReq{Text: "a"})
	must(t, err)
	b, err := s.CreateLearning(CreateLearningReq{Text: "b", Supersedes: a.ID})
	must(t, err)
	// Plant reverse edge on disk: A supersedes B, then creating C→A is fine,
	// but CreateLearning that would close B→A when A already →B is via hand edit:
	// set A.supersedes=B then try CreateLearning superseding A (extends chain — no cycle).
	// Real write-time cycle: A→B on disk, create with Supersedes=A after editing B to
	// point nowhere and A to point at the *new* id is hard. Instead: A supersedes B
	// on disk while B supersedes A — FindCycles covered above; for CreateLearning:
	path := filepath.Join(s.Root(), "learnings", a.ID+".md")
	raw, _ := os.ReadFile(path)
	updated := strings.Replace(string(raw), "source_agent:", "supersedes: "+b.ID+"\nsource_agent:", 1)
	os.WriteFile(path, []byte(updated), 0o644)
	s2, err := Open(s.Root())
	must(t, err)
	// Closing the cycle via a new learning that supersedes A, where A→B→A already
	// exists, isn't a new edge cycle involving the new id. Test self-ref refusal
	// by creating then attempting supersedes of own reserved id isn't exposed.
	// Plant: only B exists pointing at A. Edit A to supersede B. New create C→A OK.
	// Write-time: reopen and try CreateLearning with Supersedes equal to a node that
	// already transitively supersedes the new id — impossible for fresh id.
	// Use WouldCycle via Create after forcing edges A→B and request create with
	// supersedes A when we've set B's file to supersede the *next* sequential id.
	// Simpler: sequential next is LRN-003; write B.supersedes=LRN-003, A.supersedes=B,
	// then CreateLearning supersedes A → WouldCycle(003→A→B→003).
	pathB := filepath.Join(s2.Root(), "learnings", b.ID+".md")
	rawB, _ := os.ReadFile(pathB)
	// b already supersedes a; change to supersede future LRN-003
	rawB2 := strings.Replace(string(rawB), "supersedes: "+a.ID, "supersedes: LRN-003", 1)
	os.WriteFile(pathB, []byte(rawB2), 0o644)
	// a supersedes b
	rawA, _ := os.ReadFile(filepath.Join(s2.Root(), "learnings", a.ID+".md"))
	if !strings.Contains(string(rawA), "supersedes: "+b.ID) {
		t.Fatal("expected a to supersede b from earlier write")
	}
	s3, err := Open(s2.Root())
	must(t, err)
	_, err = s3.CreateLearning(CreateLearningReq{Text: "c closes cycle", Supersedes: a.ID})
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected cycle refusal, got %v", err)
	}
}

func TestDeleteLearning(t *testing.T) {
	s := scaffold(t)
	l, err := s.CreateLearning(CreateLearningReq{Text: "to be removed"})
	must(t, err)
	path := filepath.Join(s.Root(), "learnings", l.ID+".md")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file should exist before delete: %v", err)
	}
	if err := s.DeleteLearning(l.ID); err != nil {
		t.Fatalf("DeleteLearning: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("file should be gone after delete, stat err = %v", err)
	}
	if _, err := s.GetLearning(l.ID); err != ErrNotFound {
		t.Errorf("GetLearning after delete = %v, want ErrNotFound", err)
	}
	if err := s.DeleteLearning("LRN-999"); err != ErrNotFound {
		t.Errorf("delete of missing id = %v, want ErrNotFound", err)
	}
}

func TestUpdateLearning(t *testing.T) {
	s := scaffold(t)
	l, err := s.CreateLearning(CreateLearningReq{Text: "original", Tags: []string{"db"}})
	must(t, err)
	got, err := s.UpdateLearning(l.ID, func(m *learning.Learning) error {
		m.Tags = []string{"db", "cache"}
		m.Body = "\nrewritten body\n"
		return nil
	})
	must(t, err)
	if len(got.Tags) != 2 || got.Tags[1] != "cache" {
		t.Errorf("tags = %v", got.Tags)
	}
	// Persisted to disk and reloadable.
	reopened, err := Open(s.Root())
	must(t, err)
	rl, err := reopened.GetLearning(l.ID)
	must(t, err)
	if !strings.Contains(rl.Body, "rewritten body") {
		t.Errorf("body not persisted: %q", rl.Body)
	}
	if _, err := s.UpdateLearning("LRN-999", func(*learning.Learning) error { return nil }); err != ErrNotFound {
		t.Errorf("update of missing id = %v, want ErrNotFound", err)
	}
}

func TestCreateLearningComponentScope(t *testing.T) {
	s := scaffold(t)
	l, err := s.CreateLearning(CreateLearningReq{
		Text:      "the store is the single write path",
		Scope:     learning.ScopeComponent,
		Component: "internal/store",
	})
	must(t, err)
	if l.Scope != learning.ScopeComponent || l.Component != "internal/store" {
		t.Errorf("scope/component = %q/%q", l.Scope, l.Component)
	}
	// Round-trips through disk.
	reopened, err := Open(s.Root())
	must(t, err)
	rl, err := reopened.GetLearning(l.ID)
	must(t, err)
	if rl.Component != "internal/store" {
		t.Errorf("component not persisted: %q", rl.Component)
	}
	// Filterable by component.
	got := reopened.ListLearnings(LearningFilter{Component: "internal/store"})
	if len(got) != 1 || got[0].ID != l.ID {
		t.Errorf("component filter returned %v", got)
	}
	if other := reopened.ListLearnings(LearningFilter{Component: "internal/cli"}); len(other) != 0 {
		t.Errorf("component filter should exclude non-matching, got %v", other)
	}
}

func TestCreateLearningComponentRequired(t *testing.T) {
	s := scaffold(t)
	_, err := s.CreateLearning(CreateLearningReq{Text: "x", Scope: learning.ScopeComponent})
	if err == nil || !strings.Contains(err.Error(), "component") {
		t.Fatalf("expected component-required error, got %v", err)
	}
}

func TestCreateLearningNonComponentClearsComponent(t *testing.T) {
	s := scaffold(t)
	l, err := s.CreateLearning(CreateLearningReq{Text: "x", Scope: learning.ScopeGlobal, Component: "internal/store"})
	must(t, err)
	if l.Component != "" {
		t.Errorf("global learning should not carry a component, got %q", l.Component)
	}
}
