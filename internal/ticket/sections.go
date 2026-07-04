package ticket

import (
	"regexp"
	"strings"
)

// Section is a level-1 markdown section of a ticket body.
type Section struct {
	Heading string // text after "# ", trimmed
	Content string // body between this heading and the next, trailing newline trimmed
}

// Sections splits a body on level-1 ATX headings ("# Heading"). Content before
// the first heading is ignored.
func Sections(body string) []Section {
	var out []Section
	lines := strings.Split(body, "\n")
	var cur *Section
	var buf []string
	flush := func() {
		if cur != nil {
			cur.Content = strings.Trim(strings.Join(buf, "\n"), "\n")
			out = append(out, *cur)
		}
		buf = nil
	}
	for _, line := range lines {
		if h, ok := level1Heading(line); ok {
			flush()
			s := Section{Heading: h}
			cur = &s
			continue
		}
		if cur != nil {
			buf = append(buf, line)
		}
	}
	flush()
	return out
}

// SectionContent returns the content of the named section (case-insensitive).
func SectionContent(body, heading string) (string, bool) {
	want := strings.ToLower(strings.TrimSpace(heading))
	for _, s := range Sections(body) {
		if strings.ToLower(s.Heading) == want {
			return s.Content, true
		}
	}
	return "", false
}

func level1Heading(line string) (string, bool) {
	if strings.HasPrefix(line, "# ") {
		return strings.TrimSpace(line[2:]), true
	}
	return "", false
}

var bulletRe = regexp.MustCompile(`^\s*[-*]\s+(.*\S)\s*$`)
var mdLinkRe = regexp.MustCompile(`!?\[[^\]]*\]\(([^)]+)\)`)

// RelatedFiles extracts file paths listed as bullets under a "Related Files"
// section. Backtick-fenced paths are unwrapped.
func RelatedFiles(body string) []string {
	content, ok := SectionContent(body, "Related Files")
	if !ok {
		return nil
	}
	var out []string
	for _, line := range strings.Split(content, "\n") {
		if m := bulletRe.FindStringSubmatch(line); m != nil {
			out = append(out, strings.Trim(m[1], "`"))
		}
	}
	return out
}

// AttachmentRefs extracts attachment paths referenced under an "Attachments"
// section, from both bullets and markdown image/links.
func AttachmentRefs(body string) []string {
	content, ok := SectionContent(body, "Attachments")
	if !ok {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		p = strings.TrimSpace(strings.Trim(p, "`"))
		if p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	for _, line := range strings.Split(content, "\n") {
		if m := mdLinkRe.FindAllStringSubmatch(line, -1); m != nil {
			for _, g := range m {
				add(g[1])
			}
			continue
		}
		if m := bulletRe.FindStringSubmatch(line); m != nil {
			add(m[1])
		}
	}
	return out
}
