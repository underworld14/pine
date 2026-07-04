package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/underworld14/pine/internal/doctor"
)

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check the workspace for problems",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			rep := doctor.Run(s)
			out := cmd.OutOrStdout()
			errors := 0
			for _, f := range rep.Findings {
				sym := "✓"
				switch f.Level {
				case doctor.LevelWarn:
					sym = "!"
				case doctor.LevelError:
					sym = "✗"
					errors++
				}
				fmt.Fprintf(out, "%s %s\n", sym, f.Msg)
			}
			if rep.HasErrors() {
				return fmt.Errorf("doctor found %d problem(s)", errors)
			}
			return nil
		},
	}
}
