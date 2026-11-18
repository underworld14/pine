package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/underworld14/pine/internal/gitx"
	"github.com/underworld14/pine/internal/store"
	"github.com/underworld14/pine/internal/ticket"
)

// pineChange is one file under .pine/ that differs between a git ref and the
// working tree. Ticket files also carry the parsed id/title/status, and the
// status the ticket had at the ref. The JSON form has no timestamps, so it is
// byte-stable for a given tree and a gate can snapshot it and diff.
type pineChange struct {
	Path         string `json:"path"`
	Change       string `json:"change"` // added | modified | deleted
	Hash         string `json:"hash,omitempty"`
	ID           string `json:"id,omitempty"`
	Title        string `json:"title,omitempty"`
	Status       string `json:"status,omitempty"`
	StatusBefore string `json:"statusBefore,omitempty"`

	now, before *ticket.Ticket // parsed copies for callers (release-notes)
}

var changeNames = map[string]string{"A": "added", "M": "modified", "D": "deleted"}

// ticketIDOf returns the ticket id when path is <pineName>/tickets/<ID>.md.
func ticketIDOf(pineName, path string) (string, bool) {
	rest, ok := strings.CutPrefix(path, pineName+"/tickets/")
	if !ok || strings.Contains(rest, "/") {
		return "", false
	}
	id, ok := strings.CutSuffix(rest, ".md")
	return id, ok && ticket.ValidID(id)
}

// changedPineFiles lists the files under pineDir that differ between since and
// the working tree (committed since, staged, unstaged, untracked-not-ignored),
// enriching ticket files with their status at the ref and now.
func changedPineFiles(ctx context.Context, client gitx.Client, pineDir, since string) ([]pineChange, error) {
	sha, _, ok := client.ResolveCommit(ctx, since)
	if !ok {
		if !client.IsRepo(ctx) {
			return nil, errors.New("not a git repository; this command needs git history")
		}
		return nil, fmt.Errorf("unknown ref %q", since)
	}
	pineName, root := filepath.Base(pineDir), filepath.Dir(pineDir)
	files := client.ChangedFiles(ctx, sha, pineName)
	out := make([]pineChange, 0, len(files))
	for _, f := range files {
		c := pineChange{Path: f.Path, Change: changeNames[f.Code]}
		id, isTicket := ticketIDOf(pineName, f.Path)
		if f.Code != "D" {
			if data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f.Path))); err == nil {
				sum := sha256.Sum256(data)
				c.Hash = hex.EncodeToString(sum[:])
				if isTicket {
					c.now = ticket.Parse(id, data)
				}
			}
		}
		if isTicket && f.Code != "A" {
			if old, ok := client.ShowFile(ctx, sha, "./"+f.Path); ok {
				c.before = ticket.Parse(id, old)
			}
		}
		if isTicket {
			c.ID = id
			if c.now != nil {
				c.Title, c.Status = c.now.Title, c.now.Status
			}
			if c.before != nil {
				c.StatusBefore = c.before.Status
				if c.now == nil {
					c.Title = c.before.Title
				}
			}
		}
		out = append(out, c)
	}
	return out, nil
}

// --- changed ---

func newChangedCmd() *cobra.Command {
	var since string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "changed",
		Short: "List files under .pine/ changed since a git ref (for gates and scripts)",
		Long: `List every file under .pine/ that differs between a git ref and the working
tree: committed since the ref, staged, unstaged, and untracked files that are
not ignored. Ticket files also show the ticket's id, title, and status
(before → after).

--json prints [{path, change, hash, id, title, status, statusBefore}] sorted by
path with no timestamps, so the output is byte-stable. hash is the sha256 of
the working-tree file (the value pine show --json reports for a ticket).

A gate that must not fail on pre-existing .pine/ changes can snapshot the
output before delegating work and compare afterwards:

  pine changed --json > "$SNAP/pine.json"
  pine changed --json | diff -q - "$SNAP/pine.json"`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			changes, err := changedPineFiles(ctx, gitx.New(filepath.Dir(s.Root())), s.Root(), since)
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), changes)
			}
			if len(changes) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No changes under .pine/ since %s.\n", since)
				return nil
			}
			return writeChangedText(cmd.OutOrStdout(), changes)
		},
	}
	f := cmd.Flags()
	f.StringVar(&since, "since", "HEAD", "git ref (commit, tag, branch) to compare the working tree against")
	f.BoolVar(&asJSON, "json", false, "output JSON")
	return cmd
}

