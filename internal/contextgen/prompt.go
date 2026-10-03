package contextgen

import (
	"fmt"
	"regexp"
	"strings"
	"text/template"

	"github.com/underworld14/pine/internal/gitx"
	"github.com/underworld14/pine/internal/learning"
	"github.com/underworld14/pine/internal/store"
	"github.com/underworld14/pine/internal/ticket"
)

// skillsBlockTemplate renders the per-ticket required-skills section. It is
// kept textually identical to FormatSkillsBlock so default templates and the
// stale-template append path never drift.
const skillsBlockTemplate = "{{if .Skills}}## Required Skills\n" +
	"Load these skills before starting work — they are required, not optional:\n" +
	"{{range .Skills}}- `{{.}}`\n{{end}}" +
	"If one is not installed, try to find and install it first (skills.sh, GitHub, or your harness's skill finder such as /find-skills). If it is still unavailable, note that in the ticket's `# Notes` section and proceed carefully.\n\n{{end}}"

// DefaultFixTemplate is the built-in prompt template written by pine init and
// used when no .pine/prompts/fix.md exists.
const DefaultFixTemplate = "# Fix Request: {{.ID}} — {{.Title}}\n\n" +
	"## Issue\n" +
	"Status: {{.Status}} · Priority: {{.Priority}}{{if .Labels}} · Labels: {{join .Labels \", \"}}{{end}}\n\n" +
	skillsBlockTemplate +
	"{{.Body}}\n\n" +
	"## Related Files\n{{range .RelatedFiles}}- {{.}}\n{{else}}(none listed)\n{{end}}\n" +
	"{{if .Learnings}}## Relevant Learnings\n{{range .Learnings}}- **{{.ID}}** ({{.Scope}}{{if .Tags}}, tags: {{join .Tags \", \"}}{{end}}): {{.Body}}\n{{end}}\n{{end}}" +
	"## Acceptance Criteria\n" +
	"- The behavior described under \"Expected\" occurs when following \"Steps\".\n" +
	"- No regressions in the related files above.\n\n" +
	"## When done\n" +
	"- Edit `.pine/tickets/{{.ID}}.md` and set `status: testing` (then `done` once verified).\n" +
	"- Summarize your changes in a `# Fix Notes` section in the ticket.\n" +
	"- If you discovered a non-obvious project gotcha, run `pine learn \"...\"` before ending.\n"

// DefaultImplementTemplate is the feature-ticket counterpart of
// DefaultFixTemplate: acceptance criteria and the implementation plan live in
// the ticket body (see the feature template), so this prompt keeps them up
// front and asks for a proof command instead of bug reproduction.
const DefaultImplementTemplate = "# Implement: {{.ID}} — {{.Title}}\n\n" +
	"## Task\n" +
	"Status: {{.Status}} · Priority: {{.Priority}}{{if .Labels}} · Labels: {{join .Labels \", \"}}{{end}}\n\n" +
	skillsBlockTemplate +
	"{{.Body}}\n\n" +
	"## Related Files\n{{range .RelatedFiles}}- {{.}}\n{{else}}(none listed — list candidates under a \"# Related Files\" heading in the ticket)\n{{end}}\n" +
	"{{if .Learnings}}## Relevant Learnings\n{{range .Learnings}}- **{{.ID}}** ({{.Scope}}{{if .Tags}}, tags: {{join .Tags \", \"}}{{end}}): {{.Body}}\n{{end}}\n{{end}}" +
	"## When done\n" +
	"- Record a proof command in the ticket frontmatter, e.g. `verify: grep -rn \"NewX(\" internal/ | grep -v _test.go` or a smoke command — `pine close {{.ID}} --evidence` runs it and refuses to close when it fails.\n" +
	"- Edit `.pine/tickets/{{.ID}}.md` and set `status: testing` (then `done` once verified).\n" +
	"- Summarize your changes in a `# Implementation Notes` section in the ticket.\n" +
	"- If you discovered a non-obvious project gotcha, run `pine learn \"...\"` before ending.\n"

// PromptData is the template context for a fix prompt.
type PromptData struct {
	ID           string
	Title        string
	Status       string
	Priority     string
	Labels       []string
	Skills       []string
	Body         string
	RelatedFiles []string
	Learnings    []LearningRef
	Branch       string
	Dirty        int
}

