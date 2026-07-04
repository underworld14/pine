package ticket

import "sort"

// Graph answers dependency and epic questions over a fixed set of tickets. It is
// rebuilt cheaply whenever the ticket set changes; nothing is persisted.
type Graph struct {
	byID     map[string]*Ticket
	children map[string][]*Ticket // parent id -> children
	inCycle  map[string]bool
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

// Cycles returns each dependency cycle as an ordered list of ticket IDs, for
// doctor to report. Deterministic across runs.
func (g *Graph) Cycles() [][]string {
	var cycles [][]string
	seen := map[string]bool{}
	for id := range g.inCycle {
		if seen[id] {
			continue
		}
		cyc := g.recoverCycle(id)
		for _, m := range cyc {
			seen[m] = true
		}
		if len(cyc) > 0 {
			cycles = append(cycles, cyc)
		}
	}
	sort.Slice(cycles, func(i, j int) bool { return cycles[i][0] < cycles[j][0] })
	return cycles
}

// computeCycles marks every ticket that participates in a dependency cycle using
// recursive DFS with colors (0=unvisited, 1=on-stack, 2=done).
func (g *Graph) computeCycles(tickets []*Ticket) {
	color := make(map[string]int, len(tickets))
	var onStack []string

	var dfs func(id string)
	dfs = func(id string) {
		color[id] = 1
		onStack = append(onStack, id)
		if t := g.byID[id]; t != nil {
			for _, dep := range t.Deps {
				if g.byID[dep] == nil {
					continue // dangling; not part of a cycle
				}
				switch color[dep] {
				case 0:
					dfs(dep)
				case 1:
					// Back edge: everything from dep to the top of the stack is a cycle.
					mark := false
					for _, s := range onStack {
						if s == dep {
							mark = true
						}
						if mark {
							g.inCycle[s] = true
						}
					}
				}
			}
		}
		onStack = onStack[:len(onStack)-1]
		color[id] = 2
	}

	ids := make([]string, 0, len(tickets))
	for _, t := range tickets {
		ids = append(ids, t.ID)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if color[id] == 0 {
			dfs(id)
		}
	}
}

// recoverCycle walks deps edges staying within cycle members to reconstruct one
// cycle path.
func (g *Graph) recoverCycle(start string) []string {
	var path []string
	visited := map[string]bool{}
	cur := start
	for cur != "" && !visited[cur] {
		visited[cur] = true
		path = append(path, cur)
		next := ""
		if t := g.byID[cur]; t != nil {
			for _, dep := range t.Deps {
				if g.inCycle[dep] {
					next = dep
					break
				}
			}
		}
		cur = next
	}
	return path
}
