package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/underworld14/pine/internal/ticket"
)

// gitAttributesLine maps ticket files to Pine's merge driver.
const gitAttributesLine = "/.pine/tickets/*.md merge=pine"

// errMergeConflict signals git (via a non-zero exit) that the merge produced
// conflict markers needing human review.
var errMergeConflict = errors.New("merge conflict")

// newMergeDriverCmd is the hidden plumbing command git invokes as the "pine"
// merge driver: pine merge-driver %O %A %B %P (base, ours, theirs, path).
func newMergeDriverCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "merge-driver <base> <ours> <theirs> <path>",
		Short:         "Internal: git merge driver for Pine ticket files",
		Hidden:        true,
		Args:          cobra.ExactArgs(4),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			conflict, err := runMergeDriver(args[0], args[1], args[2], args[3])
			if err != nil {
				return err
			}
			if conflict {
				// Non-zero exit tells git the file is unmerged.
				return errMergeConflict
			}
			return nil
		},
	}
}

// runMergeDriver merges basePath/oursPath/theirsPath and writes the result to
// oursPath (git's %A). It returns whether the result needs review.
func runMergeDriver(basePath, oursPath, theirsPath, name string) (bool, error) {
	baseRaw, _ := os.ReadFile(basePath) // absent ancestor → empty
	oursRaw, err := os.ReadFile(oursPath)
	if err != nil {
		return false, err
	}
	theirsRaw, err := os.ReadFile(theirsPath)
	if err != nil {
		return false, err
	}

	id := strings.TrimSuffix(filepath.Base(name), ".md")
	ours := ticket.Parse(id, oursRaw)
	theirs := ticket.Parse(id, theirsRaw)

	// If either side is unparseable, Pine cannot field-merge safely — fall back
	// to standard whole-file conflict markers for a human to resolve.
	if ours.Degraded || theirs.Degraded {
		return true, os.WriteFile(oursPath, conflictMarkers(oursRaw, theirsRaw), 0o644)
	}

	var base *ticket.Ticket
	if b := ticket.Parse(id, baseRaw); !b.Degraded {
		base = b
	}

	merged, conflict := ticket.Merge3(base, ours, theirs)
	return conflict, os.WriteFile(oursPath, merged.Serialize(), 0o644)
}

func conflictMarkers(ours, theirs []byte) []byte {
	var b strings.Builder
	b.WriteString("<<<<<<< ours\n")
	b.Write(ours)
	if len(ours) > 0 && ours[len(ours)-1] != '\n' {
		b.WriteByte('\n')
	}
	b.WriteString("=======\n")
	b.Write(theirs)
	if len(theirs) > 0 && theirs[len(theirs)-1] != '\n' {
		b.WriteByte('\n')
	}
	b.WriteString(">>>>>>> theirs\n")
	return []byte(b.String())
}

// newSetupMergeCmd installs the Pine ticket merge driver into the current repo:
// a committed .gitattributes rule plus the local git config that points at it.
func newSetupMergeCmd() *cobra.Command {
	var remove bool
	cmd := &cobra.Command{
		Use:   "merge",
		Short: "Install the Pine ticket merge driver (git-native field-level merges)",
		Long: `Configure git to merge .pine/tickets/*.md with Pine's field-aware driver
instead of raw text diffs. This writes a .gitattributes rule (committed, shared
with your team) and a local git config entry (per-clone: each teammate must run
'pine setup merge' once after cloning).`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, ok := repoRoot(flagDir)
			if !ok {
				return fmt.Errorf("not inside a git repository")
			}
			out := cmd.OutOrStdout()
			if remove {
				return removeMergeDriver(cmd, root)
			}
			changed, err := ensureGitAttributes(root)
			if err != nil {
				return err
			}
			if changed {
				fmt.Fprintln(out, "  wrote .gitattributes rule for .pine/tickets/*.md")
			} else {
				fmt.Fprintln(out, "  .gitattributes already has the pine merge rule")
			}
			if err := gitConfig(root, "merge.pine.name", "Pine ticket merge"); err != nil {
				return err
			}
			if err := gitConfig(root, "merge.pine.driver", "pine merge-driver %O %A %B %P"); err != nil {
				return err
			}
			fmt.Fprintln(out, "  configured git merge.pine.driver")
			fmt.Fprintln(out, "\nDone. Teammates must run 'pine setup merge' once per clone (git config is not shared).")
			return nil
		},
	}
	cmd.Flags().BoolVar(&remove, "remove", false, "remove the pine merge driver configuration")
	return cmd
}

func removeMergeDriver(cmd *cobra.Command, root string) error {
	out := cmd.OutOrStdout()
	_ = exec.Command("git", "-C", root, "config", "--remove-section", "merge.pine").Run()
	path := filepath.Join(root, ".gitattributes")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintln(out, "  removed git merge.pine config")
			return nil
		}
		return err
	}
	var kept []string
	removed := false
	for _, ln := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(ln) == gitAttributesLine {
			removed = true
			continue
		}
		kept = append(kept, ln)
	}
	if removed {
		content := strings.TrimRight(strings.Join(kept, "\n"), "\n")
		if strings.TrimSpace(content) == "" {
			_ = os.Remove(path)
		} else if err := os.WriteFile(path, []byte(content+"\n"), 0o644); err != nil {
			return err
		}
		fmt.Fprintln(out, "  removed .gitattributes rule")
	}
	fmt.Fprintln(out, "  removed git merge.pine config")
	return nil
}

// ensureGitAttributes appends the pine merge rule to .gitattributes if absent.
// Returns whether the file was changed.
func ensureGitAttributes(root string) (bool, error) {
	path := filepath.Join(root, ".gitattributes")
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	for _, ln := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(ln) == gitAttributesLine {
			return false, nil
		}
	}
	content := string(data)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += gitAttributesLine + "\n"
	return true, os.WriteFile(path, []byte(content), 0o644)
}

func gitConfig(root, key, value string) error {
	cmd := exec.Command("git", "-C", root, "config", key, value)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git config %s: %v: %s", key, err, strings.TrimSpace(string(out)))
	}
	return nil
}
