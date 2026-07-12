package setup

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/underworld14/pine/internal/tui"
)

func newTestRunner(dir string) (*Runner, *bytes.Buffer) {
	var buf bytes.Buffer
	r := &Runner{Root: dir, Version: "0.1.0", Opts: RenderOptions{}, Out: &buf}
	return r, &buf
}

func TestRunnerInstallAllRecipes(t *testing.T) {
	dir := t.TempDir()
	r, buf := newTestRunner(dir)

	if err := r.Install(AllRecipes); err != nil {
		t.Fatalf("install: %v", err)
	}

	for _, info := range Registry() {
		if info.File == "" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, info.File))
		if err != nil {
			t.Fatalf("missing %s: %v", info.File, err)
		}
		if !strings.Contains(string(data), "pine:begin") {
			t.Fatalf("%s missing pine section:\n%s", info.File, data)
		}
	}
	if CheckCodexHook(dir) != StatusCurrent {
		t.Fatalf("expected Codex hook current after install-all, got %s", CheckCodexHook(dir))
	}
	if CheckCursorHook(dir) != StatusCurrent {
		t.Fatalf("expected Cursor hook current after install-all, got %s", CheckCursorHook(dir))
	}
	if !strings.Contains(buf.String(), "installed AGENTS.md") {
		t.Fatalf("expected install output, got:\n%s", buf.String())
	}
}

func TestRunnerInstallUnknownRecipe(t *testing.T) {
	dir := t.TempDir()
	r, _ := newTestRunner(dir)
	err := r.Install([]Recipe{Recipe("bogus")})
	if err == nil || !strings.Contains(err.Error(), "unknown setup recipe: bogus") {
		t.Fatalf("expected unknown recipe error, got %v", err)
	}
}

func TestRunnerRemove(t *testing.T) {
	dir := t.TempDir()
	r, buf := newTestRunner(dir)

	if err := r.Install([]Recipe{RecipeAgents}); err != nil {
		t.Fatal(err)
	}
	buf.Reset()

	if err := r.Remove([]Recipe{RecipeAgents}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if !strings.Contains(buf.String(), "removed section from AGENTS.md") {
		t.Fatalf("expected removed output, got:\n%s", buf.String())
	}

	// A second removal should report no section present.
	buf.Reset()
	if err := r.Remove([]Recipe{RecipeAgents}); err != nil {
		t.Fatalf("second remove: %v", err)
	}
	if !strings.Contains(buf.String(), "no pine section in AGENTS.md") {
		t.Fatalf("expected no-section output, got:\n%s", buf.String())
	}
}

func TestRunnerRemoveUnknownRecipe(t *testing.T) {
	dir := t.TempDir()
	r, _ := newTestRunner(dir)
	err := r.Remove([]Recipe{Recipe("bogus")})
	if err == nil || !strings.Contains(err.Error(), "unknown setup recipe: bogus") {
		t.Fatalf("expected unknown recipe error, got %v", err)
	}
}

func TestRunnerCheck(t *testing.T) {
	dir := t.TempDir()
	r, buf := newTestRunner(dir)

	if err := r.Check([]Recipe{RecipeAgents}); err != nil {
		t.Fatalf("check: %v", err)
	}
	if !strings.Contains(buf.String(), "missing") {
		t.Fatalf("expected missing status, got:\n%s", buf.String())
	}

	if err := r.Install([]Recipe{RecipeAgents}); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	if err := r.Check([]Recipe{RecipeAgents}); err != nil {
		t.Fatalf("check: %v", err)
	}
	if !strings.Contains(buf.String(), "current") {
		t.Fatalf("expected current status, got:\n%s", buf.String())
	}
}

func TestRunnerCheckUnknownRecipe(t *testing.T) {
	dir := t.TempDir()
	r, _ := newTestRunner(dir)
	err := r.Check([]Recipe{Recipe("bogus")})
	if err == nil || !strings.Contains(err.Error(), "unknown setup recipe: bogus") {
		t.Fatalf("expected unknown recipe error, got %v", err)
	}
}

func TestRunnerPrint(t *testing.T) {
	dir := t.TempDir()
	r, buf := newTestRunner(dir)

	if err := r.Print([]Recipe{RecipeAgents, RecipeClaude}); err != nil {
		t.Fatalf("print: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "recipe=agents") || !strings.Contains(out, "recipe=claude") {
		t.Fatalf("expected both sections rendered:\n%s", out)
	}
	if !strings.Contains(out, "---") {
		t.Fatalf("expected separator between multiple recipes:\n%s", out)
	}
}

func TestRunnerPrintUnknownRecipe(t *testing.T) {
	dir := t.TempDir()
	r, _ := newTestRunner(dir)
	err := r.Print([]Recipe{Recipe("bogus")})
	if err == nil || !strings.Contains(err.Error(), "unknown setup recipe: bogus") {
		t.Fatalf("expected unknown recipe error, got %v", err)
	}
}

func TestRunnerList(t *testing.T) {
	dir := t.TempDir()
	r, buf := newTestRunner(dir)
	r.List()
	out := buf.String()
	for _, info := range Registry() {
		if !strings.Contains(out, info.File) || !strings.Contains(out, info.Description) {
			t.Fatalf("expected list to mention %s:\n%s", info.File, out)
		}
	}
}

func TestRepoRootFindsGitDir(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := RepoRoot(nested)
	if err != nil {
		t.Fatalf("RepoRoot: %v", err)
	}
	// Resolve symlinks (e.g. macOS /tmp -> /private/tmp) before comparing.
	wantAbs, _ := filepath.EvalSymlinks(root)
	gotAbs, _ := filepath.EvalSymlinks(got)
	if gotAbs != wantAbs {
		t.Fatalf("RepoRoot = %s, want %s", got, root)
	}
}

func TestRepoRootFallsBackToStart(t *testing.T) {
	// No .git anywhere above this dir (a fresh temp dir tree has none).
	start := t.TempDir()
	nested := filepath.Join(start, "x", "y")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := RepoRoot(nested)
	if err != nil {
		t.Fatalf("RepoRoot: %v", err)
	}
	gotAbs, _ := filepath.EvalSymlinks(got)
	wantAbs, _ := filepath.EvalSymlinks(nested)
	if gotAbs != wantAbs {
		t.Fatalf("RepoRoot = %s, want fallback to start %s", got, nested)
	}
}

func TestHasPineTrueFromNestedDir(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".pine"), 0o755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "sub", "dir")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	if !HasPine(nested) {
		t.Fatalf("expected HasPine to find .pine at ancestor of %s", nested)
	}
}

func TestHasPineFalseWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	if HasPine(dir) {
		t.Fatalf("expected HasPine to be false for a directory with no .pine ancestor")
	}
}

func TestHasPineFalseWhenFileNotDir(t *testing.T) {
	dir := t.TempDir()
	// A file named .pine (not a directory) must not count.
	if err := os.WriteFile(filepath.Join(dir, ".pine"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if HasPine(dir) {
		t.Fatalf("expected HasPine to be false when .pine is a regular file")
	}
}

func TestErrUnknownRecipeError(t *testing.T) {
	err := errUnknownRecipe(Recipe("frobnicate"))
	if err.Error() != "unknown setup recipe: frobnicate" {
		t.Fatalf("unexpected error message: %q", err.Error())
	}
}

func TestWizardDefaultSelection(t *testing.T) {
	var buf bytes.Buffer
	r := &Runner{
		Out: &buf,
		PickRecipes: func(infos []RecipeInfo) ([]Recipe, error) {
			return []Recipe{RecipeAgents}, nil
		},
	}

	got, err := r.Wizard(true)
	if err != nil {
		t.Fatalf("wizard: %v", err)
	}
	if len(got) != 1 || got[0] != RecipeAgents {
		t.Fatalf("expected default [agents], got %v", got)
	}
}

func TestWizardToggleSelection(t *testing.T) {
	var buf bytes.Buffer
	r := &Runner{
		Out: &buf,
		PickRecipes: func(infos []RecipeInfo) ([]Recipe, error) {
			return []Recipe{RecipeClaude}, nil
		},
	}

	got, err := r.Wizard(true)
	if err != nil {
		t.Fatalf("wizard: %v", err)
	}
	if len(got) != 1 || got[0] != RecipeClaude {
		t.Fatalf("expected [claude], got %v", got)
	}
}

func TestWizardQuit(t *testing.T) {
	var buf bytes.Buffer
	r := &Runner{
		Out: &buf,
		PickRecipes: func(infos []RecipeInfo) ([]Recipe, error) {
			return nil, tui.ErrCancelled
		},
	}

	_, err := r.Wizard(true)
	if err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("expected cancellation error, got %v", err)
	}
}

func TestWizardNoToolsSelected(t *testing.T) {
	var buf bytes.Buffer
	r := &Runner{
		Out: &buf,
		PickRecipes: func(infos []RecipeInfo) ([]Recipe, error) {
			return nil, nil
		},
	}

	_, err := r.Wizard(true)
	if err == nil || !strings.Contains(err.Error(), "no tools selected") {
		t.Fatalf("expected no-tools-selected error, got %v", err)
	}
}

func TestWizardPickError(t *testing.T) {
	var buf bytes.Buffer
	r := &Runner{
		Out: &buf,
		PickRecipes: func(infos []RecipeInfo) ([]Recipe, error) {
			return nil, fmt.Errorf("boom")
		},
	}

	_, err := r.Wizard(true)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected boom error, got %v", err)
	}
}

