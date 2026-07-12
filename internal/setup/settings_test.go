package setup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallClaudeSkillAndHook(t *testing.T) {
	dir := t.TempDir()
	r, _ := newTestRunner(dir)
	if err := r.Install([]Recipe{RecipeClaude}); err != nil {
		t.Fatal(err)
	}
	// Skill file written.
	skill := filepath.Join(dir, ".claude", "skills", "pine", "SKILL.md")
	data, err := os.ReadFile(skill)
	if err != nil {
		t.Fatalf("skill file missing: %v", err)
	}
	if !strings.Contains(string(data), "name: pine") {
		t.Errorf("skill frontmatter missing:\n%s", data)
	}
	// Hook installed in settings.json.
	if got := countPineHooks(t, dir); got != 1 {
		t.Fatalf("expected 1 pine hook, got %d", got)
	}
}

func TestClaudeHookIdempotent(t *testing.T) {
	dir := t.TempDir()
	// Pre-seed settings.json with an unrelated user hook to ensure it survives.
	settings := filepath.Join(dir, ".claude", "settings.json")
	os.MkdirAll(filepath.Dir(settings), 0o755)
	os.WriteFile(settings, []byte(`{"model":"opus","hooks":{"Stop":[{"hooks":[{"type":"command","command":"my-own-hook"}]}]}}`), 0o644)

	r, _ := newTestRunner(dir)
	for i := 0; i < 3; i++ {
		if err := r.Install([]Recipe{RecipeClaude}); err != nil {
			t.Fatal(err)
		}
	}
	// Exactly one pine hook after three installs (no duplication).
	if got := countPineHooks(t, dir); got != 1 {
		t.Errorf("expected 1 pine hook after 3 installs, got %d", got)
	}
	// User's own hook and settings survive.
	data, _ := os.ReadFile(settings)
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("settings not valid JSON: %v\n%s", err, data)
	}
	if doc["model"] != "opus" {
		t.Errorf("unrelated setting clobbered: %v", doc["model"])
	}
	if !strings.Contains(string(data), "my-own-hook") {
		t.Errorf("user's own hook was removed:\n%s", data)
	}

	// Remove strips only the pine hook.
	if err := r.Remove([]Recipe{RecipeClaude}); err != nil {
		t.Fatal(err)
	}
	if got := countPineHooks(t, dir); got != 0 {
		t.Errorf("pine hook should be gone after remove, got %d", got)
	}
	data, _ = os.ReadFile(settings)
	if !strings.Contains(string(data), "my-own-hook") {
		t.Errorf("user's own hook must survive removal:\n%s", data)
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude", "skills", "pine", "SKILL.md")); !os.IsNotExist(err) {
		t.Errorf("skill file should be removed, stat err = %v", err)
	}
}

func TestSkillFileRefreshesOnChange(t *testing.T) {
	dir := t.TempDir()
	r, _ := newTestRunner(dir)
	if err := r.Install([]Recipe{RecipeClaude}); err != nil {
		t.Fatal(err)
	}
	skill := filepath.Join(dir, ".claude", "skills", "pine", "SKILL.md")
	// Simulate a stale, hand-edited skill file.
	os.WriteFile(skill, []byte("stale content"), 0o644)
	if got := CheckSkillFile(dir, mustLookup(t, RecipeClaude), r.Opts); got != StatusStale {
		t.Errorf("expected stale, got %s", got)
	}
	if err := r.Install([]Recipe{RecipeClaude}); err != nil {
		t.Fatal(err)
	}
	if got := CheckSkillFile(dir, mustLookup(t, RecipeClaude), r.Opts); got != StatusCurrent {
		t.Errorf("expected current after reinstall, got %s", got)
	}
}

func countPineHooks(t *testing.T, dir string) int {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	if err != nil {
		return 0
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("settings not JSON: %v", err)
	}
	hooks, _ := doc["hooks"].(map[string]any)
	stop, _ := hooks["Stop"].([]any)
	n := 0
	for _, g := range stop {
		if groupHasPineHook(g) {
			n++
		}
	}
	return n
}

func mustLookup(t *testing.T, r Recipe) RecipeInfo {
	t.Helper()
	info, ok := Lookup(r)
	if !ok {
		t.Fatalf("recipe %s not found", r)
	}
	return info
}

