package learning

import (
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/underworld14/pine/internal/frontmatter"
)

var knownKeys = map[string]bool{
	"id": true, "scope": true, "tags": true, "ticket": true,
	"component": true, "source_agent": true, "supersedes": true,
	"superseded_by": true, "cites": true, "created": true,
}

// Parse reads a learning file. id is the canonical identifier from the filename.
// Parse is lenient: it never returns an error.
func Parse(id string, raw []byte) *Learning {
	l := &Learning{ID: id, Scope: ScopeGlobal, SourceAgent: SourceManual}

	fm, body, ok := frontmatter.Split(string(raw))
	if !ok {
		l.Degraded = true
		l.Body = string(raw)
		l.Warnings = append(l.Warnings, "no YAML frontmatter delimiters found")
		return l
	}
	l.Body = body

	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(fm), &doc); err != nil {
		l.Degraded = true
		l.Body = string(raw)
		l.Warnings = append(l.Warnings, "frontmatter is not valid YAML: "+err.Error())
		return l
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		if strings.TrimSpace(fm) != "" {
			l.Warnings = append(l.Warnings, "frontmatter is not a mapping")
		}
		return l
	}

	m := doc.Content[0]
	for i := 0; i+1 < len(m.Content); i += 2 {
		key := m.Content[i].Value
		val := m.Content[i+1]
		switch key {
		case "id":
			l.FrontmatterID = strings.TrimSpace(val.Value)
		case "scope":
			l.Scope = strings.ToLower(strings.TrimSpace(val.Value))
		case "tags":
			l.Tags = frontmatter.DecodeStringList(val, func(msg string) {
				l.Warnings = append(l.Warnings, "tags "+msg)
			})
			for i := range l.Tags {
				l.Tags[i] = strings.ToLower(strings.TrimSpace(l.Tags[i]))
			}
		case "ticket":
			l.Ticket = strings.TrimSpace(val.Value)
		case "component":
			l.Component = strings.TrimSpace(val.Value)
		case "source_agent":
			l.SourceAgent = strings.ToLower(strings.TrimSpace(val.Value))
		case "supersedes":
			l.Supersedes = decodeSupersedes(val, l)
		case "superseded_by":
			// Derived at read time only; ignore any on-disk value (do not Extra).
		case "cites":
			l.Cites = frontmatter.DecodeStringList(val, func(msg string) {
				l.Warnings = append(l.Warnings, "cites "+msg)
			})
			for i := range l.Cites {
				l.Cites[i] = strings.TrimSpace(l.Cites[i])
			}
		case "created":
			l.Created = frontmatter.ParseTime(val.Value)
		default:
			if !knownKeys[key] {
				l.Extra = append(l.Extra, ExtraField{Key: key, Node: val})
			}
		}
	}
	if l.Scope == "" {
		l.Scope = ScopeGlobal
	}
	if l.SourceAgent == "" {
		l.SourceAgent = SourceManual
	}
	return l
}

// Serialize renders a learning back to bytes. Degraded learnings must not be serialized.
func (l *Learning) Serialize() []byte {
	m := &yaml.Node{Kind: yaml.MappingNode}
	add := func(k string, v *yaml.Node) {
		m.Content = append(m.Content, frontmatter.Scalar(k), v)
	}
	add("id", frontmatter.Scalar(l.ID))
	add("scope", frontmatter.Scalar(l.Scope))
	if len(l.Tags) > 0 {
		add("tags", frontmatter.Seq(l.Tags))
	}
	if l.Ticket != "" {
		add("ticket", frontmatter.Scalar(l.Ticket))
	}
	if l.Component != "" {
		add("component", frontmatter.Scalar(l.Component))
	}
	add("source_agent", frontmatter.Scalar(l.SourceAgent))
	if l.Supersedes != "" {
		add("supersedes", frontmatter.Scalar(l.Supersedes))
	}
	if len(l.Cites) > 0 {
		add("cites", frontmatter.Seq(l.Cites))
	}
	add("created", frontmatter.Scalar(frontmatter.FormatTime(l.Created)))
	for _, e := range l.Extra {
		add(e.Key, e.Node)
	}

	out, err := yaml.Marshal(m)
	if err != nil {
		out = []byte("id: " + l.ID + "\n")
	}
	return []byte("---\n" + string(out) + "---\n" + l.Body)
}

// decodeSupersedes reads a single learning ID. Sequences of length 1 are accepted
// with a warning; multi-element or non-scalar shapes are ignored with a warning.
func decodeSupersedes(n *yaml.Node, l *Learning) string {
	switch n.Kind {
	case yaml.ScalarNode:
		return strings.TrimSpace(n.Value)
	case yaml.SequenceNode:
		var ids []string
		for _, c := range n.Content {
			if c.Kind == yaml.ScalarNode {
				if v := strings.TrimSpace(c.Value); v != "" {
					ids = append(ids, v)
				}
			}
		}
		if len(ids) == 1 {
			l.Warnings = append(l.Warnings, "supersedes was a list; used the single element")
			return ids[0]
		}
		l.Warnings = append(l.Warnings, "supersedes must be a single id, not a list; ignored")
		return ""
	default:
		l.Warnings = append(l.Warnings, "supersedes has an unexpected YAML shape; ignored")
		return ""
	}
}
