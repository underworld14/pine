package ticket

import (
	"strings"
	"testing"
	"time"
)

const canonical = `---
id: BUG-001
title: Login button not working
status: testing
priority: high
labels:
    - login
    - ui
deps:
    - FEAT-002
parent: EPIC-001
created: 2026-07-04T10:12:00Z
updated: 2026-07-04T11:00:00Z
---

# Description

The login button does nothing on click.

# Steps

1. Open /login
2. Click "Sign in"
`

func TestParseCanonical(t *testing.T) {
	tk := Parse("BUG-001", []byte(canonical))
	if tk.Degraded {
		t.Fatalf("unexpected degraded parse: %v", tk.Warnings)
	}
	if tk.Title != "Login button not working" {
		t.Errorf("title = %q", tk.Title)
	}
	if tk.Status != "testing" || tk.Priority != "high" {
		t.Errorf("status/priority = %q/%q", tk.Status, tk.Priority)
	}
	if len(tk.Labels) != 2 || tk.Labels[0] != "login" || tk.Labels[1] != "ui" {
		t.Errorf("labels = %v", tk.Labels)
	}
	if len(tk.Deps) != 1 || tk.Deps[0] != "FEAT-002" {
		t.Errorf("deps = %v", tk.Deps)
	}
	if tk.Parent != "EPIC-001" {
		t.Errorf("parent = %q", tk.Parent)
	}
	want := time.Date(2026, 7, 4, 10, 12, 0, 0, time.UTC)
	if !tk.Created.Equal(want) {
		t.Errorf("created = %v want %v", tk.Created, want)
	}
	if !strings.HasPrefix(tk.Body, "\n# Description") {
		t.Errorf("body prefix = %q", tk.Body[:20])
	}
}

func TestRoundTripBodyByteIdentical(t *testing.T) {
	tk := Parse("BUG-001", []byte(canonical))
	out := tk.Serialize()
	tk2 := Parse("BUG-001", out)
	if tk2.Body != tk.Body {
		t.Errorf("body changed on round-trip:\n%q\nvs\n%q", tk.Body, tk2.Body)
	}
	// Serializing a canonical ticket should be stable (idempotent).
	if string(tk2.Serialize()) != string(out) {
		t.Errorf("serialize not idempotent")
	}
}

func TestParseCRLFAndBOM(t *testing.T) {
	crlf := "\ufeff" + strings.ReplaceAll(canonical, "\n", "\r\n")
	tk := Parse("BUG-001", []byte(crlf))
	if tk.Degraded {
		t.Fatalf("CRLF/BOM should parse: %v", tk.Warnings)
	}
	if tk.Title != "Login button not working" {
		t.Errorf("title = %q", tk.Title)
	}
}

func TestUnknownKeysPreserved(t *testing.T) {
	src := `---
id: BUG-002
title: t
status: todo
priority: medium
assignee: claude
estimate: 3
created: 2026-07-04T00:00:00Z
updated: 2026-07-04T00:00:00Z
---
body
`
	tk := Parse("BUG-002", []byte(src))
	if len(tk.Extra) != 2 {
		t.Fatalf("expected 2 extra keys, got %d", len(tk.Extra))
	}
	out := string(tk.Serialize())
	if !strings.Contains(out, "assignee: claude") || !strings.Contains(out, "estimate: 3") {
		t.Errorf("extra keys not preserved:\n%s", out)
	}
	// Extra keys must come after the canonical block.
	if strings.Index(out, "assignee") < strings.Index(out, "updated") {
		t.Errorf("extra keys not appended after canonical keys:\n%s", out)
	}
}

func TestScalarLabelWrapped(t *testing.T) {
	src := "---\nid: BUG-003\ntitle: t\nstatus: todo\npriority: low\nlabels: login\ncreated: 2026-07-04T00:00:00Z\nupdated: 2026-07-04T00:00:00Z\n---\nb\n"
	tk := Parse("BUG-003", []byte(src))
	if len(tk.Labels) != 1 || tk.Labels[0] != "login" {
		t.Errorf("labels = %v", tk.Labels)
	}
	if len(tk.Warnings) == 0 {
		t.Errorf("expected a warning about scalar label")
	}
}

func TestDegradedOnBadYAML(t *testing.T) {
	src := "---\nid: BUG-004\n title: bad\n\tstatus: [unclosed\n---\nbody\n"
	tk := Parse("BUG-004", []byte(src))
	if !tk.Degraded {
		t.Errorf("expected degraded parse for malformed YAML")
	}
	if tk.Title != "BUG-004" {
		t.Errorf("degraded title should fall back to id, got %q", tk.Title)
	}
}

func TestNoFrontmatterDegraded(t *testing.T) {
	tk := Parse("NOTE-001", []byte("# just a note\nno frontmatter here\n"))
	if !tk.Degraded {
		t.Errorf("expected degraded parse when no frontmatter")
	}
	if !strings.Contains(tk.Body, "just a note") {
		t.Errorf("body should hold whole content")
	}
}

func TestMissingTitleFallsBackToID(t *testing.T) {
	src := "---\nid: BUG-005\nstatus: todo\npriority: medium\ncreated: 2026-07-04T00:00:00Z\nupdated: 2026-07-04T00:00:00Z\n---\nb\n"
	tk := Parse("BUG-005", []byte(src))
	if tk.Title != "BUG-005" {
		t.Errorf("title fallback = %q", tk.Title)
	}
}

func TestTitleWithColonQuoted(t *testing.T) {
	tk := &Ticket{ID: "BUG-006", Title: "Login: broken", Status: "todo", Priority: "high"}
	out := string(tk.Serialize())
	reparsed := Parse("BUG-006", []byte(out))
	if reparsed.Degraded {
		t.Fatalf("colon title produced invalid YAML:\n%s", out)
	}
	if reparsed.Title != "Login: broken" {
		t.Errorf("title = %q", reparsed.Title)
	}
}
