package links_test

import (
	"testing"

	"github.com/underworld14/pine/internal/learning"
	"github.com/underworld14/pine/internal/links"
	"github.com/underworld14/pine/internal/memory"
	"github.com/underworld14/pine/internal/ticket"
)

func TestParseKindAndNormalize(t *testing.T) {
	cases := []struct {
		raw  string
		kind links.Kind
		id   string
	}{
		{"BUG-001", links.KindTicket, "BUG-001"},
		{"LRN-003", links.KindLearning, "LRN-003"},
		{"memory/analytics", links.KindTopic, "analytics"},
		{"MEMORY", links.KindMEMORY, "MEMORY"},
		{"MEMORY.md", links.KindMEMORY, "MEMORY"},
		{"not-a-ref", links.KindUnknown, "not-a-ref"},
	}
	for _, c := range cases {
		k, id := links.Normalize(c.raw)
		if k != c.kind || id != c.id {
			t.Errorf("Normalize(%q) = %s/%s want %s/%s", c.raw, k, id, c.kind, c.id)
		}
	}
}

func TestResolveExists(t *testing.T) {
	cat := links.Catalog{
		Tickets:   map[string]*ticket.Ticket{"BUG-001": {ID: "BUG-001", Title: "x"}},
		Learnings: map[string]*learning.Learning{"LRN-001": {ID: "LRN-001"}},
		Topics:    map[string]memory.Topic{"web": {Slug: "web", Title: "web"}},
		HasMEMORY: true,
	}
	if !links.Resolve("BUG-001", cat).Exists {
		t.Error("BUG-001 should exist")
	}
	if links.Resolve("BUG-999", cat).Exists {
		t.Error("BUG-999 should not exist")
	}
	if !links.Resolve("memory/web", cat).Exists {
		t.Error("memory/web should exist")
	}
	if !links.Resolve("MEMORY", cat).Exists {
		t.Error("MEMORY should exist")
	}
}

func TestBuildGraphBacklinks(t *testing.T) {
	tickets := []*ticket.Ticket{
		{ID: "BUG-001", Title: "Bug", Status: "todo", Links: []string{"memory/web"}, Deps: []string{"FEAT-001"}},
		{ID: "FEAT-001", Title: "Feat", Status: "doing", Parent: "EPIC-001"},
		{ID: "EPIC-001", Title: "Epic", Status: "todo"},
	}
	topics := []memory.Topic{
		{Slug: "web", Title: "web", Links: []string{"BUG-001"}},
	}
	g := links.Build(tickets, topics, nil, true)
	if len(g.Backlinks["BUG-001"]) == 0 {
		t.Fatalf("expected backlinks to BUG-001, got %#v", g.Backlinks)
	}
	found := false
	for _, src := range g.Backlinks["BUG-001"] {
		if src == "memory/web" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected memory/web → BUG-001 backlink, got %v", g.Backlinks["BUG-001"])
	}
	hasDep := false
	hasParent := false
	hasLink := false
	for _, e := range g.Edges {
		if e.Kind == links.EdgeDep && e.Source == "BUG-001" && e.Target == "FEAT-001" {
			hasDep = true
		}
		if e.Kind == links.EdgeParent && e.Source == "FEAT-001" && e.Target == "EPIC-001" {
			hasParent = true
		}
		if e.Kind == links.EdgeLink && e.Source == "BUG-001" && e.Target == "memory/web" {
			hasLink = true
		}
	}
	if !hasDep || !hasParent || !hasLink {
		t.Errorf("missing edges: dep=%v parent=%v link=%v", hasDep, hasParent, hasLink)
	}
}

func TestBuildDangling(t *testing.T) {
	tickets := []*ticket.Ticket{
		{ID: "BUG-001", Title: "Bug", Links: []string{"memory/missing"}},
	}
	g := links.Build(tickets, nil, nil, false)
	if len(g.Dangling) != 1 || g.Dangling[0] != "memory/missing" {
		t.Errorf("dangling = %v", g.Dangling)
	}
}

// TestBuildLinkEdgeKinds covers every addLinkEdge branch (ticket, learning,
// topic, MEMORY) and firstLine title derivation for learnings linked from a
// ticket.
func TestBuildLinkEdgeKinds(t *testing.T) {
	tickets := []*ticket.Ticket{
		{ID: "BUG-001", Title: "Bug", Links: []string{"LRN-001", "memory/web", "MEMORY"}},
	}
	topics := []memory.Topic{{Slug: "web", Title: "Web"}}
	learnings := []*learning.Learning{
		{ID: "LRN-001", Body: "# Prefer dark mode\n\nDetails here"},
	}
	g := links.Build(tickets, topics, learnings, true)

	titles := map[string]string{}
	for _, n := range g.Nodes {
		titles[n.ID] = n.Title
	}
	if titles["LRN-001"] != "Prefer dark mode" {
		t.Errorf("learning title = %q, want %q (firstLine # heading)", titles["LRN-001"], "Prefer dark mode")
	}
	if titles["memory/web"] != "Web" {
		t.Errorf("topic title = %q, want Web", titles["memory/web"])
	}
	if titles["MEMORY"] != "Project Memory" {
		t.Errorf("MEMORY title = %q, want Project Memory", titles["MEMORY"])
	}
	// A backlink from each linked target back to BUG-001.
	for _, target := range []string{"LRN-001", "memory/web", "MEMORY"} {
		found := false
		for _, src := range g.Backlinks[target] {
			if src == "BUG-001" {
				found = true
			}
		}
		if !found {
			t.Errorf("missing backlink %s ← BUG-001", target)
		}
	}
}

// TestFirstLineViaLearningTitles exercises firstLine's branches (heading,
// plain line, empty/--- only) through Build's learning-title derivation.
func TestFirstLineViaLearningTitles(t *testing.T) {
	learnings := []*learning.Learning{
		{ID: "LRN-A", Body: "plain first line\nsecond"},
		{ID: "LRN-B", Body: "---\n---\n"},
		{ID: "LRN-C", Body: "# A Heading Title"},
	}
	g := links.Build(nil, nil, learnings, false)
	titles := map[string]string{}
	for _, n := range g.Nodes {
		titles[n.ID] = n.Title
	}
	if titles["LRN-A"] != "plain first line" {
		t.Errorf("LRN-A title = %q, want plain first line", titles["LRN-A"])
	}
	if titles["LRN-B"] != "LRN-B" {
		t.Errorf("LRN-B title = %q, want fallback id", titles["LRN-B"])
	}
	if titles["LRN-C"] != "A Heading Title" {
		t.Errorf("LRN-C title = %q, want A Heading Title", titles["LRN-C"])
	}
}
