package learning

import "strings"

// MissingCitedPaths returns the subset of cites for which exists returns false.
// Policy: if ANY cited path is missing, the learning is citation-stale.
// exists receives the repo-relative path as stored (typically slash-separated).
func MissingCitedPaths(cites []string, exists func(rel string) bool) []string {
	if len(cites) == 0 || exists == nil {
		return nil
	}
	var missing []string
	for _, p := range cites {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !exists(p) {
			missing = append(missing, p)
		}
	}
	return missing
}

// IsCitationStale reports whether any cited path is missing.
func IsCitationStale(missing []string) bool {
	return len(missing) > 0
}
