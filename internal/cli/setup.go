package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/underworld14/pine/internal/setup"
)

func newSetupCmd() *cobra.Command {
	var (
		list   bool
		check  bool
		remove bool
		printT bool
	)
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Manage coding agent integrations (Codex, Claude Code, Gemini, Cursor)",
		Long:  "Configure coding agents to use Pine. Run 'pine setup agent' for the interactive wizard.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if list || check || remove || printT {
				return runSetup(cmd, setup.AllRecipes, setupFlags{
					list: list, check: check, remove: remove, print: printT,
				})
			}
			return fmt.Errorf("use 'pine setup agent' to configure coding agents\n\nTry 'pine setup --help' for other options")
		},
	}
	f := cmd.Flags()
	f.BoolVar(&list, "list", false, "list available agent integrations")
	f.BoolVar(&check, "check", false, "check if integrations are installed and current")
	f.BoolVar(&remove, "remove", false, "remove pine sections from agent files")
	f.BoolVar(&printT, "print", false, "print rendered template to stdout")

	cmd.AddCommand(newSetupAgentCmd())
	cmd.AddCommand(newSetupMergeCmd())
	for _, recipe := range setup.AllRecipes {
		cmd.AddCommand(newSetupRecipeCmd(recipe))
	}
	return cmd
}

func newSetupAgentCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Interactive wizard to choose coding agent integrations",
		Long:  "Install or refresh agent bundles (instructions + skills + hooks). Also runs automatically during 'pine init'.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !yes && !setup.IsInteractive(cmd.InOrStdin()) {
				return fmt.Errorf("non-interactive terminal: use 'pine setup agent -y' or 'pine setup agents|claude|gemini|cursor'")
			}
			return runAgentWizard(cmd, yes)
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "install all agents without prompting")
	return cmd
}

func newSetupRecipeCmd(recipe setup.Recipe) *cobra.Command {
	var (
		check  bool
		remove bool
		printT bool
	)
	info, _ := setup.Lookup(recipe)
	cmd := &cobra.Command{
		Use:   string(recipe),
		Short: fmt.Sprintf("Install or update %s (%s)", info.Label, info.Description),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSetup(cmd, []setup.Recipe{recipe}, setupFlags{
				check: check, remove: remove, print: printT,
			})
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "check installation status")
	cmd.Flags().BoolVar(&remove, "remove", false, "remove pine section")
	cmd.Flags().BoolVar(&printT, "print", false, "print rendered template")
	return cmd
}

type setupFlags struct {
	list, check, remove, print bool
}

func runSetup(cmd *cobra.Command, recipes []setup.Recipe, flags setupFlags) error {
	runner, err := newAgentRunner(cmd)
	if err != nil {
		return err
	}

	if flags.list {
		runner.List()
		return nil
	}
	if flags.print {
		return runner.Print(recipes)
	}
	if flags.check {
		return runner.Check(recipes)
	}
	if flags.remove {
		return runner.Remove(recipes)
	}
	fmt.Fprintln(runner.Out, "Installing Pine agent integrations:")
	return runner.Install(recipes)
}

func setupOpts() setup.RenderOptions {
	opts := setup.RenderOptions{}
	s, err := openStore()
	if err != nil {
		return opts
	}
	statuses := s.Board().Statuses()
	if len(statuses) > 0 {
		opts.BoardColumns = joinComma(statuses)
	}
	return opts
}

func joinComma(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	out := ss[0]
	for _, s := range ss[1:] {
		out += ", " + s
	}
	return out
}
