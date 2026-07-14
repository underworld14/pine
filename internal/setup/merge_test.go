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
	// Root injector stays short: always-on rules + pointer to the skill.
	if !strings.Contains(section, "pine learn") || !strings.Contains(section, "load the pine skill") {
		t.Fatalf("summary + skill pointer missing:\n%s", section)
	}
	if !strings.Contains(section, "pine learn -g") {
		t.Fatalf("global memory rule missing:\n%s", section)
	}
	if !strings.Contains(section, ".agents/skills/pine/SKILL.md") {
		t.Fatalf("skill path missing:\n%s", section)
	}
	// Full command catalog / learnings lifecycle belong in the skill, not the root.
	if strings.Contains(section, "Essential commands") || strings.Contains(section, "--supersedes") {
		t.Fatalf("root section should not embed the full skill body:\n%s", section)
	}
}

func TestRenderSkillIsComprehensive(t *testing.T) {
	skill := RenderSkill(RenderOptions{BoardColumns: "todo, doing, done"})
	for _, want := range []string{
		"Essential commands",
		"Persistent learnings",
		"--supersedes",
		"--cites",
		"todo, doing, done",
		"pine context",
		"Memory discipline",
		"pine learn -g",
		"~/.pine",
	} {
		if !strings.Contains(skill, want) {
			t.Fatalf("skill missing %q:\n%s", want, skill)
		}
	}
}

func TestRenderSectionUnknownRecipe(t *testing.T) {
	_, err := RenderSection(Recipe("bogus"), "0.1.0", RenderOptions{})
	if err == nil || !strings.Contains(err.Error(), "unknown setup recipe: bogus") {
		t.Fatalf("expected unknown recipe error, got %v", err)
	}
}

func TestBoardColumnsLine(t *testing.T) {
	if got := boardColumnsLine(""); got != "." {
		t.Errorf("boardColumnsLine(\"\") = %q, want %q", got, ".")
	}
	if got := boardColumnsLine("   "); got != "." {
		t.Errorf("boardColumnsLine(whitespace) = %q, want %q", got, ".")
	}
	if got := boardColumnsLine("todo, doing, done"); got != " (board columns: todo, doing, done)." {
		t.Errorf("boardColumnsLine = %q", got)
	}
}

func TestLookupUnknownRecipe(t *testing.T) {
	if _, ok := Lookup(Recipe("bogus")); ok {
		t.Fatalf("expected Lookup to report not-found for an unknown recipe")
	}
}

func TestExtractSectionNoMarkers(t *testing.T) {
	body, meta, found := ExtractSection("just some plain text, no markers here")
	if found || body != "" || meta != "" {
		t.Fatalf("expected not found for plain text, got body=%q meta=%q found=%v", body, meta, found)
	}
}

func TestExtractSectionMissingEndMarker(t *testing.T) {
	content := "<!-- pine:begin recipe=agents profile=full version=0.1.0 hash=aaa -->\nbody without end"
	body, meta, found := ExtractSection(content)
	if found {
		t.Fatalf("expected found=false when end marker missing, got body=%q meta=%q", body, meta)
	}
	if !strings.Contains(meta, "recipe=agents") {
		t.Fatalf("expected meta to still be parsed, got %q", meta)
	}
}

func TestReplaceSectionNoMarkers(t *testing.T) {
	content := "no markers at all"
	got := replaceSection(content, "<!-- pine:begin recipe=agents profile=full version=0.1.0 hash=aaa -->\nnew\n<!-- pine:end -->")
	if got != content {
		t.Fatalf("expected content unchanged when no begin marker, got %q", got)
	}
}

func TestReplaceSectionMissingEndMarker(t *testing.T) {
	content := "<!-- pine:begin recipe=agents profile=full version=0.1.0 hash=aaa -->\nbody without end"
	got := replaceSection(content, "<!-- pine:begin recipe=agents profile=full version=0.1.0 hash=bbb -->\nnew\n<!-- pine:end -->")
	if got != content {
		t.Fatalf("expected content unchanged when no end marker, got %q", got)
	}
}

func TestReplaceSectionEntireContentIsSection(t *testing.T) {
	section := "<!-- pine:begin recipe=agents profile=full version=0.1.0 hash=aaa -->\nold\n<!-- pine:end -->"
	newSection := "<!-- pine:begin recipe=agents profile=full version=0.1.0 hash=bbb -->\nnew\n<!-- pine:end -->"
	got := replaceSection(section, newSection)
	if got != newSection+"\n" {
		t.Fatalf("got %q, want %q", got, newSection+"\n")
	}
}

func TestReplaceSectionEmptyPrefixNonEmptySuffix(t *testing.T) {
	section := "<!-- pine:begin recipe=agents profile=full version=0.1.0 hash=aaa -->\nold\n<!-- pine:end -->"
	content := section + "\n\n## Trailer\nafter\n"
	newSection := "<!-- pine:begin recipe=agents profile=full version=0.1.0 hash=bbb -->\nnew\n<!-- pine:end -->"
	got := replaceSection(content, newSection)
	if !strings.HasPrefix(got, newSection) || !strings.Contains(got, "## Trailer") {
		t.Fatalf("unexpected result:\n%s", got)
	}
}

