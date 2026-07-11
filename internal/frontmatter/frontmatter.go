// Package frontmatter holds the YAML frontmatter parsing/serialization
// primitives shared by internal/ticket and internal/learning: splitting the
// "---" delimited block from the body, decoding string lists leniently, and
// formatting/parsing timestamps. Each caller keeps its own Parse/Serialize
// and field-mapping logic — only the generic mechanics live here.
package frontmatter

import (
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// TimeLayouts are tried in order when parsing created/updated timestamps.
var TimeLayouts = []string{
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"2006-01-02",
}

// Split separates the YAML frontmatter block from the body. It tolerates a
// leading BOM and CRLF line endings. The returned body is the exact substring
// after the closing delimiter line, preserving all bytes.
func Split(raw string) (fm, body string, ok bool) {
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

// DecodeStringList reads a YAML value as a list of strings. A scalar is
// wrapped into a one-element slice (with a warning) so that `labels: login`
// parses. warn is called with a short suffix (e.g. "was a scalar; wrapped
// into a list") for the caller to prefix with the field name.
func DecodeStringList(n *yaml.Node, warn func(msg string)) []string {
	switch n.Kind {
	case yaml.ScalarNode:
		v := strings.TrimSpace(n.Value)
		if v == "" {
			return nil
		}
		warn("was a scalar; wrapped into a list")
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
		warn("has an unexpected YAML shape; ignored")
		return nil
	}
}

// ParseTime tries each of TimeLayouts in order, returning the zero time if
// none match.
func ParseTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range TimeLayouts {
		if ts, err := time.Parse(layout, s); err == nil {
			return ts.UTC()
		}
	}
	return time.Time{}
}

// FormatTime renders a timestamp in RFC3339 (UTC), or "" for the zero value.
func FormatTime(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	return ts.UTC().Format(time.RFC3339)
}

// Scalar builds a string scalar node with correct YAML quoting via Encode.
func Scalar(val string) *yaml.Node {
	n := &yaml.Node{}
	_ = n.Encode(val)
	return n
}

// Seq builds a block-style sequence of string scalars.
func Seq(vals []string) *yaml.Node {
	n := &yaml.Node{Kind: yaml.SequenceNode}
	for _, v := range vals {
		n.Content = append(n.Content, Scalar(v))
	}
	return n
}