func writeChangedText(w io.Writer, changes []pineChange) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, c := range changes {
		code := strings.ToUpper(c.Change[:1])
		if c.ID == "" {
			fmt.Fprintf(tw, "%s\t%s\n", code, c.Path)
			continue
		}
		var state string
		switch {
		case c.Change == "added":
			state = "new · " + c.Status
		case c.Change == "deleted":
			state = "was " + c.StatusBefore
		case c.StatusBefore != c.Status:
			state = c.StatusBefore + " → " + c.Status
		default:
			state = c.Status
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", code, c.Path, c.ID, state, c.Title)
	}
	return tw.Flush()
}

// --- release-notes ---

type releaseCommit struct {
	Hash    string `json:"hash"`
	Subject string `json:"subject"`
}

type releaseItem struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Title    string          `json:"title"`
	Priority string          `json:"priority"`
	Labels   []string        `json:"labels"`
	Parent   string          `json:"parent,omitempty"`
	Commits  []releaseCommit `json:"commits"`

	updated time.Time
}

// idMention finds ticket-id-shaped tokens (PREFIX-suffix). Matching whole
// tokens means FEAT-0020 never counts as a mention of FEAT-002.
var idMention = regexp.MustCompile(`(?i)\b[a-z][a-z0-9]*-[a-z0-9]+\b`)

// mentionedIDs returns the distinct lowercased id-shaped tokens in msg.
func mentionedIDs(msg string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range idMention.FindAllString(msg, -1) {
		if id := strings.ToLower(m); !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// plural turns a type name into a section heading ("Story" → "Stories").
func plural(name string) string {
	n := len(name)
	switch {
	case n > 1 && name[n-1] == 'y' && !strings.ContainsRune("aeiou", rune(name[n-2])):
		return name[:n-1] + "ies"
	case strings.HasSuffix(name, "s"), strings.HasSuffix(name, "x"),
		strings.HasSuffix(name, "ch"), strings.HasSuffix(name, "sh"):
		return name + "es"
	}
	return name + "s"
}

func newReleaseNotesCmd() *cobra.Command {
	var since, groupBy string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "release-notes",
		Short: "Markdown release notes from tickets closed since a git ref",
		Long: `Generate release notes from the tickets that became done since a git ref
(a tag, branch, or commit): done in the working tree now, and not done at the
ref (or created done since). Output is Markdown ready for
gh release create --notes-file.

Each ticket links the commits in <ref>..HEAD whose message mentions its id.
Commits that only touch .pine/ (status moves, hand-offs, review verdicts) are
skipped, and so are merge commits. Tickets are grouped by type (config order)
or, with --group-by label, by their first label ("Other" when none); within a
group the highest priority comes first.

--json prints [{id, type, title, priority, labels, parent, commits}] in the
same order. Requires tickets tracked in git (config sync.tickets).`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if groupBy != "type" && groupBy != "label" {
				return fmt.Errorf("--group-by must be type or label, got %q", groupBy)
			}
			s, err := openStore()
			if err != nil {
				return err
			}
			if !s.Config().Sync.Tickets {
				return errors.New("tickets are not tracked in git (sync.tickets is off), so there is nothing to compare against a ref")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			client := gitx.New(filepath.Dir(s.Root()))
			items, err := closedSince(ctx, client, s, since)
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), items)
			}
			if len(items) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No tickets closed since %s.\n", since)
				return nil
			}
			writeReleaseNotes(cmd.OutOrStdout(), s, items, groupBy)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&since, "since", "", "git ref (tag, branch, commit) to compare against (required)")
	f.StringVar(&groupBy, "group-by", "type", "group tickets by: type | label")
	f.BoolVar(&asJSON, "json", false, "output JSON")
	_ = cmd.MarkFlagRequired("since")
	return cmd
}

