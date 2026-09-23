package cli

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/underworld14/pine/internal/ticket"
	"github.com/underworld14/pine/internal/view"
)

var phaseNumRe = regexp.MustCompile(`(?i)^p(\d+)$`)

type phaseKey struct {
	bucket int // 0 = pN, 1 = other non-empty, 2 = empty
	n      int
	key    string
}

func phaseOrder(phase string) phaseKey {
	p := strings.TrimSpace(phase)
	if p == "" {
		return phaseKey{bucket: 2}
	}
	if m := phaseNumRe.FindStringSubmatch(p); m != nil {
		n, _ := strconv.Atoi(m[1])
		return phaseKey{bucket: 0, n: n}
	}
	return phaseKey{bucket: 1, key: strings.ToLower(p)}
}

func (a phaseKey) less(b phaseKey) bool {
	if a.bucket != b.bucket {
		return a.bucket < b.bucket
	}
	if a.n != b.n {
		return a.n < b.n
	}
	return a.key < b.key
}

// sortTreeSiblings orders one sibling group: numeric phase, then in-phase
// blockers above waiters, then priority / recency / original index.
func sortTreeSiblings(views []view.Ticket, prios []string) {
	if len(views) < 2 {
		return
	}
	if len(prios) == 0 {
		prios = ticket.DefaultPriorities
	}
	orig := make(map[string]int, len(views))
	for i, v := range views {
		orig[v.ID] = i
	}

	type group struct {
		key   phaseKey
		items []view.Ticket
	}
	var groups []group
	index := map[phaseKey]int{}
	for _, v := range views {
		k := phaseOrder(v.Phase)
		if i, ok := index[k]; ok {
			groups[i].items = append(groups[i].items, v)
			continue
		}
		index[k] = len(groups)
		groups = append(groups, group{key: k, items: []view.Ticket{v}})
	}
	sort.SliceStable(groups, func(i, j int) bool {
		return groups[i].key.less(groups[j].key)
	})

	out := views[:0]
	for _, g := range groups {
		out = append(out, topoPhaseGroup(g.items, prios, orig)...)
	}
}

func topoPhaseGroup(group []view.Ticket, prios []string, orig map[string]int) []view.Ticket {
	if len(group) < 2 {
		return group
	}
	byID := make(map[string]view.Ticket, len(group))
	inGroup := make(map[string]bool, len(group))
	indeg := make(map[string]int, len(group))
	succ := make(map[string][]string, len(group))
	ids := make([]string, len(group))
	for i, v := range group {
		byID[v.ID] = v
		inGroup[v.ID] = true
		ids[i] = v.ID
	}
	seenEdge := make(map[[2]string]bool)
	for _, v := range group {
		for _, dep := range v.Deps {
			if !inGroup[dep] {
				continue
			}
			edge := [2]string{dep, v.ID}
			if seenEdge[edge] {
				continue
			}
			seenEdge[edge] = true
			succ[dep] = append(succ[dep], v.ID)
			indeg[v.ID]++
		}
	}

	less := func(a, b string) bool {
		ta, tb := byID[a], byID[b]
		ra := ticket.PriorityRank(ta.Priority, prios)
		rb := ticket.PriorityRank(tb.Priority, prios)
		if ra != rb {
			return ra > rb
		}
		if ta.Updated != tb.Updated {
			return ta.Updated > tb.Updated
		}
		return orig[a] < orig[b]
	}

	var ready []string
	for _, id := range ids {
		if indeg[id] == 0 {
			ready = append(ready, id)
		}
	}

	out := make([]view.Ticket, 0, len(group))
	emitted := make(map[string]bool, len(group))
	for len(ready) > 0 {
		sort.SliceStable(ready, func(i, j int) bool { return less(ready[i], ready[j]) })
		id := ready[0]
		ready = ready[1:]
		if emitted[id] {
			continue
		}
		emitted[id] = true
		out = append(out, byID[id])
		for _, w := range succ[id] {
			indeg[w]--
			if indeg[w] == 0 {
				ready = append(ready, w)
			}
		}
	}

	var rest []string
	for _, id := range ids {
		if !emitted[id] {
			rest = append(rest, id)
		}
	}
	sort.SliceStable(rest, func(i, j int) bool { return less(rest[i], rest[j]) })
	for _, id := range rest {
		out = append(out, byID[id])
	}
	return out
}
