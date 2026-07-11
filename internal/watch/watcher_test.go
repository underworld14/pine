package watch

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func waitEvent(t *testing.T, w *Watcher) []Event {
	t.Helper()
	select {
	case batch := <-w.Events():
		return batch
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for a watch event")
		return nil
	}
}

func TestWatcherClassifiesTicketAndConfig(t *testing.T) {
	pine := filepath.Join(t.TempDir(), ".pine")
	if err := os.MkdirAll(filepath.Join(pine, "tickets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(pine, "learnings"), 0o755); err != nil {
		t.Fatal(err)
	}
	w, err := New(pine)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	// Ticket write.
	os.WriteFile(filepath.Join(pine, "tickets", "BUG-001.md"), []byte("---\nid: BUG-001\n---\n"), 0o644)
	batch := waitEvent(t, w)
	if !hasKind(batch, KindTicket, "BUG-001") {
		t.Fatalf("expected KindTicket BUG-001, got %+v", batch)
	}

	// Config write.
	os.WriteFile(filepath.Join(pine, "config.json"), []byte("{}"), 0o644)
	batch = waitEvent(t, w)
	if !hasKind(batch, KindConfig, "") {
		t.Fatalf("expected KindConfig, got %+v", batch)
	}

	// Learning write.
	os.WriteFile(filepath.Join(pine, "learnings", "LRN-001.md"), []byte("---\nid: LRN-001\n---\n"), 0o644)
	batch = waitEvent(t, w)
	if !hasKind(batch, KindLearning, "LRN-001") {
		t.Fatalf("expected KindLearning LRN-001, got %+v", batch)
	}
}

func TestWatcherIgnoresTempAndDotFiles(t *testing.T) {
	pine := filepath.Join(t.TempDir(), ".pine")
	os.MkdirAll(filepath.Join(pine, "tickets"), 0o755)
	w, err := New(pine)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	os.WriteFile(filepath.Join(pine, "tickets", ".tmp-123"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(pine, "tickets", "notes.swp"), []byte("x"), 0o644)
	// A real ticket write should arrive; the ignored writes must not appear in it.
	os.WriteFile(filepath.Join(pine, "tickets", "BUG-002.md"), []byte("---\nid: BUG-002\n---\n"), 0o644)

	batch := waitEvent(t, w)
	for _, e := range batch {
		if e.Kind == KindOther {
			t.Errorf("ignored file leaked into events: %+v", e)
		}
	}
	if !hasKind(batch, KindTicket, "BUG-002") {
		t.Fatalf("expected BUG-002, got %+v", batch)
	}
}

func hasKind(batch []Event, kind Kind, id string) bool {
	for _, e := range batch {
		if e.Kind == kind && (id == "" || e.ID == id) {
			return true
		}
	}
	return false
}
