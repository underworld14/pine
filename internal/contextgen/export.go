package contextgen

import (
	"fmt"
	"strings"

	"github.com/underworld14/pine/internal/store"
	"github.com/underworld14/pine/internal/ticket"
)

// ExportMarkdown renders every ticket grouped by board column, for `pine export`.
func ExportMarkdown(s *store.Store) string {
	var b strings.Builder
	cfg := s.Config()
	fmt.Fprintf(&b, "# %s — Tickets\n\n", cfg.Project.Name)

	board := s.Board()
	byStatus := map[string][]*ticket.Ticket{}
	var other []*ticket.Ticket
	for _, t := range s.All() {
		if board.HasStatus(t.Status) {
			byStatus[t.Status] = append(byStatus[t.Status], t)
		} else {
			other = append(other, t)
		}
	}

	writeGroup := func(title string, ts []*ticket.Ticket) {
		if len(ts) == 0 {
			return
		}
		fmt.Fprintf(&b, "## %s\n\n", title)
		for _, t := range ts {
			writeTicket(&b, t)
		}
	}

	for _, col := range board.Columns {
		writeGroup(col.Title, byStatus[col.Status])
	}
	writeGroup("Other", other)

	return b.String()
}

func writeTicket(b *strings.Builder, t *ticket.Ticket) {
	fmt.Fprintf(b, "### [%s] %s\n", t.ID, t.Title)
	meta := fmt.Sprintf("status: %s · priority: %s", t.Status, t.Priority)
	if len(t.Labels) > 0 {
		meta += " · labels: " + strings.Join(t.Labels, ", ")
	}
	if len(t.Deps) > 0 {
		meta += " · deps: " + strings.Join(t.Deps, ", ")
	}
	if t.Parent != "" {
		meta += " · parent: " + t.Parent
	}
	fmt.Fprintf(b, "%s\n", meta)
	if body := strings.TrimSpace(t.Body); body != "" {
		fmt.Fprintf(b, "\n%s\n", body)
	}
	b.WriteString("\n")
}
