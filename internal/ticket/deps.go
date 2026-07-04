package ticket

import "sort"

// Graph answers dependency and epic questions over a fixed set of tickets. It is
// rebuilt cheaply whenever the ticket set changes; nothing is persisted.
type Graph struct {
	byID     map[string]*Ticket
	children map[string][]*Ticket // parent id -> children
	inCycle  map[string]bool
	sccs     [][]string // non-trivial strongly-connected components (cycles)
}

// NewGraph indexes the tickets and precomputes cycle membership.
func NewGraph(tickets []*Ticket) *Graph {
	g := &Graph{
		byID:     make(map[string]*Ticket, len(tickets)),
		children: make(map[string][]*Ticket),
		inCycle:  make(map[string]bool),
	}
	for _, t := range tickets {
		g.byID[t.ID] = t
	}
	for _, t := range tickets {
		if t.Parent != "" {
			g.children[t.Parent] = append(g.children[t.Parent], t)
		}
	}
	g.computeCycles(tickets)
	return g
}

// DepInfo describes a ticket's dependency state.
type DepInfo struct {
	Blocked  bool     // has unmet existing deps, or participates in a cycle
	Unmet    []string // existing deps whose status is not done
	Dangling []string // dep IDs that do not exist in the set
	InCycle  bool     // participates in a dependency cycle
}

// Deps computes the dependency state of a ticket. Missing dependencies do not
// block (you cannot wait on a ticket that does not exist) but are reported as
// Dangling for doctor. Cycle members are always blocked.
func (g *Graph) Deps(id string) DepInfo {
	var info DepInfo
	t := g.byID[id]
	if t == nil {
		return info
	}
	for _, dep := range t.Deps {
		d := g.byID[dep]
		if d == nil {
			info.Dangling = append(info.Dangling, dep)
			continue
		}
		if d.Status != StatusDone {
			info.Unmet = append(info.Unmet, dep)
		}
	}
	info.InCycle = g.inCycle[id]
	info.Blocked = len(info.Unmet) > 0 || info.InCycle
	return info
}

// Blocked reports whether a ticket is blocked.
func (g *Graph) Blocked(id string) bool { return g.Deps(id).Blocked }

// Ready reports whether a ticket is actionable now: it exists, is not done, and
// is not blocked. This drives `pine ready`.
func (g *Graph) Ready(id string) bool {
	t := g.byID[id]
	if t == nil || t.Status == StatusDone {
		return false
	}
	return !g.Blocked(id)
}

// Children returns the tickets whose parent is the given epic, sorted by ID.
func (g *Graph) Children(epicID string) []*Ticket {
	kids := append([]*Ticket(nil), g.children[epicID]...)
	sort.Slice(kids, func(i, j int) bool { return kids[i].ID < kids[j].ID })
	return kids
}

// EpicProgress returns done and total child counts for an epic.
func (g *Graph) EpicProgress(epicID string) (done, total int) {
	for _, c := range g.children[epicID] {
		total++
		if c.Status == StatusDone {
			done++
		}
	}
	return done, total
}

// Cycles returns each dependency cycle as a sorted list of the ticket IDs that
// participate in it (one entry per strongly-connected component of size > 1, plus
// any self-dependency). Deterministic across runs.
func (g *Graph) Cycles() [][]string {
	out := make([][]string, len(g.sccs))
	copy(out, g.sccs)
	sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })
	return out
}

// computeCycles finds strongly-connected components via Tarjan's algorithm and
// records those that represent real cycles: any component with more than one
// member, or a single member that depends on itself. Every member of such a
// component is a genuine cycle participant (mutually reachable), which fixes the
// fabricated-path problem of naive back-edge tracing.
func (g *Graph) computeCycles(tickets []*Ticket) {
	index := 0
	idx := make(map[string]int)
	low := make(map[string]int)
	onStack := make(map[string]bool)
	var stack []string

	var strongconnect func(v string)
	strongconnect = func(v string) {
		idx[v] = index
		low[v] = index
		index++
		stack = append(stack, v)
		onStack[v] = true

		if t := g.byID[v]; t != nil {
			for _, w := range t.Deps {
				if g.byID[w] == nil {
					continue // dangling edge; not part of any cycle
				}
				if _, seen := idx[w]; !seen {
					strongconnect(w)
					low[v] = min(low[v], low[w])
				} else if onStack[w] {
					low[v] = min(low[v], idx[w])
				}
			}
		}

		if low[v] == idx[v] {
			var comp []string
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[w] = false
				comp = append(comp, w)
				if w == v {
					break
				}
			}
			selfLoop := false
			if len(comp) == 1 {
				if t := g.byID[comp[0]]; t != nil {
					for _, d := range t.Deps {
						if d == comp[0] {
							selfLoop = true
						}
					}
				}
			}
			if len(comp) > 1 || selfLoop {
				sort.Strings(comp)
				for _, m := range comp {
					g.inCycle[m] = true
				}
				g.sccs = append(g.sccs, comp)
			}
		}
	}

	ids := make([]string, 0, len(tickets))
	for _, t := range tickets {
		ids = append(ids, t.ID)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if _, seen := idx[id]; !seen {
			strongconnect(id)
		}
	}
}
