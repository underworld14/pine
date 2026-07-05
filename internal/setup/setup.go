package setup

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Runner performs setup operations against a repository root.
type Runner struct {
	Root    string // repository root (parent of .pine)
	Version string
	Opts    RenderOptions
	Out     io.Writer
	In      io.Reader
}

// Install writes or updates sections for the given recipes.
func (r *Runner) Install(recipes []Recipe) error {
	for _, recipe := range recipes {
		info, ok := Lookup(recipe)
		if !ok {
			return errUnknownRecipe(recipe)
		}
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
	return nil
}

// Remove strips pine sections for the given recipes.
func (r *Runner) Remove(recipes []Recipe) error {
	for _, recipe := range recipes {
		info, ok := Lookup(recipe)
		if !ok {
			return errUnknownRecipe(recipe)
		}
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
	return nil
}

// Check reports install status for each recipe.
func (r *Runner) Check(recipes []Recipe) error {
	for _, recipe := range recipes {
		info, ok := Lookup(recipe)
		if !ok {
			return errUnknownRecipe(recipe)
		}
		section, err := RenderSection(recipe, r.Version, r.Opts)
		if err != nil {
			return err
		}
		body, _, _ := ExtractSection(section)
		path := filepath.Join(r.Root, info.File)
		status := CheckFile(path, body, recipe, r.Version)
		fmt.Fprintf(r.Out, "  %s (%s): %s\n", info.Label, info.File, status)
	}
	return nil
}

// Print writes rendered sections to Out.
func (r *Runner) Print(recipes []Recipe) error {
	for i, recipe := range recipes {
		if i > 0 {
			fmt.Fprintln(r.Out, "---")
		}
		section, err := RenderSection(recipe, r.Version, r.Opts)
		if err != nil {
			return err
		}
		fmt.Fprintln(r.Out, section)
	}
	return nil
}

// List prints available recipes.
func (r *Runner) List() {
	for _, info := range Registry() {
		fmt.Fprintf(r.Out, "  %-8s %s — %s\n", info.Name, info.File, info.Description)
	}
}

// Wizard runs the interactive multi-select and returns chosen recipes.
func (r *Runner) Wizard(hasPine bool) ([]Recipe, error) {
	if !hasPine {
		fmt.Fprintln(r.Out, "warning: no .pine directory found — run 'pine init' first.")
		fmt.Fprint(r.Out, "Continue anyway? [y/N] ")
		if !readYes(r.In) {
			return nil, fmt.Errorf("setup cancelled")
		}
	}

	selected := map[Recipe]bool{
		RecipeAgents: true,
		RecipeClaude: false,
		RecipeGemini: false,
	}

	in := r.In
	if in == nil {
		in = os.Stdin
	}
	reader := bufio.NewReader(in)

	for {
		fmt.Fprintln(r.Out)
		fmt.Fprintln(r.Out, "Pine agent setup — choose tools to integrate:")
		fmt.Fprintln(r.Out)
		infos := Registry()
		for i, info := range infos {
			mark := "[ ]"
			if selected[info.Name] {
				mark = "[x]"
			}
			fmt.Fprintf(r.Out, "  %s %d. %-12s (%s)\n", mark, i+1, info.Label, info.Description)
		}
		fmt.Fprintln(r.Out, "\nToggle: 1,2,3  |  Enter to confirm  |  q to cancel")
		fmt.Fprint(r.Out, "> ")

		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if strings.EqualFold(line, "q") || strings.EqualFold(line, "quit") {
			return nil, fmt.Errorf("setup cancelled")
		}
		for _, part := range strings.FieldsFunc(line, func(r rune) bool {
			return r == ',' || r == ' ' || r == '\t'
		}) {
			switch part {
			case "1":
				selected[RecipeAgents] = !selected[RecipeAgents]
			case "2":
				selected[RecipeClaude] = !selected[RecipeClaude]
			case "3":
				selected[RecipeGemini] = !selected[RecipeGemini]
			}
		}
	}

	var out []Recipe
	for _, recipe := range AllRecipes {
		if selected[recipe] {
			out = append(out, recipe)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no tools selected")
	}
	return out, nil
}

func readYes(r io.Reader) bool {
	if r == nil {
		r = os.Stdin
	}
	line, err := bufio.NewReader(r).ReadString('\n')
	if err != nil {
		return false
	}
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "y" || line == "yes"
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
