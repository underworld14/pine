package ticket

import "time"

// Merge3 performs a field-aware three-way merge of two ticket versions against
// a common ancestor (base may be nil for an add/add merge). It always returns a
// valid merged ticket; conflict reports whether human review is warranted.
//
// Merge policy:
//   - Scalars (title/status/priority/parent): if only one side changed, take it;
//     if both changed differently, the side with the newer Updated wins and the
//     divergence is flagged as a conflict (the file stays valid — git marks it
//     unmerged so the chosen value can be reviewed).
//   - Sets (labels/deps): base-aware union — additions from either side are
//     kept, and a value deleted on one side is honored.
//   - created = earliest known; updated = latest of the two.
//   - Extra frontmatter keys: union by key; on a differing collision the
//     newer-Updated side wins.
//   - Body: three-way; if both sides changed it differently, the merged body
//     carries git-style conflict markers and conflict is true.
func Merge3(base, ours, theirs *Ticket) (*Ticket, bool) {
	var b Ticket
	if base != nil {
		b = *base
	}
	conflict := false

	m := &Ticket{ID: ours.ID}

	var c bool
	m.Title, c = mergeScalar(b.Title, ours.Title, theirs.Title, ours.Updated, theirs.Updated)
	conflict = conflict || c
	m.Status, c = mergeScalar(b.Status, ours.Status, theirs.Status, ours.Updated, theirs.Updated)
	conflict = conflict || c
	m.Priority, c = mergeScalar(b.Priority, ours.Priority, theirs.Priority, ours.Updated, theirs.Updated)
	conflict = conflict || c
	m.Parent, c = mergeScalar(b.Parent, ours.Parent, theirs.Parent, ours.Updated, theirs.Updated)
	conflict = conflict || c

	m.Labels = mergeSet(b.Labels, ours.Labels, theirs.Labels)
	m.Deps = mergeSet(b.Deps, ours.Deps, theirs.Deps)

	m.Created = earliest(base, ours.Created, theirs.Created)
	m.Updated = latest(ours.Updated, theirs.Updated)

	m.Extra = mergeExtra(base, ours, theirs)

	var bc bool
	m.Body, bc = mergeBody(b.Body, ours.Body, theirs.Body)
	conflict = conflict || bc

	return m, conflict
}

// mergeScalar resolves one scalar field. On a two-sided divergence, the newer
// Updated wins and conflict is true.
func mergeScalar(base, ours, theirs string, ourUpd, theirUpd time.Time) (string, bool) {
	if ours == theirs {
		return ours, false
	}
	if ours == base {
		return theirs, false // only theirs changed
	}
	if theirs == base {
		return ours, false // only ours changed
	}
	// Both changed differently — newer wins, flag for review.
	if theirUpd.After(ourUpd) {
		return theirs, true
	}
	return ours, true
}

// mergeSet is a base-aware union: keep additions from either side; drop a value
// that existed in base but was removed on exactly one side.
func mergeSet(base, ours, theirs []string) []string {
	inB, inO, inT := toSet(base), toSet(ours), toSet(theirs)
	var out []string
	seen := map[string]bool{}
	for _, x := range append(append([]string{}, ours...), theirs...) {
		if seen[x] {
			continue
		}
		seen[x] = true
		// Honor a deletion: present in base but missing from one side.
		if inB[x] && (!inO[x] || !inT[x]) {
			continue
		}
		out = append(out, x)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// mergeBody three-way merges the markdown body, emitting conflict markers when
// both sides changed it differently.
func mergeBody(base, ours, theirs string) (string, bool) {
	if ours == theirs {
		return ours, false
	}
	if ours == base {
		return theirs, false
	}
	if theirs == base {
		return ours, false
	}
	merged := "<<<<<<< ours\n" + ensureTrailingNL(ours) + "=======\n" + ensureTrailingNL(theirs) + ">>>>>>> theirs\n"
	return merged, true
}

// mergeExtra base-aware-unions extra frontmatter keys: additions from either
// side are kept, a key deleted on one side (present in base) is honored, and a
// key changed on both sides resolves to the newer-Updated value. This mirrors
// mergeSet so an intentionally removed key (e.g. a `github:` import marker) is
// not silently resurrected.
func mergeExtra(base, ours, theirs *Ticket) []ExtraField {
	inBase := map[string]bool{}
	if base != nil {
		for _, e := range base.Extra {
			inBase[e.Key] = true
		}
	}
	oursByKey := map[string]ExtraField{}
	for _, e := range ours.Extra {
		oursByKey[e.Key] = e
	}
	theirsByKey := map[string]ExtraField{}
	for _, e := range theirs.Extra {
		theirsByKey[e.Key] = e
	}
	newerIsTheirs := theirs.Updated.After(ours.Updated)

	// Iterate the newer side first for deterministic ordering, then the other.
	order := append(append([]ExtraField{}, ours.Extra...), theirs.Extra...)
	if newerIsTheirs {
		order = append(append([]ExtraField{}, theirs.Extra...), ours.Extra...)
	}
	seen := map[string]bool{}
	var out []ExtraField
	for _, e := range order {
		k := e.Key
		if seen[k] {
			continue
		}
		seen[k] = true
		_, inO := oursByKey[k]
		_, inT := theirsByKey[k]
		// Honor a deletion: the key was in base but is now missing on one side.
		if inBase[k] && (!inO || !inT) {
			continue
		}
		switch {
		case inO && inT && newerIsTheirs:
			out = append(out, theirsByKey[k])
		case inO && inT:
			out = append(out, oursByKey[k])
		case inO:
			out = append(out, oursByKey[k])
		default:
			out = append(out, theirsByKey[k])
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func earliest(base *Ticket, ours, theirs time.Time) time.Time {
	if base != nil && !base.Created.IsZero() {
		return base.Created
	}
	switch {
	case ours.IsZero():
		return theirs
	case theirs.IsZero():
		return ours
	case theirs.Before(ours):
		return theirs
	default:
		return ours
	}
}

func latest(ours, theirs time.Time) time.Time {
	if theirs.After(ours) {
		return theirs
	}
	return ours
}

func toSet(vals []string) map[string]bool {
	m := make(map[string]bool, len(vals))
	for _, v := range vals {
		m[v] = true
	}
	return m
}

func ensureTrailingNL(s string) string {
	if s == "" || s[len(s)-1] == '\n' {
		return s
	}
	return s + "\n"
}
