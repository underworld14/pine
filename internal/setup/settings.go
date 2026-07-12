package setup

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// hookSentinel is embedded in Pine's managed hook command so re-runs can find,
// update, or remove exactly that hook without touching the user's other hooks.
const hookSentinel = "[pine:learn-reminder]"

// learnReminderCommand is the shell command Pine installs as a Claude Code Stop
// hook: a gentle, non-blocking nudge to capture learnings before the turn ends.
func learnReminderCommand() string {
	return "echo \"Pine reminder: capture durable cross-session insights with 'pine learn' before ending. " + hookSentinel + "\""
}

// InstallSkillFile writes the Pine SKILL.md into an agent's skills directory.
// The file is wholly Pine-owned: it is (re)written only when its content
// differs from the current template, so re-running setup after a Pine upgrade
// refreshes it while an unchanged run is a no-op.
func InstallSkillFile(root string, info RecipeInfo, opts RenderOptions) (string, error) {
	if info.SkillFile == "" {
		return "", nil
	}
	path := filepath.Join(root, filepath.FromSlash(info.SkillFile))
	content := RenderSkill(opts)
	if existing, err := os.ReadFile(path); err == nil {
		if string(existing) == content {
			return "current", nil
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return "installed", nil
}

// RemoveSkillFile deletes a previously installed SKILL.md (and its now-empty
// directory, best-effort). Returns whether a file was removed.
func RemoveSkillFile(root string, info RecipeInfo) (bool, error) {
	if info.SkillFile == "" {
		return false, nil
	}
	path := filepath.Join(root, filepath.FromSlash(info.SkillFile))
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	_ = os.Remove(filepath.Dir(path)) // clean up .../pine/ if empty
	return true, nil
}

// CheckSkillFile reports whether the installed SKILL.md matches the current
// template.
func CheckSkillFile(root string, info RecipeInfo, opts RenderOptions) InstallStatus {
	if info.SkillFile == "" {
		return StatusCurrent
	}
	path := filepath.Join(root, filepath.FromSlash(info.SkillFile))
	existing, err := os.ReadFile(path)
	if err != nil {
		return StatusMissing
	}
	if string(existing) == RenderSkill(opts) {
		return StatusCurrent
	}
	return StatusStale
}

// InstallClaudeHook idempotently inserts Pine's learn-reminder Stop hook into
// .claude/settings.json, preserving every other setting and hook. Returns
// "installed" (added or updated) or "current" (already present and identical).
func InstallClaudeHook(root string) (string, error) {
	path := filepath.Join(root, ".claude", "settings.json")
	doc, err := readJSONObject(path)
	if err != nil {
		return "", err
	}
	hooks := asObject(doc["hooks"])
	stop := asArray(hooks["Stop"])
	want := learnReminderCommand()

	var kept []any
	current := false
	for _, g := range stop {
		if groupHasPineHook(g) {
			if pineHookCommand(g) == want {
				current = true
				kept = append(kept, g) // up to date — keep as-is
			}
			continue // drop an outdated pine hook; a fresh one is appended below
		}
		kept = append(kept, g)
	}
	if current {
		return "current", nil
	}
	kept = append(kept, pineHookGroup(want))
	hooks["Stop"] = kept
	doc["hooks"] = hooks
	if err := writeJSONObject(path, doc); err != nil {
		return "", err
	}
	return "installed", nil
}

// RemoveClaudeHook strips Pine's learn-reminder hook from .claude/settings.json,
// leaving other settings intact. Returns whether a hook was removed.
func RemoveClaudeHook(root string) (bool, error) {
	path := filepath.Join(root, ".claude", "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return false, nil // never touch a settings file we can't parse
	}
	hooks, ok := doc["hooks"].(map[string]any)
	if !ok {
		return false, nil
	}
	stop := asArray(hooks["Stop"])
	var kept []any
	removed := false
	for _, g := range stop {
		if groupHasPineHook(g) {
			removed = true
			continue
		}
		kept = append(kept, g)
	}
	if !removed {
		return false, nil
	}
	if len(kept) == 0 {
		delete(hooks, "Stop")
	} else {
		hooks["Stop"] = kept
	}
	if len(hooks) == 0 {
		delete(doc, "hooks")
	}
	return true, writeJSONObject(path, doc)
}

// CheckClaudeHook reports whether Pine's hook is present and current.
func CheckClaudeHook(root string) InstallStatus {
	path := filepath.Join(root, ".claude", "settings.json")
	doc, err := readJSONObject(path)
	if err != nil {
		return StatusMissing
	}
	hooks := asObject(doc["hooks"])
	for _, g := range asArray(hooks["Stop"]) {
		if groupHasPineHook(g) {
			if pineHookCommand(g) == learnReminderCommand() {
				return StatusCurrent
			}
			return StatusStale
		}
	}
	return StatusMissing
}

// --- small JSON helpers (kept local to avoid a config dependency) ---

func readJSONObject(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]any{}, nil
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf(".claude/settings.json is not valid JSON: %w", err)
	}
	if doc == nil {
		doc = map[string]any{}
	}
	return doc, nil
}

func writeJSONObject(path string, doc map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func asObject(v any) map[string]any {
	if m, ok := v.(map[string]any); ok && m != nil {
		return m
	}
	return map[string]any{}
}

func asArray(v any) []any {
	if a, ok := v.([]any); ok {
		return a
	}
	return nil
}

func pineHookGroup(command string) map[string]any {
	return map[string]any{
		"hooks": []any{
			map[string]any{"type": "command", "command": command},
		},
	}
}

// pineHookCommand returns the sentinel-bearing command inside a Stop group, or "".
func pineHookCommand(g any) string {
	m, ok := g.(map[string]any)
	if !ok {
		return ""
	}
	for _, h := range asArray(m["hooks"]) {
		hm, ok := h.(map[string]any)
		if !ok {
			continue
		}
		if cmd, ok := hm["command"].(string); ok && strings.Contains(cmd, hookSentinel) {
			return cmd
		}
	}
	return ""
}

func groupHasPineHook(g any) bool {
	return pineHookCommand(g) != ""
}
