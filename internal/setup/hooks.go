package setup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

const (
	codexHookRelJSON    = ".codex/hooks.json"
	codexHookRelScript  = ".codex/hooks/pine-learn-reminder.sh"
	cursorHookRelJSON   = ".cursor/hooks.json"
	cursorHookRelScript = ".cursor/hooks/pine-learn-reminder.sh"
)

func learnReminderText() string {
	return "Pine reminder: only if you learned something durable, run 'pine learn' (MEMORY.md / memory topics — not a new LRN per ticket). " + hookSentinel
}

func codexHookCommand() string {
	return `bash "$(git rev-parse --show-toplevel)/.codex/hooks/pine-learn-reminder.sh"`
}

func cursorHookCommand() string {
	return ".cursor/hooks/pine-learn-reminder.sh"
}

func codexHookScript() string {
	msg, _ := json.Marshal(map[string]string{"systemMessage": learnReminderText()})
	return hookScriptBody(string(msg))
}

func cursorHookScript() string {
	msg, _ := json.Marshal(map[string]string{"additional_context": learnReminderText()})
	return hookScriptBody(string(msg))
}

func hookScriptBody(jsonLine string) string {
	return "#!/bin/bash\n# " + hookSentinel + "\ncat >/dev/null\ncat <<'PINE_HOOK_EOF'\n" + jsonLine + "\nPINE_HOOK_EOF\n"
}

// InstallCodexHook idempotently installs Pine's learn-reminder Stop hook into
// .codex/hooks.json and writes the companion script.
func InstallCodexHook(root string) (string, error) {
	scriptStatus, err := writeHookScript(filepath.Join(root, filepath.FromSlash(codexHookRelScript)), codexHookScript())
	if err != nil {
		return "", err
	}
	path := filepath.Join(root, filepath.FromSlash(codexHookRelJSON))
	doc, err := readJSONObject(path)
	if err != nil {
		return "", err
	}
	hooks := asObject(doc["hooks"])
	stop := asArray(hooks["Stop"])
	want := codexHookCommand()

	var kept []any
	current := false
	for _, g := range stop {
		if groupHasPineHook(g) || groupHasCodexPineHook(g) {
			if pineHookCommand(g) == want || codexGroupCommand(g) == want {
				current = true
				kept = append(kept, g)
			}
			continue
		}
		kept = append(kept, g)
	}
	if current && scriptStatus == "current" {
		return "current", nil
	}
	if !current {
		kept = append(kept, pineHookGroup(want))
	}
	hooks["Stop"] = kept
	doc["hooks"] = hooks
	if err := writeJSONObject(path, doc); err != nil {
		return "", err
	}
	return "installed", nil
}

// RemoveCodexHook strips Pine's Codex learn-reminder hook and script.
func RemoveCodexHook(root string) (bool, error) {
	path := filepath.Join(root, filepath.FromSlash(codexHookRelJSON))
	removedJSON, err := removeNestedStopHook(path, func(g any) bool {
		return groupHasPineHook(g) || groupHasCodexPineHook(g)
	})
	if err != nil {
		return false, err
	}
	removedScript, err := removeHookScript(filepath.Join(root, filepath.FromSlash(codexHookRelScript)))
	if err != nil {
		return false, err
	}
	return removedJSON || removedScript, nil
}

// CheckCodexHook reports whether Pine's Codex hook is present and current.
func CheckCodexHook(root string) InstallStatus {
	script := filepath.Join(root, filepath.FromSlash(codexHookRelScript))
	if !hookScriptCurrent(script, codexHookScript()) {
		if _, err := os.Stat(script); err != nil {
			return StatusMissing
		}
		return StatusStale
	}
	path := filepath.Join(root, filepath.FromSlash(codexHookRelJSON))
	doc, err := readJSONObject(path)
	if err != nil {
		return StatusMissing
	}
	hooks := asObject(doc["hooks"])
	want := codexHookCommand()
	found := false
	for _, g := range asArray(hooks["Stop"]) {
		if groupHasCodexPineHook(g) || groupHasPineHook(g) {
			found = true
			cmd := codexGroupCommand(g)
			if cmd == "" {
				cmd = pineHookCommand(g)
			}
			if cmd != want {
				return StatusStale
			}
		}
	}
	if !found {
		return StatusMissing
	}
	return StatusCurrent
}

// InstallCursorHook idempotently installs Pine's sessionStart learn-reminder
// into .cursor/hooks.json and writes the companion script.
func InstallCursorHook(root string) (string, error) {
	scriptStatus, err := writeHookScript(filepath.Join(root, filepath.FromSlash(cursorHookRelScript)), cursorHookScript())
	if err != nil {
		return "", err
	}
	path := filepath.Join(root, filepath.FromSlash(cursorHookRelJSON))
	doc, err := readJSONObject(path)
	if err != nil {
		return "", err
	}
	if _, ok := doc["version"]; !ok {
		doc["version"] = float64(1)
	}
	hooks := asObject(doc["hooks"])
	events := asArray(hooks["sessionStart"])
	want := cursorHookCommand()

	var kept []any
	current := false
	for _, e := range events {
		if cursorEntryIsPine(e) {
			if cursorEntryCommand(e) == want {
				current = true
				kept = append(kept, e)
			}
			continue
		}
		kept = append(kept, e)
	}
	if current && scriptStatus == "current" {
		return "current", nil
	}
	if !current {
		kept = append(kept, map[string]any{"command": want})
	}
	hooks["sessionStart"] = kept
	doc["hooks"] = hooks
	if err := writeJSONObject(path, doc); err != nil {
		return "", err
	}
	return "installed", nil
}

