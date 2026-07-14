package config

import (
	"encoding/json"
	"os"
	"path/filepath"
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

func TestSyncDefaultsAndRoundTrip(t *testing.T) {
	c := Default("x")
	if !c.Sync.Tickets || c.Sync.Attachments {
		t.Errorf("default sync = %+v, want {tickets:true attachments:false}", c.Sync)
	}
	out, err := c.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"sync"`) || !strings.Contains(string(out), `"tickets"`) {
		t.Errorf("sync not serialized:\n%s", out)
	}
	if !strings.Contains(string(out), `"attachments": false`) && !strings.Contains(string(out), `"attachments":false`) {
		t.Errorf("attachments:false should be explicit:\n%s", out)
	}

	old, err := Parse([]byte(`{"version":1,"project":{"name":"x"},"types":[{"prefix":"BUG","name":"Bug"}],"priorities":["low"],"idStyle":"hash"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !old.Sync.Tickets || old.Sync.Attachments {
		t.Errorf("legacy config sync = %+v, want defaults", old.Sync)
	}

	partial, err := Parse([]byte(`{"sync":{"attachments":true}}`))
	if err != nil {
		t.Fatal(err)
	}
	if !partial.Sync.Tickets {
		t.Errorf("tickets should stay true when omitted")
	}
	if !partial.Sync.Attachments {
		t.Errorf("attachments should be true")
	}

	both, err := Parse([]byte(`{"sync":{"tickets":false,"attachments":true}}`))
	if err != nil {
		t.Fatal(err)
	}
	if both.Sync.Tickets || !both.Sync.Attachments {
		t.Errorf("got %+v", both.Sync)
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

// --- Load / LoadBoard ---

func TestLoadConfigHappyPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	src := `{
  "version": 1,
  "project": {"name": "loaded-app"},
  "types": [{"prefix": "BUG", "name": "Bug"}],
  "priorities": ["low", "high"],
  "git": {"backend": "cli"}
}`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if c.Project.Name != "loaded-app" {
		t.Errorf("project name = %q", c.Project.Name)
	}
	if c.Git.Backend != "cli" {
		t.Errorf("backend = %q", c.Git.Backend)
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist.json")
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing config file")
	}
}

func TestLoadConfigMalformedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"version": 1,`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for malformed config JSON")
	}
	if !strings.Contains(err.Error(), "config.json is not valid JSON") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestLoadBoardHappyPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "board.json")
	src := `{"version": 1, "columns": [{"status": "todo", "title": "Todo"}, {"status": "done", "title": "Done"}]}`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := LoadBoard(path)
	if err != nil {
		t.Fatalf("LoadBoard returned error: %v", err)
	}
	if len(b.Columns) != 2 {
		t.Errorf("columns = %d, want 2", len(b.Columns))
	}
	if b.Columns[0].Status != "todo" || b.Columns[1].Status != "done" {
		t.Errorf("unexpected columns: %+v", b.Columns)
	}
}

func TestLoadBoardMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist.json")
	_, err := LoadBoard(path)
	if err == nil {
		t.Fatal("expected error for missing board file")
	}
}

func TestLoadBoardMalformedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "board.json")
	if err := os.WriteFile(path, []byte(`not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadBoard(path)
	if err == nil {
		t.Fatal("expected error for malformed board JSON")
	}
	if !strings.Contains(err.Error(), "board.json is not valid JSON") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// --- Board.Statuses ---

func TestBoardStatusesOrder(t *testing.T) {
	b := &Board{Version: 1, Columns: []Column{
		{Status: "todo", Title: "Todo"},
		{Status: "doing", Title: "Doing"},
		{Status: "review", Title: "Review"},
		{Status: "done", Title: "Done"},
	}}
	statuses := b.Statuses()
	want := []string{"todo", "doing", "review", "done"}
	if len(statuses) != len(want) {
		t.Fatalf("statuses = %v, want %v", statuses, want)
	}
	for i, s := range want {
		if statuses[i] != s {
			t.Errorf("statuses[%d] = %q, want %q", i, statuses[i], s)
		}
	}
}

func TestBoardStatusesEmpty(t *testing.T) {
	b := &Board{Version: 1}
	statuses := b.Statuses()
	if len(statuses) != 0 {
		t.Errorf("statuses = %v, want empty", statuses)
	}
}

// --- Additional Validate branches ---

func TestConfigValidateVersionZero(t *testing.T) {
	c := Default("x")
	c.Version = 0
	problems := c.Validate()
	if !containsSubstring(problems, "config.version must be >= 1") {
		t.Errorf("expected version problem, got %v", problems)
	}
}

func TestConfigValidateEmptyTypes(t *testing.T) {
	c := Default("x")
	c.Types = nil
	problems := c.Validate()
	if !containsSubstring(problems, "config.types must not be empty") {
		t.Errorf("expected types problem, got %v", problems)
	}
}

func TestConfigValidateBadTypePrefix(t *testing.T) {
	c := Default("x")
	c.Types = []TicketType{{Prefix: "bug", Name: "Bug"}}
	problems := c.Validate()
	found := false
	for _, p := range problems {
		if strings.Contains(p, "must be uppercase letters/digits") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected bad prefix problem, got %v", problems)
	}
}

func TestConfigValidateEmptyPriorities(t *testing.T) {
	c := Default("x")
	c.Priorities = nil
	problems := c.Validate()
	if !containsSubstring(problems, "config.priorities must not be empty") {
		t.Errorf("expected priorities problem, got %v", problems)
	}
}

func TestConfigValidateQualityTooLow(t *testing.T) {
	c := Default("x")
	c.Attachments.Quality = 0
	problems := c.Validate()
	if !containsSubstring(problems, "config.attachments.quality must be 1..100") {
		t.Errorf("expected quality problem, got %v", problems)
	}
}

func TestConfigValidateMaxDimensionZero(t *testing.T) {
	c := Default("x")
	c.Attachments.MaxDimension = 0
	problems := c.Validate()
	if !containsSubstring(problems, "config.attachments.maxDimension must be > 0") {
		t.Errorf("expected maxDimension problem, got %v", problems)
	}
}

func TestBoardValidateVersionZero(t *testing.T) {
	b := DefaultBoard()
	b.Version = 0
	problems := b.Validate()
	if !containsSubstring(problems, "board.version must be >= 1") {
		t.Errorf("expected version problem, got %v", problems)
	}
}

func TestBoardValidateEmptyColumns(t *testing.T) {
	b := &Board{Version: 1}
	problems := b.Validate()
	if !containsSubstring(problems, "board.columns must not be empty") {
		t.Errorf("expected columns problem, got %v", problems)
	}
}

func TestBoardValidateEmptyStatus(t *testing.T) {
	b := &Board{Version: 1, Columns: []Column{{Status: "  ", Title: "Blank"}}}
	problems := b.Validate()
	if !containsSubstring(problems, "board column has an empty status") {
		t.Errorf("expected empty status problem, got %v", problems)
	}
}

func TestBoardValidateEmptyTitle(t *testing.T) {
	b := &Board{Version: 1, Columns: []Column{{Status: "todo", Title: "  "}}}
	problems := b.Validate()
	found := false
	for _, p := range problems {
		if strings.Contains(p, `board column "todo" has an empty title`) {
			found = true
		}
	}
	if !found {
		t.Errorf("expected empty title problem, got %v", problems)
	}
}

func containsSubstring(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// --- FirstStatus / HasStatus / TypeByPrefix / HasPriority edge cases ---

func TestFirstStatusEmptyBoard(t *testing.T) {
	b := &Board{Version: 1}
	if got := b.FirstStatus(); got != "todo" {
		t.Errorf("FirstStatus() on empty board = %q, want %q", got, "todo")
	}
}

func TestHasStatusNotFound(t *testing.T) {
	b := DefaultBoard()
	if b.HasStatus("nonexistent") {
		t.Errorf("HasStatus should be false for unknown status")
	}
}

func TestHasStatusEmptyBoard(t *testing.T) {
	b := &Board{Version: 1}
	if b.HasStatus("todo") {
		t.Errorf("HasStatus should be false on empty board")
	}
}

func TestTypeByPrefixNotFound(t *testing.T) {
	c := Default("x")
	if _, ok := c.TypeByPrefix("NOPE"); ok {
		t.Errorf("TypeByPrefix should not find NOPE")
	}
}

func TestTypeByPrefixEmptyConfig(t *testing.T) {
	c := &Config{}
	if _, ok := c.TypeByPrefix("BUG"); ok {
		t.Errorf("TypeByPrefix should not find anything on empty config")
	}
}

func TestTypeByPrefixCaseInsensitive(t *testing.T) {
	c := Default("x")
	tt, ok := c.TypeByPrefix("bug")
	if !ok {
		t.Fatal("TypeByPrefix should match lowercase input against uppercase prefix")
	}
	if tt.Prefix != "BUG" {
		t.Errorf("TypeByPrefix returned %+v", tt)
	}
}

func TestHasPriorityNotFound(t *testing.T) {
	c := Default("x")
	if c.HasPriority("nonexistent") {
		t.Errorf("HasPriority should be false for unknown priority")
	}
}

func TestHasPriorityEmptyConfig(t *testing.T) {
	c := &Config{}
	if c.HasPriority("low") {
		t.Errorf("HasPriority should be false on empty config")
	}
}

func TestHasPriorityCaseInsensitive(t *testing.T) {
	c := Default("x")
	if !c.HasPriority("CRITICAL") {
		t.Errorf("HasPriority should match case-insensitively")
	}
}
