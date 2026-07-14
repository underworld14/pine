package server

import (
	"testing"
)

func TestSuggestFileItemsRankingAndDirs(t *testing.T) {
	tracked := []string{
		"README.md",
		"src/login.tsx",
		"src/lib/api.ts",
		"src/lib/ui.ts",
		"web/package.json",
	}

	items := suggestFileItems(tracked, "login", 50)
	if len(items) == 0 {
		t.Fatal("expected matches for login")
	}
	if items[0].Path != "src/login.tsx" || items[0].Kind != "file" {
		t.Fatalf("top hit = %+v, want src/login.tsx file", items[0])
	}

	items = suggestFileItems(tracked, "lib", 50)
	var sawDir, sawFile bool
	for _, it := range items {
		if it.Path == "src/lib/" && it.Kind == "dir" {
			sawDir = true
		}
		if it.Path == "src/lib/api.ts" && it.Kind == "file" {
			sawFile = true
		}
	}
	if !sawDir || !sawFile {
		t.Fatalf("expected src/lib/ dir and a file under it: %+v", items)
	}

	items = suggestFileItems(tracked, "", 3)
	if len(items) != 3 {
		t.Fatalf("empty q capped: got %d, want 3", len(items))
	}
	// Empty query must not require sorting the whole tree — still returns something useful.
	big := make([]string, 0, 200)
	for i := 0; i < 200; i++ {
		big = append(big, "pkg/mod/file"+string(rune('a'+i%26))+".go")
	}
	items = suggestFileItems(big, "", 5)
	if len(items) != 5 {
		t.Fatalf("empty q on large tree: got %d, want 5", len(items))
	}

	items = suggestFileItems(tracked, "zzz-nope", 50)
	if len(items) != 0 {
		t.Fatalf("expected no matches: %+v", items)
	}
}

func TestSuggestFileItemsBasenameBeatsSubstring(t *testing.T) {
	tracked := []string{
		"vendor/login-helper/x.go",
		"src/login.ts",
	}
	items := suggestFileItems(tracked, "login", 10)
	if len(items) < 2 {
		t.Fatalf("want both matches: %+v", items)
	}
	if items[0].Path != "src/login.ts" {
		t.Fatalf("basename match should rank first: %+v", items)
	}
}
