package store

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/izzadev/pine/internal/config"
	"github.com/izzadev/pine/internal/ticket"
)

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// scaffold creates a fresh .pine/ tree and opens a store over it.
func scaffold(t *testing.T) *Store {
	t.Helper()
	pine := filepath.Join(t.TempDir(), ".pine")
	must(t, os.MkdirAll(filepath.Join(pine, "tickets"), 0o755))
	cfgB, err := config.Default("test").Bytes()
	must(t, err)
	must(t, os.WriteFile(filepath.Join(pine, "config.json"), cfgB, 0o644))
	bB, err := config.DefaultBoard().Bytes()
	must(t, err)
	must(t, os.WriteFile(filepath.Join(pine, "board.json"), bB, 0o644))
	s, err := Open(pine)
	must(t, err)
	return s
}

// steppingClock returns times that advance one second per call.
func steppingClock() func() time.Time {
	base := time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)
	i := 0
	return func() time.Time {
		i++
		return base.Add(time.Duration(i) * time.Second)
	}
}

func TestCreateSequentialIDs(t *testing.T) {
	s := scaffold(t)
	a, err := s.Create(CreateReq{Type: "bug", Title: "one"})
	must(t, err)
	b, err := s.Create(CreateReq{Type: "bug", Title: "two"})
	must(t, err)
	if a.ID != "BUG-001" || b.ID != "BUG-002" {
		t.Fatalf("ids = %s, %s", a.ID, b.ID)
	}
	if _, err := os.Stat(s.ticketPath("BUG-001")); err != nil {
		t.Errorf("file not written: %v", err)
	}
}

func TestCreateUsesTemplateAndTypeName(t *testing.T) {
	s := scaffold(t)
	f, err := s.Create(CreateReq{Type: "feature", Title: "x"})
	must(t, err)
	if f.ID != "FEAT-001" {
		t.Errorf("id = %s", f.ID)
	}
	if !strings.Contains(f.Body, "Acceptance Criteria") {
		t.Errorf("feature template not applied: %q", f.Body)
	}
	e, err := s.Create(CreateReq{Type: "epic", Title: "e"})
	must(t, err)
	if e.ID != "EPIC-001" || !strings.Contains(e.Body, "Goals") {
		t.Errorf("epic create = %s / %q", e.ID, e.Body)
	}
}

func TestCreateUnknownType(t *testing.T) {
	s := scaffold(t)
	if _, err := s.Create(CreateReq{Type: "zzz", Title: "x"}); err != ErrUnknownType {
		t.Errorf("err = %v", err)
	}
}

func TestUpdatePreservesBodyAndBumpsUpdated(t *testing.T) {
	s := scaffold(t)
	s.SetClock(steppingClock())
	tk, err := s.Create(CreateReq{Type: "bug", Title: "x", Body: "# Description\n\nhello\n"})
	must(t, err)
	origBody := tk.Body
	updated, err := s.Update(tk.ID, func(u *ticket.Ticket) error {
		u.Status = "done"
		return nil
	})
	must(t, err)
	if updated.Status != "done" {
		t.Errorf("status = %s", updated.Status)
	}
	if updated.Body != origBody {
		t.Errorf("body changed: %q vs %q", updated.Body, origBody)
	}
	if !updated.Updated.After(tk.Updated) {
		t.Errorf("updated not bumped: %v vs %v", updated.Updated, tk.Updated)
	}
}

func TestUpdateDegradedRejected(t *testing.T) {
	s := scaffold(t)
	// Write a malformed ticket file directly and reload it.
	must(t, os.WriteFile(s.ticketPath("BUG-001"), []byte("no frontmatter here\n"), 0o644))
	_, err := s.ReloadTicket(s.ticketPath("BUG-001"))
	must(t, err)
	if _, err := s.Update("BUG-001", func(u *ticket.Ticket) error { return nil }); err != ErrDegraded {
		t.Errorf("err = %v want ErrDegraded", err)
	}
}

func TestDeleteRemovesTicketAndAttachments(t *testing.T) {
	s := scaffold(t)
	tk, err := s.Create(CreateReq{Type: "bug", Title: "x"})
	must(t, err)
	if _, err := s.WriteAttachment(tk.ID, "a.png", []byte("img")); err != nil {
		t.Fatal(err)
	}
	if len(s.Attachments(tk.ID)) != 1 {
		t.Fatalf("expected 1 attachment")
	}
	must(t, s.Delete(tk.ID))
	if _, err := s.Get(tk.ID); err != ErrNotFound {
		t.Errorf("get err = %v", err)
	}
	if _, err := os.Stat(s.attachmentDir(tk.ID)); !os.IsNotExist(err) {
		t.Errorf("attachment dir not removed")
	}
}

