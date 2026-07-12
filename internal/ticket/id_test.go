package ticket

import (
	"strings"
	"testing"
)

func TestValidIDBothForms(t *testing.T) {
	for _, id := range []string{"BUG-001", "FEAT-042", "EPIC-1000", "BUG-7f3k2a", "FEAT-9m2xq4"} {
		if !ValidID(id) {
			t.Errorf("%q should be valid", id)
		}
	}
	for _, id := range []string{"bug-1", "BUG_1", "BUG-", "BUG-7F3A", "-1", "BUG-a_b",
		"BUG-list", "TODO-notes"} { // descriptive suffixes use excluded letters (i/l/o)
		if ValidID(id) {
			t.Errorf("%q should be invalid", id)
		}
	}
}

func TestIsSequentialID(t *testing.T) {
	// Sequential form: all-numeric suffix shorter than a 6-char hash.
	for _, id := range []string{"BUG-001", "FEAT-042", "EPIC-1000", "BUG-99999"} {
		if !IsSequentialID(id) {
			t.Errorf("%q should be sequential", id)
		}
	}
	// Hash form (has letters, or a full 6-char suffix) and malformed IDs are not.
	for _, id := range []string{"BUG-7f3k2a", "FEAT-9m2xq4", "BUG-012345", "garbage", "BUG-"} {
		if IsSequentialID(id) {
			t.Errorf("%q should not be sequential", id)
		}
	}
}

func TestPrefixOfBothForms(t *testing.T) {
	if PrefixOf("BUG-001") != "BUG" || PrefixOf("BUG-7f3k2a") != "BUG" || PrefixOf("EPIC-9m2xq4") != "EPIC" {
		t.Errorf("prefix mismatch")
	}
	if PrefixOf("garbage") != "" {
		t.Errorf("malformed id should yield an empty prefix")
	}
}

func TestNewSuffix(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		s := NewSuffix()
		if len(s) != SuffixLen {
			t.Fatalf("suffix length = %d, want %d", len(s), SuffixLen)
		}
		for _, c := range s {
			if !strings.ContainsRune(suffixAlphabet, c) {
				t.Fatalf("suffix char %q not in alphabet", c)
			}
		}
		if !ValidID(MakeID("BUG", s)) {
			t.Fatalf("MakeID produced an invalid id for suffix %q", s)
		}
		seen[s] = true
	}
	if len(seen) < 990 {
		t.Errorf("suffixes not random enough: %d unique of 1000", len(seen))
	}
}

func TestIDRoundTrip(t *testing.T) {
	cases := []struct {
		id     string
		prefix string
		n      int
		valid  bool
	}{
		{"BUG-001", "BUG", 1, true},
		{"FEAT-042", "FEAT", 42, true},
		{"EPIC-1000", "EPIC", 1000, true},
		{"bug-1", "", 0, false},
		{"BUG_1", "", 0, false},
		{"BUG-", "", 0, false},
		{"-1", "", 0, false},
	}
	for _, c := range cases {
		if got := ValidID(c.id); got != c.valid {
			t.Errorf("ValidID(%q) = %v want %v", c.id, got, c.valid)
		}
		if !c.valid {
			continue
		}
		prefix, n, err := SplitID(c.id)
		if err != nil || prefix != c.prefix || n != c.n {
			t.Errorf("SplitID(%q) = %q,%d,%v", c.id, prefix, n, err)
		}
		if got := FormatID(c.prefix, c.n); got != c.id {
			t.Errorf("FormatID(%q,%d) = %q want %q", c.prefix, c.n, got, c.id)
		}
	}
}

func TestFormatIDPadding(t *testing.T) {
	if got := FormatID("BUG", 7); got != "BUG-007" {
		t.Errorf("got %q", got)
	}
	if got := FormatID("bug", 7); got != "BUG-007" {
		t.Errorf("lowercase prefix should upcase: %q", got)
	}
}

func TestScanIDs(t *testing.T) {
	cases := []struct {
		text string
		want []string
	}{
		{"fixes BUG-001 and FEAT-7f3k2a", []string{"BUG-001", "FEAT-7f3k2a"}},
		{"BUG-001", []string{"BUG-001"}},
		{"See EPIC-1000, EPIC-1000 again", []string{"EPIC-1000"}}, // dedup, first-seen order
		{"nothing here", nil},
		{"lowercase bug-1 is ignored", nil},
		{"glued aBUG-1 not a ref", nil},                                // preceding letter blocks it
		{"(BUG-2) [FEAT-3]", []string{"BUG-2", "FEAT-3"}},              // bracketed
		{"BUG-1,FEAT-2;EPIC-3", []string{"BUG-1", "FEAT-2", "EPIC-3"}}, // punctuation separators
		{"trailing BUG-001.", []string{"BUG-001"}},                     // trailing period excluded
	}
	for _, c := range cases {
		got := ScanIDs(c.text)
		if len(got) != len(c.want) {
			t.Errorf("ScanIDs(%q) = %v, want %v", c.text, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("ScanIDs(%q) = %v, want %v", c.text, got, c.want)
				break
			}
		}
	}
}

func TestPriorityRank(t *testing.T) {
	order := DefaultPriorities
	if PriorityRank("critical", order) <= PriorityRank("low", order) {
		t.Errorf("critical should outrank low")
	}
	if PriorityRank("bogus", order) != PriorityRank("medium", order) {
		t.Errorf("unknown priority should rank as medium")
	}
}
