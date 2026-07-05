package setup

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strings"
)

const profile = "full"

var (
	beginRe = regexp.MustCompile(`(?s)<!--\s*pine:begin\s+([^>]*?)\s*-->`)
	endRe   = regexp.MustCompile(`(?s)<!--\s*pine:end\s*-->`)
)

// InstallStatus reports whether a recipe section is present and current.
type InstallStatus string

const (
	StatusMissing InstallStatus = "missing"
	StatusStale   InstallStatus = "stale"
	StatusCurrent InstallStatus = "current"
)

// BeginMarker builds the opening HTML comment for a pine section.
func BeginMarker(recipe Recipe, version, contentHash string) string {
	return fmt.Sprintf("<!-- pine:begin recipe=%s profile=%s version=%s hash=%s -->",
		recipe, profile, version, contentHash)
}

// ContentHash returns a short hex digest of the section body for staleness checks.
func ContentHash(body string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(body)))
	return hex.EncodeToString(sum[:8])
}

// ExtractSection returns the pine-managed body, marker metadata, and whether it was found.
func ExtractSection(content string) (body string, meta string, found bool) {
	loc := beginRe.FindStringSubmatchIndex(content)
	if loc == nil {
		return "", "", false
	}
	meta = strings.TrimSpace(content[loc[2]:loc[3]])
	afterBegin := content[loc[1]:]
	end := endRe.FindStringIndex(afterBegin)
	if end == nil {
		return "", meta, false
	}
	body = strings.TrimSpace(afterBegin[:end[0]])
	return body, meta, true
}

// MergeFile writes or updates the pine section in path.
func MergeFile(path, markedSection string) error {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	content := string(data)
	var out string
	if _, _, found := ExtractSection(content); found {
		out = replaceSection(content, markedSection)
	} else if strings.TrimSpace(content) == "" {
		out = markedSection + "\n"
	} else {
		out = strings.TrimRight(content, "\n") + "\n\n" + markedSection + "\n"
	}
	return os.WriteFile(path, []byte(out), 0o644)
}

func replaceSection(content, markedSection string) string {
	loc := beginRe.FindStringIndex(content)
	if loc == nil {
		return content
	}
	after := content[loc[0]:]
	end := endRe.FindStringIndex(after)
	if end == nil {
		return content
	}
	prefix := strings.TrimRight(content[:loc[0]], "\n")
	suffix := strings.TrimLeft(after[end[1]:], "\n")
	if prefix == "" {
		if suffix == "" {
			return markedSection + "\n"
		}
		return markedSection + "\n\n" + suffix + "\n"
	}
	if suffix == "" {
		return prefix + "\n\n" + markedSection + "\n"
	}
	return prefix + "\n\n" + markedSection + "\n\n" + suffix + "\n"
}

// RemoveSection deletes the pine block from path. Returns whether a section existed.
func RemoveSection(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	content := string(data)
	loc := beginRe.FindStringIndex(content)
	if loc == nil {
		return false, nil
	}
	after := content[loc[0]:]
	end := endRe.FindStringIndex(after)
	if end == nil {
		return false, nil
	}
	prefix := strings.TrimRight(content[:loc[0]], "\n")
	suffix := strings.TrimLeft(after[end[1]:], "\n")
	var out string
	switch {
	case prefix == "" && suffix == "":
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return true, err
		}
		return true, nil
	case prefix == "":
		out = suffix + "\n"
	case suffix == "":
		out = prefix + "\n"
	default:
		out = prefix + "\n\n" + suffix + "\n"
	}
	if strings.TrimSpace(out) == "" {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return true, err
		}
		return true, nil
	}
	return true, os.WriteFile(path, []byte(out), 0o644)
}

// CheckFile compares the on-disk section to the expected rendered body.
func CheckFile(path string, expectedBody string, recipe Recipe, version string) InstallStatus {
	data, err := os.ReadFile(path)
	if err != nil {
		return StatusMissing
	}
	body, meta, found := ExtractSection(string(data))
	if !found {
		return StatusMissing
	}
	if !strings.Contains(meta, "recipe="+string(recipe)) {
		return StatusStale
	}
	if !strings.Contains(meta, "version="+version) {
		return StatusStale
	}
	expectedHash := ContentHash(expectedBody)
	if !strings.Contains(meta, "hash="+expectedHash) {
		return StatusStale
	}
	if strings.TrimSpace(body) != strings.TrimSpace(expectedBody) {
		return StatusStale
	}
	return StatusCurrent
}
