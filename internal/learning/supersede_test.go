package learning

import (
	"testing"
	"time"
)

func tAt(h int) time.Time {
	return time.Date(2026, 7, 11, h, 0, 0, 0, time.UTC)
}

func TestWouldCycleSelf(t *testing.T) {
	cyc := WouldCycle(nil, "LRN-001", "LRN-001")
	if len(cyc) != 1 || cyc[0] != "LRN-001" {
		t.Fatalf("self-cycle: %v", cyc)
	}
}

func TestWouldCycleDirectAB(t *testing.T) {
	edges := map[string]string{"LRN-A": "LRN-B"}
	cyc := WouldCycle(edges, "LRN-B", "LRN-A")
	if len(cyc) < 2 {
		t.Fatalf("expected A↔B cycle, got %v", cyc)
	}
}

func TestWouldCycleChainOf3FarEnd(t *testing.T) {
	edges := map[string]string{
		"LRN-C": "LRN-B",
		"LRN-B": "LRN-A",
	}
	if cyc := WouldCycle(edges, "LRN-D", "LRN-C"); cyc != nil {
		t.Fatalf("new tip D→C should not cycle: %v", cyc)
	}
	cyc := WouldCycle(edges, "LRN-A", "LRN-C")
	if cyc == nil {
		t.Fatal("expected cycle when A supersedes C")
	}
}

func TestWouldCycleNoCycle(t *testing.T) {
	edges := map[string]string{"LRN-B": "LRN-A"}
	if cyc := WouldCycle(edges, "LRN-C", "LRN-B"); cyc != nil {
		t.Fatalf("C→B→A should not cycle: %v", cyc)
	}
}

func TestTipChainOf3(t *testing.T) {
	learnings := []*Learning{
		{ID: "LRN-A", Created: tAt(1)},
		{ID: "LRN-B", Supersedes: "LRN-A", Created: tAt(2)},
		{ID: "LRN-C", Supersedes: "LRN-B", Created: tAt(3)},
	}
	_, rev := BuildEdges(learnings)
	created := CreatedMap(learnings)
	if tip := Tip(rev, created, "LRN-A"); tip != "LRN-C" {
		t.Fatalf("tip of A = %q want LRN-C", tip)
	}
	if tip := Tip(rev, created, "LRN-B"); tip != "LRN-C" {
		t.Fatalf("tip of B = %q want LRN-C", tip)
	}
	if tip := Tip(rev, created, "LRN-C"); tip != "LRN-C" {
		t.Fatalf("tip of C = %q want LRN-C", tip)
	}
}

func TestTipDanglingIgnored(t *testing.T) {
	learnings := []*Learning{
		{ID: "LRN-X", Supersedes: "LRN-GHOST", Created: tAt(1)},
	}
	_, rev := BuildEdges(learnings)
	created := CreatedMap(learnings)
	if tip := Tip(rev, created, "LRN-X"); tip != "LRN-X" {
		t.Fatalf("tip = %q", tip)
	}
	if !IsSuperseded(rev, "LRN-GHOST") {
		t.Fatal("ghost target should appear in reverse index")
	}
}

func TestSupersededByNewest(t *testing.T) {
	learnings := []*Learning{
		{ID: "LRN-A", Created: tAt(1)},
		{ID: "LRN-B", Supersedes: "LRN-A", Created: tAt(2)},
		{ID: "LRN-C", Supersedes: "LRN-A", Created: tAt(3)},
	}
	_, rev := BuildEdges(learnings)
	created := CreatedMap(learnings)
	if got := SupersededBy(rev, created, "LRN-A"); got != "LRN-C" {
		t.Fatalf("superseded by = %q want LRN-C", got)
	}
}

func TestFindCycles(t *testing.T) {
	edges := map[string]string{
		"LRN-A": "LRN-B",
		"LRN-B": "LRN-A",
	}
	cycles := FindCycles(edges)
	if len(cycles) != 1 {
		t.Fatalf("expected 1 cycle, got %v", cycles)
	}
}

func TestFindCyclesThreeNode(t *testing.T) {
	edges := map[string]string{
		"LRN-A": "LRN-B",
		"LRN-B": "LRN-C",
		"LRN-C": "LRN-A",
	}
	cycles := FindCycles(edges)
	if len(cycles) != 1 || len(cycles[0]) != 3 {
		t.Fatalf("expected 1 three-member cycle, got %v", cycles)
	}
	want := map[string]bool{"LRN-A": true, "LRN-B": true, "LRN-C": true}
	for _, id := range cycles[0] {
		if !want[id] {
			t.Errorf("unexpected cycle member %q in %v", id, cycles[0])
		}
	}
}

func TestWouldCycleEmptyIDs(t *testing.T) {
	if WouldCycle(nil, "", "LRN-A") != nil || WouldCycle(nil, "LRN-A", "") != nil {
		t.Fatal("empty ids")
	}
}

func TestSupersededByEmpty(t *testing.T) {
	if SupersededBy(nil, nil, "LRN-X") != "" {
		t.Fatal("expected empty")
	}
}

func TestTipMaxDepthAndSelfLoop(t *testing.T) {
	// Reverse edge pointing to self via empty pickNewest path: kid list with empty string.
	rev := map[string][]string{"LRN-A": {""}}
	if tip := Tip(rev, nil, "LRN-A"); tip != "LRN-A" {
		t.Fatalf("tip=%q", tip)
	}
}

func TestPickNewestEmptyAndNewerTieBreak(t *testing.T) {
	if pickNewest(nil, nil) != "" {
		t.Fatal("empty")
	}
	created := map[string]time.Time{"LRN-A": tAt(1), "LRN-B": tAt(1)}
	if !newer("LRN-B", "LRN-A", created) {
		t.Fatal("equal time should prefer higher ID")
	}
	if newer("LRN-A", "LRN-B", created) {
		t.Fatal("A should not be newer than B on tie")
	}
}

func TestFindCyclesNoCycleAndSeenBreak(t *testing.T) {
	edges := map[string]string{"LRN-A": "LRN-B"} // B not in edges
	if cyc := FindCycles(edges); len(cyc) != 0 {
		t.Fatalf("%v", cyc)
	}
	// Two separate cycles sorted by first member
	edges = map[string]string{
		"LRN-Z": "LRN-Y",
		"LRN-Y": "LRN-Z",
		"LRN-B": "LRN-A",
		"LRN-A": "LRN-B",
	}
	cyc := FindCycles(edges)
	if len(cyc) != 2 {
		t.Fatalf("%v", cyc)
	}
	if cyc[0][0] > cyc[1][0] {
		t.Fatalf("cycles not sorted: %v", cyc)
	}
}
