package server

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/underworld14/pine/internal/config"
	"github.com/underworld14/pine/internal/store"
)

func TestSearchEndpoint(t *testing.T) {
	ts := newTestServer(t)
	do(t, "POST", ts.URL+"/api/tickets", `{"type":"bug","title":"Login button broken","labels":["ui"]}`, nil)
	do(t, "POST", ts.URL+"/api/tickets", `{"type":"feature","title":"Dark mode"}`, nil)

	// The index builds asynchronously; poll briefly until a query matches.
	var hits []searchHit
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		_, body := do(t, "GET", ts.URL+"/api/search?q=login", "", nil)
		var resp struct {
			Indexing bool        `json:"indexing"`
			Hits     []searchHit `json:"hits"`
		}
		json.Unmarshal([]byte(body), &resp)
		if len(resp.Hits) > 0 {
			hits = resp.Hits
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(hits) == 0 || hits[0].ID != "BUG-001" {
		t.Fatalf("search did not find BUG-001: %+v", hits)
	}
	if hits[0].Title == "" {
		t.Errorf("hit should be enriched with title")
	}
}

// newTestServerWithStore is like newTestServer but also returns the
// underlying store, needed to create a learning directly since learnings have
// no REST endpoint (an intentional scope boundary, not an oversight).
func newTestServerWithStore(t *testing.T) (*httptest.Server, *store.Store) {
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
	ts := httptest.NewServer(New(st, "test").Handler())
	t.Cleanup(ts.Close)
	return ts, st
}

// TestSearchEndpointExcludesLearnings pins the intentional scope boundary
// documented in livesync.go ("FE does not render learnings"): learnings are
// never indexed into the server's persistent search, so GET /api/search must
// never surface one, even when its text matches the query. This turns that
// exclusion into a tested contract instead of an untested byproduct of
// learnings never being indexed.
func TestSearchEndpointExcludesLearnings(t *testing.T) {
	ts, st := newTestServerWithStore(t)
	do(t, "POST", ts.URL+"/api/tickets", `{"type":"bug","title":"UNIQUE_SEARCH_MARKER ticket"}`, nil)
	if _, err := st.CreateLearning(store.CreateLearningReq{
		Text: "UNIQUE_SEARCH_MARKER learning that must not leak into ticket search",
	}); err != nil {
		t.Fatal(err)
	}

	var hits []searchHit
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		_, body := do(t, "GET", ts.URL+"/api/search?q=UNIQUE_SEARCH_MARKER", "", nil)
		var resp struct {
			Indexing bool        `json:"indexing"`
			Hits     []searchHit `json:"hits"`
		}
		json.Unmarshal([]byte(body), &resp)
		if len(resp.Hits) > 0 {
			hits = resp.Hits
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(hits) != 1 || hits[0].ID != "BUG-001" {
		t.Fatalf("expected exactly one ticket hit and no learning hit, got: %+v", hits)
	}
}
