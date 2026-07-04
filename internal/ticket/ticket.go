// Package ticket is the pure domain layer for Pine tickets: parsing and
// serializing the markdown+frontmatter file format, reading body sections, and
// computing the dependency/epic graph. It performs no file I/O so it can be
// exhaustively unit tested with in-memory fixtures.
package ticket

import (
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Canonical status and priority values. Statuses are ultimately defined by
// board.json columns; these are the built-in defaults used for fallbacks and
// ranking.
const (
	StatusTodo    = "todo"
	StatusDoing   = "doing"
	StatusTesting = "testing"
	StatusDone    = "done"
)

// DefaultPriorities is the built-in priority ordering (lowest urgency first is
// index 0). PriorityRank uses it when config does not override.
var DefaultPriorities = []string{"low", "medium", "high", "critical"}

// ExtraField is an unknown frontmatter key preserved verbatim (with its YAML
// node) so that fields written by AI agents survive a round-trip through Pine.
type ExtraField struct {
	Key  string
	Node *yaml.Node
}

// Ticket is a parsed ticket. The Body is opaque markdown that round-trips
// byte-identically; only the frontmatter is normalized on serialize.
type Ticket struct {
	ID       string    // canonical, derived from filename (e.g. "BUG-001")
	Title    string    // frontmatter title, or filename fallback
	Status   string    // lowercased; "" when absent (board maps to first column)
	Priority string    // raw; validated against config by doctor
	Labels   []string  // may be empty
	Deps     []string  // ticket IDs this ticket is blocked by
	Parent   string    // epic ticket ID, or "" when none
	Created  time.Time // zero when unparseable (store fills from mtime)
	Updated  time.Time
	Extra    []ExtraField // unknown frontmatter keys, order-preserved
	Body     string       // markdown after the closing delimiter, verbatim

	// Runtime-only diagnostics, never serialized.
	FrontmatterID string   // the advisory `id:` value from frontmatter, if any
	Degraded      bool     // frontmatter could not be parsed; treat body read-only
	Warnings      []string // lenient-parse fallbacks taken (surfaced by doctor)
}

// Prefix returns the ID prefix (e.g. "BUG"), or "" when the ID is malformed.
func (t *Ticket) Prefix() string { return PrefixOf(t.ID) }

// PriorityRank maps a priority to an integer using the given ordering (higher
// means more urgent). Unknown priorities rank as "medium" if present in the
// ordering, else the midpoint — so malformed data never sorts to an extreme.
func PriorityRank(priority string, order []string) int {
	if len(order) == 0 {
		order = DefaultPriorities
	}
	p := strings.ToLower(strings.TrimSpace(priority))
	for i, v := range order {
		if v == p {
			return i
		}
	}
	// Unknown: fall back to "medium" rank when available, else the midpoint.
	for i, v := range order {
		if v == "medium" {
			return i
		}
	}
	return len(order) / 2
}
