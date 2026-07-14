package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/underworld14/pine/internal/memory"
	"github.com/underworld14/pine/internal/search"
	"github.com/underworld14/pine/internal/store"
)

// memStore is the memory store a learn command reads or writes: this
// repository's .pine, or the machine-wide ~/.pine reached with -g.
//
// It exists because the memory helpers only ever needed a directory, but took
// a *store.Store to get one — which is exactly what made -g impossible outside
// a pine repo.
type memStore struct {
	Dir    string // absolute path to the store directory
	Label  string // user-facing prefix: ".pine" | "~/.pine" | $PINE_HOME
	Global bool
}

// Name is the machine-readable store discriminator for --json.
func (m memStore) Name() string {
	if m.Global {
		return "global"
	}
	return "project"
}

func projectStore(s *store.Store) memStore {
	return memStore{Dir: s.Root(), Label: ".pine"}
}

// captureMemoryInsight routes a non-ticket insight into MEMORY.md or a topic file.
func captureMemoryInsight(cmd *cobra.Command, ms memStore, text string, cites []string, component, to, newTopic string, asJSON bool) error {
	if err := memory.EnsureLayout(ms.Dir); err != nil {
		return err
	}

	opts := memory.AppendOpts{Text: text, Cites: cites}

	if to != "" {
		kind, value, err := memory.ResolveTo(to)
		if err != nil {
			return err
		}
		return writeMemoryDest(cmd, ms, kind, value, opts, asJSON)
	}
	if newTopic != "" {
		return writeMemoryDest(cmd, ms, "new", memory.Slugify(newTopic), opts, asJSON)
	}

	if ms.Global {
		// Like `git config --global`, -g never asks where. Suggest is never
		// Confident against a store with no topics — the global store's normal
		// state — so routing -g through it would make `pine learn -g` fail on
		// first use. Global topics are also only name-listed in pine context,
		// so auto-filing a fact into one would hide it from agents.
		return writeMemoryDest(cmd, ms, "memory", memory.FileMEMORY, opts, asJSON)
	}

	recs, err := memory.Suggest(ms.Dir, memory.SuggestOpts{
		Text:      text,
		Cites:     cites,
		Component: component,
	})
	if err != nil {
		return err
	}

	if memory.Confident(recs) {
		kind, value, err := memory.ResolveTo(recs[0].Path)
		if err != nil {
			return err
		}
		return writeMemoryDest(cmd, ms, kind, value, opts, asJSON)
	}

	if asJSON {
		_ = writeJSON(cmd.OutOrStdout(), map[string]any{
			"error":           "ambiguous destination — pass --to or --new-topic",
			"recommendations": recs,
		})
		return fmt.Errorf("ambiguous learning destination")
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Ambiguous destination. Top recommendations:")
	printRecommendations(cmd, recs)
	fmt.Fprintln(cmd.OutOrStdout(), "\nRetry with --to MEMORY.md | --to memory/<topic>.md | --new-topic <slug>")
	return fmt.Errorf("ambiguous learning destination")
}

func writeMemoryDest(cmd *cobra.Command, ms memStore, kind, value string, opts memory.AppendOpts, asJSON bool) error {
	var dest string
	switch kind {
	case "memory":
		if err := memory.AppendMEMORY(ms.Dir, opts); err != nil {
			return err
		}
		dest = memory.FileMEMORY
	case "topic", "new":
		if err := memory.AppendTopic(ms.Dir, value, opts); err != nil {
			return err
		}
		dest = filepath.ToSlash(filepath.Join(memory.DirTopics, memory.Slugify(value)+".md"))
	default:
		return fmt.Errorf("unknown destination kind %q", kind)
	}
	if asJSON {
		return writeJSON(cmd.OutOrStdout(), map[string]any{
			"path":  dest,
			"kind":  kind,
			"text":  opts.Text,
			"store": ms.Name(),
		})
	}
	// ms.Label is ".pine" for a project store, so this stays byte-identical
	// to the message this command has always printed.
	fmt.Fprintf(cmd.OutOrStdout(), "Appended to %s/%s\n", ms.Label, dest)
	return nil
}

func printRecommendations(cmd *cobra.Command, recs []memory.Recommendation) {
	for i, r := range recs {
		fmt.Fprintf(cmd.OutOrStdout(), "  %d. %s  (score %.2f) — %s\n", i+1, r.Path, r.Score, r.Reason)
	}
}

func newLearnSuggestCmd() *cobra.Command {
	var citesCSV, component string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "suggest [text]",
		Short: "Recommend MEMORY.md / topic destinations for an insight (no write)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			recs, err := memory.Suggest(s.Root(), memory.SuggestOpts{
				Text:      args[0],
				Cites:     splitCSV(citesCSV),
				Component: component,
			})
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), map[string]any{"recommendations": recs})
			}
			if len(recs) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No recommendations.")
				return nil
			}
			printRecommendations(cmd, recs)
			if memory.Confident(recs) {
				fmt.Fprintf(cmd.OutOrStdout(), "\nAuto-append would choose: %s\n", recs[0].Path)
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "\nNot confident enough to auto-append — pass --to or --new-topic.")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&citesCSV, "cites", "", "comma-separated repo-relative file paths")
	cmd.Flags().StringVar(&component, "component", "", "component path hint")
	cmd.Flags().BoolVar(&asJSON, "json", false, "output JSON")
	return cmd
}

