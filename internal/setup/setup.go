package setup

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/underworld14/pine/internal/tui"
)

// Runner performs setup operations against a repository root.
type Runner struct {
	Root    string // repository root (parent of .pine)
	Version string
	Opts    RenderOptions
	Out     io.Writer
	In      io.Reader

	// Confirm, when set, replaces the interactive Yes/No prompt (tests inject fakes).
	Confirm func(title, description string, defaultYes bool) (bool, error)
	// PickRecipes, when set, replaces the interactive multi-select (tests inject fakes).
	PickRecipes func(infos []RecipeInfo) ([]Recipe, error)
	// MultiSelect, when set, replaces tui.MultiSelectIO inside the default pickRecipes path.
	MultiSelect func(title string, choices []tui.Choice) ([]string, error)
}

// Install writes or updates sections for the given recipes.
func (r *Runner) Install(recipes []Recipe) error {
	for _, recipe := range recipes {
		info, ok := Lookup(recipe)
		if !ok {
			return errUnknownRecipe(recipe)
		}
		if info.File != "" {
			section, err := RenderSection(recipe, r.Version, r.Opts)
			if err != nil {
				return err
			}
			path := filepath.Join(r.Root, info.File)
			if err := MergeFile(path, section); err != nil {
				return err
			}
			fmt.Fprintf(r.Out, "  installed %s\n", info.File)
		}

		if info.SkillFile != "" {
			status, err := InstallSkillFile(r.Root, info, r.Opts)
			if err != nil {
				return err
			}
			if status == "installed" {
				fmt.Fprintf(r.Out, "  installed %s\n", info.SkillFile)
			}
		}
		if info.HookKind != HookKindNone {
			status, label, err := installHook(r.Root, info.HookKind)
			if err != nil {
				return err
			}
			if status == "installed" {
				fmt.Fprintf(r.Out, "  installed %s\n", label)
			}
		}
	}
	return nil
}

// Remove strips pine sections for the given recipes.
func (r *Runner) Remove(recipes []Recipe) error {
	for _, recipe := range recipes {
		info, ok := Lookup(recipe)
		if !ok {
			return errUnknownRecipe(recipe)
		}
		if info.File != "" {
			path := filepath.Join(r.Root, info.File)
			removed, err := RemoveSection(path)
			if err != nil {
				return err
			}
			if removed {
				fmt.Fprintf(r.Out, "  removed section from %s\n", info.File)
			} else {
				fmt.Fprintf(r.Out, "  no pine section in %s\n", info.File)
			}
		}
		if info.SkillFile != "" {
			if ok, err := RemoveSkillFile(r.Root, info); err != nil {
				return err
			} else if ok {
				fmt.Fprintf(r.Out, "  removed %s\n", info.SkillFile)
			}
		}
		if info.HookKind != HookKindNone {
			ok, label, err := removeHook(r.Root, info.HookKind)
			if err != nil {
				return err
			} else if ok {
				fmt.Fprintf(r.Out, "  removed learn-reminder hook from %s\n", label)
			}
		}
	}
	return nil
}

// Check reports install status for each recipe.
func (r *Runner) Check(recipes []Recipe) error {
	for _, recipe := range recipes {
		info, ok := Lookup(recipe)
		if !ok {
			return errUnknownRecipe(recipe)
		}
		if info.File != "" {
			section, err := RenderSection(recipe, r.Version, r.Opts)
			if err != nil {
				return err
			}
			body, _, _ := ExtractSection(section)
			path := filepath.Join(r.Root, info.File)
			status := CheckFile(path, body, recipe, r.Version)
			fmt.Fprintf(r.Out, "  %s (%s): %s\n", info.Label, info.File, status)
		} else {
			fmt.Fprintf(r.Out, "  %s: (hooks only)\n", info.Label)
		}
		if info.SkillFile != "" {
			fmt.Fprintf(r.Out, "  %s: %s\n", info.SkillFile, CheckSkillFile(r.Root, info, r.Opts))
		}
		if info.HookKind != HookKindNone {
			status, label := checkHook(r.Root, info.HookKind)
			fmt.Fprintf(r.Out, "  %s: %s\n", label, status)
		}
	}
	return nil
}

