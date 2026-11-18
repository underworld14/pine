package cli

import (
	"encoding/json"
	"errors"
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
		onlyBlocked, onlyReady, showAll   bool
		asJSON                            bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List open tickets (filterable); use --all for done",
		Long: `List tickets with dependency state.

By default, done tickets are hidden (Beads-style). Pass --all to include them,
or --status done to list only closed tickets.

Tickets are shown as a hierarchical tree (epic → children).`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			// Explicit --status overrides the default hide-done filter, matching
			// Beads (bd list --status closed includes closed issues).
			hideDone := !showAll && !cmd.Flags().Changed("status")
			views := collectViews(s, store.Filter{Status: status, Type: typ, Label: label, Parent: parent, Phase: phase}, onlyBlocked, onlyReady, hideDone)
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), views)
			}
			if len(views) == 0 && hideDone && hasOnlyDoneTickets(s) {
				fmt.Fprintln(cmd.OutOrStdout(), "No open tickets. Use --all to include done.")
				return nil
			}
			out := cmd.OutOrStdout()
			renderTicketTreeWith(out, views, listStyleFor(out), s.Config().Priorities)
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
	f.BoolVarP(&showAll, "all", "a", false, "include done tickets")
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
			ready := collectViews(s, store.Filter{}, false, true, false)
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), ready)
			}
			if len(ready) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "Nothing ready — every open ticket is blocked or there are none.")
				return nil
			}
			out := cmd.OutOrStdout()
			renderTicketTreeWith(out, withEpicContext(s, ready), listStyleFor(out), s.Config().Priorities)
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
		typ, title, priority, parent, phase, verify, status, bodyFile string
		labels, deps, skills, commits                                 []string
		asJSON                                                        bool
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a ticket (bug, feature, or epic)",
		Long: `Create a ticket (bug, feature, or epic).

The body (from --body-file or the type template) uses "#"-level sections.
Pine reads two of them:

  # Acceptance Criteria     checkboxes here drive progress counters
  # Related Files           one bullet per repo-relative path, e.g.

    # Related Files
    - internal/cli/tickets.go — add the flag
    - internal/contextgen/    — whole folder

  The path is the bullet's first token (directories, backticks, and a leading
  @ all work); text after it is a free-form annotation. pine context uses
  these paths to pick the tickets relevant to your current changes.

--json prints the created ticket as the same DTO pine show --json uses
(id, type, title, status, priority, labels, deps, parent, phase, verify,
skills, links, created, updated, blocked/unmet/dangling, hash, body).

Dependencies set at creation can be rewired later with pine dep add/rm or
pine update --add-dep/--rm-dep.

--commit <rev> (repeatable) records those commits as the ticket's work
evidence: a "## Work Evidence" section listing them with a file stat. Use it to
file a ticket for work that is already committed, e.g.

  pine create --type bug --title "Fix login" --status testing --commit HEAD

pine close --evidence keeps a section recorded this way. A bad rev creates
nothing.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			body, err := readBody(bodyFile, cmd.InOrStdin())
			if err != nil {
				return err
			}
			var recorded *recordedCommits
			if len(commits) > 0 {
				if recorded, err = resolveCommits(s, commits); err != nil {
					return err
				}
			}
			t, err := s.Create(store.CreateReq{
				Type:     typ,
				Title:    title,
				Priority: priority,
				Labels:   labels,
				Deps:     normalizeIDs(deps),
				Parent:   normalizeID(parent),
				Phase:    phase,
				Verify:   verify,
				Skills:   skills,
				Status:   status,
				Body:     body,
			})
			if err != nil {
				return err
			}
			if recorded != nil {
				ev := recorded.evidence("pine create --commit")
				if t, err = s.Update(t.ID, func(u *ticket.Ticket) error {
					u.Body = appendEvidence(u.Body, ev)
					return nil
				}); err != nil {
					return fmt.Errorf("created %s but could not record commit evidence: %w", t.ID, err)
				}
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
	f.StringVar(&verify, "verify", "", "proof command run by pine close --evidence (e.g. a grep for callers or a smoke test)")
	f.StringSliceVar(&skills, "skill", nil, "agent skills the executing agent must load (repeatable or comma-separated)")
	f.StringVar(&status, "status", "", "initial status (defaults to first board column)")
	f.StringVar(&bodyFile, "body-file", "", "read the body from a file, or '-' for stdin")
	f.StringSliceVar(&commits, "commit", nil, "record these commits (sha, tag, HEAD~1, …) as the ticket's work evidence (repeatable or comma-separated)")
	f.BoolVar(&asJSON, "json", false, "output JSON")
	_ = cmd.MarkFlagRequired("type")
	_ = cmd.MarkFlagRequired("title")
	return cmd
}

// --- update ---

func newUpdateCmd() *cobra.Command {
	var (
		status, title, priority, parent, phase, verify, bodyFile string
		addLabels, rmLabels, addDeps, rmDeps                     []string
		addSkills, rmSkills                                      []string
	)
	cmd := &cobra.Command{
		Use:   "update <ID>",
		Short: "Update a ticket's fields (body via --body-file, deps via --add-dep/--rm-dep)",
		Long: `Update a ticket's fields.

