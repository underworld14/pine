package contextgen

import (
	"strings"
	"text/template"

	"github.com/underworld14/pine/internal/gitx"
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
	"## Acceptance Criteria\n" +
	"- The behavior described under \"Expected\" occurs when following \"Steps\".\n" +
	"- No regressions in the related files above.\n\n" +
	"## When done\n" +
	"- Edit `.pine/tickets/{{.ID}}.md` and set `status: testing` (then `done` once verified).\n" +
	"- Summarize your changes in a `# Fix Notes` section in the ticket.\n"

// PromptData is the template context for a fix prompt.
type PromptData struct {
	ID           string
	Title        string
	Status       string
	Priority     string
	Labels       []string
	Body         string
	RelatedFiles []string
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
	if strings.TrimSpace(tmplText) == "" {
		tmplText = DefaultFixTemplate
	}
	funcs := template.FuncMap{"join": strings.Join}
	tmpl, err := template.New("fix").Funcs(funcs).Parse(tmplText)
	if err != nil {
		tmpl = template.Must(template.New("fix").Funcs(funcs).Parse(DefaultFixTemplate))
	}
	var b strings.Builder
	if err := tmpl.Execute(&b, data); err != nil {
		// A user template that references missing fields: fall back cleanly.
		b.Reset()
		fallback := template.Must(template.New("fix").Funcs(funcs).Parse(DefaultFixTemplate))
		_ = fallback.Execute(&b, data)
	}
	return b.String(), nil
}
