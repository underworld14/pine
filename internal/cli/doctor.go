package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/underworld14/pine/internal/doctor"
	"github.com/underworld14/pine/internal/store"
)

// doctorFinding is the JSON shape for `pine doctor --json`.
type doctorFinding struct {
	Level   string `json:"level"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
	Fixable bool   `json:"fixable"`
}

func levelName(l doctor.Level) string {
	switch l {
	case doctor.LevelWarn:
		return "warn"
	case doctor.LevelError:
		return "error"
	default:
		return "ok"
	}
}

func newDoctorCmd() *cobra.Command {
	var fix, dryRun, asJSON bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check the workspace for problems (optionally auto-repair)",
		Long: `Validate the .pine workspace and report every problem at once.

With --fix, mechanically repairable findings (dangling deps/parent, frontmatter
id mismatches, dangling cites/supersedes, renameable stray files) are applied;
findings that need human judgment (malformed files, cycles, missing attachments)
are always left reported. --dry-run lists what --fix would change without
writing. --json emits the findings as machine-readable output.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			rep := doctor.Run(s)
			out := cmd.OutOrStdout()

			if fix && !dryRun {
				// In JSON mode, apply fixes silently so stdout stays valid JSON.
				applied, failed := applyDoctorFixes(cmd, s, rep, asJSON)
				if !asJSON && (applied > 0 || failed > 0) {
					fmt.Fprintf(out, "\napplied %d fix(es)", applied)
					if failed > 0 {
						fmt.Fprintf(out, ", %d failed", failed)
					}
					fmt.Fprintln(out)
				}
				// Re-open so renamed strays and rewritten files are reflected,
				// then report what remains.
				s, err = openStore()
				if err != nil {
					return err
				}
				rep = doctor.Run(s)
				if !asJSON {
					fmt.Fprintln(out, "\nremaining:")
				}
			}

			if asJSON {
				return writeJSON(out, toDoctorFindings(rep))
			}

			errCount := 0
			for _, f := range rep.Findings {
				sym := "✓"
				switch f.Level {
				case doctor.LevelWarn:
					sym = "!"
				case doctor.LevelError:
					sym = "✗"
					errCount++
				}
				suffix := ""
				if dryRun && f.Fixable() {
					suffix = "  [fixable]"
				}
				fmt.Fprintf(out, "%s %s%s\n", sym, f.Msg, suffix)
			}
			if dryRun {
				fmt.Fprintf(out, "\n%d finding(s) can be auto-fixed with 'pine doctor --fix'\n", rep.FixableCount())
			}
			if rep.HasErrors() {
				return fmt.Errorf("doctor found %d problem(s)", errCount)
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.BoolVar(&fix, "fix", false, "apply mechanical repairs for fixable findings")
	f.BoolVar(&dryRun, "dry-run", false, "show what --fix would change without writing")
	f.BoolVar(&asJSON, "json", false, "output findings as JSON")
	return cmd
}

// applyDoctorFixes runs each fixable finding's repair, reporting per-fix outcome
// unless quiet is set (JSON mode, where any text would corrupt the output).
func applyDoctorFixes(cmd *cobra.Command, s *store.Store, rep *doctor.Report, quiet bool) (applied, failed int) {
	out := cmd.OutOrStdout()
	for _, f := range rep.Findings {
		if !f.Fixable() {
			continue
		}
		if err := f.Fix(s); err != nil {
			if !quiet {
				fmt.Fprintf(out, "✗ could not fix (%s): %v\n", f.Code, err)
			}
			failed++
			continue
		}
		if !quiet {
			fmt.Fprintf(out, "✓ fixed: %s\n", f.Msg)
		}
		applied++
	}
	return applied, failed
}

func toDoctorFindings(rep *doctor.Report) []doctorFinding {
	out := make([]doctorFinding, 0, len(rep.Findings))
	for _, f := range rep.Findings {
		out = append(out, doctorFinding{
			Level:   levelName(f.Level),
			Code:    f.Code,
			Message: f.Msg,
			Fixable: f.Fixable(),
		})
	}
	return out
}