Use --body-file <path|-> to replace the markdown body (everything after the
frontmatter). Dependencies can be rewired with --add-dep/--rm-dep here, or via
pine dep add/rm (both refuse cycles). Agent skills required by the ticket are
managed with --add-skill/--rm-skill and surfaced to the executing agent by
pine prompt and pine context.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			id := normalizeID(args[0])
			flags := cmd.Flags()
			var body string
			if flags.Changed("body-file") {
				if body, err = readBody(bodyFile, cmd.InOrStdin()); err != nil {
					return err
				}
			}
			// Resolve the final dep set (and refuse cycles) before the locked
			// update, mirroring pine dep add/rm.
			depsChanged := len(addDeps) > 0 || len(rmDeps) > 0
			var mergedDeps []string
			if depsChanged {
				cur, err := s.Get(id)
				if err != nil {
					return err
				}
				mergedDeps = removeDeps(mergeDeps(cur.Deps, normalizeIDs(addDeps)), normalizeIDs(rmDeps))
				if cyc := wouldCycle(s, id, mergedDeps); cyc != nil {
					return fmt.Errorf("that dependency would create a cycle among: %s", strings.Join(cyc, ", "))
				}
			}
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
				if flags.Changed("verify") {
					if strings.EqualFold(verify, "none") {
						u.Verify = ""
					} else {
						u.Verify = verify
					}
				}
				if depsChanged {
					u.Deps = mergedDeps
				}
				if flags.Changed("body-file") {
					// Match create: the body starts on its own line after the
					// closing frontmatter delimiter.
					if body != "" && !strings.HasPrefix(body, "\n") {
						body = "\n" + body
					}
					u.Body = body
				}
				u.Labels = applySetEdits(u.Labels, addLabels, rmLabels)
				u.Skills = applySetEdits(u.Skills, addSkills, rmSkills)
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
	f.StringVar(&verify, "verify", "", "proof command run by pine close --evidence, or 'none' to clear")
	f.StringVar(&bodyFile, "body-file", "", "replace the markdown body from a file, or '-' for stdin")
	f.StringSliceVar(&addDeps, "add-dep", nil, "dependency ticket ids to add (see also pine dep add)")
	f.StringSliceVar(&rmDeps, "rm-dep", nil, "dependency ticket ids to remove (see also pine dep rm)")
	f.StringSliceVar(&addLabels, "add-label", nil, "labels to add")
	f.StringSliceVar(&rmLabels, "rm-label", nil, "labels to remove")
	f.StringSliceVar(&addSkills, "add-skill", nil, "agent skills the executing agent must load (added to skills:)")
	f.StringSliceVar(&rmSkills, "rm-skill", nil, "agent skills to remove from skills:")
	return cmd
}

// --- close ---