func TestWizardNoPineDeclined(t *testing.T) {
	var buf bytes.Buffer
	r := &Runner{
		Out: &buf,
		Confirm: func(title, description string, defaultYes bool) (bool, error) {
			if !strings.Contains(title, "No .pine") {
				t.Fatalf("unexpected confirm title: %q", title)
			}
			if defaultYes {
				t.Fatal("expected default No")
			}
			return false, nil
		},
	}

	_, err := r.Wizard(false)
	if err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("expected cancellation when declining without .pine, got %v", err)
	}
}

func TestWizardNoPineConfirmCancelled(t *testing.T) {
	var buf bytes.Buffer
	r := &Runner{
		Out: &buf,
		Confirm: func(title, description string, defaultYes bool) (bool, error) {
			return false, tui.ErrCancelled
		},
	}

	_, err := r.Wizard(false)
	if err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("expected cancellation, got %v", err)
	}
}

func TestWizardNoPineAccepted(t *testing.T) {
	var buf bytes.Buffer
	r := &Runner{
		Out: &buf,
		Confirm: func(title, description string, defaultYes bool) (bool, error) {
			return true, nil
		},
		PickRecipes: func(infos []RecipeInfo) ([]Recipe, error) {
			return []Recipe{RecipeAgents, RecipeClaude}, nil
		},
	}

	got, err := r.Wizard(false)
	if err != nil {
		t.Fatalf("wizard: %v", err)
	}
	if len(got) != 2 || got[0] != RecipeAgents || got[1] != RecipeClaude {
		t.Fatalf("expected [agents claude], got %v", got)
	}
}

func TestPickRecipesViaMultiSelectHook(t *testing.T) {
	var buf bytes.Buffer
	r := &Runner{
		Out: &buf,
		MultiSelect: func(title string, choices []tui.Choice) ([]string, error) {
			if title == "" || len(choices) == 0 {
				t.Fatal("expected title and choices")
			}
			// agents is pre-selected in choices; pick claude + agents
			return []string{string(RecipeAgents), string(RecipeClaude)}, nil
		},
	}
	got, err := r.Wizard(true)
	if err != nil {
		t.Fatalf("wizard: %v", err)
	}
	if len(got) != 2 || got[0] != RecipeAgents || got[1] != RecipeClaude {
		t.Fatalf("got %v", got)
	}
}

func TestPickRecipesMultiSelectError(t *testing.T) {
	r := &Runner{
		Out: io.Discard,
		MultiSelect: func(title string, choices []tui.Choice) ([]string, error) {
			return nil, tui.ErrCancelled
		},
	}
	_, err := r.Wizard(true)
	if err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("err=%v", err)
	}
}

func TestConfirmDefaultPathUsesHook(t *testing.T) {
	// Cover confirm() when Confirm is set (already covered) and when using MultiSelect path.
	called := false
	r := &Runner{
		Out: io.Discard,
		Confirm: func(title, description string, defaultYes bool) (bool, error) {
			called = true
			return true, nil
		},
		PickRecipes: func(infos []RecipeInfo) ([]Recipe, error) {
			return []Recipe{RecipeAgents}, nil
		},
	}
	got, err := r.Wizard(false)
	if err != nil {
		t.Fatal(err)
	}
	if !called || len(got) != 1 {
		t.Fatalf("called=%v got=%v", called, got)
	}
}
