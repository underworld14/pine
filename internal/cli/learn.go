package cli

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/underworld14/pine/internal/learning"
	"github.com/underworld14/pine/internal/search"
	"github.com/underworld14/pine/internal/store"
	"github.com/underworld14/pine/internal/view"
)

// newLearnCmd is registered from root.go.
func newLearnCmd() *cobra.Command {
	var (
		scope, ticket, component, source, tagsCSV, supersedes, citesCSV, textFlag string
		asJSON                                                                    bool
	)
	cmd := &cobra.Command{
		Use:   "learn [text]",
		Short: "Capture a persistent learning, or list/search/show learnings",
		Long: `Capture a persistent, cross-session, cross-agent insight under .pine/learnings/.

Scope the insight with --scope global (default, visible everywhere) or
--scope ticket --ticket <ID> (visible only for that ticket). When a new
insight replaces an older one, pass --supersedes <LRN-id> instead of leaving
both active. When the insight depends on a specific file, pass
--cites path/to/file so 'pine doctor' and list/search/context can flag it as
stale once that file is deleted.

If the insight text is exactly "list", "search", or "show", pass it via
--text instead of as a positional argument — those three words are also
subcommand names and would otherwise be routed there instead of captured:

  pine learn --text "list"

Subcommands: 'pine learn list', 'pine learn search <query>', 'pine learn show <id>'.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			text := textFlag
			switch {
			case text != "" && len(args) > 0:
				return fmt.Errorf("provide the insight via --text or as a positional argument, not both")
			case text == "" && len(args) == 0:
				return fmt.Errorf("learning text is required (or use: pine learn list | search | show)")
			case text == "":
				text = args[0]
			}
			s, err := openStore()
			if err != nil {
				return err
			}
			tags := splitCSV(tagsCSV)
			ticketID := ""
			if ticket != "" {
				ticketID = normalizeID(ticket)
			}
			sup := ""
			if supersedes != "" {
				sup = normalizeID(supersedes)
			}
			l, err := s.CreateLearning(store.CreateLearningReq{
				Text:        text,
				Scope:       scope,
				Tags:        tags,
				Ticket:      ticketID,
				Component:   component,
				SourceAgent: source,
				Supersedes:  sup,
				Cites:       splitCSV(citesCSV),
			})
			if err != nil {
				return err
			}
			if asJSON {
				dto := view.BuildLearning(l)
				dto.Stale = s.CitationStaleIDs([]*learning.Learning{l})[l.ID]
				return writeJSON(cmd.OutOrStdout(), dto)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Captured %s\n", l.ID)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&scope, "scope", learning.ScopeGlobal, "scope: global | ticket | component")
	f.StringVar(&tagsCSV, "tags", "", "comma-separated tags")
	f.StringVar(&ticket, "ticket", "", "ticket ID (required when --scope ticket)")
	f.StringVar(&component, "component", "", "component path/name (required when --scope component)")
	f.StringVar(&source, "source", learning.SourceManual, "source agent: claude-code|codex|cursor|gemini|manual")
	f.StringVar(&supersedes, "supersedes", "", "learning ID this insight replaces")
	f.StringVar(&citesCSV, "cites", "", "comma-separated repo-relative file paths this insight depends on")
	f.StringVar(&textFlag, "text", "", `insight text, as an alternative to the positional argument (required if the insight is exactly "list", "search", or "show")`)
	f.BoolVar(&asJSON, "json", false, "output JSON")

	cmd.AddCommand(newLearnListCmd(), newLearnSearchCmd(), newLearnShowCmd(),
		newLearnSupersedeCmd(), newLearnRmCmd())
	return cmd
}

// newLearnSupersedeCmd captures a new learning that replaces an existing one,
// inheriting the old learning's scope/ticket/component/tags unless overridden.
func newLearnSupersedeCmd() *cobra.Command {
	var (
		scope, ticket, component, source, tagsCSV, citesCSV, textFlag string
		asJSON                                                        bool
	)
	cmd := &cobra.Command{
		Use:   "supersede <old-id> [new text]",
		Short: "Replace an existing learning with a new insight",
		Long: `Capture a new learning that supersedes <old-id>. The new learning inherits
the old one's scope, ticket, component, and tags unless you override them with
the corresponding flags. The old learning is retained on disk (for history) but
hidden from list/search/context once superseded.

Provide the new insight text as positional args or via --text.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			oldID := normalizeID(args[0])
			text := strings.TrimSpace(textFlag)
			if text == "" && len(args) > 1 {
				text = strings.TrimSpace(strings.Join(args[1:], " "))
			}
			if text == "" {
				return fmt.Errorf("new insight text is required (positional or --text)")
			}
			s, err := openStore()
			if err != nil {
				return err
			}
			old, err := s.GetLearning(oldID)
			if err != nil {
				return err
			}
			// Inherit from the old learning unless the flag was explicitly set.
			f := cmd.Flags()
			if !f.Changed("scope") {
				scope = old.Scope
			}
			if !f.Changed("ticket") {
				ticket = old.Ticket
			}
			if !f.Changed("component") {
				component = old.Component
			}
			tags := splitCSV(tagsCSV)
			if !f.Changed("tags") {
				tags = append([]string(nil), old.Tags...)
			}
			ticketID := ""
			if ticket != "" {
				ticketID = normalizeID(ticket)
			}
			l, err := s.CreateLearning(store.CreateLearningReq{
				Text:        text,
				Scope:       scope,
				Tags:        tags,
				Ticket:      ticketID,
				Component:   component,
				SourceAgent: source,
				Supersedes:  oldID,
				Cites:       splitCSV(citesCSV),
			})
			if err != nil {
				return err
			}
			if asJSON {
				dto := view.BuildLearning(l)
				dto.SupersededBy = ""
				return writeJSON(cmd.OutOrStdout(), dto)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Captured %s (supersedes %s)\n", l.ID, oldID)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&scope, "scope", "", "override scope: global | ticket | component")
	f.StringVar(&ticket, "ticket", "", "override ticket ID")
	f.StringVar(&component, "component", "", "override component path/name")
	f.StringVar(&source, "source", learning.SourceManual, "source agent: claude-code|codex|cursor|gemini|manual")
	f.StringVar(&tagsCSV, "tags", "", "override tags (comma-separated)")
	f.StringVar(&citesCSV, "cites", "", "comma-separated repo-relative file paths this insight depends on")
	f.StringVar(&textFlag, "text", "", "insight text, as an alternative to positional args")
	f.BoolVar(&asJSON, "json", false, "output JSON")
	return cmd
}

// newLearnRmCmd permanently deletes a learning file.
func newLearnRmCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "rm <id>",
		Short: "Delete a learning permanently",
		Long: `Permanently remove a learning file from .pine/learnings/. Unlike supersede,
this leaves no history. Prompts for confirmation unless --yes is given.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := normalizeID(args[0])
			s, err := openStore()
			if err != nil {
				return err
			}
			l, err := s.GetLearning(id)
			if err != nil {
				return err
			}
			if !yes {
				fmt.Fprintf(cmd.OutOrStdout(), "Delete %s: %q? [y/N] ", id, truncate(strings.TrimSpace(l.Body), 50))
				if !readYesLearn(cmd.InOrStdin()) {
					fmt.Fprintln(cmd.OutOrStdout(), "Cancelled.")
					return nil
				}
			}
			if err := s.DeleteLearning(id); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted %s\n", id)
			return nil
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "skip the confirmation prompt")
	return cmd
}

// readYesLearn reads a single line and reports whether it is an affirmative.
func readYesLearn(r io.Reader) bool {
	var line string
	_, _ = fmt.Fscanln(r, &line)
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "y" || line == "yes"
}

func newLearnListCmd() *cobra.Command {
	var (
		lf     *learningFilterFlags
		asJSON bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List learnings (filterable; hides superseded and citation-stale by default)",
		Long: `List captured learnings, newest first.

By default, entries superseded by a newer learning and entries with a
missing --cites path are hidden; pass --include-superseded / --include-stale
to audit them. Narrow the list with --scope, --tags (AND), and --ticket.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			items := s.ListLearnings(lf.build())
			if asJSON {
				all := s.AllLearnings()
				_, rev := learning.BuildEdges(all)
				created := learning.CreatedMap(all)
				stale := s.CitationStaleIDs(items)
				out := make([]view.Learning, 0, len(items))
				for _, l := range items {
					dto := view.BuildLearning(l)
					dto.Stale = stale[l.ID]
					dto.SupersededBy = learning.SupersededBy(rev, created, l.ID)
					out = append(out, dto)
				}
				return writeJSON(cmd.OutOrStdout(), out)
			}
			renderLearningTable(cmd.OutOrStdout(), items, s)
			return nil
		},
	}
	lf = registerLearningFilterFlags(cmd)
	cmd.Flags().BoolVar(&asJSON, "json", false, "output JSON")
	return cmd
}

