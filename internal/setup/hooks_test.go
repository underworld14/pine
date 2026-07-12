package setup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallCodexHookWithAgents(t *testing.T) {
	dir := t.TempDir()
	r, _ := newTestRunner(dir)
	if err := r.Install([]Recipe{RecipeAgents}); err != nil {
		t.Fatal(err)
	}
	if got := CheckCodexHook(dir); got != StatusCurrent {
		t.Fatalf("expected Codex hook current, got %s", got)
	}
	script := filepath.Join(dir, ".codex", "hooks", "pine-learn-reminder.sh")
	fi, err := os.Stat(script)
	if err != nil {
		t.Fatalf("script missing: %v", err)
	}
	if fi.Mode()&0o111 == 0 {
		t.Fatalf("script not executable: %v", fi.Mode())
	}
	data, _ := os.ReadFile(script)
	if !strings.Contains(string(data), "systemMessage") || !strings.Contains(string(data), hookSentinel) {
		t.Fatalf("script missing reminder payload:\n%s", data)
	}
	// Cursor files must not appear from agents-only install.
	if _, err := os.Stat(filepath.Join(dir, ".cursor", "hooks.json")); !os.IsNotExist(err) {
		t.Fatalf("agents install should not create Cursor hooks, err=%v", err)
	}
}

func TestCodexHookIdempotent(t *testing.T) {
	dir := t.TempDir()
	hooksPath := filepath.Join(dir, ".codex", "hooks.json")
	os.MkdirAll(filepath.Dir(hooksPath), 0o755)
	os.WriteFile(hooksPath, []byte(`{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"my-own-hook"}]}]}}`), 0o644)

	r, _ := newTestRunner(dir)
	for i := 0; i < 3; i++ {
		if err := r.Install([]Recipe{RecipeAgents}); err != nil {
			t.Fatal(err)
		}
	}
	if got := countCodexPineHooks(t, dir); got != 1 {
		t.Errorf("expected 1 pine Codex hook after 3 installs, got %d", got)
	}
	data, _ := os.ReadFile(hooksPath)
	if !strings.Contains(string(data), "my-own-hook") {
		t.Errorf("user's own hook was removed:\n%s", data)
	}

	if err := r.Remove([]Recipe{RecipeAgents}); err != nil {
		t.Fatal(err)
	}
	if got := countCodexPineHooks(t, dir); got != 0 {
		t.Errorf("pine Codex hook should be gone after remove, got %d", got)
	}
	if got := CheckCodexHook(dir); got != StatusMissing {
		t.Errorf("CheckCodexHook after remove = %s, want missing", got)
	}
	data, _ = os.ReadFile(hooksPath)
	if !strings.Contains(string(data), "my-own-hook") {
		t.Errorf("user's own hook must survive removal:\n%s", data)
	}
	if _, err := os.Stat(filepath.Join(dir, ".codex", "hooks", "pine-learn-reminder.sh")); !os.IsNotExist(err) {
		t.Errorf("Codex script should be removed, stat err = %v", err)
	}
}

func TestInstallCursorHookOnly(t *testing.T) {
	dir := t.TempDir()
	r, buf := newTestRunner(dir)
	if err := r.Install([]Recipe{RecipeCursor}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("cursor recipe must not create AGENTS.md, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agents", "skills", "pine", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("cursor recipe must not create skill file, err=%v", err)
	}
	if got := CheckCursorHook(dir); got != StatusCurrent {
		t.Fatalf("expected Cursor hook current, got %s", got)
	}
	if !strings.Contains(buf.String(), ".cursor/hooks.json") {
		t.Fatalf("expected cursor hook install output:\n%s", buf.String())
	}
	script := filepath.Join(dir, ".cursor", "hooks", "pine-learn-reminder.sh")
	fi, err := os.Stat(script)
	if err != nil {
		t.Fatalf("script missing: %v", err)
	}
	if fi.Mode()&0o111 == 0 {
		t.Fatalf("script not executable: %v", fi.Mode())
	}
	data, err := os.ReadFile(script)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "additional_context") || !strings.Contains(string(data), hookSentinel) {
		t.Fatalf("script missing reminder payload:\n%s", data)
	}
}

