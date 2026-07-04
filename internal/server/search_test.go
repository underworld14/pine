package server

import (
	"encoding/json"
	"testing"
	"time"
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
