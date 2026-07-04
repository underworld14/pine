package attach

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"regexp"
	"strings"
)

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// slug normalizes a client filename (minus extension) into a safe stem, capped
// at 48 chars. Empty names (pasted blobs) become "paste".
func slug(name string) string {
	base := filepath.Base(name)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	base = strings.ToLower(base)
	base = nonSlug.ReplaceAllString(base, "-")
	base = strings.Trim(base, "-")
	if len(base) > 48 {
		base = strings.Trim(base[:48], "-")
	}
	if base == "" {
		return "paste"
	}
	return base
}

// hashName produces a content-addressed filename: <slug>-<sha256[:8]><ext>.
// Hashing the original bytes makes the name stable across optimizer settings and
// gives free dedup (identical uploads map to the same file).
func hashName(base, ext string, orig []byte) string {
	sum := sha256.Sum256(orig)
	return base + "-" + hex.EncodeToString(sum[:])[:8] + ext
}
