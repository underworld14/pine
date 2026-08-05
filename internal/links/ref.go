// Package links resolves typed graph references between tickets, memory
// topics, MEMORY.md, and learnings, and builds a unified graph with computed
// backlinks. Cycles are allowed in the links graph (only deps enforce acyclicity).
package links

import (
	"strings"

	"github.com/underworld14/pine/internal/learning"
	"github.com/underworld14/pine/internal/memory"
	"github.com/underworld14/pine/internal/ticket"
)

// Kind classifies a resolved reference target.
type Kind string

const (
	KindTicket   Kind = "ticket"
	KindLearning Kind = "learning"
	KindTopic    Kind = "topic"
	KindMEMORY   Kind = "memory"
	KindUnknown  Kind = "unknown"
)

// Ref is a typed graph reference string plus its resolved kind.
type Ref struct {
	Raw    string
	Kind   Kind
	ID     string // ticket/LRN id, topic slug, or "MEMORY"
	Exists bool
}

// Catalog is the set of known targets used to resolve refs without I/O.
type Catalog struct {
	Tickets   map[string]*ticket.Ticket
	Learnings map[string]*learning.Learning
	Topics    map[string]memory.Topic // key = slug
	HasMEMORY bool
}

// ParseKind classifies a raw ref string by shape without checking existence.
func ParseKind(raw string) Kind {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return KindUnknown
	}
	if strings.EqualFold(raw, "MEMORY") || strings.EqualFold(raw, memory.FileMEMORY) {
		return KindMEMORY
	}
	if strings.HasPrefix(raw, memory.DirTopics+"/") {
		return KindTopic
	}
	if strings.HasPrefix(strings.ToUpper(raw), "LRN-") {
		return KindLearning
	}
	// Ticket IDs: PREFIX-suffix (hash or sequential).
	if i := strings.IndexByte(raw, '-'); i > 0 {
		prefix := raw[:i]
		ok := true
		for _, c := range prefix {
			if c < 'A' || c > 'Z' {
				ok = false
				break
			}
		}
		if ok && len(raw) > i+1 {
			return KindTicket
		}
	}
	return KindUnknown
}

// Normalize returns the canonical ID for a raw ref (slug for topics, MEMORY, etc.).
func Normalize(raw string) (Kind, string) {
	raw = strings.TrimSpace(raw)
	k := ParseKind(raw)
	switch k {
	case KindMEMORY:
		return KindMEMORY, "MEMORY"
	case KindTopic:
		slug := strings.TrimPrefix(raw, memory.DirTopics+"/")
		slug = strings.TrimSuffix(slug, ".md")
		return KindTopic, memory.Slugify(slug)
	case KindLearning:
		return KindLearning, strings.ToUpper(raw)
	case KindTicket:
		return KindTicket, raw
	default:
		return KindUnknown, raw
	}
}

// Resolve classifies and looks up a raw ref against the catalog.
func Resolve(raw string, cat Catalog) Ref {
	kind, id := Normalize(raw)
	ref := Ref{Raw: strings.TrimSpace(raw), Kind: kind, ID: id}
	switch kind {
	case KindTicket:
		_, ref.Exists = cat.Tickets[id]
	case KindLearning:
		_, ref.Exists = cat.Learnings[id]
	case KindTopic:
		_, ref.Exists = cat.Topics[id]
	case KindMEMORY:
		ref.Exists = cat.HasMEMORY
	default:
		ref.Exists = false
	}
	return ref
}

// NodeKey returns a stable graph node key for a resolved ref.
func (r Ref) NodeKey() string {
	switch r.Kind {
	case KindTopic:
		return memory.DirTopics + "/" + r.ID
	case KindMEMORY:
		return "MEMORY"
	default:
		return r.ID
	}
}
