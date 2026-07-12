package ticket

import (
	"crypto/rand"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// idPattern matches a ticket ID: an uppercase prefix, a hyphen, and a suffix
// that is either the old sequential number (BUG-001) or a Crockford-base32 hash
// (BUG-7f3k2a). The suffix class is exactly what Pine generates (digits, or the
// lowercase base32 alphabet excluding i/l/o/u), so descriptive filenames like
// TODO-list.md are not mistaken for tickets.
var idPattern = regexp.MustCompile(`^([A-Z][A-Z0-9]*)-([0-9a-hj-km-np-tv-z]+)$`)

// numericSuffix matches the sequential form's suffix (digits only).
var numericSuffix = regexp.MustCompile(`^[0-9]+$`)

// scanPattern is the unanchored counterpart of idPattern, used to pull ticket
// IDs out of free text (commit messages, prose). The leading (?:^|[^A-Za-z0-9])
// stands in for a word boundary RE2 can't express directly, so "aBUG-1" (an ID
// glued to a preceding letter) is not mistaken for a reference.
var scanPattern = regexp.MustCompile(`(?:^|[^A-Za-z0-9])([A-Z][A-Z0-9]*-[0-9a-hj-km-np-tv-z]+)`)

// ScanIDs extracts every well-formed ticket ID embedded in text, de-duplicated
// and in first-seen order. Unlike ValidID (which validates a whole string), this
// finds IDs surrounded by other characters.
func ScanIDs(text string) []string {
	matches := scanPattern.FindAllStringSubmatch(text, -1)
	if matches == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, m := range matches {
		id := m[1]
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// ValidID reports whether id is a well-formed ticket ID (either form).
func ValidID(id string) bool {
	return idPattern.MatchString(id)
}

// IsSequentialID reports whether id uses the legacy sequential form — a purely
// numeric suffix shorter than a hash suffix. Hash suffixes are always exactly
// SuffixLen characters, so a shorter all-digit suffix is unambiguously
// sequential (reaching a 6-digit sequential number would take 100k tickets).
// Cross-branch aggregation uses this to refuse to merge collision-prone
// sequential IDs even when a stale config claims idStyle "hash".
func IsSequentialID(id string) bool {
	m := idPattern.FindStringSubmatch(id)
	if m == nil {
		return false
	}
	return numericSuffix.MatchString(m[2]) && len(m[2]) < SuffixLen
}

// PrefixOf returns the prefix portion of a ticket ID (e.g. "BUG" for both
// "BUG-001" and "BUG-7f3k2a"), or "" when the ID is malformed.
func PrefixOf(id string) string {
	if !ValidID(id) {
		return ""
	}
	return id[:strings.IndexByte(id, '-')]
}

// SplitID breaks a *sequential* ID into its prefix and number. It returns an
// error for hash-style IDs, which have no numeric part.
func SplitID(id string) (prefix string, n int, err error) {
	m := idPattern.FindStringSubmatch(id)
	if m == nil || !numericSuffix.MatchString(m[2]) {
		return "", 0, fmt.Errorf("not a sequential ticket id: %q", id)
	}
	n, err = strconv.Atoi(m[2])
	if err != nil {
		return "", 0, fmt.Errorf("invalid ticket id %q: %w", id, err)
	}
	return m[1], n, nil
}

// FormatID builds a sequential ID (BUG-001), zero-padded to a minimum of three
// digits (and growing naturally to BUG-1000).
func FormatID(prefix string, n int) string {
	return fmt.Sprintf("%s-%03d", strings.ToUpper(prefix), n)
}

// MakeID joins a prefix and an already-formatted suffix into an ID.
func MakeID(prefix, suffix string) string {
	return strings.ToUpper(prefix) + "-" + suffix
}

// suffixAlphabet is Crockford base32, lowercased, excluding the ambiguous
// characters i, l, o, and u.
const suffixAlphabet = "0123456789abcdefghjkmnpqrstvwxyz"

// SuffixLen is the length of a hash-style suffix. 6 base32 chars ≈ 1 billion
// values, so collisions are negligible even at thousands of tickets across
// branches.
const SuffixLen = 6

// NewSuffix returns a random hash-style suffix.
func NewSuffix() string {
	b := make([]byte, SuffixLen)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand effectively never fails; keep a valid (if fixed) suffix.
		for i := range b {
			b[i] = 0
		}
	}
	out := make([]byte, SuffixLen)
	for i, v := range b {
		out[i] = suffixAlphabet[int(v)%len(suffixAlphabet)]
	}
	return string(out)
}
