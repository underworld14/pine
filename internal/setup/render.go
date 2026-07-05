package setup

import (
	_ "embed"
	"strings"
)

//go:embed templates/core.md
var coreTemplate string

// RenderOptions supplies dynamic fragments when rendering agent instructions.
type RenderOptions struct {
	// BoardColumns is a comma-separated list of kanban statuses, e.g. "todo, doing, done".
	// When empty, the board-columns line is omitted.
	BoardColumns string
}

// RenderSection builds the marked pine section for one recipe.
func RenderSection(recipe Recipe, version string, opts RenderOptions) (string, error) {
	info, ok := Lookup(recipe)
	if !ok {
		return "", errUnknownRecipe(recipe)
	}

	core := strings.ReplaceAll(coreTemplate, "{{BOARD_COLUMNS_LINE}}", boardColumnsLine(opts.BoardColumns))
	body := strings.TrimSpace(info.Header) + "\n\n" + strings.TrimSpace(core)
	hash := ContentHash(body)
	marker := BeginMarker(recipe, version, hash)
	return marker + "\n" + body + "\n<!-- pine:end -->", nil
}

func boardColumnsLine(columns string) string {
	columns = strings.TrimSpace(columns)
	if columns == "" {
		return "."
	}
	return " (board columns: " + columns + ")."
}

type recipeError struct {
	recipe Recipe
}

func errUnknownRecipe(r Recipe) error { return recipeError{recipe: r} }

func (e recipeError) Error() string { return "unknown setup recipe: " + string(e.recipe) }
