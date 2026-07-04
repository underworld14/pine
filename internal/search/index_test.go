package search

import (
	"strings"
	"testing"
)

func buildIndex(t *testing.T) *Index {
	t.Helper()
	idx, err := New()
	if err != nil {
		t.Fatal(err)
	}
	docs := []Doc{
		{ID: "BUG-001", Title: "Login button not working", Body: "click does nothing on the sign in page",
			Labels: []string{"login", "ui"}, RelatedFiles: "src/login.tsx", Status: "todo", Priority: "high", Type: "BUG"},
		{ID: "FEAT-002", Title: "Dark mode toggle", Body: "add a theme switcher",
			Labels: []string{"ui"}, Status: "done", Priority: "low", Type: "FEAT"},
		{ID: "BUG-003", Title: "Payment retry fails", Body: "stripe returns an error on retry",
			RelatedFiles: "src/pay.ts", Status: "doing", Priority: "critical", Type: "BUG"},
	}
	for _, d := range docs {
		idx.Upsert(d)
	}
	idx.ready.Store(true)
	return idx
}

func topID(hits []Hit) string {
	if len(hits) == 0 {
		return ""
	}
	return hits[0].ID
}

func hasID(hits []Hit, id string) bool {
	for _, h := range hits {
		if h.ID == id {
			return true
		}
	}
	return false
}

func TestSearchByTitleHighlights(t *testing.T) {
	idx := buildIndex(t)
	hits := idx.Search("login", Filter{}, 10)
	if topID(hits) != "BUG-001" {
		t.Fatalf("top = %q, want BUG-001 (%d hits)", topID(hits), len(hits))
	}
	frags := hits[0].Fragments["title"]
	joined := strings.Join(frags, " ")
	if !strings.Contains(joined, "<mark>") {
		t.Errorf("expected <mark> highlight in title fragment: %q", joined)
	}
}

func TestSearchByIDExact(t *testing.T) {
	idx := buildIndex(t)
	hits := idx.Search("BUG-003", Filter{}, 10)
	if topID(hits) != "BUG-003" {
		t.Errorf("top = %q, want BUG-003", topID(hits))
	}
}

func TestSearchByFilePath(t *testing.T) {
	idx := buildIndex(t)
	hits := idx.Search("login.tsx", Filter{}, 10)
	if !hasID(hits, "BUG-001") {
		t.Errorf("file-path search should find BUG-001: %+v", hits)
	}
}

func TestSearchByLabel(t *testing.T) {
	idx := buildIndex(t)
	hits := idx.Search("ui", Filter{}, 10)
	if !hasID(hits, "BUG-001") || !hasID(hits, "FEAT-002") {
		t.Errorf("label search should find both ui-labeled tickets: %+v", hits)
	}
}

func TestSearchWithFilter(t *testing.T) {
	idx := buildIndex(t)
	hits := idx.Search("retry", Filter{Type: "BUG"}, 10)
	if !hasID(hits, "BUG-003") {
		t.Errorf("expected BUG-003: %+v", hits)
	}
	// A mismatched filter yields nothing.
	none := idx.Search("retry", Filter{Type: "FEAT"}, 10)
	if len(none) != 0 {
		t.Errorf("type filter should exclude BUG-003: %+v", none)
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	idx := buildIndex(t)
	if hits := idx.Search("", Filter{}, 10); hits != nil {
		t.Errorf("empty query should return nil, got %+v", hits)
	}
}
