package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/underworld14/pine/internal/config"
	"github.com/underworld14/pine/internal/store"
)

func acBody(id string) string {
	return "---\nid: " + id + "\ntitle: " + id + "\nstatus: todo\nupdated: 2026-07-01T10:00:00Z\n---\n\n# Acceptance Criteria\n- [ ] one\n- [ ] two\n"
}

func newLocalServer(t *testing.T, id, body string) *httptest.Server {
	t.Helper()
	repo := t.TempDir()
	pine := filepath.Join(repo, ".pine")
	if err := os.MkdirAll(filepath.Join(pine, "tickets"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default("test")
	b, _ := cfg.Bytes()
	os.WriteFile(filepath.Join(pine, "config.json"), b, 0o644)
	bb, _ := config.DefaultBoard().Bytes()
	os.WriteFile(filepath.Join(pine, "board.json"), bb, 0o644)
	os.WriteFile(filepath.Join(pine, "tickets", id+".md"), []byte(body), 0o644)
	st, err := store.Open(pine)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(New(st, "test").Handler())
	t.Cleanup(ts.Close)
	return ts
}

func hashOf(t *testing.T, jsonBody string) string {
	t.Helper()
	var m map[string]any
	json.Unmarshal([]byte(jsonBody), &m)
	h, _ := m["hash"].(string)
	return h
}

func TestChecklistToggle(t *testing.T) {
	ts := newLocalServer(t, "BUG-0a1b2c", acBody("BUG-0a1b2c"))

	_, gb := do(t, "GET", ts.URL+"/api/tickets/BUG-0a1b2c", "", nil)
	hash := hashOf(t, gb)

	resp, body := do(t, "PATCH", ts.URL+"/api/tickets/BUG-0a1b2c/checklist",
		`{"index":0,"checked":true}`,
		map[string]string{"Content-Type": "application/json", "If-Match": `"` + hash + `"`})
	if resp.StatusCode != 200 {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, `"acceptance":{"done":1,"total":2}`) {
		t.Errorf("expected 1/2 progress: %s", body)
	}
	if !strings.Contains(body, "- [x] one") {
		t.Errorf("body should show first box checked: %s", body)
	}

	if r, _ := do(t, "PATCH", ts.URL+"/api/tickets/BUG-0a1b2c/checklist",
		`{"index":9,"checked":true}`, map[string]string{"Content-Type": "application/json"}); r.StatusCode != 400 {
		t.Errorf("bad index want 400, got %d", r.StatusCode)
	}
}

func TestChecklistOffBranchRejected(t *testing.T) {
	ts := newCrossBranchServer(t, "hash")
	r, body := do(t, "PATCH", ts.URL+"/api/tickets/FEAT-3d4e5f/checklist",
		`{"index":0,"checked":true}`, map[string]string{"Content-Type": "application/json"})
	if r.StatusCode != http.StatusConflict || !strings.Contains(body, "off_branch") {
		t.Errorf("want 409 off_branch, got %d: %s", r.StatusCode, body)
	}
}
