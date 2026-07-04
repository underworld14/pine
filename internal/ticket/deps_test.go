package ticket

import (
	"reflect"
	"testing"
)

func tk(id, status string, deps ...string) *Ticket {
	return &Ticket{ID: id, Status: status, Deps: deps}
}

func TestReadyAndBlockedChain(t *testing.T) {
	// C -> B -> A means A depends on B depends on C.
	a := tk("A-001", "todo", "B-001")
	b := tk("B-001", "todo", "C-001")
	c := tk("C-001", "todo")
	g := NewGraph([]*Ticket{a, b, c})

	if !g.Blocked("A-001") || !g.Blocked("B-001") {
		t.Errorf("A and B should be blocked")
	}
	if g.Blocked("C-001") {
		t.Errorf("C should not be blocked")
	}
	if !g.Ready("C-001") {
		t.Errorf("C should be ready")
	}
	if g.Ready("A-001") {
		t.Errorf("A should not be ready while B is open")
	}
}

func TestClosingDepUnblocks(t *testing.T) {
	a := tk("A-001", "todo", "B-001")
	b := tk("B-001", "done")
	g := NewGraph([]*Ticket{a, b})
	if g.Blocked("A-001") {
		t.Errorf("A should be unblocked once B is done")
	}
	if !g.Ready("A-001") {
		t.Errorf("A should be ready")
	}
}

func TestDanglingDep(t *testing.T) {
	a := tk("A-001", "todo", "GHOST-999")
	g := NewGraph([]*Ticket{a})
	info := g.Deps("A-001")
	if info.Blocked {
		t.Errorf("dangling dep must not block")
	}
	if !reflect.DeepEqual(info.Dangling, []string{"GHOST-999"}) {
		t.Errorf("dangling = %v", info.Dangling)
	}
}

func TestCycleDetected(t *testing.T) {
	a := tk("A-001", "todo", "B-001")
	b := tk("B-001", "todo", "A-001")
	g := NewGraph([]*Ticket{a, b})
	if !g.Blocked("A-001") || !g.Blocked("B-001") {
		t.Errorf("cycle members must be blocked")
	}
	if !g.Deps("A-001").InCycle {
		t.Errorf("A should be flagged InCycle")
	}
	cycles := g.Cycles()
	if len(cycles) != 1 {
		t.Fatalf("expected 1 cycle, got %d: %v", len(cycles), cycles)
	}
	if len(cycles[0]) != 2 {
		t.Errorf("cycle should have 2 members: %v", cycles[0])
	}
}

func TestDiamondNoFalseCycle(t *testing.T) {
	// D depends on B and C; B and C both depend on A. No cycle.
	d := tk("D-001", "todo", "B-001", "C-001")
	b := tk("B-001", "todo", "A-001")
	c := tk("C-001", "todo", "A-001")
	a := tk("A-001", "done")
	g := NewGraph([]*Ticket{a, b, c, d})
	if len(g.Cycles()) != 0 {
		t.Errorf("diamond must not be a cycle: %v", g.Cycles())
	}
	if !g.Ready("B-001") || !g.Ready("C-001") {
		t.Errorf("B and C ready once A done")
	}
	if g.Ready("D-001") {
		t.Errorf("D blocked until B and C done")
	}
}

func TestCyclesReportsSCCMembersNotFabricatedPaths(t *testing.T) {
	// A depends on B and C; both B and C depend back on A. All three are mutually
	// reachable, so it is a single strongly-connected component — not two cycles
	// with a fabricated C->A->B path.
	a := tk("A-001", "todo", "B-001", "C-001")
	b := tk("B-001", "todo", "A-001")
	c := tk("C-001", "todo", "A-001")
	g := NewGraph([]*Ticket{a, b, c})
	cycles := g.Cycles()
	if len(cycles) != 1 {
		t.Fatalf("expected one cycle group, got %d: %v", len(cycles), cycles)
	}
	if len(cycles[0]) != 3 {
		t.Errorf("cycle group should contain all 3 members: %v", cycles[0])
	}
	if !g.Blocked("A-001") || !g.Blocked("B-001") || !g.Blocked("C-001") {
		t.Errorf("all cycle members must be blocked")
	}
}

func TestSelfDependencyIsACycle(t *testing.T) {
	a := tk("A-001", "todo", "A-001")
	g := NewGraph([]*Ticket{a})
	if !g.Blocked("A-001") || !g.Deps("A-001").InCycle {
		t.Errorf("a self-dependency should be a cycle")
	}
	if len(g.Cycles()) != 1 {
		t.Errorf("expected one cycle for a self-dependency: %v", g.Cycles())
	}
}

func TestEpicChildrenAndProgress(t *testing.T) {
	e := tk("EPIC-001", "doing")
	c1 := tk("BUG-001", "done")
	c1.Parent = "EPIC-001"
	c2 := tk("BUG-002", "todo")
	c2.Parent = "EPIC-001"
	g := NewGraph([]*Ticket{e, c1, c2})
	kids := g.Children("EPIC-001")
	if len(kids) != 2 || kids[0].ID != "BUG-001" {
		t.Errorf("children = %v", kids)
	}
	done, total := g.EpicProgress("EPIC-001")
	if done != 1 || total != 2 {
		t.Errorf("progress = %d/%d", done, total)
	}
}
