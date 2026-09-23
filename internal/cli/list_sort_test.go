package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/underworld14/pine/internal/ticket"
	"github.com/underworld14/pine/internal/view"
)

func TestSortTreeSiblingsPhaseNumeric(t *testing.T) {
	t.Parallel()
	views := []view.Ticket{
		{ID: "A", Phase: "p2"},
		{ID: "B", Phase: "p10"},
		{ID: "C", Phase: "p0"},
		{ID: "D", Phase: "p1"},
	}
	sortTreeSiblings(views, ticket.DefaultPriorities)
	if got := joinIDs(views); got != "C,D,A,B" {
		t.Fatalf("phase numeric order = %s", got)
	}
}

func TestSortTreeSiblingsEmptyAndNamedPhase(t *testing.T) {
	t.Parallel()
	views := []view.Ticket{
		{ID: "none", Priority: "critical"},
		{ID: "p0", Phase: "p0", Priority: "low"},
		{ID: "beta", Phase: "beta", Priority: "high"},
		{ID: "alpha", Phase: "alpha", Priority: "medium"},
	}
	sortTreeSiblings(views, ticket.DefaultPriorities)
	if got := joinIDs(views); got != "p0,alpha,beta,none" {
		t.Fatalf("named/empty phase order = %s", got)
	}
}

func TestSortTreeSiblingsDepsBeatPriority(t *testing.T) {
	t.Parallel()
	views := []view.Ticket{
		{ID: "waiter", Priority: "critical", Phase: "p0", Deps: []string{"blocker"}},
		{ID: "blocker", Priority: "low", Phase: "p0"},
	}
	sortTreeSiblings(views, ticket.DefaultPriorities)
	if got := joinIDs(views); got != "blocker,waiter" {
		t.Fatalf("blocker should precede waiter = %s", got)
	}
}

func TestSortTreeSiblingsPriorityTieBreak(t *testing.T) {
	t.Parallel()
	views := []view.Ticket{
		{ID: "low", Priority: "low", Phase: "p0"},
		{ID: "crit", Priority: "critical", Phase: "p0"},
	}
	sortTreeSiblings(views, ticket.DefaultPriorities)
	if got := joinIDs(views); got != "crit,low" {
		t.Fatalf("priority tie-break = %s", got)
	}
}

func TestSortTreeSiblingsUpdatedThenOrig(t *testing.T) {
	t.Parallel()
	views := []view.Ticket{
		{ID: "old", Priority: "high", Phase: "p0", Updated: "2026-01-01T00:00:00Z"},
		{ID: "new", Priority: "high", Phase: "p0", Updated: "2026-06-01T00:00:00Z"},
		{ID: "first", Priority: "medium"},
		{ID: "second", Priority: "medium"},
	}
	sortTreeSiblings(views, ticket.DefaultPriorities)
	if got := joinIDs(views); got != "new,old,first,second" {
		t.Fatalf("updated/orig order = %s", got)
	}
}

func TestSortTreeSiblingsCycleDoesNotHang(t *testing.T) {
	t.Parallel()
	views := []view.Ticket{
		{ID: "A", Priority: "low", Phase: "p0", Deps: []string{"B"}},
		{ID: "B", Priority: "high", Phase: "p0", Deps: []string{"A"}},
	}
	sortTreeSiblings(views, ticket.DefaultPriorities)
	if got := joinIDs(views); got != "B,A" {
		t.Fatalf("cycle remainder by priority = %s", got)
	}
}

func TestSortTreeSiblingsIgnoresCrossPhaseDeps(t *testing.T) {
	t.Parallel()
	views := []view.Ticket{
		{ID: "p1-crit", Phase: "p1", Priority: "critical"},
		{ID: "p0-low", Phase: "p0", Priority: "low", Deps: []string{"p1-crit"}},
	}
	sortTreeSiblings(views, ticket.DefaultPriorities)
	if got := joinIDs(views); got != "p0-low,p1-crit" {
		t.Fatalf("cross-phase deps must not reorder phases = %s", got)
	}
}

func TestRenderTicketTreeSortsChildrenByPhase(t *testing.T) {
	t.Parallel()
	views := []view.Ticket{
		{ID: "EPIC-1", Type: "EPIC", Title: "Epic", Status: "todo"},
		{ID: "FEAT-late", Type: "FEAT", Title: "Late", Status: "todo", Parent: "EPIC-1", Phase: "p2"},
		{ID: "FEAT-early", Type: "FEAT", Title: "Early", Status: "todo", Parent: "EPIC-1", Phase: "p0"},
	}
	var buf bytes.Buffer
	renderTicketTree(&buf, views)
	out := buf.String()
	early := strings.Index(out, "FEAT-early")
	late := strings.Index(out, "FEAT-late")
	if early < 0 || late < 0 || early > late {
		t.Fatalf("p0 child should appear before p2:\n%s", out)
	}
}

func TestListJSONKeepsPriorityOrder(t *testing.T) {
	dir := initRepo(t)
	run(t, dir, "create", "--type", "feature", "--title", "p0 work", "--phase", "p0", "-p", "low")
	run(t, dir, "create", "--type", "bug", "--title", "urgent", "-p", "critical")

	jsonOut, err := run(t, dir, "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	bugAt := strings.Index(jsonOut, `"id": "BUG-001"`)
	featAt := strings.Index(jsonOut, `"id": "FEAT-001"`)
	if bugAt < 0 || featAt < 0 || bugAt > featAt {
		t.Fatalf("JSON must stay priority-then-updated (critical bug before p0 feat):\n%s", jsonOut)
	}

	textOut, err := run(t, dir, "list")
	if err != nil {
		t.Fatal(err)
	}
	bugText := strings.Index(textOut, "BUG-001")
	featText := strings.Index(textOut, "FEAT-001")
	if bugText < 0 || featText < 0 || featText > bugText {
		t.Fatalf("text tree must put p0 feat before unphased bug:\n%s", textOut)
	}
}

func joinIDs(views []view.Ticket) string {
	ids := make([]string, len(views))
	for i, v := range views {
		ids[i] = v.ID
	}
	return strings.Join(ids, ",")
}
