package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/underworld14/pine/internal/learning"
	"github.com/underworld14/pine/internal/search"
	"github.com/underworld14/pine/internal/store"
	"github.com/underworld14/pine/internal/tui"
	"github.com/underworld14/pine/internal/view"
)

// confirmDeleteFn is the interactive delete confirm (overridable in tests).
var confirmDeleteFn = func(summary string, in io.Reader, out io.Writer) (bool, error) {
	return tui.ConfirmDeleteIO(summary, in, out)
}

// globalConflict reports the repo-local flag that cannot be combined with
// --global. The machine-wide store holds MEMORY.md and topics only: LRN files,
// tickets and components are all project-scoped by construction.
//
// --scope global is the happy path, not a conflict: it has always meant
// "repo-wide" and is the default, so `pine learn -g` must not trip on it.
func globalConflict(scopeNorm, ticketID, sup, component string, legacyLRN bool) error {
	switch {
	case scopeNorm == learning.ScopeTicket:
		return fmt.Errorf("--global cannot be combined with --scope ticket (ticket learnings are project-scoped LRN files)")
	case scopeNorm == learning.ScopeComponent:
		// Note: --scope component routes to MEMORY/topics, not to an LRN file.
		// It is rejected because components name paths inside one repository.
		return fmt.Errorf("--global cannot be combined with --scope component (components are project-scoped)")
	case strings.TrimSpace(component) != "":
		return fmt.Errorf("--global cannot be combined with --component (components are project-scoped)")
	case legacyLRN:
		return fmt.Errorf("--global cannot be combined with --legacy-lrn (LRN files are project-scoped)")
	case sup != "":
		return fmt.Errorf("--global cannot be combined with --supersedes (LRN files are project-scoped)")
	case ticketID != "":
		return fmt.Errorf("--global cannot be combined with --ticket (tickets are project-scoped)")
	}
	return nil
}

