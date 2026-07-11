package server

import (
	"bufio"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/underworld14/pine/internal/config"
	"github.com/underworld14/pine/internal/store"
	"github.com/underworld14/pine/internal/watch"
)

// TestLiveSyncExternalEdit is the core M4 contract: an edit made directly to a
// ticket file on disk (as an AI agent or the CLI would) is pushed to connected
// SSE clients as a filesystem-origin event.
func TestLiveSyncExternalEdit(t *testing.T) {
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

	st, err := store.Open(pine)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Create(store.CreateReq{Type: "bug", Title: "x"}); err != nil {
		t.Fatal(err)
	}

	srv := New(st, "test")
	stop := srv.StartLiveSync()
	defer stop()
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Connect to the SSE stream.
	resp, err := http.Get(ts.URL + "/api/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	lines := make(chan string, 64)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			lines <- sc.Text()
		}
	}()

	// Wait for the initial ": connected" so we know we are subscribed.
	if !waitFor(lines, "connected", 3*time.Second) {
		t.Fatal("never received SSE connect")
	}

	// Externally edit the ticket file on disk.
	path := filepath.Join(pine, "tickets", "BUG-001.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(raw), "status: todo", "status: done", 1)
	if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}

	// Expect a filesystem-origin ticket.updated event.
	if !waitFor(lines, `"source":"fs"`, 5*time.Second) {
		t.Fatal("no filesystem-origin SSE event after external edit")
	}
}

// newLiveSyncStore builds a bare store + Server (no HTTP server) for tests
// that drive applyWatchBatch/applyWatchEvent directly rather than through the
// real filesystem watcher.
func newLiveSyncStore(t *testing.T) (*store.Store, *Server, string) {
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
	st, err := store.Open(pine)
	if err != nil {
		t.Fatal(err)
	}
	return st, New(st, "test"), pine
}

// recvEvent waits for the next hub message, failing the test on timeout.
func recvEvent(t *testing.T, ch chan sseMsg) sseMsg {
	t.Helper()
	select {
	case msg := <-ch:
		return msg
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for hub event")
		return sseMsg{}
	}
}

// TestApplyWatchEventTicketUpdated calls applyWatchEvent directly (bypassing
// the real fs watcher) for an on-disk edit, and confirms it delegates to
// applyWatchBatch: the store cache picks up the change and a ticket.updated
// event is broadcast.
func TestApplyWatchEventTicketUpdated(t *testing.T) {
	st, srv, pine := newLiveSyncStore(t)
	tk, err := st.Create(store.CreateReq{Type: "bug", Title: "x"})
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(pine, "tickets", tk.ID+".md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(raw), "status: todo", "status: done", 1)
	if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}

	ch := srv.hub.subscribe()
	defer srv.hub.unsubscribe(ch)

	srv.applyWatchEvent(watch.Event{Kind: watch.KindTicket, Path: path, ID: tk.ID})

	got, err := st.Get(tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "done" {
		t.Fatalf("status = %q, want done", got.Status)
	}
	msg := recvEvent(t, ch)
	if msg.event != "ticket.updated" {
		t.Errorf("event = %q, want ticket.updated", msg.event)
	}
	if !strings.Contains(string(msg.data), `"source":"fs"`) {
		t.Errorf("expected fs-origin event: %s", msg.data)
	}
}

// TestApplyWatchEventTicketRemoved covers the ch.Removed branch: the ticket
// file disappears on disk (e.g. deleted by an external tool), and the server
// must evict it from the store/index and broadcast ticket.deleted.
func TestApplyWatchEventTicketRemoved(t *testing.T) {
	st, srv, pine := newLiveSyncStore(t)
	tk, err := st.Create(store.CreateReq{Type: "bug", Title: "x"})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(pine, "tickets", tk.ID+".md")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	ch := srv.hub.subscribe()
	defer srv.hub.unsubscribe(ch)

	srv.applyWatchEvent(watch.Event{Kind: watch.KindTicket, Path: path, ID: tk.ID})

	if _, err := st.Get(tk.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after removal, got %v", err)
	}
	msg := recvEvent(t, ch)
	if msg.event != "ticket.deleted" {
		t.Errorf("event = %q, want ticket.deleted", msg.event)
	}
}

// TestApplyWatchBatchConfigAndBoard covers the KindConfig and KindBoard
// branches: an external edit to config.json/board.json is reloaded into the
// store and broadcast.
func TestApplyWatchBatchConfigAndBoard(t *testing.T) {
	st, srv, pine := newLiveSyncStore(t)

	// Build fresh config/board objects rather than mutating the ones returned
	// by st.Config()/st.Board() (those are the store's live pointers, so
	// editing them in place would make ReloadConfig/ReloadBoard's before/after
	// comparison see no diff at all).
	cfg := config.Default("test")
	cfg.IDStyle = "sequential"
	cfg.Project.Name = "renamed"
	cfgB, err := cfg.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(pine, "config.json")
	if err := os.WriteFile(cfgPath, cfgB, 0o644); err != nil {
		t.Fatal(err)
	}

	board := config.DefaultBoard()
	board.Columns[0].Title = "Renamed Column"
	boardB, err := board.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	boardPath := filepath.Join(pine, "board.json")
	if err := os.WriteFile(boardPath, boardB, 0o644); err != nil {
		t.Fatal(err)
	}

	ch := srv.hub.subscribe()
	defer srv.hub.unsubscribe(ch)

	srv.applyWatchBatch([]watch.Event{
		{Kind: watch.KindConfig, Path: cfgPath},
		{Kind: watch.KindBoard, Path: boardPath},
	})

	if st.Config().Project.Name != "renamed" {
		t.Errorf("config was not reloaded: %+v", st.Config().Project)
	}
	if st.Board().Columns[0].Title != "Renamed Column" {
		t.Errorf("board was not reloaded: %+v", st.Board())
	}

	seen := map[string]bool{}
	for i := 0; i < 2; i++ {
		msg := recvEvent(t, ch)
		seen[msg.event] = true
	}
	if !seen["config.updated"] || !seen["board.updated"] {
		t.Errorf("expected config.updated and board.updated events, got %+v", seen)
	}
}

// TestApplyWatchBatchLearning covers the KindLearning branch: it must refresh
// the learning cache without emitting any SSE event (learnings are not
// rendered by the FE, per livesync.go's documented scope boundary).
func TestApplyWatchBatchLearning(t *testing.T) {
	st, srv, _ := newLiveSyncStore(t)
	l, err := st.CreateLearning(store.CreateLearningReq{Text: "original text"})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(st.Root(), "learnings", l.ID+".md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(raw), "original text", "updated text", 1)
	if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}

	ch := srv.hub.subscribe()
	defer srv.hub.unsubscribe(ch)

	srv.applyWatchBatch([]watch.Event{{Kind: watch.KindLearning, Path: path, ID: l.ID}})

	select {
	case msg := <-ch:
		t.Fatalf("expected no SSE event for a learning change, got %q", msg.event)
	case <-time.After(200 * time.Millisecond):
	}
}

func waitFor(lines <-chan string, substr string, timeout time.Duration) bool {
	deadline := time.After(timeout)
	for {
		select {
		case line := <-lines:
			if strings.Contains(line, substr) {
				return true
			}
		case <-deadline:
			return false
		}
	}
}