// Prompt renders a fix request for one ticket. tmplText is the user's fix.md (or
// empty to use the default). A malformed user template falls back to the default
// rather than failing.
func Prompt(s *store.Store, git gitx.Status, id, tmplText string) (string, error) {
	t, err := s.Get(id)
	if err != nil {
		return "", err
	}
	data := PromptData{
		ID:           t.ID,
		Title:        t.Title,
		Status:       t.Status,
		Priority:     t.Priority,
		Labels:       t.Labels,
		Skills:       t.Skills,
		Body:         strings.TrimSpace(t.Body),
		RelatedFiles: ticket.RelatedFiles(t.Body),
		Branch:       git.Branch,
		Dirty:        len(git.Changes),
	}
	selected, more := SelectLearnings(s, LearningSelectOpts{
		TicketID:     t.ID,
		TicketTitle:  t.Title,
		TicketLabels: t.Labels,
		Limit:        10,
	})
	data.Learnings = LearningRefs(selected)
	if strings.TrimSpace(tmplText) == "" {
		tmplText = DefaultFixTemplate
	}
	tmplUsesLearnings := templateReferences(tmplText, ".Learnings")
	tmplUsesSkills := templateReferences(tmplText, ".Skills")
	funcs := template.FuncMap{"join": strings.Join}
	tmpl, err := template.New("fix").Funcs(funcs).Parse(tmplText)
	if err != nil {
		tmpl = template.Must(template.New("fix").Funcs(funcs).Parse(DefaultFixTemplate))
		tmplUsesLearnings = templateReferences(DefaultFixTemplate, ".Learnings")
		tmplUsesSkills = templateReferences(DefaultFixTemplate, ".Skills")
	}
	var b strings.Builder
	if err := tmpl.Execute(&b, data); err != nil {
		// A user template that references missing fields: fall back cleanly.
		b.Reset()
		fallback := template.Must(template.New("fix").Funcs(funcs).Parse(DefaultFixTemplate))
		_ = fallback.Execute(&b, data)
		tmplUsesLearnings = templateReferences(DefaultFixTemplate, ".Learnings")
		tmplUsesSkills = templateReferences(DefaultFixTemplate, ".Skills")
	}
	out := b.String()
	if mem := FormatMemoryBlock(s, nil, t.Labels, 2); mem != "" {
		out = mem + out
	}
	if links := FormatTicketLinksBlock(s, t); links != "" {
		out = links + out
	}
	if !tmplUsesLearnings {
		out = appendLearningsBlock(out, selected, more)
	}
	if !tmplUsesSkills {
		out = appendSkillsBlock(out, t.Skills)
	}
	return out, nil
}

// templateCommentRe matches a Go-template comment ({{/* ... */}}, optionally
// with trim markers), so a comment merely mentioning ".Learnings" doesn't
// false-positive templateReferencesLearnings below.
var templateCommentRe = regexp.MustCompile(`(?s)\{\{-?\s*/\*.*?\*/\s*-?\}\}`)

// templateReferences reports whether the template already renders the given
// field (e.g. ".Learnings", ".Skills") so we must not inject a second block.
// Comments are stripped first since they're never rendered.
func templateReferences(tmplText, field string) bool {
	stripped := templateCommentRe.ReplaceAllString(tmplText, "")
	return strings.Contains(stripped, field)
}

// appendLearningsBlock appends the learnings section for stale fix.md templates
// that predate .Learnings support. Appends at the end so ticket body headings
// like "## Relevant Learnings" / "## Acceptance Criteria" cannot collide.
func appendLearningsBlock(out string, selected []*learning.Learning, more int) string {
	if len(selected) == 0 {
		return out
	}
	block := FormatLearningsBlock(selected, more)
	if block == "" {
		return out
	}
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out + "\n" + block
}

// FormatSkillsBlock renders the required-skills section for a ticket's
// skills: frontmatter list. Mirrors skillsBlockTemplate verbatim.
func FormatSkillsBlock(skills []string) string {
	var b strings.Builder
	b.WriteString("## Required Skills\n")
	b.WriteString("Load these skills before starting work — they are required, not optional:\n")
	for _, s := range skills {
		fmt.Fprintf(&b, "- `%s`\n", s)
	}
	b.WriteString("If one is not installed, try to find and install it first (skills.sh, GitHub, or your harness's skill finder such as /find-skills). If it is still unavailable, note that in the ticket's `# Notes` section and proceed carefully.\n")
	return b.String()
}

// appendSkillsBlock appends the required-skills section for stale fix.md /
// implement.md templates that predate .Skills support, so tickets that declare
// skills still reach the agent in repos initialized before the field existed.
func appendSkillsBlock(out string, skills []string) string {
	if len(skills) == 0 {
		return out
	}
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out + "\n" + FormatSkillsBlock(skills)
}
