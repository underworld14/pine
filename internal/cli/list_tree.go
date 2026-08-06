package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/underworld14/pine/internal/view"
)

// statusIcon returns a bd-style glyph for a ticket's status.
// Open tickets with unmet deps render as blocked (●), matching Beads.
func statusIcon(v view.Ticket) string {
	if v.Blocked && v.Status != "done" {
		return "●"
	}
	switch v.Status {
	case "todo":
		return "○"
	case "doing":
		return "◐"
	case "testing":
		return "◑"
	case "done":
		return "✓"
	default:
		return "○"
	}
}

// formatPrettyTicket formats one ticket for tree list output.
// Shape (Beads-inspired): ICON ID ● priority [type] Title (progress)  deps
func formatPrettyTicket(v view.Ticket) string {
	var b strings.Builder
	b.WriteString(statusIcon(v))
	b.WriteByte(' ')
	b.WriteString(v.ID)
	if v.Priority != "" {
		b.WriteString(" ● ")
		b.WriteString(v.Priority)
	}
	switch strings.ToUpper(v.Type) {
	case "EPIC":
		b.WriteString(" [epic]")
	case "BUG":
		b.WriteString(" [bug]")
	}
	if v.Title != "" {
		b.WriteByte(' ')
		b.WriteString(v.Title)
	}
	if v.EpicProgress != nil {
		fmt.Fprintf(&b, " (%d/%d)", v.EpicProgress.Done, v.EpicProgress.Total)
	}
	if ann := treeDepAnnotation(v); ann != "" {
		b.WriteString("  ")
		b.WriteString(ann)
	}
	return b.String()
}

// treeDepAnnotation is a compact blocker hint for tree lines.
// Shows unmet IDs when few; otherwise a count (same spirit as the flat table).
func treeDepAnnotation(v view.Ticket) string {
	switch {
	case v.InCycle:
		return "🔒 cycle"
	case len(v.Unmet) > 0:
		if len(v.Unmet) <= 3 {
			return "🔒 " + strings.Join(v.Unmet, ", ")
		}
		return fmt.Sprintf("🔒 %d unmet", len(v.Unmet))
	case len(v.Dangling) > 0:
		return fmt.Sprintf("⚠ %d dangling", len(v.Dangling))
	default:
		return ""
	}
}

// buildTicketTree nests tickets under their parent when the parent is present
// in the same filtered set. Orphans (no parent, or parent filtered out) are roots.
func buildTicketTree(views []view.Ticket) (roots []view.Ticket, children map[string][]view.Ticket) {
	byID := make(map[string]struct{}, len(views))
	for _, v := range views {
		byID[v.ID] = struct{}{}
	}
	children = make(map[string][]view.Ticket)
	isChild := make(map[string]bool, len(views))

	// Preserve list sort order (priority then updated) within each sibling group.
	for _, v := range views {
		if v.Parent == "" {
			continue
		}
		if _, ok := byID[v.Parent]; !ok {
			continue
		}
		children[v.Parent] = append(children[v.Parent], v)
		isChild[v.ID] = true
	}

	for _, v := range views {
		if !isChild[v.ID] {
			roots = append(roots, v)
		}
	}
	return roots, children
}

func renderTicketTree(w io.Writer, views []view.Ticket) {
	if len(views) == 0 {
		fmt.Fprintln(w, "No tickets.")
		return
	}
	roots, children := buildTicketTree(views)
	for _, root := range roots {
		fmt.Fprintln(w, formatPrettyTicket(root))
		printTreeChildren(w, children, root.ID, "")
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "%d tickets\n", len(views))
	fmt.Fprintln(w, "Status: ○ todo  ◐ doing  ◑ testing  ● blocked  ✓ done")
}

func printTreeChildren(w io.Writer, children map[string][]view.Ticket, parentID, prefix string) {
	kids := children[parentID]
	for i, child := range kids {
		last := i == len(kids)-1
		connector := "├── "
		if last {
			connector = "└── "
		}
		fmt.Fprintf(w, "%s%s%s\n", prefix, connector, formatPrettyTicket(child))
		ext := "│   "
		if last {
			ext = "    "
		}
		printTreeChildren(w, children, child.ID, prefix+ext)
	}
}
