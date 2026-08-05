package contextgen

import (
	"fmt"
	"strings"

	"github.com/underworld14/pine/internal/links"
	"github.com/underworld14/pine/internal/store"
	"github.com/underworld14/pine/internal/ticket"
)

// FormatTicketLinksBlock renders a ticket's typed links and computed backlinks.
func FormatTicketLinksBlock(s *store.Store, t *ticket.Ticket) string {
	if s == nil || t == nil {
		return ""
	}
	g := s.LinksGraph()
	var b strings.Builder
	if len(t.Links) > 0 {
		b.WriteString("## Links\n")
		for _, raw := range t.Links {
			b.WriteString("- " + raw + "\n")
		}
		b.WriteByte('\n')
	}
	if backs := g.Backlinks[t.ID]; len(backs) > 0 {
		// Build a title index once instead of an O(n) scan of g.Nodes per backlink.
		titles := make(map[string]string, len(g.Nodes))
		for _, n := range g.Nodes {
			titles[n.ID] = n.Title
		}
		b.WriteString("## Backlinks\n")
		for _, src := range backs {
			title := src
			if t, ok := titles[src]; ok && t != "" {
				title = fmt.Sprintf("%s (%s)", src, t)
			}
			b.WriteString("- " + title + "\n")
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// FormatGraphSummaryBlock renders a short overview of typed-link edges for context.
func FormatGraphSummaryBlock(s *store.Store, limit int) string {
	if s == nil {
		return ""
	}
	if limit <= 0 {
		limit = 12
	}
	g := s.LinksGraph()
	var linkEdges []links.Edge
	for _, e := range g.Edges {
		if e.Kind == links.EdgeLink {
			linkEdges = append(linkEdges, e)
		}
	}
	if len(linkEdges) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Knowledge Graph (links)\n")
	n := len(linkEdges)
	if n > limit {
		n = limit
	}
	for _, e := range linkEdges[:n] {
		fmt.Fprintf(&b, "- %s → %s\n", e.Source, e.Target)
	}
	if len(linkEdges) > limit {
		fmt.Fprintf(&b, "- … and %d more\n", len(linkEdges)-limit)
	}
	b.WriteByte('\n')
	return b.String()
}
