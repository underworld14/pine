package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/underworld14/pine/internal/store"
	"github.com/underworld14/pine/internal/ticket"
	"github.com/underworld14/pine/internal/view"
)

// --- list ---

func newListCmd() *cobra.Command {
	var (
		status, typ, label, parent string
		onlyBlocked, onlyReady     bool
		asJSON                     bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tickets (filterable), with dependency state",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			views := collectViews(s, store.Filter{Status: status, Type: typ, Label: label, Parent: parent}, onlyBlocked, onlyReady)
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), views)
			}
			renderTicketTable(cmd.OutOrStdout(), views)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&status, "status", "", "filter by status")
	f.StringVar(&typ, "type", "", "filter by type prefix (BUG, FEAT, EPIC)")
	f.StringVar(&label, "label", "", "filter by label")
	f.StringVar(&parent, "parent", "", "filter by epic parent id")
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
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			views := collectViews(s, store.Filter{}, false, true)
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), views)
			}
			if len(views) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "Nothing ready — every open ticket is blocked or there are none.")
				return nil
			}
			renderTicketTable(cmd.OutOrStdout(), views)
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
			id := strings.ToUpper(args[0])
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
		typ, title, priority, parent, status, bodyFile string
		labels, deps                                   []string
		asJSON                                         bool
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
				Deps:     upperAll(deps),
				Parent:   strings.ToUpper(parent),
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
		status, title, priority, parent string
		addLabels, rmLabels             []string
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
			id := strings.ToUpper(args[0])
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
						u.Parent = strings.ToUpper(parent)
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
	f.StringSliceVar(&addLabels, "add-label", nil, "labels to add")
	f.StringSliceVar(&rmLabels, "rm-label", nil, "labels to remove")
	return cmd
}

// --- close ---

func newCloseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "close <ID>...",
		Short: "Mark one or more tickets done",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			for _, arg := range args {
				id := strings.ToUpper(arg)
				if _, err := s.Update(id, func(u *ticket.Ticket) error {
					u.Status = ticket.StatusDone
					return nil
				}); err != nil {
					return fmt.Errorf("%s: %w", id, err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Closed %s\n", id)
			}
			return nil
		},
	}
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

func renderTicketTable(w io.Writer, views []view.Ticket) {
	if len(views) == 0 {
		fmt.Fprintln(w, "No tickets.")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSTATUS\tPRI\tTITLE\tDEPS")
	for _, v := range views {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", v.ID, v.Status, v.Priority, truncate(v.Title, 48), depSummary(v))
	}
	tw.Flush()
}

func renderTicketDetail(w io.Writer, v view.Ticket) {
	fmt.Fprintf(w, "%s  %s\n", v.ID, v.Title)
	fmt.Fprintf(w, "status: %s   priority: %s", v.Status, v.Priority)
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

func upperAll(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = strings.ToUpper(s)
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
