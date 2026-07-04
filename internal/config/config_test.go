package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDefaultConfigValid(t *testing.T) {
	c := Default("my-app")
	if problems := c.Validate(); len(problems) != 0 {
		t.Fatalf("default config invalid: %v", problems)
	}
	if _, ok := c.TypeByPrefix("EPIC"); !ok {
		t.Errorf("default config should include EPIC type")
	}
	if !c.HasPriority("critical") {
		t.Errorf("default config should have critical priority")
	}
}

func TestConfigRoundTripPreservesUnknownKeys(t *testing.T) {
	src := `{
  "version": 1,
  "project": {"name": "my-app"},
  "types": [{"prefix": "BUG", "name": "Bug"}],
  "priorities": ["low", "high"],
  "attachments": {"optimize": false, "maxDimension": 1000, "quality": 60, "maxVideoMB": 20},
  "git": {"backend": "cli"},
  "experimental": {"aiTriage": true}
}`
	c, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if c.Attachments.Optimize {
		t.Errorf("optimize should be false")
	}
	if c.Git.Backend != "cli" {
		t.Errorf("backend = %q", c.Git.Backend)
	}
	out, err := c.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "experimental") || !strings.Contains(string(out), "aiTriage") {
		t.Errorf("unknown key not preserved:\n%s", out)
	}
	// Canonical order: version before git before the appended extra key.
	s := string(out)
	if strings.Index(s, `"version"`) > strings.Index(s, `"git"`) {
		t.Errorf("version should precede git")
	}
	if strings.Index(s, `"git"`) > strings.Index(s, `"experimental"`) {
		t.Errorf("known keys should precede extras")
	}
}

func TestConfigPartialAttachmentsGetsDefaults(t *testing.T) {
	c, err := Parse([]byte(`{"attachments": {"quality": 90}}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.Attachments.Quality != 90 {
		t.Errorf("quality = %d", c.Attachments.Quality)
	}
	if c.Attachments.MaxDimension != 2000 {
		t.Errorf("maxDimension should default to 2000, got %d", c.Attachments.MaxDimension)
	}
}

func TestConfigValidateCatchesProblems(t *testing.T) {
	c := Default("x")
	c.Git.Backend = "svn"
	c.Attachments.Quality = 200
	problems := c.Validate()
	if len(problems) < 2 {
		t.Errorf("expected >=2 problems, got %v", problems)
	}
}

func TestBoardDefaultsAndValidate(t *testing.T) {
	b := DefaultBoard()
	if problems := b.Validate(); len(problems) != 0 {
		t.Fatalf("default board invalid: %v", problems)
	}
	if b.FirstStatus() != "todo" {
		t.Errorf("first status = %q", b.FirstStatus())
	}
	if !b.HasStatus("done") {
		t.Errorf("board should have done column")
	}
}

func TestBoardDuplicateStatusInvalid(t *testing.T) {
	b := &Board{Version: 1, Columns: []Column{
		{Status: "todo", Title: "Todo"},
		{Status: "todo", Title: "Again"},
	}}
	if problems := b.Validate(); len(problems) == 0 {
		t.Errorf("expected duplicate-status problem")
	}
}

func TestBoardRoundTrip(t *testing.T) {
	b := DefaultBoard()
	out, err := b.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	var b2 *Board
	b2, err = ParseBoard(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(b2.Columns) != 4 {
		t.Errorf("columns = %d", len(b2.Columns))
	}
	// Ensure valid JSON output.
	var check map[string]json.RawMessage
	if err := json.Unmarshal(out, &check); err != nil {
		t.Errorf("board output not valid JSON: %v", err)
	}
}