func newLearnSearchCmd() *cobra.Command {
	var (
		lf     *learningFilterFlags
		asJSON bool
	)
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Full-text search learnings (hides superseded and citation-stale by default)",
		Long: `Full-text search over captured learnings, ranked by relevance.

By default, entries superseded by a newer learning and entries with a
missing --cites path are excluded from the search corpus; pass
--include-superseded / --include-stale to include them. Narrow the corpus
first with --scope, --tags (AND), and --ticket.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			filter := lf.build()
			corpus := s.ListLearnings(filter)
			idx, err := search.New()
			if err != nil {
				return err
			}
			defer idx.Close()
			for _, l := range corpus {
				idx.Upsert(search.DocFromLearning(l))
			}
			hits := idx.Search(args[0], search.Filter{
				Kind:  search.KindLearning,
				Scope: filter.Scope,
				Tags:  filter.Tags,
			}, 20)

			byID := map[string]*learning.Learning{}
			for _, l := range corpus {
				byID[l.ID] = l
			}
			stale := s.CitationStaleIDs(corpus)

			if asJSON {
				all := s.AllLearnings()
				_, rev := learning.BuildEdges(all)
				created := learning.CreatedMap(all)
				out := make([]view.LearningHit, 0, len(hits))
				for _, h := range hits {
					l := byID[h.ID]
					if l == nil {
						continue
					}
					dto := view.BuildLearning(l)
					dto.Stale = stale[l.ID]
					dto.SupersededBy = learning.SupersededBy(rev, created, l.ID)
					out = append(out, view.LearningHit{Learning: dto, Score: h.Score})
				}
				return writeJSON(cmd.OutOrStdout(), out)
			}
			if len(hits) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No learnings matched.")
				return nil
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tSCOPE\tTARGET\tTAGS\tSCORE\tSTATUS\tBODY")
			for _, h := range hits {
				l := byID[h.ID]
				if l == nil {
					continue
				}
				status := ""
				if stale[l.ID] {
					status = "stale"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%.2f\t%s\t%s\n",
					l.ID, l.Scope, scopeTarget(l), strings.Join(l.Tags, ","), h.Score, status, truncate(strings.TrimSpace(l.Body), 60))
			}
			return w.Flush()
		},
	}
	lf = registerLearningFilterFlags(cmd)
	cmd.Flags().BoolVar(&asJSON, "json", false, "output JSON")
	return cmd
}

func newLearnShowCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show one learning, including supersedes relationships and cite status",
		Long: `Show one learning's full detail: scope, tags, ticket, both directions of
its supersede chain, per-citation existence (✓/✗), and the insight body.
A learning whose frontmatter could not be parsed is shown read-only with a
degraded note instead of being hidden.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			id := normalizeID(args[0])
			l, err := s.GetLearning(id)
			if err != nil {
				return err
			}
			all := s.AllLearnings()
			_, rev := learning.BuildEdges(all)
			created := learning.CreatedMap(all)
			by := learning.SupersededBy(rev, created, l.ID)
			dto := view.BuildLearning(l)
			dto.SupersededBy = by
			dto.CiteStatus = citeStatuses(s, l.Cites)
			dto.Stale = s.CitationStaleIDs([]*learning.Learning{l})[l.ID]
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), dto)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "%s  scope=%s  source=%s\n", l.ID, l.Scope, l.SourceAgent)
			if l.Degraded {
				fmt.Fprintln(w, "note: this learning is degraded (frontmatter could not be parsed); shown read-only")
			}
			if len(l.Tags) > 0 {
				fmt.Fprintf(w, "tags: %s\n", strings.Join(l.Tags, ", "))
			}
			if l.Ticket != "" {
				fmt.Fprintf(w, "ticket: %s\n", l.Ticket)
			}
			if l.Component != "" {
				fmt.Fprintf(w, "component: %s\n", l.Component)
			}
			if l.Supersedes != "" {
				fmt.Fprintf(w, "supersedes: %s\n", l.Supersedes)
			}
			if by != "" {
				fmt.Fprintf(w, "superseded by: %s\n", by)
			}
			if len(l.Cites) > 0 {
				fmt.Fprintln(w, "cites:")
				for _, cs := range dto.CiteStatus {
					mark := "✓"
					if !cs.Exists {
						mark = "✗"
					}
					fmt.Fprintf(w, "  %s %s\n", mark, cs.Path)
				}
			}
			if dto.Created != "" {
				fmt.Fprintf(w, "created: %s\n", dto.Created)
			}
			fmt.Fprintln(w)
			fmt.Fprintln(w, strings.TrimSpace(l.Body))
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output JSON")
	return cmd
}

