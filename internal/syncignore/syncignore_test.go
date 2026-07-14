package syncignore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWritePineGitignore_Defaults(t *testing.T) {
	dir := t.TempDir()
	prefs := Default()
	if err := WritePineGitignore(dir, prefs); err != nil {
		t.Fatal(err)
	}
	got := read(t, filepath.Join(dir, ".gitignore"))
	if !strings.Contains(got, beginMarker) || !strings.Contains(got, endMarker) {
		t.Fatalf("missing markers:\n%s", got)
	}
	if !strings.Contains(got, "# tickets=on attachments=off") {
		t.Fatalf("missing meta:\n%s", got)
	}
	if !strings.Contains(got, "attachments/") {
		t.Fatalf("expected attachments/ ignore:\n%s", got)
	}
	if strings.Contains(got, "tickets/") {
		t.Fatalf("tickets should be tracked by default:\n%s", got)
	}
}

func TestWritePineGitignore_TicketsOff(t *testing.T) {
	dir := t.TempDir()
	prefs := Prefs{Tickets: false, Attachments: false}
	if err := WritePineGitignore(dir, prefs); err != nil {
		t.Fatal(err)
	}
	got := read(t, filepath.Join(dir, ".gitignore"))
	if !strings.Contains(got, "tickets/") {
		t.Fatalf("expected tickets/ ignore:\n%s", got)
	}
	if !strings.Contains(got, "attachments/") {
		t.Fatalf("expected attachments/ ignore:\n%s", got)
	}
	if !strings.Contains(got, "# tickets=off attachments=off") {
		t.Fatalf("meta mismatch:\n%s", got)
	}
}

func TestWritePineGitignore_BothTracked(t *testing.T) {
	dir := t.TempDir()
	prefs := Prefs{Tickets: true, Attachments: true}
	if err := WritePineGitignore(dir, prefs); err != nil {
		t.Fatal(err)
	}
	got := read(t, filepath.Join(dir, ".gitignore"))
	if strings.Contains(got, "tickets/") || strings.Contains(got, "attachments/") {
		t.Fatalf("neither path should be ignored:\n%s", got)
	}
	if !strings.Contains(got, "# tickets=on attachments=on") {
		t.Fatalf("meta mismatch:\n%s", got)
	}
}

func TestWritePineGitignore_PreservesUserLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitignore")
	initial := "tmp/\n# pine:sync begin\n# tickets=on attachments=off\nattachments/\n# pine:sync end\n# keep-me\n"
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WritePineGitignore(dir, Prefs{Tickets: false, Attachments: true}); err != nil {
		t.Fatal(err)
	}
	got := read(t, path)
	if !strings.Contains(got, "tmp/") {
		t.Fatalf("lost user line before block:\n%s", got)
	}
	if !strings.Contains(got, "# keep-me") {
		t.Fatalf("lost user line after block:\n%s", got)
	}
	if !strings.Contains(got, "tickets/") {
		t.Fatalf("expected tickets/ after rewrite:\n%s", got)
	}
	if strings.Contains(got, "attachments/") {
		t.Fatalf("attachments should not be ignored:\n%s", got)
	}
	// Managed block should appear only once.
	if strings.Count(got, beginMarker) != 1 {
		t.Fatalf("duplicate managed blocks:\n%s", got)
	}
}

func TestWritePineGitignore_AppendsWhenNoBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(path, []byte("local-cache/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WritePineGitignore(dir, Default()); err != nil {
		t.Fatal(err)
	}
	got := read(t, path)
	if !strings.HasPrefix(strings.TrimSpace(got), "local-cache/") {
		t.Fatalf("user content should come first:\n%s", got)
	}
	if !strings.Contains(got, "attachments/") {
		t.Fatalf("expected managed ignore:\n%s", got)
	}
}

func TestParseManagedBlock(t *testing.T) {
	prefs := ParseManagedBlock("# pine:sync begin\n# tickets=off attachments=on\ntickets/\n# pine:sync end\n")
	if prefs.Tickets || !prefs.Attachments {
		t.Fatalf("got %+v", prefs)
	}
	def := ParseManagedBlock("no managed block here\n")
	if !def.Tickets || def.Attachments {
		t.Fatalf("default expected, got %+v", def)
	}
}

func TestParseManagedBlock_PathFallbackAndMalformed(t *testing.T) {
	// No meta line — infer from path entries inside the block.
	prefs := ParseManagedBlock("# pine:sync begin\ntickets/\nattachments\n# pine:sync end\n")
	if prefs.Tickets || prefs.Attachments {
		t.Fatalf("path fallback: got %+v", prefs)
	}
	// Begin without end → treat as absent managed block.
	def := ParseManagedBlock("# pine:sync begin\nattachments/\n")
	if !def.Tickets || def.Attachments {
		t.Fatalf("malformed should default, got %+v", def)
	}
	// Meta tokens without '=' are skipped.
	prefs = ParseManagedBlock("# pine:sync begin\n# tickets=on attachments=off stray\nattachments/\n# pine:sync end\n")
	if !prefs.Tickets || prefs.Attachments {
		t.Fatalf("meta with stray token: got %+v", prefs)
	}
}

func TestWritePineGitignore_MalformedBlockAndBareAfter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitignore")
	// Begin marker without end: rewrite should still produce a clean block.
	if err := os.WriteFile(path, []byte("keep/\n# pine:sync begin\nold\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WritePineGitignore(dir, Default()); err != nil {
		t.Fatal(err)
	}
	got := read(t, path)
	if !strings.Contains(got, "keep/") || !strings.Contains(got, endMarker) {
		t.Fatalf("malformed rewrite failed:\n%s", got)
	}

	// After-block content without trailing newline.
	if err := os.WriteFile(path, []byte("# pine:sync begin\n# tickets=on attachments=off\nattachments/\n# pine:sync end\nafter"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WritePineGitignore(dir, Prefs{Tickets: true, Attachments: true}); err != nil {
		t.Fatal(err)
	}
	got = read(t, path)
	if !strings.Contains(got, "after") || !strings.HasSuffix(got, "\n") {
		t.Fatalf("after block should be preserved with newline:\n%q", got)
	}
}

func TestWritePineGitignore_WriteError(t *testing.T) {
	dir := t.TempDir()
	// Make .gitignore a directory so WriteFile fails.
	if err := os.Mkdir(filepath.Join(dir, ".gitignore"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WritePineGitignore(dir, Default()); err == nil {
		t.Fatal("expected write error")
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
