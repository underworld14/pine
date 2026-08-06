package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/underworld14/pine/internal/view"
)

func TestFormatPrettyTicket(t *testing.T) {
	t.Parallel()

	got := formatPrettyTicket(view.Ticket{
		ID: "EPIC-001", Type: "EPIC", Title: "Ship it", Status: "todo", Priority: "high",
		EpicProgress: &view.Progress{Done: 1, Total: 3},
	})
	wantSub := []string{"○", "EPIC-001", "● high", "[epic]", "Ship it", "(1/3)"}
	for _, s := range wantSub {
		if !strings.Contains(got, s) {
			t.Fatalf("formatPrettyTicket missing %q in %q", s, got)
		}
	}

	blocked := formatPrettyTicket(view.Ticket{
		ID: "BUG-001", Type: "BUG", Title: "Crash", Status: "todo", Priority: "critical",
		Blocked: true, Unmet: []string{"FEAT-001"},
	})
	if !strings.HasPrefix(blocked, "● ") {
		t.Fatalf("blocked ticket should start with ●, got %q", blocked)
	}
	if !strings.Contains(blocked, "[bug]") || !strings.Contains(blocked, "🔒 FEAT-001") {
		t.Fatalf("blocked bug line missing badge/deps: %q", blocked)
	}

	done := formatPrettyTicket(view.Ticket{
		ID: "FEAT-002", Type: "FEAT", Title: "Done feat", Status: "done", Priority: "low",
	})
	if !strings.HasPrefix(done, "✓ ") {
		t.Fatalf("done ticket should start with ✓, got %q", done)
	}
	if strings.Contains(done, "[epic]") || strings.Contains(done, "[bug]") {
		t.Fatalf("feature should not have type badge: %q", done)
	}
}

func TestBuildTicketTree(t *testing.T) {
	t.Parallel()

	views := []view.Ticket{
		{ID: "BUG-orphan", Title: "Lonely", Status: "todo", Priority: "high"},
		{ID: "EPIC-1", Type: "EPIC", Title: "Parent", Status: "todo", Priority: "medium"},
		{ID: "FEAT-a", Title: "Child A", Status: "doing", Priority: "high", Parent: "EPIC-1"},
		{ID: "FEAT-b", Title: "Child B", Status: "todo", Priority: "low", Parent: "EPIC-1"},
		{ID: "FEAT-ghost", Title: "Missing parent", Status: "todo", Parent: "EPIC-missing"},
	}

	roots, children := buildTicketTree(views)
	rootIDs := make([]string, len(roots))
	for i, r := range roots {
		rootIDs[i] = r.ID
	}
	if strings.Join(rootIDs, ",") != "BUG-orphan,EPIC-1,FEAT-ghost" {
		t.Fatalf("roots = %v", rootIDs)
	}
	kids := children["EPIC-1"]
	if len(kids) != 2 || kids[0].ID != "FEAT-a" || kids[1].ID != "FEAT-b" {
		t.Fatalf("children of EPIC-1 = %v", kids)
	}
	if _, ok := children["EPIC-missing"]; ok {
		t.Fatal("ghost parent should not appear as a tree node")
	}
}

func TestRenderTicketTree(t *testing.T) {
	t.Parallel()

	views := []view.Ticket{
		{ID: "EPIC-1", Type: "EPIC", Title: "Epic", Status: "todo", Priority: "high",
			EpicProgress: &view.Progress{Done: 0, Total: 2}},
		{ID: "FEAT-a", Type: "FEAT", Title: "First", Status: "doing", Priority: "high", Parent: "EPIC-1"},
		{ID: "FEAT-b", Type: "FEAT", Title: "Second", Status: "todo", Priority: "medium", Parent: "EPIC-1",
			Blocked: true, Unmet: []string{"FEAT-a"}},
	}

	var buf bytes.Buffer
	renderTicketTree(&buf, views)
	out := buf.String()

	if !strings.Contains(out, "○ EPIC-1 ● high [epic] Epic (0/2)") {
		t.Fatalf("missing epic root line:\n%s", out)
	}
	if !strings.Contains(out, "├── ◐ FEAT-a ● high First") {
		t.Fatalf("missing first child:\n%s", out)
	}
	if !strings.Contains(out, "└── ● FEAT-b ● medium Second  🔒 FEAT-a") {
		t.Fatalf("missing last blocked child:\n%s", out)
	}
	if !strings.Contains(out, "3 tickets") {
		t.Fatalf("missing summary count:\n%s", out)
	}
	if !strings.Contains(out, "Status: ○ todo") {
		t.Fatalf("missing status legend:\n%s", out)
	}
}

func TestRenderTicketTreeEmpty(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	renderTicketTree(&buf, nil)
	if strings.TrimSpace(buf.String()) != "No tickets." {
		t.Fatalf("got %q", buf.String())
	}
}

func TestRenderTicketTreeNestedDepth(t *testing.T) {
	t.Parallel()

	views := []view.Ticket{
		{ID: "EPIC-1", Type: "EPIC", Title: "Top", Status: "todo"},
		{ID: "FEAT-mid", Title: "Mid", Status: "todo", Parent: "EPIC-1"},
		{ID: "BUG-leaf", Type: "BUG", Title: "Leaf", Status: "todo", Parent: "FEAT-mid"},
	}
	var buf bytes.Buffer
	renderTicketTree(&buf, views)
	out := buf.String()
	if !strings.Contains(out, "└── ○ FEAT-mid Mid") {
		t.Fatalf("missing mid:\n%s", out)
	}
	if !strings.Contains(out, "    └── ○ BUG-leaf [bug] Leaf") {
		t.Fatalf("missing nested leaf connector:\n%s", out)
	}
}