func newCloseCmd() *cobra.Command {
	var evidence, force bool
	var commits []string
	cmd := &cobra.Command{
		Use:   "close <ID>...",
		Short: "Mark one or more tickets done",
		Long: `Mark one or more tickets done.

With --evidence, append a "## Work Evidence" section to each ticket's body
(commits that reference/touch it + a diffstat from the commit at its creation
time to the working tree) before marking it done — a durable record of what
changed, suitable for review and audits.

When a ticket defines a ` + "`verify:`" + ` frontmatter command, --evidence also runs it
in the repo root and appends a "## Verification" section with the exit status
and output. A failing verify command refuses the close (the ticket stays open);
pass --force to close anyway and record the failure.

A Work Evidence section recorded from explicit commits (pine create --commit)
is kept as is, since it is more exact than a diff from the ticket's creation
time. Pass --commit <rev> (repeatable) with --evidence to record, or re-record,
the evidence from specific commits instead.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(commits) > 0 && !evidence {
				return errors.New("--commit records work evidence, so it needs --evidence")
			}
			s, err := openStore()
			if err != nil {
				return err
			}
			var recorded *recordedCommits
			if len(commits) > 0 {
				if recorded, err = resolveCommits(s, commits); err != nil {
					return err
				}
			}
			for _, arg := range args {
				id := normalizeID(arg)
				// Run verification and compute evidence BEFORE the locked update
				// so neither the verify command nor git runs under the store
				// write lock.
				ev := ""
				keptRecorded := false
				ver := verifyResult{}
				if evidence {
					ver = verifyEvidence(s, id)
					if ver.failed() && !force {
						tail := strings.TrimSpace(ver.output)
						if len(tail) > 600 {
							tail = "…" + tail[len(tail)-600:]
						}
						return fmt.Errorf("%s: verification FAILED (exit %d) — ticket left open; fix it or pass --force\n%s",
							id, ver.exitCode, tail)
					}
					switch cur, gerr := s.Get(id); {
					case recorded != nil:
						ev = recorded.evidence("pine close --evidence --commit")
					case gerr == nil && hasRecordedEvidence(cur.Body):
						keptRecorded = true
					default:
						ev = workEvidence(s, id)
					}
				}
				verSection := ver.section()
				if _, err := s.Update(id, func(u *ticket.Ticket) error {
					u.Status = ticket.StatusDone
					if evidence {
						if ev != "" {
							u.Body = appendEvidence(u.Body, ev)
						}
						if verSection != "" {
							u.Body = appendSection(u.Body, "Verification", verSection)
						}
					}
					return nil
				}); err != nil {
					return fmt.Errorf("%s: %w", id, err)
				}
				switch {
				case evidence && ver.failed():
					fmt.Fprintf(cmd.OutOrStdout(), "Closed %s (verification FAILED, recorded — --force was passed)\n", id)
				case evidence && keptRecorded:
					fmt.Fprintf(cmd.OutOrStdout(), "Closed %s (kept recorded commit evidence)\n", id)
				case evidence:
					fmt.Fprintf(cmd.OutOrStdout(), "Closed %s (with work evidence)\n", id)
				default:
					fmt.Fprintf(cmd.OutOrStdout(), "Closed %s\n", id)
				}
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.BoolVar(&evidence, "evidence", false, "attach a Work Evidence section (commits + file changes) and run the ticket's verify: command before marking it done")
	f.BoolVar(&force, "force", false, "close even when the verify: command fails (the failure is recorded)")
	f.StringSliceVar(&commits, "commit", nil, "with --evidence: record the evidence from these commits (sha, tag, HEAD~1, …) instead of deriving it (repeatable or comma-separated)")
	return cmd
}

// --- shared helpers ---

func collectViews(s *store.Store, filter store.Filter, onlyBlocked, onlyReady, hideDone bool) []view.Ticket {
	g := s.Graph()
	tickets := s.List(filter)
	var sel []*ticket.Ticket
	for _, t := range tickets {
		if hideDone && t.Status == ticket.StatusDone {
			continue
		}
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

// hasOnlyDoneTickets reports whether the store has at least one ticket and every
// ticket is done. Used so the list empty-state hint ("Use --all…") only appears
// when hiding done would empty an otherwise all-closed board — not when other
// filters (type/label/parent/…) simply match nothing.
func hasOnlyDoneTickets(s *store.Store) bool {
	all := s.All()
	if len(all) == 0 {
		return false
	}
	for _, t := range all {
		if t.Status != ticket.StatusDone {
			return false
		}
	}
	return true
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
	if len(v.Skills) > 0 {
		fmt.Fprintf(w, "   skills: %s", strings.Join(v.Skills, ", "))
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

// applySetEdits applies add/remove edits to a string-set frontmatter field
// (labels, skills): order-preserving dedupe, empty entries dropped, removals
// honored. With no edits it returns the set unchanged (still deduped), so a
// bare pine update never churns the file.
func applySetEdits(set, add, rm []string) []string {
	out := ticket.DedupeNonEmpty(set)
	if len(add) > 0 {
		out = ticket.DedupeNonEmpty(append(out, add...))
	}
	if len(rm) > 0 {
		remove := map[string]bool{}
		for _, v := range rm {
			remove[v] = true
		}
		var filtered []string
		for _, v := range out {
			if !remove[v] {
				filtered = append(filtered, v)
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
