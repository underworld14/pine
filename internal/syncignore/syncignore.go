// Package syncignore manages the Pine-owned block in .pine/.gitignore that
// controls whether tickets/ and attachments/ are committed or kept local.
package syncignore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	beginMarker = "# pine:sync begin"
	endMarker   = "# pine:sync end"
)

// Prefs are the git-tracking preferences for .pine/tickets and .pine/attachments.
// MEMORY.md / memory/ are always tracked and are not part of Prefs.
type Prefs struct {
	Tickets     bool // true = track in git (default)
	Attachments bool // true = track in git (default false = local)
}

// Default returns the init defaults: tickets tracked, attachments local.
func Default() Prefs {
	return Prefs{Tickets: true, Attachments: false}
}

// WritePineGitignore writes or rewrites the managed sync block in
// pineDir/.gitignore. Lines outside the markers are preserved.
func WritePineGitignore(pineDir string, prefs Prefs) error {
	path := filepath.Join(pineDir, ".gitignore")
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	out := rewriteManaged(string(existing), prefs)
	if err := os.MkdirAll(pineDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(out), 0o644)
}

// ParseManagedBlock extracts Prefs from a .gitignore body. If no managed block
// is present, it returns Default().
func ParseManagedBlock(content string) Prefs {
	prefs, ok := parseMeta(content)
	if !ok {
		return Default()
	}
	return prefs
}

func rewriteManaged(existing string, prefs Prefs) string {
	block := renderBlock(prefs)
	before, after, found := splitManaged(existing)
	if !found {
		trimmed := strings.TrimRight(existing, "\n")
		if trimmed == "" {
			return block
		}
		return trimmed + "\n\n" + block
	}
	var b strings.Builder
	if before != "" {
		b.WriteString(strings.TrimRight(before, "\n"))
		b.WriteString("\n\n")
	}
	b.WriteString(block)
	if after != "" {
		after = strings.TrimLeft(after, "\n")
		if after != "" {
			b.WriteString("\n")
			b.WriteString(after)
			if !strings.HasSuffix(after, "\n") {
				b.WriteByte('\n')
			}
		}
	}
	return b.String()
}

func renderBlock(prefs Prefs) string {
	var b strings.Builder
	b.WriteString(beginMarker)
	b.WriteByte('\n')
	b.WriteString(fmt.Sprintf("# tickets=%s attachments=%s\n", onOff(prefs.Tickets), onOff(prefs.Attachments)))
	if !prefs.Tickets {
		b.WriteString("tickets/\n")
	}
	if !prefs.Attachments {
		b.WriteString("attachments/\n")
	}
	b.WriteString(endMarker)
	b.WriteByte('\n')
	return b.String()
}

func onOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

func splitManaged(content string) (before, after string, found bool) {
	begin := strings.Index(content, beginMarker)
	if begin < 0 {
		return content, "", false
	}
	end := strings.Index(content[begin:], endMarker)
	if end < 0 {
		// Malformed: treat from begin to EOF as the block.
		return content[:begin], "", true
	}
	endAbs := begin + end + len(endMarker)
	// Consume trailing newline after end marker.
	if endAbs < len(content) && content[endAbs] == '\n' {
		endAbs++
	}
	return content[:begin], content[endAbs:], true
}

func parseMeta(content string) (Prefs, bool) {
	begin := strings.Index(content, beginMarker)
	if begin < 0 {
		return Prefs{}, false
	}
	end := strings.Index(content[begin:], endMarker)
	if end < 0 {
		return Prefs{}, false
	}
	block := content[begin : begin+end]
	prefs := Default()
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "# tickets=") {
			continue
		}
		// # tickets=on attachments=off
		rest := strings.TrimPrefix(line, "# ")
		parts := strings.Fields(rest)
		for _, p := range parts {
			k, v, ok := strings.Cut(p, "=")
			if !ok {
				continue
			}
			switch k {
			case "tickets":
				prefs.Tickets = v == "on"
			case "attachments":
				prefs.Attachments = v == "on"
			}
		}
		return prefs, true
	}
	// Fall back to path lines inside the block.
	prefs = Prefs{Tickets: true, Attachments: true}
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSpace(line)
		switch line {
		case "tickets/", "tickets":
			prefs.Tickets = false
		case "attachments/", "attachments":
			prefs.Attachments = false
		}
	}
	return prefs, true
}
