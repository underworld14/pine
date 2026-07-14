package memory

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/underworld14/pine/internal/search"
)

// SuggestOpts controls destination ranking for a new insight.
type SuggestOpts struct {
	Text      string
	Cites     []string
	Component string
}

// Suggest ranks MEMORY.md, existing topics, and optional NEW:<slug> destinations.
func Suggest(pineDir string, opts SuggestOpts) ([]Recommendation, error) {
	text := strings.TrimSpace(opts.Text)
	if text == "" {
		return nil, fmt.Errorf("insight text is required")
	}
	// Deliberately does not ensure the layout: `pine learn suggest` is a
	// read ("no write" per its help), and the write path already ensures
	// before calling here. ReadMEMORY/ListTopics degrade cleanly if absent.

	idx, err := search.New()
	if err != nil {
		return nil, err
	}
	defer idx.Close()

	// Index MEMORY.
	if mem, err := ReadMEMORY(pineDir); err == nil && strings.TrimSpace(mem) != "" {
		idx.Upsert(search.Doc{
			ID:    FileMEMORY,
			Title: "Project memory",
			Body:  mem,
			Kind:  search.KindMemory,
		})
	}

	topics, err := ListTopics(pineDir)
	if err != nil {
		return nil, err
	}
	for _, t := range topics {
		idx.Upsert(search.Doc{
			ID:           t.RelPath,
			Title:        t.Title,
			Body:         t.Body,
			Kind:         search.KindMemory,
			RelatedFiles: extractCites(t.Body),
		})
	}

	query := text
	for _, c := range opts.Cites {
		query += " " + filepath.Base(c) + " " + c
	}
	if opts.Component != "" {
		query += " " + opts.Component
	}

	hits := idx.Search(query, search.Filter{Kind: search.KindMemory}, 20)
	byPath := map[string]Recommendation{}

	// Always seed MEMORY as a low baseline candidate.
	byPath[FileMEMORY] = Recommendation{
		Path:   FileMEMORY,
		Score:  0.15,
		Reason: "general project memory fallback",
	}

	for _, h := range hits {
		path := h.ID
		kind := "body match"
		score := h.Score
		if path == FileMEMORY {
			score += 0.05
			kind = "memory match"
		}
		if strings.HasPrefix(path, DirTopics+"/") {
			slug := strings.TrimSuffix(filepath.Base(path), ".md")
			boost := citeBoost(slug, opts.Cites, opts.Component)
			score += boost
			if boost > 0 {
				kind = "cite path + body match"
				// Strong cite/component + body hit should clear auto-append thresholds.
				floor := AutoMinScore + AutoMinGap
				if score < floor {
					score = floor
				}
			} else {
				kind = "topic body match"
			}
		}
		prev, ok := byPath[path]
		if !ok || score > prev.Score {
			byPath[path] = Recommendation{Path: path, Score: score, Reason: kind}
		}
	}

	// Cite/component soft hint for topics that didn't bleve-hit — never auto-confident alone.
	for _, t := range topics {
		boost := citeBoost(t.Slug, opts.Cites, opts.Component)
		if boost <= 0 {
			continue
		}
		prev, ok := byPath[t.RelPath]
		if ok && strings.Contains(prev.Reason, "body") {
			// Already have body relevance; small extra boost only.
			prev.Score += boost * 0.1
			byPath[t.RelPath] = prev
			continue
		}
		score := 0.25 + boost // capped below typical AutoMinScore when boost is modest
		if score > 0.5 {
			score = 0.5
		}
		if ok && prev.Score >= score {
			continue
		}
		byPath[t.RelPath] = Recommendation{
			Path:   t.RelPath,
			Score:  score,
			Reason: "cite/component path match",
		}
	}

	var recs []Recommendation
	for _, r := range byPath {
		recs = append(recs, r)
	}
	sortRecommendations(recs)

	bestTopic := 0.0
	for _, r := range recs {
		if strings.HasPrefix(r.Path, DirTopics+"/") && r.Score > bestTopic {
			bestTopic = r.Score
		}
	}
	if bestTopic < SoftTopicThreshold {
		slug := suggestNewSlug(text, opts.Cites, opts.Component)
		// Avoid duplicating an existing topic.
		exists := false
		for _, t := range topics {
			if t.Slug == slug {
				exists = true
				break
			}
		}
		if !exists {
			recs = append(recs, Recommendation{
				Path:   "NEW:" + slug,
				Score:  0.2,
				Reason: "no strong topic match — create new topic",
			})
			sortRecommendations(recs)
		}
	}

	if len(recs) > 8 {
		recs = recs[:8]
	}
	return recs, nil
}

