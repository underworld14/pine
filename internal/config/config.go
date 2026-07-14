// Package config loads, validates, and saves Pine's two JSON config files:
// .pine/config.json (project settings) and .pine/board.json (kanban columns).
// Both preserve unknown top-level keys across a round-trip so that fields added
// by future versions or by hand are never dropped.
package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// ConfigVersion is the current schema version written by pine init.
const ConfigVersion = 1

// Config mirrors .pine/config.json.
type Config struct {
	Version     int          `json:"version"`
	Project     Project      `json:"project"`
	Types       []TicketType `json:"types"`
	Priorities  []string     `json:"priorities"`
	Attachments Attachments  `json:"attachments"`
	Git         Git          `json:"git"`
	IDStyle     string       `json:"idStyle"` // "hash" (default) | "sequential"
	CrossBranch CrossBranch  `json:"crossBranch"`
	Sync        Sync         `json:"sync"`
	Context     Context      `json:"context"`

	// Extra holds unknown top-level keys, preserved verbatim on save.
	Extra map[string]json.RawMessage `json:"-"`
}

type Project struct {
	Name string `json:"name"`
}

// TicketType maps an ID prefix (BUG) to a display name (Bug).
type TicketType struct {
	Prefix string `json:"prefix"`
	Name   string `json:"name"`
}

// Attachments controls the image optimizer.
type Attachments struct {
	Optimize     bool `json:"optimize"`
	MaxDimension int  `json:"maxDimension"`
	Quality      int  `json:"quality"`
	MaxVideoMB   int  `json:"maxVideoMB"`
}

// Git selects the git backend: "auto" (CLI when available, else go-git),
// "gogit", or "cli".
type Git struct {
	Backend string `json:"backend"`
}

// CrossBranch controls aggregation of tickets that live on other git branches.
// When enabled (and the repo uses hash IDs), the board shows tickets from recent
// local branches, read-only, in addition to the checked-out working tree.
// Remote (origin/*) scanning is a planned addition and not covered by v1.
type CrossBranch struct {
	Enabled          bool `json:"enabled"`          // master toggle
	ActiveBranchDays int  `json:"activeBranchDays"` // only branches touched within N days
}

// Sync controls whether tickets/ and attachments/ under .pine/ are tracked by
// git. MEMORY.md and memory/ are always tracked and are not part of Sync.
// Defaults: tickets tracked, attachments local (gitignored via .pine/.gitignore).
type Sync struct {
	Tickets     bool `json:"tickets"`
	Attachments bool `json:"attachments"`
}

// Context controls what pine context injects beyond this repository.
// GlobalMemory injects the machine-wide store at ~/.pine (see pine learn -g)
// above Project Memory; set it false in shared repos where personal
// preferences should not appear. It also gates pine serve's web UI, which
// renders context through the same generator.
type Context struct {
	GlobalMemory bool `json:"globalMemory"`
}

var prefixRe = regexp.MustCompile(`^[A-Z][A-Z0-9]*$`)

// Default returns the configuration pine init writes for a fresh project.
func Default(projectName string) *Config {
	return &Config{
		Version: ConfigVersion,
		Project: Project{Name: projectName},
		Types: []TicketType{
			{Prefix: "BUG", Name: "Bug"},
			{Prefix: "FEAT", Name: "Feature"},
			{Prefix: "EPIC", Name: "Epic"},
		},
		Priorities:  []string{"low", "medium", "high", "critical"},
		Attachments: Attachments{Optimize: true, MaxDimension: 2000, Quality: 80, MaxVideoMB: 50},
		Git:         Git{Backend: "auto"},
		IDStyle:     "hash",
		CrossBranch: CrossBranch{Enabled: true, ActiveBranchDays: 30},
		Sync:        Sync{Tickets: true, Attachments: false},
		Context:     Context{GlobalMemory: true},
	}
}

// Load reads and parses config.json from path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// Parse decodes config JSON, filling absent keys from defaults and retaining
// unknown keys. The project name default is empty; callers set it as needed.
func Parse(data []byte) (*Config, error) {
	return parseOnto(Default(""), data)
}

// ParseOnto overlays the JSON's present keys onto a copy of base, so a partial
// update (e.g. a config PUT) preserves any key the caller omitted instead of
// resetting it to a default.
func ParseOnto(base *Config, data []byte) (*Config, error) {
	return parseOnto(base.clone(), data)
}

func (c *Config) clone() *Config {
	n := *c
	n.Types = append([]TicketType(nil), c.Types...)
	n.Priorities = append([]string(nil), c.Priorities...)
	n.Extra = map[string]json.RawMessage{}
	for k, v := range c.Extra {
		n.Extra[k] = v
	}
	return &n
}