// RemoveCursorHook strips Pine's Cursor learn-reminder hook and script.
func RemoveCursorHook(root string) (bool, error) {
	path := filepath.Join(root, filepath.FromSlash(cursorHookRelJSON))
	data, err := os.ReadFile(path)
	removedJSON := false
	if err != nil {
		if !os.IsNotExist(err) {
			return false, err
		}
	} else {
		var doc map[string]any
		if err := json.Unmarshal(data, &doc); err != nil {
			// leave unreadable config alone
		} else {
			hooks, ok := doc["hooks"].(map[string]any)
			if ok {
				events := asArray(hooks["sessionStart"])
				var kept []any
				for _, e := range events {
					if cursorEntryIsPine(e) {
						removedJSON = true
						continue
					}
					kept = append(kept, e)
				}
				if removedJSON {
					if len(kept) == 0 {
						delete(hooks, "sessionStart")
					} else {
						hooks["sessionStart"] = kept
					}
					if len(hooks) == 0 {
						delete(doc, "hooks")
					}
					if err := writeJSONObject(path, doc); err != nil {
						return false, err
					}
				}
			}
		}
	}
	removedScript, err := removeHookScript(filepath.Join(root, filepath.FromSlash(cursorHookRelScript)))
	if err != nil {
		return false, err
	}
	return removedJSON || removedScript, nil
}

// CheckCursorHook reports whether Pine's Cursor hook is present and current.
func CheckCursorHook(root string) InstallStatus {
	script := filepath.Join(root, filepath.FromSlash(cursorHookRelScript))
	if !hookScriptCurrent(script, cursorHookScript()) {
		if _, err := os.Stat(script); err != nil {
			return StatusMissing
		}
		return StatusStale
	}
	path := filepath.Join(root, filepath.FromSlash(cursorHookRelJSON))
	doc, err := readJSONObject(path)
	if err != nil {
		return StatusMissing
	}
	hooks := asObject(doc["hooks"])
	want := cursorHookCommand()
	found := false
	for _, e := range asArray(hooks["sessionStart"]) {
		if cursorEntryIsPine(e) {
			found = true
			if cursorEntryCommand(e) != want {
				return StatusStale
			}
		}
	}
	if !found {
		return StatusMissing
	}
	return StatusCurrent
}

func writeHookScript(path, content string) (string, error) {
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
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		return "", err
	}
	return "installed", nil
}

func removeHookScript(path string) (bool, error) {
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	_ = os.Remove(filepath.Dir(path))               // best-effort empty hooks/
	_ = os.Remove(filepath.Dir(filepath.Dir(path))) // best-effort .codex/ or .cursor/
	return true, nil
}

func hookScriptCurrent(path, want string) bool {
	existing, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return string(existing) == want
}

func removeNestedStopHook(path string, isPine func(any) bool) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return false, nil
	}
	hooks, ok := doc["hooks"].(map[string]any)
	if !ok {
		return false, nil
	}
	stop := asArray(hooks["Stop"])
	var kept []any
	removed := false
	for _, g := range stop {
		if isPine(g) {
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

func groupHasCodexPineHook(g any) bool {
	return strings.Contains(codexGroupCommand(g), "pine-learn-reminder.sh")
}

func codexGroupCommand(g any) string {
	m, ok := g.(map[string]any)
	if !ok {
		return ""
	}
	for _, h := range asArray(m["hooks"]) {
		hm, ok := h.(map[string]any)
		if !ok {
			continue
		}
		cmd, _ := hm["command"].(string)
		if strings.Contains(cmd, "pine-learn-reminder.sh") || strings.Contains(cmd, hookSentinel) {
			return cmd
		}
	}
	return ""
}

func cursorEntryCommand(e any) string {
	m, ok := e.(map[string]any)
	if !ok {
		return ""
	}
	cmd, _ := m["command"].(string)
	return cmd
}

func cursorEntryIsPine(e any) bool {
	cmd := cursorEntryCommand(e)
	return strings.Contains(cmd, "pine-learn-reminder.sh") || strings.Contains(cmd, hookSentinel)
}

// installHook dispatches to the recipe's hook installer.
func installHook(root string, kind HookKind) (status string, pathLabel string, err error) {
	switch kind {
	case HookKindClaude:
		status, err = InstallClaudeHook(root)
		return status, ".claude/settings.json (learn-reminder hook)", err
	case HookKindCodex:
		status, err = InstallCodexHook(root)
		return status, ".codex/hooks.json (learn-reminder hook)", err
	case HookKindCursor:
		status, err = InstallCursorHook(root)
		return status, ".cursor/hooks.json (learn-reminder hook)", err
	default:
		return "", "", nil
	}
}

func removeHook(root string, kind HookKind) (removed bool, pathLabel string, err error) {
	switch kind {
	case HookKindClaude:
		removed, err = RemoveClaudeHook(root)
		return removed, ".claude/settings.json", err
	case HookKindCodex:
		removed, err = RemoveCodexHook(root)
		return removed, ".codex/hooks.json", err
	case HookKindCursor:
		removed, err = RemoveCursorHook(root)
		return removed, ".cursor/hooks.json", err
	default:
		return false, "", nil
	}
}

func checkHook(root string, kind HookKind) (status InstallStatus, pathLabel string) {
	switch kind {
	case HookKindClaude:
		return CheckClaudeHook(root), ".claude/settings.json (learn-reminder hook)"
	case HookKindCodex:
		return CheckCodexHook(root), ".codex/hooks.json (learn-reminder hook)"
	case HookKindCursor:
		return CheckCursorHook(root), ".cursor/hooks.json (learn-reminder hook)"
	default:
		return StatusCurrent, ""
	}
}