// tryShowMemory prints MEMORY.md or a topic file. Returns handled=true when id is a memory path.
func tryShowMemory(cmd *cobra.Command, ms memStore, id string, asJSON bool) (handled bool, err error) {
	lower := strings.ToLower(id)
	isMem := lower == "memory.md" || strings.HasPrefix(lower, "memory/") ||
		strings.EqualFold(id, memory.FileMEMORY) || strings.HasPrefix(id, ".pine/")
	if !isMem && !strings.HasPrefix(lower, "new:") {
		// bare slug with .md only if file exists
		if !strings.HasSuffix(lower, ".md") && !strings.Contains(id, "/") {
			path := memory.TopicPath(ms.Dir, id)
			if _, e := os.Stat(path); e != nil {
				return false, nil
			}
		} else if !strings.HasSuffix(lower, ".md") {
			return false, nil
		}
	}

	kind, value, err := memory.ResolveTo(id)
	if err != nil {
		return false, nil
	}
	pineDir := ms.Dir
	switch kind {
	case "memory":
		body, err := memory.ReadMEMORY(pineDir)
		if err != nil {
			return true, err
		}
		if strings.TrimSpace(body) == "" {
			return true, fmt.Errorf("MEMORY.md not found")
		}
		if asJSON {
			return true, writeJSON(cmd.OutOrStdout(), map[string]any{"path": memory.FileMEMORY, "body": body})
		}
		fmt.Fprint(cmd.OutOrStdout(), body)
		if !strings.HasSuffix(body, "\n") {
			fmt.Fprintln(cmd.OutOrStdout())
		}
		return true, nil
	case "topic", "new":
		path := memory.TopicPath(pineDir, value)
		data, err := os.ReadFile(path)
		if err != nil {
			return true, err
		}
		if asJSON {
			t, _ := memory.ReadTopic(pineDir, value)
			return true, writeJSON(cmd.OutOrStdout(), map[string]any{
				"path": t.RelPath, "slug": t.Slug, "title": t.Title, "body": string(data),
			})
		}
		fmt.Fprint(cmd.OutOrStdout(), string(data))
		return true, nil
	}
	return false, nil
}

func listMemoryEntries(cmd *cobra.Command, ms memStore) error {
	pineDir := ms.Dir
	// Deliberately does not ensure the layout: listing is a read, and against
	// the global store this would create ~/.pine and seed it with the *project*
	// text. ReadMEMORY and ListTopics both degrade cleanly when absent.
	header := "MEMORY / topics:"
	if ms.Global {
		header = "Global MEMORY / topics:"
	}
	fmt.Fprintln(cmd.OutOrStdout(), header)
	if body, err := memory.ReadMEMORY(pineDir); err == nil && strings.TrimSpace(body) != "" {
		lines := len(strings.Split(strings.TrimSpace(body), "\n"))
		fmt.Fprintf(cmd.OutOrStdout(), "  MEMORY.md\t(%d lines)\n", lines)
	}
	topics, err := memory.ListTopics(pineDir)
	if err != nil {
		return err
	}
	for _, t := range topics {
		fmt.Fprintf(cmd.OutOrStdout(), "  %s\t%s\n", t.RelPath, t.Title)
	}
	if len(topics) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "  (no topic files yet)")
	}
	fmt.Fprintln(cmd.OutOrStdout())
	return nil
}

// listMemoryJSON is the --json rendering of listMemoryEntries. It exists
// because the LRN listing it normally shares needs a *store.Store, which -g
// does not have.
func listMemoryJSON(cmd *cobra.Command, ms memStore) error {
	out := map[string]any{"store": ms.Name()}
	if body, err := memory.ReadMEMORY(ms.Dir); err == nil && strings.TrimSpace(body) != "" {
		out["memory"] = map[string]any{
			"path":  memory.FileMEMORY,
			"lines": len(strings.Split(strings.TrimSpace(body), "\n")),
		}
	}
	topics, err := memory.ListTopics(ms.Dir)
	if err != nil {
		return err
	}
	list := make([]map[string]any, 0, len(topics))
	for _, t := range topics {
		list = append(list, map[string]any{"path": t.RelPath, "slug": t.Slug, "title": t.Title})
	}
	out["topics"] = list
	return writeJSON(cmd.OutOrStdout(), out)
}

func searchMemoryInto(idx *search.Index, pineDir string) {
	if mem, err := memory.ReadMEMORY(pineDir); err == nil && strings.TrimSpace(mem) != "" {
		idx.Upsert(search.Doc{
			ID:    memory.FileMEMORY,
			Title: "Project memory",
			Body:  mem,
			Kind:  search.KindMemory,
		})
	}
	topics, _ := memory.ListTopics(pineDir)
	for _, t := range topics {
		idx.Upsert(search.Doc{
			ID:    t.RelPath,
			Title: t.Title,
			Body:  t.Body,
			Kind:  search.KindMemory,
		})
	}
}
