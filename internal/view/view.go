// Package view builds the presentation DTO for a ticket, joining the parsed
// ticket with computed dependency state, epic children, attachments, and its
// content hash. Both the CLI (--json) and the HTTP API render from it, so the
// two surfaces never drift.
package view

import (
	"time"

	"github.com/izzadev/pine/internal/store"
	"github.com/izzadev/pine/internal/ticket"
)

// Ticket is the JSON-facing representation of a ticket.
type Ticket struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"` // ID prefix, e.g. "BUG"
	Title    string   `json:"title"`
	Status   string   `json:"status"`
	Priority string   `json:"priority"`
	Labels   []string `json:"labels"`
	Deps     []string `json:"deps"`
	Parent   string   `json:"parent,omitempty"`
	Created  string   `json:"created"`
	Updated  string   `json:"updated"`

	Blocked  bool     `json:"blocked"`
	Unmet    []string `json:"unmet,omitempty"`
	Dangling []string `json:"dangling,omitempty"`
	InCycle  bool     `json:"inCycle,omitempty"`

	Children     []ChildRef `json:"children,omitempty"`
	EpicProgress *Progress  `json:"epicProgress,omitempty"`

	Hash        string                 `json:"hash"`
	Degraded    bool                   `json:"degraded,omitempty"`
	Body        string                 `json:"body,omitempty"`
	Attachments []store.AttachmentInfo `json:"attachments"`
}

// ChildRef is a lightweight reference to an epic's child ticket.
type ChildRef struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

// Progress is an epic's done/total child count.
type Progress struct {
	Done  int `json:"done"`
	Total int `json:"total"`
}

// Build assembles the view for one ticket. includeBody controls whether the
// (potentially large) markdown body is included.
func Build(s *store.Store, g *ticket.Graph, t *ticket.Ticket, includeBody bool) Ticket {
	info := g.Deps(t.ID)
	v := Ticket{
		ID:          t.ID,
		Type:        t.Prefix(),
		Title:       t.Title,
		Status:      t.Status,
		Priority:    t.Priority,
		Labels:      nonNil(t.Labels),
		Deps:        nonNil(t.Deps),
		Parent:      t.Parent,
		Created:     fmtTime(t.Created),
		Updated:     fmtTime(t.Updated),
		Blocked:     info.Blocked,
		Unmet:       info.Unmet,
		Dangling:    info.Dangling,
		InCycle:     info.InCycle,
		Degraded:    t.Degraded,
		Attachments: attachmentsOrEmpty(s.Attachments(t.ID)),
	}
	if h, ok := s.Hash(t.ID); ok {
		v.Hash = h
	}
	if includeBody {
		v.Body = t.Body
	}
	if kids := g.Children(t.ID); len(kids) > 0 {
		for _, c := range kids {
			v.Children = append(v.Children, ChildRef{ID: c.ID, Title: c.Title, Status: c.Status})
		}
		done, total := g.EpicProgress(t.ID)
		v.EpicProgress = &Progress{Done: done, Total: total}
	}
	return v
}

// BuildAll returns views for every ticket, sorted by ID.
func BuildAll(s *store.Store, includeBody bool) []Ticket {
	g := s.Graph()
	all := s.All()
	out := make([]Ticket, 0, len(all))
	for _, t := range all {
		out = append(out, Build(s, g, t, includeBody))
	}
	return out
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func attachmentsOrEmpty(a []store.AttachmentInfo) []store.AttachmentInfo {
	if a == nil {
		return []store.AttachmentInfo{}
	}
	return a
}

func fmtTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
