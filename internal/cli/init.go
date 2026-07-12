package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/underworld14/pine/internal/config"
	"github.com/underworld14/pine/internal/contextgen"
	"github.com/underworld14/pine/internal/memory"
	"github.com/underworld14/pine/internal/setup"
)

// Default template and prompt files written by pine init. The fix prompt reuses
// the same default the AI commands fall back to, so they never drift.
const (
	tmplBug     = "# Description\n\n# Steps to Reproduce\n\n# Expected\n\n# Actual\n\n# Acceptance Criteria\n- [ ] Define acceptance criteria\n\n# Related Files\n\n# Attachments\n"
	tmplFeature = "# Description\n\n# Acceptance Criteria\n- [ ] Define acceptance criteria\n\n# Implementation Plan\n\n# Notes\n\n# Related Files\n\n# Attachments\n"
	tmplEpic    = "# Description\n\n# Goals\n\n# Child Tickets\n"

	promptFix = contextgen.DefaultFixTemplate
)

func newInitCmd() *cobra.Command {
	var skipAgents bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create a .pine workspace in this repository",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runInit(cmd, skipAgents)
		},
	}
	cmd.Flags().BoolVar(&skipAgents, "skip-agents", false, "skip the coding-agent setup wizard")
	return cmd
}

func runInit(cmd *cobra.Command, skipAgents bool) error {
	w := cmd.OutOrStdout()
	base, err := filepath.Abs(flagDir)
	if err != nil {
		return err
	}
	root, isRepo := repoRoot(base)
	pineDir := filepath.Join(root, ".pine")
	projectName := filepath.Base(root)

	for _, d := range []string{"", "tickets", "attachments", "templates", "prompts", "learnings", "memory"} {
		if err := os.MkdirAll(filepath.Join(pineDir, d), 0o755); err != nil {
			return err
		}
	}

	cfgBytes, err := config.Default(projectName).Bytes()
	if err != nil {
		return err
	}
	boardBytes, err := config.DefaultBoard().Bytes()
	if err != nil {
		return err
	}

	files := []struct {
		path    string
		content []byte
	}{
		{filepath.Join(pineDir, "config.json"), cfgBytes},
		{filepath.Join(pineDir, "board.json"), boardBytes},
		{filepath.Join(pineDir, "templates", "bug.md"), []byte(tmplBug)},
		{filepath.Join(pineDir, "templates", "feature.md"), []byte(tmplFeature)},
		{filepath.Join(pineDir, "templates", "epic.md"), []byte(tmplEpic)},
		{filepath.Join(pineDir, "prompts", "fix.md"), []byte(promptFix)},
		{filepath.Join(pineDir, "MEMORY.md"), []byte(memory.DefaultMEMORY)},
	}
	createdAny := false
	for _, f := range files {
		status, err := writeIfAbsent(f.path, f.content)
		if err != nil {
			return err
		}
		if status == "created" {
			createdAny = true
		}
		rel, _ := filepath.Rel(root, f.path)
		fmt.Fprintf(w, "  %-8s %s\n", status, rel)
	}

	fmt.Fprintf(w, "\nPine workspace ready at %s\n", mustRel(base, pineDir))
	if !isRepo {
		fmt.Fprintln(w, "warning: not inside a git repository — git features will be disabled")
	} else if ignored, rule := pineIgnored(root); ignored {
		fmt.Fprintf(w, "warning: .pine appears to be gitignored (%q); Pine data is meant to be committed\n", rule)
	}
	fmt.Fprintln(w, "\nTickets are committed with your code, so they're branch-scoped — see \"Pine & git branches\" in the README.")

	if !skipAgents && createdAny {
		fmt.Fprintln(w)
		if setup.IsInteractive(cmd.InOrStdin()) {
			if err := runAgentWizard(cmd, false); err != nil {
				fmt.Fprintf(w, "warning: agent setup skipped (%v). Run 'pine setup agent' later.\n", err)
			}
		} else {
			fmt.Fprintln(w, "Skipped agent setup (non-interactive). Run 'pine setup agent' to configure coding agents.")
		}
	} else if !skipAgents && !createdAny {
		fmt.Fprintln(w, "\nSkipped agent setup (workspace already exists). Run 'pine setup agent' to configure coding agents.")
	}

	fmt.Fprintln(w, "\nNext: 'pine create --type bug --title \"...\"' or 'pine open'")
	return nil
}

// writeIfAbsent writes content only when the file does not already exist,
// reporting "created" or "exists".
func writeIfAbsent(path string, content []byte) (string, error) {
	if _, err := os.Stat(path); err == nil {
		return "exists", nil
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return "", err
	}
	return "created", nil
}

// pineIgnored reports whether a .gitignore rule at the repo root ignores .pine.
func pineIgnored(root string) (bool, string) {
	f, err := os.Open(filepath.Join(root, ".gitignore"))
	if err != nil {
		return false, ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		trimmed := strings.TrimSuffix(strings.TrimPrefix(line, "/"), "/")
		if trimmed == ".pine" {
			return true, line
		}
	}
	return false, ""
}

func mustRel(base, target string) string {
	if rel, err := filepath.Rel(base, target); err == nil {
		return rel
	}
	return target
}
