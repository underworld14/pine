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
	// SkillFile is the repo-relative path where this agent looks for a skill
	// file, or "" when the agent has no skill convention. When set, `pine setup`
	// installs the Pine SKILL.md there in addition to merging the markdown block.
	SkillFile string
	// InstallsHook is true for agents that support an executable session hook
	// (currently only Claude Code, via .claude/settings.json).
	InstallsHook bool
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
			SkillFile:   ".agents/skills/pine/SKILL.md",
		},
		{
			Name:         RecipeClaude,
			File:         "CLAUDE.md",
			Label:        "CLAUDE.md",
			Description:  "Claude Code",
			Header:       "Claude Code: read this before working in the repository.",
			SkillFile:    ".claude/skills/pine/SKILL.md",
			InstallsHook: true,
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