func TestCursorHookIdempotent(t *testing.T) {
	dir := t.TempDir()
	hooksPath := filepath.Join(dir, ".cursor", "hooks.json")
	os.MkdirAll(filepath.Dir(hooksPath), 0o755)
	os.WriteFile(hooksPath, []byte(`{"version":1,"hooks":{"afterFileEdit":[{"command":"./format.sh"}],"sessionStart":[{"command":"my-own-start.sh"}]}}`), 0o644)

	r, _ := newTestRunner(dir)
	for i := 0; i < 3; i++ {
		if err := r.Install([]Recipe{RecipeCursor}); err != nil {
			t.Fatal(err)
		}
	}
	if got := countCursorPineHooks(t, dir); got != 1 {
		t.Errorf("expected 1 pine Cursor hook after 3 installs, got %d", got)
	}
	data, _ := os.ReadFile(hooksPath)
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("hooks not valid JSON: %v\n%s", err, data)
	}
	if doc["version"] != float64(1) {
		t.Errorf("version clobbered: %v", doc["version"])
	}
	if !strings.Contains(string(data), "my-own-start.sh") || !strings.Contains(string(data), "format.sh") {
		t.Errorf("user's own hooks were removed:\n%s", data)
	}

	if err := r.Remove([]Recipe{RecipeCursor}); err != nil {
		t.Fatal(err)
	}
	if got := countCursorPineHooks(t, dir); got != 0 {
		t.Errorf("pine Cursor hook should be gone after remove, got %d", got)
	}
	if got := CheckCursorHook(dir); got != StatusMissing {
		t.Errorf("CheckCursorHook after remove = %s, want missing", got)
	}
	data, _ = os.ReadFile(hooksPath)
	if !strings.Contains(string(data), "my-own-start.sh") || !strings.Contains(string(data), "format.sh") {
		t.Errorf("user's own hooks must survive removal:\n%s", data)
	}
	if _, err := os.Stat(filepath.Join(dir, ".cursor", "hooks", "pine-learn-reminder.sh")); !os.IsNotExist(err) {
		t.Errorf("Cursor script should be removed, stat err = %v", err)
	}
}

func TestCursorHookStaleScript(t *testing.T) {
	dir := t.TempDir()
	r, _ := newTestRunner(dir)
	if err := r.Install([]Recipe{RecipeCursor}); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, ".cursor", "hooks", "pine-learn-reminder.sh")
	os.WriteFile(script, []byte("#!/bin/bash\necho stale\n"), 0o755)
	if got := CheckCursorHook(dir); got != StatusStale {
		t.Fatalf("expected stale, got %s", got)
	}
	if err := r.Install([]Recipe{RecipeCursor}); err != nil {
		t.Fatal(err)
	}
	if got := CheckCursorHook(dir); got != StatusCurrent {
		t.Fatalf("expected current after reinstall, got %s", got)
	}
}

func TestCodexHookStaleScript(t *testing.T) {
	dir := t.TempDir()
	r, _ := newTestRunner(dir)
	if err := r.Install([]Recipe{RecipeAgents}); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, ".codex", "hooks", "pine-learn-reminder.sh")
	os.WriteFile(script, []byte("#!/bin/bash\necho stale\n"), 0o755)
	if got := CheckCodexHook(dir); got != StatusStale {
		t.Fatalf("expected stale, got %s", got)
	}
	if err := r.Install([]Recipe{RecipeAgents}); err != nil {
		t.Fatal(err)
	}
	if got := CheckCodexHook(dir); got != StatusCurrent {
		t.Fatalf("expected current after reinstall, got %s", got)
	}
}

func countCodexPineHooks(t *testing.T, dir string) int {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".codex", "hooks.json"))
	if err != nil {
		return 0
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("hooks not JSON: %v", err)
	}
	hooks, _ := doc["hooks"].(map[string]any)
	stop, _ := hooks["Stop"].([]any)
	n := 0
	for _, g := range stop {
		if groupHasCodexPineHook(g) || groupHasPineHook(g) {
			n++
		}
	}
	return n
}

