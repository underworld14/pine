package learning

import (
	"strings"
	"testing"
	"time"
)

const canonical = `---
id: LRN-001
scope: global
tags:
    - db
    - migration
source_agent: claude-code
created: 2026-07-11T10:00:00Z
---

Jangan pakai raw SQL migration, selalu lewat query builder di
internal/db/query.go — pernah bikin schema drift, lihat BUG-014.
`

func TestParseCanonical(t *testing.T) {
	l := Parse("LRN-001", []byte(canonical))
	if l.Degraded {
		t.Fatalf("unexpected degraded parse: %v", l.Warnings)
	}
	if l.Scope != ScopeGlobal {
		t.Errorf("scope = %q", l.Scope)
	}
	if len(l.Tags) != 2 || l.Tags[0] != "db" || l.Tags[1] != "migration" {
		t.Errorf("tags = %v", l.Tags)
	}
	if l.SourceAgent != SourceClaudeCode {
		t.Errorf("source_agent = %q", l.SourceAgent)
	}
	want := time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC)
	if !l.Created.Equal(want) {
		t.Errorf("created = %v want %v", l.Created, want)
	}
	if !strings.Contains(l.Body, "raw SQL migration") {
		t.Errorf("body = %q", l.Body)
	}
}

func TestParseTicketScope(t *testing.T) {
	src := `---
id: LRN-002
scope: ticket
ticket: BUG-014
tags:
    - db
source_agent: manual
created: 2026-07-11T11:00:00Z
---
insight text
`
	l := Parse("LRN-002", []byte(src))
	if l.Scope != ScopeTicket || l.Ticket != "BUG-014" {
		t.Errorf("scope/ticket = %q/%q", l.Scope, l.Ticket)
	}
}

func TestParseSupersedes(t *testing.T) {
	src := `---
id: LRN-010
scope: global
source_agent: manual
supersedes: LRN-003
created: 2026-07-11T12:00:00Z
---
new rule
`
	l := Parse("LRN-010", []byte(src))
	if l.Supersedes != "LRN-003" {
		t.Errorf("supersedes = %q", l.Supersedes)
	}
	out := string(l.Serialize())
	if !strings.Contains(out, "supersedes: LRN-003") {
		t.Errorf("serialize missing supersedes:\n%s", out)
	}
}

func TestParseCites(t *testing.T) {
	src := `---
id: LRN-020
scope: global
source_agent: manual
cites:
  - internal/webhook/retry.go
  - internal/db/query.go
created: 2026-07-11T12:00:00Z
---
race condition note
`
	l := Parse("LRN-020", []byte(src))
	if len(l.Cites) != 2 || l.Cites[0] != "internal/webhook/retry.go" || l.Cites[1] != "internal/db/query.go" {
		t.Errorf("cites = %v", l.Cites)
	}
	out := string(l.Serialize())
	if !strings.Contains(out, "internal/webhook/retry.go") || !strings.Contains(out, "cites:") {
		t.Errorf("serialize missing cites:\n%s", out)
	}
}

func TestParseCitesScalarWrapped(t *testing.T) {
	src := "---\nid: LRN-021\nscope: global\ncites: internal/foo.go\nsource_agent: manual\ncreated: 2026-07-11T00:00:00Z\n---\nx\n"
	l := Parse("LRN-021", []byte(src))
	if len(l.Cites) != 1 || l.Cites[0] != "internal/foo.go" {
		t.Errorf("cites = %v", l.Cites)
	}
}

func TestParseSupersedesListWarns(t *testing.T) {
	src := `---
id: LRN-011
scope: global
source_agent: manual
supersedes:
  - LRN-001
  - LRN-002
created: 2026-07-11T12:00:00Z
---
x
`
	l := Parse("LRN-011", []byte(src))
	if l.Supersedes != "" {
		t.Errorf("multi-element supersedes should be ignored, got %q", l.Supersedes)
	}
	if len(l.Warnings) == 0 {
		t.Error("expected warning for list supersedes")
	}
}

