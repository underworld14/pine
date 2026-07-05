package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMergeInsertAndUpdate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")

	section1 := "<!-- pine:begin recipe=agents profile=full version=0.1.0 hash=aaa -->\nold body\n<!-- pine:end -->"
	if err := MergeFile(path, section1); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "old body") {
		t.Fatalf("insert failed:\n%s", data)
	}

	// User content outside markers should survive updates.
	os.WriteFile(path, []byte("# My project\n\n"+section1+"\n\n## Custom\nnotes\n"), 0o644)

	section2 := "<!-- pine:begin recipe=agents profile=full version=0.1.0 hash=bbb -->\nnew body\n<!-- pine:end -->"
	if err := MergeFile(path, section2); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	s := string(data)
	if !strings.Contains(s, "# My project") || !strings.Contains(s, "## Custom") {
		t.Fatalf("user content lost:\n%s", s)
	}
	if !strings.Contains(s, "new body") || strings.Contains(s, "old body") {
		t.Fatalf("section not updated:\n%s", s)
	}
}

func TestRemovePreservesUserContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")
	section := "<!-- pine:begin recipe=claude profile=full version=0.1.0 hash=aaa -->\nbody\n<!-- pine:end -->"
	os.WriteFile(path, []byte("# Header\n\n"+section+"\n"), 0o644)

	ok, err := RemoveSection(path)
	if err != nil || !ok {
		t.Fatalf("remove: ok=%v err=%v", ok, err)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "pine:begin") || !strings.Contains(string(data), "# Header") {
		t.Fatalf("bad remove result:\n%s", data)
	}
}

func TestCheckStatuses(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "GEMINI.md")
	body := "section text"
	version := "0.1.0"
	hash := ContentHash(body)
	section := BeginMarker(RecipeGemini, version, hash) + "\n" + body + "\n<!-- pine:end -->"

	if st := CheckFile(path, body, RecipeGemini, version); st != StatusMissing {
		t.Fatalf("want missing, got %s", st)
	}

	os.WriteFile(path, []byte(section), 0o644)
	if st := CheckFile(path, body, RecipeGemini, version); st != StatusCurrent {
		t.Fatalf("want current, got %s", st)
	}

	if st := CheckFile(path, body, RecipeGemini, "0.2.0"); st != StatusStale {
		t.Fatalf("want stale on version, got %s", st)
	}
}

func TestRenderSectionIncludesHeader(t *testing.T) {
	section, err := RenderSection(RecipeAgents, "0.1.0", RenderOptions{BoardColumns: "todo, done"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(section, "recipe=agents") || !strings.Contains(section, "issue tracking") {
		t.Fatalf("bad section:\n%s", section)
	}
	if !strings.Contains(section, "todo, done") {
		t.Fatalf("board columns missing:\n%s", section)
	}
}
