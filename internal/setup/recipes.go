package setup

// Recipe names a target agent integration.
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

// RecipeInfo describes one agent integration bundle.
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
	// SectionRecipe, when set, is the recipe id used in the AGENTS.md/CLAUDE.md
	// section marker. Cursor shares AGENTS.md with Codex, so it renders and
	// checks as RecipeAgents while still owning only the Cursor hook on remove.
	SectionRecipe Recipe
}

// OwnsInstructionReports whether this recipe owns the instruction file / skill
// for install-remove. Shared consumers (Cursor → AGENTS.md) do not.
func (info RecipeInfo) OwnsInstruction() bool {
	return info.SectionRecipe == "" || info.SectionRecipe == info.Name
}

// InstructionRecipe is the recipe id used when rendering/checking the
// instruction file section.
func (info RecipeInfo) InstructionRecipe() Recipe {
	if info.SectionRecipe != "" {
		return info.SectionRecipe
	}
	return info.Name
}

// Registry returns metadata for every built-in recipe.
func Registry() []RecipeInfo {
	return []RecipeInfo{
		{
			Name:        RecipeAgents,
			File:        "AGENTS.md",
			Label:       "Codex",
			Description: "AGENTS.md + skill + Stop hook",
			Header:      "This project uses Pine for issue tracking.",
			SkillFile:   ".agents/skills/pine/SKILL.md",
			HookKind:    HookKindCodex,
		},
		{
			Name:        RecipeClaude,
			File:        "CLAUDE.md",
			Label:       "Claude Code",
			Description: "CLAUDE.md + skill + Stop hook",
			Header:      "Claude Code: read this before working in the repository.",
			SkillFile:   ".claude/skills/pine/SKILL.md",
			HookKind:    HookKindClaude,
		},
		{
			Name:        RecipeGemini,
			File:        "GEMINI.md",
			Label:       "Gemini CLI",
			Description: "GEMINI.md + shared skill",
			Header:      "Gemini CLI: read this before working in the repository.",
			// Gemini has no first-class skills dir; reuse the AGENTS skill path so
			// GEMINI.md's pointer to the full workflow resolves after setup.
			SkillFile: ".agents/skills/pine/SKILL.md",
		},
		{
			Name:          RecipeCursor,
			File:          "AGENTS.md",
			Label:         "Cursor",
			Description:   "AGENTS.md + skill + sessionStart hook",
			Header:        "This project uses Pine for issue tracking.",
			SkillFile:     ".agents/skills/pine/SKILL.md",
			HookKind:      HookKindCursor,
			SectionRecipe: RecipeAgents,
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
