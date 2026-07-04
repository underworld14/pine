package ticket

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// idPattern matches a ticket ID like "BUG-001": an uppercase prefix followed by
// a hyphen and a run of digits. The prefix must start with a letter.
var idPattern = regexp.MustCompile(`^([A-Z][A-Z0-9]*)-([0-9]+)$`)

// ValidID reports whether id is a well-formed ticket ID.
func ValidID(id string) bool {
	return idPattern.MatchString(id)
}

// SplitID breaks a ticket ID into its prefix and numeric parts.
func SplitID(id string) (prefix string, n int, err error) {
	m := idPattern.FindStringSubmatch(id)
	if m == nil {
		return "", 0, fmt.Errorf("invalid ticket id %q", id)
	}
	n, err = strconv.Atoi(m[2])
	if err != nil {
		return "", 0, fmt.Errorf("invalid ticket id %q: %w", id, err)
	}
	return m[1], n, nil
}

// PrefixOf returns the prefix portion of a ticket ID (e.g. "BUG" for "BUG-001"),
// or "" when the ID is malformed.
func PrefixOf(id string) string {
	prefix, _, err := SplitID(id)
	if err != nil {
		return ""
	}
	return prefix
}

// FormatID builds a ticket ID from a prefix and number, zero-padding the number
// to a minimum width of three digits (BUG-001, and naturally BUG-1000).
func FormatID(prefix string, n int) string {
	return fmt.Sprintf("%s-%03d", strings.ToUpper(prefix), n)
}
