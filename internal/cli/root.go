// Package cli implements Pine's cobra command tree: init, serve, the Beads-style
// ticket commands (list/show/create/update/close/dep/ready), and the AI helpers
// (context/prompt/export/doctor/optimize).
package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/underworld14/pine/internal/store"
)

// version is set by Execute from the main package's build-time value.
var version = "dev"

// flagDir is the working directory pine treats as its starting point (like
// git's -C). Commands locate .pine by walking up from here.
var flagDir string

// Execute builds and runs the root command, exiting non-zero on error.
func Execute(v string) {
	version = v
	root := newRootCmd()
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "pine: "+err.Error())
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "pine",
		Short:         "Git-native local workspace for AI-assisted development",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVarP(&flagDir, "dir", "C", ".", "run as if pine was started in this directory")

	// Commands are registered per milestone as they are implemented.
	root.AddCommand(
		newInitCmd(),
		newServeCmd(),
		newOpenCmd(),
		newListCmd(),
		newShowCmd(),
		newCreateCmd(),
		newUpdateCmd(),
		newCloseCmd(),
		newDepCmd(),
		newReadyCmd(),
		newContextCmd(),
		newPromptCmd(),
		newExportCmd(),
		newDoctorCmd(),
		newOptimizeCmd(),
		newSetupCmd(),
	)
	return root
}

// findPineDir walks up from start looking for a .pine directory.
func findPineDir(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		p := filepath.Join(dir, ".pine")
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			return p, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("no .pine directory found here or above; run 'pine init' first")
		}
		dir = parent
	}
}

// openStore locates and opens the store for the current --dir.
func openStore() (*store.Store, error) {
	pineDir, err := findPineDir(flagDir)
	if err != nil {
		return nil, err
	}
	return store.Open(pineDir)
}

// repoRoot walks up from start to find a directory containing .git (a directory
// or a file, as in linked worktrees). It returns the start directory and false
// when none is found.
func repoRoot(start string) (string, bool) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return start, false
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return start, false
		}
		dir = parent
	}
}
