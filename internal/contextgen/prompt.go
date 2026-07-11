package contextgen

import (
	"regexp"
	"strings"
	"text/template"

	"github.com/underworld14/pine/internal/gitx"
	"github.com/underworld14/pine/internal/learning"
	"github.com/underworld14/pine/internal/store"
	"github.com/underworld14/pine/internal/ticket"
)

// DefaultFixTemplate is the built-in prompt template written by pine init and
// used when no .pine/prompts/fix.md exists.
const DefaultFixTemplate = "# Fix Request: {{.ID}} — {{.Title}}\n\n" +
	"## Issue\n" +
	"Status: {{.Status}} · Priority: {{.Priority}}{{if .Labels}} · Labels: {{join .Labels \", \"}}{{end}}\n\n" +
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

// PromptData is the template context for a fix prompt.
type PromptData struct {
	ID           string
	Title        string
	Status       string
	Priority     string
	Labels       []string
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
	tmplUsesLearnings := templateReferencesLearnings(tmplText)
	funcs := template.FuncMap{"join": strings.Join}
	tmpl, err := template.New("fix").Funcs(funcs).Parse(tmplText)
	if err != nil {
		tmpl = template.Must(template.New("fix").Funcs(funcs).Parse(DefaultFixTemplate))
		tmplUsesLearnings = templateReferencesLearnings(DefaultFixTemplate)
	}
	var b strings.Builder
	if err := tmpl.Execute(&b, data); err != nil {
		// A user template that references missing fields: fall back cleanly.
		b.Reset()
		fallback := template.Must(template.New("fix").Funcs(funcs).Parse(DefaultFixTemplate))
		_ = fallback.Execute(&b, data)
		tmplUsesLearnings = templateReferencesLearnings(DefaultFixTemplate)
	}
	out := b.String()
	if !tmplUsesLearnings {
		out = appendLearningsBlock(out, selected, more)
	}
	return out, nil
}

// templateCommentRe matches a Go-template comment ({{/* ... */}}, optionally
// with trim markers), so a comment merely mentioning ".Learnings" doesn't
// false-positive templateReferencesLearnings below.
var templateCommentRe = regexp.MustCompile(`(?s)\{\{-?\s*/\*.*?\*/\s*-?\}\}`)

// templateReferencesLearnings reports whether the template already renders
// .Learnings (so we must not inject a second block). Comments are stripped
// first since they're never rendered.
func templateReferencesLearnings(tmplText string) bool {
	stripped := templateCommentRe.ReplaceAllString(tmplText, "")
	return strings.Contains(stripped, ".Learnings")
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
