package ticket

import "testing"

func TestAcceptanceProgress(t *testing.T) {
	cases := []struct {
		name      string
		body      string
		done, tot int
	}{
		{"no section", "# Description\nhi\n", 0, 0},
		{"empty section", "# Acceptance Criteria\n\n", 0, 0},
		{"mixed", "# Acceptance Criteria\n- [x] a\n- [ ] b\n- [X] c\n", 2, 3},
		{"only AC counts", "# Acceptance Criteria\n- [x] a\n# Implementation Plan\n- [ ] step\n", 1, 1},
		{"case-insensitive heading", "# acceptance criteria\n- [ ] a\n", 0, 1},
		{"non-checkbox bullets ignored", "# Acceptance Criteria\n- plain\n- [ ] real\n", 0, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d, tot := AcceptanceProgress(c.body)
			if d != c.done || tot != c.tot {
				t.Errorf("got %d/%d, want %d/%d", d, tot, c.done, c.tot)
			}
		})
	}
}

func TestSetChecklistItem(t *testing.T) {
	body := "# Acceptance Criteria\n- [ ] a\n- [x] b\n# Impl\n  - [ ] c\n"

	nb, ok := SetChecklistItem(body, 0, true)
	if !ok || nb != "# Acceptance Criteria\n- [x] a\n- [x] b\n# Impl\n  - [ ] c\n" {
		t.Fatalf("index 0 ->true: ok=%v\n%q", ok, nb)
	}
	nb2, ok := SetChecklistItem(nb, 1, true)
	if !ok || nb2 != nb {
		t.Errorf("idempotent set failed: ok=%v changed=%v", ok, nb2 != nb)
	}
	nb3, ok := SetChecklistItem(body, 2, true)
	if !ok || nb3 != "# Acceptance Criteria\n- [ ] a\n- [x] b\n# Impl\n  - [x] c\n" {
		t.Errorf("index 2 ->true:\n%q", nb3)
	}
	if _, ok := SetChecklistItem(body, 9, true); ok {
		t.Errorf("out-of-range index should return ok=false")
	}
}
