package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/underworld14/pine/internal/setup"
)

func newAgentRunner(cmd *cobra.Command) (setup.Runner, error) {
	root, err := setup.RepoRoot(flagDir)
	if err != nil {
		return setup.Runner{}, err
	}
	return setup.Runner{
		Root:    root,
		Version: version,
		Opts:    setupOpts(),
		Out:     cmd.OutOrStdout(),
		In:      cmd.InOrStdin(),
	}, nil
}

// runAgentWizard interactively installs agent instruction files.
func runAgentWizard(cmd *cobra.Command, yes bool) error {
	runner, err := newAgentRunner(cmd)
	if err != nil {
		return err
	}
	var recipes []setup.Recipe
	if yes {
		recipes = setup.AllRecipes
	} else {
		recipes, err = runner.Wizard(setup.HasPine(flagDir))
		if err != nil {
			return err
		}
	}
	fmt.Fprintln(runner.Out, "Installing Pine agent instructions:")
	return runner.Install(recipes)
}
