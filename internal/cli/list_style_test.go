package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/underworld14/pine/internal/view"
)

func TestListStyleNoopByDefault(t *testing.T) {
	t.Parallel()
	var s listStyle
	if got := s.wrap("31", "hi"); got != "hi" {
		t.Fatalf("noop wrap = %q", got)
	}
	if got := s.priority("critical"); got != "● critical" {
		t.Fatalf("noop priority = %q", got)
	}
	if got := s.typeBadge("BUG"); got != "[bug]" {
		t.Fatalf("noop badge = %q", got)
	}
}

func TestListStyleColorWrap(t *testing.T) {
	t.Parallel()
	s := listStyle{color: true}
	if got := s.wrap("31", "hi"); got != "\x1b[31mhi\x1b[0m" {
		t.Fatalf("wrap = %q", got)
	}
	if got := s.wrap("31", ""); got != "" {
		t.Fatalf("empty wrap = %q", got)
	}
	if got := s.priority("high"); got != "\x1b[33m● high\x1b[0m" {
		t.Fatalf("high = %q", got)
	}
	if got := s.priority("critical"); got != "\x1b[31m● critical\x1b[0m" {
		t.Fatalf("critical = %q", got)
	}
	if got := s.priority("medium"); got != "\x1b[34m● medium\x1b[0m" {
		t.Fatalf("medium = %q", got)
	}
	if got := s.priority("low"); got != "\x1b[2m● low\x1b[0m" {
		t.Fatalf("low = %q", got)
	}
	if got := s.typeBadge("FEAT"); got != "\x1b[36m[feat]\x1b[0m" {
		t.Fatalf("feat = %q", got)
	}
	if got := s.typeBadge("TASK"); got != "\x1b[2m[task]\x1b[0m" {
		t.Fatalf("other type = %q", got)
	}
	if got := s.phase("p0"); got != "\x1b[2;36m[p0]\x1b[0m" {
		t.Fatalf("phase = %q", got)
	}
}

func TestListStyleForBufferIsPlain(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if listStyleFor(&buf).color {
		t.Fatal("non-file writer must stay plain")
	}
}

func TestListStyleForRespectsNOCOLOR(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	if listStyleFor(os.Stdout).color {
		t.Fatal("NO_COLOR must disable color even on stdout")
	}
}

func TestFormatPrettyTicketColoredStripsToPlain(t *testing.T) {
	t.Parallel()
	v := view.Ticket{
		ID: "BUG-001", Type: "BUG", Title: "Crash", Status: "todo", Priority: "critical", Phase: "p0",
		Blocked: true, Unmet: []string{"FEAT-001"},
	}
	plain := formatPrettyTicket(v)
	colored := formatPrettyTicketWith(v, listStyle{color: true})
	if !strings.Contains(colored, "\x1b[") {
		t.Fatalf("colored output missing ANSI: %q", colored)
	}
	if stripANSI(colored) != plain {
		t.Fatalf("strip mismatch\nplain: %q\nstrip: %q", plain, stripANSI(colored))
	}
	if !strings.Contains(colored, "\x1b[33m●\x1b[0m") && !strings.Contains(colored, "\x1b[31m● critical\x1b[0m") {
		t.Fatalf("expected blocked icon and/or critical priority color: %q", colored)
	}
}

func TestRenderTicketTreeColorConnectors(t *testing.T) {
	t.Parallel()
	views := []view.Ticket{
		{ID: "EPIC-1", Type: "EPIC", Title: "E", Status: "todo"},
		{ID: "FEAT-a", Type: "FEAT", Title: "A", Status: "todo", Parent: "EPIC-1"},
	}
	var buf bytes.Buffer
	renderTicketTreeWith(&buf, views, listStyle{color: true}, nil)
	out := buf.String()
	if !strings.Contains(out, "\x1b[2m└── \x1b[0m") {
		t.Fatalf("dim connector missing:\n%q", out)
	}
	if stripANSI(out) == out {
		t.Fatalf("expected ANSI in colored tree:\n%q", out)
	}
}

func TestTypeBadgeFromIDPrefix(t *testing.T) {
	t.Parallel()
	got := formatPrettyTicket(view.Ticket{ID: "FEAT-001", Title: "From id"})
	if !strings.Contains(got, "[feat]") {
		t.Fatalf("expected [feat] from ID prefix: %q", got)
	}
	custom := formatPrettyTicket(view.Ticket{ID: "TASK-001", Type: "TASK", Title: "Chore"})
	if !strings.Contains(custom, "[task]") {
		t.Fatalf("expected [task]: %q", custom)
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			if j < len(s) {
				i = j + 1
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
