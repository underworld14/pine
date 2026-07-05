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

func TestParseOntoPreservesOmittedKeys(t *testing.T) {
	base := Default("my-app")
	base.Attachments.Quality = 55
	updated, err := ParseOnto(base, []byte(`{"git":{"backend":"cli"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if updated.Git.Backend != "cli" {
		t.Errorf("present key not applied: %q", updated.Git.Backend)
	}
	if updated.Attachments.Quality != 55 {
		t.Errorf("omitted key was reset to default: %d", updated.Attachments.Quality)
	}
	if updated.Project.Name != "my-app" {
		t.Errorf("omitted project name was reset")
	}
	if base.Git.Backend == "cli" {
		t.Errorf("ParseOnto must not mutate base")
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

func TestIDStyleDefaultAndValidation(t *testing.T) {
	if c := Default("x"); c.IDStyle != "hash" {
		t.Errorf("default idStyle = %q, want hash", c.IDStyle)
	}
	bad := Default("x")
	bad.IDStyle = "weird"
	if len(bad.Validate()) == 0 {
		t.Errorf("invalid idStyle should be flagged")
	}
	seq := Default("x")
	seq.IDStyle = "sequential"
	if problems := seq.Validate(); len(problems) != 0 {
		t.Errorf("sequential should be valid: %v", problems)
	}
}

func TestCrossBranchDefaultsAndRoundTrip(t *testing.T) {
	c := Default("x")
	if !c.CrossBranch.Enabled || c.CrossBranch.ActiveBranchDays != 30 {
		t.Errorf("default crossBranch = %+v, want {true 30}", c.CrossBranch)
	}
	out, err := c.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"crossBranch"`) || !strings.Contains(string(out), `"activeBranchDays"`) {
		t.Errorf("crossBranch not serialized:\n%s", out)
	}

	// Backward-compat: a config predating the feature (no crossBranch key)
	// inherits the enabled-by-default value.
	old, err := Parse([]byte(`{"version":1,"project":{"name":"x"},"types":[{"prefix":"BUG","name":"Bug"}],"priorities":["low"],"idStyle":"hash"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !old.CrossBranch.Enabled || old.CrossBranch.ActiveBranchDays != 30 {
		t.Errorf("legacy config crossBranch = %+v, want {true 30}", old.CrossBranch)
	}

	// A partial crossBranch object keeps unspecified fields at their default.
	partial, err := Parse([]byte(`{"crossBranch":{"enabled":false}}`))
	if err != nil {
		t.Fatal(err)
	}
	if partial.CrossBranch.Enabled {
		t.Errorf("enabled should be false")
	}
	if partial.CrossBranch.ActiveBranchDays != 30 {
		t.Errorf("activeBranchDays should stay 30, got %d", partial.CrossBranch.ActiveBranchDays)
	}
}

func TestCrossBranchValidation(t *testing.T) {
	c := Default("x")
	c.CrossBranch.ActiveBranchDays = 0
	if len(c.Validate()) == 0 {
		t.Errorf("activeBranchDays=0 should be flagged")
	}
	// enabled + sequential is NOT a hard validation error (doctor warns instead).
	seq := Default("x")
	seq.IDStyle = "sequential"
	seq.CrossBranch.Enabled = true
	if problems := seq.Validate(); len(problems) != 0 {
		t.Errorf("enabled+sequential should not be a hard error: %v", problems)
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
