package ticket

import "testing"

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

func TestPriorityRank(t *testing.T) {
	order := DefaultPriorities
	if PriorityRank("critical", order) <= PriorityRank("low", order) {
		t.Errorf("critical should outrank low")
	}
	if PriorityRank("bogus", order) != PriorityRank("medium", order) {
		t.Errorf("unknown priority should rank as medium")
	}
}
