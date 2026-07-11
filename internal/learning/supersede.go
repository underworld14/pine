package learning

import (
	"sort"
	"time"
)

// MaxSupersedeDepth caps tip/cycle walks (same role as ticket dep path guards).
const MaxSupersedeDepth = 64

// BuildEdges indexes supersede relationships. Forward: id -> supersedes target.
// Reverse: target -> ids that supersede it.
func BuildEdges(learnings []*Learning) (forward map[string]string, reverse map[string][]string) {
	forward = map[string]string{}
	reverse = map[string][]string{}
	for _, l := range learnings {
		if l == nil || l.Degraded || l.Supersedes == "" {
			continue
		}
		forward[l.ID] = l.Supersedes
		reverse[l.Supersedes] = append(reverse[l.Supersedes], l.ID)
	}
	return forward, reverse
}

// CreatedMap builds id -> Created for tip/superseded-by tie-breaks.
func CreatedMap(learnings []*Learning) map[string]time.Time {
	out := make(map[string]time.Time, len(learnings))
	for _, l := range learnings {
		if l != nil {
			out[l.ID] = l.Created
		}
	}
	return out
}

// WouldCycle reports cycle members if adding edge from → to would cycle.
// from is the new/updated learning id; to is the supersedes target.
func WouldCycle(edges map[string]string, from, to string) []string {
	if from == "" || to == "" {
		return nil
	}
	if from == to {
		return []string{from}
	}
	sim := make(map[string]string, len(edges)+1)
	for k, v := range edges {
		sim[k] = v
	}
	sim[from] = to

	seen := map[string]bool{from: true}
	path := []string{from}
	cur := to
	for i := 0; i < MaxSupersedeDepth && cur != ""; i++ {
		path = append(path, cur)
		if seen[cur] {
			return uniqueSorted(path)
		}
		seen[cur] = true
		next, ok := sim[cur]
		if !ok {
			return nil
		}
		cur = next
	}
	return nil
}

// IsSuperseded reports whether any learning points at id.
func IsSuperseded(rev map[string][]string, id string) bool {
	return len(rev[id]) > 0
}

// SupersededBy returns the direct superseder of id (newest Created, then higher ID).
func SupersededBy(rev map[string][]string, created map[string]time.Time, id string) string {
	kids := rev[id]
	if len(kids) == 0 {
		return ""
	}
	return pickNewest(kids, created)
}

// Tip walks reverse supersede edges from id to the current tip.
func Tip(rev map[string][]string, created map[string]time.Time, id string) string {
	cur := id
	for i := 0; i < MaxSupersedeDepth; i++ {
		kids := rev[cur]
		if len(kids) == 0 {
			return cur
		}
		next := pickNewest(kids, created)
		if next == "" || next == cur {
			return cur
		}
		cur = next
	}
	return cur
}

// FindCycles returns each supersede cycle as a sorted member list.
func FindCycles(edges map[string]string) [][]string {
	var cycles [][]string
	seen := map[string]bool{}
	for id := range edges {
		if seen[id] {
			continue
		}
		pathIdx := map[string]int{}
		var path []string
		cur := id
		for i := 0; i < MaxSupersedeDepth && cur != ""; i++ {
			if idx, ok := pathIdx[cur]; ok {
				cyc := uniqueSorted(path[idx:])
				cycles = append(cycles, cyc)
				for _, m := range cyc {
					seen[m] = true
				}
				break
			}
			if seen[cur] {
				break
			}
			pathIdx[cur] = len(path)
			path = append(path, cur)
			next, ok := edges[cur]
			if !ok {
				break
			}
			cur = next
		}
		for _, m := range path {
			seen[m] = true
		}
	}
	sort.Slice(cycles, func(i, j int) bool {
		if len(cycles[i]) == 0 || len(cycles[j]) == 0 {
			return len(cycles[i]) < len(cycles[j])
		}
		return cycles[i][0] < cycles[j][0]
	})
	return cycles
}

func pickNewest(ids []string, created map[string]time.Time) string {
	if len(ids) == 0 {
		return ""
	}
	best := ids[0]
	for _, id := range ids[1:] {
		if newer(id, best, created) {
			best = id
		}
	}
	return best
}

func newer(a, b string, created map[string]time.Time) bool {
	ca, cb := created[a], created[b]
	if !ca.Equal(cb) {
		return ca.After(cb)
	}
	return a > b
}

func uniqueSorted(ids []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
