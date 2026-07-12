// Package learning is the pure domain layer for Pine learnings: parsing and
// serializing markdown+frontmatter files under .pine/learnings/. No file I/O.
package learning

import (
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ID prefix for all learnings.
const Prefix = "LRN"

// Scope values.
const (
	ScopeGlobal    = "global"
	ScopeTicket    = "ticket"
	ScopeComponent = "component"
)

// Source agent values.
const (
	SourceClaudeCode = "claude-code"
	SourceCodex      = "codex"
	SourceCursor     = "cursor"
	SourceGemini     = "gemini"
	SourceManual     = "manual"
)

var validScopes = map[string]bool{
	ScopeGlobal:    true,
	ScopeTicket:    true,
	ScopeComponent: true,
}

var validSourceAgents = map[string]bool{
	SourceClaudeCode: true,
	SourceCodex:      true,
	SourceCursor:     true,
	SourceGemini:     true,
	SourceManual:     true,
}

// ExtraField is an unknown frontmatter key preserved verbatim.
type ExtraField struct {
	Key  string
	Node *yaml.Node
}

// Learning is a parsed learning file. Body is the insight text after frontmatter.
type Learning struct {
	ID          string // canonical, from filename (e.g. "LRN-001")
	Scope       string // global | ticket | component
	Tags        []string
	Ticket      string // when scope == ticket
	Component   string // when scope == component (a repo-relative path or module name)
	SourceAgent string
	Supersedes  string   // optional: ID of the learning this replaces
	Cites       []string // optional: repo-relative paths this insight depends on
	Created     time.Time
	Extra       []ExtraField
	Body        string

	// Runtime-only diagnostics, never serialized.
	FrontmatterID string
	Degraded      bool
	Warnings      []string
}

// ValidScope reports whether s is a known scope value.
func ValidScope(s string) bool {
	return validScopes[strings.ToLower(strings.TrimSpace(s))]
}

// ValidSourceAgent reports whether a is a known source_agent value.
func ValidSourceAgent(a string) bool {
	return validSourceAgents[strings.ToLower(strings.TrimSpace(a))]
}
