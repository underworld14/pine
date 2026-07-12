package search

import (
	"strconv"
	"strings"
	"testing"
	"time"
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

func TestSearchByHashID(t *testing.T) {
	idx, err := New()
	if err != nil {
		t.Fatal(err)
	}
	idx.Upsert(Doc{ID: "BUG-7f3k2a", Title: "Some ticket", Type: "BUG"})
	idx.ready.Store(true)
	// Both the exact stored casing and a lowercased prefix must find it.
	for _, q := range []string{"BUG-7f3k2a", "bug-7f3k2a"} {
		hits := idx.Search(q, Filter{}, 10)
		if topID(hits) != "BUG-7f3k2a" {
			t.Errorf("query %q: top = %q, want BUG-7f3k2a", q, topID(hits))
		}
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	idx := buildIndex(t)
	if hits := idx.Search("", Filter{}, 10); hits != nil {
		t.Errorf("empty query should return nil, got %+v", hits)
	}
}

func TestBuildAsyncThenSearch(t *testing.T) {
	idx, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()

	if idx.Ready() {
		t.Fatal("Ready should be false before BuildAsync completes")
	}

	// > batchSize (200) so BuildAsync flushes mid-loop, then a final partial batch.
	docs := make([]Doc, 0, 210)
	for i := 0; i < 209; i++ {
		docs = append(docs, Doc{
			ID:    "DOC-" + strconv.Itoa(i),
			Title: "filler document",
			Body:  "unrelated content",
			Kind:  KindMemory,
		})
	}
	docs = append(docs, Doc{
		ID: "memory/widgets.md", Title: "Widget styling", Body: "always use text-white on colored squares",
		Kind: KindMemory,
	})

	idx.BuildAsync(docs)
	deadline := time.Now().Add(5 * time.Second)
	for !idx.Ready() {
		if time.Now().After(deadline) {
			t.Fatal("BuildAsync did not become Ready")
		}
		time.Sleep(5 * time.Millisecond)
	}

	hits := idx.Search("widget", Filter{Kind: KindMemory}, 10)
	if !hasID(hits, "memory/widgets.md") {
		t.Fatalf("expected memory doc in KindMemory filter results: %+v", hits)
	}

	idx.Delete("memory/widgets.md")
	hits = idx.Search("widget", Filter{Kind: KindMemory}, 10)
	if hasID(hits, "memory/widgets.md") {
		t.Fatalf("deleted doc should not appear: %+v", hits)
	}
}

func TestSearchFilterKindMemory(t *testing.T) {
	idx, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	idx.Upsert(Doc{ID: "BUG-1", Title: "widget bug", Body: "broken widget", Kind: KindTicket, Type: "BUG"})
	idx.Upsert(Doc{ID: "MEMORY.md", Title: "Project memory", Body: "widget preference", Kind: KindMemory})
	idx.ready.Store(true)

	memHits := idx.Search("widget", Filter{Kind: KindMemory}, 10)
	if !hasID(memHits, "MEMORY.md") || hasID(memHits, "BUG-1") {
		t.Fatalf("KindMemory filter: %+v", memHits)
	}
	ticketHits := idx.Search("widget", Filter{Kind: KindTicket}, 10)
	if !hasID(ticketHits, "BUG-1") || hasID(ticketHits, "MEMORY.md") {
		t.Fatalf("KindTicket filter: %+v", ticketHits)
	}
}
