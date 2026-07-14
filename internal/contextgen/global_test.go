package contextgen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/underworld14/pine/internal/memory"
)

// TestMain pins PINE_HOME to a throwaway dir for the whole package, so a test
// that forgets its own t.Setenv can never reach the developer's real ~/.pine.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "pine-home-")
	if err != nil {
		panic(err)
	}
	os.Setenv(memory.EnvHome, dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// seedGlobal points PINE_HOME at a fresh dir and writes body to its MEMORY.md.
func seedGlobal(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(memory.EnvHome, dir)
	if _, err := memory.EnsureGlobalLayout(); err != nil {
		t.Fatal(err)
	}
	if body != "" {
		if err := os.WriteFile(memory.MemoryPath(dir), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestFormatMemoryBlockIncludesGlobalPreferences(t *testing.T) {
	seedGlobal(t, "# Personal memory\n\n## Log\n- I use pnpm, never npm\n")
	s := scaffold(t)
	if err := memory.AppendMEMORY(s.Root(), memory.AppendOpts{Text: "project rule here"}); err != nil {
		t.Fatal(err)
	}
	md := FormatMemoryBlock(s, nil, nil, 3)
	for _, want := range []string{
		"## Your Preferences (global)",
		"If anything here conflicts with Project Memory, Project Memory wins.",
		"I use pnpm, never npm",
		"## Project Memory",
		"project rule here",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in:\n%s", want, md)
		}
	}
}

func TestFormatMemoryBlockOrdersGlobalBeforeProject(t *testing.T) {
	seedGlobal(t, "# Personal memory\n\n## Log\n- global pref\n")
	s := scaffold(t)
	if err := memory.AppendMEMORY(s.Root(), memory.AppendOpts{Text: "project rule"}); err != nil {
		t.Fatal(err)
	}
	if err := memory.AppendTopic(s.Root(), "analytics", memory.AppendOpts{Text: "topic body"}); err != nil {
		t.Fatal(err)
	}
	md := FormatMemoryBlock(s, nil, nil, 3)
	g := strings.Index(md, "## Your Preferences (global)")
	p := strings.Index(md, "## Project Memory")
	tp := strings.Index(md, "## Memory Topics")
	if g < 0 || p < 0 || tp < 0 {
		t.Fatalf("missing a section:\n%s", md)
	}
	if !(g < p && p < tp) {
		t.Errorf("want global < project < topics, got %d/%d/%d:\n%s", g, p, tp, md)
	}
}

func TestFormatMemoryBlockListsGlobalTopicsByName(t *testing.T) {
	dir := seedGlobal(t, "# Personal memory\n\n## Log\n- pref\n")
	for _, tc := range []struct{ slug, body string }{
		{"pnpm", "GLOBAL_TOPIC_BODY_SENTINEL_A"},
		{"git-habits", "GLOBAL_TOPIC_BODY_SENTINEL_B"},
	} {
		if err := memory.AppendTopic(dir, tc.slug, memory.AppendOpts{Text: tc.body}); err != nil {
			t.Fatal(err)
		}
	}
	s := scaffold(t)
	md := FormatMemoryBlock(s, nil, nil, 3)
	if !strings.Contains(md, "Global topics: git-habits, pnpm") {
		t.Errorf("want global topics listed by name:\n%s", md)
	}
	if !strings.Contains(md, "if relevant") {
		t.Errorf("want the read-on-demand hint:\n%s", md)
	}
	// Decision 6: names only. Bodies must never be inlined.
	for _, sentinel := range []string{"GLOBAL_TOPIC_BODY_SENTINEL_A", "GLOBAL_TOPIC_BODY_SENTINEL_B"} {
		if strings.Contains(md, sentinel) {
			t.Errorf("global topic body must not be inlined (%s):\n%s", sentinel, md)
		}
	}
}

func TestFormatMemoryBlockNeverCreatesGlobalStore(t *testing.T) {
	// The central invariant: only `pine learn -g` may create ~/.pine.
	dir := filepath.Join(t.TempDir(), "absent")
	t.Setenv(memory.EnvHome, dir)
	s := scaffold(t)
	md := FormatMemoryBlock(s, nil, nil, 3)
	if strings.Contains(md, "Your Preferences") {
		t.Errorf("absent global store must render nothing:\n%s", md)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("pine context created the global store at %s", dir)
	}
}

func TestFormatMemoryBlockOmitsGlobalWhenUnreadable(t *testing.T) {
	// PINE_HOME points at a regular file → ReadMEMORY errors.
	f := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(memory.EnvHome, f)
	s := scaffold(t)
	if err := memory.AppendMEMORY(s.Root(), memory.AppendOpts{Text: "project survives"}); err != nil {
		t.Fatal(err)
	}
	md := FormatMemoryBlock(s, nil, nil, 3)
	if strings.Contains(md, "Your Preferences") {
		t.Errorf("unreadable global store must be omitted:\n%s", md)
	}
	if !strings.Contains(md, "project survives") {
		t.Errorf("project block must be intact:\n%s", md)
	}
}

func TestFormatMemoryBlockRespectsGlobalMemoryOptOut(t *testing.T) {
	seedGlobal(t, "# Personal memory\n\n## Log\n- opted out pref\n")
	s := scaffold(t)
	if err := memory.AppendMEMORY(s.Root(), memory.AppendOpts{Text: "project rule"}); err != nil {
		t.Fatal(err)
	}
	cfg := s.Config()
	cfg.Context.GlobalMemory = false
	md := FormatMemoryBlock(s, nil, nil, 3)
	if strings.Contains(md, "Your Preferences") || strings.Contains(md, "opted out pref") {
		t.Errorf("opt-out must suppress the global block:\n%s", md)
	}
	if !strings.Contains(md, "## Project Memory") {
		t.Errorf("project block must remain:\n%s", md)
	}
}

func TestFormatMemoryBlockCapsGlobalAt2048(t *testing.T) {
	dir := seedGlobal(t, "# Personal memory\n\n## Log\n"+strings.Repeat("- a long personal preference line\n", 400))
	s := scaffold(t)
	if err := memory.AppendMEMORY(s.Root(), memory.AppendOpts{Text: "PROJECT_SENTINEL"}); err != nil {
		t.Fatal(err)
	}
	md := FormatMemoryBlock(s, nil, nil, 3)
	g := strings.Index(md, "## Your Preferences (global)")
	p := strings.Index(md, "## Project Memory")
	if g < 0 || p < 0 {
		t.Fatalf("missing a section:\n%s", md)
	}
	// Body is capped at ContextGlobalCap; the rest is the fixed header, the
	// precedence line and the truncation notice (~220B). Untruncated this
	// body would be ~13 KB, so this bound still proves the cap bites.
	if n := p - g; n > memory.ContextGlobalCap+320 {
		t.Errorf("global block %d bytes, want <= %d", n, memory.ContextGlobalCap+320)
	}
	if !strings.Contains(md, "… truncated") {
		t.Errorf("want a truncation notice:\n%s", md)
	}
	if !strings.Contains(md, memory.GlobalLabel(dir)+"/MEMORY.md`") {
		t.Errorf("notice must name the global store, not .pine:\n%s", md)
	}
	// Caps are independent — the project block is not truncated by the global one.
	if !strings.Contains(md, "PROJECT_SENTINEL") {
		t.Errorf("project block must be complete:\n%s", md)
	}
}

func TestFormatMemoryBlockGlobalTopicsOnlyWhenNoMEMORY(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(memory.EnvHome, dir)
	if err := memory.AppendTopic(dir, "pnpm", memory.AppendOpts{Text: "body"}); err != nil {
		t.Fatal(err)
	}
	// Blank out MEMORY.md, keep the topic.
	if err := os.WriteFile(memory.MemoryPath(dir), []byte("   \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := scaffold(t)
	md := FormatMemoryBlock(s, nil, nil, 3)
	if !strings.Contains(md, "## Your Preferences (global)") || !strings.Contains(md, "Global topics: pnpm") {
		t.Errorf("topics alone should still render the header + list:\n%s", md)
	}
}

func TestContextGlobalConventionLineTracksOptOut(t *testing.T) {
	// The convention line says "shown above under Your Preferences" — it must
	// not appear when the opt-out removed that very section.
	seedGlobal(t, "# Personal memory\n\n## Log\n- a pref\n")
	s := scaffold(t)

	on := Context(s, fakeGit(), time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC))
	if !strings.Contains(on, "## Your Preferences (global)") {
		t.Fatalf("expected the global block:\n%s", on)
	}
	if !strings.Contains(on, "pine learn -g") {
		t.Errorf("expected the convention line when enabled:\n%s", on)
	}

	s.Config().Context.GlobalMemory = false
	off := Context(s, fakeGit(), time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC))
	if strings.Contains(off, "Your Preferences") {
		t.Errorf("opt-out must remove every mention of the section:\n%s", off)
	}
	if strings.Contains(off, "pine learn -g") {
		t.Errorf("convention line must not advertise -g when opted out:\n%s", off)
	}
}

func TestContextIncludesGlobalPreferences(t *testing.T) {
	seedGlobal(t, "# Personal memory\n\n## Log\n- I use pnpm, never npm\n")
	s := scaffold(t)
	md := Context(s, fakeGit(), time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC))
	for _, want := range []string{
		"## Your Preferences (global)",
		"Project Memory wins",
		"I use pnpm, never npm",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in:\n%s", want, md)
		}
	}
	// Ordering against the neighbouring blocks.
	done := strings.Index(md, "## Recently Done")
	glob := strings.Index(md, "## Your Preferences (global)")
	conv := strings.Index(md, "## Conventions")
	if done >= 0 && !(done < glob) {
		t.Errorf("global block should follow Recently Done")
	}
	if !(glob < conv) {
		t.Errorf("global block should precede Conventions")
	}
}
