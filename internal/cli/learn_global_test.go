package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/underworld14/pine/internal/memory"
)

// globalHome points PINE_HOME at a fresh dir for one test. Mandatory for every
// test that touches global content: TestMain's dir is shared package-wide, so
// without this a stray `learn -g` would leak into a later assertion.
func globalHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(memory.EnvHome, dir)
	return dir
}

func readGlobalMEMORY(t *testing.T, dir string) string {
	t.Helper()
	body, err := memory.ReadMEMORY(dir)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestLearnGlobalWorksOutsideRepo(t *testing.T) {
	// The decision-2 test: fails if openStore() stays unconditional.
	home := globalHome(t)
	bare := t.TempDir() // no .pine anywhere
	out, err := run(t, bare, "learn", "-g", "I use pnpm, never npm")
	if err != nil {
		t.Fatalf("learn -g outside a repo must succeed: %v\n%s", err, out)
	}
	if !strings.Contains(readGlobalMEMORY(t, home), "I use pnpm, never npm") {
		t.Errorf("insight not written to the global store")
	}
}

func TestLearnGlobalAppendsWithoutSuggest(t *testing.T) {
	// The blocker regression. This exact text is what TestLearnMemoryAmbiguousAndJSON
	// proves is "ambiguous" on the project path — -g must not route through Suggest.
	home := globalHome(t)
	out, err := run(t, t.TempDir(), "learn", "-g", "completely novel insight about quux widgets")
	if err != nil {
		t.Fatalf("learn -g must never be ambiguous: %v\n%s", err, out)
	}
	for _, want := range []string{"Appended to", "MEMORY.md"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if !strings.Contains(readGlobalMEMORY(t, home), "quux widgets") {
		t.Errorf("insight not written")
	}
}

func TestLearnGlobalSeedIsPersonal(t *testing.T) {
	home := globalHome(t)
	if _, err := run(t, t.TempDir(), "learn", "-g", "some pref"); err != nil {
		t.Fatal(err)
	}
	body := readGlobalMEMORY(t, home)
	if !strings.Contains(body, "# Personal memory") {
		t.Errorf("global store must use the personal seed:\n%s", body)
	}
	if strings.Contains(body, "for this repository") {
		t.Errorf("project wording leaked into the global seed:\n%s", body)
	}
}

func TestLearnGlobalDoesNotTouchProject(t *testing.T) {
	globalHome(t)
	dir := initRepo(t)
	projectMEM := filepath.Join(dir, ".pine", "MEMORY.md")
	if err := os.WriteFile(projectMEM, []byte("# Project memory\n\n## Log\n- untouched\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(projectMEM)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, dir, "learn", "-g", "a personal preference"); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(projectMEM)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Errorf("learn -g modified the project store:\n%s", after)
	}
}

func TestLearnGlobalNewTopic(t *testing.T) {
	home := globalHome(t)
	dir := initRepo(t)
	if _, err := run(t, dir, "learn", "-g", "prefer conventional commits", "--new-topic", "git-habits"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, "memory", "git-habits.md")); err != nil {
		t.Errorf("global topic not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".pine", "memory", "git-habits.md")); !os.IsNotExist(err) {
		t.Errorf("topic leaked into the project store")
	}
}

func TestLearnGlobalToTopicPath(t *testing.T) {
	home := globalHome(t)
	if _, err := run(t, t.TempDir(), "learn", "-g", "pnpm workspaces need a filter", "--to", "memory/pnpm.md"); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(home, "memory", "pnpm.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "pnpm workspaces need a filter") {
		t.Errorf("--to did not land in the global topic:\n%s", body)
	}
}

func TestLearnGlobalAllowsScopeGlobal(t *testing.T) {
	// --scope global is the default and means "repo-wide"; it must not collide.
	globalHome(t)
	if _, err := run(t, t.TempDir(), "learn", "-g", "--scope", "global", "a pref"); err != nil {
		t.Errorf("-g --scope global must succeed: %v", err)
	}
}

func TestLearnGlobalFlagConflicts(t *testing.T) {
	globalHome(t)
	dir := initRepo(t)
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"learn", "-g", "x", "--scope", "ticket", "--ticket", "BUG-001"}, "--scope ticket"},
		{[]string{"learn", "-g", "x", "--scope", "component", "--component", "web"}, "--scope component"},
		{[]string{"learn", "-g", "x", "--component", "web"}, "--component"},
		{[]string{"learn", "-g", "x", "--legacy-lrn"}, "--legacy-lrn"},
		{[]string{"learn", "-g", "x", "--supersedes", "LRN-001"}, "--supersedes"},
		{[]string{"learn", "-g", "x", "--ticket", "BUG-001"}, "--ticket"},
	}
	for _, c := range cases {
		out, err := run(t, dir, c.args...)
		if err == nil {
			t.Errorf("%v should conflict with --global, got success:\n%s", c.args, out)
			continue
		}
		if !strings.Contains(err.Error(), "--global cannot be combined with "+c.want) {
			t.Errorf("%v: want a %q conflict, got: %v", c.args, c.want, err)
		}
	}
}

