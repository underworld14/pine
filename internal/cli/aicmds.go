package cli

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/underworld14/pine/internal/contextgen"
	"github.com/underworld14/pine/internal/gitx"
	"github.com/underworld14/pine/internal/store"
	"github.com/underworld14/pine/internal/view"
)

func newContextCmd() *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Print an AI-ready project briefing (pipe into your agent)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			md := contextgen.Context(s, gitStatus(s), time.Now())
			return writeOut(out, md, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "write to a file instead of stdout")
	return cmd
}

func newPromptCmd() *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:   "prompt <ID>",
		Short: "Generate a fix-request prompt for a ticket",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			tmpl := ""
			if data, err := os.ReadFile(filepath.Join(s.Root(), "prompts", "fix.md")); err == nil {
				tmpl = string(data)
			}
			md, err := contextgen.Prompt(s, gitStatus(s), strings.ToUpper(args[0]), tmpl)
			if err != nil {
				return err
			}
			return writeOut(out, md, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "write to a file instead of stdout")
	return cmd
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
