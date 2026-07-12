package setup

// Recipe names a target agent instruction file.
type Recipe string

const (
	RecipeAgents Recipe = "agents"
	RecipeClaude Recipe = "claude"
	RecipeGemini Recipe = "gemini"
	RecipeCursor Recipe = "cursor"
)

// HookKind identifies which agent hook config Pine manages for a recipe.
type HookKind string

const (
	HookKindNone   HookKind = ""
	HookKindClaude HookKind = "claude"
	HookKindCodex  HookKind = "codex"
	HookKindCursor HookKind = "cursor"
)

// AllRecipes is the default wizard selection order.
var AllRecipes = []Recipe{RecipeAgents, RecipeClaude, RecipeGemini, RecipeCursor}

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
	// HookKind is the session-hook target for this recipe, or HookKindNone.
	HookKind HookKind
}

// Registry returns metadata for every built-in recipe.
func Registry() []RecipeInfo {
	return []RecipeInfo{
		{
			Name:        RecipeAgents,
			File:        "AGENTS.md",
			Label:       "AGENTS.md",
			Description: "Codex, Cursor, Factory, and generic coding agents",
			Header:      "This project uses Pine for issue tracking.",
			SkillFile:   ".agents/skills/pine/SKILL.md",
			HookKind:    HookKindCodex,
		},
		{
			Name:        RecipeClaude,
			File:        "CLAUDE.md",
			Label:       "CLAUDE.md",
			Description: "Claude Code",
			Header:      "Claude Code: read this before working in the repository.",
			SkillFile:   ".claude/skills/pine/SKILL.md",
			HookKind:    HookKindClaude,
		},
		{
			Name:        RecipeGemini,
			File:        "GEMINI.md",
			Label:       "GEMINI.md",
			Description: "Gemini CLI",
			Header:      "Gemini CLI: read this before working in the repository.",
			// Gemini has no first-class skills dir; reuse the AGENTS skill path so
			// GEMINI.md's pointer to the full workflow resolves after setup.
			SkillFile: ".agents/skills/pine/SKILL.md",
		},
		{
			Name:        RecipeCursor,
			File:        "",
			Label:       "Cursor hooks",
			Description: ".cursor/hooks.json (reuses AGENTS.md)",
			HookKind:    HookKindCursor,
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
