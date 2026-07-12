package view

import (
	"strings"

	"github.com/underworld14/pine/internal/learning"
)

// Learning is the JSON-facing representation of a learning.
type Learning struct {
	ID           string       `json:"id"`
	Scope        string       `json:"scope"`
	Tags         []string     `json:"tags,omitempty"`
	Ticket       string       `json:"ticket,omitempty"`
	Component    string       `json:"component,omitempty"`
	SourceAgent  string       `json:"source_agent"`
	Supersedes   string       `json:"supersedes,omitempty"`
	SupersededBy string       `json:"superseded_by,omitempty"`
	Cites        []string     `json:"cites,omitempty"`
	CiteStatus   []CiteStatus `json:"cite_status,omitempty"`
	Created      string       `json:"created"`
	Body         string       `json:"body"`
	Degraded     bool         `json:"degraded,omitempty"`
	Stale        bool         `json:"stale,omitempty"`
}

// CiteStatus is a live existence check for one cited path.
type CiteStatus struct {
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
}

// BuildLearning projects a domain learning into the JSON DTO.
func BuildLearning(l *learning.Learning) Learning {
	return Learning{
		ID:          l.ID,
		Scope:       l.Scope,
		Tags:        append([]string(nil), l.Tags...),
		Ticket:      l.Ticket,
		Component:   l.Component,
		SourceAgent: l.SourceAgent,
		Supersedes:  l.Supersedes,
		Cites:       append([]string(nil), l.Cites...),
		Created:     fmtTime(l.Created),
		Body:        strings.TrimSpace(l.Body),
		Degraded:    l.Degraded,
	}
}

// LearningHit is a search result: the full Learning DTO plus a relevance
// score, so `pine learn search --json` carries the same fields (supersedes,
// superseded_by, cites, cite_status, degraded, stale) as list/show.
type LearningHit struct {
	Learning
	Score float64 `json:"score"`
}
