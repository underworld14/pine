package links

import (
	"sort"
	"strings"

	"github.com/underworld14/pine/internal/learning"
	"github.com/underworld14/pine/internal/memory"
	"github.com/underworld14/pine/internal/ticket"
)

// EdgeKind labels why two nodes are connected.
type EdgeKind string

const (
	EdgeDep    EdgeKind = "dep"
	EdgeParent EdgeKind = "parent"
	EdgeLink   EdgeKind = "link"
)

// Node is one vertex in the unified graph.
type Node struct {
	ID     string `json:"id"`
	Kind   Kind   `json:"kind"`
	Title  string `json:"title"`
	Status string `json:"status,omitempty"`
}

// Edge is a directed relationship from Source → Target.
type Edge struct {
	Source string   `json:"source"`
	Target string   `json:"target"`
	Kind   EdgeKind `json:"kind"`
}

// Graph is the unified tickets + memory + learnings graph with computed backlinks.
type Graph struct {
	Nodes     []Node
	Edges     []Edge
	Backlinks map[string][]string // node key → nodes that link to it via EdgeLink
	Dangling  []string            // unresolved link refs (raw strings)
}

// Build constructs the unified graph from tickets, topics, and learnings.
func Build(tickets []*ticket.Ticket, topics []memory.Topic, learnings []*learning.Learning, hasMEMORY bool) *Graph {
	cat := Catalog{
		Tickets:   make(map[string]*ticket.Ticket, len(tickets)),
		Learnings: make(map[string]*learning.Learning, len(learnings)),
		Topics:    make(map[string]memory.Topic, len(topics)),
		HasMEMORY: hasMEMORY,
	}
	for _, t := range tickets {
		cat.Tickets[t.ID] = t
	}
	for _, l := range learnings {
		cat.Learnings[l.ID] = l
	}
	for _, t := range topics {
		cat.Topics[t.Slug] = t
	}

	g := &Graph{
		Backlinks: map[string][]string{},
	}
	seenNode := map[string]bool{}
	addNode := func(n Node) {
		if seenNode[n.ID] {
			return
		}
		seenNode[n.ID] = true
		g.Nodes = append(g.Nodes, n)
	}
	seenEdge := map[string]bool{}
	addEdge := func(e Edge) {
		key := string(e.Kind) + "|" + e.Source + "|" + e.Target
		if seenEdge[key] {
			return
		}
		seenEdge[key] = true
		g.Edges = append(g.Edges, e)
	}

	for _, t := range tickets {
		addNode(Node{ID: t.ID, Kind: KindTicket, Title: t.Title, Status: t.Status})
		for _, dep := range t.Deps {
			addEdge(Edge{Source: t.ID, Target: dep, Kind: EdgeDep})
			if dt, ok := cat.Tickets[dep]; ok {
				addNode(Node{ID: dep, Kind: KindTicket, Title: dt.Title, Status: dt.Status})
			} else {
				addNode(Node{ID: dep, Kind: KindTicket, Title: dep})
			}
		}
		if t.Parent != "" {
			addEdge(Edge{Source: t.ID, Target: t.Parent, Kind: EdgeParent})
			if p, ok := cat.Tickets[t.Parent]; ok {
				addNode(Node{ID: t.Parent, Kind: KindTicket, Title: p.Title, Status: p.Status})
			} else {
				addNode(Node{ID: t.Parent, Kind: KindTicket, Title: t.Parent})
			}
		}
		for _, raw := range t.Links {
			addLinkEdge(g, cat, addNode, addEdge, t.ID, raw)
		}
	}

	for _, topic := range topics {
		key := memory.DirTopics + "/" + topic.Slug
		addNode(Node{ID: key, Kind: KindTopic, Title: topic.Title})
		for _, raw := range topic.Links {
			addLinkEdge(g, cat, addNode, addEdge, key, raw)
		}
	}

	for _, l := range learnings {
		title := firstLine(l.Body)
		if title == "" {
			title = l.ID
		}
		addNode(Node{ID: l.ID, Kind: KindLearning, Title: title})
		if l.Ticket != "" {
			addLinkEdge(g, cat, addNode, addEdge, l.ID, l.Ticket)
		}
	}

	if hasMEMORY {
		addNode(Node{ID: "MEMORY", Kind: KindMEMORY, Title: "Project Memory"})
	}

	for _, e := range g.Edges {
		if e.Kind != EdgeLink {
			continue
		}
		g.Backlinks[e.Target] = append(g.Backlinks[e.Target], e.Source)
	}
	for k := range g.Backlinks {
		sort.Strings(g.Backlinks[k])
	}
	sort.Slice(g.Nodes, func(i, j int) bool { return g.Nodes[i].ID < g.Nodes[j].ID })
	sort.Slice(g.Edges, func(i, j int) bool {
		if g.Edges[i].Source != g.Edges[j].Source {
			return g.Edges[i].Source < g.Edges[j].Source
		}
		if g.Edges[i].Target != g.Edges[j].Target {
			return g.Edges[i].Target < g.Edges[j].Target
		}
		return g.Edges[i].Kind < g.Edges[j].Kind
	})
	sort.Strings(g.Dangling)
	return g
}

func addLinkEdge(
	g *Graph,
	cat Catalog,
	addNode func(Node),
	addEdge func(Edge),
	sourceID, raw string,
) {
	ref := Resolve(raw, cat)
	targetKey := ref.NodeKey()
	if !ref.Exists {
		g.Dangling = append(g.Dangling, raw)
		addNode(Node{ID: targetKey, Kind: ref.Kind, Title: raw})
	} else {
		switch ref.Kind {
		case KindTicket:
			t := cat.Tickets[ref.ID]
			addNode(Node{ID: targetKey, Kind: KindTicket, Title: t.Title, Status: t.Status})
		case KindLearning:
			l := cat.Learnings[ref.ID]
			title := firstLine(l.Body)
			if title == "" {
				title = l.ID
			}
			addNode(Node{ID: targetKey, Kind: KindLearning, Title: title})
		case KindTopic:
			t := cat.Topics[ref.ID]
			addNode(Node{ID: targetKey, Kind: KindTopic, Title: t.Title})
		case KindMEMORY:
			addNode(Node{ID: "MEMORY", Kind: KindMEMORY, Title: "Project Memory"})
		}
	}
	addEdge(Edge{Source: sourceID, Target: targetKey, Kind: EdgeLink})
}

func firstLine(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "---" {
			continue
		}
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
		return line
	}
	return ""
}