func TestParseSupersededByNotPersisted(t *testing.T) {
	src := `---
id: LRN-012
scope: global
source_agent: manual
superseded_by: LRN-099
created: 2026-07-11T12:00:00Z
---
x
`
	l := Parse("LRN-012", []byte(src))
	out := string(l.Serialize())
	if strings.Contains(out, "superseded_by") {
		t.Errorf("superseded_by must not round-trip to disk:\n%s", out)
	}
	for _, e := range l.Extra {
		if e.Key == "superseded_by" {
			t.Error("superseded_by must not land in Extra")
		}
	}
}

func TestRoundTripBodyByteIdentical(t *testing.T) {
	l := Parse("LRN-001", []byte(canonical))
	out := l.Serialize()
	l2 := Parse("LRN-001", out)
	if l2.Body != l.Body {
		t.Errorf("body changed on round-trip:\n%q\nvs\n%q", l.Body, l2.Body)
	}
	if string(l2.Serialize()) != string(out) {
		t.Errorf("serialize not idempotent")
	}
}

func TestParseCRLFAndBOM(t *testing.T) {
	crlf := "\ufeff" + strings.ReplaceAll(canonical, "\n", "\r\n")
	l := Parse("LRN-001", []byte(crlf))
	if l.Degraded {
		t.Fatalf("CRLF/BOM should parse: %v", l.Warnings)
	}
}

func TestUnknownKeysPreserved(t *testing.T) {
	src := `---
id: LRN-003
scope: global
reviewed: true
source_agent: manual
created: 2026-07-11T00:00:00Z
---
body
`
	l := Parse("LRN-003", []byte(src))
	if len(l.Extra) != 1 {
		t.Fatalf("expected 1 extra key, got %d", len(l.Extra))
	}
	out := string(l.Serialize())
	if !strings.Contains(out, "reviewed: true") {
		t.Errorf("extra key not preserved:\n%s", out)
	}
}

func TestScalarTagWrapped(t *testing.T) {
	src := "---\nid: LRN-004\nscope: global\ntags: db\nsource_agent: manual\ncreated: 2026-07-11T00:00:00Z\n---\nb\n"
	l := Parse("LRN-004", []byte(src))
	if len(l.Tags) != 1 || l.Tags[0] != "db" {
		t.Errorf("tags = %v", l.Tags)
	}
	if len(l.Warnings) == 0 {
		t.Errorf("expected a warning about scalar tag")
	}
}

func TestDegradedOnBadYAML(t *testing.T) {
	src := "---\nid: LRN-005\n scope: bad\n\tstatus: [unclosed\n---\nbody\n"
	l := Parse("LRN-005", []byte(src))
	if !l.Degraded {
		t.Errorf("expected degraded parse for malformed YAML")
	}
}

func TestNoFrontmatterDegraded(t *testing.T) {
	l := Parse("LRN-006", []byte("# just a note\nno frontmatter here\n"))
	if !l.Degraded {
		t.Errorf("expected degraded parse when no frontmatter")
	}
}

func TestParseFrontmatterNotMapping(t *testing.T) {
	// Valid YAML, but the root node is a sequence, not a mapping.
	src := "---\n- a\n- b\n---\nbody\n"
	l := Parse("LRN-007", []byte(src))
	if l.Degraded {
		t.Errorf("a non-mapping frontmatter should not be degraded, just left at defaults: %v", l.Warnings)
	}
	if l.Scope != ScopeGlobal || l.SourceAgent != SourceManual {
		t.Errorf("expected default scope/source_agent, got scope=%q source_agent=%q", l.Scope, l.SourceAgent)
	}
	if len(l.Warnings) == 0 {
		t.Error("expected a warning that frontmatter is not a mapping")
	}
}

func TestValidScopeAndSourceAgent(t *testing.T) {
	if !ValidScope("global") || !ValidScope("ticket") {
		t.Error("valid scopes rejected")
	}
	if ValidScope("component") {
		t.Error("component scope should not be valid in v1")
	}
	if !ValidSourceAgent("claude-code") || !ValidSourceAgent("manual") {
		t.Error("valid source agents rejected")
	}
	if ValidSourceAgent("unknown") {
		t.Error("unknown source agent should be invalid")
	}
}
