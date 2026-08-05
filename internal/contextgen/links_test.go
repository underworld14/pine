package contextgen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/underworld14/pine/internal/memory"
	"github.com/underworld14/pine/internal/store"
	"github.com/underworld14/pine/internal/ticket"
)

// writeTopicFile writes a memory topic file with a title heading and links,
// then invalidates the store's links-graph cache so the next read sees it.
func writeTopicFile(t *testing.T, s *store.Store, slug, title string, links []string) {
	t.Helper()
	path := memory.TopicPath(s.Root(), slug)
	var b strings.Builder
	b.WriteString("---\ntopic: " + slug + "\n")
	if len(links) > 0 {
		b.WriteString("links:\n")
		for _, l := range links {
			b.WriteString("  - " + l + "\n")
		}
	}
	b.WriteString("---\n\n# " + title + "\n")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	s.InvalidateLinksGraph()
}

// TestFormatTicketLinksBlock covers the nil guards, the Links block, and the
// Backlinks block (with title resolution via the cached graph).
func TestFormatTicketLinksBlock(t *testing.T) {
	if got := FormatTicketLinksBlock(nil, &ticket.Ticket{ID: "BUG-1"}); got != "" {
		t.Errorf("nil store should yield empty, got %q", got)
	}
	s := scaffold(t)
	if got := FormatTicketLinksBlock(s, nil); got != "" {
		t.Errorf("nil ticket should yield empty, got %q", got)
	}

	// BUG-1 links to memory/web; the web topic links back to BUG-1 (backlink).
	tk, err := s.Create(store.CreateReq{Type: "bug", Title: "Bug One", Links: []string{"memory/web"}})
	if err != nil {
		t.Fatal(err)
	}
	writeTopicFile(t, s, "web", "Web Topic", []string{tk.ID})

	out := FormatTicketLinksBlock(s, tk)
	if !strings.Contains(out, "## Links") || !strings.Contains(out, "memory/web") {
		t.Fatalf("Links block missing:\n%s", out)
	}
	// The web topic links to BUG-001, so BUG-001's backlink source is memory/web.
	if !strings.Contains(out, "## Backlinks") {
		t.Fatalf("Backlinks block missing:\n%s", out)
	}
	// The backlink source should be resolved to its title.
	if !strings.Contains(out, "memory/web (Web Topic)") {
		t.Errorf("backlink title not resolved:\n%s", out)
	}

	// A ticket with no links and no backlinks yields empty.
	plain, _ := s.Create(store.CreateReq{Type: "bug", Title: "Plain"})
	if got := FormatTicketLinksBlock(s, plain); got != "" {
		t.Errorf("plain ticket should yield empty, got %q", got)
	}
}

// TestFormatGraphSummaryBlock covers the nil guard, the no-edges case, the
// limit, and the "… and N more" overflow line.
func TestFormatGraphSummaryBlock(t *testing.T) {
	if got := FormatGraphSummaryBlock(nil, 10); got != "" {
		t.Errorf("nil store should yield empty, got %q", got)
	}
	s := scaffold(t)
	if got := FormatGraphSummaryBlock(s, 10); got != "" {
		t.Errorf("empty graph should yield empty, got %q", got)
	}

	// Create several link edges (each ticket links to MEMORY).
	for i := 0; i < 5; i++ {
		if _, err := s.Create(store.CreateReq{Type: "bug", Title: "b", Links: []string{"MEMORY"}}); err != nil {
			t.Fatal(err)
		}
	}
	out := FormatGraphSummaryBlock(s, 3)
	if !strings.Contains(out, "## Knowledge Graph (links)") {
		t.Fatalf("summary header missing:\n%s", out)
	}
	if !strings.Contains(out, "… and 2 more") {
		t.Errorf("overflow line missing:\n%s", out)
	}
	// A large limit that fits all edges → no overflow line.
	out2 := FormatGraphSummaryBlock(s, 100)
	if strings.Contains(out2, "… and") {
		t.Errorf("no overflow expected:\n%s", out2)
	}
}