func TestConcurrentCreateUniqueIDs(t *testing.T) {
	s := scaffold(t)
	const n = 20
	var wg sync.WaitGroup
	ids := make(chan string, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if tk, err := s.Create(CreateReq{Type: "bug", Title: "x"}); err == nil {
				ids <- tk.ID
			}
		}()
	}
	wg.Wait()
	close(ids)
	seen := map[string]bool{}
	for id := range ids {
		if seen[id] {
			t.Errorf("duplicate id %s", id)
		}
		seen[id] = true
	}
	if len(seen) != n {
		t.Errorf("got %d unique ids, want %d", len(seen), n)
	}
}

func TestCreateSkipsExistingID(t *testing.T) {
	s := scaffold(t)
	// Simulate an externally-created ticket file occupying BUG-001.
	must(t, os.WriteFile(s.ticketPath("BUG-001"), []byte("x"), 0o644))
	tk, err := s.Create(CreateReq{Type: "bug", Title: "x"})
	must(t, err)
	if tk.ID == "BUG-001" {
		t.Errorf("should not reuse an existing id")
	}
}

func TestReloadTicketDedupe(t *testing.T) {
	s := scaffold(t)
	tk, err := s.Create(CreateReq{Type: "bug", Title: "x"})
	must(t, err)
	raw, err := os.ReadFile(s.ticketPath(tk.ID))
	must(t, err)
	edited := strings.Replace(string(raw), "status: todo", "status: done", 1)
	must(t, os.WriteFile(s.ticketPath(tk.ID), []byte(edited), 0o644))

	ch, err := s.ReloadTicket(s.ticketPath(tk.ID))
	must(t, err)
	if !ch.Changed {
		t.Errorf("expected change on external edit")
	}
	got, _ := s.Get(tk.ID)
	if got.Status != "done" {
		t.Errorf("status = %s", got.Status)
	}
	ch2, err := s.ReloadTicket(s.ticketPath(tk.ID))
	must(t, err)
	if ch2.Changed {
		t.Errorf("reload of unchanged file should dedupe")
	}
}

func TestHashChangesOnUpdate(t *testing.T) {
	s := scaffold(t)
	tk, err := s.Create(CreateReq{Type: "bug", Title: "x"})
	must(t, err)
	h1, _ := s.Hash(tk.ID)
	if _, err := s.Update(tk.ID, func(u *ticket.Ticket) error { u.Title = "y"; return nil }); err != nil {
		t.Fatal(err)
	}
	h2, _ := s.Hash(tk.ID)
	if h1 == h2 || h1 == "" || h2 == "" {
		t.Errorf("hash should change: %q -> %q", h1, h2)
	}
}

func TestListFilter(t *testing.T) {
	s := scaffold(t)
	if _, err := s.Create(CreateReq{Type: "bug", Title: "a", Labels: []string{"ui"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(CreateReq{Type: "feature", Title: "b"}); err != nil {
		t.Fatal(err)
	}
	if len(s.List(Filter{Type: "BUG"})) != 1 {
		t.Errorf("type filter")
	}
	if len(s.List(Filter{Label: "ui"})) != 1 {
		t.Errorf("label filter")
	}
	if len(s.All()) != 2 {
		t.Errorf("all")
	}
}

func TestAttachmentPathTraversalNeutralized(t *testing.T) {
	s := scaffold(t)
	// A traversal attempt must never resolve outside the ticket's attachment dir.
	// It may be rejected outright or collapsed to a safe basename; either is fine.
	if p, err := s.AttachmentFilePath("BUG-001", "../../etc/passwd"); err == nil {
		if !strings.HasPrefix(p, s.attachmentDir("BUG-001")+string(filepath.Separator)) {
			t.Errorf("path escaped attachments dir: %s", p)
		}
	}
	if _, err := s.AttachmentFilePath("BUG-001", "ok.png"); err != nil {
		t.Errorf("valid name rejected: %v", err)
	}
	if _, err := s.AttachmentFilePath("bad-id", "ok.png"); err == nil {
		t.Errorf("invalid ticket id should be rejected")
	}
}
