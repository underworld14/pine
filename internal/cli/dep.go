package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/underworld14/pine/internal/store"
	"github.com/underworld14/pine/internal/ticket"
)

func newDepCmd() *cobra.Command {
	dep := &cobra.Command{
		Use:   "dep",
		Short: "Manage ticket dependencies (blocked-by relationships)",
	}
	dep.AddCommand(newDepAddCmd(), newDepRmCmd(), newDepTreeCmd())
	return dep
}

func newDepAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <ID> <DEP-ID>...",
		Short: "Add one or more dependencies to a ticket (refuses cycles)",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			id := normalizeID(args[0])
			cur, err := s.Get(id)
			if err != nil {
				return err
			}
			merged := mergeDeps(cur.Deps, normalizeIDs(args[1:]))
			if cyc := wouldCycle(s, id, merged); cyc != nil {
				return fmt.Errorf("that dependency would create a cycle among: %s", strings.Join(cyc, ", "))
			}
			if _, err := s.Update(id, func(u *ticket.Ticket) error {
				u.Deps = merged
				return nil
			}); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s now depends on: %s\n", id, strings.Join(merged, ", "))
			return nil
		},
	}
}

func newDepRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <ID> <DEP-ID>...",
		Short: "Remove dependencies from a ticket",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			id := normalizeID(args[0])
			cur, err := s.Get(id)
			if err != nil {
				return err
			}
			remaining := removeDeps(cur.Deps, normalizeIDs(args[1:]))
			if _, err := s.Update(id, func(u *ticket.Ticket) error {
				u.Deps = remaining
				return nil
			}); err != nil {
				return err
			}
			if len(remaining) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "%s has no dependencies\n", id)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "%s now depends on: %s\n", id, strings.Join(remaining, ", "))
			}
			return nil
		},
	}
}

func newDepTreeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tree <ID>",
		Short: "Print a ticket's dependency tree",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			id := normalizeID(args[0])
			if _, err := s.Get(id); err != nil {
				return err
			}
			byID := map[string]*ticket.Ticket{}
			for _, t := range s.All() {
				byID[t.ID] = t
			}
			printDepTree(cmd.OutOrStdout(), byID, id, 0, map[string]bool{})
			return nil
		},
	}
}

// wouldCycle simulates setting id's dependencies to deps and reports the cycle
// (list of member IDs) if that introduces one, else nil.
func wouldCycle(s *store.Store, id string, deps []string) []string {
	all := s.All()
	for _, t := range all {
		if t.ID == id {
			t.Deps = deps
		}
	}
	g := ticket.NewGraph(all)
	if !g.Deps(id).InCycle {
		return nil
	}
	for _, c := range g.Cycles() {
		for _, m := range c {
			if m == id {
				return c
			}
		}
	}
	return []string{id}
}

func printDepTree(w io.Writer, byID map[string]*ticket.Ticket, id string, depth int, onPath map[string]bool) {
	indent := strings.Repeat("  ", depth)
	t := byID[id]
	if t == nil {
		fmt.Fprintf(w, "%s%s (missing)\n", indent, id)
		return
	}
	if onPath[id] {
		fmt.Fprintf(w, "%s%s ↺ cycle\n", indent, id)
		return
	}
	fmt.Fprintf(w, "%s%s [%s] %s\n", indent, id, t.Status, t.Title)
	onPath[id] = true
	for _, dep := range t.Deps {
		printDepTree(w, byID, dep, depth+1, onPath)
	}
	delete(onPath, id)
}

func mergeDeps(cur, add []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, d := range cur {
		if !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	for _, d := range add {
		if d != "" && !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	return out
}

func removeDeps(cur, rm []string) []string {
	remove := map[string]bool{}
	for _, d := range rm {
		remove[d] = true
	}
	var out []string
	for _, d := range cur {
		if !remove[d] {
			out = append(out, d)
		}
	}
	return out
}