// Confident reports whether the top recommendation may be auto-applied.
func Confident(recs []Recommendation) bool {
	if len(recs) == 0 {
		return false
	}
	best := recs[0]
	if strings.HasPrefix(best.Path, "NEW:") {
		return false // never auto-create topics
	}
	if best.Score < AutoMinScore {
		return false
	}
	if len(recs) == 1 {
		return true
	}
	return best.Score-recs[1].Score >= AutoMinGap
}

func citeBoost(slug string, cites []string, component string) float64 {
	slug = strings.ToLower(slug)
	boost := 0.0
	for _, c := range cites {
		c = filepath.ToSlash(strings.ToLower(c))
		parts := strings.Split(c, "/")
		for _, p := range parts {
			p = strings.TrimSuffix(p, filepath.Ext(p))
			if p == "" {
				continue
			}
			if p == slug || strings.Contains(p, slug) || strings.Contains(slug, p) {
				boost += 0.25
			}
		}
	}
	if component != "" {
		comp := filepath.ToSlash(strings.ToLower(component))
		base := filepath.Base(comp)
		if base == slug || strings.Contains(comp, slug) || strings.Contains(slug, base) {
			boost += 0.2
		}
	}
	if boost > 0.5 {
		boost = 0.5
	}
	return boost
}

func suggestNewSlug(text string, cites []string, component string) string {
	if component != "" {
		return Slugify(filepath.Base(component))
	}
	for _, c := range cites {
		parts := strings.Split(filepath.ToSlash(c), "/")
		// Prefer a meaningful directory segment near the leaf.
		for i := len(parts) - 2; i >= 0; i-- {
			p := parts[i]
			if p == "" || p == "src" || p == "lib" || p == "modules" || p == "internal" || p == "app" || p == "apps" {
				continue
			}
			return Slugify(p)
		}
		if base := strings.TrimSuffix(filepath.Base(c), filepath.Ext(c)); base != "" {
			return Slugify(base)
		}
	}
	// First few words of the insight.
	fields := strings.Fields(strings.ToLower(text))
	var words []string
	for _, w := range fields {
		w = topicSlugRe.ReplaceAllString(w, "")
		if len(w) < 3 {
			continue
		}
		switch w {
		case "the", "and", "for", "with", "from", "that", "this", "always", "never", "prefer", "use":
			continue
		}
		words = append(words, w)
		if len(words) == 3 {
			break
		}
	}
	if len(words) == 0 {
		return "general"
	}
	return Slugify(strings.Join(words, "-"))
}

func extractCites(body string) string {
	// Pull paths from "(cites: a, b)" suffixes for relatedFiles boosting.
	var paths []string
	for _, line := range strings.Split(body, "\n") {
		i := strings.Index(line, "(cites: ")
		if i < 0 {
			continue
		}
		rest := strings.TrimSuffix(strings.TrimSpace(line[i+8:]), ")")
		for _, p := range strings.Split(rest, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				paths = append(paths, p)
			}
		}
	}
	return strings.Join(paths, " ")
}

func sortRecommendations(recs []Recommendation) {
	for i := 0; i < len(recs); i++ {
		for j := i + 1; j < len(recs); j++ {
			if recs[j].Score > recs[i].Score ||
				(recs[j].Score == recs[i].Score && recs[j].Path < recs[i].Path) {
				recs[i], recs[j] = recs[j], recs[i]
			}
		}
	}
}