func parseOnto(c *Config, data []byte) (*Config, error) {
	var all map[string]json.RawMessage
	if err := json.Unmarshal(data, &all); err != nil {
		return nil, fmt.Errorf("config.json is not valid JSON: %w", err)
	}
	for key, raw := range all {
		switch key {
		case "version":
			_ = json.Unmarshal(raw, &c.Version)
		case "project":
			_ = json.Unmarshal(raw, &c.Project)
		case "types":
			_ = json.Unmarshal(raw, &c.Types)
		case "priorities":
			_ = json.Unmarshal(raw, &c.Priorities)
		case "attachments":
			_ = json.Unmarshal(raw, &c.Attachments)
		case "git":
			_ = json.Unmarshal(raw, &c.Git)
		case "idStyle":
			_ = json.Unmarshal(raw, &c.IDStyle)
		case "crossBranch":
			_ = json.Unmarshal(raw, &c.CrossBranch)
		case "sync":
			_ = json.Unmarshal(raw, &c.Sync)
		case "context":
			_ = json.Unmarshal(raw, &c.Context)
		default:
			if c.Extra == nil {
				c.Extra = map[string]json.RawMessage{}
			}
			c.Extra[key] = raw
		}
	}
	return c, nil
}

// MarshalJSON emits config in canonical key order with unknown keys appended,
// pretty-printed with a trailing newline for clean diffs.
func (c *Config) MarshalJSON() ([]byte, error) {
	return orderedJSON([]kv{
		{"version", c.Version},
		{"project", c.Project},
		{"types", c.Types},
		{"priorities", c.Priorities},
		{"attachments", c.Attachments},
		{"git", c.Git},
		{"idStyle", c.IDStyle},
		{"crossBranch", c.CrossBranch},
		{"sync", c.Sync},
		{"context", c.Context},
	}, c.Extra)
}

// Bytes returns the serialized config for atomic writing.
func (c *Config) Bytes() ([]byte, error) { return json.Marshal(c) }

// Validate checks structural invariants. It returns a slice of problems (empty
// when valid) so doctor can report them all at once.
func (c *Config) Validate() []string {
	var problems []string
	if c.Version <= 0 {
		problems = append(problems, "config.version must be >= 1")
	}
	if len(c.Types) == 0 {
		problems = append(problems, "config.types must not be empty")
	}
	for _, t := range c.Types {
		if !prefixRe.MatchString(t.Prefix) {
			problems = append(problems, fmt.Sprintf("config type prefix %q must be uppercase letters/digits", t.Prefix))
		}
	}
	if len(c.Priorities) == 0 {
		problems = append(problems, "config.priorities must not be empty")
	}
	switch c.Git.Backend {
	case "auto", "gogit", "cli":
	default:
		problems = append(problems, fmt.Sprintf("config.git.backend %q must be auto|gogit|cli", c.Git.Backend))
	}
	if c.Attachments.Quality < 1 || c.Attachments.Quality > 100 {
		problems = append(problems, "config.attachments.quality must be 1..100")
	}
	if c.Attachments.MaxDimension <= 0 {
		problems = append(problems, "config.attachments.maxDimension must be > 0")
	}
	switch c.IDStyle {
	case "hash", "sequential":
	default:
		problems = append(problems, fmt.Sprintf("config.idStyle %q must be hash|sequential", c.IDStyle))
	}
	if c.CrossBranch.ActiveBranchDays <= 0 {
		problems = append(problems, "config.crossBranch.activeBranchDays must be > 0")
	}
	// Note: crossBranch.enabled with idStyle "sequential" is NOT a hard error
	// (it would reject unrelated config saves); it is surfaced as a doctor
	// warning and disabled at runtime, since sequential IDs collide across
	// branches and cannot be safely merged.
	return problems
}

// TypeByPrefix returns the ticket type for an ID prefix, if configured.
func (c *Config) TypeByPrefix(prefix string) (TicketType, bool) {
	for _, t := range c.Types {
		if t.Prefix == strings.ToUpper(prefix) {
			return t, true
		}
	}
	return TicketType{}, false
}

// HasPriority reports whether p is a configured priority.
func (c *Config) HasPriority(p string) bool {
	p = strings.ToLower(p)
	for _, v := range c.Priorities {
		if v == p {
			return true
		}
	}
	return false
}

// --- shared ordered-JSON helper ---

type kv struct {
	key string
	val any
}

func orderedJSON(pairs []kv, extra map[string]json.RawMessage) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	first := true
	write := func(key string, raw []byte) {
		if !first {
			buf.WriteByte(',')
		}
		first = false
		kb, _ := json.Marshal(key)
		buf.Write(kb)
		buf.WriteByte(':')
		buf.Write(raw)
	}
	for _, p := range pairs {
		raw, err := json.Marshal(p.val)
		if err != nil {
			return nil, err
		}
		write(p.key, raw)
	}
	keys := make([]string, 0, len(extra))
	for k := range extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		write(k, extra[k])
	}
	buf.WriteByte('}')

	var out bytes.Buffer
	if err := json.Indent(&out, buf.Bytes(), "", "  "); err != nil {
		return nil, err
	}
	out.WriteByte('\n')
	return out.Bytes(), nil
}