func countCursorPineHooks(t *testing.T, dir string) int {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".cursor", "hooks.json"))
	if err != nil {
		return 0
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("hooks not JSON: %v", err)
	}
	hooks, _ := doc["hooks"].(map[string]any)
	events, _ := hooks["sessionStart"].([]any)
	n := 0
	for _, e := range events {
		if cursorEntryIsPine(e) {
			n++
		}
	}
	return n
}

func TestCheckHookDefaultAndDispatch(t *testing.T) {
	dir := t.TempDir()
	st, label := checkHook(dir, HookKindNone)
	if st != StatusCurrent || label != "" {
		t.Fatalf("HookKindNone: status=%s label=%q", st, label)
	}
	st, label = checkHook(dir, HookKind("unknown"))
	if st != StatusCurrent || label != "" {
		t.Fatalf("unknown kind: status=%s label=%q", st, label)
	}

	// Missing installs report missing via checkHook dispatch.
	st, label = checkHook(dir, HookKindClaude)
	if st != StatusMissing || !strings.Contains(label, ".claude/settings.json") {
		t.Fatalf("claude missing: %s %q", st, label)
	}
	st, label = checkHook(dir, HookKindCodex)
	if st != StatusMissing || !strings.Contains(label, ".codex/hooks.json") {
		t.Fatalf("codex missing: %s %q", st, label)
	}
	st, label = checkHook(dir, HookKindCursor)
	if st != StatusMissing || !strings.Contains(label, ".cursor/hooks.json") {
		t.Fatalf("cursor missing: %s %q", st, label)
	}
}

func TestRemoveHooksWhenMissing(t *testing.T) {
	dir := t.TempDir()
	removed, err := RemoveCodexHook(dir)
	if err != nil || removed {
		t.Fatalf("RemoveCodexHook missing: removed=%v err=%v", removed, err)
	}
	removed, err = RemoveCursorHook(dir)
	if err != nil || removed {
		t.Fatalf("RemoveCursorHook missing: removed=%v err=%v", removed, err)
	}
	ok, label, err := removeHook(dir, HookKindNone)
	if err != nil || ok || label != "" {
		t.Fatalf("removeHook none: %v %q %v", ok, label, err)
	}
	status, label, err := installHook(dir, HookKindNone)
	if err != nil || status != "" || label != "" {
		t.Fatalf("installHook none: %q %q %v", status, label, err)
	}
}