// Print writes rendered sections to Out.
func (r *Runner) Print(recipes []Recipe) error {
	printed := 0
	for _, recipe := range recipes {
		info, ok := Lookup(recipe)
		if !ok {
			return errUnknownRecipe(recipe)
		}
		if info.File == "" {
			continue
		}
		if printed > 0 {
			fmt.Fprintln(r.Out, "---")
		}
		section, err := RenderSection(recipe, r.Version, r.Opts)
		if err != nil {
			return err
		}
		fmt.Fprintln(r.Out, section)
		printed++
	}
	return nil
}

// List prints available recipes.
func (r *Runner) List() {
	for _, info := range Registry() {
		file := info.File
		if file == "" {
			file = "(hooks only)"
		}
		fmt.Fprintf(r.Out, "  %-8s %s — %s\n", info.Name, file, info.Description)
	}
}

// Wizard runs the interactive multi-select and returns chosen recipes.
func (r *Runner) Wizard(hasPine bool) ([]Recipe, error) {
	if !hasPine {
		ok, err := r.confirm(
			"No .pine directory found — run 'pine init' first.",
			"Continue anyway?",
			false,
		)
		if err != nil {
			if errors.Is(err, tui.ErrCancelled) {
				return nil, fmt.Errorf("setup cancelled")
			}
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("setup cancelled")
		}
	}

	recipes, err := r.pickRecipes(Registry())
	if err != nil {
		if errors.Is(err, tui.ErrCancelled) {
			return nil, fmt.Errorf("setup cancelled")
		}
		return nil, err
	}
	if len(recipes) == 0 {
		return nil, fmt.Errorf("no tools selected")
	}
	return recipes, nil
}

func (r *Runner) confirm(title, description string, defaultYes bool) (bool, error) {
	if r.Confirm != nil {
		return r.Confirm(title, description, defaultYes)
	}
	return tui.ConfirmIO(title, description, defaultYes, r.In, r.Out)
}

func (r *Runner) pickRecipes(infos []RecipeInfo) ([]Recipe, error) {
	if r.PickRecipes != nil {
		return r.PickRecipes(infos)
	}

	choices := make([]tui.Choice, 0, len(infos))
	for _, info := range infos {
		choices = append(choices, tui.Choice{
			Key:         string(info.Name),
			Label:       info.Label,
			Description: info.Description,
			Selected:    info.Name == RecipeAgents,
		})
	}

	var (
		keys []string
		err  error
	)
	if r.MultiSelect != nil {
		keys, err = r.MultiSelect("Pine agent setup — choose tools to integrate:", choices)
	} else {
		keys, err = tui.MultiSelectIO(
			"Pine agent setup — choose tools to integrate:",
			choices,
			r.In,
			r.Out,
		)
	}
	if err != nil {
		return nil, err
	}

	selected := make(map[Recipe]bool, len(keys))
	for _, key := range keys {
		selected[Recipe(key)] = true
	}
	var out []Recipe
	for _, recipe := range AllRecipes {
		if selected[recipe] {
			out = append(out, recipe)
		}
	}
	return out, nil
}

// RepoRoot resolves the repository root from a starting directory. When no
// .git directory is found, the starting directory is used as the project root.
func RepoRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	startDir := dir
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return startDir, nil
		}
		dir = parent
	}
}

// HasPine reports whether a .pine directory exists at or above start.
func HasPine(start string) bool {
	dir, err := filepath.Abs(start)
	if err != nil {
		return false
	}
	for {
		if fi, err := os.Stat(filepath.Join(dir, ".pine")); err == nil && fi.IsDir() {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}