func TestReplaceSectionNonEmptyPrefixEmptySuffix(t *testing.T) {
	section := "<!-- pine:begin recipe=agents profile=full version=0.1.0 hash=aaa -->\nold\n<!-- pine:end -->"
	content := "# Header\n\n" + section
	newSection := "<!-- pine:begin recipe=agents profile=full version=0.1.0 hash=bbb -->\nnew\n<!-- pine:end -->"
	got := replaceSection(content, newSection)
	if !strings.HasPrefix(got, "# Header") || !strings.HasSuffix(strings.TrimRight(got, "\n"), "<!-- pine:end -->") {
		t.Fatalf("unexpected result:\n%s", got)
	}
	if !strings.Contains(got, "new") || strings.Contains(got, "old") {
		t.Fatalf("section body not updated:\n%s", got)
	}
}

func TestMergeFileAppendsAfterExistingUnmarkedContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")
	if err := os.WriteFile(path, []byte("# My project\n\nsome existing notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	section := "<!-- pine:begin recipe=agents profile=full version=0.1.0 hash=aaa -->\nbody\n<!-- pine:end -->"
	if err := MergeFile(path, section); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	s := string(data)
	if !strings.Contains(s, "# My project") || !strings.Contains(s, "some existing notes") {
		t.Fatalf("expected existing content preserved:\n%s", s)
	}
	if !strings.Contains(s, "pine:begin") {
		t.Fatalf("expected section appended:\n%s", s)
	}
}

func TestRemoveSectionFileNotExist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.md")
	ok, err := RemoveSection(path)
	if err != nil || ok {
		t.Fatalf("ok=%v err=%v, want ok=false err=nil", ok, err)
	}
}

func TestRemoveSectionNoMarkers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")
	os.WriteFile(path, []byte("plain content, no markers\n"), 0o644)
	ok, err := RemoveSection(path)
	if err != nil || ok {
		t.Fatalf("ok=%v err=%v, want ok=false err=nil", ok, err)
	}
}

func TestRemoveSectionMissingEndMarker(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")
	os.WriteFile(path, []byte("<!-- pine:begin recipe=agents profile=full version=0.1.0 hash=aaa -->\nbody without end\n"), 0o644)
	ok, err := RemoveSection(path)
	if err != nil || ok {
		t.Fatalf("ok=%v err=%v, want ok=false err=nil", ok, err)
	}
}

func TestRemoveSectionEntireFileIsSection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")
	section := "<!-- pine:begin recipe=agents profile=full version=0.1.0 hash=aaa -->\nbody\n<!-- pine:end -->"
	os.WriteFile(path, []byte(section+"\n"), 0o644)

	ok, err := RemoveSection(path)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v, want ok=true err=nil", ok, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected file to be removed when section was the whole content, stat err=%v", err)
	}
}

func TestRemoveSectionEmptyPrefixNonEmptySuffix(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")
	section := "<!-- pine:begin recipe=agents profile=full version=0.1.0 hash=aaa -->\nbody\n<!-- pine:end -->"
	os.WriteFile(path, []byte(section+"\n\n## After\ntext\n"), 0o644)

	ok, err := RemoveSection(path)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	data, _ := os.ReadFile(path)
	s := string(data)
	if strings.Contains(s, "pine:begin") || !strings.Contains(s, "## After") {
		t.Fatalf("unexpected result:\n%s", s)
	}
}

func TestCheckFileNoMarker(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")
	os.WriteFile(path, []byte("plain content\n"), 0o644)
	if st := CheckFile(path, "body", RecipeAgents, "0.1.0"); st != StatusMissing {
		t.Fatalf("want missing, got %s", st)
	}
}

func TestCheckFileWrongRecipe(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")
	body := "section text"
	hash := ContentHash(body)
	section := BeginMarker(RecipeAgents, "0.1.0", hash) + "\n" + body + "\n<!-- pine:end -->"
	os.WriteFile(path, []byte(section), 0o644)

	if st := CheckFile(path, body, RecipeClaude, "0.1.0"); st != StatusStale {
		t.Fatalf("want stale for recipe mismatch, got %s", st)
	}
}

func TestCheckFileHashMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")
	body := "section text"
	// Marker hash doesn't correspond to the body actually present.
	section := BeginMarker(RecipeAgents, "0.1.0", "deadbeef") + "\n" + body + "\n<!-- pine:end -->"
	os.WriteFile(path, []byte(section), 0o644)

	if st := CheckFile(path, body, RecipeAgents, "0.1.0"); st != StatusStale {
		t.Fatalf("want stale for hash mismatch, got %s", st)
	}
}
