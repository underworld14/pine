package server

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/underworld14/pine/internal/config"
	"github.com/underworld14/pine/internal/store"
)

// TestLiveSyncExternalEdit is the core M4 contract: an edit made directly to a
// ticket file on disk (as an AI agent or the CLI would) is pushed to connected
// SSE clients as a filesystem-origin event.
func TestLiveSyncExternalEdit(t *testing.T) {
	pine := filepath.Join(t.TempDir(), ".pine")
	if err := os.MkdirAll(filepath.Join(pine, "tickets"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfgB, _ := config.Default("test").Bytes()
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