func TestCheckClaudeHookStatuses(t *testing.T) {
	dir := t.TempDir()
	if got := CheckClaudeHook(dir); got != StatusMissing {
		t.Fatalf("missing: %s", got)
	}
	r, _ := newTestRunner(dir)
	if err := r.Install([]Recipe{RecipeClaude}); err != nil {
		t.Fatal(err)
	}
	if got := CheckClaudeHook(dir); got != StatusCurrent {
		t.Fatalf("current: %s", got)
	}
	settings := filepath.Join(dir, ".claude", "settings.json")
	data, _ := os.ReadFile(settings)
	// JSON stores escaped quotes; mutate a stable substring of the command.
	stale := strings.Replace(string(data), "[pine:learn-reminder]", "[pine:learn-reminder] outdated", 1)
	if stale == string(data) {
		t.Fatal("failed to mutate hook command")
	}
	os.WriteFile(settings, []byte(stale), 0o644)
	if got := CheckClaudeHook(dir); got != StatusStale {
		t.Fatalf("stale: %s", got)
	}
}

func TestInstallSkillFileOverwriteAndEmpty(t *testing.T) {
	dir := t.TempDir()
	info := mustLookup(t, RecipeClaude)
	opts := RenderOptions{}

	status, err := InstallSkillFile(dir, RecipeInfo{}, opts)
	if err != nil || status != "" {
		t.Fatalf("empty SkillFile: status=%q err=%v", status, err)
	}
	if got := CheckSkillFile(dir, RecipeInfo{}, opts); got != StatusCurrent {
		t.Fatalf("empty skill check: %s", got)
	}
	removed, err := RemoveSkillFile(dir, RecipeInfo{})
	if err != nil || removed {
		t.Fatalf("empty remove: %v %v", removed, err)
	}
	removed, err = RemoveSkillFile(dir, info)
	if err != nil || removed {
		t.Fatalf("remove missing: %v %v", removed, err)
	}

	status, err = InstallSkillFile(dir, info, opts)
	if err != nil || status != "installed" {
		t.Fatalf("first install: %q %v", status, err)
	}
	status, err = InstallSkillFile(dir, info, opts)
	if err != nil || status != "current" {
		t.Fatalf("idempotent: %q %v", status, err)
	}

	path := filepath.Join(dir, filepath.FromSlash(info.SkillFile))
	os.WriteFile(path, []byte("stale"), 0o644)
	status, err = InstallSkillFile(dir, info, opts)
	if err != nil || status != "installed" {
		t.Fatalf("overwrite stale: %q %v", status, err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != RenderSkill(opts) {
		t.Fatal("overwrite did not restore template")
	}
}

func TestRemoveClaudeHookMissingAndUnreadable(t *testing.T) {
	dir := t.TempDir()
	removed, err := RemoveClaudeHook(dir)
	if err != nil || removed {
		t.Fatalf("missing settings: %v %v", removed, err)
	}
	settings := filepath.Join(dir, ".claude", "settings.json")
	os.MkdirAll(filepath.Dir(settings), 0o755)
	os.WriteFile(settings, []byte("not-json"), 0o644)
	removed, err = RemoveClaudeHook(dir)
	if err != nil || removed {
		t.Fatalf("unreadable: %v %v", removed, err)
	}
	os.WriteFile(settings, []byte(`{"model":"opus"}`), 0o644)
	removed, err = RemoveClaudeHook(dir)
	if err != nil || removed {
		t.Fatalf("no hooks key: %v %v", removed, err)
	}
}

func TestReadJSONObjectEdges(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.json")
	os.WriteFile(path, []byte("   \n"), 0o644)
	doc, err := readJSONObject(path)
	if err != nil || len(doc) != 0 {
		t.Fatalf("empty file: %#v %v", doc, err)
	}
	os.WriteFile(path, []byte("null"), 0o644)
	doc, err = readJSONObject(path)
	if err != nil || doc == nil {
		t.Fatalf("null json: %#v %v", doc, err)
	}
	_, err = readJSONObject(filepath.Join(dir, "missing.json"))
	if err != nil {
		t.Fatalf("missing: %v", err)
	}
	os.WriteFile(path, []byte("{"), 0o644)
	if _, err := readJSONObject(path); err == nil {
		t.Fatal("expected invalid JSON error")
	}
}

func TestPineHookCommandSkipsBadEntries(t *testing.T) {
	if pineHookCommand("not-a-map") != "" {
		t.Fatal("non-map")
	}
	g := map[string]any{"hooks": []any{"x", map[string]any{"command": "no-sentinel"}}}
	if pineHookCommand(g) != "" {
		t.Fatal("no sentinel")
	}
}
