package selfupdate

import (
	"strconv"
	"strings"
)

// NormalizeVersion strips a leading "v" and surrounding whitespace.
func NormalizeVersion(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}

// NeedsUpgrade reports whether latest is newer than current.
// Non-semver currents (empty, "dev", "*-dev", garbage) always need an upgrade
// when latest is a parseable release version.
func NeedsUpgrade(current, latest string) bool {
	cur := NormalizeVersion(current)
	lat := NormalizeVersion(latest)
	if lat == "" {
		return false
	}
	if !isReleaseSemver(cur) {
		return true
	}
	if !isReleaseSemver(lat) {
		return false
	}
	return compareSemver(cur, lat) < 0
}

func isReleaseSemver(v string) bool {
	if v == "" || v == "dev" {
		return false
	}
	if strings.HasSuffix(v, "-dev") {
		return false
	}
	parts := strings.Split(v, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return false
	}
	for _, p := range parts {
		// Allow pre-release suffix on the last component (e.g. 1.2.3-rc.1)
		// by only accepting pure numeric parts for "release" comparison.
		if _, err := strconv.Atoi(p); err != nil {
			return false
		}
	}
	return true
}

// compareSemver returns -1 if a<b, 0 if equal, 1 if a>b. Both must be
// major.minor[.patch] with numeric components.
func compareSemver(a, b string) int {
	ap := padSemver(a)
	bp := padSemver(b)
	for i := 0; i < 3; i++ {
		ai, _ := strconv.Atoi(ap[i])
		bi, _ := strconv.Atoi(bp[i])
		if ai < bi {
			return -1
		}
		if ai > bi {
			return 1
		}
	}
	return 0
}

func padSemver(v string) [3]string {
	var out [3]string
	out[0], out[1], out[2] = "0", "0", "0"
	parts := strings.Split(v, ".")
	for i := 0; i < len(parts) && i < 3; i++ {
		out[i] = parts[i]
	}
	return out
}
