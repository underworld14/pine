package ticket

import (
	"regexp"
	"strings"
)

// checkboxRe matches a markdown task-list item: an optional indent, a bullet,
// then a [ ]/[x]/[X] box followed by non-empty text.
var checkboxRe = regexp.MustCompile(`^(\s*[-*]\s+)\[([ xX])\]\s+\S`)

// AcceptanceProgress counts checked and total checkbox items under the
// "Acceptance Criteria" section. Returns 0,0 when the section is absent or empty.
func AcceptanceProgress(body string) (done, total int) {
	content, ok := SectionContent(body, "Acceptance Criteria")
	if !ok {
		return 0, 0
	}
	for _, line := range strings.Split(content, "\n") {
		m := checkboxRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		total++
		if m[2] == "x" || m[2] == "X" {
			done++
		}
	}
	return done, total
}

// SetChecklistItem sets the index-th checkbox in body (document order, 0-based)
// to checked. It preserves the line's indentation, bullet, and text. It is
// idempotent. ok is false when index is out of range.
func SetChecklistItem(body string, index int, checked bool) (string, bool) {
	lines := strings.Split(body, "\n")
	n := -1
	for i, line := range lines {
		m := checkboxRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		n++
		if n != index {
			continue
		}
		mark := " "
		if checked {
			mark = "x"
		}
		// m[1] is the "indent+bullet" prefix; replace only the "[.]" box (3 chars).
		rest := line[len(m[1])+3:]
		lines[i] = m[1] + "[" + mark + "]" + rest
		return strings.Join(lines, "\n"), true
	}
	return body, false
}
