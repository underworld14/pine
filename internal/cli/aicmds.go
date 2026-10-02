package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/underworld14/pine/internal/contextgen"
	"github.com/underworld14/pine/internal/gitx"
	"github.com/underworld14/pine/internal/store"
	"github.com/underworld14/pine/internal/view"
)

func newContextCmd() *cobra.Command {
	var out string
	var full bool
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Print a compact AI-ready project briefing (pipe into your agent)",
		Long: `Print an AI-ready project briefing (pipe into your agent).

The default output is compact — board counts, ready work, the tickets relevant
to your uncommitted changes and recent commits, memory, and conventions — so
done work no longer floods an agent's context window before it starts. Pass
--full for the exhaustive briefing (recent commits, every open ticket table,
acceptance criteria, epics, recently done, knowledge graph).`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			git := gitStatus(s)
			md := contextgen.ContextCompact(s, git, time.Now())
			if full {
				md = contextgen.Context(s, git, time.Now())
			}
			return writeOut(out, md, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "write to a file instead of stdout")
	cmd.Flags().BoolVar(&full, "full", false, "render the exhaustive briefing instead of the compact one")
	return cmd
}

func newPromptCmd() *cobra.Command {
	var out, template string
	cmd := &cobra.Command{
		Use:   "prompt <ID>",
		Short: "Generate a fix or implement prompt for a ticket",
		Long: `Generate an agent-ready prompt for one ticket.

The template is chosen by ticket type: bug (and epic) tickets render
prompts/fix.md, every other type (feature, chore, …) renders
prompts/implement.md — falling back to fix.md in repos initialized before
implement.md existed. --template <name> renders prompts/<name>.md explicitly.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			id := normalizeID(args[0])
			name, err := promptTemplateName(s, id, template)
			if err != nil {
				return err
			}
			tmpl := ""
			if data, err := os.ReadFile(filepath.Join(s.Root(), "prompts", name+".md")); err == nil {
				tmpl = string(data)
			}
			md, err := contextgen.Prompt(s, gitStatus(s), id, tmpl)
			if err != nil {
				return err
			}
			return writeOut(out, md, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&template, "template", "", "prompt template name (prompts/<name>.md) instead of the type default")
	cmd.Flags().StringVar(&out, "out", "", "write to a file instead of stdout")
	return cmd
}

// promptTemplateName resolves which prompts/<name>.md to render for a ticket:
// an explicit --template wins (erroring when the file is missing), otherwise
// BUG/EPIC → fix, everything else → implement with fallback to fix for repos
// that predate implement.md.
func promptTemplateName(s *store.Store, id, explicit string) (string, error) {
	promptsDir := filepath.Join(s.Root(), "prompts")
	if explicit != "" {
		if _, err := os.Stat(filepath.Join(promptsDir, explicit+".md")); err != nil {
			return "", fmt.Errorf("prompt template prompts/%s.md not found (create it or pick another name)", explicit)
		}
		return explicit, nil
	}
	name := "implement"
	t, err := s.Get(id)
	if err != nil {
		return "", err
	}
	if p := t.Prefix(); p == "BUG" || p == "EPIC" {
		name = "fix"
	}
	if _, err := os.Stat(filepath.Join(promptsDir, name+".md")); err == nil {
		return name, nil
	}
	// Missing implement.md (old repo) or missing fix.md (Prompt falls back to
	// its built-in default) — either way fix.md is the fallback.
	return "fix", nil
}

func newExportCmd() *cobra.Command {
	var (
		format string
		out    string
	)
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export all tickets as markdown or JSON",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			var data string
			switch format {
			case "json":
				b, err := json.MarshalIndent(view.BuildAll(s, true), "", "  ")
				if err != nil {
					return err
				}
				data = string(b) + "\n"
			default:
				data = contextgen.ExportMarkdown(s)
			}
			return writeOut(out, data, cmd.OutOrStdout())
		},
	}
	f := cmd.Flags()
	f.StringVar(&format, "format", "md", "output format: md | json")
	f.StringVar(&out, "out", "", "write to a file instead of stdout")
	return cmd
}

// gitStatus takes a bounded git snapshot for the repo containing the store.
func gitStatus(s *store.Store) gitx.Status {
	client := gitx.New(filepath.Dir(s.Root()))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return client.Snapshot(ctx, 10)
}

func writeOut(outPath, content string, w io.Writer) error {
	if outPath == "" {
		_, err := io.WriteString(w, content)
		return err
	}
	return os.WriteFile(outPath, []byte(content), 0o644)
}