// newLearnCmd is registered from root.go.
func newLearnCmd() *cobra.Command {
	var (
		scope, ticket, component, source, tagsCSV, supersedes, citesCSV, textFlag string
		toPath, newTopic                                                          string
		asJSON, legacyLRN, global                                                 bool
	)
	cmd := &cobra.Command{
		Use:   "learn [text]",
		Short: "Capture a durable insight into MEMORY.md, a topic file, or a ticket LRN",
		Long: `Capture a persistent, cross-session insight.

By default (no --scope ticket), Pine suggests a destination and appends to
.pine/MEMORY.md or .pine/memory/<topic>.md — avoiding one-file-per-insight bloat.
Use pine learn suggest "<text>" to preview recommendations.

Ticket-scoped one-shots still create .pine/learnings/LRN-*.md:
  pine learn "Fixed only for this ticket" --scope ticket --ticket BUG-001

Force a destination with --to MEMORY.md | --to memory/analytics.md | --new-topic slug.
Pass --legacy-lrn to create a global LRN-* file (escape hatch).

Preferences that hold in every repository go in your machine-wide store:
  pine learn -g "I use pnpm, never npm"     # → ~/.pine/MEMORY.md
  pine learn -g "..." --new-topic pnpm      # → ~/.pine/memory/pnpm.md
-g works outside a pine repo, and applies to list / search / show too.
Unlike the project store, -g appends to MEMORY.md directly instead of
suggesting a topic — pass --new-topic or --to to file it elsewhere.

Subcommands: list, search, show, suggest, supersede, rm.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			text := textFlag
			switch {
			case text != "" && len(args) > 0:
				return fmt.Errorf("provide the insight via --text or as a positional argument, not both")
			case text == "" && len(args) == 0:
				return fmt.Errorf("learning text is required (or use: pine learn list | search | show | suggest)")
			case text == "":
				text = args[0]
			}
			tags := splitCSV(tagsCSV)
			cites := splitCSV(citesCSV)
			ticketID := ""
			if ticket != "" {
				ticketID = normalizeID(ticket)
			}
			sup := ""
			if supersedes != "" {
				sup = normalizeID(supersedes)
			}

			scopeNorm := strings.ToLower(strings.TrimSpace(scope))
			if scopeNorm == "" {
				scopeNorm = learning.ScopeGlobal
			}

			// The machine-wide store is resolved before openStore, and instead
			// of it: -g must work outside a pine repo.
			if global {
				if err := globalConflict(scopeNorm, ticketID, sup, component, legacyLRN); err != nil {
					return err
				}
				ms, err := globalMemStore()
				if err != nil {
					return err
				}
				return captureMemoryInsight(cmd, ms, text, cites, "", toPath, newTopic, asJSON)
			}

			s, err := openStore()
			if err != nil {
				return err
			}

			// Ticket-scoped (or explicit legacy LRN) → classic learning file.
			if scopeNorm == learning.ScopeTicket || legacyLRN || sup != "" {
				if scopeNorm == learning.ScopeGlobal && legacyLRN {
					// keep global
				} else if scopeNorm != learning.ScopeTicket && sup != "" {
					// supersede via main command still creates LRN
				}
				l, err := s.CreateLearning(store.CreateLearningReq{
					Text:        text,
					Scope:       scopeNorm,
					Tags:        tags,
					Ticket:      ticketID,
					Component:   component,
					SourceAgent: source,
					Supersedes:  sup,
					Cites:       cites,
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
			}

			if scopeNorm == learning.ScopeComponent && strings.TrimSpace(component) == "" {
				return fmt.Errorf("--component is required when --scope component")
			}

			// Default / component → MEMORY.md or memory/<topic>.md
			return captureMemoryInsight(cmd, projectStore(s), text, cites, component, toPath, newTopic, asJSON)
		},
	}
	f := cmd.Flags()
	// "global (repo-wide)" defuses the collision with -g/--global printed a few
	// lines below: --scope global has always meant project-wide, not machine-wide.
	f.StringVar(&scope, "scope", learning.ScopeGlobal, "scope: global (repo-wide, default → MEMORY/topics) | ticket | component")
	f.BoolVarP(&global, "global", "g", false, "read/write your machine-wide memory in ~/.pine (works outside a repo; not the same as --scope global)")
	f.StringVar(&tagsCSV, "tags", "", "comma-separated tags (LRN only)")
	f.StringVar(&ticket, "ticket", "", "ticket ID (required when --scope ticket)")
	f.StringVar(&component, "component", "", "component path hint for topic matching")
	f.StringVar(&source, "source", learning.SourceManual, "source agent: claude-code|codex|cursor|gemini|manual")
	f.StringVar(&supersedes, "supersedes", "", "learning ID this insight replaces (creates LRN)")
	f.StringVar(&citesCSV, "cites", "", "comma-separated repo-relative file paths this insight depends on")
	f.StringVar(&textFlag, "text", "", `insight text, as an alternative to the positional argument (required if the insight is exactly "list", "search", "show", or "suggest")`)
	f.StringVar(&toPath, "to", "", "force destination: MEMORY.md or memory/<topic>.md")
	f.StringVar(&newTopic, "new-topic", "", "create/append memory/<slug>.md")
	f.BoolVar(&legacyLRN, "legacy-lrn", false, "create a classic LRN-* file instead of MEMORY/topics")
	f.BoolVar(&asJSON, "json", false, "output JSON")

	cmd.AddCommand(newLearnListCmd(), newLearnSearchCmd(), newLearnShowCmd(),
		newLearnSuggestCmd(), newLearnSupersedeCmd(), newLearnRmCmd())
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
				summary := fmt.Sprintf("%s: %q", id, truncate(strings.TrimSpace(l.Body), 50))
				ok, err := confirmDeleteFn(summary, cmd.InOrStdin(), cmd.OutOrStdout())
				if err != nil {
					if errors.Is(err, tui.ErrCancelled) {
						fmt.Fprintln(cmd.OutOrStdout(), "Cancelled.")
						return nil
					}
					return err
				}
				if !ok {
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

func newLearnListCmd() *cobra.Command {
	var (
		global bool
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
			// Must return before s.ListLearnings below: -g has no store, and
			// LRN files are project-only anyway.
			if global {
				ms, err := readGlobalMemStore()
				if err != nil {
					return err
				}
				if asJSON {
					return listMemoryJSON(cmd, ms)
				}
				return listMemoryEntries(cmd, ms)
			}
			s, err := openStore()
			if err != nil {
				return err
			}
			if !asJSON {
				if err := listMemoryEntries(cmd, projectStore(s)); err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), "LRN learnings:")
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
	cmd.Flags().BoolVarP(&global, "global", "g", false, "list your machine-wide memory in ~/.pine instead (works outside a repo)")
	return cmd
}

func newLearnSearchCmd() *cobra.Command {
	var (
		lf             *learningFilterFlags
		asJSON, global bool
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
			// Must return before s.ListLearnings / s.AllLearnings below: -g has
			// no store, and the LRN filter flags are meaningless against it.
			if global {
				ms, err := readGlobalMemStore()
				if err != nil {
					return err
				}
				idx, err := search.New()
				if err != nil {
					return err
				}
				defer idx.Close()
				searchMemoryInto(idx, ms.Dir)
				hits := idx.Search(args[0], search.Filter{Kind: search.KindMemory}, 20)
				if asJSON {
					out := make([]map[string]any, 0, len(hits))
					for _, h := range hits {
						out = append(out, map[string]any{
							"kind": "memory", "store": "global", "path": h.ID, "score": h.Score,
						})
					}
					return writeJSON(cmd.OutOrStdout(), out)
				}
				if len(hits) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "No global memory matched.")
					return nil
				}
				w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "PATH\tSCORE")
				for _, h := range hits {
					fmt.Fprintf(w, "%s\t%.2f\n", h.ID, h.Score)
				}
				return w.Flush()
			}
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
			searchMemoryInto(idx, s.Root())
			hits := idx.Search(args[0], search.Filter{
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
				type hitOut struct {
					Kind  string  `json:"kind"`
					Score float64 `json:"score"`
					Path  string  `json:"path,omitempty"`
					view.LearningHit
				}
				out := make([]hitOut, 0, len(hits))
				for _, h := range hits {
					if l := byID[h.ID]; l != nil {
						dto := view.BuildLearning(l)
						dto.Stale = stale[l.ID]
						dto.SupersededBy = learning.SupersededBy(rev, created, l.ID)
						out = append(out, hitOut{
							Kind:         "learning",
							Score:        h.Score,
							LearningHit:  view.LearningHit{Learning: dto, Score: h.Score},
						})
						continue
					}
					out = append(out, hitOut{Kind: "memory", Score: h.Score, Path: h.ID})
				}
				return writeJSON(cmd.OutOrStdout(), out)
			}
			if len(hits) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No learnings matched.")
				return nil
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tKIND\tSCOPE\tTARGET\tTAGS\tSCORE\tSTATUS\tBODY")
			for _, h := range hits {
				l := byID[h.ID]
				if l == nil {
					fmt.Fprintf(w, "%s\tmemory\t-\t-\t-\t%.2f\t\t\n", h.ID, h.Score)
					continue
				}
				status := ""
				if stale[l.ID] {
					status = "stale"
				}
				fmt.Fprintf(w, "%s\tlearning\t%s\t%s\t%s\t%.2f\t%s\t%s\n",
					l.ID, l.Scope, scopeTarget(l), strings.Join(l.Tags, ","), h.Score, status, truncate(strings.TrimSpace(l.Body), 60))
			}
			return w.Flush()
		},
	}
	lf = registerLearningFilterFlags(cmd)
	cmd.Flags().BoolVar(&asJSON, "json", false, "output JSON")
	cmd.Flags().BoolVarP(&global, "global", "g", false, "search your machine-wide memory in ~/.pine instead (works outside a repo)")
	return cmd
}

func newLearnShowCmd() *cobra.Command {
	var asJSON, global bool
	cmd := &cobra.Command{
		Use:   "show <id-or-path>",
		Short: "Show one learning, MEMORY.md, or a memory topic file",
		Long: `Show one learning's full detail, or a memory destination:

  pine learn show LRN-001
  pine learn show MEMORY.md
  pine learn show memory/analytics.md
  pine learn show -g MEMORY.md          # your machine-wide store`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Must return before openStore: -g works outside a repo, and there
			// are no LRN files in the machine-wide store to fall through to.
			if global {
				ms, err := readGlobalMemStore()
				if err != nil {
					return err
				}
				handled, err := tryShowMemory(cmd, ms, args[0], asJSON)
				if !handled && err == nil {
					return fmt.Errorf("no global memory entry %q (use MEMORY.md or memory/<topic>.md)", args[0])
				}
				return err
			}
			s, err := openStore()
			if err != nil {
				return err
			}
			if handled, err := tryShowMemory(cmd, projectStore(s), args[0], asJSON); handled {
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
	cmd.Flags().BoolVarP(&global, "global", "g", false, "show from your machine-wide memory in ~/.pine (works outside a repo)")
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
