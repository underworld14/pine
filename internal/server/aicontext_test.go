package server

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/underworld14/pine/internal/config"
	"github.com/underworld14/pine/internal/store"
)

func TestHandleContext(t *testing.T) {
	ts := newTestServer(t)
	do(t, "POST", ts.URL+"/api/tickets", `{"type":"bug","title":"Context Ticket","priority":"critical"}`, nil)

	resp, body := do(t, "GET", ts.URL+"/api/context", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/markdown; charset=utf-8" {
		t.Errorf("content-type = %q", ct)
	}
	if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("missing nosniff header")
	}
	if !strings.Contains(body, "# Project Context: test") {
		t.Errorf("body missing project header: %s", body)
	}
	if !strings.Contains(body, "BUG-001") {
		t.Errorf("body should mention the critical ticket: %s", body)
	}
}

func TestHandlePrompt(t *testing.T) {
	ts := newTestServer(t)
	do(t, "POST", ts.URL+"/api/tickets", `{"type":"bug","title":"Prompt Ticket","priority":"high"}`, nil)

	resp, body := do(t, "GET", ts.URL+"/api/tickets/BUG-001/prompt", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/markdown; charset=utf-8" {
		t.Errorf("content-type = %q", ct)
	}
	if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("missing nosniff header")
	}
	if !strings.Contains(body, "# Fix Request: BUG-001") {
		t.Errorf("body missing fix request header: %s", body)
	}
	if !strings.Contains(body, "Prompt Ticket") {
		t.Errorf("body should include the ticket title: %s", body)
	}
}

// TestHandlePromptImplementTemplate checks the web prompt endpoint picks
// prompts/implement.md for non-bug tickets when the file exists, mirroring the
// CLI's type-based selection.
func TestHandlePromptImplementTemplate(t *testing.T) {
	pine := filepath.Join(t.TempDir(), ".pine")
	os.MkdirAll(filepath.Join(pine, "tickets"), 0o755)
	os.MkdirAll(filepath.Join(pine, "prompts"), 0o755)
	cfg := config.Default("test")
	cfg.IDStyle = "sequential"
	cfgB, _ := cfg.Bytes()
	os.WriteFile(filepath.Join(pine, "config.json"), cfgB, 0o644)
	bB, _ := config.DefaultBoard().Bytes()
	os.WriteFile(filepath.Join(pine, "board.json"), bB, 0o644)
	os.WriteFile(filepath.Join(pine, "prompts", "implement.md"), []byte("# Implement: {{.ID}}\n"), 0o644)
	os.WriteFile(filepath.Join(pine, "prompts", "fix.md"), []byte("# Fix Request: {{.ID}}\n"), 0o644)
	st, err := store.Open(pine)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(New(st, "test").Handler())
	t.Cleanup(ts.Close)

	do(t, "POST", ts.URL+"/api/tickets", `{"type":"feature","title":"Impl Ticket"}`, nil)
	resp, body := do(t, "GET", ts.URL+"/api/tickets/FEAT-001/prompt", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, "# Implement: FEAT-001") {
		t.Fatalf("feature prompt should use implement.md:\n%s", body)
	}

	// Bugs keep fix.md even when implement.md exists.
	do(t, "POST", ts.URL+"/api/tickets", `{"type":"bug","title":"Bug Ticket"}`, nil)
	_, body = do(t, "GET", ts.URL+"/api/tickets/BUG-001/prompt", "", nil)
	if !strings.Contains(body, "# Fix Request: BUG-001") {
		t.Fatalf("bug prompt should use fix.md:\n%s", body)
	}
}

func TestHandlePromptUnknownTicket404(t *testing.T) {
	ts := newTestServer(t)
	resp, body := do(t, "GET", ts.URL+"/api/tickets/BUG-999/prompt", "", nil)
	if resp.StatusCode != 404 {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, "not_found") {
		t.Errorf("expected not_found code: %s", body)
	}
}
