package contextgen

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/underworld14/pine/internal/memory"
	"github.com/underworld14/pine/internal/search"
	"github.com/underworld14/pine/internal/store"
)

// FormatMemoryBlock renders MEMORY.md + ranked topic excerpts for pine context.
func FormatMemoryBlock(s *store.Store, cwdHints []string, ticketLabels []string, limitTopics int) string {
	if s == nil {
		return ""
	}
	pineDir := s.Root()
	_ = memory.EnsureLayout(pineDir)

	var b strings.Builder
	// Global first, project second: the project is nearer the work and reads
	// last, and the fixed notice below says it wins on conflict.
	if s.Config().Context.GlobalMemory {
		b.WriteString(formatGlobalBlock())
	}
	mem, err := memory.ReadMEMORY(pineDir)
	if err == nil && strings.TrimSpace(mem) != "" {
		b.WriteString("## Project Memory\n")
		b.WriteString(memory.TruncateForContext(strings.TrimSpace(mem), memory.ContextMEMORYCap, ".pine/"+memory.FileMEMORY))
		if !strings.HasSuffix(b.String(), "\n") {
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}

	topics, err := memory.ListTopics(pineDir)
	if err != nil || len(topics) == 0 {
		return b.String()
	}
	if limitTopics <= 0 {
		limitTopics = 3
	}

	ranked := rankTopics(topics, cwdHints, ticketLabels, limitTopics)
	if len(ranked) == 0 {
		return b.String()
	}
	b.WriteString("## Memory Topics\n")
	for _, t := range ranked {
		excerpt := strings.TrimSpace(t.Body)
		// Prefer last ~8 lines (newest bullets).
		lines := strings.Split(excerpt, "\n")
		if len(lines) > 10 {
			lines = lines[len(lines)-10:]
			excerpt = "…\n" + strings.Join(lines, "\n")
		}
		fmt.Fprintf(&b, "### %s (`%s`)\n%s\n\n", t.Title, t.RelPath, excerpt)
	}
	return b.String()
}

// formatGlobalBlock renders the machine-wide store at ~/.pine.
//
// It creates nothing and never returns an error: pine context must not break —
// nor conjure a global store — because of a store the user may never have
// opted into. A missing store simply yields "".
func formatGlobalBlock() string {
	dir, err := memory.GlobalDir()
	if err != nil {
		return ""
	}
	body := ""
	if mem, err := memory.ReadMEMORY(dir); err == nil {
		body = strings.TrimSpace(mem)
	}
	topics, _ := memory.ListTopics(dir)
	if body == "" && len(topics) == 0 {
		return ""
	}

	label := memory.GlobalLabel(dir)
	var b strings.Builder
	b.WriteString("## Your Preferences (global)\n")
	b.WriteString("If anything here conflicts with Project Memory, Project Memory wins.\n\n")
	if body != "" {
		b.WriteString(memory.TruncateForContext(body, memory.ContextGlobalCap, label+"/"+memory.FileMEMORY))
		if !strings.HasSuffix(b.String(), "\n") {
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}
	if len(topics) > 0 {
		// Names only — never ranked, never inlined. Global topics therefore
		// never enter rankTopics' byPath map, where they would collide with
		// project topics: RelPath is "memory/<slug>.md" in both stores.
		names := make([]string, 0, len(topics))
		for _, t := range topics {
			names = append(names, t.Slug)
		}
		fmt.Fprintf(&b, "Global topics: %s — read %s/%s/<slug>.md if relevant\n\n",
			strings.Join(names, ", "), label, memory.DirTopics)
	}
	return b.String()
}

func rankTopics(topics []memory.Topic, cwdHints, ticketLabels []string, limit int) []memory.Topic {
	if len(topics) == 0 {
		return nil
	}
	idx, err := search.New()
	if err != nil {
		if len(topics) > limit {
			return topics[:limit]
		}
		return topics
	}
	defer idx.Close()
	for _, t := range topics {
		idx.Upsert(search.Doc{
			ID:    t.RelPath,
			Title: t.Title,
			Body:  t.Body,
			Kind:  search.KindMemory,
		})
	}
	var qparts []string
	for _, h := range cwdHints {
		qparts = append(qparts, filepath.Base(h), h)
	}
	qparts = append(qparts, ticketLabels...)
	query := strings.TrimSpace(strings.Join(qparts, " "))
	if query == "" {
		// No hints — return most recently updated topics.
		out := append([]memory.Topic(nil), topics...)
		for i := 0; i < len(out); i++ {
			for j := i + 1; j < len(out); j++ {
				if out[j].Updated.After(out[i].Updated) {
					out[i], out[j] = out[j], out[i]
				}
			}
		}
		if len(out) > limit {
			out = out[:limit]
		}
		return out
	}
	hits := idx.Search(query, search.Filter{Kind: search.KindMemory}, limit)
	byPath := map[string]memory.Topic{}
	for _, t := range topics {
		byPath[t.RelPath] = t
	}
	var out []memory.Topic
	seen := map[string]bool{}
	for _, h := range hits {
		if t, ok := byPath[h.ID]; ok {
			out = append(out, t)
			seen[h.ID] = true
		}
	}
	// Fill with recent topics if under limit.
	if len(out) < limit {
		rest := append([]memory.Topic(nil), topics...)
		for i := 0; i < len(rest); i++ {
			for j := i + 1; j < len(rest); j++ {
				if rest[j].Updated.After(rest[i].Updated) {
					rest[i], rest[j] = rest[j], rest[i]
				}
			}
		}
		for _, t := range rest {
			if seen[t.RelPath] {
				continue
			}
			out = append(out, t)
			if len(out) >= limit {
				break
			}
		}
	}
	return out
}
