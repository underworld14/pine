package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/underworld14/pine/internal/attach"
	"github.com/underworld14/pine/internal/ticket"
)

func newOptimizeCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "optimize",
		Short: "Re-compress images dropped into attachments without optimization",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			cfg := attach.FromConfig(s.Config().Attachments)
			out := cmd.OutOrStdout()
			var before, after int64
			count := 0

			for _, id := range s.AttachmentDirs() {
				for _, a := range s.Attachments(id) {
					if a.Kind != "image" {
						continue
					}
					path, err := s.AttachmentFilePath(id, a.Name)
					if err != nil {
						continue
					}
					data, err := os.ReadFile(path)
					if err != nil {
						continue
					}
					p, err := attach.Process(a.Name, data, cfg)
					if err != nil || !p.Optimized || p.FileName == a.Name {
						continue
					}
					count++
					before += a.Size
					after += p.FinalBytes
					fmt.Fprintf(out, "%s: %s → %s (%s → %s)\n", id, a.Name, p.FileName, humanBytes(a.Size), humanBytes(p.FinalBytes))
					if dryRun {
						continue
					}
					if _, err := s.WriteAttachment(id, p.FileName, p.Data); err != nil {
						continue
					}
					_ = s.DeleteAttachment(id, a.Name)
					oldName, newName := a.Name, p.FileName
					_, _ = s.Update(id, func(u *ticket.Ticket) error {
						u.Body = strings.ReplaceAll(u.Body, oldName, newName)
						return nil
					})
				}
			}

			if count == 0 {
				fmt.Fprintln(out, "Nothing to optimize.")
				return nil
			}
			verb := "saved"
			if dryRun {
				verb = "would save"
			}
			fmt.Fprintf(out, "\n%d file(s) — %s %s\n", count, verb, humanBytes(before-after))
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would change without writing")
	return cmd
}