// learningFilterFlags is the filter flag set shared by `learn list` and
// `learn search`.
type learningFilterFlags struct {
	scope, tagsCSV, ticket, component string
	includeSuper, includeStale        bool
}

func registerLearningFilterFlags(cmd *cobra.Command) *learningFilterFlags {
	lf := &learningFilterFlags{}
	f := cmd.Flags()
	f.StringVar(&lf.scope, "scope", "", "filter by scope")
	f.StringVar(&lf.tagsCSV, "tags", "", "filter by tags (comma-separated, AND)")
	f.StringVar(&lf.ticket, "ticket", "", "filter by ticket ID")
	f.StringVar(&lf.component, "component", "", "filter by component path/name")
	f.BoolVar(&lf.includeSuper, "include-superseded", false, "include learnings replaced by a newer one")
	f.BoolVar(&lf.includeStale, "include-stale", false, "include learnings with a missing cited path")
	return lf
}

func (lf *learningFilterFlags) build() store.LearningFilter {
	ticket := lf.ticket
	if ticket != "" {
		ticket = normalizeID(ticket)
	}
	return store.LearningFilter{
		Scope:             lf.scope,
		Tags:              splitCSV(lf.tagsCSV),
		Ticket:            ticket,
		Component:         lf.component,
		IncludeSuperseded: lf.includeSuper,
		IncludeStale:      lf.includeStale,
	}
}

