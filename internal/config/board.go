package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// BoardVersion is the current board.json schema version.
const BoardVersion = 1

// Board mirrors .pine/board.json. It defines kanban columns only; a ticket's
// column is decided by its frontmatter status, never stored here.
type Board struct {
	Version int      `json:"version"`
	Columns []Column `json:"columns"`

	Extra map[string]json.RawMessage `json:"-"`
}

// Column is one kanban column: a status value and its display title.
type Column struct {
	Status string `json:"status"`
	Title  string `json:"title"`
}

// DefaultBoard returns the columns pine init writes.
func DefaultBoard() *Board {
	return &Board{
		Version: BoardVersion,
		Columns: []Column{
			{Status: "todo", Title: "Todo"},
			{Status: "doing", Title: "Doing"},
			{Status: "testing", Title: "Testing"},
			{Status: "done", Title: "Done"},
		},
	}
}

// LoadBoard reads and parses board.json from path.
func LoadBoard(path string) (*Board, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseBoard(data)
}

// ParseBoard decodes board JSON, retaining unknown keys.
func ParseBoard(data []byte) (*Board, error) {
	var all map[string]json.RawMessage
	if err := json.Unmarshal(data, &all); err != nil {
		return nil, fmt.Errorf("board.json is not valid JSON: %w", err)
	}
	b := DefaultBoard()
	b.Columns = nil // replaced if present; otherwise validated as empty
	for key, raw := range all {
		switch key {
		case "version":
			_ = json.Unmarshal(raw, &b.Version)
		case "columns":
			_ = json.Unmarshal(raw, &b.Columns)
		default:
			if b.Extra == nil {
				b.Extra = map[string]json.RawMessage{}
			}
			b.Extra[key] = raw
		}
	}
	if b.Columns == nil {
		b.Columns = DefaultBoard().Columns
	}
	return b, nil
}

// MarshalJSON emits board in canonical key order with unknown keys appended.
func (b *Board) MarshalJSON() ([]byte, error) {
	return orderedJSON([]kv{
		{"version", b.Version},
		{"columns", b.Columns},
	}, b.Extra)
}

// Bytes returns the serialized board for atomic writing.
func (b *Board) Bytes() ([]byte, error) { return json.Marshal(b) }

// Validate checks structural invariants.
func (b *Board) Validate() []string {
	var problems []string
	if b.Version <= 0 {
		problems = append(problems, "board.version must be >= 1")
	}
	if len(b.Columns) == 0 {
		problems = append(problems, "board.columns must not be empty")
	}
	seen := map[string]bool{}
	for _, c := range b.Columns {
		if strings.TrimSpace(c.Status) == "" {
			problems = append(problems, "board column has an empty status")
			continue
		}
		if seen[c.Status] {
			problems = append(problems, fmt.Sprintf("board has a duplicate column status %q", c.Status))
		}
		seen[c.Status] = true
		if strings.TrimSpace(c.Title) == "" {
			problems = append(problems, fmt.Sprintf("board column %q has an empty title", c.Status))
		}
	}
	return problems
}

// HasStatus reports whether status matches a column.
func (b *Board) HasStatus(status string) bool {
	for _, c := range b.Columns {
		if c.Status == status {
			return true
		}
	}
	return false
}

// FirstStatus returns the status of the first column (the default column for a
// ticket with no/unknown status), or "todo" if the board somehow has none.
func (b *Board) FirstStatus() string {
	if len(b.Columns) > 0 {
		return b.Columns[0].Status
	}
	return "todo"
}

// Statuses returns the ordered list of column statuses.
func (b *Board) Statuses() []string {
	out := make([]string, len(b.Columns))
	for i, c := range b.Columns {
		out[i] = c.Status
	}
	return out
}
