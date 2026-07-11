package contextgen

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/underworld14/pine/internal/learning"
	"github.com/underworld14/pine/internal/search"
	"github.com/underworld14/pine/internal/store"
)

// LearningSelectOpts controls which learnings are surfaced in context/prompt.
type LearningSelectOpts struct {
	TicketID     string
	TicketTitle  string
	TicketLabels []string
	CwdHints     []string // modified file paths, etc.
	Query        string   // optional explicit search query
	Limit        int      // default 10
}

// LearningRef is a compact learning for prompt templates.
type LearningRef struct {
	ID    string
	Scope string
	Tags  []string
	Body  string
}

// SelectLearnings returns the tip-resolved top-N relevant learnings and how many more exist.
func SelectLearnings(s *store.Store, opts LearningSelectOpts) (included []*learning.Learning, more int) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 10
	}
	all := s.AllLearnings()
	if len(all) == 0 {
		return nil, 0
	}

	_, rev := learning.BuildEdges(all)
	created := learning.CreatedMap(all)
	byID := map[string]*learning.Learning{}
	for _, l := range all {
		byID[l.ID] = l
	}

	// Resolve every learning to its supersede-tip first and dedupe, so a
	// scope/ticket admission decision is always made about the CURRENT, live
	// insight — never about a possibly-superseded, possibly-differently-scoped
	// source entry.
	seenTip := map[string]bool{}
	var tips []*learning.Learning
	for _, l := range all {
		if l.Degraded {
			continue
		}
		tipID := learning.Tip(rev, created, l.ID)
		if seenTip[tipID] {
			continue
		}
		seenTip[tipID] = true
		if tip := byID[tipID]; tip != nil && !tip.Degraded {
			tips = append(tips, tip)
		}
	}
	if len(tips) == 0 {
		return nil, 0
	}

	// Only global learnings, or ticket-scoped learnings for the current
	// ticket, are ever candidates. Relevance (tags, cwd hints) only ranks
	// within that admitted set below via the search index — it never grants
	// visibility across scopes or tickets.
	raw := filterLearningCandidates(tips, opts)
	if len(raw) == 0 {
		return nil, 0
	}

	// Drop candidates that are citation-stale (any cited path missing on
	// disk). Existence is memoized per path for this call.
	citeCache := map[string]bool{}
	exists := func(rel string) bool {
		if v, ok := citeCache[rel]; ok {
			return v
		}
		v := s.CiteExists(rel)
		citeCache[rel] = v
		return v
	}
	var candidates []*learning.Learning
	for _, l := range raw {
		if learning.IsCitationStale(learning.MissingCitedPaths(l.Cites, exists)) {
			continue
		}
		candidates = append(candidates, l)
	}
	if len(candidates) == 0 {
		return nil, 0
	}

	q := strings.TrimSpace(opts.Query)
	if q == "" {
		var parts []string
		if opts.TicketTitle != "" {
			parts = append(parts, opts.TicketTitle)
		}
		parts = append(parts, opts.TicketLabels...)
		for _, h := range opts.CwdHints {
			base := filepath.Base(h)
			if base != "" && base != "." {
				parts = append(parts, strings.TrimSuffix(base, filepath.Ext(base)))
			}
		}
		q = strings.Join(parts, " ")
	}

	if strings.TrimSpace(q) == "" {
		if len(candidates) > limit {
			return candidates[:limit], len(candidates) - limit
		}
		return candidates, 0
	}

	idx, err := search.New()
	if err != nil {
		if len(candidates) > limit {
			return candidates[:limit], len(candidates) - limit
		}
		return candidates, 0
	}
	defer idx.Close()
	candByID := map[string]*learning.Learning{}
	for _, l := range candidates {
		candByID[l.ID] = l
		idx.Upsert(search.DocFromLearning(l))
	}
	hits := idx.Search(q, search.Filter{Kind: search.KindLearning}, len(candidates))
	var ranked []*learning.Learning
	seen := map[string]bool{}
	for _, h := range hits {
		if l := candByID[h.ID]; l != nil && !seen[l.ID] {
			ranked = append(ranked, l)
			seen[l.ID] = true
		}
	}
	for _, l := range candidates {
		if !seen[l.ID] {
			ranked = append(ranked, l)
		}
	}
	if len(ranked) > limit {
		return ranked[:limit], len(ranked) - limit
	}
	return ranked, 0
}

// filterLearningCandidates admits a learning only when it's globally scoped,
// or ticket-scoped for the ticket currently in context. Tag/path relevance is
// a ranking signal (applied later via the search index), never an admission
// bypass — a ticket-scoped learning must never surface outside its own ticket.
func filterLearningCandidates(learnings []*learning.Learning, opts LearningSelectOpts) []*learning.Learning {
	var out []*learning.Learning
	for _, l := range learnings {
		switch {
		case l.Scope == learning.ScopeGlobal:
			out = append(out, l)
		case l.Scope == learning.ScopeTicket && opts.TicketID != "" && l.Ticket == opts.TicketID:
			out = append(out, l)
		}
	}
	return out
}

// FormatLearningsBlock renders the "## Relevant Learnings" markdown section.
// Returns empty string when there are no learnings.
func FormatLearningsBlock(included []*learning.Learning, more int) string {
	if len(included) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Relevant Learnings\n")
	for _, l := range included {
		meta := l.Scope
		if len(l.Tags) > 0 {
			meta += ", tags: " + strings.Join(l.Tags, ", ")
		}
		if l.Ticket != "" {
			meta += ", ticket: " + l.Ticket
		}
		body := strings.TrimSpace(l.Body)
		body = strings.ReplaceAll(body, "\n", " ")
		fmt.Fprintf(&b, "- **%s** (%s): %s\n", l.ID, meta, body)
	}
	if more > 0 {
		fmt.Fprintf(&b, "+%d more — run `pine learn search \"<topic>\"`\n", more)
	}
	b.WriteByte('\n')
	return b.String()
}

// LearningRefs converts learnings to template-friendly refs. Body has
// embedded newlines flattened to spaces, matching FormatLearningsBlock, so a
// multi-line insight doesn't break the "{{range .Learnings}}" bullet list in
// a fix-request template.
func LearningRefs(ls []*learning.Learning) []LearningRef {
	out := make([]LearningRef, 0, len(ls))
	for _, l := range ls {
		body := strings.TrimSpace(l.Body)
		body = strings.ReplaceAll(body, "\n", " ")
		out = append(out, LearningRef{
			ID:    l.ID,
			Scope: l.Scope,
			Tags:  append([]string(nil), l.Tags...),
			Body:  body,
		})
	}
	return out
}
