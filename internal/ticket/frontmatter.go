package ticket

import (
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/underworld14/pine/internal/frontmatter"
)

// knownKeys are the frontmatter keys Pine maps onto struct fields. Everything
// else is preserved verbatim in Ticket.Extra.
var knownKeys = map[string]bool{
	"id": true, "title": true, "status": true, "priority": true,
	"labels": true, "deps": true, "parent": true, "created": true, "updated": true,
}

// Parse reads a ticket file. id is the canonical identifier taken from the
// filename. Parse is lenient: it never returns an error. Malformed frontmatter
// yields a Degraded ticket whose whole content is treated as read-only body, so
// an AI-written file that Pine cannot understand is surfaced rather than lost.
func Parse(id string, raw []byte) *Ticket {
	t := &Ticket{ID: id, Title: id}

	fm, body, ok := frontmatter.Split(string(raw))
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
			t.Labels = frontmatter.DecodeStringList(val, func(msg string) {
				t.Warnings = append(t.Warnings, "labels "+msg)
			})
		case "deps":
			t.Deps = frontmatter.DecodeStringList(val, func(msg string) {
				t.Warnings = append(t.Warnings, "deps "+msg)
			})
		case "parent":
			t.Parent = strings.TrimSpace(val.Value)
		case "created":
			t.Created = frontmatter.ParseTime(val.Value)
		case "updated":
			t.Updated = frontmatter.ParseTime(val.Value)
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
		m.Content = append(m.Content, frontmatter.Scalar(k), v)
	}
	add("id", frontmatter.Scalar(t.ID))
	add("title", frontmatter.Scalar(t.Title))
	add("status", frontmatter.Scalar(t.Status))
	add("priority", frontmatter.Scalar(t.Priority))
	if len(t.Labels) > 0 {
		add("labels", frontmatter.Seq(t.Labels))
	}
	if len(t.Deps) > 0 {
		add("deps", frontmatter.Seq(t.Deps))
	}
	if t.Parent != "" {
		add("parent", frontmatter.Scalar(t.Parent))
	}
	add("created", frontmatter.Scalar(frontmatter.FormatTime(t.Created)))
	add("updated", frontmatter.Scalar(frontmatter.FormatTime(t.Updated)))
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