func TestCodexHookStaleCommand(t *testing.T) {
	dir := t.TempDir()
	r, _ := newTestRunner(dir)
	if err := r.Install([]Recipe{RecipeAgents}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ".codex", "hooks.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Mutate the command path inside JSON (escaped quotes differ from Go string form).
	stale := strings.Replace(string(data), "pine-learn-reminder.sh", "outdated-pine-learn-reminder.sh", 1)
	if stale == string(data) {
		t.Fatal("failed to mutate command")
	}
	if err := os.WriteFile(path, []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := CheckCodexHook(dir); got != StatusStale {
		t.Fatalf("expected stale command, got %s", got)
	}
}

func TestCursorHookStaleCommand(t *testing.T) {
	dir := t.TempDir()
	r, _ := newTestRunner(dir)
	if err := r.Install([]Recipe{RecipeCursor}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ".cursor", "hooks.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	stale := strings.Replace(string(data), "pine-learn-reminder.sh", "outdated-pine-learn-reminder.sh", 1)
	if stale == string(data) {
		t.Fatal("failed to mutate command")
	}
	if err := os.WriteFile(path, []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := CheckCursorHook(dir); got != StatusStale {
		t.Fatalf("expected stale command, got %s", got)
	}
}

func TestCodexGroupAndCursorEntryHelpers(t *testing.T) {
	if codexGroupCommand("x") != "" || cursorEntryCommand("x") != "" {
		t.Fatal("non-map")
	}
	g := map[string]any{"hooks": []any{"bad", map[string]any{"command": "echo hi"}}}
	if codexGroupCommand(g) != "" {
		t.Fatal("no pine cmd")
	}
	g2 := map[string]any{"hooks": []any{map[string]any{"command": "run pine-learn-reminder.sh"}}}
	if !strings.Contains(codexGroupCommand(g2), "pine-learn-reminder") {
		t.Fatal(codexGroupCommand(g2))
	}
	if !cursorEntryIsPine(map[string]any{"command": "x " + hookSentinel}) {
		t.Fatal("sentinel")
	}
}

func TestRemoveNestedStopHookNoPine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hooks.json")
	os.WriteFile(path, []byte(`{"hooks":{"Stop":[{"hooks":[{"command":"other"}]}]}}`), 0o644)
	removed, err := removeNestedStopHook(path, groupHasPineHook)
	if err != nil || removed {
		t.Fatalf("%v %v", removed, err)
	}
	os.WriteFile(path, []byte(`not-json`), 0o644)
	removed, err = removeNestedStopHook(path, groupHasPineHook)
	if err != nil || removed {
		t.Fatalf("bad json: %v %v", removed, err)
	}
	removed, err = removeNestedStopHook(filepath.Join(dir, "missing.json"), groupHasPineHook)
	if err != nil || removed {
		t.Fatalf("missing: %v %v", removed, err)
	}
}

func TestWriteHookScriptCurrent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "h.sh")
	status, err := writeHookScript(path, "#!/bin/bash\necho 1\n")
	if err != nil || status != "installed" {
		t.Fatalf("%q %v", status, err)
	}
	status, err = writeHookScript(path, "#!/bin/bash\necho 1\n")
	if err != nil || status != "current" {
		t.Fatalf("idempotent: %q %v", status, err)
	}
}

func TestCheckHooksMissingJSONWithScript(t *testing.T) {
	dir := t.TempDir()
	// Script current but JSON missing → Check reports missing.
	os.MkdirAll(filepath.Join(dir, ".cursor", "hooks"), 0o755)
	os.WriteFile(filepath.Join(dir, ".cursor", "hooks", "pine-learn-reminder.sh"), []byte(cursorHookScript()), 0o755)
	if got := CheckCursorHook(dir); got != StatusMissing {
		t.Fatalf("cursor=%s", got)
	}
	os.MkdirAll(filepath.Join(dir, ".codex", "hooks"), 0o755)
	os.WriteFile(filepath.Join(dir, ".codex", "hooks", "pine-learn-reminder.sh"), []byte(codexHookScript()), 0o755)
	if got := CheckCodexHook(dir); got != StatusMissing {
		t.Fatalf("codex=%s", got)
	}
}

func TestInstallHooksReturnCurrent(t *testing.T) {
	dir := t.TempDir()
	r, _ := newTestRunner(dir)
	if err := r.Install([]Recipe{RecipeAgents, RecipeCursor}); err != nil {
		t.Fatal(err)
	}
	status, err := InstallCodexHook(dir)
	if err != nil || status != "current" {
		t.Fatalf("codex current: %q %v", status, err)
	}
	status, err = InstallCursorHook(dir)
	if err != nil || status != "current" {
		t.Fatalf("cursor current: %q %v", status, err)
	}
}

func TestRemoveCursorHookOnlyScript(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, ".cursor", "hooks", "pine-learn-reminder.sh")
	os.MkdirAll(filepath.Dir(script), 0o755)
	os.WriteFile(script, []byte("x"), 0o755)
	removed, err := RemoveCursorHook(dir)
	if err != nil || !removed {
		t.Fatalf("%v %v", removed, err)
	}
}

func TestRemoveCodexHookOnlyScript(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, ".codex", "hooks", "pine-learn-reminder.sh")
	os.MkdirAll(filepath.Dir(script), 0o755)
	os.WriteFile(script, []byte("x"), 0o755)
	removed, err := RemoveCodexHook(dir)
	if err != nil || !removed {
		t.Fatalf("%v %v", removed, err)
	}
}
