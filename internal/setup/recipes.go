package setup

// Recipe names a target agent instruction file.
type Recipe string

const (
	RecipeAgents Recipe = "agents"
	RecipeClaude Recipe = "claude"
	RecipeGemini Recipe = "gemini"
)

// AllRecipes is the default wizard selection order.
var AllRecipes = []Recipe{RecipeAgents, RecipeClaude, RecipeGemini}

// RecipeInfo describes one integration target.
type RecipeInfo struct {
	Name        Recipe
	File        string
	Label       string
	Description string
	Header      string
}

// Registry returns metadata for every built-in recipe.
func Registry() []RecipeInfo {
	return []RecipeInfo{
		{
			Name:        RecipeAgents,
			File:        "AGENTS.md",
			Label:       "AGENTS.md",
			Description: "Codex, Factory, and generic coding agents",
			Header:      "This project uses Pine for issue tracking.",
		},
		{
			Name:        RecipeClaude,
			File:        "CLAUDE.md",
			Label:       "CLAUDE.md",
			Description: "Claude Code",
			Header:      "Claude Code: read this before working in the repository.",
		},
		{
			Name:        RecipeGemini,
			File:        "GEMINI.md",
			Label:       "GEMINI.md",
			Description: "Gemini CLI",
			Header:      "Gemini CLI: read this before working in the repository.",
		},
	}
}

// Lookup returns recipe metadata or false.
func Lookup(name Recipe) (RecipeInfo, bool) {
	for _, r := range Registry() {
		if r.Name == name {
			return r, true
		}
	}
	return RecipeInfo{}, false
}
