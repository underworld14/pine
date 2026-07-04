package ticket

import (
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// knownKeys are the frontmatter keys Pine maps onto struct fields. Everything
// else is preserved verbatim in Ticket.Extra.
var knownKeys = map[string]bool{
	"id": true, "title": true, "status": true, "priority": true,
	"labels": true, "deps": true, "parent": true, "created": true, "updated": true,
}

// timeLayouts are tried in order when parsing created/updated.
var timeLayouts = []string{
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"2006-01-02",
}

// Parse reads a ticket file. id is the canonical identifier taken from the
// filename. Parse is lenient: it never returns an error. Malformed frontmatter
// yields a Degraded ticket whose whole content is treated as read-only body, so
// an AI-written file that Pine cannot understand is surfaced rather than lost.
func Parse(id string, raw []byte) *Ticket {
	t := &Ticket{ID: id, Title: id}

	fm, body, ok := splitFrontmatter(string(raw))
	if !ok {
		t.Degraded = true
		t.Body = string(raw)
		t.Warnings = append(t.Warnings, "no YAML frontmatter delimiters found")
		return t
	}
	t.Body = body

	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(fm), &doc); err != nil {
		t.Degraded = true
		t.Body = string(raw)
		t.Warnings = append(t.Warnings, "frontmatter is not valid YAML: "+err.Error())
		return t
	}
	// Empty frontmatter (just delimiters) is valid; leave defaults.
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		if strings.TrimSpace(fm) != "" {
			t.Warnings = append(t.Warnings, "frontmatter is not a mapping")
		}
		return t
	}

	m := doc.Content[0]
	for i := 0; i+1 < len(m.Content); i += 2 {
		key := m.Content[i].Value
		val := m.Content[i+1]
		switch key {
		case "id":
			t.FrontmatterID = strings.TrimSpace(val.Value)
		case "title":
			t.Title = val.Value
		case "status":
			t.Status = strings.ToLower(strings.TrimSpace(val.Value))
		case "priority":
			t.Priority = strings.ToLower(strings.TrimSpace(val.Value))
		case "labels":
			t.Labels = decodeStringList(val, t, "labels")
		case "deps":
			t.Deps = decodeStringList(val, t, "deps")
		case "parent":
			t.Parent = strings.TrimSpace(val.Value)
		case "created":
			t.Created = parseTime(val.Value)
		case "updated":
			t.Updated = parseTime(val.Value)
		default:
			t.Extra = append(t.Extra, ExtraField{Key: key, Node: val})
		}
	}
	if t.Title == "" {
		t.Title = id
	}
	return t
}

// Serialize renders a ticket back to bytes. Known keys are written in canonical
// order (stable diffs), empty optional keys are omitted, unknown keys are
// appended verbatim, and the body is emitted byte-identically. Degraded tickets
// must never be serialized (callers guard on t.Degraded).
func (t *Ticket) Serialize() []byte {
	m := &yaml.Node{Kind: yaml.MappingNode}
	add := func(k string, v *yaml.Node) {
		m.Content = append(m.Content, scalar(k), v)
	}
	add("id", scalar(t.ID))
	add("title", scalar(t.Title))
	add("status", scalar(t.Status))
	add("priority", scalar(t.Priority))
	if len(t.Labels) > 0 {
		add("labels", seq(t.Labels))
	}
	if len(t.Deps) > 0 {
		add("deps", seq(t.Deps))
	}
	if t.Parent != "" {
		add("parent", scalar(t.Parent))
	}
	add("created", scalar(formatTime(t.Created)))
	add("updated", scalar(formatTime(t.Updated)))
	for _, e := range t.Extra {
		add(e.Key, e.Node)
	}

	out, err := yaml.Marshal(m)
	if err != nil {
		// Should never happen for string content; fall back to a minimal doc.
		out = []byte("id: " + t.ID + "\n")
	}
	return []byte("---\n" + string(out) + "---\n" + t.Body)
}

// splitFrontmatter separates the YAML frontmatter block from the body. It
// tolerates a leading BOM and CRLF line endings. The returned body is the exact
// substring after the closing delimiter line, preserving all bytes.
func splitFrontmatter(raw string) (fm, body string, ok bool) {
	raw = strings.TrimPrefix(raw, "\ufeff")
	if !strings.HasPrefix(raw, "---\n") && !strings.HasPrefix(raw, "---\r\n") {
		return "", "", false
	}
	nl := strings.IndexByte(raw, '\n')
	rest := raw[nl+1:]

	idx := 0
	for {
		lineEnd := strings.IndexByte(rest[idx:], '\n')
		var line string
		var next int
		if lineEnd == -1 {
			line = rest[idx:]
			next = len(rest)
		} else {
			line = rest[idx : idx+lineEnd]
			next = idx + lineEnd + 1
		}
		if strings.TrimRight(line, "\r") == "---" {
			return rest[:idx], rest[next:], true
		}
		if lineEnd == -1 {
			return "", "", false // no closing delimiter
		}
		idx = next
	}
}

// decodeStringList reads a YAML value as a list of strings. A scalar is wrapped
// into a one-element slice (with a warning) so that `labels: login` parses.
func decodeStringList(n *yaml.Node, t *Ticket, field string) []string {
	switch n.Kind {
	case yaml.ScalarNode:
		v := strings.TrimSpace(n.Value)
		if v == "" {
			return nil
		}
		t.Warnings = append(t.Warnings, field+" was a scalar; wrapped into a list")
		return []string{v}
	case yaml.SequenceNode:
		out := make([]string, 0, len(n.Content))
		for _, c := range n.Content {
			if c.Kind == yaml.ScalarNode {
				if v := strings.TrimSpace(c.Value); v != "" {
					out = append(out, v)
				}
			}
		}
		return out
	default:
		t.Warnings = append(t.Warnings, field+" has an unexpected YAML shape; ignored")
		return nil
	}
}

func parseTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range timeLayouts {
		if ts, err := time.Parse(layout, s); err == nil {
			return ts.UTC()
		}
	}
	return time.Time{}
}

func formatTime(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	return ts.UTC().Format(time.RFC3339)
}

// scalar builds a string scalar node with correct YAML quoting via Encode.
func scalar(val string) *yaml.Node {
	n := &yaml.Node{}
	_ = n.Encode(val)
	return n
}

// seq builds a block-style sequence of string scalars.
func seq(vals []string) *yaml.Node {
	n := &yaml.Node{Kind: yaml.SequenceNode}
	for _, v := range vals {
		n.Content = append(n.Content, scalar(v))
	}
	return n
}
