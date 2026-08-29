package cli

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/underworld14/pine/internal/selfupdate"
)

// errUpdateAvailable is returned by `pine upgrade --check` when a newer
// release exists so Execute can exit 1 for scripts without treating it as a
// hard failure message on stderr.
var errUpdateAvailable = errors.New("update available")

// upgradeNewClient builds the GitHub client; overridden in tests.
var upgradeNewClient = selfupdate.NewClient

func newUpgradeCmd() *cobra.Command {
	var check, force bool
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Update the pine binary from the latest GitHub release",
		Long: `Check GitHub Releases for a newer pine version and replace this
binary in place.

Works for both PATH installs and go install: the official release archive
(with embedded UI) is always downloaded and written over the current
executable.

  pine upgrade           # upgrade if a newer release exists
  pine upgrade --check   # print current vs latest; exit 1 if update available
  pine upgrade --force   # reinstall latest even when versions match

Optional GITHUB_TOKEN or GH_TOKEN raises API rate limits.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Minute)
			defer cancel()

			opt := selfupdate.Options{
				CurrentVersion: version,
				Force:          force,
				Client:         upgradeNewClient(version),
			}

			if check {
				res, err := selfupdate.Check(ctx, opt)
				if err != nil {
					return err
				}
				printUpgradeStatus(cmd, res)
				if res.NeedsUpdate {
					return errUpdateAvailable
				}
				return nil
			}

			res, err := selfupdate.Upgrade(ctx, opt)
			if err != nil {
				return err
			}
			printUpgradeStatus(cmd, res)
			if res.Updated {
				fmt.Fprintf(cmd.OutOrStdout(), "Updated pine to %s\n", res.Latest)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Already up to date (%s)\n", res.Current)
			return nil
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "only check for a newer release (exit 1 if available)")
	cmd.Flags().BoolVar(&force, "force", false, "reinstall latest even if versions match")
	cmd.MarkFlagsMutuallyExclusive("check", "force")
	return cmd
}

func printUpgradeStatus(cmd *cobra.Command, res *selfupdate.Result) {
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "current: %s\n", res.Current)
	fmt.Fprintf(w, "latest:  %s\n", res.Latest)
	if res.InstallHint != "" {
		fmt.Fprintf(w, "install: %s\n", res.InstallHint)
	}
	if res.NeedsUpdate && !res.Updated {
		fmt.Fprintf(w, "status:  update available (%s)\n", res.Asset)
	} else if res.Updated {
		fmt.Fprintf(w, "status:  updated\n")
	} else {
		fmt.Fprintf(w, "status:  up to date\n")
	}
}
