package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/underworld14/pine/internal/store"
	"github.com/underworld14/pine/internal/ticket"
	"github.com/underworld14/pine/internal/view"
)

// --- list ---

func newListCmd() *cobra.Command {
	var (
		status, typ, label, parent, phase string
		onlyBlocked, onlyReady            bool
		asJSON                            bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tickets (filterable), with dependency state",
		Long: `List tickets with dependency state.

Tickets are shown as a hierarchical tree (epic → children), Beads-style.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			views := collectViews(s, store.Filter{Status: status, Type: typ, Label: label, Parent: parent, Phase: phase}, onlyBlocked, onlyReady)
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), views)
			}
			renderTicketTree(cmd.OutOrStdout(), views)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&status, "status", "", "filter by status")
	f.StringVar(&typ, "type", "", "filter by type prefix (BUG, FEAT, EPIC)")
	f.StringVar(&label, "label", "", "filter by label")
	f.StringVar(&parent, "parent", "", "filter by epic parent id")
	f.StringVar(&phase, "phase", "", "filter by phase (e.g. p0, p1)")
	f.BoolVar(&onlyBlocked, "blocked", false, "only blocked tickets")
	f.BoolVar(&onlyReady, "ready", false, "only ready (unblocked, open) tickets")
	f.BoolVar(&asJSON, "json", false, "output JSON")
	return cmd
}

// --- ready ---

func newReadyCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "ready",
		Short: "List actionable tickets: open and unblocked, most urgent first",
		Long: `List actionable tickets (open and unblocked), most urgent first.

Text output is an epic → children tree. Parent epics that are not themselves
ready are still shown as headers so ready children stay nested. JSON is the
ready set only (no injected epic headers).`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			ready := collectViews(s, store.Filter{}, false, true)
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), ready)
			}
			if len(ready) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "Nothing ready — every open ticket is blocked or there are none.")
				return nil
			}
			renderTicketTree(cmd.OutOrStdout(), withEpicContext(s, ready))
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output JSON")
	return cmd
}

// --- show ---

func newShowCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "show <ID>",
		Short: "Show a ticket in full, with dependencies and children",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			id := normalizeID(args[0])
			t, err := s.Get(id)
			if err != nil {
				return err
			}
			v := view.Build(s, s.Graph(), t, true)
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), v)
			}
			renderTicketDetail(cmd.OutOrStdout(), v)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output JSON")
	return cmd
}

// --- create ---

func newCreateCmd() *cobra.Command {
	var (
		typ, title, priority, parent, phase, status, bodyFile string
		labels, deps                                          []string
		asJSON                                                bool
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a ticket (bug, feature, or epic)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			body, err := readBody(bodyFile, cmd.InOrStdin())
			if err != nil {
				return err
			}
			t, err := s.Create(store.CreateReq{
				Type:     typ,
				Title:    title,
				Priority: priority,
				Labels:   labels,
				Deps:     normalizeIDs(deps),
				Parent:   normalizeID(parent),
				Phase:    phase,
				Status:   status,
				Body:     body,
			})
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), view.Build(s, s.Graph(), t, true))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created %s: %s\n", t.ID, t.Title)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&typ, "type", "", "ticket type: bug | feature | epic (required)")
	f.StringVar(&title, "title", "", "ticket title (required)")
	f.StringVarP(&priority, "priority", "p", "", "priority: low|medium|high|critical")
	f.StringSliceVarP(&labels, "label", "l", nil, "labels (repeatable or comma-separated)")
	f.StringSliceVar(&deps, "deps", nil, "dependency ticket ids (blocked until they are done)")
	f.StringVar(&parent, "parent", "", "epic ticket id this belongs to")
	f.StringVar(&phase, "phase", "", "optional phase (e.g. p0, p1, p2)")
	f.StringVar(&status, "status", "", "initial status (defaults to first board column)")
	f.StringVar(&bodyFile, "body-file", "", "read the body from a file, or '-' for stdin")
	f.BoolVar(&asJSON, "json", false, "output JSON")
	_ = cmd.MarkFlagRequired("type")
	_ = cmd.MarkFlagRequired("title")
	return cmd
}

// --- update ---

func newUpdateCmd() *cobra.Command {
	var (
		status, title, priority, parent, phase string
		addLabels, rmLabels                    []string
	)
	cmd := &cobra.Command{
		Use:   "update <ID>",
		Short: "Update a ticket's fields (body is left untouched)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			id := normalizeID(args[0])
			flags := cmd.Flags()
			t, err := s.Update(id, func(u *ticket.Ticket) error {
				if flags.Changed("status") {
					u.Status = status
				}
				if flags.Changed("title") {
					u.Title = title
				}
				if flags.Changed("priority") {
					u.Priority = priority
				}
				if flags.Changed("parent") {
					if strings.EqualFold(parent, "none") || parent == "" {
						u.Parent = ""
					} else {
						u.Parent = normalizeID(parent)
					}
				}
				if flags.Changed("phase") {
					if strings.EqualFold(phase, "none") {
						u.Phase = ""
					} else {
						u.Phase = phase
					}
				}
				u.Labels = applyLabelEdits(u.Labels, addLabels, rmLabels)
				return nil
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Updated %s\n", t.ID)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&status, "status", "", "new status")
	f.StringVar(&title, "title", "", "new title")
	f.StringVarP(&priority, "priority", "p", "", "new priority")
	f.StringVar(&parent, "parent", "", "new epic parent id, or 'none' to clear")
	f.StringVar(&phase, "phase", "", "new phase (e.g. p0, p1), or 'none' to clear")
	f.StringSliceVar(&addLabels, "add-label", nil, "labels to add")
	f.StringSliceVar(&rmLabels, "rm-label", nil, "labels to remove")
	return cmd
}

// --- close ---

func newCloseCmd() *cobra.Command {
	var evidence bool
	cmd := &cobra.Command{
		Use:   "close <ID>...",
		Short: "Mark one or more tickets done",
		Long: `Mark one or more tickets done.

With --evidence, append a "## Work Evidence" section to each ticket's body
(commits that reference/touch it + a diffstat from the commit at its creation
time to the working tree) before marking it done — a durable record of what
changed, suitable for review and audits.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			for _, arg := range args {
				id := normalizeID(arg)
				// Compute evidence BEFORE the locked update so git never runs
				// under the store write lock.
				ev := ""
				if evidence {
					ev = workEvidence(s, id)
				}
				if _, err := s.Update(id, func(u *ticket.Ticket) error {
					u.Status = ticket.StatusDone
					if evidence {
						u.Body = appendEvidence(u.Body, ev)
					}
					return nil
				}); err != nil {
					return fmt.Errorf("%s: %w", id, err)
				}
				if evidence {
					fmt.Fprintf(cmd.OutOrStdout(), "Closed %s (with work evidence)\n", id)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "Closed %s\n", id)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&evidence, "evidence", false, "attach a Work Evidence section (commits + file changes) to each ticket before marking it done")
	return cmd
}

// --- shared helpers ---

func collectViews(s *store.Store, filter store.Filter, onlyBlocked, onlyReady bool) []view.Ticket {
	g := s.Graph()
	tickets := s.List(filter)
	var sel []*ticket.Ticket
	for _, t := range tickets {
		if onlyBlocked && !g.Blocked(t.ID) {
			continue
		}
		if onlyReady && !g.Ready(t.ID) {
			continue
		}
		sel = append(sel, t)
	}
	s.SortByPriorityThenUpdated(sel)
	out := make([]view.Ticket, 0, len(sel))
	for _, t := range sel {
		out = append(out, view.Build(s, g, t, false))
	}
	return out
}

// withEpicContext injects parent epic views that are missing from views so
// ready children can nest under their epic in tree output. JSON callers should
// pass the ready set without this helper.
func withEpicContext(s *store.Store, views []view.Ticket) []view.Ticket {
	if len(views) == 0 {
		return views
	}
	byID := make(map[string]struct{}, len(views))
	for _, v := range views {
		byID[v.ID] = struct{}{}
	}
	g := s.Graph()
	var extra []view.Ticket
	seen := map[string]bool{}
	for _, v := range views {
		if v.Parent == "" || seen[v.Parent] {
			continue
		}
		seen[v.Parent] = true
		if _, ok := byID[v.Parent]; ok {
			continue
		}
		t, err := s.Get(v.Parent)
		if err != nil {
			continue
		}
		extra = append(extra, view.Build(s, g, t, false))
	}
	if len(extra) == 0 {
		return views
	}
	out := append(append([]view.Ticket(nil), views...), extra...)
	sortViewsByPriorityThenUpdated(s, out)
	return out
}

func sortViewsByPriorityThenUpdated(s *store.Store, views []view.Ticket) {
	prios := s.Config().Priorities
	sort.SliceStable(views, func(i, j int) bool {
		ri := ticket.PriorityRank(views[i].Priority, prios)
		rj := ticket.PriorityRank(views[j].Priority, prios)
		if ri != rj {
			return ri > rj
		}
		return views[i].Updated > views[j].Updated
	})
}

func renderTicketDetail(w io.Writer, v view.Ticket) {
	fmt.Fprintf(w, "%s  %s\n", v.ID, v.Title)
	fmt.Fprintf(w, "status: %s   priority: %s", v.Status, v.Priority)
	if v.Phase != "" {
		fmt.Fprintf(w, "   phase: %s", v.Phase)
	}
	if len(v.Labels) > 0 {
		fmt.Fprintf(w, "   labels: %s", strings.Join(v.Labels, ", "))
	}
	fmt.Fprintln(w)
	if v.Parent != "" {
		fmt.Fprintf(w, "parent: %s\n", v.Parent)
	}
	if len(v.Deps) > 0 {
		fmt.Fprintf(w, "deps: %s", strings.Join(v.Deps, ", "))
		if v.Blocked {
			fmt.Fprintf(w, "   [BLOCKED: %s]", depSummary(v))
		}
		fmt.Fprintln(w)
	}
	if v.Degraded {
		fmt.Fprintln(w, "note: this ticket is degraded (frontmatter could not be parsed); shown read-only")
	}
	fmt.Fprintf(w, "created: %s   updated: %s\n", v.Created, v.Updated)

	if v.EpicProgress != nil {
		fmt.Fprintf(w, "\nchildren (%d/%d done):\n", v.EpicProgress.Done, v.EpicProgress.Total)
		for _, c := range v.Children {
			fmt.Fprintf(w, "  %s  [%s]  %s\n", c.ID, c.Status, c.Title)
		}
	}

	if strings.TrimSpace(v.Body) != "" {
		fmt.Fprintf(w, "\n%s\n", strings.TrimRight(v.Body, "\n"))
	}
	if len(v.Attachments) > 0 {
		fmt.Fprintln(w, "\nattachments:")
		for _, a := range v.Attachments {
			fmt.Fprintf(w, "  %s  (%s, %s)\n", a.Name, a.Kind, humanBytes(a.Size))
		}
	}
}

func depSummary(v view.Ticket) string {
	switch {
	case v.InCycle:
		return "🔒 cycle"
	case len(v.Unmet) > 0:
		return fmt.Sprintf("🔒 %d unmet", len(v.Unmet))
	case len(v.Dangling) > 0:
		return fmt.Sprintf("⚠ %d dangling", len(v.Dangling))
	case len(v.Deps) > 0:
		return "ready"
	default:
		return ""
	}
}

func applyLabelEdits(labels, add, rm []string) []string {
	set := map[string]bool{}
	var out []string
	for _, l := range labels {
		if !set[l] {
			set[l] = true
			out = append(out, l)
		}
	}
	for _, l := range add {
		if l != "" && !set[l] {
			set[l] = true
			out = append(out, l)
		}
	}
	if len(rm) > 0 {
		remove := map[string]bool{}
		for _, l := range rm {
			remove[l] = true
		}
		var filtered []string
		for _, l := range out {
			if !remove[l] {
				filtered = append(filtered, l)
			}
		}
		out = filtered
	}
	return out
}

func readBody(bodyFile string, stdin io.Reader) (string, error) {
	if bodyFile == "" {
		return "", nil
	}
	if bodyFile == "-" {
		data, err := io.ReadAll(stdin)
		return string(data), err
	}
	data, err := os.ReadFile(bodyFile)
	return string(data), err
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// normalizeID uppercases the prefix of a ticket id while preserving the suffix.
// Hash suffixes are lowercase (BUG-7f3k2a), so uppercasing the whole id would
// break the match; only the type prefix is normalized (so "bug-1" → "BUG-1").
func normalizeID(id string) string {
	i := strings.IndexByte(id, '-')
	if i < 0 {
		return strings.ToUpper(id)
	}
	return strings.ToUpper(id[:i]) + id[i:]
}

func normalizeIDs(ids []string) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = normalizeID(id)
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGT"[exp])
}