// closedSince returns the tickets newly done since ref, each with the code
// commits that mention it (oldest first), in a deterministic order.
func closedSince(ctx context.Context, client gitx.Client, s *store.Store, since string) ([]releaseItem, error) {
	changes, err := changedPineFiles(ctx, client, s.Root(), since)
	if err != nil {
		return nil, err
	}
	var items []releaseItem
	for _, c := range changes {
		if c.now == nil || c.now.Status != ticket.StatusDone {
			continue
		}
		if c.before != nil && c.before.Status == ticket.StatusDone {
			continue
		}
		items = append(items, releaseItem{
			ID: c.ID, Type: c.now.Prefix(), Title: c.now.Title, Priority: c.now.Priority,
			Labels: append([]string{}, c.now.Labels...), Parent: c.now.Parent,
			Commits: []releaseCommit{}, updated: c.now.Updated,
		})
	}
	if len(items) == 0 {
		return []releaseItem{}, nil
	}

	// RangeLog is newest first; walk it backwards so links read oldest → newest.
	commits := client.RangeLog(ctx, since, "HEAD", filepath.Base(s.Root()))
	byID := map[string][]releaseCommit{}
	for i := len(commits) - 1; i >= 0; i-- {
		for _, id := range mentionedIDs(commits[i].Message) {
			byID[id] = append(byID[id], releaseCommit{Hash: commits[i].Hash, Subject: commits[i].Subject})
		}
	}
	for i := range items {
		if cs := byID[strings.ToLower(items[i].ID)]; cs != nil {
			items[i].Commits = cs
		}
	}
	return items, nil
}

// releaseGroup names the section a ticket belongs to and its sort rank.
func releaseGroup(s *store.Store, it releaseItem, groupBy string) (string, int) {
	cfg := s.Config()
	if groupBy == "label" {
		if len(it.Labels) > 0 {
			return it.Labels[0], 0
		}
		return "Other", 1
	}
	for i, t := range cfg.Types {
		if t.Prefix == it.Type {
			return plural(t.Name), i
		}
	}
	return "Other", len(cfg.Types)
}

func writeReleaseNotes(w io.Writer, s *store.Store, items []releaseItem, groupBy string) {
	prios := s.Config().Priorities
	type keyed struct {
		it    releaseItem
		group string
		rank  int
	}
	ks := make([]keyed, len(items))
	for i, it := range items {
		g, r := releaseGroup(s, it, groupBy)
		ks[i] = keyed{it, g, r}
	}
	sort.SliceStable(ks, func(i, j int) bool {
		a, b := ks[i], ks[j]
		if a.rank != b.rank {
			return a.rank < b.rank
		}
		if a.group != b.group {
			return strings.ToLower(a.group) < strings.ToLower(b.group)
		}
		if pa, pb := ticket.PriorityRank(a.it.Priority, prios), ticket.PriorityRank(b.it.Priority, prios); pa != pb {
			return pa > pb
		}
		if !a.it.updated.Equal(b.it.updated) {
			return a.it.updated.After(b.it.updated)
		}
		return a.it.ID < b.it.ID
	})
	for i, k := range ks {
		if i == 0 || k.group != ks[i-1].group {
			if i > 0 {
				fmt.Fprintln(w)
			}
			fmt.Fprintf(w, "## %s\n\n", k.group)
		}
		fmt.Fprintf(w, "- %s (%s)", k.it.Title, k.it.ID)
		for j, c := range k.it.Commits {
			sep := ", "
			if j == 0 {
				sep = " · "
			}
			fmt.Fprintf(w, "%s`%s`", sep, c.Hash)
		}
		fmt.Fprintln(w)
	}
}
