package server

import (
	"path"
	"sort"
	"strings"
)

// FileItem is one autocomplete suggestion from /api/files.
type FileItem struct {
	Path string `json:"path"`
	Kind string `json:"kind"` // "file" | "dir"
}

// suggestFileItems builds ranked file/dir suggestions from git ls-files paths.
// q is matched case-insensitively against the full path; empty q returns a
// short prefix of the tree (bounded work, not a full-repo materialize).
func suggestFileItems(tracked []string, q string, capN int) []FileItem {
	if capN <= 0 {
		capN = fileSuggestCap
	}
	q = strings.ToLower(strings.TrimSpace(q))

	// Empty query: cheap discoverability — first N files + dirs derived from them.
	if q == "" {
		return suggestEmptyQuery(tracked, capN)
	}

	type scored struct {
		item  FileItem
		score int
	}
	var cands []scored
	seen := map[string]bool{}

	add := func(p, kind string, score int) {
		if score <= 0 || seen[p] {
			return
		}
		seen[p] = true
		cands = append(cands, scored{FileItem{Path: p, Kind: kind}, score})
	}

	for _, f := range tracked {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		fl := strings.ToLower(f)
		if strings.Contains(fl, q) {
			add(f, "file", matchScore(fl, path.Base(fl), q, true))
		}
		parts := strings.Split(f, "/")
		if len(parts) < 2 {
			continue
		}
		prefix := ""
		for i := 0; i < len(parts)-1; i++ {
			if parts[i] == "" {
				continue
			}
			prefix += parts[i] + "/"
			pl := strings.ToLower(prefix)
			base := strings.ToLower(parts[i])
			if strings.Contains(pl, q) || strings.Contains(base, q) {
				add(prefix, "dir", matchScore(pl, base, q, false))
			}
		}
	}

	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].score != cands[j].score {
			return cands[i].score > cands[j].score
		}
		if cands[i].item.Kind != cands[j].item.Kind {
			return cands[i].item.Kind == "file"
		}
		return cands[i].item.Path < cands[j].item.Path
	})

	if len(cands) > capN {
		cands = cands[:capN]
	}
	out := make([]FileItem, len(cands))
	for i, c := range cands {
		out[i] = c.item
	}
	return out
}

func suggestEmptyQuery(tracked []string, capN int) []FileItem {
	seen := map[string]bool{}
	out := make([]FileItem, 0, capN)
	add := func(p, kind string) {
		if seen[p] || len(out) >= capN {
			return
		}
		seen[p] = true
		out = append(out, FileItem{Path: p, Kind: kind})
	}
	for _, f := range tracked {
		if len(out) >= capN {
			break
		}
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		add(f, "file")
		parts := strings.Split(f, "/")
		prefix := ""
		for i := 0; i < len(parts)-1 && len(out) < capN; i++ {
			if parts[i] == "" {
				continue
			}
			prefix += parts[i] + "/"
			add(prefix, "dir")
		}
	}
	return out
}

// matchScore ranks how well path/base match q. Higher is better.
func matchScore(pathLower, baseLower, q string, isFile bool) int {
	if q == "" {
		if isFile {
			return 1
		}
		return 0
	}
	score := 0
	if baseLower == q {
		score = 100
	} else if strings.HasPrefix(baseLower, q) {
		score = 80
	} else if strings.Contains(baseLower, q) {
		score = 60
	} else if strings.HasPrefix(pathLower, q) {
		score = 50
	} else if strings.Contains(pathLower, q) {
		score = 30
	}
	if isFile && score > 0 {
		score++
	}
	return score
}
