package ticket

import (
	"strings"
	"testing"
	"time"
)

// A ticket that carries a manual board order round-trips it byte-stably and
// places the key right after priority (before labels/created).
func TestOrderRoundTrip(t *testing.T) {
	src := "---\nid: BUG-100\ntitle: ordered\nstatus: todo\npriority: high\norder: 98304.5\nlabels:\n    - ui\ncreated: 2026-07-04T00:00:00Z\nupdated: 2026-07-04T00:00:00Z\n---\nbody\n"
	tk := Parse("BUG-100", []byte(src))
	if tk.Degraded {
		t.Fatalf("unexpected degraded parse: %v", tk.Warnings)
	}
	if tk.Order != 98304.5 {
		t.Fatalf("order = %v want 98304.5", tk.Order)
	}
	out := string(tk.Serialize())
	if !strings.Contains(out, "order: 98304.5") {
		t.Errorf("serialized order missing:\n%s", out)
	}
	if strings.Index(out, "order:") < strings.Index(out, "priority:") {
		t.Errorf("order should come after priority:\n%s", out)
	}
	if strings.Index(out, "order:") > strings.Index(out, "labels:") {
		t.Errorf("order should come before labels:\n%s", out)
	}
	// Re-parse is stable and idempotent.
	tk2 := Parse("BUG-100", []byte(out))
	if tk2.Order != tk.Order {
		t.Errorf("order changed on round-trip: %v vs %v", tk2.Order, tk.Order)
	}
	if string(tk2.Serialize()) != out {
		t.Errorf("serialize not idempotent")
	}
}

// A whole-number order still round-trips (no accidental type/precision drift).
func TestOrderWholeNumber(t *testing.T) {
	tk := &Ticket{ID: "BUG-101", Title: "t", Status: "doing", Priority: "medium", Order: 65536}
	out := string(tk.Serialize())
	if !strings.Contains(out, "order: 65536") {
		t.Errorf("whole order not serialized cleanly:\n%s", out)
	}
	if got := Parse("BUG-101", []byte(out)); got.Order != 65536 {
		t.Errorf("order = %v want 65536", got.Order)
	}
}

// Negative orders (produced by repeated drag-to-top drops) round-trip exactly.
func TestOrderNegativeRoundTrip(t *testing.T) {
	tk := &Ticket{ID: "BUG-106", Title: "t", Status: "todo", Priority: "low", Order: -32768}
	out := string(tk.Serialize())
	if !strings.Contains(out, "order: -32768") {
		t.Errorf("negative order not serialized cleanly:\n%s", out)
	}
	if got := Parse("BUG-106", []byte(out)); got.Order != -32768 {
		t.Errorf("order = %v want -32768", got.Order)
	}
}

// Unset order is omitted so untouched tickets keep a minimal frontmatter.
func TestOrderOmittedWhenUnset(t *testing.T) {
	tk := &Ticket{ID: "BUG-102", Title: "t", Status: "todo", Priority: "low"}
	out := string(tk.Serialize())
	if strings.Contains(out, "order:") {
		t.Errorf("unset order should be omitted:\n%s", out)
	}
}

// A non-numeric order is ignored leniently (warn, not degrade) — malformed
// agent-written data never crashes the board.
func TestOrderInvalidIgnored(t *testing.T) {
	src := "---\nid: BUG-103\ntitle: t\nstatus: todo\npriority: low\norder: soon\ncreated: 2026-07-04T00:00:00Z\nupdated: 2026-07-04T00:00:00Z\n---\nb\n"
	tk := Parse("BUG-103", []byte(src))
	if tk.Degraded {
		t.Fatalf("bad order should not degrade the whole ticket")
	}
	if tk.Order != 0 {
		t.Errorf("order = %v want 0", tk.Order)
	}
	if len(tk.Warnings) == 0 {
		t.Errorf("expected a warning about non-numeric order")
	}
}

// Non-finite orders (nan/inf) parse via ParseFloat but would break json.Marshal
// of the whole board response, so they must be rejected leniently (warn, unset).
func TestOrderNonFiniteRejected(t *testing.T) {
	for _, tok := range []string{"nan", "NaN", "inf", "+Inf", "-inf", "Infinity"} {
		src := "---\nid: BUG-105\ntitle: t\nstatus: todo\npriority: low\norder: " + tok +
			"\ncreated: 2026-07-04T00:00:00Z\nupdated: 2026-07-04T00:00:00Z\n---\nb\n"
		tk := Parse("BUG-105", []byte(src))
		if tk.Degraded {
			t.Fatalf("order %q should not degrade the ticket", tok)
		}
		if tk.Order != 0 {
			t.Errorf("order %q = %v want 0 (rejected)", tok, tk.Order)
		}
		if len(tk.Warnings) == 0 {
			t.Errorf("order %q expected a warning", tok)
		}
	}
}

// Merge takes a one-sided order change and flags a two-sided divergence.
func TestOrderMerge(t *testing.T) {
	base := &Ticket{ID: "BUG-104", Order: 100}
	// Only theirs changed → take theirs, no conflict.
	ours := &Ticket{ID: "BUG-104", Order: 100, Updated: time.Unix(10, 0)}
	theirs := &Ticket{ID: "BUG-104", Order: 200, Updated: time.Unix(20, 0)}
	m, conflict := Merge3(base, ours, theirs)
	if m.Order != 200 || conflict {
		t.Errorf("one-sided merge: order=%v conflict=%v want 200/false", m.Order, conflict)
	}
	// Both diverged → newer Updated wins and it's flagged.
	ours2 := &Ticket{ID: "BUG-104", Order: 150, Updated: time.Unix(30, 0)}
	theirs2 := &Ticket{ID: "BUG-104", Order: 250, Updated: time.Unix(20, 0)}
	m2, conflict2 := Merge3(base, ours2, theirs2)
	if m2.Order != 150 || !conflict2 {
		t.Errorf("two-sided merge: order=%v conflict=%v want 150/true", m2.Order, conflict2)
	}
}
