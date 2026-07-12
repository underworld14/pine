package cli

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/underworld14/pine/internal/frontmatter"
	"github.com/underworld14/pine/internal/store"
	"github.com/underworld14/pine/internal/ticket"
)

// ghLabel and ghIssue mirror the subset of `gh issue list --json` output Pine uses.
type ghLabel struct {
	Name string `json:"name"`
}

type ghIssue struct {
	Number int       `json:"number"`
	Title  string    `json:"title"`
	Body   string    `json:"body"`
	URL    string    `json:"url"`
	Labels []ghLabel `json:"labels"`
}

// ghListIssues and ghCurrentRepo are indirected as package vars so tests can
// stub the GitHub calls without a real `gh` binary or network.
var ghListIssues = realGHListIssues
var ghCurrentRepo = realGHCurrentRepo

func realGHListIssues(repo, state string, limit int) ([]ghIssue, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return nil, fmt.Errorf("the GitHub CLI 'gh' is required: install it and run 'gh auth login' (https://cli.github.com)")
	}
	args := []string{"issue", "list", "--state", state, "--json", "number,title,body,url,labels", "--limit", strconv.Itoa(limit)}
	if repo != "" {
		args = append(args, "--repo", repo)
	}
	out, err := exec.Command("gh", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("gh issue list failed: %w", err)
	}
	var issues []ghIssue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("could not parse gh output: %w", err)
	}
	return issues, nil
}

func realGHCurrentRepo(dir string) (string, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return "", fmt.Errorf("the GitHub CLI 'gh' is required: install it and run 'gh auth login' (https://cli.github.com)")
	}
	out, err := exec.Command("gh", "repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner").Output()
	if err != nil {
		return "", fmt.Errorf("could not determine the current repo; pass <owner/repo> explicitly")
	}
	return strings.TrimSpace(string(out)), nil
}

func newImportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import tickets from an external tracker",
	}
	cmd.AddCommand(newImportGithubCmd(), newImportBeadsCmd())
	return cmd
}

func newImportGithubCmd() *cobra.Command {
	var (
		state, label, typeMapCSV string
		limit                    int
		dryRun                   bool
	)
	cmd := &cobra.Command{
		Use:   "github [owner/repo]",
		Short: "Import GitHub issues as Pine tickets (via the gh CLI)",
		Long: `Import GitHub issues as Pine tickets using your existing gh CLI auth.

Labels map to ticket types via --type-map (default: bug=BUG, enhancement=FEAT,
feature=FEAT, epic=EPIC); unmapped issues use the first configured type. Each
imported ticket records its source issue URL, so re-running skips issues already
imported. With no owner/repo, the current repository is used.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			repo := ""
			if len(args) == 1 {
				repo = args[0]
			} else {
				repo, err = ghCurrentRepo(flagDir)
				if err != nil {
					return err
				}
			}

			issues, err := ghListIssues(repo, state, limit)
			if err != nil {
				return err
			}
			typeMap := parseTypeMap(typeMapCSV)
			defaultType := "BUG"
			if types := s.Config().Types; len(types) > 0 {
				defaultType = types[0].Prefix
			}

			// URLs already imported (idempotency).
			imported := map[string]bool{}
			for _, t := range s.All() {
				if u := ticketGithubURL(t); u != "" {
					imported[u] = true
				}
			}

			out := cmd.OutOrStdout()
			created, skipped, failed := 0, 0, 0
			for _, iss := range issues {
				if label != "" && !issueHasLabel(iss, label) {
					continue
				}
				if imported[iss.URL] {
					skipped++
					continue
				}
				typ := mapIssueType(iss.Labels, typeMap, defaultType)
				if dryRun {
					fmt.Fprintf(out, "  would import #%d [%s] %s\n", iss.Number, typ, iss.Title)
					created++
					continue
				}
				t, err := s.Create(store.CreateReq{
					Type:   typ,
					Title:  iss.Title,
					Body:   iss.Body,
					Labels: labelNames(iss.Labels),
				})
				if err != nil {
					fmt.Fprintf(out, "  ! skipped #%d (%v)\n", iss.Number, err)
					failed++
					continue
				}
				if _, err := s.Update(t.ID, func(tt *ticket.Ticket) error {
					tt.Extra = append(tt.Extra, ticket.ExtraField{Key: "github", Node: frontmatter.Scalar(iss.URL)})
					return nil
				}); err != nil {
					fmt.Fprintf(out, "  ! %s created but could not record source URL (%v)\n", t.ID, err)
				}
				fmt.Fprintf(out, "  %s ← #%d %s\n", t.ID, iss.Number, iss.Title)
				created++
			}

			verb := "imported"
			if dryRun {
				verb = "would import"
			}
			fmt.Fprintf(out, "\n%s %d, skipped %d already-imported", verb, created, skipped)
			if failed > 0 {
				fmt.Fprintf(out, ", %d failed", failed)
			}
			fmt.Fprintln(out)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&state, "state", "open", "issue state: open | closed | all")
	f.StringVar(&label, "label", "", "only import issues carrying this label")
	f.StringVar(&typeMapCSV, "type-map", "", "label→type overrides, e.g. bug=BUG,enhancement=FEAT")
	f.IntVar(&limit, "limit", 100, "maximum number of issues to fetch")
	f.BoolVar(&dryRun, "dry-run", false, "show what would be imported without writing")
	return cmd
}

// ticketGithubURL returns the source GitHub URL recorded in a ticket's
// frontmatter, or "".
func ticketGithubURL(t *ticket.Ticket) string {
	for _, e := range t.Extra {
		if e.Key == "github" && e.Node != nil {
			return strings.TrimSpace(e.Node.Value)
		}
	}
	return ""
}

func issueHasLabel(iss ghIssue, want string) bool {
	want = strings.ToLower(want)
	for _, l := range iss.Labels {
		if strings.ToLower(l.Name) == want {
			return true
		}
	}
	return false
}

func labelNames(labels []ghLabel) []string {
	if len(labels) == 0 {
		return nil
	}
	out := make([]string, 0, len(labels))
	for _, l := range labels {
		if l.Name != "" {
			out = append(out, l.Name)
		}
	}
	return out
}

// mapIssueType picks a ticket type from the issue's labels via the override map,
// then the built-in defaults, falling back to def.
func mapIssueType(labels []ghLabel, overrides map[string]string, def string) string {
	builtin := map[string]string{"bug": "BUG", "enhancement": "FEAT", "feature": "FEAT", "epic": "EPIC"}
	for _, l := range labels {
		key := strings.ToLower(strings.TrimSpace(l.Name))
		if t, ok := overrides[key]; ok {
			return t
		}
	}
	for _, l := range labels {
		key := strings.ToLower(strings.TrimSpace(l.Name))
		if t, ok := builtin[key]; ok {
			return t
		}
	}
	return def
}

func parseTypeMap(csv string) map[string]string {
	out := map[string]string{}
	for _, pair := range strings.Split(csv, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		k, v, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		out[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
	}
	return out
}
