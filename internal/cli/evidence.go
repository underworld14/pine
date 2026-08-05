package cli

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/underworld14/pine/internal/gitx"
	"github.com/underworld14/pine/internal/store"
)

// workEvidence computes a markdown "## Work Evidence" block summarizing the
// file changes attributable to a ticket: commits that reference or touch it,
// plus a `git diff --stat` from the commit at the ticket's creation time to the
// working tree. Best-effort: degrades to a note when git is absent or the
// ticket's creation time is unknown.
//
// The git work runs OUTSIDE the store lock (this is called before s.Update),
// so the only locked step is the body mutation in the caller.
func workEvidence(s *store.Store, id string) string {
	client := gitx.New(filepath.Dir(s.Root()))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var b strings.Builder
	b.WriteString("## Work Evidence\n\n")
	b.WriteString(fmt.Sprintf("Closed by `pine close --evidence` on %s.\n\n", time.Now().UTC().Format("2006-01-02")))

	if !client.IsRepo(ctx) {
		b.WriteString("_(not a git repository — no file-change evidence available)_\n")
		return b.String()
	}

	t, err := s.Get(id)
	if err != nil {
		b.WriteString("_(ticket not found — no file-change evidence available)_\n")
		return b.String()
	}

	base := client.CommitBefore(ctx, t.Created)
	pathspec := path.Join(filepath.Base(s.Root()), "tickets", id+".md")
	commits := client.Log(ctx, pathspec, id, 50)
	stat := client.DiffStat(ctx, base)

	if base != "" {
		b.WriteString(fmt.Sprintf("- Base: `%s` (last commit at or before ticket created %s)\n", shortSHA(base), t.Created.Format("2006-01-02")))
	} else {
		b.WriteString("- Base: _(none — ticket predates git history or creation time unknown; showing uncommitted changes only)_\n")
	}

	hasCommits := len(commits) > 0
	hasStat := strings.TrimSpace(stat) != ""

	if hasCommits {
		b.WriteString(fmt.Sprintf("- Commits (%d):\n", len(commits)))
		for _, c := range commits {
			b.WriteString(fmt.Sprintf("  - `%s` — %s\n", c.Hash, c.Subject))
		}
	}
	if hasStat {
		b.WriteString("- Files changed (base → working tree):\n\n```\n")
		b.WriteString(stat)
		b.WriteString("\n```\n")
	}
	if !hasCommits && !hasStat {
		b.WriteString("- _(no file changes detected since ticket creation)_\n")
	}
	return b.String()
}

func shortSHA(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

// appendEvidence replaces any existing "## Work Evidence" section in the body
// with the new block, or appends it. Everything before the section is preserved
// so the ticket's original notes/description stay intact across re-runs.
func appendEvidence(body, evidence string) string {
	if idx := strings.Index(body, "## Work Evidence"); idx >= 0 {
		kept := strings.TrimRight(body[:idx], "\n")
		if kept == "" {
			return evidence
		}
		return kept + "\n\n" + evidence
	}
	if strings.TrimSpace(body) == "" {
		return evidence
	}
	return strings.TrimRight(body, "\n") + "\n\n" + evidence
}