func splitCSV(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func citeStatuses(s *store.Store, cites []string) []view.CiteStatus {
	if len(cites) == 0 {
		return nil
	}
	out := make([]view.CiteStatus, 0, len(cites))
	for _, p := range cites {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, view.CiteStatus{Path: p, Exists: s.CiteExists(p)})
	}
	return out
}

func renderLearningTable(w io.Writer, items []*learning.Learning, s *store.Store) {
	if len(items) == 0 {
		fmt.Fprintln(w, "No learnings.")
		return
	}
	stale := s.CitationStaleIDs(items)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSCOPE\tTARGET\tTAGS\tSOURCE\tSTATUS\tBODY")
	for _, l := range items {
		status := ""
		if stale[l.ID] {
			status = "stale"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			l.ID, l.Scope, scopeTarget(l), strings.Join(l.Tags, ","), l.SourceAgent, status, truncate(strings.TrimSpace(l.Body), 60))
	}
	_ = tw.Flush()
}

// scopeTarget returns the scope's referent for display: the ticket ID for
// ticket-scoped learnings, the component for component-scoped, else "".
func scopeTarget(l *learning.Learning) string {
	switch l.Scope {
	case learning.ScopeTicket:
		return l.Ticket
	case learning.ScopeComponent:
		return l.Component
	default:
		return ""
	}
}
