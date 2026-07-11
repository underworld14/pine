package search

import (
	"strings"

	"github.com/underworld14/pine/internal/learning"
)

// DocFromLearning projects a learning into an indexable Doc.
func DocFromLearning(l *learning.Learning) Doc {
	tags := make([]string, 0, len(l.Tags))
	for _, t := range l.Tags {
		tags = append(tags, strings.ToLower(strings.TrimSpace(t)))
	}
	return Doc{
		ID:          l.ID,
		Title:       strings.TrimSpace(l.Body),
		Body:        l.Body,
		Kind:        KindLearning,
		Scope:       l.Scope,
		Tags:        tags,
		Ticket:      l.Ticket,
		SourceAgent: l.SourceAgent,
	}
}