func TestLearnGlobalJSONReportsStore(t *testing.T) {
	globalHome(t)
	out, err := run(t, t.TempDir(), "learn", "-g", "a pref", "--json")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, want := range []string{`"store": "global"`, `"path": "MEMORY.md"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestLearnProjectJSONReportsStore(t *testing.T) {
	// Guards the additive JSON change on the project path.
	globalHome(t)
	dir := initRepo(t)
	out, err := run(t, dir, "learn", "a project rule", "--to", "MEMORY.md", "--json")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out, `"store": "project"`) {
		t.Errorf("project writes should report their store:\n%s", out)
	}
}

func TestLearnListGlobal(t *testing.T) {
	globalHome(t)
	if _, err := run(t, t.TempDir(), "learn", "-g", "a pref"); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, t.TempDir(), "learn", "list", "-g")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out, "Global MEMORY / topics:") {
		t.Errorf("want the global header:\n%s", out)
	}
	if strings.Contains(out, "LRN learnings:") {
		t.Errorf("LRN files are project-only; must not appear under -g:\n%s", out)
	}
}

func TestLearnListGlobalJSON(t *testing.T) {
	// Guards the s.ListLearnings nil-deref.
	globalHome(t)
	if _, err := run(t, t.TempDir(), "learn", "-g", "a pref"); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, t.TempDir(), "learn", "list", "-g", "--json")
	if err != nil {
		t.Fatalf("list -g --json must not fail: %v\n%s", err, out)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if got["store"] != "global" {
		t.Errorf("want store=global, got %v", got["store"])
	}
}

func TestLearnSearchGlobalJSON(t *testing.T) {
	// Guards the s.AllLearnings nil-deref.
	globalHome(t)
	if _, err := run(t, t.TempDir(), "learn", "-g", "pnpm is my package manager"); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, t.TempDir(), "learn", "search", "-g", "pnpm", "--json")
	if err != nil {
		t.Fatalf("search -g --json must not fail: %v\n%s", err, out)
	}
	var got []map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
}

func TestLearnListGlobalOutsideRepo(t *testing.T) {
	globalHome(t)
	if _, err := run(t, t.TempDir(), "learn", "-g", "a pref"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, t.TempDir(), "learn", "list", "-g"); err != nil {
		t.Errorf("list -g must work outside a repo: %v", err)
	}
}

func TestLearnSearchGlobalOutsideRepo(t *testing.T) {
	globalHome(t)
	if _, err := run(t, t.TempDir(), "learn", "-g", "a pref"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, t.TempDir(), "learn", "search", "-g", "pref"); err != nil {
		t.Errorf("search -g must work outside a repo: %v", err)
	}
}

func TestLearnListGlobalCreatesNothing(t *testing.T) {
	// Guards the deleted EnsureLayout in listMemoryEntries: a read must never
	// create the store, least of all seeding it with the project text.
	dir := filepath.Join(t.TempDir(), "absent")
	t.Setenv(memory.EnvHome, dir)
	out, err := run(t, t.TempDir(), "learn", "list", "-g")
	if err != nil {
		t.Fatalf("list -g on a missing store must succeed: %v\n%s", err, out)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("learn list -g created the global store at %s", dir)
	}
}

func TestLearnSearchGlobal(t *testing.T) {
	globalHome(t)
	if _, err := run(t, t.TempDir(), "learn", "-g", "always squash before merging", "--new-topic", "git-habits"); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, t.TempDir(), "learn", "search", "-g", "squash")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out, "PATH") || !strings.Contains(out, "git-habits") {
		t.Errorf("want the global topic in a PATH/SCORE table:\n%s", out)
	}
}

func TestLearnShowGlobal(t *testing.T) {
	globalHome(t)
	if _, err := run(t, t.TempDir(), "learn", "-g", "a personal pref"); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, t.TempDir(), "learn", "show", "-g", "MEMORY.md")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, want := range []string{"# Personal memory", "a personal pref"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestLearnShowGlobalUnknownErrors(t *testing.T) {
	globalHome(t)
	if _, err := run(t, t.TempDir(), "learn", "-g", "a pref"); err != nil {
		t.Fatal(err)
	}
	_, err := run(t, t.TempDir(), "learn", "show", "-g", "LRN-001")
	if err == nil {
		t.Fatal("LRN ids do not exist in the global store")
	}
	if !strings.Contains(err.Error(), "memory/<topic>.md") {
		t.Errorf("error should point at the valid shapes, got: %v", err)
	}
}

func TestContextDoesNotCreateGlobalStore(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "absent")
	t.Setenv(memory.EnvHome, dir)
	repo := initRepo(t)
	if _, err := run(t, repo, "context"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("pine context created the global store at %s", dir)
	}
}

func TestContextIncludesGlobalPreferences(t *testing.T) {
	globalHome(t)
	repo := initRepo(t)
	if _, err := run(t, repo, "learn", "-g", "I use pnpm, never npm"); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, repo, "context")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"## Your Preferences (global)",
		"Project Memory wins",
		"I use pnpm, never npm",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestLearnGlobalErrorsWhenStoreUnwritable(t *testing.T) {
	// PINE_HOME points at a regular file: -g must fail cleanly, not panic.
	f := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(memory.EnvHome, f)
	if _, err := run(t, t.TempDir(), "learn", "-g", "a pref"); err == nil {
		t.Fatal("expected an error when the global store cannot be created")
	}
}

func TestLearnListGlobalJSONEmptyStore(t *testing.T) {
	// A resolvable but empty store: JSON must still be valid, with no memory key.
	t.Setenv(memory.EnvHome, t.TempDir())
	out, err := run(t, t.TempDir(), "learn", "list", "-g", "--json")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if got["store"] != "global" {
		t.Errorf("want store=global, got %v", got["store"])
	}
	if topics, ok := got["topics"].([]any); !ok || len(topics) != 0 {
		t.Errorf("want an empty topics array, got %v", got["topics"])
	}
}

func TestFindPineDirSkipsGlobalOnlyStore(t *testing.T) {
	// `pine learn -g` creates ~/.pine. From a non-repo dir under $HOME the
	// upward walk must not mistake that private store for this project's, or
	// every command fails on its absent config.json instead of saying there is
	// no project here.
	home := t.TempDir()
	t.Setenv(memory.EnvHome, filepath.Join(home, ".pine"))
	if _, err := run(t, t.TempDir(), "learn", "-g", "prefer tabs"); err != nil {
		t.Fatal(err)
	}
	scratch := filepath.Join(home, "scratch")
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := run(t, scratch, "learn", "x") // the -g they forgot
	if err == nil {
		t.Fatal("expected an error: there is no project store here")
	}
	if !strings.Contains(err.Error(), "no .pine directory found here or above") {
		t.Errorf("want the actionable no-project error, got: %v", err)
	}
	if strings.Contains(err.Error(), "config.json") {
		t.Errorf("must not surface the global store as a broken project: %v", err)
	}
}

func TestFindPineDirKeepsGlobalPathWhenItIsARealProject(t *testing.T) {
	// A ~/.pine that someone deliberately ran `pine init` in has a config.json
	// and is a real project store — the skip must not break it.
	home := t.TempDir()
	t.Setenv(memory.EnvHome, filepath.Join(home, ".pine"))
	if _, err := run(t, home, "init", "--skip-agents"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".pine", "config.json")); err != nil {
		t.Fatalf("precondition: init should have written config.json: %v", err)
	}
	if _, err := run(t, home, "learn", "a project rule", "--to", "MEMORY.md"); err != nil {
		t.Errorf("a real project at the global path must still resolve: %v", err)
	}
}
