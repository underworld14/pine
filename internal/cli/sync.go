package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/underworld14/pine/internal/config"
	"github.com/underworld14/pine/internal/setup"
	"github.com/underworld14/pine/internal/syncignore"
	"github.com/underworld14/pine/internal/tui"
)

// applySyncPrefs writes .pine/.gitignore and updates config.json sync fields.
func applySyncPrefs(pineDir string, prefs syncignore.Prefs) error {
	if err := syncignore.WritePineGitignore(pineDir, prefs); err != nil {
		return err
	}
	cfgPath := filepath.Join(pineDir, "config.json")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	cfg.Sync = config.Sync{Tickets: prefs.Tickets, Attachments: prefs.Attachments}
	data, err := cfg.Bytes()
	if err != nil {
		return err
	}
	return os.WriteFile(cfgPath, data, 0o644)
}

// resolveSyncPrefs returns sync preferences from an interactive checklist or the
// provided seed (CLI flag defaults).
func resolveSyncPrefs(cmd *cobra.Command, seed syncignore.Prefs) (syncignore.Prefs, error) {
	w := cmd.OutOrStdout()
	fmt.Fprintln(w, "Project memory (MEMORY.md / memory/) is always committed for cross-machine use.")
	if !setup.IsInteractive(cmd.InOrStdin()) {
		return seed, nil
	}
	selected, err := tui.MultiSelectAllowEmpty(
		"Pine sync — what should git track under .pine/?",
		[]tui.Choice{
			{
				Key:         "tickets",
				Label:       "Tickets",
				Description: ".pine/tickets/ — branch-scoped with your code",
				Selected:    seed.Tickets,
			},
			{
				Key:         "attachments",
				Label:       "Attachments",
				Description: ".pine/attachments/ — keep local (default)",
				Selected:    seed.Attachments,
			},
		},
	)
	if err != nil {
		return seed, err
	}
	prefs := syncignore.Prefs{}
	for _, k := range selected {
		switch k {
		case "tickets":
			prefs.Tickets = true
		case "attachments":
			prefs.Attachments = true
		}
	}
	return prefs, nil
}

func printSyncSummary(cmd *cobra.Command, prefs syncignore.Prefs) {
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "  sync: tickets=%s attachments=%s\n", onOff(prefs.Tickets), onOff(prefs.Attachments))
}

func onOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

func syncPrefsFromFlags(tickets, noTickets, attachments, noAttachments bool) syncignore.Prefs {
	prefs := syncignore.Default()
	prefs.Tickets = tickets
	prefs.Attachments = attachments
	if noTickets {
		prefs.Tickets = false
	}
	if noAttachments {
		prefs.Attachments = false
	}
	return prefs
}

func newSetupSyncCmd() *cobra.Command {
	var (
		syncTickets       = true
		syncAttachments   = false
		noSyncTickets     bool
		noSyncAttachments bool
	)
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Choose what git tracks under .pine/ (tickets, attachments)",
		Long: `Configure whether .pine/tickets and .pine/attachments are committed.

Writes a managed block in .pine/.gitignore and updates config.json "sync".
Project memory (MEMORY.md / memory/) is always tracked.

Interactive by default; use --sync-tickets / --sync-attachments (and --no-*) for scripts.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSetupSync(cmd, syncPrefsFromFlags(syncTickets, noSyncTickets, syncAttachments, noSyncAttachments))
		},
	}
	f := cmd.Flags()
	f.BoolVar(&syncTickets, "sync-tickets", true, "track .pine/tickets in git")
	f.BoolVar(&noSyncTickets, "no-sync-tickets", false, "keep .pine/tickets local (gitignored)")
	f.BoolVar(&syncAttachments, "sync-attachments", false, "track .pine/attachments in git")
	f.BoolVar(&noSyncAttachments, "no-sync-attachments", false, "keep .pine/attachments local (gitignored)")
	return cmd
}

func runSetupSync(cmd *cobra.Command, seed syncignore.Prefs) error {
	root, isRepo := repoRoot(flagDir)
	if !isRepo {
		// Still allow configuring a .pine dir outside a repo.
		abs, err := filepath.Abs(flagDir)
		if err != nil {
			return err
		}
		root = abs
	}
	pineDir := filepath.Join(root, ".pine")
	if _, err := os.Stat(filepath.Join(pineDir, "config.json")); err != nil {
		return fmt.Errorf("no Pine workspace at %s — run 'pine init' first", pineDir)
	}

	// Non-interactive: apply flags directly. Interactive: checklist (seeded by flags).
	prefs := seed
	if setup.IsInteractive(cmd.InOrStdin()) {
		var err error
		prefs, err = resolveSyncPrefs(cmd, seed)
		if err != nil {
			return err
		}
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "Project memory (MEMORY.md / memory/) is always committed for cross-machine use.")
	}

	if err := applySyncPrefs(pineDir, prefs); err != nil {
		return err
	}
	printSyncSummary(cmd, prefs)
	fmt.Fprintf(cmd.OutOrStdout(), "Updated %s\n", filepath.Join(pineDir, ".gitignore"))
	return nil
}

func syncMessage(prefs syncignore.Prefs) string {
	var b strings.Builder
	if prefs.Tickets {
		b.WriteString("Tickets are committed with your code, so they're branch-scoped — see \"Pine & git branches\" in the README.")
	} else {
		b.WriteString("Tickets are kept local (gitignored under .pine/). They are not branch-scoped via git — change this later with 'pine setup sync'.")
	}
	if !prefs.Attachments {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("Attachments stay local by default (see .pine/.gitignore).")
	}
	return b.String()
}
