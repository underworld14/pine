package cli

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/underworld14/pine/internal/gitx"
)

func newLogCmd() *cobra.Command {
	var asJSON bool
	var limit int
	cmd := &cobra.Command{
		Use:   "log <ID>",
		Short: "Show git commits that reference or touch a ticket",
		Long: `List git commits related to a ticket: those whose message mentions the
ticket ID and those that modified the ticket's file under .pine/tickets/.
Results are de-duplicated and ordered newest first.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			id := normalizeID(args[0])
			if _, err := s.Get(id); err != nil {
				return err
			}

			client := gitx.New(filepath.Dir(s.Root()))
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if !client.IsRepo(ctx) {
				return fmt.Errorf("not a git repository; pine log needs git history")
			}
			// The tickets dir relative to the repo root, forward-slashed for git.
			pathspec := path.Join(filepath.Base(s.Root()), "tickets", id+".md")
			commits := client.Log(ctx, pathspec, id, limit)

			if asJSON {
				return writeJSON(cmd.OutOrStdout(), commits)
			}
			if len(commits) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No commits reference or touch %s.\n", id)
				return nil
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "COMMIT\tDATE\tAUTHOR\tSUBJECT")
			for _, c := range commits {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", c.Hash, c.When.Format("2006-01-02"), c.Author, c.Subject)
			}
			return w.Flush()
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output JSON")
	cmd.Flags().IntVar(&limit, "limit", 30, "maximum number of commits to show")
	return cmd
}
